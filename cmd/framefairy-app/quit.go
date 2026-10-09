package main

import (
	"sync"
	"time"
)

// quitAgain is how long the second Cmd+Q has to come after the first. The
// interface shows the question for as long.
const quitAgain = 3 * time.Second

// leaving decides what Cmd+Q, Quit in the menu and closing the app's window
// do, and it is where Wails asks, ShouldQuit.
//
// Stopping everything the app started takes a moment: a search has to let
// go of the model and llama-server has to go. On macOS Wails runs its
// shutdown hooks on the main thread, so doing it there froze the app with
// the spinning wheel for as long as it took, and nothing said the key had
// been heard. So the first answer is always no. The app says at once what
// is happening, stops everything on a goroutine of its own, and then quits
// for real, which Wails asks about again and this time hears yes.
//
// The first Cmd+Q only asks, the way Chrome does, and the second within
// quitAgain quits. Always, on every page, whether work runs or not. It
// asked only while work ran, which in the workspace is nearly always and
// on the settings page nearly never, so one page took two presses and the
// other quit on the first, and a key that does not do the same thing
// twice is a key nobody can trust.
type leaving struct {
	mu      sync.Mutex
	askedAt time.Time
	going   bool
	gone    bool
	// windowGone is set once the app's window is closing. Nobody can press
	// Cmd+Q a second time into a window that is no longer there.
	windowGone bool

	// asked says whether the quitting was asked for in the app already, by
	// Relaunch on the Updates page, which quits into the new build and has
	// nothing to ask about.
	asked func() bool
	// say tells the interface: "ask" for the question, "going" once the app
	// is on its way out.
	say func(string)
	// stop stops everything the app started, and quit quits for real.
	stop func()
	quit func()
}

func (l *leaving) shouldQuit() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.gone:
		return true
	case l.going:
		return false
	case !l.windowGone && !l.asked() && time.Since(l.askedAt) > quitAgain:
		l.askedAt = time.Now()
		l.say("ask")
		return false
	}
	l.going = true
	l.say("going")
	go func() {
		l.stop()
		l.mu.Lock()
		l.gone = true
		l.mu.Unlock()
		l.quit()
	}()
	return false
}

// stay is the question taken away, with Escape or a click. The next Cmd+Q
// asks again. It did not: the question went from the screen and the press
// still counted, so Cmd+Q, Escape, a click on Settings and Cmd+Q there
// quit on what looked like the first press.
func (l *leaving) stay() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.going {
		l.askedAt = time.Time{}
	}
}

// closing is the app's window on its way. The app quits after it without
// asking.
func (l *leaving) closing() {
	l.mu.Lock()
	l.windowGone = true
	l.mu.Unlock()
}
