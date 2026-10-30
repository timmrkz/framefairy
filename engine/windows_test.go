package engine

import (
	"fmt"
	"testing"

	"framefairy/engine/timed"
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

// The window moves on when the app chooses it, from where the fewest
// searches have been. Moved here from the workspace with the window's
// one owner, #458.
func TestNextWindow(t *testing.T) {
	const half, hours, six = 30 * 60.0, 4 * 3600.0, 6 * 60.0
	pass := func(from, to float64, times int) Pass { return Pass{timed.Window{Start: from, End: to}, times} }
	for _, c := range []struct {
		name           string
		passes         []Pass
		duration, size float64
		least          float64
		want           timed.Window
	}{
		{"a fresh episode opens on the first half hour", []Pass{pass(0, hours, 0)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"after the first search it moves past what was searched",
			[]Pass{pass(0, half, 1), pass(half, hours, 0)}, hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a gap in the middle is taken before the end",
			[]Pass{pass(0, half, 1), pass(half, 2*half, 0), pass(2*half, 3*half, 1), pass(3*half, hours, 0)},
			hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a part a little longer than the half hour is taken whole",
			[]Pass{pass(0, 40*60, 0), pass(40*60, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: 40 * 60}},
		{"a part well over the half hour gives up only that much",
			[]Pass{pass(0, 3600, 0), pass(3600, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"an episode searched end to end starts over at the start", []Pass{pass(0, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"and the second round walks on the way the first did",
			[]Pass{pass(0, half, 2), pass(half, hours, 1)}, hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a short episode is searched whole", []Pass{pass(0, six, 0)}, six, six, 0, timed.Window{Start: 0, End: six}},
		{"again and again", []Pass{pass(0, six, 3)}, six, six, 0, timed.Window{Start: 0, End: six}},
		{"a scrap too short for a clip is passed over",
			[]Pass{pass(0, 10, 0), pass(10, half, 1), pass(half, hours, 0)}, hours, half, 20, timed.Window{Start: half, End: 2 * half}},
		{"nothing to go by is the whole episode", nil, hours, half, 0, timed.Window{Start: 0, End: hours}},
	} {
		if got := NextWindow(c.passes, c.duration, c.size, c.least); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// After a search the window walks on as long as it was left.
func TestFollowingWindow(t *testing.T) {
	const six, short = 6 * 60.0, 5*60 + 25.0
	for _, c := range []struct {
		name                    string
		ended, length, duration float64
		want                    timed.Window
	}{
		{"a minute at four goes on to a minute at five", 240, 60, six, timed.Window{Start: 240, End: 300}},
		{"the last one is cut at the end of the episode", 300, 60, short, timed.Window{Start: 300, End: short}},
		{"and the one after it is as long as it was set again, from the start", short, 60, short, timed.Window{Start: 0, End: 60}},
		{"a scrap too short for a clip is passed over", 310, 60, short, timed.Window{Start: 0, End: 60}},
		{"a short episode searched whole is searched whole again", six, six, six, timed.Window{Start: 0, End: six}},
	} {
		if got := FollowingWindow(c.ended, c.length, c.duration, 20); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A window the app places lands on a round step that follows the episode
// alone, so the first search and the workspace place the same window.
func TestAWindowTheAppPlacesLandsOnTheStep(t *testing.T) {
	const hours = 14423.0
	for _, c := range []struct {
		duration, want float64
	}{{hours, 120}, {3600, 30}, {360, 5}, {60, 1}, {30 * 3600, 600}, {100 * 3600, 1800}} {
		if got := PlaceStep(c.duration); got != c.want {
			t.Errorf("the step of %s is %v, want %v", HMS(c.duration), got, c.want)
		}
	}
	for _, c := range []struct {
		name         string
		w            timed.Window
		length, step float64
		duration     float64
		least        float64
		want         timed.Window
	}{
		{"the app's own window of 30:03 is 30:00 on the step", timed.Window{Start: 0, End: 1803}, 1803, 120, hours, 20, timed.Window{Start: 0, End: 1800}},
		{"a window after one that ended off the step starts on it", timed.Window{Start: 4202, End: 6004}, 1802, 120, hours, 20, timed.Window{Start: 4200, End: 6000}},
		{"the last one takes in the scrap after it", timed.Window{Start: 12600, End: 14400}, 1800, 120, hours, 20, timed.Window{Start: 12600, End: hours}},
		{"never shorter than a step", timed.Window{Start: 60, End: 62}, 2, 5, 360, 0, timed.Window{Start: 60, End: 65}},
	} {
		if got := OnStep(c.w, c.length, c.duration, c.step, c.least); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}
