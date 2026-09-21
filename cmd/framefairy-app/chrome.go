package main

import (
	"math"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Chrome is where macOS put the window's own furniture, in whole CSS
// pixels, measured rather than guessed.
//
// The app draws its own bar across the top of the window and macOS draws
// the close, minimise and zoom buttons on top of it. Nothing public moves
// those buttons: the height the window is asked to leave empty only says
// how far down a drag still moves the window, and the one thing that does
// decide where they sit, the title bar AppKit lays out, is AppKit's to
// lay out. So the bar stops trying to be a height of its own and becomes
// exactly the title bar, which centres the buttons in it by construction,
// on every version of macOS and at every control size.
//
// All zeros mean the system draws its own title bar, which is every other
// system, and then the stylesheet keeps its own numbers.
type Chrome struct {
	// The title bar's height, which is the height of the bar the app draws.
	Bar int `json:"bar"`
	// The left edge of the first window button and the right edge of the
	// last, so the name of what is on screen starts after them.
	Left  int `json:"left"`
	Right int `json:"right"`
	// The middle of the window buttons, from the top of the page, so the
	// name sits on their line rather than on the bar's.
	Middle int `json:"middle"`
}

// chromeRaw is what the system answers, in points and unrounded.
type chromeRaw struct {
	bar, left, right, middle float64
	fullscreen               bool
}

// chromeWatch keeps the window's furniture up to date. It measures when
// macOS says something happened that could have moved it, and tells the
// window only when the answer changed.
type chromeWatch struct {
	app    *application.App
	window *application.WebviewWindow

	mu sync.Mutex
	// The bar's height while there is a title bar to measure. In native
	// fullscreen there is none, and the bar keeps this height so the
	// workspace under it does not jump on the way in and out.
	outside int
	// Whether the window is on its way into fullscreen, into it, or on its
	// way out. All three are fullscreen as far as the bar is concerned.
	full bool
	// What the window was last told, so nothing is said twice.
	said Chrome
	told bool
}

// measureChrome is what asks the system. A test puts its own in here, so
// nothing in this file needs a window or a main thread to be tested.
var measureChrome = func(window *application.WebviewWindow) chromeRaw {
	if window == nil {
		return chromeRaw{}
	}
	// AppKit answers about its own layout only on the main thread, and a
	// window event callback runs on a goroutine of its own.
	return application.InvokeSyncWithResult(func() chromeRaw {
		return chromeMeasure(window.NativeWindow())
	})
}

// measure asks where the furniture is and puts the answer on whole pixels.
func (c *chromeWatch) measure() Chrome {
	raw := measureChrome(c.window)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.settle(raw)
}

// settle turns one answer into what the window is told. It is the whole of
// the thinking, and it touches nothing outside the struct, so a test can
// drive it with answers of its own.
func (c *chromeWatch) settle(raw chromeRaw) Chrome {
	if raw.fullscreen {
		c.full = true
	}
	if c.full {
		// No title bar and no buttons while the window is fullscreen. The
		// bar stays the height it had and says there is nothing to line up
		// with, so the name simply moves to the left edge.
		return Chrome{Bar: c.outside}
	}
	out := Chrome{Bar: px(raw.bar), Left: px(raw.left), Right: px(raw.right), Middle: px(raw.middle)}
	if out.Bar > 0 {
		c.outside = out.Bar
	}
	return out
}

// px puts a measurement on a whole pixel. It is the only place anything is
// rounded: what crosses to the window is never a fraction, because an
// element on a fraction is painted in one place normally and another the
// moment anything puts it on a surface of its own.
func px(v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return int(math.Round(v))
}

// publish measures and tells the window, but only when the answer is not
// what it was told last time. A window being dragged by its corner raises
// one of these a frame, and almost none of them change anything.
func (c *chromeWatch) publish() {
	now := c.measure()
	c.mu.Lock()
	same := c.told && c.said == now
	c.said = now
	c.told = true
	c.mu.Unlock()
	if same || c.app == nil {
		return
	}
	c.app.Event.Emit("chrome", now)
}

// watchChrome follows the window's furniture for as long as the window is
// open.
//
// Every event here is one after which AppKit may have laid the title bar
// out again. Nothing is moved by any of this, so there is nothing that can
// snap back: the app reads where the buttons went and lays itself out
// around them.
func watchChrome(app *application.App, window *application.WebviewWindow) *chromeWatch {
	c := &chromeWatch{app: app, window: window}
	if window == nil {
		return c
	}
	at := func(kind events.WindowEventType, before func()) {
		window.OnWindowEvent(kind, func(*application.WindowEvent) {
			if before != nil {
				before()
			}
			c.publish()
		})
	}
	// The toolbar is put on after the window is made, and it is what makes
	// the title bar taller, so the first measurement worth having is this
	// one. The page asks for itself as well, once it is up.
	at(events.Mac.WindowDidChangeToolbar, nil)
	at(events.Common.WindowRuntimeReady, nil)
	// A resize, a zoom, and the end of either fullscreen animation.
	at(events.Mac.WindowDidResize, nil)
	// Fullscreen, marked before the animation starts so no frame of it
	// leaks a bar that is shrinking away.
	at(events.Mac.WindowWillEnterFullScreen, func() { c.set(true) })
	at(events.Mac.WindowDidEnterFullScreen, nil)
	at(events.Mac.WindowWillExitFullScreen, nil)
	at(events.Mac.WindowDidExitFullScreen, func() { c.set(false) })
	// Another display, another scale, another arrangement of them.
	at(events.Mac.WindowDidChangeScreen, nil)
	at(events.Mac.WindowDidChangeBackingProperties, nil)
	at(events.Mac.WindowDidChangeScreenParameters, nil)
	// Light and dark, after which AppKit lays the title bar out again.
	at(events.Mac.WindowDidChangeEffectiveAppearance, nil)
	// Back from the Dock.
	at(events.Mac.WindowDidDeminiaturize, nil)
	return c
}

// set says whether the window is fullscreen, which the events know before
// a measurement can.
func (c *chromeWatch) set(full bool) {
	c.mu.Lock()
	c.full = full
	c.mu.Unlock()
}
