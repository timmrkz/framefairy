package dispenser

import (
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/licence"
)

// The simulation runs the real engine in a world that goes wrong: buyers
// buy, Paddle sends its webhooks late, twice, in the wrong order or never,
// takes money back and gives it back, the database crashes before and
// after it commits, the mail service fails or fails after it sent, Paddle
// goes down, the pool runs dry, the signer's hand-over drops, keys are
// posted in public, and the database comes back from a backup. Every step
// is drawn from one seed, so a run that fails runs again the same way:
//
//	go test ./licence/dispenser -run TestSimulation -seed 1234 -v
//
// After every step the rules that must always hold are checked: no key is
// ever given to two sales, no letter carries a key of someone else's sale,
// and the store is what its record adds up to. Then the world calms down,
// everything pending is let through, and the rules that hold once it has
// are checked: every paid sale has its keys and the letter with them, and
// the revocation list is exactly what Paddle and the posted keys say.

var (
	simSeed  = flag.Uint64("seed", 0, "run the simulation with this seed only")
	simSeeds = flag.Int("seeds", 100, "how many seeds the simulation runs")
	simSteps = flag.Int("steps", 400, "how many steps each seed runs")
)

func TestSimulation(t *testing.T) {
	if *simSeed != 0 {
		simulate(t, *simSeed, *simSteps, true)
		return
	}
	for seed := range uint64(*simSeeds) {
		t.Run(fmt.Sprint(seed+1), func(t *testing.T) {
			t.Parallel()
			simulate(t, seed+1, *simSteps, false)
		})
	}
}

var errCrash = errors.New("the database went away")

// crashing is a store that, now and then, loses the connection before a
// transaction commits, or after, so the caller is told it failed when it
// did not.
type crashing struct {
	*Memory
	w             *world
	before, after float64
}

func (c *crashing) Update(ctx context.Context, fn func(Tx) error) error {
	if c.w.r.Float64() < c.before {
		c.w.log("  the database is lost before a commit")
		return errCrash
	}
	err := c.Memory.Update(ctx, fn)
	if err == nil && c.w.r.Float64() < c.after {
		c.w.log("  the database commits, and the answer is lost")
		return errCrash
	}
	return err
}

// simMail is a mail service that fails, and sometimes fails after it sent.
type simMail struct {
	w        *world
	letters  []sent
	warnings []string
	fail     float64
}

func (m *simMail) Keys(ctx context.Context, to string, l Letter) error {
	roll := m.w.r.Float64()
	if roll < m.fail {
		m.w.log("  the mail service is down")
		return errors.New("the mail service is down")
	}
	m.letters = append(m.letters, sent{to, l})
	m.w.letter(to, l)
	if roll < m.fail*1.5 {
		m.w.log("  the mail service sends, and fails to say so")
		return errors.New("the mail service timed out")
	}
	return nil
}

func (m *simMail) Us(ctx context.Context, subject, body string) error {
	m.warnings = append(m.warnings, subject+": "+body)
	return nil
}

// webhook is one Paddle has yet to deliver.
type webhook struct {
	kind  string // "completed" or "adjusted"
	sale  Sale   // as it was when the webhook was sent
	first time.Time
}

// handover is a batch the signer handed over, and the pool generation it
// was signed for.
type handover struct {
	generation string
	keys       []licence.Key
}

// world is everything around the dispenser.
type world struct {
	t       testing.TB
	seed    uint64
	r       *rand.Rand
	clock   *clock
	store   *crashing
	mail    *simMail
	shop    *fakeShop
	engine  *Engine
	trust   licence.Trust
	history []string
	verbose bool

	hooks    []webhook
	sales    int
	owner    map[licence.Key]string // the sale every key was ever given to
	replaced map[licence.Fingerprint]bool
	partners map[string]string // a partner sale's reference, and its buyer
	pending  []handover        // batches the signer handed over without hearing back
	ids      uint64
	backup   *memoryDB
	backedUp time.Time
	restored bool
	lastDay  int
}

const simBatch = 30

func simulate(t *testing.T, seed uint64, steps int, verbose bool) {
	w := &world{
		t: t, seed: seed, verbose: verbose,
		r:        rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		clock:    &clock{at: now},
		shop:     &fakeShop{},
		owner:    map[licence.Key]string{},
		replaced: map[licence.Fingerprint]bool{},
		partners: map[string]string{},
	}
	w.store = &crashing{Memory: &Memory{Twice: seed%2 == 0}, w: w, before: 0.03, after: 0.03}
	w.mail = &simMail{w: w, fail: 0.1}
	w.trust = licence.Trust{Signers: map[uint8]ed25519.PublicKey{0: testSigner().Public().(ed25519.PublicKey)}}
	w.restart()
	defer func() {
		if t.Failed() {
			t.Logf("seed %d, the last steps:\n%s", seed, strings.Join(w.history[max(0, len(w.history)-60):], "\n"))
		}
	}()

	w.signer()
	for step := range steps {
		if t.Failed() {
			return
		}
		w.step(step)
		if step%25 == 0 {
			w.audit()
		}
	}
	w.calm()
}

func (w *world) log(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	w.history = append(w.history, line)
	if w.verbose {
		w.t.Log(line)
	}
}

// restart starts a new engine on the same store, as a serverless container
// that crashed is started again.
func (w *world) restart() {
	e, err := New(w.store, Config{
		Signers: w.trust.Signers,
		Batch:   simBatch,
		Now:     w.clock.now,
		Mailer:  w.mail,
		Orders:  w.shop,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	w.engine = e
}

func (w *world) ctx() context.Context { return context.Background() }

func (w *world) step(step int) {
	switch n := w.r.IntN(100); {
	case n < 18:
		w.buy()
	case n < 42:
		w.deliver()
	case n < 47:
		w.adjust()
	case n < 55:
		w.log("%d: mail runs", step)
		w.engine.SendMail(w.ctx())
	case n < 62:
		w.signer()
	case n < 65:
		w.lostKey()
	case n < 67:
		w.replace()
	case n < 70:
		w.partner()
	case n < 72:
		w.log("%d: the dispenser restarts", step)
		w.restart()
	case n < 74:
		down := w.r.IntN(2) == 0
		w.log("%d: Paddle down %v", step, down)
		w.shop.setDown(down)
	case n < 75:
		w.takeBackup()
	case n < 76:
		w.restore()
	default:
		d := time.Duration(1+w.r.IntN(360)) * time.Minute
		w.clock.add(d)
		w.log("%d: %s pass, now %s", step, d, w.clock.now().Format(time.DateTime))
		w.daily()
	}
}

// daily is the CRON run, once a day: catch up with Paddle, warn about the
// pool.
func (w *world) daily() {
	day := int(w.clock.now().Sub(now) / (24 * time.Hour))
	if day == w.lastDay {
		return
	}
	w.lastDay = day
	n, err := w.engine.Reconcile(w.ctx(), w.clock.now().Add(-7*24*time.Hour))
	w.log("  the daily run reconciles %d sales: %v", n, err)
	w.engine.CheckPool(w.ctx())
}

func (w *world) buy() {
	w.sales++
	sale := Sale{
		Ref:   fmt.Sprintf("txn_%04d", w.sales),
		Seats: 1 + w.r.IntN(3),
		Email: fmt.Sprintf("buyer%d@example.com", w.r.IntN(12)),
		At:    w.clock.now(),
	}
	w.shop.put(sale, w.clock.now())
	w.hooks = append(w.hooks, webhook{kind: "completed", sale: sale, first: w.clock.now()})
	w.log("%s bought %d seats as %s", sale.Email, sale.Seats, sale.Ref)
}

// deliver hands one webhook to the dispenser, the way the Paddle adapter
// will: a completed sale is assigned from what the webhook says, an
// adjustment settles the sale with what Paddle says now. A webhook that
// fails stays and comes again, sometimes one that succeeded comes again
// too, and after three days Paddle gives up on it.
func (w *world) deliver() {
	if len(w.hooks) == 0 {
		return
	}
	i := w.r.IntN(len(w.hooks))
	h := w.hooks[i]
	if w.clock.now().Sub(h.first) > 72*time.Hour {
		w.log("Paddle gives up on the %s webhook of %s", h.kind, h.sale.Ref)
		w.hooks = slices.Delete(w.hooks, i, i+1)
		return
	}
	var err error
	if h.kind == "completed" {
		var keys []licence.Key
		keys, err = w.engine.Assign(w.ctx(), Order{Source: "paddle", Ref: h.sale.Ref, Seats: h.sale.Seats, Email: h.sale.Email, At: h.sale.At})
		w.given(h.sale.Ref, keys)
	} else {
		err = w.engine.Settle(w.ctx(), h.sale.Ref)
	}
	w.log("the %s webhook of %s arrives: %v", h.kind, h.sale.Ref, err)
	if err == nil && w.r.IntN(8) != 0 {
		w.hooks = slices.Delete(w.hooks, i, i+1)
	}
}

func (w *world) adjust() {
	if w.sales == 0 {
		return
	}
	ref := fmt.Sprintf("txn_%04d", 1+w.r.IntN(w.sales))
	w.shop.mu.Lock()
	sale := w.shop.sales[ref]
	sale.TakenBack = !sale.TakenBack
	w.shop.mu.Unlock()
	w.shop.put(sale, w.clock.now())
	w.hooks = append(w.hooks, webhook{kind: "adjusted", sale: sale, first: w.clock.now()})
	w.log("Paddle says %s is taken back: %v", ref, sale.TakenBack)
}

// signer is the signer waking up: it hands over again what it never heard
// back about, and refills the pool when it runs low.
func (w *world) signer() {
	if len(w.pending) > 0 {
		h := w.pending[0]
		added, err := w.engine.Stock(w.ctx(), h.generation, h.keys)
		w.log("the signer hands over a batch again: %d new, %v", added, err)
		if err == nil || errors.Is(err, ErrStale) {
			w.pending = w.pending[1:]
		}
		return
	}
	level, err := w.engine.PoolLevel(w.ctx())
	if err != nil || level.Left*10 >= level.Batch*3 {
		return
	}
	batch := level.Batch
	keys := make([]licence.Key, batch)
	for i := range keys {
		w.ids++
		var id licence.ID
		for b := range 8 {
			id[b] = byte(w.ids >> (56 - 8*b))
		}
		id[0] ^= byte(w.seed)
		k, err := licence.Sign(licence.Licence{Format: licence.Format, ID: id, Signed: w.clock.now()}, testSigner())
		if err != nil {
			w.t.Fatal(err)
		}
		keys[i] = k
	}
	added, err := w.engine.Stock(w.ctx(), level.Generation, keys)
	w.log("the signer hands over %d keys: %d new, %v", len(keys), added, err)
	if err != nil && !errors.Is(err, ErrStale) {
		w.pending = append(w.pending, handover{level.Generation, keys})
	}
}

func (w *world) lostKey() {
	email := fmt.Sprintf("buyer%d@example.com", w.r.IntN(12))
	err := w.engine.Resend(w.ctx(), email)
	w.log("%s asks for their keys again: %v", email, err)
}

func (w *world) replace() {
	if w.sales == 0 {
		return
	}
	ref := fmt.Sprintf("txn_%04d", 1+w.r.IntN(w.sales))
	keys, err := w.engine.Keys(w.ctx(), "paddle", ref)
	if err != nil {
		return
	}
	old := keys[w.r.IntN(len(keys))]
	next, seat, err := w.engine.Replace(w.ctx(), old.Fingerprint(), "posted in public")
	w.log("a key of %s is posted in public and replaced: %v", ref, err)
	if err == nil {
		w.replaced[old.Fingerprint()] = true
		w.given(seat.Ref, []licence.Key{next})
	}
	// A crash after the commit replaced it all the same.
	if errors.Is(err, errCrash) {
		if now, _ := w.engine.Keys(w.ctx(), "paddle", ref); !slices.Contains(now, old) {
			w.replaced[old.Fingerprint()] = true
		}
	}
}

func (w *world) partner() {
	ref := fmt.Sprintf("p%d", w.r.IntN(20))
	email := fmt.Sprintf("partnerbuyer%s@example.com", ref)
	w.partners["partner:bundle/"+ref] = email
	keys, err := w.engine.Assign(w.ctx(), Order{Source: "partner:bundle", Ref: ref, Seats: 1, Email: email, At: w.clock.now()})
	w.log("the partner sells %s: %v", ref, err)
	w.given("partner:bundle/"+ref, keys)
}

func (w *world) takeBackup() {
	db := w.store.Memory.db.clone()
	w.backup, w.backedUp = &db, w.clock.now()
	w.log("the database is backed up")
}

// restore brings the database back from the backup and follows the
// procedure in docs/LICENCE.md: set the unsold pool aside, refill, catch
// up with Paddle from the backup's time.
func (w *world) restore() {
	if w.backup == nil {
		return
	}
	w.store.Memory.db = w.backup.clone()
	w.restored = true
	w.log("the database is lost and comes back from the backup of %s", w.backedUp.Format(time.DateTime))
	w.restart()
	for {
		n, err := w.engine.Retire(w.ctx(), "restored from a backup")
		w.log("  the unsold pool is set aside: %d, %v", n, err)
		if err == nil {
			break
		}
	}
	w.signer()
	n, err := w.engine.Reconcile(w.ctx(), w.backedUp.Add(-time.Hour))
	w.log("  caught up with Paddle: %d, %v", n, err)
}

// given notes that keys went to a sale, and fails the run if any of them
// ever went to another.
func (w *world) given(sale string, keys []licence.Key) {
	for _, k := range keys {
		o, ok := w.owner[k]
		if !ok {
			if _, err := licence.Check(k, w.trust); err != nil {
				w.t.Errorf("seed %d: %s was given a key that does not check: %v", w.seed, sale, err)
			}
		}
		if ok && o != sale {
			w.t.Errorf("seed %d: key %s went to %s and to %s", w.seed, k.Fingerprint(), o, sale)
		}
		w.owner[k] = sale
	}
}

// letter checks every letter as it goes: it holds only keys of its own
// sale, and it goes to that sale's buyer.
func (w *world) letter(to string, l Letter) {
	sale := l.Source + "/" + l.Ref
	if l.Source == "paddle" {
		sale = l.Ref
		w.shop.mu.Lock()
		buyer := w.shop.sales[l.Ref].Email
		w.shop.mu.Unlock()
		if to != buyer {
			w.t.Errorf("seed %d: the letter of %s went to %s, its buyer is %s", w.seed, l.Ref, to, buyer)
		}
	} else if to != w.partners[sale] {
		w.t.Errorf("seed %d: the letter of %s went to %s", w.seed, sale, to)
	}
	w.log("  a %s letter of %s goes to %s with %d keys", l.Kind, l.Ref, to, len(l.Keys))
	w.given(sale, l.Keys)
}

func (w *world) audit() {
	if _, err := w.engine.Audit(w.ctx()); err != nil {
		w.t.Errorf("seed %d: %v", w.seed, err)
	}
}

// calm lets everything through: no more crashes or failures, every
// webhook delivered, the pool refilled, the mail sent, a last reconcile.
// Then the rules that hold once the world is calm are checked.
func (w *world) calm() {
	w.log("the world calms down")
	w.store.before, w.store.after, w.mail.fail = 0, 0, 0
	w.shop.setDown(false)
	for range 200 {
		if len(w.pending) == 0 && len(w.hooks) == 0 {
			break
		}
		w.signer()
		if len(w.hooks) > 0 {
			h := w.hooks[0]
			h.first = w.clock.now()
			w.hooks[0] = h
			w.deliver()
		}
	}
	// Refill and catch up until nothing is missing, as we would by hand
	// after a long outage.
	for round := 0; ; round++ {
		w.signer()
		n, err := w.engine.Reconcile(w.ctx(), now.Add(-time.Hour))
		w.log("a last reconcile settles %d: %v", n, err)
		if err == nil {
			break
		}
		if round == 50 {
			w.t.Errorf("seed %d: catching up still fails after 50 refills: %v", w.seed, err)
			return
		}
	}
	for range 30 {
		w.clock.add(6 * time.Hour)
		w.engine.SendMail(w.ctx())
	}
	if w.t.Failed() {
		return
	}
	w.audit()

	revoked, err := w.engine.Revocations(w.ctx())
	if err != nil {
		w.t.Fatal(err)
	}
	want := map[licence.Fingerprint]bool{}
	for f := range w.replaced {
		want[f] = true
	}
	w.shop.mu.Lock()
	sales := maps2slice(w.shop.sales)
	w.shop.mu.Unlock()
	for _, sale := range sales {
		keys, err := w.engine.Keys(w.ctx(), "paddle", sale.Ref)
		if err != nil || len(keys) != sale.Seats {
			w.t.Errorf("seed %d: the paid sale %s has %d keys of %d once calm: %v", w.seed, sale.Ref, len(keys), sale.Seats, err)
			continue
		}
		if sale.TakenBack {
			for _, k := range keys {
				want[k.Fingerprint()] = true
			}
		}
		if !w.mailed(sale, keys) {
			w.t.Errorf("seed %d: %s never got a letter with the keys of %s", w.seed, sale.Email, sale.Ref)
		}
	}
	got := map[licence.Fingerprint]bool{}
	for _, f := range revoked {
		got[f] = true
	}
	for f := range want {
		if !got[f] && !w.restored {
			w.t.Errorf("seed %d: key %s should be revoked and is not", w.seed, f)
		}
	}
	for f := range got {
		if !want[f] {
			w.t.Errorf("seed %d: key %s is revoked and should not be", w.seed, f)
		}
	}
}

// mailed says whether the buyer of sale got a letter with exactly its keys
// as they are now, or we were told a letter of it was given up.
func (w *world) mailed(sale Sale, keys []licence.Key) bool {
	for _, warning := range w.mail.warnings {
		if strings.Contains(warning, sale.Ref+" ") {
			return true
		}
	}
	want := slices.Clone(keys)
	slices.Sort(want)
	for _, s := range w.mail.letters {
		if s.to != sale.Email || s.letter.Ref != sale.Ref {
			continue
		}
		got := slices.Clone(s.letter.Keys)
		slices.Sort(got)
		if slices.Equal(got, want) {
			return true
		}
	}
	return false
}

func maps2slice(m map[string]Sale) []Sale {
	out := make([]Sale, 0, len(m))
	for _, s := range m {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Sale) int { return strings.Compare(a.Ref, b.Ref) })
	return out
}
