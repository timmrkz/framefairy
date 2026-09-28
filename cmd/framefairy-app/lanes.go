package main

import (
	"context"
	"sync"
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
// for by itself, is neither taken back nor held up.
type lanes struct {
	mu      sync.Mutex
	held    map[string]*holder
	asks    []*ask
	changed chan struct{}
}

// A turn is either plain, or a step of a search: hearing, which gives way,
// or finding, which the hearing lane gives way to.
type turnKind int

const (
	plainTurn turnKind = iota
	hearingTurn
	findingTurn
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
		if hearing := l.held[LaneHearing]; hearing != nil && hearing.kind == hearingTurn {
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
// while a search finds. l.mu is held.
func (l *lanes) mayLocked(a *ask) bool {
	if l.held[a.lane] != nil {
		return false
	}
	if a.kind == hearingTurn {
		if finding := l.held[LaneFinding]; finding != nil && finding.kind == findingTurn {
			return false
		}
	}
	for _, earlier := range l.asks {
		if earlier == a {
			return true
		}
		if earlier.lane == a.lane {
			return false
		}
	}
	return true
}

func (l *lanes) dropLocked(a *ask) {
	for i, x := range l.asks {
		if x == a {
			l.asks = append(l.asks[:i], l.asks[i+1:]...)
			return
		}
	}
}

// laneOfStep is the lane a step of a job runs in.
func laneOfStep(step string) (string, turnKind) {
	switch step {
	case "hearing":
		return LaneHearing, hearingTurn
	case "finding":
		return LaneFinding, findingTurn
	case "framing":
		return LaneFraming, plainTurn
	}
	return LaneRendering, plainTurn
}
