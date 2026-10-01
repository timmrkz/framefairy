package dispenser

import (
	"context"
	"errors"
	"fmt"
	"time"

	"framefairy/licence"
)

// What a letter is about.
const (
	MailKeys     = "keys"     // the keys of a sale, just bought
	MailReplaced = "replaced" // a seat's new key, after the old one was posted in public
	MailResend   = "resend"   // the keys of a sale, asked for again on the lost-key page
)

// Letter is what the mailer is asked to send to a buyer.
type Letter struct {
	Kind   string
	Source string
	Ref    string
	Keys   []licence.Key
}

// Mailer sends letters. Keys sends to a buyer, Us to ourselves. An error
// means the letter may not have gone, and it is tried again.
type Mailer interface {
	Keys(ctx context.Context, to string, l Letter) error
	Us(ctx context.Context, subject, body string) error
}

// How long a letter is tried before it is given up and we are told, and
// how long a run that took a letter has before another run may take it.
const (
	GiveUpAfter = 3 * 24 * time.Hour
	lease       = 10 * time.Minute
)

// backoff is how long to wait after the nth failed try.
func backoff(tries int) time.Duration {
	steps := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}
	if tries-1 < len(steps) {
		return steps[max(tries-1, 0)]
	}
	return 6 * time.Hour
}

// queueMail queues a letter for a Paddle sale in the transaction that made
// it necessary, so a sale is never recorded without its letter.
func queueMail(tx Tx, now time.Time, source, ref, kind string) (int64, error) {
	if source != "paddle" {
		return 0, nil
	}
	return tx.AddMail(Mail{Source: source, Ref: ref, Kind: kind, Since: now, Due: now})
}

// deliver sends one queued letter, to the address given or, without one,
// to the one Paddle has for the sale. The letter is taken first, so two
// runs never send it at the same time, and taken off the queue only after
// it went. A crash between the two sends it again later: a buyer can get a
// letter twice, never no letter.
func (e *Engine) deliver(ctx context.Context, id int64, to string) error {
	now := e.now()
	var m Mail
	var keys []licence.Key
	err := e.store.Update(ctx, func(tx Tx) error {
		var err error
		m, err = tx.Mail(id)
		if err != nil {
			return err
		}
		if m.Due.After(now) {
			return errTaken
		}
		seats, err := tx.Seats(m.Source, m.Ref)
		if err != nil {
			return err
		}
		if keys, err = keysOf(tx, seats); err != nil {
			return err
		}
		if len(keys) == 0 {
			// A letter for a sale with no keys has nothing to say.
			if err := tx.RemoveMail(m.ID); err != nil {
				return err
			}
			return errTaken
		}
		taken := m
		taken.Due = now.Add(lease)
		return tx.UpdateMail(taken)
	})
	if errors.Is(err, errTaken) || errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	sendErr := e.send(ctx, to, m, keys)
	// Everything the transaction writes is worked out before it, because
	// a store may run it twice: nothing in it changes what is outside it.
	after := m
	after.Tries++
	after.Due = now.Add(backoff(after.Tries))
	givenUp := sendErr != nil && now.Sub(m.Since) >= GiveUpAfter
	if givenUp {
		// We are told before the letter leaves the queue, so a crash in
		// between tells us twice rather than never. If telling us fails,
		// the letter stays and is given up on the next run.
		if err := e.mailer.Us(ctx, "A letter was given up",
			fmt.Sprintf("The %s letter of %s %s failed for %s: %v", m.Kind, m.Source, m.Ref, now.Sub(m.Since).Round(time.Minute), sendErr)); err != nil {
			return e.store.Update(ctx, func(tx Tx) error { return tx.UpdateMail(after) })
		}
	}
	return e.store.Update(ctx, func(tx Tx) error {
		if sendErr == nil || givenUp {
			err := tx.RemoveMail(m.ID)
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			return err
		}
		return tx.UpdateMail(after)
	})
}

var errTaken = errors.New("the letter is being sent by another run")

func (e *Engine) send(ctx context.Context, to string, m Mail, keys []licence.Key) error {
	if to == "" {
		if e.orders == nil {
			return errors.New("no address, and no shop to ask for it")
		}
		sale, err := e.orders.Sale(ctx, m.Ref)
		if err != nil {
			return fmt.Errorf("asking the shop for the address: %w", err)
		}
		to = sale.Email
		if err := checkEmail(to); err != nil {
			return fmt.Errorf("the shop's address for %s: %w", m.Ref, err)
		}
	}
	return e.mailer.Keys(ctx, to, Letter{Kind: m.Kind, Source: m.Source, Ref: m.Ref, Keys: keys})
}

// SendMail sends every letter whose time has come. The daily run and a run
// every few minutes call it. It says how many went and how many failed.
func (e *Engine) SendMail(ctx context.Context) (sent, failed int, err error) {
	var due []Mail
	err = e.store.View(ctx, func(tx Tx) error {
		due, err = tx.DueMail(e.now(), 100)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	for _, m := range due {
		if err := e.deliver(ctx, m.ID, ""); err != nil {
			return sent, failed, err
		}
		left, err := e.queued(ctx, m.ID)
		if err != nil {
			return sent, failed, err
		}
		if left {
			failed++
		} else {
			sent++
		}
	}
	return sent, failed, nil
}

// queued says whether letter id is still waiting.
func (e *Engine) queued(ctx context.Context, id int64) (bool, error) {
	var left bool
	err := e.store.View(ctx, func(tx Tx) error {
		_, err := tx.Mail(id)
		left = err == nil
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	})
	return left, err
}

// Resend sends the keys of every sale an address bought to that address
// and to no other. It answers the same whether the address bought anything
// or not, so the lost-key page tells nobody who our buyers are.
func (e *Engine) Resend(ctx context.Context, email string) error {
	if err := checkEmail(email); err != nil {
		return err
	}
	if e.orders == nil {
		return errors.New("no shop to ask which sales an address bought")
	}
	refs, err := e.orders.Refs(ctx, email)
	if err != nil {
		return fmt.Errorf("asking the shop for the sales of an address: %w", err)
	}
	for _, ref := range refs {
		if checkSale("paddle", ref) != nil {
			continue
		}
		keys, err := e.Keys(ctx, "paddle", ref)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := e.mailer.Keys(ctx, email, Letter{Kind: MailResend, Source: "paddle", Ref: ref, Keys: keys}); err != nil {
			return err
		}
	}
	return nil
}

// Warning is the share of a batch below which the pool warns us.
const Warning = 0.2

// CheckPool tells us when the pool is below a fifth of a batch, at most
// once a day, every day until it is refilled. A sale and the daily run
// call it.
func (e *Engine) CheckPool(ctx context.Context) error {
	level, err := e.PoolLevel(ctx)
	if err != nil {
		return err
	}
	left, batch := level.Left, level.Batch
	if float64(left) >= Warning*float64(batch) {
		return nil
	}
	today := e.now().UTC().Format(time.DateOnly)
	var last string
	err = e.store.View(ctx, func(tx Tx) error {
		last, err = tx.Note("pool warned")
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	})
	if err != nil || last == today {
		return err
	}
	// Sent before it is noted, so a failure to send is tried again with
	// the next sale. Two sales at once may both warn, which is better than
	// neither.
	if err := e.mailer.Us(ctx, "The pool of licence keys is running low",
		fmt.Sprintf("%d keys are left, below a fifth of a batch of %d. Switch the signer on.", left, batch)); err != nil {
		return err
	}
	return e.store.Update(ctx, func(tx Tx) error { return tx.SetNote("pool warned", today) })
}
