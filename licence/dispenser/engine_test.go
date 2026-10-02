package dispenser

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/licence"
)

func testSigner() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("framefairy test signer, never shipped"))
	return ed25519.NewKeyFromSeed(seed[:])
}

func strangerSigner() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("framefairy dispenser tests, a stranger"))
	return ed25519.NewKeyFromSeed(seed[:])
}

var now = time.Date(2026, time.October, 28, 14, 5, 9, 0, time.UTC)

// ids gives every key in a test run its own ID.
var ids atomic.Uint64

// sign makes n pool keys as the signer would.
func sign(t testing.TB, n int) []licence.Key {
	t.Helper()
	return signWith(t, n, testSigner(), 0, "")
}

func signWith(t testing.TB, n int, key ed25519.PrivateKey, signer uint8, name string) []licence.Key {
	t.Helper()
	keys := make([]licence.Key, n)
	for i := range keys {
		var id licence.ID
		binary.BigEndian.PutUint64(id[:], ids.Add(1))
		k, err := licence.Sign(licence.Licence{Format: licence.Format, Signer: signer, ID: id, Signed: now, Name: name}, key)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = k
	}
	return keys
}

type fixture struct {
	t      testing.TB
	store  *Memory
	engine *Engine
	ctx    context.Context
	clock  *clock
	mail   *fakeMail
	shop   *fakeShop
}

// stores runs a test once on a plain memory store and once on one that
// runs every transaction twice, as a database retrying a collision does.
func stores(t *testing.T, test func(t *testing.T, f *fixture)) {
	for _, twice := range []bool{false, true} {
		t.Run(fmt.Sprintf("twice=%v", twice), func(t *testing.T) {
			test(t, newFixture(t, twice))
		})
	}
}

func newFixture(t testing.TB, twice bool) *fixture {
	f := &fixture{t: t, store: &Memory{Twice: twice}, ctx: context.Background(),
		clock: &clock{at: now}, mail: &fakeMail{}, shop: &fakeShop{}}
	e, err := New(f.store, Config{
		Signers: map[uint8]ed25519.PublicKey{0: testSigner().Public().(ed25519.PublicKey)},
		Batch:   1000,
		Now:     f.clock.now,
		Mailer:  f.mail,
		Orders:  f.shop,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.engine = e
	return f
}

func (f *fixture) stock(n int) []licence.Key {
	f.t.Helper()
	keys := sign(f.t, n)
	added, err := f.engine.Stock(f.ctx, f.generation(), keys)
	if err != nil {
		f.t.Fatal(err)
	}
	if added != n {
		f.t.Fatalf("stocked %d of %d", added, n)
	}
	return keys
}

func (f *fixture) left() int {
	f.t.Helper()
	l, err := f.engine.PoolLevel(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return l.Left
}

// generation is the pool's generation, which a batch must carry.
func (f *fixture) generation() string {
	f.t.Helper()
	l, err := f.engine.PoolLevel(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return l.Generation
}

func (f *fixture) verify() Anchor {
	f.t.Helper()
	head, err := f.engine.VerifyRecord(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return head
}

func order(ref string, seats int) Order {
	return Order{Source: "paddle", Ref: ref, Seats: seats, Email: "anna@example.com", At: now}
}

func TestBoughtThroughPaddle(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(5)
		keys, fresh, _, err := f.engine.assign(f.ctx, order("txn_01", 1))
		if err != nil {
			t.Fatal(err)
		}
		if !fresh || len(keys) != 1 || keys[0] != pool[0] {
			t.Fatalf("got %v, fresh %v, want the first key in the pool", keys, fresh)
		}
		// The same webhook again: the same key, and not fresh, so no mail.
		again, fresh, _, err := f.engine.assign(f.ctx, order("txn_01", 1))
		if err != nil {
			t.Fatal(err)
		}
		if fresh || len(again) != 1 || again[0] != keys[0] {
			t.Fatalf("the same order again gave %v, fresh %v", again, fresh)
		}
		if f.left() != 4 {
			t.Fatalf("%d left, want 4", f.left())
		}
		if head := f.verify(); head.Seq != 6 {
			t.Fatalf("record has %d lines, want 5 stock and 1 assign", head.Seq)
		}
	})
}

func TestSeveralSeats(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		pool := f.stock(10)
		f.engine.Assign(f.ctx, order("txn_a", 2))
		keys, err := f.engine.Assign(f.ctx, order("txn_b", 3))
		if err != nil {
			t.Fatal(err)
		}
		// Handed out in the order they came in.
		for i, k := range keys {
			if k != pool[2+i] {
				t.Fatalf("seat %d got key %d of the pool", i+1, i)
			}
		}
		got, err := f.engine.Keys(f.ctx, "paddle", "txn_b")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0] != keys[0] || got[2] != keys[2] {
			t.Fatalf("Keys gave %v, Assign %v", got, keys)
		}
	})
}

func TestSameOrderWithOtherSeats(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(10)
		if _, err := f.engine.Assign(f.ctx, order("txn_a", 2)); err != nil {
			t.Fatal(err)
		}
		before := f.verify()
		if _, err := f.engine.Assign(f.ctx, order("txn_a", 3)); !errors.Is(err, ErrConflict) {
			t.Fatalf("got %v, want ErrConflict", err)
		}
		if f.left() != 8 || f.verify() != before {
			t.Fatal("a refused order changed something")
		}
	})
}

// A sale bigger than the pool gets nothing at all, so it never holds half
// its keys, and it is assigned in full once the pool is refilled.
func TestPoolEmpty(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(2)
		before := f.verify()
		if _, err := f.engine.Assign(f.ctx, order("txn_big", 3)); !errors.Is(err, ErrPoolEmpty) {
			t.Fatalf("got %v, want ErrPoolEmpty", err)
		}
		if f.left() != 2 || f.verify() != before {
			t.Fatal("a sale the pool could not fill changed something")
		}
		if _, err := f.engine.Keys(f.ctx, "paddle", "txn_big"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		f.stock(1)
		keys, err := f.engine.Assign(f.ctx, order("txn_big", 3))
		if err != nil || len(keys) != 3 {
			t.Fatalf("after a refill: %v, %v", keys, err)
		}
		if _, err := f.engine.Assign(f.ctx, order("txn_next", 1)); !errors.Is(err, ErrPoolEmpty) {
			t.Fatalf("got %v, want ErrPoolEmpty", err)
		}
	})
}

func TestKeysBeforeTheSale(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		if _, err := f.engine.Keys(f.ctx, "paddle", "txn_waiting"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		if _, err := f.engine.Keys(f.ctx, "paddle", "txn waiting"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("got %v, want ErrInvalid", err)
		}
	})
}

// Partners' orders are their own: the same reference from two partners, or
// from a partner and Paddle, is three sales.
func TestSourcesAreApart(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(3)
		seen := map[licence.Key]bool{}
		for _, src := range []string{"paddle", "partner:bundle-hunt", "partner:stacksocial"} {
			keys, err := f.engine.Assign(f.ctx, Order{Source: src, Ref: "1001", Seats: 1, At: now})
			if err != nil {
				t.Fatal(err)
			}
			if seen[keys[0]] {
				t.Fatalf("%s got a key another source holds", src)
			}
			seen[keys[0]] = true
		}
	})
}

func TestInvalidOrders(t *testing.T) {
	f := newFixture(t, false)
	f.stock(5)
	before := f.verify()
	bad := map[string]Order{
		"no source":            {Ref: "a", Seats: 1, At: now},
		"unknown source":       {Source: "stripe", Ref: "a", Seats: 1, At: now},
		"partner without name": {Source: "partner:", Ref: "a", Seats: 1, At: now},
		"partner in capitals":  {Source: "partner:Bundle", Ref: "a", Seats: 1, At: now},
		"partner too long":     {Source: "partner:" + string(make([]byte, 33)), Ref: "a", Seats: 1, At: now},
		"no reference":         {Source: "paddle", Seats: 1, At: now},
		"space in reference":   {Source: "paddle", Ref: "txn 1", Seats: 1, At: now},
		"slash in reference":   {Source: "paddle", Ref: "../txn", Seats: 1, At: now},
		"reference too long":   {Source: "paddle", Ref: string(make([]byte, MaxRef+1)), Seats: 1, At: now},
		"unicode reference":    {Source: "paddle", Ref: "txn_ü", Seats: 1, At: now},
		"no seats":             {Source: "paddle", Ref: "a", Seats: 0, At: now},
		"fewer than none":      {Source: "paddle", Ref: "a", Seats: -1, At: now},
		"too many seats":       {Source: "paddle", Ref: "a", Seats: MaxSeats + 1, At: now},
		"no time":              {Source: "paddle", Ref: "a", Seats: 1},
		"email without at":     {Source: "paddle", Ref: "a", Seats: 1, At: now, Email: "anna.example.com"},
		"email with a newline": {Source: "paddle", Ref: "a", Seats: 1, At: now, Email: "anna@example.com\nBcc: x@y"},
		"email ending in at":   {Source: "paddle", Ref: "a", Seats: 1, At: now, Email: "anna@"},
		"email starting at":    {Source: "paddle", Ref: "a", Seats: 1, At: now, Email: "@example.com"},
		"two addresses":        {Source: "paddle", Ref: "a", Seats: 1, At: now, Email: "a@b.c,d@e.f"},
	}
	for name, o := range bad {
		if _, err := f.engine.Assign(f.ctx, o); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
	if f.left() != 5 || f.verify() != before {
		t.Fatal("refused orders changed something")
	}
}

func TestStockRefusesTheWholeBatch(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		good := sign(t, 3)
		stranger := signWith(t, 1, strangerSigner(), 0, "")
		unknown := signWith(t, 1, testSigner(), 9, "")
		named := signWith(t, 1, testSigner(), 0, "Lena Fischer")
		tampered := good[1][:len(good[1])-2] + "AA"
		cases := map[string][]licence.Key{
			"signed by a stranger":     append(append([]licence.Key{}, good...), stranger...),
			"from an unknown signer":   append(append([]licence.Key{}, good...), unknown...),
			"a named key":              append(append([]licence.Key{}, good...), named...),
			"a tampered key":           {good[0], tampered},
			"not a key":                {good[0], "hello"},
			"a key twice":              {good[0], good[1], good[0]},
			"empty":                    {},
			"too big":                  make([]licence.Key, MaxStock+1),
			"a key with space after":   {good[0] + " "},
			"a key in another letters": {"ff1-" + good[0][4:]},
		}
		for name, batch := range cases {
			if _, err := f.engine.Stock(f.ctx, f.generation(), batch); !errors.Is(err, ErrInvalid) {
				t.Errorf("%s: got %v, want ErrInvalid", name, err)
			}
		}
		if f.left() != 0 {
			t.Fatalf("%d keys in the pool from refused batches", f.left())
		}
		if head := f.verify(); head.Seq != 0 {
			t.Fatalf("%d lines in the record from refused batches", head.Seq)
		}
	})
}

// A batch handed over twice, because the signer never heard the answer,
// adds its keys once.
func TestStockTwice(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		batch := f.stock(4)
		if _, err := f.engine.Assign(f.ctx, order("txn_1", 1)); err != nil {
			t.Fatal(err)
		}
		added, err := f.engine.Stock(f.ctx, f.generation(), batch)
		if err != nil || added != 0 {
			t.Fatalf("the same batch again added %d, %v", added, err)
		}
		more := sign(t, 2)
		added, err = f.engine.Stock(f.ctx, f.generation(), append(append([]licence.Key{}, batch[2:]...), more...))
		if err != nil || added != 2 {
			t.Fatalf("a batch half known added %d, %v", added, err)
		}
		if f.left() != 5 {
			t.Fatalf("%d left, want 5", f.left())
		}
		// The sold key stays sold.
		keys, _ := f.engine.Keys(f.ctx, "paddle", "txn_1")
		next, _ := f.engine.Assign(f.ctx, order("txn_2", 1))
		if next[0] == keys[0] {
			t.Fatal("a sold key was sold again after it was stocked again")
		}
	})
}

// Two keys with the same ID cannot both be in the pool. The signer never
// draws one twice, and the dispenser does not trust that it never will.
func TestStockRefusesAnIDTwice(t *testing.T) {
	f := newFixture(t, false)
	id := licence.ID{7, 7, 7, 7, 7, 7, 7, 7}
	a, _ := licence.Sign(licence.Licence{Format: 1, ID: id, Signed: now}, testSigner())
	b, _ := licence.Sign(licence.Licence{Format: 1, ID: id, Signed: now.AddDate(0, 0, 1)}, testSigner())
	if _, err := f.engine.Stock(f.ctx, f.generation(), []licence.Key{a}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.Stock(f.ctx, f.generation(), []licence.Key{b}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v, want ErrInvalid", err)
	}
}

func TestPoolLevel(t *testing.T) {
	f := newFixture(t, false)
	l, err := f.engine.PoolLevel(f.ctx)
	if err != nil || l.Left != 0 || l.Batch != 1000 || l.Generation != "" {
		t.Fatalf("%+v, %v", l, err)
	}
	f.stock(7)
	f.engine.Assign(f.ctx, order("txn_1", 3))
	if f.left() != 4 {
		t.Fatalf("%d left, want 4", f.left())
	}
}

func TestNewRefuses(t *testing.T) {
	pub := testSigner().Public().(ed25519.PublicKey)
	for name, c := range map[string]Config{
		"no signers":      {Batch: 1000},
		"a short key":     {Signers: map[uint8]ed25519.PublicKey{1: pub[:10]}, Batch: 1000},
		"no batch":        {Signers: map[uint8]ed25519.PublicKey{1: pub}},
		"too big a batch": {Signers: map[uint8]ed25519.PublicKey{1: pub}, Batch: MaxStock + 1},
	} {
		c.Mailer = &fakeMail{}
		if _, err := New(&Memory{}, c); err == nil {
			t.Errorf("%s: made an engine", name)
		}
	}
	if _, err := New(&Memory{}, Config{Signers: map[uint8]ed25519.PublicKey{1: pub}, Batch: 1}); err == nil {
		t.Error("made an engine without a mailer")
	}
	if _, err := New(nil, Config{Signers: map[uint8]ed25519.PublicKey{1: pub}, Batch: 1, Mailer: &fakeMail{}}); err == nil {
		t.Error("made an engine without a store")
	}
}

func TestRecordChain(t *testing.T) {
	f := newFixture(t, false)
	f.stock(3)
	f.engine.Assign(f.ctx, order("txn_1", 2))
	head := f.verify()
	if head.Seq != 5 {
		t.Fatalf("head at %d, want 5", head.Seq)
	}
	// The head is an anchor of itself, and of the record after more lines.
	f.engine.Assign(f.ctx, order("txn_2", 1))
	if _, err := f.engine.VerifyRecord(f.ctx, head); err != nil {
		t.Fatal(err)
	}

	tamper := func(name string, change func(r []Line) []Line, anchors ...Anchor) {
		t.Helper()
		saved := f.store.db.record
		f.store.db.record = change(append([]Line(nil), saved...))
		defer func() { f.store.db.record = saved }()
		if _, err := f.engine.VerifyRecord(f.ctx, anchors...); !errors.Is(err, ErrBrokenRecord) {
			t.Errorf("%s: got %v, want ErrBrokenRecord", name, err)
		}
	}
	tamper("a reference changed", func(r []Line) []Line { r[4].Ref = "txn_9"; return r })
	tamper("a seat changed", func(r []Line) []Line { r[4].Seat = 3; return r })
	tamper("a key changed", func(r []Line) []Line { r[3].Fingerprint[0] ^= 1; return r })
	tamper("a time changed", func(r []Line) []Line { r[3].At = r[3].At.Add(time.Second); return r })
	tamper("an event changed", func(r []Line) []Line { r[0].Event = EventAssign; return r })
	tamper("a line removed", func(r []Line) []Line { return append(r[:2], r[3:]...) })
	tamper("two lines swapped", func(r []Line) []Line { r[1], r[2] = r[2], r[1]; return r })
	tamper("a line doubled", func(r []Line) []Line { return append(r[:3], r[2:]...) })
	// Every line rewritten and hashed again holds as a chain, which is why
	// the head is published where the dispenser cannot reach it.
	tamper("history rewritten", func(r []Line) []Line {
		r[3].Ref = "txn_9"
		prev := r[2].Hash
		for i := 3; i < len(r); i++ {
			r[i].Prev = prev
			r[i].Hash = r[i].digest()
			prev = r[i].Hash
		}
		return r
	}, head)
	tamper("the published line removed from the end", func(r []Line) []Line { return r[:head.Seq-1] }, head)
}

// The hash covers every field, so two lines that differ anywhere hash
// differently, even where the field bounds could be read another way.
func TestDigestTellsFieldsApart(t *testing.T) {
	base := Line{Seq: 1, At: now, Event: "assign", Source: "paddle", Ref: "ab", Seat: 1}
	other := base
	other.Source, other.Ref = "paddlea", "b"
	if base.digest() == other.digest() {
		t.Fatal("moving a letter from one field to the next does not change the hash")
	}
	seen := map[[32]byte]bool{base.digest(): true}
	for _, change := range []func(*Line){
		func(l *Line) { l.Seq = 2 },
		func(l *Line) { l.At = l.At.Add(time.Second) },
		func(l *Line) { l.Event = "stock" },
		func(l *Line) { l.Source = "partner:x" },
		func(l *Line) { l.Ref = "ac" },
		func(l *Line) { l.Seat = 2 },
		func(l *Line) { l.Why = "x" },
		func(l *Line) { l.Fingerprint[15] = 1 },
		func(l *Line) { l.Prev[31] = 1 },
	} {
		l := base
		change(&l)
		d := l.digest()
		if seen[d] {
			t.Fatalf("a change left the hash as it was: %+v", l)
		}
		seen[d] = true
	}
}

// Many sales at once, each webhook arriving several times, in no order,
// from many goroutines, on both stores. No key is sold twice, every order
// gets exactly its own keys every time it asks, and the record holds one
// line per seat and still checks.
func TestManySalesAtOnce(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		const orders, repeats, workers = 120, 3, 16
		f.stock(orders * 3)
		type job struct {
			ref   string
			seats int
		}
		var jobs []job
		for i := range orders {
			for range repeats {
				jobs = append(jobs, job{fmt.Sprintf("txn_%03d", i), 1 + i%3})
			}
		}
		rand.Shuffle(len(jobs), func(i, j int) { jobs[i], jobs[j] = jobs[j], jobs[i] })

		var mu sync.Mutex
		got := map[string][]licence.Key{}
		fresh := map[string]int{}
		next := atomic.Int64{}
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				for {
					i := int(next.Add(1)) - 1
					if i >= len(jobs) {
						return
					}
					j := jobs[i]
					keys, isNew, _, err := f.engine.assign(f.ctx, order(j.ref, j.seats))
					if err != nil {
						t.Error(err)
						return
					}
					// Read while others write.
					if _, err := f.engine.Keys(f.ctx, "paddle", j.ref); err != nil {
						t.Error(err)
						return
					}
					if _, err := f.engine.PoolLevel(f.ctx); err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					if isNew {
						fresh[j.ref]++
					}
					if prev, ok := got[j.ref]; ok {
						if len(prev) != len(keys) {
							t.Errorf("%s got %d keys, then %d", j.ref, len(prev), len(keys))
						}
						for k := range keys {
							if prev[k] != keys[k] {
								t.Errorf("%s got another key the second time", j.ref)
							}
						}
					}
					got[j.ref] = keys
					mu.Unlock()
				}
			})
		}
		wg.Wait()

		owner := map[licence.Key]string{}
		seats := 0
		for ref, keys := range got {
			if fresh[ref] != 1 {
				t.Errorf("%s was new %d times, so it would be mailed that often", ref, fresh[ref])
			}
			for _, k := range keys {
				if o, ok := owner[k]; ok {
					t.Fatalf("one key sold to %s and %s", o, ref)
				}
				owner[k] = ref
				seats++
			}
		}
		if len(got) != orders {
			t.Fatalf("%d orders served, want %d", len(got), orders)
		}
		if f.left() != orders*3-seats {
			t.Fatalf("%d left, %d sold, of %d", f.left(), seats, orders*3)
		}
		if head := f.verify(); head.Seq != int64(orders*3+seats) {
			t.Fatalf("the record has %d lines, want %d", head.Seq, orders*3+seats)
		}
	})
}

func TestCancelledContext(t *testing.T) {
	f := newFixture(t, false)
	f.stock(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.engine.Assign(ctx, order("txn_1", 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if f.left() != 1 {
		t.Fatal("a cancelled sale took a key")
	}
}
