package dispenser

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

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
	pool    []PoolKey // in the order they came in
	byKey   map[licence.Fingerprint]int
	byID    map[licence.ID]bool
	seats   map[string][]Seat              // by source and ref
	held    map[licence.Fingerprint]bool   // held by a seat, now or before
	holder  map[licence.Fingerprint]string // the seat holding it now, seatKey and number
	revoked map[licence.Fingerprint]Revocation
	record  []Line
	mail    []Mail
	mailID  int64
	notes   map[string]string
}

func (db memoryDB) clone() memoryDB {
	c := memoryDB{
		pool:    slices.Clone(db.pool),
		byKey:   maps.Clone(db.byKey),
		byID:    maps.Clone(db.byID),
		seats:   make(map[string][]Seat, len(db.seats)),
		held:    maps.Clone(db.held),
		holder:  maps.Clone(db.holder),
		revoked: maps.Clone(db.revoked),
		record:  slices.Clone(db.record),
		mail:    slices.Clone(db.mail),
		mailID:  db.mailID,
		notes:   maps.Clone(db.notes),
	}
	for k, v := range db.seats {
		c.seats[k] = slices.Clone(v)
	}
	if c.byKey == nil {
		c.byKey = map[licence.Fingerprint]int{}
		c.byID = map[licence.ID]bool{}
		c.held = map[licence.Fingerprint]bool{}
		c.holder = map[licence.Fingerprint]string{}
		c.revoked = map[licence.Fingerprint]Revocation{}
		c.notes = map[string]string{}
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
	t.db.holder[s.Key] = holderOf(s.Source, s.Ref, s.Seat)
	t.db.seats[k] = append(t.db.seats[k], s)
	slices.SortFunc(t.db.seats[k], func(a, b Seat) int { return a.Seat - b.Seat })
	return nil
}

func holderOf(source, ref string, seat int) string {
	return fmt.Sprintf("%s\x00%d", seatKey(source, ref), seat)
}

func (t *memoryTx) SeatOf(f licence.Fingerprint) (Seat, error) {
	h, ok := t.db.holder[f]
	if !ok {
		return Seat{}, ErrNotFound
	}
	for _, seats := range t.db.seats {
		for _, s := range seats {
			if s.Key == f && holderOf(s.Source, s.Ref, s.Seat) == h {
				return s, nil
			}
		}
	}
	return Seat{}, ErrNotFound
}

func (t *memoryTx) SetSeatKey(source, ref string, seat int, f licence.Fingerprint) error {
	if err := t.write(); err != nil {
		return err
	}
	if t.db.held[f] {
		return ErrExists
	}
	seats := t.db.seats[seatKey(source, ref)]
	for i := range seats {
		if seats[i].Seat == seat {
			delete(t.db.holder, seats[i].Key)
			seats[i].Key = f
			t.db.held[f] = true
			t.db.holder[f] = holderOf(source, ref, seat)
			return nil
		}
	}
	return ErrNotFound
}

func (t *memoryTx) AllSeats() ([]Seat, error) {
	var out []Seat
	for _, seats := range t.db.seats {
		out = append(out, seats...)
	}
	slices.SortFunc(out, func(a, b Seat) int {
		if c := strings.Compare(a.Source+"\x00"+a.Ref, b.Source+"\x00"+b.Ref); c != 0 {
			return c
		}
		return a.Seat - b.Seat
	})
	return out, nil
}

func (t *memoryTx) Pool() ([]PoolKey, error) { return slices.Clone(t.db.pool), nil }

func (t *memoryTx) Revoke(f licence.Fingerprint, kind Revocation) error {
	if err := t.write(); err != nil {
		return err
	}
	if _, ok := t.db.revoked[f]; ok {
		return ErrExists
	}
	t.db.revoked[f] = kind
	return nil
}

func (t *memoryTx) Unrevoke(f licence.Fingerprint) error {
	if err := t.write(); err != nil {
		return err
	}
	if _, ok := t.db.revoked[f]; !ok {
		return ErrNotFound
	}
	delete(t.db.revoked, f)
	return nil
}

func (t *memoryTx) Revocation(f licence.Fingerprint) (Revocation, error) {
	r, ok := t.db.revoked[f]
	if !ok {
		return "", ErrNotFound
	}
	return r, nil
}

func (t *memoryTx) Revoked() (map[licence.Fingerprint]Revocation, error) {
	return maps.Clone(t.db.revoked), nil
}

func (t *memoryTx) AddMail(m Mail) (int64, error) {
	if err := t.write(); err != nil {
		return 0, err
	}
	t.db.mailID++
	m.ID = t.db.mailID
	t.db.mail = append(t.db.mail, m)
	return m.ID, nil
}

func (t *memoryTx) DueMail(now time.Time, limit int) ([]Mail, error) {
	var out []Mail
	for _, m := range t.db.mail {
		if !m.Due.After(now) && len(out) < limit {
			out = append(out, m)
		}
	}
	return out, nil
}

func (t *memoryTx) mailAt(id int64) int {
	for i, m := range t.db.mail {
		if m.ID == id {
			return i
		}
	}
	return -1
}

func (t *memoryTx) Mail(id int64) (Mail, error) {
	i := t.mailAt(id)
	if i < 0 {
		return Mail{}, ErrNotFound
	}
	return t.db.mail[i], nil
}

func (t *memoryTx) UpdateMail(m Mail) error {
	if err := t.write(); err != nil {
		return err
	}
	i := t.mailAt(m.ID)
	if i < 0 {
		return ErrNotFound
	}
	t.db.mail[i].Tries, t.db.mail[i].Due = m.Tries, m.Due
	return nil
}

func (t *memoryTx) RemoveMail(id int64) error {
	if err := t.write(); err != nil {
		return err
	}
	i := t.mailAt(id)
	if i < 0 {
		return ErrNotFound
	}
	t.db.mail = slices.Delete(t.db.mail, i, i+1)
	return nil
}

func (t *memoryTx) Note(name string) (string, error) {
	v, ok := t.db.notes[name]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (t *memoryTx) SetNote(name, value string) error {
	if err := t.write(); err != nil {
		return err
	}
	t.db.notes[name] = value
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
