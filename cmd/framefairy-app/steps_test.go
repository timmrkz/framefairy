package main

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"framefairy/engine"
)

// Everything the window can do to searches, renders and clips made by
// hand, at once, from many goroutines, while they run: Search, I and O,
// Continue, Cancel, Render, the job list read and cleared. An episode never
// has two searches running, no two clips made by hand share an id, and when
// it is over nothing is left running.
//
// Each goroutine does a number of rounds, not as many as fit in some
// seconds, so the test does the same work on any machine. The one that
// presses Cancel waits for what it called off to stop, told by the queue,
// so its next round finds that work stopped and carries it on. Every change
// to a job is looked at for two searches of one episode, the moment it is
// made, until the drive is over.
func TestSearchesFromEverywhereAtOnce(t *testing.T) {
	d := open(t)
	a := d.add("a", minutes5)
	b := d.add("b", minutes5)
	d.idle(a)
	var plan string
	if clips := d.clips(a); len(clips) > 0 {
		plan = clips[0].Plan
	}
	most, over := 0, false
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		d.svc.jobs.until(func(jobs []*Job) bool {
			for _, ep := range []string{a, b} {
				running := 0
				for _, j := range jobs {
					// One that was called off and is still stopping does
					// not count.
					if j.Episode == ep && j.Kind == engine.JobSearch && j.going() && j.ctx.Err() == nil {
						running++
					}
				}
				most = max(most, running)
			}
			return over
		}, nil)
	}()
	const rounds = 10
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep := []string{a, b}[i%2]
			for n := range rounds {
				switch i % 5 {
				case 0:
					d.svc.Search(ep, engine.PlanRequest{From: 0, To: 60, Count: 1, Min: 5, Replan: true}, "")
				case 1:
					called := map[string]bool{}
					for _, j := range d.svc.Jobs() {
						if j.Episode == ep && (j.Kind == engine.JobSearch || j.Kind == engine.JobClip) {
							if j.State == JobInterrupted || j.State == JobFailed {
								d.svc.Continue(j.ID, "")
							} else {
								d.svc.CancelJob(j.ID)
								called[j.ID] = true
							}
						}
					}
					d.svc.jobs.until(func(jobs []*Job) bool {
						return !slices.ContainsFunc(jobs, func(j *Job) bool { return called[j.ID] && j.going() })
					}, nil)
				case 2:
					if plan != "" {
						d.svc.Render(a, engine.RenderRequest{Plan: plan, Preview: true})
					}
				case 3:
					d.svc.Jobs()
				case 4:
					d.svc.MakeClip(ep, float64(10+(n*37)%250), n%2 == 1)
				}
			}
		}()
	}
	wg.Wait()
	// The watcher hears that the drive is over the way it hears of every
	// change to a job.
	d.svc.jobs.mu.Lock()
	over = true
	d.svc.jobs.changed.wakeLocked()
	d.svc.jobs.mu.Unlock()
	<-watched
	if most > 1 {
		t.Errorf("%d searches of one episode at once", most)
	}
	for _, j := range d.svc.Jobs() {
		if j.State == JobRunning || j.State == JobQueued {
			d.svc.CancelJob(j.ID)
		}
	}
	d.idle(a)
	d.idle(b)
	for _, ep := range []string{a, b} {
		seen := map[string]bool{}
		for _, c := range d.clips(ep) {
			if seen[c.Key] {
				t.Errorf("%s twice in the list", c.Key)
			}
			seen[c.Key] = true
		}
	}
}

// Search and Cancel pressed one after the other reach the Go side in either
// order, each call on a goroutine of its own, the way Wails hands them
// over. Whichever comes first, the search ends Stopped with Continue, and
// none runs on. The model holds its answers, so a search that slipped
// through would still be running at the end rather than done. In every
// third round Cancel goes only once Search has answered, in every third
// Search goes only once Cancel has, and in the rest the two race, so both
// orders are taken for certain.
func TestNewAndCancelInEitherOrder(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	d.model.hangs(true)
	for i := range 40 {
		click := fmt.Sprintf("press-%d", i)
		var wg sync.WaitGroup
		start := make(chan struct{})
		pressed, stopped := make(chan struct{}), make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if i%3 == 2 {
				<-stopped
			}
			d.press(ep, window{0, 120}, click)
			close(pressed)
		}()
		go func() {
			defer wg.Done()
			<-start
			if i%3 == 1 {
				<-pressed
			}
			d.stop(ep, click)
			close(stopped)
		}()
		close(start)
		wg.Wait()
		d.idle(ep)
		if o := d.outcome(ep); !o.stopped {
			t.Fatalf("press %d: New and Cancel at once leave the search saying %+v", i, o)
		}
	}
}
