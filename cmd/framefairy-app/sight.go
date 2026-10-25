package main

import (
	"math"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// sight follows whether macOS shows the app's window or has it covered,
// by other apps or by nothing at all, and says in the log when it goes
// out of sight and when it comes back, with how long it was away.
//
// The page says the same as hidden and shown, see lib/said.ts, but only
// while WebKit counts a covered window as a hidden page, which keepDrawing
// turns off. Tim saw part of the app flicker when Mission Control showed
// it after a while behind other apps, and a glance in Mission Control is
// a line here and a line a second or two later, #536.
type sight struct {
	mu sync.Mutex
	// Whether the window was out of sight at the last answer, and since
	// when. Nothing is known before the first answer.
	known bool
	away  bool
	since time.Time
	// What WebKit's processes had used by then.
	used cpuUse
}

// cpuUse is the processor time WebKit's processes for the app's window
// have used, in ms: the web process that runs the page and the GPU process
// that draws it. Below zero where it is not known.
type cpuUse struct {
	web, gpu float64
}

// since is what was used from the earlier reading to this one, below zero
// where either is not known. A process WebKit started again in between
// counts from nothing, so it is not known either.
func (u cpuUse) since(before cpuUse) cpuUse {
	spent := func(now, then float64) float64 {
		if now < 0 || then < 0 || now < then {
			return -1
		}
		return now - then
	}
	return cpuUse{web: spent(u.web, before.web), gpu: spent(u.gpu, before.gpu)}
}

// sightChange is what one answer from macOS changed: whether the app came
// back into sight, and for how long it was away when it did, with what
// WebKit used while it was. That is what keeping a covered app drawn
// costs, #536.
type sightChange struct {
	back bool
	away time.Duration
	used cpuUse
}

// seen takes one answer, whether the window is in sight at that moment,
// and says what changed. An answer that changes nothing says so, since
// macOS tells the window both on being covered and on being hidden. The
// first answer is only said when the app starts out of sight: an app that
// starts in sight has come back from nowhere.
func (s *sight) seen(visible bool, at time.Time, used cpuUse) (sightChange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := !s.known
	if !first && s.away == !visible {
		return sightChange{}, false
	}
	change := sightChange{back: visible, used: cpuUse{web: -1, gpu: -1}}
	if visible && !first {
		change.away = at.Sub(s.since)
		change.used = used.since(s.used)
	}
	s.known = true
	s.away = !visible
	s.since = at
	s.used = used
	return change, !(first && visible)
}

// watchSight logs the app's window going out of sight and coming back.
// macOS tells it on the window's occlusion state, which Wails hands on as
// the window shown and hidden. Wails runs each listener on a goroutine of
// its own, so a hide and a show close together can arrive in either
// order: every event asks the window where it stands now instead of
// taking the event's word for it.
func watchSight(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	s := &sight{}
	tell := func(*application.WindowEvent) {
		used := application.InvokeSyncWithResult(func() cpuUse {
			return webCPU(window.NativeWindow())
		})
		change, ok := s.seen(window.IsVisible(), time.Now(), used)
		if !ok {
			return
		}
		if !change.back {
			theLog.line(zerolog.DebugLevel, "window").Msg("the app went out of sight")
			return
		}
		line := theLog.line(zerolog.DebugLevel, "window").Int64("away_ms", change.away.Milliseconds())
		if change.used.web >= 0 {
			line = line.Int64("web_cpu_ms", int64(math.Round(change.used.web)))
		}
		if change.used.gpu >= 0 {
			line = line.Int64("gpu_cpu_ms", int64(math.Round(change.used.gpu)))
		}
		line.Msg("the app came back into sight")
	}
	window.OnWindowEvent(events.Mac.WindowHide, tell)
	window.OnWindowEvent(events.Mac.WindowShow, tell)
}

// What came of asking the web view to keep its page drawn while the app's
// window is covered.
const (
	drawingNoWebView = iota
	drawingNoSetting
	drawingKept
	drawingNotDropped
)

// keepDrawing asks WebKit to keep the page of the app's window drawn
// while the window is covered. WebKit on the Mac counts a covered window
// as a hidden page and stops drawing it, and Mission Control uncovering it
// may then show the window before WebKit has drawn it again. That is the
// likeliest cause of the flicker Tim saw, not proven, and this build is
// the test of it, #536. What it costs while a covered app is busy is still
// to be measured before it stays.
//
// It is done once the page is up, and again whenever it is loaded anew.
func keepDrawing(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		done := application.InvokeSyncWithResult(func() int {
			return drawingKeep(window.NativeWindow())
		})
		level, msg := drawingSaid(done)
		theLog.line(level, "window").Msg(msg)
	})
}

// drawingSaid is how the answer reads in the log.
func drawingSaid(done int) (zerolog.Level, string) {
	switch done {
	case drawingKept:
		return zerolog.InfoLevel, "the page stays drawn while the app is covered"
	case drawingNotDropped:
		return zerolog.DebugLevel, "the page is left as this system's web view draws a covered window"
	case drawingNoSetting:
		return zerolog.WarnLevel, "the page is hidden while the app is covered: this WebKit has no setting to keep it"
	default:
		return zerolog.WarnLevel, "the page is hidden while the app is covered: the app's window has no web view to set"
	}
}
