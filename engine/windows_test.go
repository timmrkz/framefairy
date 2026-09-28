package engine

import (
	"fmt"
	"testing"
)

func TestWhereTheModelHasLooked(t *testing.T) {
	plans := []PlanSummary{
		{From: 600, To: 1200},
		// Two passes that meet become one part.
		{From: 1200, To: 1800},
		{From: 120, To: 300},
	}
	searched := SearchedWindows(plans, 3600)
	if fmt.Sprint(searched) != "[{120 300} {600 1800}]" {
		t.Fatalf("searched %v", searched)
	}
	free := FreeWindows(searched, 3600, 30)
	if fmt.Sprint(free) != "[{0 120} {300 600} {1800 3600}]" {
		t.Fatalf("free %v", free)
	}
	// A gap too short to hold a clip is not offered.
	short := FreeWindows([]Window{{0, 100}, {110, 3600}}, 3600, 30)
	if len(short) != 0 {
		t.Errorf("a 10 second gap was offered: %v", short)
	}
	// A plan without a window covers the whole episode, so nothing is left.
	whole := SearchedWindows([]PlanSummary{{}}, 3600)
	if fmt.Sprint(whole) != "[{0 3600}]" {
		t.Fatalf("whole %v", whole)
	}
	if left := FreeWindows(whole, 3600, 30); len(left) != 0 {
		t.Errorf("something was left of a whole episode: %v", left)
	}
	// Nothing searched leaves the whole episode.
	if all := FreeWindows(nil, 3600, 30); fmt.Sprint(all) != "[{0 3600}]" {
		t.Errorf("all %v", all)
	}
}

// Clips made by hand searched nothing. Their set has no window, and a set
// with no window is otherwise a search of the whole episode, which laid the
// window over the whole range picker and hid every mark under it. It says
// who made it, and that is what counts, not what the file is called.
func TestAClipSetMadeByHandSearchedNothing(t *testing.T) {
	plans := []PlanSummary{
		{Path: "/ep.framefairy/logs/clips-0-1800.json", From: 0, To: 1800, Clips: 12},
		{Path: "/ep.framefairy/logs/clips-by-any-name.json", By: ByHand, Clips: 1},
	}
	got := SearchedWindows(plans, 14400)
	if len(got) != 1 || got[0] != (Window{0, 1800}) {
		t.Errorf("searched %v, want only the first half hour", got)
	}
	free := FreeWindows(got, 14400, 600)
	if len(free) != 1 || free[0] != (Window{1800, 14400}) {
		t.Errorf("free %v, want the rest of the episode", free)
	}
}
