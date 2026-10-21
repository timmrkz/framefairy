package main

import (
	"testing"
)

// The rates the screen of Tim's MacBook Pro offers: ProMotion up to 120
// and the fixed rates of System Settings, Apple's list for the 14 and 16
// inch MacBook Pro.
var macBookPro = []screenMode{{hz: 120, now: true}, {hz: 60}, {hz: 59.94}, {hz: 50}, {hz: 48}, {hz: 47.95}}

// A video of 25 or 50 frames a second asks for 50 Hz, where its frames
// stand 2 or 1 refreshes each, 40 or 20 ms, instead of 4 and 5 by turns at
// 120. 24, 30 and 60 are even at 120 already, and the screen stays as it
// is. 29.97 needs 59.94, and 23.976 needs 47.95. Issue #251.
func TestTheScreenRunsAtARateThatShowsTheVideoEvenly(t *testing.T) {
	cases := []struct {
		fps  float64
		want float64
	}{
		{25, 50},
		{50, 50},
		{24, 0},
		{30, 0},
		{60, 0},
		{30000.0 / 1001, 60000.0 / 1001},
		{24000.0 / 1001, 48000.0 / 1001},
		{0, 0},
	}
	modes := append([]screenMode(nil), macBookPro...)
	modes[2].hz = 60000.0 / 1001
	modes[5].hz = 48000.0 / 1001
	for _, c := range cases {
		if got := matchRate(modes, c.fps); got != c.want {
			t.Errorf("a video of %.3f fps asks for %.3f Hz, want %.3f", c.fps, got, c.want)
		}
	}
}

// A screen with no rate that fits leaves the video as it is, and a screen
// that runs at a fitting rate already is not changed for a higher one.
func TestTheScreenIsNotChangedWhenNothingFitsOrItFitsAlready(t *testing.T) {
	if got := matchRate([]screenMode{{hz: 60, now: true}}, 25); got != 0 {
		t.Errorf("a screen of only 60 Hz asks for %.0f Hz", got)
	}
	if got := matchRate([]screenMode{{hz: 50, now: true}, {hz: 100}}, 25); got != 0 {
		t.Errorf("a screen at 50 Hz asks for %.0f Hz", got)
	}
	if got := matchRate([]screenMode{{hz: 60, now: true}, {hz: 50}, {hz: 100}}, 25); got != 100 {
		t.Errorf("a screen offering 50 and 100 Hz asks for %.0f Hz, want the higher", got)
	}
}

// Every answer reads as what happened to the screen, and one that would
// not change is a warning.
func TestTheLogSaysWhatCameOfMatchingTheScreen(t *testing.T) {
	for done := matchNoScreen; done <= matchNothingToGive; done++ {
		level, msg := matchSaid(done)
		if msg == "" {
			t.Errorf("done %d says nothing", done)
		}
		if (done == matchFailed) != (level.String() == "warn") {
			t.Errorf("done %d is said at %s", done, level)
		}
	}
}

// Away from a window, as in the walks and on a system that matches
// nothing yet, asking changes nothing and fails nothing.
func TestMatchingWithNoWindowChangesNothing(t *testing.T) {
	(&FrameFairy{}).MatchScreen(25)
	if done, _, _, _ := screenMatch(nil, 25); done != matchNoScreen {
		t.Errorf("no window did %d", done)
	}
}
