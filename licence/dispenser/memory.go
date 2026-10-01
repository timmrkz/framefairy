package dispenser

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"framefairy/licence"
)

// Memory is a Store in memory, for tests and for trying the dispenser out.
// Transactions run one at a time, and each works on a copy that replaces
// the whole only when it commits, so a transaction that fails leaves
// nothing behind.
type Memory struct {
	mu sync.Mutex
	db memoryDB

	// Twice makes every Update run its function once more before the run
	// that counts, and throw the first away, the way a database retries a
	// transaction that collided with another. It proves the engine keeps
	// nothing from a run that did not commit.
	Twice bool
}

type memoryDB struct {
	pool   []PoolKey // in the order they came in
	byKey  map[licence.Fingerprint]int
	byID   map[licence.ID]bool
	seats  map[string][]Seat // by source and ref
	held   map[licence.Fingerprint]bool
	record []Line
}

func (db memoryDB) clone() memoryDB {
	c := memoryDB{
		pool:   slices.Clone(db.pool),
		byKey:  maps.Clone(db.byKey),
		byID:   maps.Clone(db.byID),
		seats:  make(map[string][]Seat, len(db.seats)),
		held:   maps.Clone(db.held),
		record: slices.Clone(db.record),
	}
	for k, v := range db.seats {
		c.seats[k] = slices.Clone(v)
	}
	if c.byKey == nil {
		c.byKey = map[licence.Fingerprint]int{}
		c.byID = map[licence.ID]bool{}
		c.held = map[licence.Fingerprint]bool{}
	}
	return c
}

// Update runs fn on a copy and keeps the copy when fn returns nil.
func (m *Memory) Update(ctx context.Context, fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Twice {
		throwaway := &memoryTx{db: m.db.clone()}
		_ = fn(throwaway)
	}
	tx := &memoryTx{db: m.db.clone()}
	if err := fn(tx); err != nil {
		return err
	}
	m.db = tx.db
	return nil
}

// View runs fn on a copy that is thrown away.
func (m *Memory) View(ctx context.Context, fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(&memoryTx{db: m.db.clone(), readOnly: true})
}

type memoryTx struct {
	db       memoryDB
	readOnly bool
}

func (t *memoryTx) write() error {
	if t.readOnly {
		return fmt.Errorf("a change inside View")
	}
	return nil
}

func seatKey(source, ref string) string { return source + "\x00" + ref }

func (t *memoryTx) AddKey(k PoolKey) error {
	if err := t.write(); err != nil {
		return err
	}
	if _, ok := t.db.byKey[k.Fingerprint]; ok || t.db.byID[k.ID] {
		return ErrExists
	}
	t.db.byKey[k.Fingerprint] = len(t.db.pool)
	t.db.byID[k.ID] = true
	t.db.pool = append(t.db.pool, k)
	return nil
}

func (t *memoryTx) Key(f licence.Fingerprint) (PoolKey, error) {
	i, ok := t.db.byKey[f]
	if !ok {
		return PoolKey{}, ErrNotFound
	}
	return t.db.pool[i], nil
}

func (t *memoryTx) NextUnsold() (PoolKey, error) {
	for _, k := range t.db.pool {
		if k.State == Unsold {
			return k, nil
		}
	}
	return PoolKey{}, ErrNotFound
}

func (t *memoryTx) SetState(f licence.Fingerprint, s State) error {
	if err := t.write(); err != nil {
		return err
	}
	i, ok := t.db.byKey[f]
	if !ok {
		return ErrNotFound
	}
	t.db.pool[i].State = s
	return nil
}

func (t *memoryTx) Count(s State) (int, error) {
	n := 0
	for _, k := range t.db.pool {
		if k.State == s {
			n++
		}
	}
	return n, nil
}

func (t *memoryTx) Seats(source, ref string) ([]Seat, error) {
	return slices.Clone(t.db.seats[seatKey(source, ref)]), nil
}

func (t *memoryTx) AddSeat(s Seat) error {
	if err := t.write(); err != nil {
		return err
	}
	k := seatKey(s.Source, s.Ref)
	for _, o := range t.db.seats[k] {
		if o.Seat == s.Seat {
			return ErrExists
		}
	}
	if t.db.held[s.Key] {
		return ErrExists
	}
	t.db.held[s.Key] = true
	t.db.seats[k] = append(t.db.seats[k], s)
	slices.SortFunc(t.db.seats[k], func(a, b Seat) int { return a.Seat - b.Seat })
	return nil
}

func (t *memoryTx) Head() (Line, error) {
	if len(t.db.record) == 0 {
		return Line{}, ErrNotFound
	}
	return t.db.record[len(t.db.record)-1], nil
}

func (t *memoryTx) Append(l Line) error {
	if err := t.write(); err != nil {
		return err
	}
	if l.Seq != int64(len(t.db.record))+1 {
		return ErrExists
	}
	t.db.record = append(t.db.record, l)
	return nil
}

func (t *memoryTx) Lines(after int64, limit int) ([]Line, error) {
	if after < 0 {
		after = 0
	}
	if after >= int64(len(t.db.record)) {
		return nil, nil
	}
	end := min(int(after)+limit, len(t.db.record))
	return slices.Clone(t.db.record[after:end]), nil
}
