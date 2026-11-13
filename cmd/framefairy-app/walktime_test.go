package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// walkTime is the clock the bridge hands the app's machine, see
// engine.Clock: how long removed work can be put back, and how often a
// search says how far it has come. It is held and let go with the page's clock,
// through /clock?hold and /clock?go, see holdClock and letClockGo in
// walks/bridge.mjs, and a walk runs it on by as much as it runs the
// page's clock on, through /clock?run=, see runClock. So ten seconds a
// walk runs on in the page are ten seconds here too, in step, and while
// the page's time is held a busy machine spends none of Go's. Let go, it
// runs as the system's does, as the page's does.
type walkTime struct {
	mu sync.Mutex
	// ahead is how far it was run on.
	ahead time.Duration
	// held is set while it stands still, at the moment it stands at.
	held   bool
	heldAt time.Time
	// due is every call set and not yet made or stopped, and set is
	// closed and made again each time a call is set, see next.
	due map[*walkCall]struct{}
	set chan struct{}
}

// walkCall is one call due at a moment of the walk's clock. timer makes
// it at that moment of the system's while the clock is let go, unless the
// clock is run past it first. It is nil while the clock is held.
type walkCall struct {
	at    time.Time
	f     func()
	timer *time.Timer
}

// stop keeps the system's clock from making the call, under the lock.
func (c *walkCall) stop() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func newWalkTime() *walkTime {
	return &walkTime{due: map[*walkCall]struct{}{}, set: make(chan struct{})}
}

func (w *walkTime) Now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.nowLocked()
}

func (w *walkTime) nowLocked() time.Time {
	if w.held {
		return w.heldAt
	}
	return time.Now().Add(w.ahead)
}

// hold stops the clock where it is. A call due meanwhile waits for the
// clock to be run on or let go.
func (w *walkTime) hold() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.held {
		return
	}
	w.heldAt, w.held = w.nowLocked(), true
	for c := range w.due {
		c.stop()
	}
}

// letGo runs the clock on from where it was held, as the system's runs.
func (w *walkTime) letGo() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.held {
		return
	}
	w.ahead, w.held = w.heldAt.Sub(time.Now()), false
	now := w.nowLocked()
	for c := range w.due {
		w.arm(c, c.at.Sub(now))
	}
}

// arm has the system's clock make c once d has passed on it, and only
// while the clock is let go.
func (w *walkTime) arm(c *walkCall, d time.Duration) {
	c.timer = time.AfterFunc(d, func() { w.make(c) }) //nolint:forbidigo // the walk's clock runs as the system's while it is let go
}

func (w *walkTime) AfterFunc(d time.Duration, f func()) func() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := &walkCall{at: w.nowLocked().Add(d), f: f}
	w.due[c] = struct{}{}
	close(w.set)
	w.set = make(chan struct{})
	if !w.held {
		w.arm(c, d)
	}
	return func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		_, was := w.due[c]
		delete(w.due, c)
		c.stop()
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

// next runs the clock on to the call due first and makes it, and the
// others due at that moment, and says how far it ran. That is any call on
// the clock, a removed row forgotten too, the way the time would have
// passed. With nothing due it waits until a call is set, the way a walk
// waits on news: a search the walk started may not have begun to keep its
// time yet. A walk that holds the clock hears the next report of a
// running search this way, without knowing how often a search reports.
// A search that ended before it reported again sets no call, so the walk
// waits until make walks' -timeout ends it and names the walk, or until
// ctx, the walk's request, is called off.
func (w *walkTime) next(ctx context.Context) (time.Duration, error) {
	for {
		w.mu.Lock()
		now := w.nowLocked()
		var first *walkCall
		for c := range w.due {
			if first == nil || c.at.Before(first.at) {
				first = c
			}
		}
		set := w.set
		w.mu.Unlock()
		if first == nil {
			select {
			case <-set:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
			continue
		}
		d := max(0, first.at.Sub(now))
		w.run(d)
		return d, nil
	}
}

// run runs the clock on by d, and makes every call due by then, in the
// order they are due, before it answers, so what they set going has
// begun by the time the walk looks.
func (w *walkTime) run(d time.Duration) {
	w.mu.Lock()
	if w.held {
		w.heldAt = w.heldAt.Add(d)
	} else {
		w.ahead += d
	}
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
		w.mu.Lock()
		c.stop()
		w.mu.Unlock()
		w.make(c)
	}
}

// The walks' clock stands still while it is held, however long the
// machine takes, makes what is due when it is run on, and goes on from
// where it stood when it is let go.
func TestTheWalksClockStandsStillWhileHeld(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWalkTime()
		var made []string
		var mu sync.Mutex
		seen := func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(made) }
		note := func(s string) func() {
			return func() { mu.Lock(); made = append(made, s); mu.Unlock() }
		}
		w.hold()
		at := w.Now()
		w.AfterFunc(10*time.Second, note("run on"))
		w.AfterFunc(20*time.Second, note("let go"))
		time.Sleep(time.Minute) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if !w.Now().Equal(at) || len(seen()) != 0 {
			t.Fatalf("held for a minute, the clock moved %v and made %v", w.Now().Sub(at), seen())
		}
		w.run(10 * time.Second)
		if got := seen(); len(got) != 1 || got[0] != "run on" {
			t.Fatalf("run on ten seconds, the clock made %v", got)
		}
		w.letGo()
		time.Sleep(10*time.Second - time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(seen()) != 1 {
			t.Fatalf("let go, the clock made %v before its time", seen())
		}
		time.Sleep(time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(seen()) != 2 {
			t.Errorf("let go, the clock made %v once its time was up", seen())
		}
	})
}

// Run on to its next call, the walks' clock makes that call and the
// others due with it, and leaves the later ones. With nothing set, it
// waits for a call to be set, the way a search that has just started
// sets its first report.
func TestTheWalksClockRunsOnToItsNextCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWalkTime()
		w.hold()
		var made []string
		var mu sync.Mutex
		seen := func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(made) }
		note := func(s string) func() {
			return func() { mu.Lock(); made = append(made, s); mu.Unlock() }
		}
		ran := make(chan time.Duration)
		go func() {
			d, _ := w.next(context.Background())
			ran <- d
		}()
		synctest.Wait()
		w.AfterFunc(500*time.Millisecond, note("a"))
		w.AfterFunc(500*time.Millisecond, note("b"))
		w.AfterFunc(time.Second, note("c"))
		if d := <-ran; d != 500*time.Millisecond {
			t.Errorf("the clock ran on %v to its next call, want 500ms", d)
		}
		if got := seen(); len(got) != 2 || !slices.Contains(got, "a") || !slices.Contains(got, "b") {
			t.Errorf("run on to its next call, the clock made %v, want a and b", got)
		}
		if d, _ := w.next(context.Background()); d != 500*time.Millisecond || len(seen()) != 3 {
			t.Errorf("the next call: ran %v and made %v", d, seen())
		}
	})
}

// A walk whose search ended before it reported again gives up its ask for
// the next call, and nothing waits on after it.
func TestTheWalksClockStopsWaitingForANextCallWhenTheWalkDoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newWalkTime()
		w.hold()
		ctx, cancel := context.WithCancel(context.Background())
		got := make(chan error)
		go func() {
			_, err := w.next(ctx)
			got <- err
		}()
		synctest.Wait()
		cancel()
		if err := <-got; err == nil {
			t.Error("the clock went on waiting for a call after the walk gave up")
		}
	})
}
