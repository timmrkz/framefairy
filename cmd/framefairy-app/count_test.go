package main

import (
	"testing"

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
	job := d.svc.Search(ep, engine.PlanRequest{From: 0, To: 60, Count: 4, Min: 5, Replan: true})
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
