package dispenser

import (
	"context"
	"errors"
	"time"

	"framefairy/licence"
)

// State is where a key in the pool stands.
type State string

const (
	// Unsold is waiting to be handed to a sale.
	Unsold State = "unsold"
	// Sold belongs to a seat of a sale.
	Sold State = "sold"
	// Retired was set aside after a restore. It is never handed out and
	// not revoked, because some buyer may hold it.
	Retired State = "retired"
	// Burned was revoked before anyone bought it, after the database was
	// stolen or its signer leaked.
	Burned State = "burned"
)

// PoolKey is one key the signer handed over.
type PoolKey struct {
	Key         licence.Key
	Fingerprint licence.Fingerprint
	ID          licence.ID
	Signer      uint8
	State       State
}

// Revocation is why a key is on the revocation list.
type Revocation string

const (
	// Revoked is a sold key taken back, after a chargeback or a refund.
	// Restore takes it off the list again.
	Revoked Revocation = "revoked"
	// Replaced is a sold key that was posted in public and replaced by
	// another. It stays on the list.
	Replaced Revocation = "replaced"
	// BurnedKey is an unsold key revoked with the rest of the pool. It
	// stays on the list.
	BurnedKey Revocation = "burned"
)

// Seat is one seat of one sale and the key it holds now.
type Seat struct {
	Source string // "paddle" or "partner:<name>"
	Ref    string // the sale's reference at its source
	Seat   int    // from 1
	Key    licence.Fingerprint
	At     time.Time
}

// Errors the store answers with. A store returns these, wrapped or not,
// and the engine tells them apart with errors.Is.
var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already there")
)

// Store keeps the pool, the sales and the record. Every change the engine
// makes goes through Update, and either all of it lands or none of it does.
type Store interface {
	// Update runs fn in one transaction that is as if no other ran at the
	// same time, and commits it when fn returns nil. A store may run fn
	// more than once, when a transaction collides with another, so fn
	// changes nothing outside the transaction.
	Update(ctx context.Context, fn func(Tx) error) error
	// View runs fn in a transaction that only reads.
	View(ctx context.Context, fn func(Tx) error) error
}

// Tx is what a transaction can do. It is plain storage: every rule is the
// engine's.
type Tx interface {
	// AddKey puts a key at the back of the pool. ErrExists when its
	// fingerprint or its ID is there already.
	AddKey(k PoolKey) error
	// Key finds a key by its fingerprint. ErrNotFound when there is none.
	Key(f licence.Fingerprint) (PoolKey, error)
	// NextUnsold is the unsold key that came into the pool first.
	// ErrNotFound when no key is unsold.
	NextUnsold() (PoolKey, error)
	// SetState moves a key in the pool. ErrNotFound when there is none.
	SetState(f licence.Fingerprint, s State) error
	// Count is how many keys in the pool are in state s.
	Count(s State) (int, error)

	// Seats are the seats of one sale, by seat number.
	Seats(source, ref string) ([]Seat, error)
	// AddSeat records a seat. ErrExists when that seat of that sale is
	// there already, or when another seat holds the same key.
	AddSeat(s Seat) error

	// SeatOf is the seat that holds key f now. ErrNotFound when none does.
	SeatOf(f licence.Fingerprint) (Seat, error)
	// SetSeatKey gives a seat another key. ErrNotFound when there is no
	// such seat, ErrExists when the key is or was held by any seat.
	SetSeatKey(source, ref string, seat int, f licence.Fingerprint) error
	// AllSeats are every seat of every sale.
	AllSeats() ([]Seat, error)
	// Pool is every key in the pool, in the order they came in.
	Pool() ([]PoolKey, error)

	// Revoke puts a key on the revocation list for a reason of kind.
	// ErrExists when it is on it already.
	Revoke(f licence.Fingerprint, kind Revocation) error
	// Unrevoke takes a key off the list. ErrNotFound when it is not on it.
	Unrevoke(f licence.Fingerprint) error
	// Revocation says why a key is on the list. ErrNotFound when it is not.
	Revocation(f licence.Fingerprint) (Revocation, error)
	// Revoked is the whole list.
	Revoked() (map[licence.Fingerprint]Revocation, error)

	// Head is the newest line of the record. ErrNotFound when it is empty.
	Head() (Line, error)
	// Append adds a line to the record. ErrExists when its number is taken.
	Append(l Line) error
	// Lines are up to limit lines of the record after line number after,
	// oldest first.
	Lines(after int64, limit int) ([]Line, error)
}
