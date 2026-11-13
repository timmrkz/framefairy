package engine

import (
	"bytes"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine/runlog"
)

// A ticking clock ticks each time its time has passed, leaves out a tick
// that comes while the one before it is unread, the way a time.Ticker
// does, and ticks no more once it is stopped.
func TestTickingTicksEachTimeItsTimeHasPassed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ticks, stop := ticking(SystemClock{}, 500*time.Millisecond)
		time.Sleep(499 * time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(ticks) != 0 {
			t.Fatal("it ticked before its time")
		}
		time.Sleep(time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(ticks) != 1 {
			t.Fatal("it did not tick once its time had passed")
		}
		<-ticks
		// Three ticks' time with nobody reading is one tick, not three.
		time.Sleep(1500 * time.Millisecond) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		<-ticks
		if len(ticks) != 0 {
			t.Fatal("it kept the ticks nobody read")
		}
		stop()
		time.Sleep(time.Hour) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(ticks) != 0 {
			t.Error("it ticked after it was stopped")
		}
	})
}

// handClock is a clock a test moves by hand. It stands still, and tick
// makes the calls set on it, the way the walks' clock does while a walk
// holds it.
type handClock struct {
	mu    sync.Mutex
	at    time.Time
	calls []func()
	asked []time.Duration
}

func (h *handClock) Now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.at
}

func (h *handClock) AfterFunc(d time.Duration, f func()) func() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.asked = append(h.asked, d)
	i := len(h.calls)
	h.calls = append(h.calls, f)
	return func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		was := h.calls[i] != nil
		h.calls[i] = nil
		return was
	}
}

// tick runs the clock on by d and makes every call set so far.
func (h *handClock) tick(d time.Duration) {
	h.mu.Lock()
	h.at = h.at.Add(d)
	calls := h.calls
	h.calls = make([]func(), len(calls))
	h.mu.Unlock()
	for _, f := range calls {
		if f != nil {
			f()
		}
	}
}

// A search says how far it has come every half second of the machine's
// clock, and only on it, so a walk that holds that clock hears a report
// for each step it runs it on, and none while it holds it.
func TestASearchReportsOnTheMachinesClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := &handClock{}
		log := runlog.New(&bytes.Buffer{}, false, false)
		reports := make(chan runlog.Event, 10)
		log.SetSink(func(ev runlog.Event) {
			if ev.Kind == runlog.EventProgress {
				reports <- ev
			}
		})
		c := newSearchClock(log, clock, nil, "gemma", true, 1000, 12, 2048)
		c.enter(partReading)
		go c.run()
		defer c.stop()
		time.Sleep(time.Minute) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if len(reports) != 0 {
			t.Fatalf("a minute of the system's clock passed with the machine's held, and the search reported %d times", len(reports))
		}
		for i := range 3 {
			clock.tick(500 * time.Millisecond)
			synctest.Wait()
			if n := len(reports); n != 1 {
				t.Fatalf("step %d of the machine's clock made %d reports, want 1", i+1, n)
			}
			<-reports
		}
		clock.mu.Lock()
		asked := clock.asked
		clock.mu.Unlock()
		for _, d := range asked {
			if d != 500*time.Millisecond {
				t.Errorf("the search asked to report after %v, want every half second", d)
			}
		}
	})
}
