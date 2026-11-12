package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine"
)

// Every search, render, clip made by hand and whole video heard goes
// through addSteps and takes a turn in a lane for each of its steps, and
// Search, Continue, Cancel, Stop, Remove, a search set aside, Relaunch and
// quitting all act on it from the window's goroutine while it runs. So
// here they all happen at once, over many seeds, in a bubble whose clock
// the steps rest on, for the race detector and for what must always hold.
// A seed picks what each hand does and when, but hands that wake at the
// same moment run in the order Go's scheduler picks, so a seed does not
// walk a failure again. TestContinueWhileTheOneBeforeStillStops walks the
// order that matters most, one step at a time. Issue #613: the queue's
// other race test drives add, which only the downloads of a model take.
//
// What must always hold:
//   - A search or the whole video heard that runs and was not called off
//     finds its own record as it wrote it, never forgotten, written over
//     by the one before or said to be stopped by one that ended before
//     it. That is Continue clicked while the one before still stops.
//   - Once the app quits, no job waits or runs, and none of their
//     goroutines is left.
//   - No lane is left held or asked for.
func TestTheStepPathUnderEverythingTheAppDoesAtOnce(t *testing.T) {
	t.Parallel()
	for seed := range uint64(24) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) { walkTheStepPath(t, seed) })
		})
	}
}

// Continue clicked while the search or the whole video heard before it
// still stops: the new one runs to its end on its own record, which the
// one before neither writes over on its way out nor says is stopped as it
// ends. Each step is taken once everything else in the bubble waits, so
// this is one order, the same on every run.
func TestContinueWhileTheOneBeforeStillStops(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{engine.JobSearch, engine.JobTranscribe} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				st := memoryStore()
				q := newQueue(st, func(JobUpdate) {}, func(string) {})
				logJobsOnFailure(t, q)
				hands := handsOn(q, &fakeJobs{t: t, st: st, about: kind})
				const ep = "/eps/a.mp4"
				ask := func(rest time.Duration) Job {
					if kind == engine.JobSearch {
						return hands.search(ep, "", rest)
					}
					return hands.transcribe(ep, rest)
				}

				// The first takes 10 ms to stop. The second rests 25 ms
				// in each of its steps, so it looks at its record again
				// well after the first has ended, never at the same
				// moment.
				first := ask(10 * time.Millisecond)
				synctest.Wait()
				q.stopByHand(first.ID)
				synctest.Wait()
				// Continue, while the first takes its moment to stop. A
				// search waits for it, so the press is a goroutine of its
				// own, the way the window's call is.
				var second Job
				pressed := make(chan struct{})
				go func() {
					defer close(pressed)
					second = ask(25 * time.Millisecond)
				}()
				<-pressed
				if second.ID == "" || second.ID == first.ID {
					t.Fatalf("Continue gave %+v after %s", second, first.ID)
				}
				q.waitJob(second.ID)
				for _, j := range q.list() {
					if j.ID == second.ID && j.State != JobDone {
						t.Errorf("the %s carried on ended %s, not done", kind, j.State)
					}
				}
				if recs := engine.ReadJobs(st.files, ep); len(recs) != 0 {
					t.Errorf("records left once the %s carried on was done: %+v", kind, recs)
				}
				if !q.shutDown() {
					t.Error("the jobs did not stop when the app quit")
				}
			})
		})
	}
}

func walkTheStepPath(t *testing.T, seed uint64) {
	st := memoryStore()
	q := newQueue(st, func(u JobUpdate) {
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.State == "" {
			t.Errorf("a job snapshot came out half written: %+v", u.Job)
		}
	}, func(string) {})
	logJobsOnFailure(t, q)
	episodes := []string{"/eps/a.mp4", "/eps/b.mp4"}
	hands := handsOn(q, &fakeJobs{t: t, st: st, about: fmt.Sprintf("seed %d", seed)})

	// Each hand does what the window can do, in an order its seed picks.
	var all sync.WaitGroup
	for hand := range 6 {
		r := rand.New(rand.NewPCG(seed, uint64(hand)))
		all.Go(func() {
			for k := range 12 {
				ep := episodes[r.IntN(len(episodes))]
				rest := time.Duration(1+r.IntN(40)) * time.Millisecond
				click := fmt.Sprintf("click-%d-%d", hand, k)
				switch r.IntN(11) {
				case 0, 1:
					hands.search(ep, click, rest)
				case 10, 2:
					hands.transcribe(ep, rest)
				case 3:
					hands.render(ep, rest)
				case 4:
					// Cancel at the head of the clip list.
					q.stopClips(ep, click)
				case 5:
					hands.stop(ep, r)
				case 6:
					// Remove: everything of the video called off and
					// waited for, how each ended forgotten, and the video
					// added again.
					reopen := q.closeEpisode(ep)
					q.waitEpisode(ep)
					q.forgetEpisode(ep)
					reopen()
				case 7:
					// The window moved off a stopped search, or back onto it.
					aside := r.IntN(2) == 0
					q.setAside(ep, func(*Job) bool { return aside })
				case 8:
					// Relaunch asks whether the app is idle, and does not
					// go after all.
					reopen, _ := q.closeIfIdle()
					reopen()
				case 9:
					q.list()
					q.find(ep, engine.JobSearch)
				}
				// The window does the next thing after a moment.
				time.Sleep(time.Duration(r.IntN(30)) * time.Millisecond) //nolint:forbidigo // synctest bubble
			}
		})
	}
	all.Wait()

	// Then the app quits.
	if !q.shutDown() {
		t.Errorf("seed %d: the jobs did not stop when the app quit", seed)
	}
	synctest.Wait()
	q.mu.Lock()
	for _, j := range q.jobs {
		if j.going() || j.alive {
			t.Errorf("seed %d: %s %s of %s is still %s after the app quit, its goroutine there %v", seed, j.ID, j.Kind, j.Episode, j.State, j.alive)
		}
	}
	q.mu.Unlock()
	q.lanes.mu.Lock()
	if len(q.lanes.held) != 0 || len(q.lanes.asks) != 0 {
		t.Errorf("seed %d: left behind in the lanes: %d held, %d asked", seed, len(q.lanes.held), len(q.lanes.asks))
	}
	q.lanes.mu.Unlock()
}

// windowHands is what the window asks of the queue for a video, through
// the queue's own makeWayForSearch and makeWayForTranscribe, with fake
// jobs in place of the engine's.
type windowHands struct {
	q    *queue
	jobs *fakeJobs

	mu      sync.Mutex
	renders int
}

func handsOn(q *queue, jobs *fakeJobs) *windowHands {
	return &windowHands{q: q, jobs: jobs}
}

// search is Search and Continue as FrameFairy.search asks the queue for
// them.
func (h *windowHands) search(ep, click string, rest time.Duration) Job {
	h.q.makeWayForSearch(ep)
	return h.q.addSteps(ep, engine.JobSearch, "Find clips", true, func(j *Job) {
		j.JobRecord.ID, j.click = engine.SearchID, click
	}, h.jobs.steps(ep, engine.SearchID, engine.JobSearch, []string{engine.StepHearing, engine.StepFinding}, rest))
}

// transcribe is Transcribe video and Continue transcribing as
// FrameFairy.Transcribe asks the queue for them. Refused, it gives no job.
func (h *windowHands) transcribe(ep string, rest time.Duration) Job {
	if !h.q.makeWayForTranscribe(ep) {
		return Job{}
	}
	return h.q.addSteps(ep, engine.JobTranscribe, "Transcribe whole video", true, func(j *Job) {
		j.JobRecord.ID = engine.TranscribeID
	}, h.jobs.steps(ep, engine.TranscribeID, engine.JobTranscribe, []string{engine.StepHearing, engine.StepHearing}, rest))
}

func (h *windowHands) render(ep string, rest time.Duration) {
	h.mu.Lock()
	h.renders++
	id := fmt.Sprintf("render-%d", h.renders)
	h.mu.Unlock()
	h.q.addSteps(ep, engine.JobRender, "Render", false, func(j *Job) { j.JobRecord.ID = id },
		h.jobs.steps(ep, id, engine.JobRender, []string{engine.StepRendering}, rest))
}

// stop is Cancel on a row, or Stop transcribing: one going job of the
// video called off by hand, which is where FrameFairy.CancelJob ends for
// a job that runs.
func (h *windowHands) stop(ep string, r *rand.Rand) {
	var going []Job
	for _, j := range h.q.list() {
		if j.Episode == ep && j.going() && j.ctx.Err() == nil {
			going = append(going, j)
		}
	}
	if len(going) > 0 {
		h.q.stopByHand(going[r.IntN(len(going))].ID)
	}
}

// fakeJobs makes jobs that stand in for the engine's Search,
// TranscribeJob and RenderJob, the way engine/jobs.go keeps a record.
type fakeJobs struct {
	t     *testing.T
	st    *store
	about string

	mu     sync.Mutex
	passes int
}

// steps is one job: it writes its record waiting, takes a turn for each
// step and writes the step, rests in it for a moment of the bubble's
// clock or until it is called off, and, done, takes its record away.
// Called off, it lets its turn go and takes a moment to stop, and its
// record stays as it was, but a search writes it once more at the end of
// that moment, as the engine's writes its record as it goes, see
// makeWayForSearch. Each job writes a pass of
// its own, so a record another job wrote is told from its own.
func (f *fakeJobs) steps(ep, id, kind string, names []string, rest time.Duration) func(context.Context, *engine.Project, engine.Host) (string, error) {
	f.mu.Lock()
	f.passes++
	pass := f.passes
	f.mu.Unlock()
	t, files := f.t, f.st.files
	return func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
		var mu sync.Mutex
		rec := engine.JobRecord{ID: id, Kind: kind, Step: engine.StepWaiting, Pass: pass}
		// over is set once the job has ended, after which it writes
		// nothing, as the engine's job does.
		over := false
		write := func() {
			if err := engine.WriteJob(files, ep, rec); err != nil {
				t.Errorf("writing the record of %s: %v", id, err)
			}
			host.Changed(rec)
		}
		// A search or the whole video heard that runs and was not called
		// off finds its record as it last wrote it. The look comes before
		// the context is asked, since calling off comes before another
		// job's end writes over it.
		mine := func() {
			if kind == engine.JobRender {
				return
			}
			var on *engine.JobRecord
			for _, r := range engine.ReadJobs(files, ep) {
				if r.ID == id {
					on = &r
				}
			}
			if (on == nil || on.Step != rec.Step || on.Pass != rec.Pass) && ctx.Err() == nil {
				t.Errorf("%s: a %s of %s that runs found its record %+v, it wrote %s in pass %d", f.about, kind, ep, on, rec.Step, pass)
			}
		}
		mu.Lock()
		write()
		mu.Unlock()
		release := func() {}
		defer func() { release() }()
		// Called off, it lets its turn go and takes a moment to stop, the
		// way a transcription saves what it heard and a render ends its
		// ffmpeg, and Continue may be clicked in that moment. A search
		// that had begun writes its record once more as it stops.
		stopping := func() (string, error) {
			mu.Lock()
			over = true
			stepped := rec.Step != engine.StepWaiting
			mu.Unlock()
			release()
			release = func() {}
			time.Sleep(rest) //nolint:forbidigo // synctest bubble
			if kind == engine.JobSearch && stepped {
				mu.Lock()
				write()
				mu.Unlock()
			}
			return "", engine.ErrCancelled
		}
		for i := 0; i < len(names); {
			if ctx.Err() != nil {
				return stopping()
			}
			waits := func() {
				mu.Lock()
				defer mu.Unlock()
				if !over && ctx.Err() == nil && rec.Step != engine.StepWaiting {
					rec.Step = engine.StepWaiting
					write()
				}
			}
			stepCtx, rel, err := host.Turn(ctx, names[i], waits)
			if err != nil {
				return stopping()
			}
			release = rel
			mu.Lock()
			mine()
			if stepCtx.Err() == nil {
				rec.Step = names[i]
				write()
			}
			mu.Unlock()
			select {
			case <-stepCtx.Done():
			case <-time.After(rest): //nolint:forbidigo // synctest bubble
			}
			// A turn taken back asks for the same step again.
			if stepCtx.Err() == nil {
				i++
			}
		}
		if ctx.Err() != nil {
			return stopping()
		}
		mu.Lock()
		defer mu.Unlock()
		over = true
		mine()
		if ctx.Err() != nil {
			return "", engine.ErrCancelled
		}
		if err := engine.RemoveJob(files, ep, id); err != nil {
			t.Errorf("taking the record of %s away: %v", id, err)
		}
		rec.Step = ""
		host.Changed(rec)
		return "ok", nil
	}
}
