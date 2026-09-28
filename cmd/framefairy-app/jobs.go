package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"framefairy/asr"
	"framefairy/engine"
)

// Job states.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobDone      = "done"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
	// JobInterrupted is a search or a render whose record says it was
	// running when the app last stopped: the app was closed or fell over
	// in the middle of it. Continue carries it on.
	JobInterrupted = "interrupted"
)

// Job is one engine step the interface asked for.
type Job struct {
	ID      string `json:"id"`
	Episode string `json:"episode"`
	// Kind is transcribe, plan or render.
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Error  string `json:"error,omitempty"`
	Result string `json:"result,omitempty"`
	// Plan and Clips say what a render is of, from the moment it is
	// queued, so the Render button of that clip can show it running.
	// Result only says it once the job is over. No clips means the whole
	// plan.
	Plan  string   `json:"plan,omitempty"`
	Clips []string `json:"clips,omitempty"`
	// Last is the most recent event, so a screen opened later still shows
	// where the job is.
	Last     *engine.Event `json:"last,omitempty"`
	Progress *engine.Event `json:"progress,omitempty"`
	Queued   time.Time     `json:"queued"`
	// Lane is "hearing", "finding" or "rendering". Each lane runs one job at
	// a time, so a long transcription never holds up finding or rendering
	// clips, and a render never holds up a search. A search moves from one
	// lane to the other as it goes from hearing to finding.
	Lane string `json:"lane"`
	// Step is where a search or a render is: waiting, hearing, finding or
	// rendering. See docs/JOBS.md.
	Step string `json:"step,omitempty"`
	// Record is the id of the record a search or a render keeps in the
	// episode's work folder, see engine/jobs.go.
	Record string `json:"record,omitempty"`
	// From and To are the window of a search, To 0 for the end of the
	// episode.
	From float64 `json:"from,omitempty"`
	To   float64 `json:"to,omitempty"`
	// At and Backward are a clip made by hand: the moment I or O was
	// pressed at, and whether it was O.
	At       float64 `json:"at,omitempty"`
	Backward bool    `json:"backward,omitempty"`
	// Underway is every clip the job has on the way, proposed and not
	// written yet, whoever proposed it, see engine.Underway. The clip list
	// shows each in its place until it is written.
	Underway []engine.Underway `json:"underway,omitempty"`
	// Seq grows with every change to any job, and is set under the queue's
	// lock, so of two snapshots of a job the later one has the larger
	// number. News is sent after the lock is let go, so two changes made
	// at nearly the same moment can reach the window the other way round:
	// a job that ran and finished read as queued again, for good. The
	// window keeps the snapshot with the larger number.
	Seq uint64 `json:"seq"`

	work func(ctx context.Context, p *engine.Project) (string, error)
	// steps is the work of a job that takes a turn in a lane for each of
	// its steps, a search or a render, instead of one for the whole job.
	steps  func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error)
	ask    *ask
	cancel context.CancelFunc
	ctx    context.Context
	// byHand is a search called off with Cancel, which says so where its
	// work was the way one cut off by the app closing does, see stopByHand.
	byHand bool
}

// JobUpdate is what the interface receives on the "job" event.
type JobUpdate struct {
	Job   Job           `json:"job"`
	Event *engine.Event `json:"event,omitempty"`
}

// Lanes the queue runs side by side, one for each kind of machinery a step
// uses: the speech model, the language model, ffmpeg encoding a short, and
// ffmpeg decoding the picture to place the crop of a clip made by hand. A
// search places its own clips' crops in its turn of finding.
const (
	LaneHearing   = "hearing"
	LaneFinding   = "finding"
	LaneRendering = "rendering"
	LaneFraming   = "framing"
)

// queue runs one job in each lane at a time. More would only make each of
// them slower. Every job runs on a goroutine of its own and waits for its
// turn in a lane, see lanes.go.
type queue struct {
	mu    sync.Mutex
	jobs  []*Job
	next  int
	lanes *lanes
	// idle is a queue that takes work and does none of it, see
	// newIdleQueue.
	idle   bool
	emit   func(JobUpdate)
	store  *store
	notify func(episode string)
	// closed counts, per episode, the removals under way. While an episode
	// is being removed nothing new is queued for it. A search that is told
	// to stop carries on the transcription it paused on its way out, and
	// the open workspace can ask for a search of its own, so without this
	// a job started after the removal began ran on for hours on an episode
	// that was no longer there, and the removal waited for it in vain.
	closed map[string]int
	// shut is set when the app quits. Nothing is queued after it.
	shut bool
	seq  uint64
}

// stampLocked marks a change to a job. q.mu is held.
func (q *queue) stampLocked(j *Job) {
	q.seq++
	j.Seq = q.seq
}

// newQueue builds the queue.
func newQueue(s *store, emit func(JobUpdate), notify func(string)) *queue {
	return &queue{emit: quietly(emit), store: s, notify: quietly(notify), closed: map[string]int{},
		lanes: newLanes()}
}

// newIdleQueue builds the queue without setting it running, so work can be
// asked for and nothing does it.
//
// It is here for the tests about what gets queued. Some of the work is a
// real download of half a gigabyte: a test that asks for one and lets the
// queue run is a test that needs the network, and on a build runner it is
// half a gigabyte an hour and a part file still being written when the
// test's own folder is taken away. That is exactly what happened, as
// "TempDir RemoveAll cleanup: directory not empty".
func newIdleQueue(s *store, emit func(JobUpdate), notify func(string)) *queue {
	q := newQueue(s, emit, notify)
	q.idle = true
	return q
}

// Which lane a kind of work runs in. Each model install shares the lane of
// the work that needs it, so whatever is queued behind it waits for the
// model rather than failing on it: the speech model with hearing, which
// cannot start without it, and the language model with finding clips.
// That way installing one does not hold up the other. A search starts in
// the lane of finding and moves as its steps go, see lanes.go.
func laneFor(kind string) string {
	switch kind {
	case "model":
		return LaneHearing
	case "render":
		return LaneRendering
	case engine.JobClip:
		return LaneFraming
	}
	return LaneFinding
}

// addOnce queues work unless the same work is already queued or running,
// in which case it hands back the job that is already there.
//
// Looking first and then adding is two locks with a gap between them, and
// two calls that arrive together both look, both see nothing, and both add.
// The interface can do that by being opened twice, or by a customer
// pressing a button twice, and the result is two transcriptions of one
// episode or two downloads writing over each other's unpacking folder. So
// the looking and the adding happen under one lock.
func (q *queue) addOnce(episode, kind, label string,
	work func(ctx context.Context, p *engine.Project) (string, error)) Job {
	return q.queue(episode, kind, label, true, work)
}

func (q *queue) add(episode, kind, label string,
	work func(ctx context.Context, p *engine.Project) (string, error)) Job {
	return q.queue(episode, kind, label, false, work)
}

// addFor queues work that is about some clips of a plan, and says so on
// the job from the start.
func (q *queue) addFor(episode, kind, label, plan string, clips []string,
	work func(ctx context.Context, p *engine.Project) (string, error)) Job {
	return q.queueFor(episode, kind, label, plan, clips, false, work)
}

func (q *queue) queue(episode, kind, label string, once bool,
	work func(ctx context.Context, p *engine.Project) (string, error)) Job {
	return q.queueFor(episode, kind, label, "", nil, once, work)
}

func (q *queue) queueFor(episode, kind, label, plan string, clips []string, once bool,
	work func(ctx context.Context, p *engine.Project) (string, error)) Job {
	lane := laneFor(kind)
	q.mu.Lock()
	if q.closed[episode] > 0 {
		q.mu.Unlock()
		return q.refuse(episode, kind, label, "the episode is being removed")
	}
	if q.shut {
		q.mu.Unlock()
		return q.refuse(episode, kind, label, "the app is closing")
	}
	if once {
		for i := len(q.jobs) - 1; i >= 0; i-- {
			j := q.jobs[i]
			if j.Episode == episode && j.Kind == kind &&
				(j.State == JobQueued || j.State == JobRunning) {
				already := *j
				q.mu.Unlock()
				return already
			}
		}
	}
	q.next++
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{ID: fmt.Sprintf("job-%d", q.next), Episode: episode, Kind: kind, Label: label,
		State: JobQueued, Queued: time.Now(), Lane: lane, work: work, cancel: cancel, ctx: ctx,
		Plan: plan, Clips: append([]string(nil), clips...)}
	// Its turn is asked for now, under the lock, so jobs of one lane run
	// in the order they were queued.
	job.ask = q.lanes.enqueue(lane, plainTurn)
	q.stampLocked(job)
	q.jobs = append(q.jobs, job)
	snapshot := *job
	idle := q.idle
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
	if !idle {
		go q.runSafely(job)
	}
	return snapshot
}

// addSteps queues a search or a render: work that takes a turn in a lane
// for each of its steps. It runs from the start, waiting for its first
// turn. With once, it hands back the job of the kind that already runs for
// the episode instead, looked for under the same lock, see addOnce.
func (q *queue) addSteps(episode, kind, label string, once bool, prepare func(*Job),
	steps func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error)) Job {
	q.mu.Lock()
	if q.closed[episode] > 0 {
		q.mu.Unlock()
		return q.refuse(episode, kind, label, "the episode is being removed")
	}
	if q.shut {
		q.mu.Unlock()
		return q.refuse(episode, kind, label, "the app is closing")
	}
	if once {
		for _, j := range q.jobs {
			// One that was called off and is still stopping is not it: New
			// pressed right after Cancel is a new search.
			if j.Episode == episode && j.Kind == kind && (j.State == JobQueued || j.State == JobRunning) &&
				j.ctx.Err() == nil {
				already := *j
				q.mu.Unlock()
				return already
			}
		}
	}
	q.next++
	ctx, cancel := context.WithCancel(context.Background())
	job := &Job{ID: fmt.Sprintf("job-%d", q.next), Episode: episode, Kind: kind, Label: label,
		State: JobRunning, Step: engine.StepWaiting, Queued: time.Now(), Lane: laneFor(kind),
		steps: steps, cancel: cancel, ctx: ctx}
	if prepare != nil {
		prepare(job)
	}
	q.stampLocked(job)
	q.jobs = append(q.jobs, job)
	snapshot := *job
	idle := q.idle
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
	if !idle {
		go q.runSafely(job)
	}
	return snapshot
}

// turn is how a job with steps waits for the lane of each. The job says
// which lane it waits for, so the interface can say what it waits to do,
// and which step it is in once it has its turn. How far the step before
// came is not how far this one is, so the progress starts again with it.
func (q *queue) turn(job *Job) engine.Turn {
	return func(ctx context.Context, step string) (context.Context, func(), error) {
		lane, kind := laneOfStep(step)
		q.update(job, nil, func(j *Job) { j.Step, j.Lane, j.Progress = engine.StepWaiting, lane, nil })
		stepCtx, release, err := q.lanes.take(ctx, lane, kind)
		if err != nil {
			return nil, nil, err
		}
		q.update(job, nil, func(j *Job) { j.Step, j.Lane, j.Progress = step, lane, nil })
		return stepCtx, release, nil
	}
}

// makesClips says whether a kind of job makes clips: a search and a clip
// made by hand. Called off by hand, such a job keeps its record and says
// Stopped with Continue, because what it did stays and can be carried on,
// and the clip list's Cancel and Continue take all of them at once.
func makesClips(kind string) bool {
	return kind == engine.JobSearch || kind == engine.JobClip
}

// jobLabel is what a job is called in Activity, by what it does.
func jobLabel(kind string, preview bool) string {
	switch {
	case kind == engine.JobRender && preview:
		return "Preview"
	case kind == engine.JobRender:
		return "Render"
	case kind == engine.JobClip:
		return "Make a clip"
	}
	return "Find clips"
}

// restore puts the searches and renders the episodes' records say were
// cut off or failed into the queue, as the app starts, so each says so
// where its work was. See engine/jobs.go.
func (q *queue) restore(episodes []string) {
	for _, episode := range episodes {
		for _, rec := range engine.ReadJobs(episode) {
			state := JobInterrupted
			if !rec.Interrupted() {
				state = JobFailed
			}
			q.mu.Lock()
			q.next++
			job := &Job{ID: fmt.Sprintf("job-%d", q.next), Episode: episode, Kind: rec.Kind,
				Label: jobLabel(rec.Kind, rec.Preview), State: state, Error: rec.Error, Queued: rec.Asked,
				Lane: laneFor(rec.Kind), Step: rec.Step, Record: rec.ID, From: rec.From, To: rec.To,
				At: rec.At, Backward: rec.Backward, Underway: rec.Underway(), Plan: rec.Plan,
				Clips: append([]string(nil), rec.Clips...), cancel: func() {}, ctx: context.Background()}
			q.stampLocked(job)
			q.jobs = append(q.jobs, job)
			snapshot := *job
			q.mu.Unlock()
			q.emit(JobUpdate{Job: snapshot})
		}
	}
}

// refuse records a job that was never started, so the interface hears why
// in the same place it hears everything else about its jobs.
func (q *queue) refuse(episode, kind, label, reason string) Job {
	lane := laneFor(kind)
	q.mu.Lock()
	q.next++
	job := &Job{ID: fmt.Sprintf("job-%d", q.next), Episode: episode, Kind: kind, Label: label,
		State: JobFailed, Error: reason, Queued: time.Now(), Lane: lane,
		cancel: func() {}, ctx: context.Background()}
	q.stampLocked(job)
	q.jobs = append(q.jobs, job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
	return snapshot
}

// busy says whether any job runs or waits to.
func (q *queue) busy() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.State == JobQueued || j.State == JobRunning {
			return true
		}
	}
	return false
}

func (q *queue) list() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, len(q.jobs))
	for i, j := range q.jobs {
		out[i] = *j
	}
	return out
}

func (q *queue) cancel(id string) {
	q.mu.Lock()
	var job *Job
	for _, j := range q.jobs {
		if j.ID == id {
			job = j
		}
	}
	if job == nil {
		q.mu.Unlock()
		return
	}
	job.cancel()
	if job.State == JobQueued {
		job.State = JobCancelled
	}
	// Always said, also for a job that had already ended: the window asked
	// because it thinks the job is still going, and without an answer it
	// went on saying Cancelling for good.
	q.stampLocked(job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
}

// How long a job is given to stop after it is told to. A render has an
// ffmpeg to end and a transcription has what it has heard to save, so
// stopping is not instant.
var stopWait = 15 * time.Second

// closeEpisode stops every job of an episode and lets nothing new be
// queued for it until the reopen it hands back is called. Closing and
// cancelling happen under one lock, so no job slips in between the two.
func (q *queue) closeEpisode(episode string) (reopen func()) {
	q.mu.Lock()
	q.closed[episode]++
	var queued []Job
	for _, j := range q.jobs {
		if j.Episode != episode || (j.State != JobQueued && j.State != JobRunning) {
			continue
		}
		j.cancel()
		if j.State == JobQueued {
			j.State = JobCancelled
			q.stampLocked(j)
			queued = append(queued, *j)
		}
	}
	q.mu.Unlock()
	for _, j := range queued {
		q.emit(JobUpdate{Job: j})
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			q.mu.Lock()
			if q.closed[episode]--; q.closed[episode] <= 0 {
				delete(q.closed, episode)
			}
			q.mu.Unlock()
		})
	}
}

// openEpisode lets work be queued for an episode again. An episode that was
// removed stays closed until it is added again: the app checks that an
// episode is in the library and then queues, and a removal that finished
// between the two let a job in for an episode that was gone.
func (q *queue) openEpisode(episode string) {
	q.mu.Lock()
	delete(q.closed, episode)
	q.mu.Unlock()
}

// waitEpisode waits until no job of an episode is running any more, and
// says whether that happened within stopWait.
func (q *queue) waitEpisode(episode string) bool {
	deadline := time.Now().Add(stopWait)
	for {
		running := false
		for _, j := range q.list() {
			if j.Episode == episode && j.State == JobRunning {
				running = true
			}
		}
		if !running {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// shutDown stops every job and waits, at most stopWait, for the running
// ones to end. The app calls it on the way out. A render's ffmpeg and a
// transcription were left running after the app had gone, and a
// transcription keeps the file it writes open, so the next start found it
// half written by a program nobody could see.
func (q *queue) shutDown() bool {
	q.mu.Lock()
	q.shut = true
	for _, j := range q.jobs {
		if j.State == JobQueued || j.State == JobRunning {
			j.cancel()
			if j.State == JobQueued {
				j.State = JobCancelled
				q.stampLocked(j)
			}
		}
	}
	q.mu.Unlock()
	deadline := time.Now().Add(stopWait)
	for {
		running := false
		for _, j := range q.list() {
			if j.State == JobRunning {
				running = true
			}
		}
		if !running {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// cancelEpisode stops every job of an episode and waits until none of them
// is running any more. It says whether they all stopped, because what the
// caller does next is delete the episode's files and there is no safe way
// to do that while something is still writing them.
func (q *queue) cancelEpisode(episode string) bool {
	reopen := q.closeEpisode(episode)
	defer reopen()
	return q.waitEpisode(episode)
}

// stopByHand calls off a job that makes clips with Cancel, see makesClips.
// It ends as work that stopped, not as work that is gone, see runJob.
func (q *queue) stopByHand(id string) {
	q.mu.Lock()
	for _, j := range q.jobs {
		if j.ID == id && makesClips(j.Kind) && j.Record != "" {
			j.byHand = true
		}
	}
	q.mu.Unlock()
	q.cancel(id)
}

// settle marks the searches or renders of an episode that stopped, cut
// off or failed, as taken care of: carried on by a new job, or their note
// taken away. A record id picks one of them, and "" all of that kind.
func (q *queue) settle(episode, kind, record string) {
	q.mu.Lock()
	var settled []Job
	for _, j := range q.jobs {
		if j.Episode == episode && j.Kind == kind && (record == "" || j.Record == record) &&
			(j.State == JobInterrupted || (j.State == JobFailed && j.Record != "")) {
			j.State = JobCancelled
			q.stampLocked(j)
			settled = append(settled, *j)
		}
	}
	q.mu.Unlock()
	for _, j := range settled {
		q.emit(JobUpdate{Job: j})
	}
}

// waitJob waits, at most stopWait, until a job is no longer running.
func (q *queue) waitJob(id string) {
	deadline := time.Now().Add(stopWait)
	for time.Now().Before(deadline) {
		running := false
		for _, j := range q.list() {
			if j.ID == id && (j.State == JobRunning || j.State == JobQueued) {
				running = true
			}
		}
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// clear forgets finished jobs.
func (q *queue) clear() {
	q.mu.Lock()
	kept := q.jobs[:0]
	for _, j := range q.jobs {
		// A search or a render that stopped and has not been acted on is
		// not finished: it says so where its work was until it is.
		if j.State == JobQueued || j.State == JobRunning || j.State == JobInterrupted ||
			(j.State == JobFailed && j.Record != "") {
			kept = append(kept, j)
		}
	}
	q.jobs = kept
	q.mu.Unlock()
}

// find returns the newest job of a kind for an episode, if any.
func (q *queue) find(episode, kind string) (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := len(q.jobs) - 1; i >= 0; i-- {
		if j := q.jobs[i]; j.Episode == episode && j.Kind == kind {
			return *j, true
		}
	}
	return Job{}, false
}

// runSafely runs a job and keeps the lane alive whatever happens around
// it. The work itself is guarded by run, but the lane also sets the job
// up and reports on it, and a panic there ended the goroutine that is the
// lane: every job queued after it waited for ever, and the app looked
// frozen with nothing to say why.
func (q *queue) runSafely(job *Job) {
	defer func() {
		if caught := recover(); caught != nil {
			q.update(job, nil, func(j *Job) {
				if j.State == JobRunning {
					j.State = JobFailed
					j.Error = fmt.Sprintf("%s stopped unexpectedly: %v", j.Label, caught)
					j.Progress = nil
				}
			})
		}
	}()
	q.runJob(job)
}

// quietly is a function that never panics into its caller. What the queue
// hands its news to is the window's, and the queue must not die of it.
func quietly[T any](f func(T)) func(T) {
	if f == nil {
		return nil
	}
	return func(v T) {
		defer func() { _ = recover() }()
		f(v)
	}
}

func (q *queue) runJob(job *Job) {
	ctx := job.ctx
	if job.ask != nil {
		turnCtx, release, err := q.lanes.wait(job.ctx, job.ask)
		if err != nil {
			// Called off while it waited. Cancel has said so already.
			return
		}
		defer release()
		ctx = turnCtx
		// Called off in the moment its turn came, it does not start: it
		// said cancelled already, and nothing waits for a job that says
		// so, so work it did now would run on behind everyone's back.
		started := false
		q.update(job, nil, func(j *Job) {
			if j.State == JobQueued && j.ctx.Err() == nil {
				j.State = JobRunning
				started = true
			}
		})
		if !started {
			return
		}
	} else {
		q.update(job, nil, func(j *Job) {})
	}

	log := engine.NewLog(io.Discard, false, false)
	log.SetSink(func(ev engine.Event) {
		// Detail lines are for bug reports, not for the interface.
		if ev.Kind == engine.EventDetail {
			return
		}
		copied := ev
		if copied.Kind == engine.EventUnderway {
			q.update(job, nil, func(j *Job) { j.Underway = copied.Underway })
			return
		}
		q.update(job, &copied, func(j *Job) {
			j.Last = &copied
			if copied.Kind == engine.EventProgress {
				j.Progress = &copied
			} else if copied.Kind == engine.EventIdle || copied.Kind == engine.EventStepDone {
				j.Progress = nil
			}
		})
	})
	e := engine.NewEngine(log)
	opts := q.store.Settings().options()
	setUp(e, &opts)
	project := engine.NewProject(e, job.Episode, opts)

	result, err := run(ctx, job, project, q.turn(job))
	// A search called off by hand says so in its record before it says
	// it has ended, so the note is there whenever anyone looks, the app
	// quit a moment later included.
	q.mu.Lock()
	byHand := job.byHand
	q.mu.Unlock()
	if byHand && errors.Is(err, engine.ErrCancelled) {
		if serr := engine.StopJob(job.Episode, job.Record); serr != nil {
			project.Log().Warn("could not note the search as stopped: %s", serr)
		}
	}
	log.SetSink(nil)
	q.update(job, nil, func(j *Job) {
		j.Progress = nil
		j.Result = result
		if j.steps != nil && err == nil {
			j.Step = ""
		}
		switch {
		case err == nil:
			j.State, j.Underway = JobDone, nil
		case errors.Is(err, engine.ErrCancelled) && j.byHand:
			// Called off by hand, a search says Stopped, Click Continue,
			// because what it heard stays and it can be carried on.
			j.State, j.Step = JobInterrupted, engine.StepStopped
		case errors.Is(err, engine.ErrCancelled):
			j.State, j.Underway = JobCancelled, nil
		default:
			j.State = JobFailed
			j.Error = project.LastError()
			if j.Error == "" {
				j.Error = err.Error()
			}
		}
	})
	if q.notify != nil {
		q.notify(job.Episode)
	}
}

// setUp readies the engine and the options a job runs with: the speech
// model the app carries, unless a test has put stand-ins in standIns.
func setUp(e *engine.Engine, o *engine.Options) {
	if f := standIns.Load(); f != nil {
		(*f)(e, o)
		return
	}
	e.OpenRecognizer = asr.Open
}

// standIns is how the path tests hand the app a stand-in speech model and a
// stand-in language model and run everything else as it is. It is read by
// every job and written by the tests, from goroutines of their own, so it
// is atomic: a job still finishing from one test read it while the next
// test put its own in, which the race detector caught.
var standIns atomic.Pointer[func(*engine.Engine, *engine.Options)]

// run does the work of a job and turns a panic into a job that failed. A
// desktop app that dies takes the interface, the other lane and whatever
// was being transcribed with it, and one bad episode is not worth that.
func run(ctx context.Context, job *Job, project *engine.Project, turn engine.Turn) (result string, err error) {
	defer func() {
		if caught := recover(); caught != nil {
			err = fmt.Errorf("%s stopped unexpectedly: %v\n%s", job.Label, caught, debug.Stack())
		}
	}()
	if job.steps != nil {
		return job.steps(ctx, project, turn)
	}
	return job.work(ctx, project)
}

func (q *queue) update(job *Job, ev *engine.Event, change func(*Job)) {
	q.mu.Lock()
	change(job)
	q.stampLocked(job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot, Event: ev})
}
