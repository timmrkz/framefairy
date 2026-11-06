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

// closeDown is the one way the app stops everything it started, on the
// way out. main runs it as the shutdown hook and as what Cmd+Q stops, and
// the walks' bridge runs it on the app it closes before it removes that
// app's folders. Once it returns, nothing the app started writes into a
// video's work folder or the config folder: the watch and the news, the
// models, the jobs with any ffmpeg they run, a recording, the caption
// files of the video preview and a draft of Report a Problem. A part that
// writes is stopped here, or the bridge's reset fails on it with
// "directory not empty", the way it did each time on a writer the fixes
// before had not named, issue #713. It says whether the jobs stopped in
// time.
func (s *FrameFairy) closeDown() bool {
	s.watch.stop()
	s.news.stop()
	s.store.machine.CloseModels()
	s.CancelListening()
	stopped := s.stopAll()
	// A caption file goes on being made after the request that asked for
	// it is gone, and a draft the report window saved as it closed may be
	// waiting on its lock. In the app the process ending cuts both off,
	// half written.
	s.store.machine.ClosePreviews()
	s.report.end()
	return stopped
}

// stopAll stops everything the app started, on the way out, after the
// models: the jobs, and with them any ffmpeg they run, the measuring of
// loudness, and a build that is ready goes in place.
//
// A video let go because its folder was deleted outside the app has what
// was written back into the folder deleted once its jobs have ended, on a
// goroutine of its own, see clearLetGo. The app waits for that delete
// before it goes. Quitting in the middle of it left part of a work folder
// that no video in the library points to, and the video added again found
// it and was not marked new, issue #507. The jobs are what a delete waits
// for, so it is waited for only when they stopped in time. When they did
// not, each delete gives up after stopWait of its own, and quitting is
// not held that much longer.
//
// The models are closed by closeDown, just before this, with the watch
// and the news, so what the window is told and what runs stop together.
// It says whether the jobs stopped in time.
func (s *FrameFairy) stopAll() bool {
	// A WaitGroup takes no new work while it is waited for, so a video let
	// go from here on is cleared where it is let go.
	s.clearMu.Lock()
	s.quitting = true
	s.clearMu.Unlock()
	stopped := s.jobs.shutDown()
	if stopped {
		s.clearing.Wait()
	}
	s.levels.shutDown()
	s.updates.installOnQuit()
	return stopped
}
