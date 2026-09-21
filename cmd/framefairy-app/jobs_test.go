package main

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/engine"
)

// The queue runs jobs on goroutines of its own while the window asks it
// things from another, so here everything it can be asked is asked at
// once. make test runs this with the race detector, which is the point of
// it: a queue that only works when nobody is looking is not a queue.
func TestTheQueueTakesEverythingAtOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	st := openStore()

	var events, broken atomic.Int64
	q := newQueue(st, func(u JobUpdate) {
		events.Add(1)
		// Every snapshot the window receives is a whole job.
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.Lane == "" || u.Job.State == "" {
			broken.Add(1)
		}
	}, func(string) {})

	// Work that says something and then waits to be told to stop, so jobs
	// are running while everything else happens to them.
	work := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().Info("%s", "working")
		p.Log().ProgressOf("working", 0.5, 1)
		<-ctx.Done()
		return "", engine.ErrCancelled
	}

	const hands, each = 6, 8
	var wg sync.WaitGroup
	ids := make(chan string, hands*each)

	for hand := 0; hand < hands; hand++ {
		wg.Add(1)
		go func(hand int) {
			defer wg.Done()
			kinds := []string{"transcribe", "plan", "render"}
			for k := 0; k < each; k++ {
				kind := kinds[(hand+k)%len(kinds)]
				job := q.add(filepath.Join(home, "ep.mp4"), kind, "Work", work)
				ids <- job.ID
			}
		}(hand)
	}
	// Everything else the window does while jobs are being added and run.
	for hand := 0; hand < hands; hand++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < each*4; k++ {
				q.list()
				q.find(filepath.Join(home, "ep.mp4"), "plan")
				if k%7 == 0 {
					q.clear()
				}
			}
		}()
	}
	// And a hand that stops jobs as soon as it hears of them.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < hands*each; i++ {
			q.cancel(<-ids)
		}
	}()
	wg.Wait()

	// Nothing may be left running: every job was asked to stop.
	deadline := time.Now().Add(20 * time.Second)
	for {
		busy := 0
		for _, j := range q.list() {
			if j.State == JobQueued || j.State == JobRunning {
				busy++
			}
		}
		if busy == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d jobs never stopped", busy)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if events.Load() == 0 {
		t.Error("the window was told nothing")
	}
	if broken.Load() != 0 {
		t.Errorf("%d job snapshots came out half written", broken.Load())
	}
}

// Stopping every job of an episode waits for the ones that are running, so
// the files can be deleted after it. It is asked from several places at
// once, because the window does not wait for one removal before the next.
func TestStoppingAnEpisodeWaitsForItsJobs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	st := openStore()
	q := newQueue(st, func(JobUpdate) {}, func(string) {})

	episode := filepath.Join(home, "ep.mp4")
	running := make(chan struct{}, 8)
	for i := 0; i < 4; i++ {
		q.add(episode, "plan", "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
			running <- struct{}{}
			<-ctx.Done()
			return "", engine.ErrCancelled
		})
	}
	// Wait until one of them really is running, so the stop has something
	// to wait for rather than finding everything still queued.
	select {
	case <-running:
	case <-time.After(10 * time.Second):
		t.Fatal("no job ever started")
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.cancelEpisode(episode)
		}()
	}
	wg.Wait()

	for _, j := range q.list() {
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("job %s is still %s", j.ID, j.State)
		}
	}
}
