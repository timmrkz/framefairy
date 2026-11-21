package main

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// The log says whether the page updates at the screen's own rate, and a
// page left at about 60 is a warning, since the video preview then cannot
// play a video of 25 frames a second evenly on a screen of 120 Hz, issue
// #251.
func TestTheLogSaysWhetherThePageUpdatesAtTheScreensRate(t *testing.T) {
	cases := []struct {
		done  int
		level zerolog.Level
		says  string
	}{
		{refreshFreed, zerolog.InfoLevel, "at the screen's own rate"},
		{refreshNotHeld, zerolog.DebugLevel, "as this system's web view does by itself"},
		{refreshNoSetting, zerolog.WarnLevel, "no setting to lift it"},
		{refreshNoWebView, zerolog.WarnLevel, "no web view to set"},
	}
	for _, c := range cases {
		level, msg := refreshSaid(refreshRaw{done: c.done, most: 120})
		if level != c.level || !strings.Contains(msg, c.says) {
			t.Errorf("done %d said %s %q, want %s with %q", c.done, level, msg, c.level, c.says)
		}
	}
}
