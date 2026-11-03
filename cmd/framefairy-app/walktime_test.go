package main

import (
	"slices"
	"sync"
	"time"
)

// walkTime is the clock the bridge hands the app's removals, see
// timeKeeper in removal.go. It runs as the system's does, and a walk
// runs it on by as much as it runs the page's clock on, through
// /clock?run=, see runClock in walks/bridge.mjs. So ten seconds a walk
// runs on in the page are ten seconds here too, in step, and nothing in
// the walk waits for the real ones.
type walkTime struct {
	mu sync.Mutex
	// ahead is how far it was run on.
	ahead time.Duration
	// due is every call set and not yet made or stopped.
	due map[*walkCall]struct{}
}

// walkCall is one call due at a moment of the walk's clock. timer makes
// it at that moment of the system's, unless the clock is run past it
// first.
type walkCall struct {
	at    time.Time
	f     func()
	timer *time.Timer
}

func newWalkTime() *walkTime {
	return &walkTime{due: map[*walkCall]struct{}{}}
}

func (w *walkTime) Now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nowLocked()
}

func (w *walkTime) nowLocked() time.Time {
	return time.Now().Add(w.ahead)
}

func (w *walkTime) AfterFunc(d time.Duration, f func()) func() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := &walkCall{at: w.nowLocked().Add(d), f: f}
	w.due[c] = struct{}{}
	c.timer = time.AfterFunc(d, func() { w.make(c) }) //nolint:forbidigo // the walk's clock runs as the system's between the times a walk runs it on
	return func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		_, was := w.due[c]
		delete(w.due, c)
		c.timer.Stop()
		return was
	}
}

// make makes a call once, whichever comes first, its moment on the
// system's clock or the walk running the clock past it.
func (w *walkTime) make(c *walkCall) {
	w.mu.Lock()
	_, was := w.due[c]
	delete(w.due, c)
	w.mu.Unlock()
	if was {
		c.f()
	}
}

// run runs the clock on by d, and makes every call due by then, in the
// order they are due, before it answers, so what they set going has
// begun by the time the walk looks.
func (w *walkTime) run(d time.Duration) {
	w.mu.Lock()
	w.ahead += d
	now := w.nowLocked()
	var calls []*walkCall
	for c := range w.due {
		if !c.at.After(now) {
			calls = append(calls, c)
		}
	}
	w.mu.Unlock()
	slices.SortFunc(calls, func(a, b *walkCall) int { return a.at.Compare(b.at) })
	for _, c := range calls {
		c.timer.Stop()
		w.make(c)
	}
}
