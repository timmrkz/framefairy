package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
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
	// Lane is "transcribe" or "work". Each lane runs one job at a time, so a
	// long transcription never holds up finding or rendering clips.
	Lane string `json:"lane"`

	work   func(ctx context.Context, p *engine.Project) (string, error)
	cancel context.CancelFunc
	ctx    context.Context
}

// JobUpdate is what the interface receives on the "job" event.
type JobUpdate struct {
	Job   Job           `json:"job"`
	Event *engine.Event `json:"event,omitempty"`
}

// Lanes the queue runs side by side.
const (
	LaneTranscribe = "transcribe"
	LaneWork       = "work"
)

// queue runs one transcription and one other job at a time. More would
// only make each of them slower.
type queue struct {
	mu     sync.Mutex
	jobs   []*Job
	next   int
	wake   map[string]chan struct{}
	emit   func(JobUpdate)
	store  *store
	notify func(episode string)
}

// newQueue builds the queue and sets its lanes running.
func newQueue(s *store, emit func(JobUpdate), notify func(string)) *queue {
	q := newIdleQueue(s, emit, notify)
	for lane := range q.wake {
		go q.loop(lane)
	}
	return q
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
	return &queue{emit: emit, store: s, notify: notify, wake: map[string]chan struct{}{
		LaneTranscribe: make(chan struct{}, 1), LaneWork: make(chan struct{}, 1)}}
}

// Which lane a kind of work runs in. Each model install shares the lane of
// the work that needs it, so whatever is queued behind it waits for the
// model rather than failing on it: the speech model with transcribing,
// which cannot start without it, and the language model with finding
// clips. That way installing one does not hold up the other.
func laneFor(kind string) string {
	switch kind {
	case "transcribe", "model":
		return LaneTranscribe
	}
	return LaneWork
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
	q.jobs = append(q.jobs, job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
	select {
	case q.wake[lane] <- struct{}{}:
	default:
	}
	return snapshot
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
	q.jobs = append(q.jobs, job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot})
	return snapshot
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
	queued := job.State == JobQueued
	if queued {
		job.State = JobCancelled
	}
	snapshot := *job
	q.mu.Unlock()
	if queued {
		q.emit(JobUpdate{Job: snapshot})
	}
}

// How long a job is given to stop after it is told to. A render has an
// ffmpeg to end and a transcription has what it has heard to save, so
// stopping is not instant.
var stopWait = 15 * time.Second

// cancelEpisode stops every job of an episode and waits until none of them
// is running any more. It says whether they all stopped, because what the
// caller does next is delete the episode's files and there is no safe way
// to do that while something is still writing them.
func (q *queue) cancelEpisode(episode string) bool {
	for _, j := range q.list() {
		if j.Episode == episode && (j.State == JobQueued || j.State == JobRunning) {
			q.cancel(j.ID)
		}
	}
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

// clear forgets finished jobs.
func (q *queue) clear() {
	q.mu.Lock()
	kept := q.jobs[:0]
	for _, j := range q.jobs {
		if j.State == JobQueued || j.State == JobRunning {
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

func (q *queue) take(lane string) *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.jobs {
		if j.State == JobQueued && j.Lane == lane {
			j.State = JobRunning
			return j
		}
	}
	return nil
}

func (q *queue) loop(lane string) {
	for {
		job := q.take(lane)
		if job == nil {
			<-q.wake[lane]
			continue
		}
		q.runJob(job)
	}
}

func (q *queue) runJob(job *Job) {
	q.update(job, nil, func(j *Job) {})

	log := engine.NewLog(io.Discard, false, false)
	log.SetSink(func(ev engine.Event) {
		// Detail lines are for bug reports, not for the interface.
		if ev.Kind == engine.EventDetail {
			return
		}
		copied := ev
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
	e.OpenRecognizer = asr.Open
	project := engine.NewProject(e, job.Episode, q.store.Settings().options())

	result, err := run(job, project)
	log.SetSink(nil)
	q.update(job, nil, func(j *Job) {
		j.Progress = nil
		j.Result = result
		switch {
		case err == nil:
			j.State = JobDone
		case errors.Is(err, engine.ErrCancelled):
			j.State = JobCancelled
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

// run does the work of a job and turns a panic into a job that failed. A
// desktop app that dies takes the interface, the other lane and whatever
// was being transcribed with it, and one bad episode is not worth that.
func run(job *Job, project *engine.Project) (result string, err error) {
	defer func() {
		if caught := recover(); caught != nil {
			err = fmt.Errorf("%s stopped unexpectedly: %v\n%s", job.Label, caught, debug.Stack())
		}
	}()
	return job.work(job.ctx, project)
}

func (q *queue) update(job *Job, ev *engine.Event, change func(*Job)) {
	q.mu.Lock()
	change(job)
	snapshot := *job
	q.mu.Unlock()
	q.emit(JobUpdate{Job: snapshot, Event: ev})
}
