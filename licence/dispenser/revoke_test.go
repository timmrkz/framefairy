package dispenser

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"framefairy/licence"
)

func (f *fixture) audit() {
	f.t.Helper()
	if _, err := f.engine.Audit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revoked() []licence.Fingerprint {
	f.t.Helper()
	list, err := f.engine.Revocations(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return list
}

func (f *fixture) assign(ref string, seats int) []licence.Key {
	f.t.Helper()
	keys, err := f.engine.Assign(f.ctx, order(ref, seats))
	if err != nil {
		f.t.Fatal(err)
	}
	return keys
}

func fingerprints(keys []licence.Key) []licence.Fingerprint {
	out := make([]licence.Fingerprint, len(keys))
	for i, k := range keys {
		out[i] = k.Fingerprint()
	}
	slices.SortFunc(out, func(a, b licence.Fingerprint) int { return strings.Compare(string(a[:]), string(b[:])) })
	return out
}

func sale(ref string) Target { return Target{Source: "paddle", Ref: ref} }

// Use cases 8 and 10: every key of the sale is on the next list, and the
// same adjustment twice changes nothing.
func TestChargeback(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(10)
		keys := f.assign("txn_a", 2)
		other := f.assign("txn_b", 1)
		n, err := f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback adj_01")
		if err != nil || n != 2 {
			t.Fatalf("revoked %d, %v", n, err)
		}
		if got := f.revoked(); !slices.Equal(got, fingerprints(keys)) {
			t.Fatalf("list %v, want %v", got, fingerprints(keys))
		}
		if n, err := f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback adj_01"); err != nil || n != 0 {
			t.Fatalf("the same chargeback again revoked %d, %v", n, err)
		}
		// The keys still show on the thank-you page: they are the sale's,
		// revoked or not.
		if got, _ := f.engine.Keys(f.ctx, "paddle", "txn_a"); len(got) != 2 {
			t.Fatal("a revoked sale lost its keys")
		}
		if slices.Contains(f.revoked(), other[0].Fingerprint()) {
			t.Fatal("another sale's key was revoked")
		}
		f.audit()
	})
}

// Use case 9: those keys leave the list again.
func TestChargebackReversed(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(10)
		f.assign("txn_a", 2)
		f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback")
		n, err := f.engine.Restore(f.ctx, sale("txn_a"), "chargeback reversed")
		if err != nil || n != 2 {
			t.Fatalf("restored %d, %v", n, err)
		}
		if len(f.revoked()) != 0 {
			t.Fatal("restored keys are still on the list")
		}
		if n, _ := f.engine.Restore(f.ctx, sale("txn_a"), "again"); n != 0 {
			t.Fatal("restored twice")
		}
		// And can be revoked again, should the bank change its mind again.
		if n, _ := f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback again"); n != 2 {
			t.Fatal("not revoked after a restore")
		}
		f.audit()
	})
}

func TestRevokeOneKey(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(10)
		keys := f.assign("txn_a", 3)
		n, err := f.engine.Revoke(f.ctx, Target{Key: keys[1].Fingerprint()}, "one seat refunded")
		if err != nil || n != 1 {
			t.Fatalf("revoked %d, %v", n, err)
		}
		if got := f.revoked(); len(got) != 1 || got[0] != keys[1].Fingerprint() {
			t.Fatalf("list %v", got)
		}
		f.audit()
	})
}

func TestRevokeRefuses(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(5)
		keys := f.assign("txn_a", 1)
		cases := []struct {
			name   string
			target Target
			why    string
			want   error
		}{
			{"no reason", sale("txn_a"), "", ErrInvalid},
			{"a reason on two lines", sale("txn_a"), "charge\nback", ErrInvalid},
			{"a reason turned around", sale("txn_a"), "‮chargeback", ErrInvalid},
			{"a reason too long", sale("txn_a"), strings.Repeat("x", MaxWhy+1), ErrInvalid},
			{"no target", Target{}, "x", ErrInvalid},
			{"a sale and a key", Target{Source: "paddle", Ref: "txn_a", Key: keys[0].Fingerprint()}, "x", ErrInvalid},
			{"a sale not there", sale("txn_nope"), "x", ErrNotFound},
			{"a bad reference", sale("txn nope"), "x", ErrInvalid},
			{"a key not there", Target{Key: licence.Fingerprint{1}}, "x", ErrNotFound},
			{"an unsold key", Target{Key: pool[3].Fingerprint()}, "x", ErrInvalid},
		}
		for _, c := range cases {
			if _, err := f.engine.Revoke(f.ctx, c.target, c.why); !errors.Is(err, c.want) {
				t.Errorf("revoke, %s: got %v, want %v", c.name, err, c.want)
			}
			if _, err := f.engine.Restore(f.ctx, c.target, c.why); !errors.Is(err, c.want) {
				t.Errorf("restore, %s: got %v, want %v", c.name, err, c.want)
			}
		}
		if len(f.revoked()) != 0 {
			t.Fatal("a refused revoke revoked something")
		}
		f.audit()
	})
}

// Use case 11: the key is revoked, the seat holds the next pool key, and
// the old one can never come back, not even by a reversed chargeback.
func TestKeyPostedInPublic(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(5)
		keys := f.assign("txn_a", 2)
		next, seat, err := f.engine.Replace(f.ctx, keys[1].Fingerprint(), "posted on a forum")
		if err != nil {
			t.Fatal(err)
		}
		if next != pool[2] || seat.Seat != 2 || seat.Ref != "txn_a" || seat.Key != next.Fingerprint() {
			t.Fatalf("got %s at %+v", next, seat)
		}
		now, _ := f.engine.Keys(f.ctx, "paddle", "txn_a")
		if now[0] != keys[0] || now[1] != next {
			t.Fatal("the sale does not hold the new key in the old key's seat")
		}
		if got := f.revoked(); len(got) != 1 || got[0] != keys[1].Fingerprint() {
			t.Fatalf("list %v", got)
		}
		// A chargeback and its reversal later leave the posted key revoked.
		f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback")
		if n, err := f.engine.Restore(f.ctx, sale("txn_a"), "reversed"); err != nil || n != 2 {
			t.Fatalf("restored %d, %v", n, err)
		}
		if got := f.revoked(); len(got) != 1 || got[0] != keys[1].Fingerprint() {
			t.Fatalf("after the reversal the list is %v", got)
		}
		// The old key is no seat's any more, so it cannot be revoked,
		// restored or replaced again.
		old := keys[1].Fingerprint()
		if _, err := f.engine.Restore(f.ctx, Target{Key: old}, "x"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("restoring a replaced key: %v", err)
		}
		if _, _, err := f.engine.Replace(f.ctx, old, "x"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("replacing a replaced key: %v", err)
		}
		f.audit()
	})
}

func TestReplaceRefuses(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(2)
		keys := f.assign("txn_a", 1)
		f.assign("txn_b", 1)
		if _, _, err := f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted"); !errors.Is(err, ErrPoolEmpty) {
			t.Fatalf("an empty pool: %v", err)
		}
		f.stock(2)
		if _, _, err := f.engine.Replace(f.ctx, keys[0].Fingerprint(), ""); !errors.Is(err, ErrInvalid) {
			t.Fatalf("no reason: %v", err)
		}
		if _, _, err := f.engine.Replace(f.ctx, licence.Fingerprint{9}, "posted"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("a key not there: %v", err)
		}
		f.engine.Revoke(f.ctx, sale("txn_a"), "chargeback")
		if _, _, err := f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("a revoked key: %v", err)
		}
		if f.left() != 2 || len(f.revoked()) != 1 || pool[0] != keys[0] {
			t.Fatal("a refused replace changed something")
		}
		f.audit()
	})
}

// Use case 14: every unsold key is revoked, sold keys keep working, and the
// pool takes a fresh batch.
func TestDatabaseStolen(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(6)
		sold := f.assign("txn_a", 2)
		n, err := f.engine.RevokeUnsold(f.ctx, "database stolen")
		if err != nil || n != 4 {
			t.Fatalf("burned %d, %v", n, err)
		}
		if got := f.revoked(); !slices.Equal(got, fingerprints(pool[2:])) {
			t.Fatal("the list is not the unsold keys")
		}
		for _, k := range sold {
			if slices.Contains(f.revoked(), k.Fingerprint()) {
				t.Fatal("a sold key was burned")
			}
		}
		if _, err := f.engine.Assign(f.ctx, order("txn_b", 1)); !errors.Is(err, ErrPoolEmpty) {
			t.Fatalf("sold from a burned pool: %v", err)
		}
		// The burned keys never come back, not even handed over again.
		if added, err := f.engine.Stock(f.ctx, f.generation(), pool); err != nil || added != 0 {
			t.Fatalf("a burned batch handed over again added %d, %v", added, err)
		}
		fresh := f.stock(3)
		if got := f.assign("txn_b", 1); got[0] != fresh[0] {
			t.Fatal("not sold from the fresh batch")
		}
		f.audit()
	})
}

// Use case 15: every key unsold in the backup is set aside, not revoked,
// and never handed out again. The Anna example in docs/LICENCE.md.
func TestRestoredFromBackup(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(5)
		f.assign("txn_before", 1)
		// The backup is taken here. Anna buys after it, gets pool[1], and
		// the database is lost. What comes back is the store as it was.
		backup := f.store.db.clone()
		anna := f.assign("txn_anna", 1)
		if anna[0] != pool[1] {
			t.Fatal("Anna did not get the next key")
		}
		f.store.db = backup

		n, err := f.engine.Retire(f.ctx, "restored from the 10:00 backup")
		if err != nil || n != 4 {
			t.Fatalf("retired %d, %v", n, err)
		}
		if len(f.revoked()) != 0 {
			t.Fatal("retiring revoked keys, and Anna's key with them")
		}
		fresh := f.stock(3)
		// The catch-up finds Anna's sale at Paddle and assigns it again.
		again := f.assign("txn_anna", 1)
		if again[0] != fresh[0] {
			t.Fatal("Anna's sale was not given a fresh key")
		}
		if again[0] == anna[0] {
			t.Fatal("Anna's old key was handed out again")
		}
		// No one else is ever given pool[1].
		for i := range 2 {
			k := f.assign(fmt.Sprintf("txn_next_%d", i), 1)
			if k[0] == anna[0] {
				t.Fatal("a retired key was sold")
			}
		}
		if _, err := f.engine.Assign(f.ctx, order("txn_last", 1)); !errors.Is(err, ErrPoolEmpty) {
			t.Fatal("sold a retired key")
		}
		f.audit()
	})
}

// Use case 17: the genuine list is every pool key that was sold, revoked
// ones included, and none that was not.
func TestGenuine(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(6)
		a := f.assign("txn_a", 2)
		b := f.assign("txn_b", 1)
		f.engine.Revoke(f.ctx, sale("txn_b"), "chargeback")
		next, _, err := f.engine.Replace(f.ctx, a[0].Fingerprint(), "posted")
		if err != nil {
			t.Fatal(err)
		}
		got, err := f.engine.Genuine(f.ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		want := fingerprints([]licence.Key{a[0], a[1], b[0], next})
		if !slices.Equal(got, want) {
			t.Fatalf("genuine %v, want %v", got, want)
		}
		if other, _ := f.engine.Genuine(f.ctx, 1); len(other) != 0 {
			t.Fatal("signer 1 has genuine keys it never signed")
		}
	})
}

func TestAuditFindsAStoreThatDiffers(t *testing.T) {
	f := newFixture(t, false)
	f.stock(4)
	keys := f.assign("txn_a", 1)
	if n, err := f.engine.Revoke(f.ctx, sale("txn_a"), "x"); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	f.audit()

	tamper := func(name string, change func(db *memoryDB)) {
		t.Helper()
		saved := f.store.db.clone()
		change(&f.store.db)
		defer func() { f.store.db = saved }()
		if _, err := f.engine.Audit(f.ctx); !errors.Is(err, ErrAudit) {
			t.Errorf("%s: got %v, want ErrAudit", name, err)
		}
	}
	tamper("a key marked unsold again", func(db *memoryDB) { db.pool[0].State = Unsold })
	tamper("a key marked sold", func(db *memoryDB) { db.pool[2].State = Sold })
	tamper("a key retired", func(db *memoryDB) { db.pool[2].State = Retired })
	tamper("a key taken off the list", func(db *memoryDB) { db.revoked = map[licence.Fingerprint]Revocation{} })
	tamper("a key put on the list", func(db *memoryDB) { db.revoked[db.pool[3].Fingerprint] = Revoked })
	tamper("a seat given another key", func(db *memoryDB) {
		db.seats[seatKey("paddle", "txn_a")][0].Key = db.pool[3].Fingerprint
	})
	tamper("a seat added", func(db *memoryDB) {
		db.seats[seatKey("paddle", "txn_c")] = []Seat{{Source: "paddle", Ref: "txn_c", Seat: 1, Key: db.pool[3].Fingerprint}}
	})
	tamper("a key swapped for another", func(db *memoryDB) {
		db.pool[1].Key = keys[0]
	})
	tamper("two keys in the pool swapped", func(db *memoryDB) { db.pool[1], db.pool[2] = db.pool[2], db.pool[1] })
	tamper("a key dropped from the pool", func(db *memoryDB) { db.pool = db.pool[:3] })
}

// Every call at once from many goroutines, under the race detector, on
// both stores, and then the audit: whatever order they ran in, the store
// is what its record adds up to, and no key is held by two seats.
func TestEverythingAtOnce(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(400)
		var wg sync.WaitGroup
		for g := range 12 {
			wg.Go(func() {
				for i := range 30 {
					ref := fmt.Sprintf("txn_%d", (g*7+i)%40)
					var err error
					switch (g + i) % 8 {
					case 0, 1, 2:
						_, err = f.engine.Assign(f.ctx, order(ref, 1+i%3))
						if errors.Is(err, ErrConflict) {
							err = nil
						}
					case 3:
						_, err = f.engine.Revoke(f.ctx, sale(ref), "chargeback")
					case 4:
						_, err = f.engine.Restore(f.ctx, sale(ref), "reversed")
					case 5:
						var keys []licence.Key
						keys, err = f.engine.Keys(f.ctx, "paddle", ref)
						if err == nil {
							_, _, err = f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted")
						}
						if errors.Is(err, ErrInvalid) {
							err = nil // revoked, or replaced by another goroutine first
						}
					case 6:
						_, err = f.engine.Revocations(f.ctx)
					case 7:
						_, err = f.engine.Stock(f.ctx, f.generation(), sign(t, 2))
					}
					if errors.Is(err, ErrNotFound) {
						err = nil
					}
					if err != nil {
						t.Error(err)
						return
					}
				}
			})
		}
		wg.Wait()
		f.audit()
	})
}
