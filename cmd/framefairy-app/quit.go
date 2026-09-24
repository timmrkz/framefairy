package main

import (
	"sync"
	"time"
)

// quitAgain is how long the second Cmd+Q has to come after the first while
// work runs. The interface shows the question for as long.
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
// While work runs, the first Cmd+Q only asks, the way Chrome does, because
// quitting stops a search or a render half way. The second within
// quitAgain quits. With nothing running, one is enough.
type leaving struct {
	mu      sync.Mutex
	askedAt time.Time
	going   bool
	gone    bool
	// windowGone is set once the app's window is closing. Nobody can press
	// Cmd+Q a second time into a window that is no longer there.
	windowGone bool

	// busy says whether any work runs or waits.
	busy func() bool
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
	case !l.windowGone && l.busy() && time.Since(l.askedAt) > quitAgain:
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

// closing is the app's window on its way. The app quits after it without
// asking.
func (l *leaving) closing() {
	l.mu.Lock()
	l.windowGone = true
	l.mu.Unlock()
}
