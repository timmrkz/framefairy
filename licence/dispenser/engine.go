package dispenser

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"framefairy/licence"
)

// Limits on what the engine takes, so nothing it is handed can be larger
// than what it was built for.
const (
	MaxSeats = 100   // seats in one order
	MaxStock = 10000 // keys in one batch from the signer
	MaxRef   = 200   // bytes in a sale's reference
	MaxWhy   = 200   // bytes in a reason
)

// Errors the engine answers with, wrapped with what went wrong.
var (
	// ErrInvalid is a request that breaks a rule: an order with no seats,
	// a reference with a space in it, a key that does not check.
	ErrInvalid = errors.New("invalid")
	// ErrPoolEmpty is a sale that came when the pool had no key left for
	// it. Nothing is assigned, and the sale is tried again later.
	ErrPoolEmpty = errors.New("the pool has no key left")
	// ErrConflict is a sale that is already there with a different
	// number of seats.
	ErrConflict = errors.New("the sale is already there and differs")
)

// Config is how an engine is set up.
type Config struct {
	// Signers are the public keys whose batches the pool takes, by signer
	// number. A batch from any other is refused.
	Signers map[uint8]ed25519.PublicKey
	// Batch is how many keys the signer signs when the pool runs low.
	Batch int
	// Now is the clock. time.Now when nil.
	Now func() time.Time
	// Mailer sends the keys to buyers and warnings to us.
	Mailer Mailer
	// Orders asks the shop for a buyer's address. Without it, a letter
	// whose first try failed waits until it is set.
	Orders Orders
}

// Engine holds every rule about sales and keys.
type Engine struct {
	store   Store
	signers map[uint8]ed25519.PublicKey
	batch   int
	now     func() time.Time
	mailer  Mailer
	orders  Orders
}

// New makes an engine on store.
func New(store Store, c Config) (*Engine, error) {
	if store == nil {
		return nil, errors.New("an engine needs a store")
	}
	if len(c.Signers) == 0 {
		return nil, errors.New("an engine needs the public key of at least one signer")
	}
	for n, pub := range c.Signers {
		if len(pub) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("signer %d has a public key of %d bytes", n, len(pub))
		}
	}
	if c.Mailer == nil {
		return nil, errors.New("an engine needs a mailer")
	}
	if c.Batch < 1 || c.Batch > MaxStock {
		return nil, fmt.Errorf("a batch is 1 to %d keys, not %d", MaxStock, c.Batch)
	}
	e := &Engine{store: store, signers: maps.Clone(c.Signers), batch: c.Batch, now: c.Now, mailer: c.Mailer, orders: c.Orders}
	if e.now == nil {
		e.now = time.Now
	}
	return e, nil
}

// Order is a sale, from wherever it came.
type Order struct {
	Source string // "paddle" or "partner:<name>"
	Ref    string // its reference at the source, the same every time it is sent
	Seats  int
	Email  string // where the keys go. Used to send, never stored
	At     time.Time
}

// Assign hands one key from the pool to every seat of the order. An order
// that is already there gets the keys it was given before, so a webhook
// that comes twice changes nothing and sends nothing. Either every seat
// gets a key or none does.
//
// The keys of a Paddle sale are queued for mail in the same transaction,
// then sent at once to the address of the order. If that fails, the
// letter waits in the queue, and SendMail tries it again with the address
// Paddle has. A partner's buyer gets one try, when the partner gave an
// address: the partner has the keys in its answer. Neither failure fails
// the sale, which is done once its keys are recorded.
func (e *Engine) Assign(ctx context.Context, o Order) ([]licence.Key, error) {
	keys, fresh, mail, err := e.assign(ctx, o)
	if err != nil || !fresh {
		return keys, err
	}
	switch {
	case mail > 0:
		_ = e.deliver(ctx, mail, o.Email)
	case o.Email != "":
		_ = e.mailer.Keys(ctx, o.Email, Letter{Kind: MailKeys, Source: o.Source, Ref: o.Ref, Keys: keys})
	}
	_ = e.CheckPool(ctx)
	return keys, nil
}

// assign also says whether the keys are new, which decides whether they
// are mailed, and which letter was queued for them.
func (e *Engine) assign(ctx context.Context, o Order) (keys []licence.Key, fresh bool, mail int64, err error) {
	if err := checkOrder(o); err != nil {
		return nil, false, 0, err
	}
	now := e.now()
	err = e.store.Update(ctx, func(tx Tx) error {
		keys, fresh, mail = nil, false, 0
		seats, err := tx.Seats(o.Source, o.Ref)
		if err != nil {
			return err
		}
		if len(seats) > 0 {
			if len(seats) != o.Seats {
				return fmt.Errorf("%w: %s %s has %d seats, this order says %d", ErrConflict, o.Source, o.Ref, len(seats), o.Seats)
			}
			keys, err = keysOf(tx, seats)
			return err
		}
		fresh = true
		for seat := 1; seat <= o.Seats; seat++ {
			k, err := tx.NextUnsold()
			if errors.Is(err, ErrNotFound) {
				return fmt.Errorf("%w: %s %s needs %d", ErrPoolEmpty, o.Source, o.Ref, o.Seats)
			}
			if err != nil {
				return err
			}
			if err := tx.SetState(k.Fingerprint, Sold); err != nil {
				return err
			}
			if err := tx.AddSeat(Seat{Source: o.Source, Ref: o.Ref, Seat: seat, Key: k.Fingerprint, At: now}); err != nil {
				return err
			}
			if err := write(tx, Line{At: now, Event: EventAssign, Fingerprint: k.Fingerprint, Source: o.Source, Ref: o.Ref, Seat: seat}); err != nil {
				return err
			}
			keys = append(keys, k.Key)
		}
		mail, err = queueMail(tx, now, o.Source, o.Ref, MailKeys)
		return err
	})
	if err != nil {
		return nil, false, 0, err
	}
	return keys, fresh, mail, nil
}

// Keys are the keys a sale holds now, by seat. ErrNotFound when the sale
// has none yet, which the thank-you page shows as waiting.
func (e *Engine) Keys(ctx context.Context, source, ref string) ([]licence.Key, error) {
	if err := checkSale(source, ref); err != nil {
		return nil, err
	}
	var keys []licence.Key
	err := e.store.View(ctx, func(tx Tx) error {
		seats, err := tx.Seats(source, ref)
		if err != nil {
			return err
		}
		if len(seats) == 0 {
			return ErrNotFound
		}
		keys, err = keysOf(tx, seats)
		return err
	})
	return keys, err
}

func keysOf(tx Tx, seats []Seat) ([]licence.Key, error) {
	keys := make([]licence.Key, 0, len(seats))
	for i, s := range seats {
		if s.Seat != i+1 {
			return nil, fmt.Errorf("%s %s has seat %d where seat %d should be", s.Source, s.Ref, s.Seat, i+1)
		}
		k, err := tx.Key(s.Key)
		if err != nil {
			return nil, fmt.Errorf("the key of %s %s seat %d: %w", s.Source, s.Ref, s.Seat, err)
		}
		keys = append(keys, k.Key)
	}
	return keys, nil
}

// Stock takes a batch from the signer into the pool. Every key must check
// against a signer the engine trusts and carry no name, or the whole batch
// is refused. A key the pool has already is skipped, so a batch handed
// over twice, after a connection dropped, adds nothing the second time.
// It says how many keys were new.
func (e *Engine) Stock(ctx context.Context, batch []licence.Key) (int, error) {
	if len(batch) < 1 || len(batch) > MaxStock {
		return 0, fmt.Errorf("%w: a batch holds 1 to %d keys, not %d", ErrInvalid, MaxStock, len(batch))
	}
	trust := licence.Trust{Signers: e.signers}
	keys := make([]PoolKey, 0, len(batch))
	seen := make(map[licence.Fingerprint]bool, len(batch))
	for i, k := range batch {
		l, err := licence.Check(k, trust)
		if err != nil {
			return 0, fmt.Errorf("%w: key %d of the batch: %w", ErrInvalid, i+1, err)
		}
		if l.Name != "" {
			return 0, fmt.Errorf("%w: key %d of the batch has a name, and pool keys have none", ErrInvalid, i+1)
		}
		f := k.Fingerprint()
		if seen[f] {
			return 0, fmt.Errorf("%w: key %d of the batch is in it twice", ErrInvalid, i+1)
		}
		seen[f] = true
		keys = append(keys, PoolKey{Key: k, Fingerprint: f, ID: l.ID, Signer: l.Signer, State: Unsold})
	}
	now := e.now()
	var added int
	err := e.store.Update(ctx, func(tx Tx) error {
		added = 0
		for _, k := range keys {
			had, err := tx.Key(k.Fingerprint)
			if err == nil && had.Key == k.Key {
				continue
			}
			if err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
			if err := tx.AddKey(k); err != nil {
				if errors.Is(err, ErrExists) {
					return fmt.Errorf("%w: key %s has the ID of a key the pool has", ErrInvalid, k.ID)
				}
				return err
			}
			if err := write(tx, Line{At: now, Event: EventStock, Fingerprint: k.Fingerprint}); err != nil {
				return err
			}
			added++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// PoolLevel is how many keys are left to sell, and how many the signer
// signs when it refills.
func (e *Engine) PoolLevel(ctx context.Context) (left, batch int, err error) {
	err = e.store.View(ctx, func(tx Tx) error {
		left, err = tx.Count(Unsold)
		return err
	})
	return left, e.batch, err
}

// checkOrder refuses an order the engine was not built for.
func checkOrder(o Order) error {
	if err := checkSale(o.Source, o.Ref); err != nil {
		return err
	}
	if o.Seats < 1 || o.Seats > MaxSeats {
		return fmt.Errorf("%w: an order has 1 to %d seats, not %d", ErrInvalid, MaxSeats, o.Seats)
	}
	if o.At.IsZero() {
		return fmt.Errorf("%w: an order needs the time it was made", ErrInvalid)
	}
	if o.Email != "" {
		if err := checkEmail(o.Email); err != nil {
			return err
		}
	}
	return nil
}

// checkSale refuses a source or a reference that is not plain: a source is
// paddle or a partner's name, a reference is letters, digits and _ . : -.
func checkSale(source, ref string) error {
	switch {
	case source == "paddle":
	case strings.HasPrefix(source, "partner:") && partnerName(source[len("partner:"):]):
	default:
		return fmt.Errorf("%w: source %q", ErrInvalid, source)
	}
	if !plain(ref, MaxRef, "_.:-") {
		return fmt.Errorf("%w: reference %q", ErrInvalid, ref)
	}
	return nil
}

// partnerName is lowercase letters, digits and dashes, so one partner is
// never two by the way its name was typed.
func partnerName(s string) bool {
	return plain(s, 32, "-") && strings.ToLower(s) == s
}

func plain(s string, max int, extra string) bool {
	if s == "" || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune(extra, rune(c)) {
			return false
		}
	}
	return true
}

// checkEmail refuses what cannot be an address to send to. Whether it is
// one is the mail service's to find out.
func checkEmail(s string) error {
	at := strings.LastIndexByte(s, '@')
	if len(s) > 254 || at < 1 || at == len(s)-1 || strings.ContainsAny(s, " \t\r\n<>,;\"\\") {
		return fmt.Errorf("%w: email address", ErrInvalid)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return fmt.Errorf("%w: email address", ErrInvalid)
		}
	}
	return nil
}
