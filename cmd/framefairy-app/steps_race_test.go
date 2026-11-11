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
// here they all happen at once, over many seeds, each seed one order of
// the hands in a bubble whose clock the steps rest on, so a failure is
// walked again by its seed. Issue #613: the queue's other race test drives
// add, which only the downloads of a model take.
//
// What must always hold:
//   - A search or the whole video heard that runs and was not called off
//     finds its own record as it wrote it, never forgotten or said to be
//     stopped by one that ended before it. That is Continue clicked while
//     the one before still stops.
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

func walkTheStepPath(t *testing.T, seed uint64) {
	st := memoryStore()
	q := newQueue(st, func(u JobUpdate) {
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.State == "" {
			t.Errorf("a job snapshot came out half written: %+v", u.Job)
		}
	}, func(string) {})
	logJobsOnFailure(t, q)
	episodes := []string{"/eps/a.mp4", "/eps/b.mp4"}

	// steps stands in for the engine's Search and RenderJob, the way
	// engine/jobs.go keeps a record: it writes its record waiting, takes a
	// turn for each step and writes the step, rests in it for a moment of
	// the bubble's clock or until it is called off, and, done, takes its
	// record away. Called off, its record stays as it was.
	steps := func(ep, id, kind string, names []string, rest time.Duration) func(context.Context, *engine.Project, engine.Host) (string, error) {
		return func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
			var mu sync.Mutex
			rec := engine.JobRecord{ID: id, Kind: kind, Step: engine.StepWaiting}
			// over is set once the job has ended, after which it writes
			// nothing, as the engine's job does.
			over := false
			write := func() {
				if err := engine.WriteJob(st.files, ep, rec); err != nil {
					t.Errorf("writing the record of %s: %v", id, err)
				}
				host.Changed(rec)
			}
			// A search or the whole video heard that runs and was not
			// called off finds its record as it last wrote it. The look comes before the context is
			// asked, since calling off comes before another job's end
			// writes over it.
			mine := func() {
				if kind == engine.JobRender {
					return
				}
				var on *engine.JobRecord
				for _, r := range engine.ReadJobs(st.files, ep) {
					if r.ID == id {
						on = &r
					}
				}
				if (on == nil || on.Step != rec.Step) && ctx.Err() == nil {
					t.Errorf("seed %d: a %s of %s that runs found its record %+v, it wrote %s", seed, kind, ep, on, rec.Step)
				}
			}
			mu.Lock()
			write()
			mu.Unlock()
			release := func() {}
			defer func() { release() }()
			// Called off, it takes a moment to stop, the way a
			// transcription saves what it heard and a render ends its
			// ffmpeg, and Continue may be clicked in that moment.
			stopping := func() (string, error) {
				mu.Lock()
				over = true
				mu.Unlock()
				time.Sleep(rest) //nolint:forbidigo // synctest bubble
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
			if err := engine.RemoveJob(st.files, ep, id); err != nil {
				t.Errorf("taking the record of %s away: %v", id, err)
			}
			rec.Step = ""
			host.Changed(rec)
			return "ok", nil
		}
	}

	// search is Search and Continue as FrameFairy.search does them on the
	// queue: a search still on its way out is waited for, one that
	// stopped is settled, and the new one is asked for once.
	search := func(ep, click string, rest time.Duration) {
		for _, j := range q.list() {
			if j.Episode == ep && j.Kind == engine.JobSearch && j.State == JobRunning && j.ctx.Err() != nil {
				q.waitJob(j.ID)
			}
			// The whole video heard stops for the work on its clips, see
			// stopTranscribing.
			if j.Episode == ep && j.Kind == engine.JobTranscribe && j.going() {
				q.stopByHand(j.ID)
			}
		}
		q.settle(ep, engine.JobSearch, "")
		q.addSteps(ep, engine.JobSearch, "Find clips", true, func(j *Job) {
			j.JobRecord.ID, j.click = engine.SearchID, click
		}, steps(ep, engine.SearchID, engine.JobSearch, []string{engine.StepHearing, engine.StepFinding}, rest))
	}
	// transcribe is Transcribe video and Continue transcribing as
	// FrameFairy.Transcribe does them: refused while the clips are worked
	// on, and asked for at once, while the one before may still stop.
	transcribe := func(ep string, rest time.Duration) {
		for _, j := range q.list() {
			if j.Episode == ep && makesClips(j.Kind) && j.going() {
				return
			}
		}
		q.settle(ep, engine.JobTranscribe, "")
		q.addSteps(ep, engine.JobTranscribe, "Transcribe whole video", true, func(j *Job) {
			j.JobRecord.ID = engine.TranscribeID
		}, steps(ep, engine.TranscribeID, engine.JobTranscribe, []string{engine.StepHearing, engine.StepHearing}, rest))
	}
	renders := 0
	var renderMu sync.Mutex
	render := func(ep string, rest time.Duration) {
		renderMu.Lock()
		renders++
		id := fmt.Sprintf("render-%d", renders)
		renderMu.Unlock()
		q.addSteps(ep, engine.JobRender, "Render", false, func(j *Job) { j.JobRecord.ID = id },
			steps(ep, id, engine.JobRender, []string{engine.StepRendering}, rest))
	}
	// stop is Cancel on a row, or Stop transcribing: one going job of the
	// video, called off by hand.
	stop := func(ep string, r *rand.Rand) {
		var going []Job
		for _, j := range q.list() {
			if j.Episode == ep && j.going() && j.ctx.Err() == nil {
				going = append(going, j)
			}
		}
		if len(going) > 0 {
			q.stopByHand(going[r.IntN(len(going))].ID)
		}
	}

	// Each hand does what the window can do, in an order its seed picks.
	var hands sync.WaitGroup
	for hand := range 6 {
		r := rand.New(rand.NewPCG(seed, uint64(hand)))
		hands.Go(func() {
			for k := range 12 {
				ep := episodes[r.IntN(len(episodes))]
				rest := time.Duration(1+r.IntN(40)) * time.Millisecond
				click := fmt.Sprintf("click-%d-%d", hand, k)
				switch r.IntN(11) {
				case 0, 1:
					search(ep, click, rest)
				case 10, 2:
					transcribe(ep, rest)
				case 3:
					render(ep, rest)
				case 4:
					// Cancel at the head of the clip list.
					q.stopClips(ep, click)
				case 5:
					stop(ep, r)
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
	hands.Wait()

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
