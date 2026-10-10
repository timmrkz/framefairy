package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// Everything the window can do to searches, renders and clips made by
// hand, at once, from many goroutines, while they run: New, I and O,
// Continue, Cancel, Render, the job list read and cleared. An episode never
// has two searches running, no two clips made by hand share an id, and when
// it is over nothing is left running.
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
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ep := []string{a, b}[i%2]
			for n := 0; time.Now().Before(stop); n++ {
				switch i % 5 {
				case 0:
					d.svc.Search(ep, engine.PlanRequest{From: 0, To: 60, Count: 1, Min: 5, Replan: true}, "")
				case 1:
					for _, j := range d.svc.Jobs() {
						if j.Episode == ep && (j.Kind == engine.JobSearch || j.Kind == engine.JobClip) {
							if j.State == JobInterrupted || j.State == JobFailed {
								d.svc.Continue(j.ID, "")
							} else {
								d.svc.CancelJob(j.ID)
							}
						}
					}
				case 2:
					if plan != "" {
						d.svc.Render(a, engine.RenderRequest{Plan: plan, Preview: true})
					}
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
				case 4:
					d.svc.MakeClip(ep, float64(10+(n*37)%250), n%2 == 1)
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

// New and Cancel pressed one after the other reach the Go side in either
// order, each call on a goroutine of its own, the way Wails hands them
// over. Whichever comes first, the search ends Stopped with Continue, and
// none runs on. The model holds its answers, so a search that slipped
// through would still be running at the end rather than done.
func TestNewAndCancelInEitherOrder(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	d.model.hangs(true)
	for i := range 40 {
		click := fmt.Sprintf("press-%d", i)
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			d.press(ep, window{0, 120}, click)
		}()
		go func() {
			defer wg.Done()
			<-start
			if i%2 == 1 {
				time.Sleep(time.Millisecond)
			}
			d.stop(ep, click)
		}()
		close(start)
		wg.Wait()
		d.idle(ep)
		if o := d.outcome(ep); !o.stopped {
			t.Fatalf("press %d: New and Cancel at once leave the search saying %+v", i, o)
		}
	}
}
