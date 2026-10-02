package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"framefairy/licence"
	"framefairy/licence/dispenser"
)

// clock is the time of the pretend world. It runs with the real one and
// can be moved forward, never back, so a day can pass in a click.
type clock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().UTC().Add(c.offset)
}

func (c *clock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	c.offset += d
	c.mu.Unlock()
}

func (c *clock) Ahead() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offset
}

// How a pretend service behaves. Works, is down before it does anything,
// or does its work and then fails to say so, which is the hardest of the
// three to get right.
const (
	works     = "works"
	down      = "down"
	losesWord = "loses-answer"
)

func behaviour(s string) bool { return s == works || s == down || s == losesWord }

// letter is one mail the outbox took.
type letter struct {
	At     time.Time
	To     string
	Kind   string
	Ref    string
	Keys   []licence.Key
	Failed bool // sent, but the service said it failed
}

// note is one mail to us.
type note struct {
	At            time.Time
	Subject, Body string
}

// outbox is the mail service. It sends nothing: every letter is kept here
// to be read on the console.
type outbox struct {
	mu      sync.Mutex
	clock   *clock
	mode    string
	letters []letter
	notes   []note
}

var errMailDown = errors.New("the pretend mail service is down")

func (o *outbox) Keys(ctx context.Context, to string, l dispenser.Letter) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.mode == down {
		return errMailDown
	}
	lost := o.mode == losesWord
	o.letters = append(o.letters, letter{At: o.clock.Now(), To: to, Kind: l.Kind, Ref: l.Ref, Keys: l.Keys, Failed: lost})
	if lost {
		return errors.New("the pretend mail service sent the letter and timed out")
	}
	return nil
}

func (o *outbox) Us(ctx context.Context, subject, body string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.mode == down {
		return errMailDown
	}
	o.notes = append(o.notes, note{At: o.clock.Now(), Subject: subject, Body: body})
	if o.mode == losesWord {
		return errors.New("the pretend mail service sent the note and timed out")
	}
	return nil
}

func (o *outbox) set(mode string) {
	o.mu.Lock()
	o.mode = mode
	o.mu.Unlock()
}

func (o *outbox) read() (mode string, letters []letter, notes []note) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.mode, append([]letter(nil), o.letters...), append([]note(nil), o.notes...)
}

// database is the dispenser's store with a switch: it works, is down, or
// commits and loses the answer, as a database whose connection drops at
// the wrong moment does.
type database struct {
	*dispenser.Memory
	mu   sync.Mutex
	mode string
}

var errDatabaseDown = errors.New("the pretend database is down")

func (d *database) state() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.mode
}

func (d *database) set(mode string) {
	d.mu.Lock()
	d.mode = mode
	d.mu.Unlock()
}

func (d *database) Update(ctx context.Context, fn func(dispenser.Tx) error) error {
	mode := d.state()
	if mode == down {
		return errDatabaseDown
	}
	err := d.Memory.Update(ctx, fn)
	if err == nil && mode == losesWord {
		return errors.New("the pretend database committed and lost the answer")
	}
	return err
}

func (d *database) View(ctx context.Context, fn func(dispenser.Tx) error) error {
	if d.state() == down {
		return errDatabaseDown
	}
	return d.Memory.View(ctx, fn)
}
