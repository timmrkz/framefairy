package dispenser

import (
	"slices"
	"testing"
	"time"
)

// A chargeback and its reversal, whichever webhook arrives first and
// however often, end with the keys working.
func TestSettleIsOrderFree(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		sale := Sale{Ref: "txn_a", Seats: 2, Email: "anna@example.com", At: now}
		f.shop.put(sale, now)
		// The reversal's webhook arrives first, when Paddle already says
		// the money is back with us.
		if err := f.engine.Settle(f.ctx, "txn_a"); err != nil {
			t.Fatal(err)
		}
		keys, _ := f.engine.Keys(f.ctx, "paddle", "txn_a")
		if len(keys) != 2 {
			t.Fatal("settling a sale did not assign it")
		}
		// Then the chargeback's webhook, late: Paddle still says reversed.
		if err := f.engine.Settle(f.ctx, "txn_a"); err != nil || len(f.revoked()) != 0 {
			t.Fatalf("a late chargeback webhook revoked a reversed sale: %v", err)
		}
		sale.TakenBack = true
		f.shop.put(sale, now)
		for range 3 {
			if err := f.engine.Settle(f.ctx, "txn_a"); err != nil {
				t.Fatal(err)
			}
		}
		if !slices.Equal(f.revoked(), fingerprints(keys)) {
			t.Fatal("a chargeback did not revoke the sale")
		}
		sale.TakenBack = false
		f.shop.put(sale, now)
		f.engine.Settle(f.ctx, "txn_a")
		if len(f.revoked()) != 0 {
			t.Fatal("a reversal did not restore the sale")
		}
		if len(f.mail.sent()) != 1 {
			t.Fatalf("%d letters, want the one for the sale", len(f.mail.sent()))
		}
		f.audit()
	})
}

// A key posted in public stays revoked, whatever Paddle says about its
// sale.
func TestSettleLeavesReplacedKeys(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(5)
		f.shop.put(Sale{Ref: "txn_a", Seats: 1, Email: "anna@example.com", At: now}, now)
		f.engine.Settle(f.ctx, "txn_a")
		keys, _ := f.engine.Keys(f.ctx, "paddle", "txn_a")
		f.engine.Replace(f.ctx, keys[0].Fingerprint(), "posted")
		if err := f.engine.Settle(f.ctx, "txn_a"); err != nil {
			t.Fatal(err)
		}
		if got := f.revoked(); len(got) != 1 || got[0] != keys[0].Fingerprint() {
			t.Fatal("settling lifted a replaced key")
		}
		f.audit()
	})
}

func TestSettleRefuses(t *testing.T) {
	f := newFixture(t, false)
	f.stock(5)
	if err := f.engine.Settle(f.ctx, "txn_unknown"); err != nil {
		t.Fatalf("a sale Paddle does not know: %v", err)
	}
	if err := f.engine.Settle(f.ctx, "txn bad"); err == nil {
		t.Fatal("settled a reference that is no reference")
	}
	f.shop.put(Sale{Ref: "txn_a", Seats: 1, Email: "anna@example.com", At: now}, now)
	f.shop.setDown(true)
	if err := f.engine.Settle(f.ctx, "txn_a"); err == nil {
		t.Fatal("Paddle down, and Settle said it settled")
	}
	f.shop.setDown(false)
	// Paddle answers about another sale than the one asked about.
	f.shop.mu.Lock()
	f.shop.sales["txn_b"] = Sale{Ref: "txn_a", Seats: 1, At: now}
	f.shop.mu.Unlock()
	if err := f.engine.Settle(f.ctx, "txn_b"); err == nil {
		t.Fatal("settled txn_b with Paddle's answer about txn_a")
	}
	if f.left() != 5 {
		t.Fatal("a refused settle assigned keys")
	}
}

// Use case 19: what never arrived as a webhook is caught up, and a sale
// that cannot be settled does not stop the rest.
func TestReconcile(t *testing.T) {
	stores(t, func(t *testing.T, f *fixture) {
		f.stock(3)
		day := 24 * time.Hour
		f.shop.put(Sale{Ref: "txn_old", Seats: 1, Email: "a@example.com", At: now.Add(-10 * day)}, now.Add(-10*day))
		f.shop.put(Sale{Ref: "txn_a", Seats: 1, Email: "a@example.com", At: now}, now)
		f.shop.put(Sale{Ref: "txn_big", Seats: 5, Email: "b@example.com", At: now}, now)
		f.shop.put(Sale{Ref: "txn_c", Seats: 1, Email: "c@example.com", At: now, TakenBack: true}, now)
		n, err := f.engine.Reconcile(f.ctx, now.Add(-3*day))
		if n != 2 || err == nil {
			t.Fatalf("settled %d, %v. Want 2, and the one too big for the pool failing", n, err)
		}
		if _, err := f.engine.Keys(f.ctx, "paddle", "txn_old"); err == nil {
			t.Fatal("reconciled a sale older than asked")
		}
		if len(f.revoked()) != 1 {
			t.Fatal("a sale taken back was not revoked when caught up")
		}
		f.stock(5)
		if n, err := f.engine.Reconcile(f.ctx, now.Add(-3*day)); n != 3 || err != nil {
			t.Fatalf("after a refill: %d, %v", n, err)
		}
		f.audit()
	})
}
