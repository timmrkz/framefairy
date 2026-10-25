package main

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// A glance in Mission Control at an app covered for a minute is two lines
// in the log, out of sight and back after a minute, and macOS telling the
// window the same twice is one, #536.
func TestTheLogSaysHowLongTheAppWasOutOfSight(t *testing.T) {
	start := time.Date(2026, 10, 25, 13, 47, 46, 0, time.UTC)
	steps := []struct {
		visible bool
		after   time.Duration
		said    bool
		back    bool
		away    time.Duration
	}{
		{true, 0, true, true, 0},
		{false, 2 * time.Second, true, false, 0},
		{false, 3 * time.Second, false, false, 0},
		{true, 72 * time.Second, true, true, 70 * time.Second},
		{true, 73 * time.Second, false, false, 0},
		{false, 74 * time.Second, true, false, 0},
		{true, 75*time.Second + 354*time.Millisecond, true, true, 1354 * time.Millisecond},
	}
	s := &sight{}
	for i, st := range steps {
		change, said := s.seen(st.visible, start.Add(st.after))
		if said != st.said {
			t.Fatalf("step %d, in sight %v: said %v, want %v", i, st.visible, said, st.said)
		}
		if said && (change.back != st.back || change.away != st.away) {
			t.Errorf("step %d, in sight %v: said back %v after %s, want back %v after %s", i, st.visible, change.back, change.away, st.back, st.away)
		}
	}
}

// The page kept drawn while the app is covered is said once in the log,
// and a WebKit without the setting is a warning, since the test build of
// #536 then tests nothing.
func TestTheLogSaysWhetherThePageStaysDrawnWhileCovered(t *testing.T) {
	cases := []struct {
		done  int
		level zerolog.Level
		says  string
	}{
		{drawingKept, zerolog.InfoLevel, "stays drawn while the app is covered"},
		{drawingNotDropped, zerolog.DebugLevel, "as this system's web view does by itself"},
		{drawingNoSetting, zerolog.WarnLevel, "no setting to keep it"},
		{drawingNoWebView, zerolog.WarnLevel, "no web view to set"},
	}
	for _, c := range cases {
		level, msg := drawingSaid(c.done)
		if level != c.level || !strings.Contains(msg, c.says) {
			t.Errorf("done %d said %s %q, want %s with %q", c.done, level, msg, c.level, c.says)
		}
	}
}

// Away from the Mac nothing lets go of a covered page, and a window that
// is not there yet is no fault.
func TestOnlyTheMacLetsGoOfACoveredPage(t *testing.T) {
	got := drawingKeep(nil)
	want := drawingNotDropped
	if runtime.GOOS == "darwin" {
		want = drawingNoWebView
	}
	if got != want {
		t.Errorf("drawingKeep(nil) did %d, want %d", got, want)
	}
}
