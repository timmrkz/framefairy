package main

import (
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
}

// sightChange is what one answer from macOS changed: whether the app came
// back into sight, and for how long it was away when it did.
type sightChange struct {
	back bool
	away time.Duration
}

// seen takes one answer, whether the window is in sight at that moment,
// and says what changed. An answer that changes nothing says so, since
// macOS tells the window both on being covered and on being hidden.
func (s *sight) seen(visible bool, at time.Time) (sightChange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known && s.away == !visible {
		return sightChange{}, false
	}
	change := sightChange{back: visible}
	if visible && s.known {
		change.away = at.Sub(s.since)
	}
	s.known = true
	s.away = !visible
	s.since = at
	return change, true
}

// watchSight logs the app's window going out of sight and coming back.
// macOS tells it on the window's occlusion state, which Wails hands on as
// the window shown and hidden.
func watchSight(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	s := &sight{}
	at := func(kind events.WindowEventType, visible bool) {
		window.OnWindowEvent(kind, func(*application.WindowEvent) {
			change, ok := s.seen(visible, time.Now())
			if !ok {
				return
			}
			if !change.back {
				theLog.line(zerolog.DebugLevel, "window").Msg("the app went out of sight")
				return
			}
			theLog.line(zerolog.DebugLevel, "window").Int64("away_ms", change.away.Milliseconds()).Msg("the app came back into sight")
		})
	}
	at(events.Mac.WindowHide, false)
	at(events.Mac.WindowShow, true)
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
// as a hidden page: it stops drawing it and, after a while, lets go of
// what it drew. Mission Control then uncovers the window and WebKit draws
// it again while it is shown, and until it has, the window's own colour
// shows where the page was. That is the most likely cause of the flicker
// Tim saw, #536, and this build is the test of it. A page that is still
// asks for no frames, so an app kept drawn behind other apps costs
// nothing while nothing in it moves.
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
		return zerolog.DebugLevel, "the page stays drawn while the app is covered, as this system's web view does by itself"
	case drawingNoSetting:
		return zerolog.WarnLevel, "the page is let go of while the app is covered: this WebKit has no setting to keep it"
	default:
		return zerolog.WarnLevel, "the page is let go of while the app is covered: the app's window has no web view to set"
	}
}
