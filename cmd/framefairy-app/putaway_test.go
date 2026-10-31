package main

import (
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// The row of a search that stopped can be removed, issue #431. Once its
// time to be put back has run out the window calls CancelJob, and the
// search settles, its record goes, and after a restart it is not back. That
// holds for a search that failed, one cut off or stopped by hand, and one
// the queue refused, which never wrote a record. Removed from several
// places at once, while the job list is read, each says it settled.
func TestARemovedStoppedSearchIsGoneForGood(t *testing.T) {
	s, eps := memoryLibrary(t, "failed.mp4", "stopped.mp4", "refused.mp4")
	var mu sync.Mutex
	settled := map[string]int{}
	s.jobs = newQueue(s.store, func(u JobUpdate) {
		if u.Job.State == JobCancelled {
			mu.Lock()
			settled[u.Job.ID]++
			mu.Unlock()
		}
	}, func(string) {})

	write := func(ep, step, why string) {
		t.Helper()
		rec := engine.JobRecord{ID: engine.SearchID, Kind: engine.JobSearch, From: 0, To: 60, Count: 2,
			Min: 5, Step: step, Error: why, Asked: time.Now()}
		if err := engine.WriteJob(s.store.files, ep, rec); err != nil {
			t.Fatal(err)
		}
	}
	write(eps[0], engine.StepFailed, "could not reach Anthropic")
	write(eps[1], engine.StepStopped, "")
	s.jobs.restore(eps[:2])
	refused := s.jobs.refuse(eps[2], engine.JobSearch, "Find clips", "the app is closing")

	var ids []string
	for _, j := range s.jobs.list() {
		if j.State == JobFailed || j.State == JobInterrupted {
			ids = append(ids, j.ID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("%d searches stopped, want 3: %+v", len(ids), s.jobs.list())
	}

	var wg sync.WaitGroup
	for range 4 {
		for _, id := range ids {
			wg.Add(2)
			go func() {
				defer wg.Done()
				s.CancelJob(id)
			}()
			go func() {
				defer wg.Done()
				s.Jobs()
			}()
		}
	}
	wg.Wait()

	for _, j := range s.jobs.list() {
		if j.State != JobCancelled {
			t.Errorf("%s of %s is %s after its row was removed", j.ID, j.Episode, j.State)
		}
	}
	mu.Lock()
	for _, id := range ids {
		if settled[id] == 0 {
			t.Errorf("%s was never said to settle", id)
		}
	}
	mu.Unlock()
	if refused.JobRecord.ID != "" {
		t.Errorf("the refused search has the record %q", refused.JobRecord.ID)
	}
	for _, ep := range eps {
		if recs := engine.ReadJobs(s.store.files, ep); len(recs) != 0 {
			t.Errorf("%s still has %d records", ep, len(recs))
		}
	}

	// The app starts again and finds nothing to say about them.
	again := newQueue(s.store, func(JobUpdate) {}, func(string) {})
	again.restore(eps)
	if left := again.list(); len(left) != 0 {
		t.Errorf("after a restart %d searches are back: %+v", len(left), left)
	}
}
