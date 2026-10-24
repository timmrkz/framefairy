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
