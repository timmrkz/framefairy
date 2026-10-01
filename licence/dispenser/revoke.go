package dispenser

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"framefairy/licence"
)

// Events in the record, beyond stock and assign.
const (
	EventRevoke  = "revoke"  // a sold key went on the revocation list
	EventRestore = "restore" // a sold key came off it again
	EventReplace = "replace" // a sold key was revoked and its seat given the next key
	EventRetire  = "retire"  // an unsold key was set aside for good
	EventBurn    = "burn"    // an unsold key was revoked
)

// Target is what Revoke and Restore act on: every key of a sale, or one key.
// Exactly one of the two is given.
type Target struct {
	Source, Ref string
	Key         licence.Fingerprint
}

func (t Target) sale() bool { return t.Source != "" || t.Ref != "" }

// keys are the fingerprints a target stands for, and the seats they are in.
func (t Target) seats(tx Tx) ([]Seat, error) {
	switch {
	case t.sale() && t.Key != (licence.Fingerprint{}):
		return nil, fmt.Errorf("%w: a target is a sale or a key, not both", ErrInvalid)
	case t.sale():
		if err := checkSale(t.Source, t.Ref); err != nil {
			return nil, err
		}
		seats, err := tx.Seats(t.Source, t.Ref)
		if err != nil {
			return nil, err
		}
		if len(seats) == 0 {
			return nil, fmt.Errorf("%w: no sale %s %s", ErrNotFound, t.Source, t.Ref)
		}
		return seats, nil
	case t.Key != (licence.Fingerprint{}):
		s, err := tx.SeatOf(t.Key)
		if errors.Is(err, ErrNotFound) {
			return nil, notHeld(tx, t.Key)
		}
		if err != nil {
			return nil, err
		}
		return []Seat{s}, nil
	}
	return nil, fmt.Errorf("%w: a target needs a sale or a key", ErrInvalid)
}

// notHeld says why no seat holds f.
func notHeld(tx Tx, f licence.Fingerprint) error {
	k, err := tx.Key(f)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: no key %s", ErrNotFound, f)
	}
	if err != nil {
		return err
	}
	if k.State == Sold {
		return fmt.Errorf("%w: key %s was replaced and is no seat's any more", ErrInvalid, k.ID)
	}
	return fmt.Errorf("%w: key %s is %s, not sold", ErrInvalid, k.ID, k.State)
}

// Revoke puts every key of a sale, or one sold key, on the revocation
// list, after a chargeback or a refund. A key already on it stays as it
// is, so the same adjustment twice changes nothing. It says how many keys
// went on the list.
func (e *Engine) Revoke(ctx context.Context, t Target, why string) (int, error) {
	if err := checkWhy(why); err != nil {
		return 0, err
	}
	now := e.now()
	var n int
	err := e.store.Update(ctx, func(tx Tx) error {
		n = 0
		seats, err := t.seats(tx)
		if err != nil {
			return err
		}
		for _, s := range seats {
			if _, err := tx.Revocation(s.Key); err == nil {
				continue
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			if err := tx.Revoke(s.Key, Revoked); err != nil {
				return err
			}
			if err := write(tx, Line{At: now, Event: EventRevoke, Fingerprint: s.Key, Source: s.Source, Ref: s.Ref, Seat: s.Seat, Why: why}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// Restore takes the keys Revoke put on the list off it again, after a
// chargeback was reversed. A key replaced or burned stays on it: those
// were revoked because the key itself got out, which no bank decision
// undoes. It says how many keys came off the list.
func (e *Engine) Restore(ctx context.Context, t Target, why string) (int, error) {
	if err := checkWhy(why); err != nil {
		return 0, err
	}
	now := e.now()
	var n int
	err := e.store.Update(ctx, func(tx Tx) error {
		n = 0
		seats, err := t.seats(tx)
		if err != nil {
			return err
		}
		for _, s := range seats {
			r, err := tx.Revocation(s.Key)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if r != Revoked {
				return fmt.Errorf("%w: key %s is %s and stays revoked", ErrInvalid, s.Key, r)
			}
			if err := tx.Unrevoke(s.Key); err != nil {
				return err
			}
			if err := write(tx, Line{At: now, Event: EventRestore, Fingerprint: s.Key, Source: s.Source, Ref: s.Ref, Seat: s.Seat, Why: why}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// Replace revokes a sold key that was posted in public and gives its seat
// the next key in the pool, which it returns with the seat. A key whose
// sale was revoked is not replaced: the buyer has no claim to a new one.
func (e *Engine) Replace(ctx context.Context, f licence.Fingerprint, why string) (licence.Key, Seat, error) {
	if err := checkWhy(why); err != nil {
		return "", Seat{}, err
	}
	now := e.now()
	var key licence.Key
	var seat Seat
	err := e.store.Update(ctx, func(tx Tx) error {
		key, seat = "", Seat{}
		s, err := tx.SeatOf(f)
		if errors.Is(err, ErrNotFound) {
			return notHeld(tx, f)
		}
		if err != nil {
			return err
		}
		if r, err := tx.Revocation(f); err == nil {
			return fmt.Errorf("%w: key %s is %s, and a revoked key is not replaced", ErrInvalid, f, r)
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		next, err := tx.NextUnsold()
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("%w: replacing %s", ErrPoolEmpty, f)
		}
		if err != nil {
			return err
		}
		if err := tx.Revoke(f, Replaced); err != nil {
			return err
		}
		if err := write(tx, Line{At: now, Event: EventReplace, Fingerprint: f, Source: s.Source, Ref: s.Ref, Seat: s.Seat, Why: why}); err != nil {
			return err
		}
		if err := tx.SetState(next.Fingerprint, Sold); err != nil {
			return err
		}
		if err := tx.SetSeatKey(s.Source, s.Ref, s.Seat, next.Fingerprint); err != nil {
			return err
		}
		if err := write(tx, Line{At: now, Event: EventAssign, Fingerprint: next.Fingerprint, Source: s.Source, Ref: s.Ref, Seat: s.Seat}); err != nil {
			return err
		}
		key = next.Key
		seat = s
		seat.Key = next.Fingerprint
		return nil
	})
	if err != nil {
		return "", Seat{}, err
	}
	return key, seat, nil
}

// RevokeUnsold revokes every key still in the pool, after the database was
// stolen. Sold keys keep working. It says how many keys it burned.
func (e *Engine) RevokeUnsold(ctx context.Context, why string) (int, error) {
	return e.clearPool(ctx, why, Burned)
}

// Retire sets every key still in the pool aside for good without revoking
// it, after the database was restored from a backup: some of those keys
// were sold after the backup was taken, and the backup cannot say which.
// It says how many keys it retired.
func (e *Engine) Retire(ctx context.Context, why string) (int, error) {
	return e.clearPool(ctx, why, Retired)
}

func (e *Engine) clearPool(ctx context.Context, why string, to State) (int, error) {
	if err := checkWhy(why); err != nil {
		return 0, err
	}
	now := e.now()
	event := map[State]string{Burned: EventBurn, Retired: EventRetire}[to]
	var n int
	err := e.store.Update(ctx, func(tx Tx) error {
		n = 0
		pool, err := tx.Pool()
		if err != nil {
			return err
		}
		for _, k := range pool {
			if k.State != Unsold {
				continue
			}
			if err := tx.SetState(k.Fingerprint, to); err != nil {
				return err
			}
			if to == Burned {
				if err := tx.Revoke(k.Fingerprint, BurnedKey); err != nil {
					return err
				}
			}
			if err := write(tx, Line{At: now, Event: event, Fingerprint: k.Fingerprint, Why: why}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// Revocations is the revocation list for the update feed, sorted, so the
// same list is always written the same way.
func (e *Engine) Revocations(ctx context.Context) ([]licence.Fingerprint, error) {
	var out []licence.Fingerprint
	err := e.store.View(ctx, func(tx Tx) error {
		revoked, err := tx.Revoked()
		if err != nil {
			return err
		}
		out = sorted(revoked)
		return nil
	})
	return out, err
}

// Genuine is every pool key of a signer that was sold, for the genuine
// list after that signer's private key leaked. Keys revoked are on it too,
// so a chargeback reversed later needs no new list. The keys the signer
// issued for partners and by hand are on the signer's own list, and the
// two are published together.
func (e *Engine) Genuine(ctx context.Context, signer uint8) ([]licence.Fingerprint, error) {
	var out []licence.Fingerprint
	err := e.store.View(ctx, func(tx Tx) error {
		pool, err := tx.Pool()
		if err != nil {
			return err
		}
		set := map[licence.Fingerprint]bool{}
		for _, k := range pool {
			if k.Signer == signer && k.State == Sold {
				set[k.Fingerprint] = true
			}
		}
		out = sorted(set)
		return nil
	})
	return out, err
}

func sorted[V any](m map[licence.Fingerprint]V) []licence.Fingerprint {
	out := make([]licence.Fingerprint, 0, len(m))
	for f := range m {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b licence.Fingerprint) int { return strings.Compare(string(a[:]), string(b[:])) })
	return out
}

// checkWhy refuses a reason that is empty or would not read as one line.
// Every change after a sale says why it was made.
func checkWhy(why string) error {
	if why == "" || len(why) > MaxWhy || !utf8.ValidString(why) || strings.TrimSpace(why) != why {
		return fmt.Errorf("%w: a reason of 1 to %d bytes of plain text", ErrInvalid, MaxWhy)
	}
	for _, r := range why {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || r == ' ' || r == ' ' {
			return fmt.Errorf("%w: a reason cannot hold the character %U", ErrInvalid, r)
		}
	}
	return nil
}
