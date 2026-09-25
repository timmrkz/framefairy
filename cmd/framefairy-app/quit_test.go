package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A leaving that records what it said and whether it stopped and quit.
type leaveProbe struct {
	mu      sync.Mutex
	said    []string
	stopped atomic.Int32
	quit    chan struct{}
}

func newLeaving(busy bool, stopTakes time.Duration) (*leaving, *leaveProbe) {
	p := &leaveProbe{quit: make(chan struct{}, 4)}
	l := &leaving{
		busy: func() bool { return busy },
		say: func(what string) {
			p.mu.Lock()
			p.said = append(p.said, what)
			p.mu.Unlock()
		},
		stop: func() {
			time.Sleep(stopTakes)
			p.stopped.Add(1)
		},
	}
	l.quit = func() {
		// Quitting for real asks again, the way Wails does.
		if l.shouldQuit() {
			p.quit <- struct{}{}
		}
	}
	return l, p
}

func (p *leaveProbe) words() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.said...)
}

func (p *leaveProbe) quitWithin(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-p.quit:
	case <-time.After(d):
		t.Fatal("the app never quit")
	}
}

// Cmd+Q answers at once, whatever stopping takes. It used to stop
// everything on the main thread, which froze the app under the spinning
// wheel for five seconds with nothing to say the key had been heard.
func TestCmdQAnswersAtOnceAndQuitsWhenStopped(t *testing.T) {
	l, p := newLeaving(false, 300*time.Millisecond)
	began := time.Now()
	if l.shouldQuit() {
		t.Fatal("quit before anything was stopped")
	}
	if took := time.Since(began); took > 50*time.Millisecond {
		t.Errorf("Cmd+Q took %s to answer", took)
	}
	if got := p.words(); len(got) != 1 || got[0] != "going" {
		t.Errorf("the interface heard %v", got)
	}
	p.quitWithin(t, 2*time.Second)
	if p.stopped.Load() != 1 {
		t.Errorf("stopped %d times", p.stopped.Load())
	}
}

// While work runs the first Cmd+Q only asks, and the second quits.
func TestCmdQWhileWorkRunsAsksFirst(t *testing.T) {
	l, p := newLeaving(true, 0)
	if l.shouldQuit() {
		t.Fatal("the first Cmd+Q quit while a search ran")
	}
	if got := p.words(); len(got) != 1 || got[0] != "ask" {
		t.Fatalf("the interface heard %v", got)
	}
	if p.stopped.Load() != 0 {
		t.Fatal("the first Cmd+Q stopped the work")
	}
	l.shouldQuit()
	p.quitWithin(t, 2*time.Second)
	if got := p.words(); len(got) != 2 || got[1] != "going" {
		t.Errorf("the interface heard %v", got)
	}
}

// A second Cmd+Q long after the first asks again.
func TestALateSecondCmdQAsksAgain(t *testing.T) {
	l, p := newLeaving(true, 0)
	l.shouldQuit()
	l.mu.Lock()
	l.askedAt = time.Now().Add(-2 * quitAgain)
	l.mu.Unlock()
	l.shouldQuit()
	if got := p.words(); len(got) != 2 || got[1] != "ask" {
		t.Errorf("the interface heard %v", got)
	}
}

// Closing the app's window quits without asking: there is no window left to
// press Cmd+Q into a second time.
func TestClosingTheWindowQuitsWithoutAsking(t *testing.T) {
	l, p := newLeaving(true, 0)
	l.closing()
	l.shouldQuit()
	p.quitWithin(t, 2*time.Second)
}

// Cmd+Q pressed again and again while the app stops stops it once and
// quits once.
func TestCmdQPressedOverAndOverStopsOnce(t *testing.T) {
	l, p := newLeaving(false, 200*time.Millisecond)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { l.shouldQuit() })
	}
	wg.Wait()
	p.quitWithin(t, 2*time.Second)
	select {
	case <-p.quit:
		t.Error("quit twice")
	case <-time.After(300 * time.Millisecond):
	}
	if p.stopped.Load() != 1 {
		t.Errorf("stopped %d times", p.stopped.Load())
	}
}
