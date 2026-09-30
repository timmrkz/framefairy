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

// asked is quitting asked for in the app already, the way Relaunch does.
func newLeaving(asked bool, stopTakes time.Duration) (*leaving, *leaveProbe) {
	p := &leaveProbe{quit: make(chan struct{}, 4)}
	l := &leaving{
		asked: func() bool { return asked },
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
	l.shouldQuit()
	began := time.Now()
	if l.shouldQuit() {
		t.Fatal("quit before anything was stopped")
	}
	if took := time.Since(began); took > 50*time.Millisecond {
		t.Errorf("Cmd+Q took %s to answer", took)
	}
	if got := p.words(); len(got) != 2 || got[1] != "going" {
		t.Errorf("the interface heard %v", got)
	}
	p.quitWithin(t, 2*time.Second)
	if p.stopped.Load() != 1 {
		t.Errorf("stopped %d times", p.stopped.Load())
	}
}

// The first Cmd+Q only asks, and the second quits. Always, whether work
// runs or not: it asked only while work ran, so the workspace took two
// presses and the settings page quit on the first.
func TestTheFirstCmdQAlwaysAsks(t *testing.T) {
	l, p := newLeaving(false, 0)
	if l.shouldQuit() {
		t.Fatal("the first Cmd+Q quit")
	}
	if got := p.words(); len(got) != 1 || got[0] != "ask" {
		t.Fatalf("the interface heard %v", got)
	}
	if p.stopped.Load() != 0 {
		t.Fatal("the first Cmd+Q stopped everything")
	}
	l.shouldQuit()
	p.quitWithin(t, 2*time.Second)
	if got := p.words(); len(got) != 2 || got[1] != "going" {
		t.Errorf("the interface heard %v", got)
	}
}

// A second Cmd+Q long after the first asks again.
func TestALateSecondCmdQAsksAgain(t *testing.T) {
	l, p := newLeaving(false, 0)
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
	l, p := newLeaving(false, 0)
	l.closing()
	l.shouldQuit()
	p.quitWithin(t, 2*time.Second)
}

// Relaunch on the Updates page quits into the new build without asking: it
// was asked for with a click, and the question would stand in its way.
func TestRelaunchQuitsWithoutAsking(t *testing.T) {
	l, p := newLeaving(true, 0)
	l.shouldQuit()
	p.quitWithin(t, 2*time.Second)
	if got := p.words(); len(got) != 1 || got[0] != "going" {
		t.Errorf("the interface heard %v", got)
	}
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
