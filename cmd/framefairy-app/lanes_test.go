package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine"
)

// given takes a turn. Every test here runs in a synctest bubble, so a turn
// that never comes leaves everything in the bubble blocked, and synctest
// fails the test at once.
func given(t *testing.T, l *lanes, lane string, kind turnKind) (context.Context, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	turn, release, err := l.take(ctx, lane, kind)
	if err != nil {
		t.Fatalf("no turn in %s: %v", lane, err)
	}
	return turn, release
}

// waiting starts asking for a turn and says on the channel when it comes.
// It gives up as the test ends, so nothing is left waiting in the bubble.
func waiting(t *testing.T, l *lanes, lane string, kind turnKind) (chan func(), context.CancelFunc) {
	got := make(chan func(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := l.enqueue(lane, kind)
	go func() {
		if _, release, err := l.wait(ctx, a); err == nil {
			got <- release
		}
	}()
	return got, cancel
}

// comes says that a turn asked for has come. Once everything else in the
// bubble is blocked, nothing more happens until the test takes its next
// step, so a turn that has not come by then never comes on its own.
func comes(t *testing.T, got chan func(), what string) func() {
	t.Helper()
	synctest.Wait()
	select {
	case release := <-got:
		return release
	default:
		t.Fatalf("%s never had its turn", what)
		return nil
	}
}

// waits says that a turn asked for has not come, by the same reasoning.
func waits(t *testing.T, got chan func(), what string) {
	t.Helper()
	synctest.Wait()
	select {
	case <-got:
		t.Fatalf("%s had its turn", what)
	default:
	}
}

func TestOneTurnAtATimeInALaneInTheOrderAsked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		_, release := given(t, l, LaneFinding, plainTurn)
		second, _ := waiting(t, l, LaneFinding, plainTurn)
		third, _ := waiting(t, l, LaneFinding, plainTurn)
		// The other lane is free all the while.
		_, other := given(t, l, LaneHearing, plainTurn)
		other()
		waits(t, second, "the second")
		release()
		release = comes(t, second, "the second")
		waits(t, third, "the third")
		release()
		comes(t, third, "the third")()
	})
}

func TestATurnGivenUpLeavesItsPlace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		_, release := given(t, l, LaneFinding, plainTurn)
		second, giveUp := waiting(t, l, LaneFinding, plainTurn)
		third, _ := waiting(t, l, LaneFinding, plainTurn)
		giveUp()
		release()
		comes(t, third, "the one behind the one that gave up")()
		waits(t, second, "the one that gave up")
	})
}

// A search that comes to finding takes the lane back from a search that
// hears, and hearing waits until it has found.
func TestFindingTakesHearingBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		hearing, heard := given(t, l, LaneHearing, hearingTurn)
		_, found := given(t, l, LaneFinding, findingTurn)
		// Finding takes the lane back as it takes its turn, before take
		// returns.
		if hearing.Err() == nil {
			t.Fatal("the hearing search was not stopped")
		}
		heard()
		again, _ := waiting(t, l, LaneHearing, hearingTurn)
		waits(t, again, "hearing while a search finds")
		found()
		comes(t, again, "hearing after the search found")()
	})
}

// A search whose language model is in the cloud shares nothing with the
// speech model, so a search that hears goes on while it finds and is not
// taken back. A video added while it waited for its answer sat at Waiting
// to transcribe for as long as the cloud took. On this machine the rule
// holds. Plan row 2.222.
func TestFindingInTheCloudLeavesHearingAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		lane, kind := laneOfStep("search", "finding", false)
		hearing, heard := given(t, l, LaneHearing, hearingTurn)
		_, found := given(t, l, lane, kind)
		if hearing.Err() != nil {
			t.Fatal("the hearing search was stopped by a search finding in the cloud")
		}
		heard()
		again, _ := waiting(t, l, LaneHearing, hearingTurn)
		comes(t, again, "hearing while a search finds in the cloud")()
		found()
		if _, kind := laneOfStep("search", "finding", true); kind != findingTurn {
			t.Error("finding with the model on this machine no longer holds hearing up")
		}
	})
}

// A search takes the turn of finding the clips by where its settings put
// the language model, so a search asking the cloud lets hearing go on and
// one asking the model on this machine holds it up. A local model served
// by llama-server on another machine, LLMURL, shares nothing with the
// speech model either, issue #276.
func TestASearchTakesItsTurnByWhereItsModelIs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, c := range []struct {
			planner, url string
			holds        bool
		}{{"api", "", false}, {"local", "", true}, {"", "", true}, {"local", "http://studio.local:8080", false}} {
			q := newQueue(nil, func(JobUpdate) {}, func(string) {})
			ctx, cancel := context.WithCancel(context.Background())
			_, found, err := q.turn(&Job{Kind: "search"}, engine.Options{Planner: c.planner, LLMURL: c.url})(ctx, "finding")
			if err != nil {
				t.Fatal(err)
			}
			hearing, stop := waiting(t, q.lanes, LaneHearing, hearingTurn)
			if c.holds {
				waits(t, hearing, "hearing while a search with the model "+c.planner+" "+c.url+" finds")
			} else {
				comes(t, hearing, "hearing while a search with the model "+c.planner+" "+c.url+" finds")()
			}
			found()
			stop()
			cancel()
		}
	})
}

// A turn taken back says the job waits the moment it is, before whoever
// took it has its turn, while the step still finishes the piece it is in,
// so the whole video goes on hold on the range picker as Search is
// pressed and not once its piece is heard, Tim's report of 24 October. A turn let go of as its step ends says
// nothing of the kind, and neither does a job that is stopped.
func TestATurnTakenBackSaysItWaitsAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := newQueue(nil, func(JobUpdate) {}, func(string) {})
		step := func(j *Job) string {
			q.mu.Lock()
			defer q.mu.Unlock()
			return j.Step
		}
		local := engine.Options{Planner: "local"}

		whole := &Job{Kind: engine.JobTranscribe}
		_, heard, err := q.turn(whole, local)(context.Background(), engine.StepHearing)
		if err != nil {
			t.Fatal(err)
		}
		_, found, err := q.turn(&Job{Kind: engine.JobSearch}, local)(context.Background(), "finding")
		if err != nil {
			t.Fatal(err)
		}
		// Before the search has said it finds.
		if got := step(whole); got != engine.StepWaiting {
			t.Errorf("the whole video taken back by a search that finds says %q", got)
		}
		heard()
		found()

		// Let go of as its step ends, it stays in the step it was in.
		done := &Job{Kind: engine.JobTranscribe}
		_, let, err := q.turn(done, local)(context.Background(), engine.StepHearing)
		if err != nil {
			t.Fatal(err)
		}
		let()
		synctest.Wait()
		if got := step(done); got != engine.StepHearing {
			t.Errorf("a turn let go of says %q", got)
		}

		// Stopped, it is the stop that says what became of it.
		ctx, stop := context.WithCancel(context.Background())
		stopped := &Job{Kind: engine.JobTranscribe}
		_, gone, err := q.turn(stopped, local)(ctx, engine.StepHearing)
		if err != nil {
			t.Fatal(err)
		}
		stop()
		synctest.Wait()
		if got := step(stopped); got != engine.StepHearing {
			t.Errorf("a job stopped says %q", got)
		}
		gone()
	})
}

// A clip made by hand hears first: it takes the lane back from a search
// that hears, goes ahead of a search waiting to hear, and does not wait for
// a search that finds. Clips made by hand hear in the order asked.
func TestAClipMadeByHandHearsFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		hearing, heard := given(t, l, LaneHearing, hearingTurn)
		later, _ := waiting(t, l, LaneHearing, hearingTurn)
		first, _ := waiting(t, l, LaneHearing, handHearingTurn)
		second, _ := waiting(t, l, LaneHearing, handHearingTurn)
		synctest.Wait()
		if hearing.Err() == nil {
			t.Fatal("the hearing search was not stopped for a clip made by hand")
		}
		// The search lets go, saving what it heard, and asks again behind them.
		heard()
		again, _ := waiting(t, l, LaneHearing, hearingTurn)
		done := comes(t, first, "the first clip made by hand")
		waits(t, second, "the second clip made by hand before the first is done")
		waits(t, later, "a search asked before the clips made by hand")
		done()
		comes(t, second, "the second clip made by hand")()
		comes(t, later, "the search asked before")()
		comes(t, again, "the search that was taken back")()

		// Not held up by a search that finds, and not taken back by one.
		_, found := given(t, l, LaneFinding, findingTurn)
		hand, handDone := given(t, l, LaneHearing, handHearingTurn)
		found2, _ := waiting(t, l, LaneFinding, findingTurn)
		found()
		comes(t, found2, "the next search's finding")()
		if hand.Err() != nil {
			t.Error("a clip made by hand was stopped for a search that finds")
		}
		handDone()
	})
}

// Work that is not a step of a search is neither taken back nor held up:
// a model being installed goes on while a search finds.
func TestFindingLeavesOtherWorkAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		install, done := given(t, l, LaneHearing, plainTurn)
		_, found := given(t, l, LaneFinding, findingTurn)
		if install.Err() != nil {
			t.Error("an install was stopped for a search")
		}
		done()
		_, next := given(t, l, LaneHearing, plainTurn)
		next()
		found()
	})
}

// Asked for, given up and let go from many goroutines at once, a lane
// never has two turns at a time and every turn that is not given up comes,
// the whole video's hearing that gives way to every other among them.
func TestTurnsFromEverywhereAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		var inLane [2]atomic.Int32
		lanesOf := []string{LaneHearing, LaneFinding}
		var wg sync.WaitGroup
		for i := range 40 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				which := i % 2
				kind := []turnKind{plainTurn, hearingTurn, findingTurn, handHearingTurn, wholeHearingTurn}[(i/2)%5]
				if which == 0 && kind == findingTurn {
					kind = hearingTurn
				}
				if which == 1 && (kind == hearingTurn || kind == handHearingTurn || kind == wholeHearingTurn) {
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
				case <-time.After(2 * time.Millisecond): //nolint:forbidigo // synctest bubble
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
	})
}

// The whole video heard by itself, Transcribe whole video in its menu,
// hears last: it gives the hearing lane to a search or a clip made by hand
// that asks for it, a search asked after it goes ahead of it, and it waits
// while a search finds. Nobody waits on it the way somebody waits on a
// search, and it can take an hour. Issue #432.
//
// Every wait is on a turn coming or a turn's context ending, and which of
// two comes first is read from the one channel that gets it.
func TestTheWholeVideoHearsLast(t *testing.T) {
	if lane, kind := laneOfStep(engine.JobTranscribe, "hearing", true); lane != LaneHearing || kind != wholeHearingTurn {
		t.Fatalf("the whole video hears in %s as %d", lane, kind)
	}
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		whole, wholeDone := given(t, l, LaneHearing, wholeHearingTurn)
		search, _ := waiting(t, l, LaneHearing, hearingTurn)
		<-whole.Done()
		wholeDone()
		again, _ := waiting(t, l, LaneHearing, wholeHearingTurn)
		searched := comes(t, search, "the search")
		hand, _ := waiting(t, l, LaneHearing, handHearingTurn)
		searched()
		comes(t, hand, "the clip made by hand, before the whole video,")()
		comes(t, again, "the whole video")()

		// A search asked after it goes ahead of it.
		_, held := given(t, l, LaneHearing, plainTurn)
		later, _ := waiting(t, l, LaneHearing, wholeHearingTurn)
		first, _ := waiting(t, l, LaneHearing, hearingTurn)
		held()
		comes(t, first, "the search asked after the whole video")()
		comes(t, later, "the whole video")()

		// A search that finds takes it back and holds it up.
		whole, wholeDone = given(t, l, LaneHearing, wholeHearingTurn)
		_, found := given(t, l, LaneFinding, findingTurn)
		<-whole.Done()
		wholeDone()
		after, _ := waiting(t, l, LaneHearing, wholeHearingTurn)
		waits(t, after, "the whole video while a search finds")
		found()
		comes(t, after, "the whole video")()

		// A model being installed takes it back too, and goes first: it
		// waited behind the whole video for as long as that heard. Found by
		// the second review of #458.
		whole, wholeDone = given(t, l, LaneHearing, wholeHearingTurn)
		install, _ := waiting(t, l, LaneHearing, plainTurn)
		<-whole.Done()
		if !errors.Is(whole.Err(), context.Canceled) {
			t.Fatalf("the whole video kept the lane from the model being installed: %v", whole.Err())
		}
		wholeDone()
		resumed, _ := waiting(t, l, LaneHearing, wholeHearingTurn)
		comes(t, install, "the model being installed, before the whole video,")()
		comes(t, resumed, "the whole video")()
	})
}

// What is asked for after the whole video never waits behind it, because
// the whole video waits for every search that asks to hear. A model being
// installed, asked for after it and before a search, once waited behind
// the whole video, the search behind the install, and the whole video for
// the search, so none of them ever heard. Found by the review of #458.
func TestNothingWaitsBehindTheWholeVideo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLanes()
		_, held := given(t, l, LaneHearing, plainTurn)
		whole := l.enqueue(LaneHearing, wholeHearingTurn)
		install := l.enqueue(LaneHearing, plainTurn)
		search := l.enqueue(LaneHearing, hearingTurn)
		held()
		l.mu.Lock()
		wholeMay, installMay, searchMay := l.mayLocked(whole), l.mayLocked(install), l.mayLocked(search)
		l.mu.Unlock()
		if wholeMay || !installMay || searchMay {
			t.Fatalf("with the lane free, the whole video may hear %v, the install %v, the search %v",
				wholeMay, installMay, searchMay)
		}
		// And so each has its turn, the whole video last.
		for _, a := range []*ask{install, search, whole} {
			_, done, err := l.wait(context.Background(), a)
			if err != nil {
				t.Fatal(err)
			}
			done()
		}
	})
}
