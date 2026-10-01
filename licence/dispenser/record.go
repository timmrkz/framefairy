package dispenser

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"framefairy/licence"
)

// Events in the record.
const (
	EventStock  = "stock"  // a key came into the pool
	EventAssign = "assign" // a key went to a seat
)

// Line is one event in the record. Lines are numbered from 1 without a
// gap, and each carries the hash of the line before it, so a line changed,
// removed or slipped in breaks the chain from there on.
type Line struct {
	Seq         int64
	At          time.Time // to the second, UTC
	Event       string
	Fingerprint licence.Fingerprint
	Source      string
	Ref         string
	Seat        int
	Why         string
	Prev        [32]byte // Hash of the line before, zero for the first
	Hash        [32]byte
}

// digest is the hash of a line: every field but Hash, each written with
// its length, so no two lines can be read the same.
func (l Line) digest() [32]byte {
	h := sha256.New()
	h.Write([]byte("framefairy record v1\n"))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(l.Seq))
	h.Write(n[:])
	binary.BigEndian.PutUint64(n[:], uint64(l.At.Unix()))
	h.Write(n[:])
	for _, s := range []string{l.Event, l.Source, l.Ref, l.Why} {
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	binary.BigEndian.PutUint64(n[:], uint64(l.Seat))
	h.Write(n[:])
	h.Write(l.Fingerprint[:])
	h.Write(l.Prev[:])
	var out [32]byte
	h.Sum(out[:0])
	return out
}

// write adds a line after the head of the record.
func write(tx Tx, l Line) error {
	head, err := tx.Head()
	switch {
	case errors.Is(err, ErrNotFound):
		l.Seq, l.Prev = 1, [32]byte{}
	case err != nil:
		return err
	default:
		l.Seq, l.Prev = head.Seq+1, head.Hash
	}
	l.At = l.At.UTC().Truncate(time.Second)
	l.Hash = l.digest()
	return tx.Append(l)
}

// Anchor is a link of the record published where the dispenser cannot
// change it: the update feed, signed elsewhere and kept in the releases.
type Anchor struct {
	Seq  int64
	Hash [32]byte
}

// ErrBrokenRecord is a record whose chain does not hold.
var ErrBrokenRecord = errors.New("the record's chain is broken")

// VerifyRecord walks the whole record and checks every link, and that each
// anchor given is a link of it. It returns the head, the link to publish
// next.
func (e *Engine) VerifyRecord(ctx context.Context, anchors ...Anchor) (Anchor, error) {
	var head Anchor
	err := e.store.View(ctx, func(tx Tx) error {
		head = Anchor{}
		want := map[int64][32]byte{}
		for _, a := range anchors {
			want[a.Seq] = a.Hash
		}
		var prev [32]byte
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
				if l.Seq != seq+1 {
					return fmt.Errorf("%w: line %d follows line %d", ErrBrokenRecord, l.Seq, seq)
				}
				if l.Prev != prev || l.digest() != l.Hash {
					return fmt.Errorf("%w at line %d", ErrBrokenRecord, l.Seq)
				}
				if h, ok := want[l.Seq]; ok && h != l.Hash {
					return fmt.Errorf("%w: line %d is not the line that was published", ErrBrokenRecord, l.Seq)
				}
				delete(want, l.Seq)
				seq, prev = l.Seq, l.Hash
			}
		}
		for s := range want {
			return fmt.Errorf("%w: line %d was published and is not there", ErrBrokenRecord, s)
		}
		head = Anchor{Seq: seq, Hash: prev}
		return nil
	})
	return head, err
}
