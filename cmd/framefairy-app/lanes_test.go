package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// given takes a turn and fails the test if it does not come in time.
func given(t *testing.T, l *lanes, lane string, kind turnKind) (context.Context, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	turn, release, err := l.take(ctx, lane, kind)
	if err != nil {
		t.Fatalf("no turn in %s: %v", lane, err)
	}
	return turn, release
}

// waiting starts asking for a turn and says on the channel when it comes.
func waiting(l *lanes, lane string, kind turnKind) (chan func(), context.CancelFunc) {
	got := make(chan func(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	a := l.enqueue(lane, kind)
	go func() {
		if _, release, err := l.wait(ctx, a); err == nil {
			got <- release
		}
	}()
	return got, cancel
}

func comes(t *testing.T, got chan func(), what string) func() {
	t.Helper()
	select {
	case release := <-got:
		return release
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never had its turn", what)
		return nil
	}
}

func waits(t *testing.T, got chan func(), what string) {
	t.Helper()
	select {
	case <-got:
		t.Fatalf("%s had its turn", what)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestOneTurnAtATimeInALaneInTheOrderAsked(t *testing.T) {
	l := newLanes()
	_, release := given(t, l, LaneWork, plainTurn)
	second, _ := waiting(l, LaneWork, plainTurn)
	third, _ := waiting(l, LaneWork, plainTurn)
	// The other lane is free all the while.
	_, other := given(t, l, LaneTranscribe, plainTurn)
	other()
	waits(t, second, "the second")
	release()
	release = comes(t, second, "the second")
	waits(t, third, "the third")
	release()
	comes(t, third, "the third")()
}

func TestATurnGivenUpLeavesItsPlace(t *testing.T) {
	l := newLanes()
	_, release := given(t, l, LaneWork, plainTurn)
	second, giveUp := waiting(l, LaneWork, plainTurn)
	third, _ := waiting(l, LaneWork, plainTurn)
	giveUp()
	release()
	comes(t, third, "the one behind the one that gave up")()
	waits(t, second, "the one that gave up")
}

// A search that comes to finding takes the lane back from a search that
// hears, and hearing waits until it has found.
func TestFindingTakesHearingBack(t *testing.T) {
	l := newLanes()
	hearing, heard := given(t, l, LaneTranscribe, hearingTurn)
	_, found := given(t, l, LaneWork, findingTurn)
	select {
	case <-hearing.Done():
	case <-time.After(time.Second):
		t.Fatal("the hearing search was not stopped")
	}
	heard()
	again, _ := waiting(l, LaneTranscribe, hearingTurn)
	waits(t, again, "hearing while a search finds")
	found()
	comes(t, again, "hearing after the search found")()
}

// Work that is not a step of a search is neither taken back nor held up:
// a model being installed goes on while a search finds.
func TestFindingLeavesOtherWorkAlone(t *testing.T) {
	l := newLanes()
	install, done := given(t, l, LaneTranscribe, plainTurn)
	_, found := given(t, l, LaneWork, findingTurn)
	if install.Err() != nil {
		t.Error("an install was stopped for a search")
	}
	done()
	_, next := given(t, l, LaneTranscribe, plainTurn)
	next()
	found()
}

// Asked for, given up and let go from many goroutines at once, a lane
// never has two turns at a time and every turn that is not given up comes.
func TestTurnsFromEverywhereAtOnce(t *testing.T) {
	l := newLanes()
	var inLane [2]atomic.Int32
	lanesOf := []string{LaneTranscribe, LaneWork}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			which := i % 2
			kind := []turnKind{plainTurn, hearingTurn, findingTurn}[i%3]
			if which == 0 && kind == findingTurn {
				kind = hearingTurn
			}
			if which == 1 && kind == hearingTurn {
				kind = findingTurn
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if i%7 == 0 {
				// Some give up at once.
				short, stop := context.WithTimeout(ctx, time.Millisecond)
				defer stop()
				ctx = short
			}
			turn, release, err := l.take(ctx, lanesOf[which], kind)
			if err != nil {
				if i%7 != 0 {
					t.Errorf("turn %d never came: %v", i, err)
				}
				return
			}
			if n := inLane[which].Add(1); n > 1 {
				t.Errorf("%d turns at once in %s", n, lanesOf[which])
			}
			select {
			case <-turn.Done():
			case <-time.After(2 * time.Millisecond):
			}
			inLane[which].Add(-1)
			release()
			release()
		}()
	}
	wg.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.held) != 0 || len(l.asks) != 0 {
		t.Errorf("left behind: %d held, %d asked", len(l.held), len(l.asks))
	}
}
