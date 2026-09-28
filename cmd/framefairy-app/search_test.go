package main

import (
	"testing"

	"framefairy/engine"
)

// A new episode's first window is the first of the equal windows it is
// cut into, the whole of a short one, and never shorter than a target
// typed for it needs.
func TestTheFirstWindowFollowsTheEpisode(t *testing.T) {
	for _, c := range []struct {
		duration float64
		count    int
		want     float64
	}{
		{1800, 0, 600},
		{3600, 0, 900},
		{4 * 3600, 0, 1800},
		{600, 0, 0},
		{900, 0, 0},
		// Twelve clips of 60 s need twelve minutes.
		{1800, 12, 720},
	} {
		req := engine.PlanRequest{Count: c.count, Min: 60, Max: 90}
		if got := firstWindowEnd(c.duration, engine.Room{}, req); got != c.want {
			t.Errorf("%s with %d clips: ends at %v, want %v", engine.HMS(c.duration), c.count, got, c.want)
		}
	}
}
