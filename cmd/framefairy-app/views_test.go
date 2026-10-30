package main

import (
	"math"
	"testing"
)

// The number keys ask the engine where a share of the chosen clip lands,
// and the answer is a moment of that clip, read from its own plan. Clip
// 02 of historyPlan runs from 20 to 22 s. Issue #539.
func TestAShareOfAClipIsAMomentOfThatClip(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	for _, c := range []struct{ share, at float64 }{{0, 20}, {0.5, 21}, {0.9, 21.8}} {
		got, err := svc.ShareOfClip(mine, plan, "02", c.share)
		if err != nil || math.Abs(got-c.at) > 1e-6 {
			t.Errorf("%.1f of clip 02 is %v, %v, want %.1f", c.share, got, err, c.at)
		}
	}
	if _, err := svc.ShareOfClip(mine, plan, "09", 0.5); err == nil {
		t.Error("a share of a clip the plan does not have was answered")
	}
}
