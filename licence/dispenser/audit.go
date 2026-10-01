package dispenser

import (
	"context"
	"errors"
	"fmt"

	"framefairy/licence"
)

// ErrAudit is a store whose pool, sales or revocation list are not what
// its record adds up to.
var ErrAudit = errors.New("the store is not what its record adds up to")

// Audit checks the whole store against its record. The record is the
// history, and the pool, the sales and the revocation list are what it
// adds up to, so the audit plays every line from the first, refusing any
// line that could not have happened at that point, and then compares what
// it built with what the store holds, key by key and seat by seat. It also
// checks the chain and the anchors given, as VerifyRecord does. The daily
// run calls it, and every test of the engine ends with it.
func (e *Engine) Audit(ctx context.Context, anchors ...Anchor) (Anchor, error) {
	head, err := e.VerifyRecord(ctx, anchors...)
	if err != nil {
		return head, err
	}
	err = e.store.View(ctx, func(tx Tx) error {
		r := newReplay()
		var seq int64
		for {
			lines, err := tx.Lines(seq, 1000)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				break
			}
			for _, l := range lines {
				if err := r.play(l); err != nil {
					return fmt.Errorf("%w: line %d, %s: %w", ErrAudit, l.Seq, l.Event, err)
				}
				seq = l.Seq
			}
		}
		if seq != head.Seq {
			return fmt.Errorf("%w: the record changed while it was read", ErrAudit)
		}
		return r.compare(tx)
	})
	return head, err
}

// replay is the state built from the record alone.
type replay struct {
	pool    map[licence.Fingerprint]State
	order   []licence.Fingerprint
	seats   map[seatAt]licence.Fingerprint
	holder  map[licence.Fingerprint]seatAt
	held    map[licence.Fingerprint]bool
	revoked map[licence.Fingerprint]Revocation
}

type seatAt struct {
	source, ref string
	seat        int
}

func newReplay() *replay {
	return &replay{
		pool:    map[licence.Fingerprint]State{},
		seats:   map[seatAt]licence.Fingerprint{},
		holder:  map[licence.Fingerprint]seatAt{},
		held:    map[licence.Fingerprint]bool{},
		revoked: map[licence.Fingerprint]Revocation{},
	}
}

func (r *replay) play(l Line) error {
	f := l.Fingerprint
	at := seatAt{l.Source, l.Ref, l.Seat}
	state, inPool := r.pool[f]
	switch l.Event {
	case EventStock:
		if inPool {
			return errors.New("a key stocked twice")
		}
		r.pool[f] = Unsold
		r.order = append(r.order, f)
	case EventAssign:
		if state != Unsold {
			return fmt.Errorf("a key that is %q assigned", state)
		}
		if _, taken := r.seats[at]; taken || l.Seat < 1 {
			return errors.New("a seat assigned that holds a key")
		}
		if at.seat > 1 {
			if _, ok := r.seats[seatAt{l.Source, l.Ref, l.Seat - 1}]; !ok {
				return errors.New("a seat assigned before the seat before it")
			}
		}
		r.pool[f] = Sold
		r.seats[at] = f
		r.holder[f] = at
		r.held[f] = true
	case EventRevoke:
		if r.holder[f] != at || state != Sold {
			return errors.New("a key revoked that is not that seat's")
		}
		if _, ok := r.revoked[f]; ok {
			return errors.New("a key revoked twice")
		}
		r.revoked[f] = Revoked
	case EventRestore:
		if r.holder[f] != at || r.revoked[f] != Revoked {
			return errors.New("a key restored that was not revoked at that seat")
		}
		delete(r.revoked, f)
	case EventReplace:
		if r.holder[f] != at || state != Sold {
			return errors.New("a key replaced that is not that seat's")
		}
		if _, ok := r.revoked[f]; ok {
			return errors.New("a revoked key replaced")
		}
		r.revoked[f] = Replaced
		delete(r.seats, at)
		delete(r.holder, f)
	case EventRetire, EventBurn:
		if state != Unsold || !inPool {
			return fmt.Errorf("a key that is %q set aside", state)
		}
		if l.Event == EventRetire {
			r.pool[f] = Retired
		} else {
			r.pool[f] = Burned
			r.revoked[f] = BurnedKey
		}
	default:
		return errors.New("an event the dispenser does not know")
	}
	return nil
}

// compare finds the first place the store differs from the replay.
func (r *replay) compare(tx Tx) error {
	pool, err := tx.Pool()
	if err != nil {
		return err
	}
	if len(pool) != len(r.order) {
		return fmt.Errorf("%w: %d keys in the pool, the record stocked %d", ErrAudit, len(pool), len(r.order))
	}
	for i, k := range pool {
		if k.Fingerprint != r.order[i] {
			return fmt.Errorf("%w: key %d of the pool is not the key stocked %d", ErrAudit, i+1, i+1)
		}
		if k.Key.Fingerprint() != k.Fingerprint {
			return fmt.Errorf("%w: key %s does not have its own fingerprint", ErrAudit, k.ID)
		}
		if k.State != r.pool[k.Fingerprint] {
			return fmt.Errorf("%w: key %s is %s, the record says %s", ErrAudit, k.ID, k.State, r.pool[k.Fingerprint])
		}
	}
	seats, err := tx.AllSeats()
	if err != nil {
		return err
	}
	if len(seats) != len(r.seats) {
		return fmt.Errorf("%w: %d seats, the record has %d", ErrAudit, len(seats), len(r.seats))
	}
	for _, s := range seats {
		if f, ok := r.seats[seatAt{s.Source, s.Ref, s.Seat}]; !ok || f != s.Key {
			return fmt.Errorf("%w: %s %s seat %d holds a key the record did not give it", ErrAudit, s.Source, s.Ref, s.Seat)
		}
	}
	revoked, err := tx.Revoked()
	if err != nil {
		return err
	}
	if len(revoked) != len(r.revoked) {
		return fmt.Errorf("%w: %d keys revoked, the record revoked %d", ErrAudit, len(revoked), len(r.revoked))
	}
	for f, why := range revoked {
		if r.revoked[f] != why {
			return fmt.Errorf("%w: key %s is %s, the record says %q", ErrAudit, f, why, r.revoked[f])
		}
	}
	return nil
}
