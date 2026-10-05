package dispenser

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/licence"
)

// The database fails at every write of every use case, one write at a
// time, once before it saves and once after it saved but before the
// engine heard so. Then the same call comes again, the way Paddle sends a
// webhook again, the cron runs again, or we type the command again, and
// it has to end where an undisturbed run ends: as many keys with each
// sale, the same keys revoked, the same pool, every letter, at most twice,
// and an audit that holds.
//
// The simulation reaches these by chance, in whichever of its hundred
// random runs happens to lose the database at the right moment. Here every
// moment is tried, every run.

var errBroken = errors.New("the database went away")

// breaking is a store that fails one write, counted from 1.
type breaking struct {
	*Memory
	mu    sync.Mutex
	n     int
	at    int  // the write that fails, 0 for none
	after bool // after it saved, rather than before
	// The step inside a transaction that fails, counted over the whole
	// call, reads and writes alike, 0 for none.
	steps, stepAt int
	// down fails every step, the database gone for as long as it lasts.
	down bool
}

func (b *breaking) Update(ctx context.Context, fn func(Tx) error) error {
	b.mu.Lock()
	b.n++
	fails := b.n == b.at
	b.mu.Unlock()
	if fails && !b.after {
		return errBroken
	}
	err := b.Memory.Update(ctx, func(tx Tx) error { return fn(failingTx{tx, b}) })
	if err == nil && fails {
		return errBroken
	}
	return err
}

// failingTx is a transaction whose steps can fail one at a time, the way a
// statement of a real database can, and the transaction then saves
// nothing.
type failingTx struct {
	Tx
	b *breaking
}

func (t failingTx) AddKey(k PoolKey) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.AddKey(k)
}

func (t failingTx) Key(f licence.Fingerprint) (PoolKey, error) {
	if err := t.b.step(); err != nil {
		var zero PoolKey
		return zero, err
	}
	return t.Tx.Key(f)
}

func (t failingTx) NextUnsold() (PoolKey, error) {
	if err := t.b.step(); err != nil {
		var zero PoolKey
		return zero, err
	}
	return t.Tx.NextUnsold()
}

func (t failingTx) SetState(f licence.Fingerprint, s State) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.SetState(f, s)
}

func (t failingTx) Count(s State) (int, error) {
	if err := t.b.step(); err != nil {
		var zero int
		return zero, err
	}
	return t.Tx.Count(s)
}

func (t failingTx) Seats(source, ref string) ([]Seat, error) {
	if err := t.b.step(); err != nil {
		var zero []Seat
		return zero, err
	}
	return t.Tx.Seats(source, ref)
}

func (t failingTx) AddSeat(s Seat) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.AddSeat(s)
}

func (t failingTx) SeatOf(f licence.Fingerprint) (Seat, error) {
	if err := t.b.step(); err != nil {
		var zero Seat
		return zero, err
	}
	return t.Tx.SeatOf(f)
}

func (t failingTx) SetSeatKey(source, ref string, seat int, f licence.Fingerprint) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.SetSeatKey(source, ref, seat, f)
}

func (t failingTx) AllSeats() ([]Seat, error) {
	if err := t.b.step(); err != nil {
		var zero []Seat
		return zero, err
	}
	return t.Tx.AllSeats()
}

func (t failingTx) Pool() ([]PoolKey, error) {
	if err := t.b.step(); err != nil {
		var zero []PoolKey
		return zero, err
	}
	return t.Tx.Pool()
}

func (t failingTx) Revoke(f licence.Fingerprint, kind Revocation) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.Revoke(f, kind)
}

func (t failingTx) Unrevoke(f licence.Fingerprint) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.Unrevoke(f)
}

func (t failingTx) Revocation(f licence.Fingerprint) (Revocation, error) {
	if err := t.b.step(); err != nil {
		var zero Revocation
		return zero, err
	}
	return t.Tx.Revocation(f)
}

func (t failingTx) Revoked() (map[licence.Fingerprint]Revocation, error) {
	if err := t.b.step(); err != nil {
		var zero map[licence.Fingerprint]Revocation
		return zero, err
	}
	return t.Tx.Revoked()
}

func (t failingTx) AddMail(m Mail) (int64, error) {
	if err := t.b.step(); err != nil {
		var zero int64
		return zero, err
	}
	return t.Tx.AddMail(m)
}

func (t failingTx) DueMail(now time.Time, limit int) ([]Mail, error) {
	if err := t.b.step(); err != nil {
		var zero []Mail
		return zero, err
	}
	return t.Tx.DueMail(now, limit)
}

func (t failingTx) Mail(id int64) (Mail, error) {
	if err := t.b.step(); err != nil {
		var zero Mail
		return zero, err
	}
	return t.Tx.Mail(id)
}

func (t failingTx) UpdateMail(m Mail) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.UpdateMail(m)
}

func (t failingTx) RemoveMail(id int64) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.RemoveMail(id)
}

func (t failingTx) Note(name string) (string, error) {
	if err := t.b.step(); err != nil {
		var zero string
		return zero, err
	}
	return t.Tx.Note(name)
}

func (t failingTx) SetNote(name, value string) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.SetNote(name, value)
}

func (t failingTx) Head() (Line, error) {
	if err := t.b.step(); err != nil {
		var zero Line
		return zero, err
	}
	return t.Tx.Head()
}

func (t failingTx) Append(l Line) error {
	if err := t.b.step(); err != nil {
		return err
	}
	return t.Tx.Append(l)
}

func (t failingTx) Lines(after int64, limit int) ([]Line, error) {
	if err := t.b.step(); err != nil {
		var zero []Line
		return zero, err
	}
	return t.Tx.Lines(after, limit)
}

// step counts a step of a transaction and fails the one asked for.
func (b *breaking) step() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.steps++
	if b.down || b.steps == b.stepAt {
		return errBroken
	}
	return nil
}

func (b *breaking) View(ctx context.Context, fn func(Tx) error) error {
	return b.Memory.View(ctx, func(tx Tx) error { return fn(failingTx{tx, b}) })
}

// arm makes the store fail its at'th write from now on, and disarm stops it.
func (b *breaking) arm(at int, after bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n, b.at, b.after, b.steps, b.stepAt = 0, at, after, 0, 0
}

// armStep makes the store fail the at'th step of a transaction from now on.
func (b *breaking) armStep(at int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n, b.at, b.steps, b.stepAt = 0, 0, 0, at
}

func (b *breaking) stepsTaken() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.steps
}

func (b *breaking) writes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.n
}

// failureCase is a use case: what is there before, and the call that the
// database fails under.
type failureCase struct {
	name string
	// set up returns the keys put in the pool, in order, so two runs can
	// say which key went where although their keys differ.
	setUp func(f *fixture) []licence.Key
	call  func(f *fixture, keys []licence.Key) error
}

var failureCases = []failureCase{
	{"use case 1, bought through Paddle",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.shop.put(Sale{Ref: "txn_a", Seats: 2, Email: "anna@example.com", At: now}, now)
			return keys
		},
		func(f *fixture, _ []licence.Key) error { return f.engine.Settle(f.ctx, "txn_a") }},
	{"use case 4, sold by a partner",
		func(f *fixture) []licence.Key { return f.stock(6) },
		func(f *fixture, _ []licence.Key) error {
			_, err := f.engine.Assign(f.ctx, Order{Source: "partner:acme", Ref: "po_1", Seats: 2, At: now})
			return err
		}},
	{"use case 8, chargeback",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			sale := Sale{Ref: "txn_a", Seats: 2, Email: "anna@example.com", At: now}
			f.shop.put(sale, now)
			mustSettle(f, "txn_a")
			sale.TakenBack = true
			f.shop.put(sale, now)
			return keys
		},
		func(f *fixture, _ []licence.Key) error { return f.engine.Settle(f.ctx, "txn_a") }},
	{"use case 9, chargeback reversed",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			sale := Sale{Ref: "txn_a", Seats: 2, Email: "anna@example.com", At: now}
			f.shop.put(sale, now)
			mustSettle(f, "txn_a")
			sale.TakenBack = true
			f.shop.put(sale, now)
			mustSettle(f, "txn_a")
			sale.TakenBack = false
			f.shop.put(sale, now)
			return keys
		},
		func(f *fixture, _ []licence.Key) error { return f.engine.Settle(f.ctx, "txn_a") }},
	{"use case 11, key posted in public",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.shop.sold("txn_a", "anna@example.com")
			mustSettle(f, "txn_a")
			return keys
		},
		func(f *fixture, keys []licence.Key) error {
			_, _, err := f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted")
			return err
		}},
	{"use case 12, a batch from the signer",
		func(f *fixture) []licence.Key { return f.stock(2) },
		func(f *fixture, _ []licence.Key) error {
			// The signer asks for the pool's generation first, and that
			// can fail too.
			level, err := f.engine.PoolLevel(f.ctx)
			if err != nil {
				return err
			}
			_, err = f.engine.Stock(f.ctx, level.Generation, batchFor(f))
			return err
		}},
	{"use case 14, database stolen",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.shop.sold("txn_a", "anna@example.com")
			mustSettle(f, "txn_a")
			return keys
		},
		func(f *fixture, _ []licence.Key) error {
			_, err := f.engine.RevokeUnsold(f.ctx, "stolen")
			return err
		}},
	{"use case 15, restored from a backup",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.shop.sold("txn_a", "anna@example.com")
			mustSettle(f, "txn_a")
			return keys
		},
		func(f *fixture, _ []licence.Key) error {
			_, err := f.engine.Retire(f.ctx, "restored")
			return err
		}},
	{"use case 19, the daily catch-up",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.shop.sold("txn_a", "anna@example.com")
			f.shop.sold("txn_b", "ben@example.com")
			return keys
		},
		func(f *fixture, _ []licence.Key) error {
			_, err := f.engine.Reconcile(f.ctx, now.Add(-time.Hour))
			return err
		}},
	{"use case 21, letters sent once the mail is back",
		func(f *fixture) []licence.Key {
			keys := f.stock(6)
			f.mail.failing(func(string, Letter) bool { return true })
			f.shop.sold("txn_a", "anna@example.com")
			mustSettle(f, "txn_a")
			f.mail.failing(nil)
			return keys
		},
		func(f *fixture, _ []licence.Key) error {
			// The letter is due again an hour on.
			f.clock.add(time.Hour)
			_, _, err := f.engine.SendMail(f.ctx)
			return err
		}},
}

// batches are the signer's batches, one per fixture, so the batch handed
// over again after a failure is the same batch, as the signer's would be.
var batches sync.Map

func batchFor(f *fixture) []licence.Key {
	if b, ok := batches.Load(f); ok {
		return b.([]licence.Key)
	}
	b := sign(f.t, 4)
	batches.Store(f, b)
	return b
}

func mustSettle(f *fixture, ref string) {
	f.t.Helper()
	if err := f.engine.Settle(f.ctx, ref); err != nil {
		f.t.Fatal(err)
	}
}

func TestEveryWriteFailing(t *testing.T) {
	for _, c := range failureCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// Undisturbed, which also counts the writes the call makes.
			mem := &Memory{}
			store := &breaking{Memory: mem}
			f := newFixtureOn(t, mem, store)
			keys := c.setUp(f)
			store.arm(0, false)
			if err := c.call(f, keys); err != nil {
				t.Fatalf("undisturbed: %v", err)
			}
			writes, steps := store.writes(), store.stepsTaken()
			want := outcomeOf(f, keys)
			if writes == 0 {
				t.Fatal("the call wrote nothing, so nothing could fail")
			}
			// The call once with the store broken as arm says, then again
			// with it working.
			again := func(t *testing.T, arm func(*breaking)) {
				mem := &Memory{}
				store := &breaking{Memory: mem}
				f := newFixtureOn(t, mem, store)
				keys := c.setUp(f)
				arm(store)
				// A write made after the sale is saved, like the one that
				// says a letter went out, may fail without the caller
				// hearing: the mail run catches it up.
				if err := c.call(f, keys); err != nil && !errors.Is(err, errBroken) {
					t.Fatalf("the failure came back as %v", err)
				}
				store.arm(0, false)
				// Called again, it may say it has nothing left to do, the
				// way a key replaced already is no seat's any more. What
				// counts is where it ends.
				if err := c.call(f, keys); err != nil {
					t.Logf("called again: %v", err)
				}
				if got := outcomeOf(f, keys); got != want {
					t.Errorf("called again, it ended\n%s\nwhere undisturbed it ended\n%s", got, want)
				}
			}
			for at := 1; at <= writes; at++ {
				for _, after := range []bool{false, true} {
					when := "before"
					if after {
						when = "after"
					}
					t.Run(fmt.Sprintf("write %d of %d fails %s it saved", at, writes, when), func(t *testing.T) {
						again(t, func(b *breaking) { b.arm(at, after) })
					})
				}
			}
			// A step inside a transaction, a read or a write, fails, and
			// the transaction saves none of what it did.
			for at := 1; at <= steps; at++ {
				t.Run(fmt.Sprintf("step %d of %d fails", at, steps), func(t *testing.T) {
					again(t, func(b *breaking) { b.armStep(at) })
				})
			}
		})
	}
}

// outcomeOf is where a run ended, in words that two runs can compare
// although their keys differ: a sale's keys by the sale and their seat, a
// key the pool still holds as pool. Which pool key a sale got does not
// matter, that each sale holds its own does. The letters are sent first,
// the way the mail cron would, an hour on.
func outcomeOf(f *fixture, _ []licence.Key) string {
	f.t.Helper()
	f.clock.add(time.Hour)
	if _, _, err := f.engine.SendMail(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	f.audit()
	names := map[licence.Fingerprint]string{}
	var b strings.Builder
	for _, sale := range [][2]string{{"paddle", "txn_a"}, {"paddle", "txn_b"}, {"partner:acme", "po_1"}} {
		keys, err := f.engine.Keys(f.ctx, sale[0], sale[1])
		if err != nil && !errors.Is(err, ErrNotFound) {
			f.t.Fatal(err)
		}
		for i, k := range keys {
			names[k.Fingerprint()] = fmt.Sprintf("%s#%d", sale[1], i+1)
		}
		fmt.Fprintf(&b, "%s: %d keys\n", sale[1], len(keys))
	}
	name := func(fp licence.Fingerprint) string {
		if n, ok := names[fp]; ok {
			return n
		}
		return "another"
	}
	var revoked []string
	for _, fp := range f.revoked() {
		revoked = append(revoked, name(fp))
	}
	slices.Sort(revoked)
	fmt.Fprintf(&b, "revoked: %s\npool: %d left\n", strings.Join(revoked, ","), f.left())
	var letters []string
	for _, l := range f.mail.sent() {
		var keys []string
		for _, k := range l.letter.Keys {
			keys = append(keys, name(k.Fingerprint()))
		}
		slices.Sort(keys)
		letters = append(letters, fmt.Sprintf("%s %v %s", l.to, l.letter.Kind, strings.Join(keys, ",")))
	}
	// A letter can come twice, never not at all: when the database fails
	// just after the mail service took it, saving that it went is lost,
	// and the mail run sends it again. Sending and saving cannot be one
	// step, and the other order loses the letter. See docs/LICENCE.md.
	slices.Sort(letters)
	letters = slices.Compact(letters)
	fmt.Fprintf(&b, "letters: %s", strings.Join(letters, " | "))
	return b.String()
}

// With the database gone, every endpoint says so with an error its caller
// tries again on, never a success it did not have: Paddle sends its
// webhook again, the signer hands its batch over again, the cron runs
// again. And when the database is back, nothing of the time without it is
// left in the store.
func TestEveryEndpointWithTheDatabaseDown(t *testing.T) {
	mem := &Memory{}
	store := &breaking{Memory: mem}
	w := newWebOn(t, newFixtureOn(t, mem, store))
	w.stock(6)
	w.shop.sold("txn_a", "anna@example.com")
	keys := w.assign("txn_a", 1)
	w.shop.sold("txn_b", "ben@example.com")
	fp := keys[0].Fingerprint().String()
	store.mu.Lock()
	store.down = true
	store.mu.Unlock()

	requests := []struct {
		method, path, token string
		body                any
	}{
		{"GET", "/v1/thanks/txn_a", "", nil},
		{"POST", "/v1/lost", "", map[string]string{"email": "anna@example.com"}},
		{"POST", "/v1/orders", bundleToken, map[string]any{"ref": "1001", "seats": 1, "email": "max@example.com"}},
		{"GET", "/v1/orders/1001", bundleToken, nil},
		{"POST", "/v1/orders/1001/revoke", bundleToken, map[string]string{"why": "refund"}},
		{"GET", "/v1/pool", signerToken, nil},
		{"GET", "/v1/feed", releaseToken, nil},
		{"POST", "/v1/cron/mail", cronToken, nil},
		{"POST", "/v1/cron/daily", cronToken, nil},
		{"POST", "/v1/admin/replace", adminToken, map[string]string{"fingerprint": fp, "why": "posted"}},
		{"POST", "/v1/admin/revoke", adminToken, map[string]string{"source": "paddle", "ref": "txn_a", "why": "x"}},
		{"POST", "/v1/admin/restore", adminToken, map[string]string{"source": "paddle", "ref": "txn_a", "why": "x"}},
		{"POST", "/v1/admin/settle", adminToken, map[string]string{"ref": "txn_b"}},
		{"POST", "/v1/admin/reconcile", adminToken, map[string]string{"since": now.Add(-time.Hour).Format(time.RFC3339)}},
		{"POST", "/v1/admin/retire", adminToken, map[string]string{"why": "restored"}},
		{"POST", "/v1/admin/audit", adminToken, map[string]string{}},
		{"GET", "/healthz", "", nil},
	}
	for _, r := range requests {
		if code, out := w.call(r.method, r.path, r.token, r.body); code < 500 {
			t.Errorf("%s %s with the database down: %d %v", r.method, r.path, code, out)
		}
	}
	if code, out := w.webhook("transaction.completed", "txn_b", now); code < 500 {
		t.Errorf("Paddle's webhook with the database down: %d %v, and Paddle would not send it again", code, out)
	}

	store.mu.Lock()
	store.down = false
	store.mu.Unlock()
	w.audit()
	if got, _ := w.engine.Keys(w.ctx, "paddle", "txn_b"); len(got) != 0 {
		t.Error("a sale went through while the database was down")
	}
	if len(w.revoked()) != 0 {
		t.Error("a key was revoked while the database was down")
	}
}

// Use case 7 with the mail service down: the lost-key page cannot send
// the keys, says so to its caller, and sends them when asked again once
// the mail is back. Nothing about the sale changes.
func TestLostKeyWhileTheMailIsDown(t *testing.T) {
	f := newFixture(t, false)
	f.stock(4)
	f.shop.sold("txn_a", "anna@example.com")
	keys := f.assign("txn_a", 1)
	f.mail.failing(func(string, Letter) bool { return true })
	before := len(f.mail.sent())
	if err := f.engine.Resend(f.ctx, "anna@example.com"); err == nil {
		t.Fatal("the keys were said to be sent with the mail service down")
	}
	f.mail.failing(nil)
	if err := f.engine.Resend(f.ctx, "anna@example.com"); err != nil {
		t.Fatal(err)
	}
	sent := f.mail.sent()[before:]
	if len(sent) != 1 || sent[0].to != "anna@example.com" || sent[0].letter.Kind != MailResend ||
		!slices.Equal(fingerprints(sent[0].letter.Keys), fingerprints(keys)) {
		t.Fatalf("sent %+v, want the sale's key to its buyer", sent)
	}
	f.audit()
}

// Use case 19 with Paddle down: the daily catch-up cannot ask what was
// sold, says so, and catches everything up the next time it runs.
func TestCatchUpWhilePaddleIsDown(t *testing.T) {
	f := newFixture(t, false)
	f.stock(4)
	f.shop.sold("txn_missed", "anna@example.com")
	f.shop.setDown(true)
	if n, err := f.engine.Reconcile(f.ctx, now.Add(-time.Hour)); err == nil || n != 0 {
		t.Fatalf("caught up %d with Paddle down: %v", n, err)
	}
	f.shop.setDown(false)
	if n, err := f.engine.Reconcile(f.ctx, now.Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("caught up %d once Paddle was back: %v", n, err)
	}
	if keys, _ := f.engine.Keys(f.ctx, "paddle", "txn_missed"); len(keys) != 1 {
		t.Fatal("the missed sale has no key")
	}
	f.audit()
}
