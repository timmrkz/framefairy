package main

import (
	"context"
	"sync"

	"framefairy/engine"
)

// lanes hands out turns in the queue's lanes, one job at a time in each, in
// the order they were asked for. A job that goes through several steps,
// like a search that hears and then finds, asks for a turn in the lane of
// each step as it comes to it, and lets the last one go.
//
// One rule is kept here, where it can be seen: while a search finds, no
// search hears. The speech model and the language model want the same
// memory and the same graphics chip, and each is slower for sharing them.
// So a search that comes to finding takes the hearing lane back from a
// search that hears, which saves what it heard and waits for its turn
// again, and hearing waits for as long as a search finds. Work that is
// not a step of a search, a model being installed or a transcription asked
// for by itself, is neither taken back nor held up. A search whose
// language model is in the cloud shares nothing with the speech model, so
// it finds like any other work and hearing goes on beside it: a video
// added while it waited for its answer sat at Waiting to transcribe for
// as long as the cloud took. Plan row 2.222.
//
// And one more: a clip made by hand hears first. Somebody pressed I or O
// and is looking at the card, and what it hears is the minute around the
// playhead, where a search may hear an hour. So it goes ahead of every
// search waiting to hear, takes the lane back from one that hears, the way
// finding does, and does not wait for a search that finds: a minute of
// audio is heard in a second or two. Clips made by hand hear in the order
// they were asked for.
//
// And last: the whole video heard by itself, Transcribe whole video in
// its menu, hears only when no search and no clip needs the lane. It can
// take an hour, and nobody waits on it the way somebody waits on a search
// or a clip, so it goes behind every one of them that asks to hear, gives
// the lane back to each, and waits while a search finds, like a search
// that hears. Issue #432.
type lanes struct {
	mu      sync.Mutex
	held    map[string]*holder
	asks    []*ask
	changed chan struct{}
}

// A turn is either plain, or a step of a search: hearing, which gives way,
// or finding, which the hearing lane gives way to, or the hearing of a clip
// made by hand, which everything of a search gives way to, or the hearing
// of a whole video, which gives way to all of them.
type turnKind int

const (
	plainTurn turnKind = iota
	hearingTurn
	findingTurn
	handHearingTurn
	wholeHearingTurn
)

type holder struct {
	kind   turnKind
	cancel context.CancelFunc
}

// ask is a turn asked for and not yet given.
type ask struct {
	lane string
	kind turnKind
}

func newLanes() *lanes {
	return &lanes{held: map[string]*holder{}, changed: make(chan struct{})}
}

// wakeLocked tells everyone waiting that something changed. l.mu is held.
func (l *lanes) wakeLocked() {
	close(l.changed)
	l.changed = make(chan struct{})
}

// enqueue asks for a turn now, so the turn keeps its place from this
// moment, and wait gives it.
func (l *lanes) enqueue(lane string, kind turnKind) *ask {
	a := &ask{lane: lane, kind: kind}
	l.mu.Lock()
	l.asks = append(l.asks, a)
	l.mu.Unlock()
	return a
}

// take asks for a turn and waits for it.
func (l *lanes) take(ctx context.Context, lane string, kind turnKind) (context.Context, func(), error) {
	return l.wait(ctx, l.enqueue(lane, kind))
}

// wait waits for a turn asked for, and gives the context the turn runs in,
// which ends when the lane is taken back, and the function that lets it
// go. It gives up when ctx ends.
func (l *lanes) wait(ctx context.Context, a *ask) (context.Context, func(), error) {
	l.mu.Lock()
	for ctx.Err() != nil || !l.mayLocked(a) {
		// Given up, it gives up even when the lane has just come free:
		// the turn goes to whoever is next.
		if ctx.Err() != nil {
			l.dropLocked(a)
			l.wakeLocked()
			l.mu.Unlock()
			return nil, nil, ctx.Err()
		}
		// A clip made by hand takes the lane back from a search that
		// hears. The search saves what it heard and asks again, behind it.
		// Both take it back from the whole video heard by itself.
		if hearing := l.held[a.lane]; hearing != nil && givesWay(hearing.kind, a.kind) {
			hearing.cancel()
		}
		changed := l.changed
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.dropLocked(a)
			l.wakeLocked()
			l.mu.Unlock()
			return nil, nil, ctx.Err()
		case <-changed:
		}
		l.mu.Lock()
	}
	l.dropLocked(a)
	turnCtx, cancel := context.WithCancel(ctx)
	h := &holder{kind: a.kind, cancel: cancel}
	l.held[a.lane] = h
	if a.kind == findingTurn {
		if hearing := l.held[LaneHearing]; hearing != nil && (hearing.kind == hearingTurn || hearing.kind == wholeHearingTurn) {
			hearing.cancel()
		}
	}
	l.wakeLocked()
	l.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			l.mu.Lock()
			if l.held[a.lane] == h {
				delete(l.held, a.lane)
			}
			l.wakeLocked()
			l.mu.Unlock()
			cancel()
		})
	}
	return turnCtx, release, nil
}

// mayLocked says whether an ask may have its turn now: its lane is free,
// nobody asked for that lane before it, and it is not a search hearing
// while a search finds. A clip made by hand that hears goes ahead of a
// search that asked to hear before it. l.mu is held.
func (l *lanes) mayLocked(a *ask) bool {
	if l.held[a.lane] != nil {
		return false
	}
	if a.kind == hearingTurn || a.kind == wholeHearingTurn {
		if finding := l.held[LaneFinding]; finding != nil && finding.kind == findingTurn {
			return false
		}
		for _, other := range l.asks {
			if other.lane == a.lane && givesWay(a.kind, other.kind) {
				return false
			}
		}
	}
	for _, earlier := range l.asks {
		if earlier == a {
			return true
		}
		if earlier.lane == a.lane && !passes(a.kind, earlier.kind) {
			return false
		}
	}
	return true
}

// givesWay says whether a turn that hears gives the hearing lane to another
// that asks for it: a search to a clip made by hand, and the whole video
// to both.
func givesWay(held, asks turnKind) bool {
	switch held {
	case hearingTurn:
		return asks == handHearingTurn
	case wholeHearingTurn:
		return asks == hearingTurn || asks == handHearingTurn
	}
	return false
}

// passes says whether an ask goes ahead of one in the same lane asked for
// before it: a clip made by hand goes ahead of everything but another clip,
// and a search that hears goes ahead of the whole video.
func passes(a, earlier turnKind) bool {
	switch a {
	case handHearingTurn:
		return earlier != handHearingTurn
	case hearingTurn:
		return earlier == wholeHearingTurn
	}
	return false
}

func (l *lanes) dropLocked(a *ask) {
	for i, x := range l.asks {
		if x == a {
			l.asks = append(l.asks[:i], l.asks[i+1:]...)
			return
		}
	}
}

// laneOfStep is the lane a step of a kind of job runs in, and how it
// takes its turn. local says whether the job's language model runs on
// this machine, where finding holds hearing up.
func laneOfStep(kind, step string, local bool) (string, turnKind) {
	switch step {
	case "hearing":
		if kind == engine.JobClip {
			return LaneHearing, handHearingTurn
		}
		if kind == engine.JobTranscribe {
			return LaneHearing, wholeHearingTurn
		}
		return LaneHearing, hearingTurn
	case "finding":
		if !local {
			return LaneFinding, plainTurn
		}
		return LaneFinding, findingTurn
	case "framing":
		return LaneFraming, plainTurn
	}
	return LaneRendering, plainTurn
}
