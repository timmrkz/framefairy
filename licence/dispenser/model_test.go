package dispenser

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"framefairy/licence"
)

// model is the dispenser written a second time, as plainly as it can be,
// from docs/LICENCE.md and nothing else: no store, no transactions, no
// record. The engine and the model are given the same random calls, and
// every answer and every error must agree. Two implementations that agree
// on thousands of random sequences are far less likely to share a mistake
// than one implementation is to hold one.
type model struct {
	pool    []licence.Key // in the order stocked
	state   map[licence.Fingerprint]State
	sales   map[string][]licence.Key // by reference, by seat
	revoked map[licence.Fingerprint]Revocation
}

func newModel() *model {
	return &model{
		state:   map[licence.Fingerprint]State{},
		sales:   map[string][]licence.Key{},
		revoked: map[licence.Fingerprint]Revocation{},
	}
}

func (m *model) unsold() []licence.Key {
	var out []licence.Key
	for _, k := range m.pool {
		if m.state[k.Fingerprint()] == Unsold {
			out = append(out, k)
		}
	}
	return out
}

func (m *model) stock(batch []licence.Key) int {
	n := 0
	for _, k := range batch {
		if _, ok := m.state[k.Fingerprint()]; ok {
			continue
		}
		m.pool = append(m.pool, k)
		m.state[k.Fingerprint()] = Unsold
		n++
	}
	return n
}

func (m *model) assign(ref string, seats int) ([]licence.Key, error) {
	if keys, ok := m.sales[ref]; ok {
		if len(keys) != seats {
			return nil, ErrConflict
		}
		return slices.Clone(keys), nil
	}
	free := m.unsold()
	if len(free) < seats {
		return nil, ErrPoolEmpty
	}
	keys := slices.Clone(free[:seats])
	for _, k := range keys {
		m.state[k.Fingerprint()] = Sold
	}
	m.sales[ref] = keys
	return slices.Clone(keys), nil
}

func (m *model) keys(ref string) ([]licence.Key, error) {
	keys, ok := m.sales[ref]
	if !ok {
		return nil, ErrNotFound
	}
	return slices.Clone(keys), nil
}

// holder finds the sale and seat that hold f now.
func (m *model) holder(f licence.Fingerprint) (string, int, bool) {
	for ref, keys := range m.sales {
		for i, k := range keys {
			if k.Fingerprint() == f {
				return ref, i, true
			}
		}
	}
	return "", 0, false
}

func (m *model) notHeld(f licence.Fingerprint) error {
	if _, ok := m.state[f]; !ok {
		return ErrNotFound
	}
	return ErrInvalid
}

func (m *model) targetKeys(ref string, f licence.Fingerprint) ([]licence.Fingerprint, error) {
	if ref != "" {
		keys, ok := m.sales[ref]
		if !ok {
			return nil, ErrNotFound
		}
		return fingerprintsInOrder(keys), nil
	}
	if _, _, ok := m.holder(f); !ok {
		return nil, m.notHeld(f)
	}
	return []licence.Fingerprint{f}, nil
}

func fingerprintsInOrder(keys []licence.Key) []licence.Fingerprint {
	out := make([]licence.Fingerprint, len(keys))
	for i, k := range keys {
		out[i] = k.Fingerprint()
	}
	return out
}

func (m *model) revoke(ref string, f licence.Fingerprint) (int, error) {
	fs, err := m.targetKeys(ref, f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, f := range fs {
		if _, ok := m.revoked[f]; !ok {
			m.revoked[f] = Revoked
			n++
		}
	}
	return n, nil
}

func (m *model) restore(ref string, f licence.Fingerprint) (int, error) {
	fs, err := m.targetKeys(ref, f)
	if err != nil {
		return 0, err
	}
	for _, f := range fs {
		if r, ok := m.revoked[f]; ok && r != Revoked {
			return 0, ErrInvalid
		}
	}
	n := 0
	for _, f := range fs {
		if _, ok := m.revoked[f]; ok {
			delete(m.revoked, f)
			n++
		}
	}
	return n, nil
}

func (m *model) replace(f licence.Fingerprint) (licence.Key, error) {
	ref, seat, ok := m.holder(f)
	if !ok {
		return "", m.notHeld(f)
	}
	if _, ok := m.revoked[f]; ok {
		return "", ErrInvalid
	}
	free := m.unsold()
	if len(free) == 0 {
		return "", ErrPoolEmpty
	}
	m.revoked[f] = Replaced
	next := free[0]
	m.state[next.Fingerprint()] = Sold
	m.sales[ref][seat] = next
	return next, nil
}

func (m *model) clear(to State) int {
	n := 0
	for _, k := range m.unsold() {
		m.state[k.Fingerprint()] = to
		if to == Burned {
			m.revoked[k.Fingerprint()] = BurnedKey
		}
		n++
	}
	return n
}

func (m *model) revocations() []licence.Fingerprint { return sorted(m.revoked) }

func (m *model) genuine() []licence.Fingerprint {
	set := map[licence.Fingerprint]bool{}
	for _, k := range m.pool {
		if m.state[k.Fingerprint()] == Sold {
			set[k.Fingerprint()] = true
		}
	}
	return sorted(set)
}

// kind is the one error of the engine's an answer is, or nil.
func kind(err error) error {
	for _, k := range []error{ErrInvalid, ErrNotFound, ErrPoolEmpty, ErrConflict} {
		if errors.Is(err, k) {
			return k
		}
	}
	return err
}

func TestEngineAgreesWithTheModel(t *testing.T) {
	for _, twice := range []bool{false, true} {
		for seed := range uint64(12) {
			t.Run(fmt.Sprintf("twice=%v/seed=%d", twice, seed), func(t *testing.T) {
				t.Parallel()
				run(t, twice, seed, 600)
			})
		}
	}
}

func run(t *testing.T, twice bool, seed uint64, steps int) {
	f := newFixture(t, twice)
	m := newModel()
	r := rand.New(rand.NewPCG(seed, seed*7919+1))
	var stocked []licence.Key
	refs := func() string { return fmt.Sprintf("txn_%d", r.IntN(30)) }
	// anyKey is a key a call might name: one sold, one unsold, one
	// replaced, one never seen.
	anyKey := func() licence.Fingerprint {
		if len(stocked) == 0 || r.IntN(10) == 0 {
			return licence.Fingerprint{byte(r.IntN(256)), 1}
		}
		return stocked[r.IntN(len(stocked))].Fingerprint()
	}
	for step := range steps {
		where := func(what string) string { return fmt.Sprintf("step %d, %s", step, what) }
		switch op := r.IntN(100); {
		case op < 12:
			var batch []licence.Key
			if len(stocked) > 0 && r.IntN(4) == 0 {
				// A batch handed over again, in part.
				from := r.IntN(len(stocked))
				batch = slices.Clone(stocked[from:min(len(stocked), from+1+r.IntN(5))])
			}
			batch = append(batch, sign(t, 1+r.IntN(6))...)
			got, err := f.engine.Stock(f.ctx, f.generation(), batch)
			want := m.stock(batch)
			if err != nil || got != want {
				t.Fatalf("%s: engine added %d (%v), model %d", where("stock"), got, err, want)
			}
			for _, k := range batch {
				if !slices.Contains(stocked, k) {
					stocked = append(stocked, k)
				}
			}
		case op < 45:
			ref, seats := refs(), 1+r.IntN(3)
			got, err := f.engine.Assign(f.ctx, order(ref, seats))
			want, werr := m.assign(ref, seats)
			if kind(err) != werr || !slices.Equal(got, want) {
				t.Fatalf("%s %s %d: engine %v %v, model %v %v", where("assign"), ref, seats, got, err, want, werr)
			}
		case op < 55:
			ref := refs()
			got, err := f.engine.Keys(f.ctx, "paddle", ref)
			want, werr := m.keys(ref)
			if kind(err) != werr || !slices.Equal(got, want) {
				t.Fatalf("%s %s: engine %v %v, model %v %v", where("keys"), ref, got, err, want, werr)
			}
		case op < 68:
			ref, key := "", licence.Fingerprint{}
			if r.IntN(3) == 0 {
				key = anyKey()
			} else {
				ref = refs()
			}
			got, err := f.engine.Revoke(f.ctx, Target{Source: map[bool]string{true: "paddle"}[ref != ""], Ref: ref, Key: key}, "chargeback")
			want, werr := m.revoke(ref, key)
			if kind(err) != werr || got != want {
				t.Fatalf("%s %q %s: engine %d %v, model %d %v", where("revoke"), ref, key, got, err, want, werr)
			}
		case op < 78:
			ref, key := "", licence.Fingerprint{}
			if r.IntN(3) == 0 {
				key = anyKey()
			} else {
				ref = refs()
			}
			got, err := f.engine.Restore(f.ctx, Target{Source: map[bool]string{true: "paddle"}[ref != ""], Ref: ref, Key: key}, "reversed")
			want, werr := m.restore(ref, key)
			if kind(err) != werr || got != want {
				t.Fatalf("%s %q %s: engine %d %v, model %d %v", where("restore"), ref, key, got, err, want, werr)
			}
		case op < 88:
			key := anyKey()
			got, _, err := f.engine.Replace(f.ctx, key, "posted")
			want, werr := m.replace(key)
			if kind(err) != werr || got != want {
				t.Fatalf("%s %s: engine %v %v, model %v %v", where("replace"), key, got, err, want, werr)
			}
		case op < 90:
			to := []State{Retired, Burned}[r.IntN(2)]
			var got int
			var err error
			if to == Retired {
				got, err = f.engine.Retire(f.ctx, "restored")
			} else {
				got, err = f.engine.RevokeUnsold(f.ctx, "stolen")
			}
			if want := m.clear(to); err != nil || got != want {
				t.Fatalf("%s %s: engine %d %v, model %d", where("clear"), to, got, err, want)
			}
		case op < 95:
			got, err := f.engine.Revocations(f.ctx)
			if want := m.revocations(); err != nil || !slices.Equal(got, want) {
				t.Fatalf("%s: engine %d keys %v, model %d", where("revocations"), len(got), err, len(want))
			}
		default:
			got, err := f.engine.Genuine(f.ctx, 0)
			if want := m.genuine(); err != nil || !slices.Equal(got, want) {
				t.Fatalf("%s: engine %d keys %v, model %d", where("genuine"), len(got), err, len(want))
			}
		}
		if step%100 == 0 {
			f.audit()
		}
		left := f.left()
		if want := len(m.unsold()); left != want {
			t.Fatalf("step %d: engine has %d left, model %d", step, left, want)
		}
	}
	f.audit()
	if got, _ := f.engine.Revocations(f.ctx); !slices.Equal(got, m.revocations()) {
		t.Fatal("the lists differ at the end")
	}
}
