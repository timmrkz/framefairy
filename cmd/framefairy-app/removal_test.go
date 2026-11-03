package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine"
)

// stoppedSearches is a library of videos with a search each that
// stopped, read back as the app opens, and the ids of their jobs.
func stoppedSearches(t *testing.T, names ...string) (*FrameFairy, []string, []string) {
	t.Helper()
	s, eps := memoryLibrary(t, names...)
	s.jobs = newQueue(s.store, func(JobUpdate) {}, func(string) {})
	for _, ep := range eps {
		rec := engine.JobRecord{ID: engine.SearchID, Kind: engine.JobSearch, To: 60, Count: 2, Min: 5,
			Step: engine.StepStopped, Asked: time.Now()}
		if err := engine.WriteJob(s.store.files, ep, rec); err != nil {
			t.Fatal(err)
		}
	}
	s.jobs.restore(eps)
	var ids []string
	for _, j := range s.jobs.list() {
		ids = append(ids, j.ID)
	}
	return s, eps, ids
}

// removalOf is the state job id is in now, and how its removal stands.
func removalOf(s *FrameFairy, id string) (string, *JobAway) {
	for _, j := range s.jobs.list() {
		if j.ID == id {
			return j.State, j.Away
		}
	}
	return "", nil
}

// Work that stopped and was removed is forgotten, its record and all, once
// its time is up, which Go keeps, so it runs on whichever video is open.
// Held by a hand on its row, the time stands still, and put back, the work
// stays as it was. The window once kept the time and called the work off.
func TestRemovedWorkIsForgottenWhenItsTimeIsUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, eps, ids := stoppedSearches(t, "removed.mp4", "held.mp4", "back.mp4", "settled.mp4")
		removed, held, back, settled := ids[0], ids[1], ids[2], ids[3]
		for _, id := range ids {
			_, _ = s.RemoveWork(context.Background(), id)
		}
		if state, away := removalOf(s, removed); state != JobStopped || away == nil || away.Until == 0 || away.Whole != removalTime.Seconds() {
			t.Fatalf("removed, the search is %s with %+v", state, away)
		}
		_, _ = s.PutWorkBack(context.Background(), back)
		// Settled before its time is up, as a search carried on by a new
		// one is, whose record it is by then.
		s.jobs.settle(eps[3], engine.JobSearch, "")
		time.Sleep(removalTime / 2) //nolint:forbidigo // synctest bubble
		s.HoldRemoval(held, true, 1)
		if _, away := removalOf(s, held); away == nil || away.Until != 0 || away.Left != (removalTime/2).Seconds() {
			t.Errorf("held halfway, the removal is %+v", away)
		}
		time.Sleep(removalTime) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if state, _ := removalOf(s, removed); state != JobSettled {
			t.Errorf("once its time was up, the removed search is %s", state)
		}
		if engine.ReadSearch(s.store.files, eps[0]) != nil {
			t.Error("the removed search's record is still there")
		}
		if state, _ := removalOf(s, held); state != JobStopped {
			t.Errorf("held, the search is %s", state)
		}
		if state, away := removalOf(s, back); state != JobStopped || away != nil {
			t.Errorf("put back, the search is %s with %+v", state, away)
		}
		s.HoldRemoval(held, false, 2)
		time.Sleep(removalTime/2 - time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if state, _ := removalOf(s, held); state != JobStopped {
			t.Errorf("let go, the search was forgotten before its time left was up: %s", state)
		}
		time.Sleep(time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if state, _ := removalOf(s, held); state != JobSettled {
			t.Errorf("let go, the search is %s once its time left was up", state)
		}
		if engine.ReadSearch(s.store.files, eps[2]) == nil {
			t.Error("the search put back lost its record")
		}
		if state, away := removalOf(s, settled); state != JobSettled || away != nil || engine.ReadSearch(s.store.files, eps[3]) == nil {
			t.Errorf("settled before its time was up, the search is %s with %+v, and its record went", state, away)
		}
	})
}

// Everything the window can do to removed work at once, from several
// goroutines, while the jobs are listed: removed, held, let go, put back
// and removed again. Once all is still, each job is either stopped with
// its removal as the last call left it, or forgotten.
func TestRemovalsAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, ids := stoppedSearches(t, "a.mp4", "b.mp4", "c.mp4")
		var all sync.WaitGroup
		var press atomic.Int64
		for range 4 {
			for _, id := range ids {
				all.Go(func() { _, _ = s.RemoveWork(context.Background(), id) })
				all.Go(func() { s.HoldRemoval(id, true, int(press.Add(1))) })
				all.Go(func() { s.HoldRemoval(id, false, int(press.Add(1))) })
				all.Go(func() { _, _ = s.PutWorkBack(context.Background(), id) })
				all.Go(func() { s.jobs.list() })
			}
		}
		all.Wait()
		for _, id := range ids {
			_, _ = s.PutWorkBack(context.Background(), id)
			_, _ = s.RemoveWork(context.Background(), id)
		}
		time.Sleep(2 * removalTime) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		for _, id := range ids {
			if state, away := removalOf(s, id); state != JobSettled || away != nil {
				t.Errorf("removed last, the job %s is %s with %+v", id, state, away)
			}
		}
		s.jobs.mu.Lock()
		defer s.jobs.mu.Unlock()
		if len(s.jobs.removals) != 0 {
			t.Errorf("%d timers were left behind", len(s.jobs.removals))
		}
	})
}

// A let go that arrives before the hold it follows, as two calls can, is
// taken as the later of the two: the hold, older, is left out, and the
// time runs on. Taken in the order they came, the time stood held for
// good and the row said Removed until a hand passed over it again.
func TestALetGoThatOvertakesItsHoldLetsTheTimeRunOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, ids := stoppedSearches(t, "overtaken.mp4")
		id := ids[0]
		_, _ = s.RemoveWork(context.Background(), id)
		s.HoldRemoval(id, false, 2)
		s.HoldRemoval(id, true, 1)
		if _, away := removalOf(s, id); away == nil || away.Until == 0 {
			t.Fatalf("the hold that came last but was older held the removal: %+v", away)
		}
		time.Sleep(removalTime) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if state, _ := removalOf(s, id); state != JobSettled {
			t.Errorf("once its time was up, the search is %s", state)
		}
	})
}
