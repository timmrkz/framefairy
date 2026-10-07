package engine

import (
	"fmt"
	"testing"
)

// Clips made by hand searched nothing. Their set has no window, and a set
// with no window is otherwise a search of the whole episode, which laid the
// window over the whole range picker and hid every mark under it. It says
// who made it, and that is what counts, not what the file is called.
func TestAClipSetMadeByHandSearchedNothing(t *testing.T) {
	plans := []PlanSummary{
		{Path: "/ep.framefairy/logs/clips-0-1800.json", From: 0, To: 1800, Clips: 12},
		{Path: "/ep.framefairy/logs/clips-by-any-name.json", By: ByHand, Clips: 1},
	}
	got := fmt.Sprint(SearchPasses(plans, 14400))
	if want := "[{{0 1800} 1} {{1800 14400} 0}]"; got != want {
		t.Errorf("passes %s, want only the first half hour searched, %s", got, want)
	}
}

// The episode in parts by how many searches read them, the parts nobody
// searched too, and a search of the whole episode counted over all of it.
func TestSearchPasses(t *testing.T) {
	plans := []PlanSummary{
		{From: 0, To: 600, Clips: 3},
		{From: 600, To: 1200, Clips: 3},
		{From: 0, To: 600, Clips: 2},
		{Name: HandPlanName, By: ByHand, Clips: 1},
		{From: 1200, To: 1800},
	}
	got := fmt.Sprint(SearchPasses(plans, 2400))
	want := "[{{0 600} 2} {{600 1800} 1} {{1800 2400} 0}]"
	if got != want {
		t.Errorf("passes:\n got %s\nwant %s", got, want)
	}
	if got := fmt.Sprint(SearchPasses(nil, 360)); got != "[{{0 360} 0}]" {
		t.Errorf("an episode nobody searched: %s", got)
	}
	whole := []PlanSummary{{}, {From: 0, To: 360}}
	if got := fmt.Sprint(SearchPasses(whole, 360)); got != "[{{0 360} 2}]" {
		t.Errorf("an episode searched whole twice: %s", got)
	}
}
