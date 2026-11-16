package engine

import (
	"io"
	"sync"
	"time"
)

// Clock is the time kept for work that waits on its own: the time of
// day, and a call once a while has gone by, which can be stopped. The
// programs keep the system's, SystemClock. The walks' bridge hands in
// the clock a walk runs on, held and run on with the page's, so a walk
// that holds its time holds this too and a busy machine spends none of
// it, see walkTime in cmd/framefairy-app/walktime_test.go. A test of the
// engine runs in a synctest bubble, whose clock is the system's to the
// engine and moves only as the bubble lets it.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

// SystemClock is the system's clock.
type SystemClock struct{}

// Now is the time of day.
func (SystemClock) Now() time.Time { return time.Now() }

// AfterFunc calls f once d has passed.
func (SystemClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// ticking ticks on its channel each time d has passed on clock, until the
// stop it returns is called, the way a time.Ticker does: a tick that
// comes while the one before it is still unread is left out.
func ticking(clock Clock, d time.Duration) (ticks <-chan struct{}, stop func()) {
	tick := make(chan struct{}, 1)
	var mu sync.Mutex
	stopped := false
	var cancel func() bool
	var arm func()
	arm = func() {
		cancel = clock.AfterFunc(d, func() {
			select {
			case tick <- struct{}{}:
			default:
			}
			mu.Lock()
			defer mu.Unlock()
			if !stopped {
				arm()
			}
		})
	}
	mu.Lock()
	arm()
	mu.Unlock()
	return tick, func() {
		mu.Lock()
		defer mu.Unlock()
		stopped = true
		cancel()
	}
}

// quietWatch calls gaveUp once nothing has arrived for after on its
// clock: a connection that went quiet without closing, a server that
// hangs. Every read through reader that brings bytes, and every call of
// arrived, puts it back. It keeps one call on the clock at a time, and
// when that comes early, because something arrived since, it asks again
// for what is left, so a stream that sends a piece every moment costs no
// timer a piece.
type quietWatch struct {
	clock  Clock
	after  time.Duration
	gaveUp func()

	mu      sync.Mutex
	last    time.Time
	cancel  func() bool
	stopped bool
}

// watchQuiet starts a quietWatch, the time from now on counting as quiet.
func watchQuiet(clock Clock, after time.Duration, gaveUp func()) *quietWatch {
	w := &quietWatch{clock: clock, after: after, gaveUp: gaveUp, last: clock.Now()}
	w.arm(after)
	return w
}

// arm asks the clock for a call in d. The clock is never asked while the
// lock is held, so a clock that calls at once does not wait on it.
func (w *quietWatch) arm(d time.Duration) {
	cancel := w.clock.AfterFunc(d, w.check)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		cancel()
		return
	}
	w.cancel = cancel
}

func (w *quietWatch) check() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	left := w.after - w.clock.Now().Sub(w.last)
	if left <= 0 {
		w.stopped = true
	}
	w.mu.Unlock()
	if left > 0 {
		w.arm(left)
		return
	}
	w.gaveUp()
}

// arrived puts the watch back: the quiet counts from now.
func (w *quietWatch) arrived() {
	now := w.clock.Now()
	w.mu.Lock()
	w.last = now
	w.mu.Unlock()
}

// Stop ends the watch, and gaveUp is not called from then on.
func (w *quietWatch) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.cancel != nil {
		w.cancel()
	}
}

// reader is r, and every read that brings something puts the watch back.
func (w *quietWatch) reader(r io.Reader) io.Reader {
	return readerFunc(func(p []byte) (int, error) {
		n, err := r.Read(p)
		if n > 0 {
			w.arrived()
		}
		return n, err
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
