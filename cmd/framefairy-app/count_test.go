package main

import (
	"testing"
	"time"

	"framefairy/engine"
)

// A search says how many clips it was asked for, from the moment it is
// asked for to the moment it is over, so the clip list holds that many rows
// open, not what the workspace would ask for now. The first search of an
// episode is asked for by the Go side, and the workspace worked out
// another number and opened three rows for a search that looked for six.
func TestASearchSaysHowManyItWasAskedFor(t *testing.T) {
	d := open(t)
	ep := d.add("a", minutes5)
	d.idle(ep)
	job := d.svc.Search(ep, engine.PlanRequest{From: 0, To: 60, Count: 4, Min: 5, Replan: true}, "")
	if job.Count != 4 {
		t.Fatalf("the search was asked for 4 and says %d", job.Count)
	}
	d.idle(ep)
	for _, j := range d.svc.Jobs() {
		if j.ID == job.ID && j.Count != 4 {
			t.Fatalf("the search, %s, says it was asked for %d", j.State, j.Count)
		}
	}
}

// A search the queue will not start says which window it was asked for,
// so its row and its Continue are about that window. It wrote no record,
// so Continue does not take up the one on disk, which is an older
// search's, about another window.
func TestARefusedSearchKeepsItsWindow(t *testing.T) {
	d := open(t)
	ep := d.add("a", minutes5)
	d.idle(ep)
	// An older search of another window, stopped, left on disk.
	older := engine.JobRecord{ID: engine.SearchID, Kind: engine.JobSearch, From: 0, To: 60, Count: 1,
		Min: 5, Step: engine.StepStopped, Asked: time.Now()}
	if err := engine.WriteJob(ep, older); err != nil {
		t.Fatal(err)
	}
	d.svc.jobs.shutDown()
	refused := d.svc.Search(ep, engine.PlanRequest{From: 120, To: 180, Count: 2, Min: 5}, "")
	if refused.State != JobFailed || refused.Record != "" {
		t.Fatalf("the search was %s with record %q, not refused", refused.State, refused.Record)
	}
	if refused.From != 120 || refused.To != 180 || refused.Count != 2 {
		t.Fatalf("the refused search says %.0f to %.0f for %d", refused.From, refused.To, refused.Count)
	}
	again := d.svc.Continue(refused.ID, "")
	if again.From != 120 || again.To != 180 {
		t.Fatalf("Continue asked for %.0f to %.0f, not the window that was refused", again.From, again.To)
	}
}
