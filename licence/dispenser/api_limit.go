package dispenser

import (
	"sync"
	"time"
)

// limiter allows n requests per key in each window of time. It lives in
// one container's memory, so with several containers running each has its
// own count: it keeps a page from being used to flood an inbox, not a
// determined attacker from asking a few times more.
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	counts map[string]*count
}

type count struct {
	n     int
	since time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, counts: map[string]*count{}}
}

// allow says whether one more request for key fits, and counts it if so.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.counts) > 100000 {
		// Forget the windows that have ended, so the map cannot grow
		// without bound.
		for k, c := range l.counts {
			if now.Sub(c.since) >= l.window {
				delete(l.counts, k)
			}
		}
	}
	c, ok := l.counts[key]
	if !ok || now.Sub(c.since) >= l.window {
		l.counts[key] = &count{n: 1, since: now}
		return true
	}
	if c.n >= l.n {
		return false
	}
	c.n++
	return true
}
