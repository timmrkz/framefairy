package dispenser

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sale is what the shop says about one of its sales now.
type Sale struct {
	Ref       string
	Seats     int
	Email     string
	At        time.Time
	TakenBack bool // charged back or refunded, and not reversed since
}

// Orders is what the dispenser asks the shop. It keeps none of it.
type Orders interface {
	// Sale is a completed Paddle sale as it stands now. ErrNotFound when
	// Paddle has no completed sale by that reference.
	Sale(ctx context.Context, ref string) (Sale, error)
	// Since are the references of every sale completed or adjusted at
	// or after t.
	Since(ctx context.Context, t time.Time) ([]string, error)
	// Refs are the Paddle sales an address has bought.
	Refs(ctx context.Context, email string) ([]string, error)
}

// Settle makes a Paddle sale what Paddle says it is now: its keys
// assigned, and revoked exactly when Paddle has taken the money back. It
// asks Paddle rather than trusting a webhook, so adjustments that arrive
// late, twice or in the wrong order all end in the same place: a
// chargeback and its reversal leave the keys working whichever webhook
// comes first. Every adjustment webhook calls it, and so does Reconcile.
//
// Keys revoked because they were posted in public are not Paddle's
// business and stay revoked.
func (e *Engine) Settle(ctx context.Context, ref string) error {
	if e.orders == nil {
		return errors.New("no shop to ask what a sale is")
	}
	if err := checkSale("paddle", ref); err != nil {
		return err
	}
	s, err := e.orders.Sale(ctx, ref)
	if errors.Is(err, ErrNotFound) {
		// Not a completed sale, or not yet: Paddle's answers can trail its
		// webhooks. The webhook comes again, and the daily run catches up.
		return fmt.Errorf("%w: Paddle has no completed sale %s", ErrNotFound, ref)
	}
	if err != nil {
		return fmt.Errorf("asking the shop about %s: %w", ref, err)
	}
	if s.Ref != ref {
		return fmt.Errorf("asked the shop about %s and it answered about %q", ref, s.Ref)
	}
	if _, err := e.Assign(ctx, Order{Source: "paddle", Ref: ref, Seats: s.Seats, Email: s.Email, At: s.At}); err != nil {
		return err
	}
	if s.TakenBack {
		_, err = e.Revoke(ctx, Target{Source: "paddle", Ref: ref}, "taken back at Paddle")
	} else {
		_, err = e.Restore(ctx, Target{Source: "paddle", Ref: ref}, "not taken back at Paddle")
	}
	return err
}

// Reconcile settles every sale Paddle completed or adjusted since t, so
// whatever never arrived as a webhook, or arrived when the dispenser could
// not take it, is caught up. It runs every day over the last days, and
// after a restore from the backup's time. It goes on past a sale that
// fails, and says how many it settled and what failed.
func (e *Engine) Reconcile(ctx context.Context, since time.Time) (int, error) {
	if e.orders == nil {
		return 0, errors.New("no shop to reconcile with")
	}
	refs, err := e.orders.Since(ctx, since)
	if err != nil {
		return 0, fmt.Errorf("asking the shop for its sales since %s: %w", since.Format(time.RFC3339), err)
	}
	var errs []error
	n := 0
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if err := e.Settle(ctx, ref); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ref, err))
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
