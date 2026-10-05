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
}

func (b *breaking) Update(ctx context.Context, fn func(Tx) error) error {
	b.mu.Lock()
	b.n++
	fails := b.n == b.at
	b.mu.Unlock()
	if fails && !b.after {
		return errBroken
	}
	err := b.Memory.Update(ctx, fn)
	if err == nil && fails {
		return errBroken
	}
	return err
}

// arm makes the store fail its at'th write from now on, and disarm stops it.
func (b *breaking) arm(at int, after bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.n, b.at, b.after = 0, at, after
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
			_, err := f.engine.Stock(f.ctx, f.generation(), batchFor(f))
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
			writes := store.writes()
			want := outcomeOf(f, keys)
			if writes == 0 {
				t.Fatal("the call wrote nothing, so nothing could fail")
			}
			for at := 1; at <= writes; at++ {
				for _, after := range []bool{false, true} {
					when := "before"
					if after {
						when = "after"
					}
					t.Run(fmt.Sprintf("write %d of %d fails %s it saved", at, writes, when), func(t *testing.T) {
						mem := &Memory{}
						store := &breaking{Memory: mem}
						f := newFixtureOn(t, mem, store)
						keys := c.setUp(f)
						store.arm(at, after)
						// A write made after the sale is saved, like the
						// one that says a letter went out, may fail without
						// the caller hearing: the mail run catches it up.
						if err := c.call(f, keys); err != nil && !errors.Is(err, errBroken) {
							t.Fatalf("the failure came back as %v", err)
						}
						store.arm(0, false)
						// Called again, it may say it has nothing left to
						// do, the way a key replaced already is no seat's
						// any more. What counts is where it ends.
						if err := c.call(f, keys); err != nil {
							t.Logf("called again: %v", err)
						}
						if got := outcomeOf(f, keys); got != want {
							t.Errorf("called again, it ended\n%s\nwhere undisturbed it ended\n%s", got, want)
						}
					})
				}
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
