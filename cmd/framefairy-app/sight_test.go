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
// window the same twice is one. An app that starts in sight says nothing,
// since it came back from nowhere. Coming back says what WebKit used while
// the app was away, and nothing it cannot know: a process it does not
// name, or one started again in between, #536.
func TestTheLogSaysHowLongTheAppWasOutOfSight(t *testing.T) {
	start := time.Date(2026, 10, 25, 13, 47, 46, 0, time.UTC)
	unknown := cpuUse{web: -1, gpu: -1}
	steps := []struct {
		visible bool
		after   time.Duration
		used    cpuUse
		said    bool
		back    bool
		away    time.Duration
		spent   cpuUse
	}{
		{true, 0, cpuUse{100, 50}, false, false, 0, unknown},
		{false, 2 * time.Second, cpuUse{1200, 300}, true, false, 0, unknown},
		{false, 3 * time.Second, cpuUse{1210, 301}, false, false, 0, unknown},
		{true, 72 * time.Second, cpuUse{1500, 340}, true, true, 70 * time.Second, cpuUse{300, 40}},
		{true, 73 * time.Second, cpuUse{1510, 341}, false, false, 0, unknown},
		{false, 74 * time.Second, cpuUse{1520, -1}, true, false, 0, unknown},
		{true, 75*time.Second + 354*time.Millisecond, cpuUse{1540, 400}, true, true, 1354 * time.Millisecond, cpuUse{20, -1}},
		{false, 80 * time.Second, cpuUse{1600, 410}, true, false, 0, unknown},
		{true, 90 * time.Second, cpuUse{40, 420}, true, true, 10 * time.Second, cpuUse{-1, 10}},
	}
	s := &sight{}
	for i, st := range steps {
		change, said := s.seen(st.visible, start.Add(st.after), st.used)
		if said != st.said {
			t.Fatalf("step %d, in sight %v: said %v, want %v", i, st.visible, said, st.said)
		}
		if said && (change.back != st.back || change.away != st.away || change.used != st.spent) {
			t.Errorf("step %d, in sight %v: said back %v after %s using %v, want back %v after %s using %v", i, st.visible, change.back, change.away, change.used, st.back, st.away, st.spent)
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
		{drawingNotDropped, zerolog.DebugLevel, "as this system's web view draws a covered window"},
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

// Away from the Mac the page is left as the system draws it, and a window
// that is not there yet is no fault.
func TestOnlyTheMacIsToldToKeepACoveredPageDrawn(t *testing.T) {
	got := drawingKeep(nil)
	want := drawingNotDropped
	if runtime.GOOS == "darwin" {
		want = drawingNoWebView
	}
	if got != want {
		t.Errorf("drawingKeep(nil) did %d, want %d", got, want)
	}
}
