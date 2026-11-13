package engine

import (
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
