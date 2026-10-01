package dispenser

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/licence"
)

func (f *fixture) queue() []Mail {
	f.t.Helper()
	var out []Mail
	f.store.View(f.ctx, func(tx Tx) error {
		out, _ = tx.DueMail(time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC), 1000)
		return nil
	})
	return out
}

func (f *fixture) sendMail() (int, int) {
	f.t.Helper()
	sent, failed, err := f.engine.SendMail(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return sent, failed
}

// Use case 1: the keys go to the buyer once, and the same webhook again
// sends nothing.
func TestKeysAreMailedOnce(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		keys := f.assign("txn_a", 2)
		f.assign("txn_a", 2)
		got := f.mail.sent()
		if len(got) != 1 {
			t.Fatalf("%d letters, want 1", len(got))
		}
		if got[0].to != "anna@example.com" || got[0].letter.Kind != MailKeys || !slices.Equal(got[0].letter.Keys, keys) {
			t.Fatalf("letter %+v", got[0])
		}
		if q := f.queue(); len(q) != 0 {
			t.Fatalf("%d letters still queued", len(q))
		}
	})
}

// Use case 21: the sale goes through while the mail service is down, the
// thank-you page has the keys, and the letter goes later, to the address
// Paddle has, after waiting longer each time.
func TestMailFails(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		f.shop.sold("txn_a", "anna@example.com")
		f.mail.failing(func(string, Letter) bool { return true })
		keys := f.assign("txn_a", 1)
		if got, err := f.engine.Keys(f.ctx, "paddle", "txn_a"); err != nil || got[0] != keys[0] {
			t.Fatal("the thank-you page has no keys while mail is down")
		}
		q := f.queue()
		if len(q) != 1 || q[0].Tries != 1 || !q[0].Due.Equal(now.Add(time.Minute)) {
			t.Fatalf("queue %+v", q)
		}
		// Not yet due.
		if sent, failed := f.sendMail(); sent+failed != 0 {
			t.Fatal("a letter was tried before its time")
		}
		for i, wait := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour} {
			f.clock.add(wait)
			if sent, failed := f.sendMail(); sent != 0 || failed != 1 {
				t.Fatalf("try %d: sent %d, failed %d", i+2, sent, failed)
			}
		}
		f.mail.failing(nil)
		f.clock.add(6 * time.Hour)
		if sent, _ := f.sendMail(); sent != 1 {
			t.Fatal("the letter did not go once mail was back")
		}
		got := f.mail.sent()
		if len(got) != 1 || got[0].to != "anna@example.com" || got[0].letter.Keys[0] != keys[0] {
			t.Fatalf("letters %+v", got)
		}
		if len(f.queue()) != 0 {
			t.Fatal("a sent letter is still queued")
		}
	})
}

// After three days a letter is given up and we are told. The keys are
// still on the thank-you page and the lost-key page.
func TestMailGivenUp(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(500)
		f.mail.failing(func(string, Letter) bool { return true })
		f.assign("txn_a", 1)
		for range 20 {
			f.clock.add(6 * time.Hour)
			f.sendMail()
		}
		if len(f.queue()) != 0 {
			t.Fatal("a letter is still tried after three days")
		}
		w := f.mail.warnings()
		if len(w) != 1 || !strings.Contains(w[0], "txn_a") {
			t.Fatalf("warnings %q", w)
		}
	})
}

// Without an address from the order, and with Paddle down or not knowing
// the sale, a letter waits.
func TestMailWaitsForAnAddress(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		o := order("txn_a", 1)
		o.Email = ""
		if _, err := f.engine.Assign(f.ctx, o); err != nil {
			t.Fatal(err)
		}
		if len(f.mail.sent()) != 0 || len(f.queue()) != 1 {
			t.Fatal("a letter without an address went, or was not queued")
		}
		f.shop.down = true
		f.clock.add(time.Minute)
		f.sendMail()
		f.shop.down = false
		f.shop.sold("txn_a", "bad address")
		f.clock.add(5 * time.Minute)
		f.sendMail()
		if len(f.mail.sent()) != 0 || len(f.queue()) != 1 {
			t.Fatal("a letter went to an address that is none")
		}
		f.shop.sold("txn_a", "anna@example.com")
		f.clock.add(30 * time.Minute)
		if sent, _ := f.sendMail(); sent != 1 || f.mail.sent()[0].to != "anna@example.com" {
			t.Fatal("the letter did not go once Paddle knew the address")
		}
	})
}

// A partner's buyer gets one try, and no letter waits: the partner has the
// keys in its answer.
func TestPartnerMail(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		o := Order{Source: "partner:bundle-hunt", Ref: "1001", Seats: 1, Email: "max@example.com", At: now}
		keys, err := f.engine.Assign(f.ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.mail.sent(); len(got) != 1 || got[0].to != "max@example.com" || got[0].letter.Keys[0] != keys[0] {
			t.Fatalf("letters %+v", got)
		}
		f.mail.failing(func(string, Letter) bool { return true })
		o.Ref = "1002"
		if _, err := f.engine.Assign(f.ctx, o); err != nil {
			t.Fatalf("a failed letter failed the sale: %v", err)
		}
		if len(f.queue()) != 0 {
			t.Fatal("a partner's letter was queued")
		}
		o.Ref, o.Email = "1003", ""
		f.mail.failing(nil)
		f.engine.Assign(f.ctx, o)
		if len(f.mail.sent()) != 1 {
			t.Fatal("a letter went for a partner sale without an address")
		}
	})
}

// Use case 11, the letter: the buyer gets the new key, at the address
// Paddle has.
func TestReplacedKeyIsMailed(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		f.shop.sold("txn_a", "anna@example.com")
		keys := f.assign("txn_a", 2)
		next, _, err := f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted")
		if err != nil {
			t.Fatal(err)
		}
		got := f.mail.sent()
		if len(got) != 2 {
			t.Fatalf("%d letters, want the keys and the replacement", len(got))
		}
		l := got[1]
		if l.to != "anna@example.com" || l.letter.Kind != MailReplaced || l.letter.Keys[0] != next || l.letter.Keys[1] != keys[1] {
			t.Fatalf("letter %+v", l)
		}
	})
}

// Use case 7: the keys go to that address only, the answer is the same
// whether it bought anything or not, and an address that is no address is
// refused.
func TestLostKey(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(10)
		f.shop.sold("txn_a", "anna@example.com")
		f.shop.sold("txn_b", "anna@example.com")
		f.shop.sold("txn_c", "max@example.com")
		f.shop.sold("txn_unassigned", "anna@example.com")
		f.shop.sold("bad ref", "anna@example.com")
		a := f.assign("txn_a", 1)
		b := f.assign("txn_b", 2)
		f.assign("txn_c", 1)
		before := len(f.mail.sent())

		if err := f.engine.Resend(f.ctx, "anna@example.com"); err != nil {
			t.Fatal(err)
		}
		got := f.mail.sent()[before:]
		if len(got) != 2 {
			t.Fatalf("%d letters, want one per sale of hers", len(got))
		}
		var keys []licence.Key
		for _, s := range got {
			if s.to != "anna@example.com" || s.letter.Kind != MailResend {
				t.Fatalf("letter %+v", s)
			}
			keys = append(keys, s.letter.Keys...)
		}
		want := append(append([]licence.Key{}, a...), b...)
		slices.Sort(keys)
		slices.Sort(want)
		if !slices.Equal(keys, want) {
			t.Fatal("the letters do not hold exactly her keys")
		}

		before = len(f.mail.sent())
		if err := f.engine.Resend(f.ctx, "nobody@example.com"); err != nil {
			t.Fatalf("an address that bought nothing: %v", err)
		}
		if len(f.mail.sent()) != before {
			t.Fatal("a letter went to an address that bought nothing")
		}
		for _, bad := range []string{"", "anna", "anna@example.com\nBcc: eve@example.com", "anna@example.com, eve@example.com"} {
			if err := f.engine.Resend(f.ctx, bad); !errors.Is(err, ErrInvalid) {
				t.Errorf("%q: got %v, want ErrInvalid", bad, err)
			}
		}
		f.shop.down = true
		if err := f.engine.Resend(f.ctx, "anna@example.com"); err == nil {
			t.Fatal("Paddle down, and Resend said it sent")
		}
	})
}

// Use case 13: below a fifth of a batch, one warning a day.
func TestPoolWarning(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(202)
		f.assign("txn_1", 2)
		if len(f.mail.warnings()) != 0 {
			t.Fatal("warned with 200 of 1000 left")
		}
		f.assign("txn_2", 1)
		f.assign("txn_3", 1)
		if w := f.mail.warnings(); len(w) != 1 || !strings.Contains(w[0], "199 keys are left") {
			t.Fatalf("warnings %q", w)
		}
		// The clock stands at 14:05 UTC: an hour later is the same day,
		// eleven hours later the next.
		f.clock.add(time.Hour)
		f.assign("txn_4", 1)
		if len(f.mail.warnings()) != 1 {
			t.Fatal("warned twice on one day")
		}
		f.clock.add(10 * time.Hour)
		if err := f.engine.CheckPool(f.ctx); err != nil {
			t.Fatal(err)
		}
		if len(f.mail.warnings()) != 2 {
			t.Fatal("not warned again the next day")
		}
		f.stock(1000)
		f.clock.add(24 * time.Hour)
		f.engine.CheckPool(f.ctx)
		if len(f.mail.warnings()) != 2 {
			t.Fatal("warned after the pool was refilled")
		}
	})
}

// Several runs of SendMail at once, as overlapping triggers would make
// them, with the mail service failing now and then: every letter goes
// exactly once.
func TestSendMailRunsAtOnce(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(100)
		var mu sync.Mutex
		n := 0
		f.mail.failing(func(string, Letter) bool {
			mu.Lock()
			defer mu.Unlock()
			n++
			return n%3 != 0
		})
		for i := range 30 {
			ref := fmt.Sprintf("txn_%d", i)
			f.shop.sold(ref, ref+"@example.com")
			o := order(ref, 1)
			o.Email = ref + "@example.com"
			if _, err := f.engine.Assign(f.ctx, o); err != nil {
				t.Fatal(err)
			}
		}
		for range 10 {
			f.clock.add(lease + 6*time.Hour)
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					if _, _, err := f.engine.SendMail(f.ctx); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
		}
		got := map[string]int{}
		for _, s := range f.mail.sent() {
			got[s.letter.Ref]++
			if s.to != s.letter.Ref+"@example.com" {
				t.Fatalf("%s's letter went to %s", s.letter.Ref, s.to)
			}
		}
		for i := range 30 {
			if c := got[fmt.Sprintf("txn_%d", i)]; c != 1 {
				t.Fatalf("txn_%d got %d letters", i, c)
			}
		}
		if len(f.queue()) != 0 {
			t.Fatal("letters left over")
		}
	})
}
