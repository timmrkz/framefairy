package main

import (
	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// What came of asking the web view to update at the screen's own rate.
const (
	refreshNoWebView = iota
	refreshNoSetting
	refreshFreed
	refreshNotHeld
)

// refreshRaw is what the system answered: what came of it, and the most
// frames a second the screen can show, 0 where it does not say.
type refreshRaw struct {
	done int
	most int
}

// freeRefresh lets the video preview change its picture as often as the
// screen refreshes. WebKit on the Mac holds a page to about 60 updates a
// second, to save power on websites, and a Mac's own screen refreshes 120
// times a second. A video of 25 frames a second on 60 updates stands 2 and
// 3 updates by turns, 33 and 50 ms, which Tim saw as playback not quite
// even, where QuickTime on the same screen stands each frame 40 ms give or
// take 4, issue #251. The page asks for an update only while something
// moves, so a still app costs nothing more.
//
// It is done once the page is up, and again whenever it is loaded anew.
func freeRefresh(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		raw := application.InvokeSyncWithResult(func() refreshRaw {
			return refreshFree(window.NativeWindow())
		})
		level, msg := refreshSaid(raw)
		theLog.line(level, "window").Int("screen_most_fps", raw.most).Msg(msg)
	})
}

// refreshSaid is how the answer reads in the log.
func refreshSaid(raw refreshRaw) (zerolog.Level, string) {
	switch raw.done {
	case refreshFreed:
		return zerolog.InfoLevel, "the page updates at the screen's own rate"
	case refreshNotHeld:
		return zerolog.DebugLevel, "the page updates at the screen's own rate, as this system's web view does by itself"
	case refreshNoSetting:
		return zerolog.WarnLevel, "the page stays held to about 60 updates a second: this WebKit has no setting to lift it"
	default:
		return zerolog.WarnLevel, "the page stays held to about 60 updates a second: the app's window has no web view to set"
	}
}
