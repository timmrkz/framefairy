package main

import (
	"testing"

	"framefairy/engine"
)

// A new video's window is the first of the equal windows it is cut into,
// on a round step, the whole of a short one, and no longer than one
// search can read. The first search and the workspace both take it from
// here, so the window searched is the window drawn, #458.
func TestTheFirstWindowFollowsTheVideo(t *testing.T) {
	for _, c := range []struct {
		name           string
		duration, most float64
		want           KeptWindow
	}{
		{"half an hour", 1800, 0, KeptWindow{0, 600, 600}},
		{"an hour", 3600, 0, KeptWindow{0, 900, 900}},
		{"four hours", 4 * 3600, 0, KeptWindow{0, 1800, 1800}},
		{"a little over four hours, 30:02 a window, on the step", 4*3600 + 15, 0, KeptWindow{0, 1800, 1800}},
		{"ten minutes is searched whole", 600, 0, KeptWindow{0, 600, 600}},
		{"a quarter of an hour too", 900, 0, KeptWindow{0, 900, 900}},
		{"no longer than a search reads", 3600, 700, KeptWindow{0, 690, 690}},
	} {
		passes := engine.SearchPasses(nil, c.duration)
		if got := placeWindow(passes, c.duration, 20, c.most); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// Where speech is uneven, the window the app places fits the reach the
// range picker fits every window to, and stays on the step, so the
// workspace has nothing to cut and write back. The room at the video's
// average rate let it be longer, the fourth review of #458.
func TestAPlacedWindowFitsTheRangePickersReach(t *testing.T) {
	const duration = 3600.0
	// Slow speech for the first half hour, fast speech after it.
	var lines []engine.LineWeight
	for at := 0.0; at+3 <= duration; at += 4 {
		chars := 40
		if at >= 1800 {
			chars = 160
		}
		lines = append(lines, engine.LineWeight{Start: at, End: at + 3, Chars: chars})
	}
	const chars = 30000
	average := chars / engine.RateOf(lines)
	reach := engine.NewReach(chars, lines, [][2]float64{{0, duration}}, engine.RateOf(lines), duration).Anywhere()
	if reach >= average {
		t.Fatalf("the reach %v is not shorter than the room at the average rate %v, so this proves nothing", reach, average)
	}
	got := placeWindow(engine.SearchPasses(nil, duration), duration, 20, reach)
	step := engine.PlaceStep(duration)
	if got.To-got.From > reach || got.Length > reach {
		t.Errorf("the window %+v is longer than the reach %v", got, reach)
	}
	if got.From != 0 || got.Length != float64(int(got.Length/step))*step {
		t.Errorf("the window %+v is not on the step of %v", got, step)
	}
}
