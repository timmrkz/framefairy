package main

import (
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// Everything the window can do to searches and renders, at once, from many
// goroutines, while they run: New, Continue, Cancel, Render, the job list
// read and cleared. An episode never has two searches running, and when it
// is over nothing is left running.
func TestSearchesFromEverywhereAtOnce(t *testing.T) {
	d := open(t)
	a := d.add("a", minutes5)
	b := d.add("b", minutes5)
	d.idle(a)
	var plan string
	if clips := d.clips(a); len(clips) > 0 {
		plan = clips[0].Plan
	}
	stop := time.Now().Add(3 * time.Second)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep := []string{a, b}[i%2]
			for time.Now().Before(stop) {
				switch i % 4 {
				case 0:
					d.svc.Search(ep, engine.PlanRequest{From: 0, To: 60, Count: 1, Min: 5, Replan: true})
				case 1:
					for _, j := range d.svc.Jobs() {
						if j.Episode == ep && j.Kind == engine.JobSearch {
							if j.State == JobInterrupted || j.State == JobFailed {
								d.svc.Continue(j.ID)
							} else {
								d.svc.CancelJob(j.ID)
							}
						}
					}
				case 2:
					if plan != "" {
						d.svc.Render(a, engine.RenderRequest{Plan: plan, Preview: true})
					}
					d.svc.ClearJobs()
				case 3:
					running := 0
					for _, j := range d.svc.Jobs() {
						// One that was called off and is still stopping
						// does not count.
						if j.Episode == ep && j.Kind == engine.JobSearch &&
							(j.State == JobRunning || j.State == JobQueued) && j.ctx.Err() == nil {
							running++
						}
					}
					if running > 1 {
						t.Errorf("%d searches of one episode at once", running)
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
	for _, j := range d.svc.Jobs() {
		if j.State == JobRunning || j.State == JobQueued {
			d.svc.CancelJob(j.ID)
		}
	}
	d.idle(a)
	d.idle(b)
}
