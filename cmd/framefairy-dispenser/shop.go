package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"framefairy/licence/dispenser"
	"framefairy/licence/paddle"
)

// How the shop sends the next webhooks, chosen on the dev page.
const (
	sendNow   = "now"   // at once, as Paddle does
	sendTwice = "twice" // twice, as Paddle does when an answer got lost
	sendHold  = "hold"  // kept until released by hand, to send them in another order
	sendLose  = "lose"  // never, so only the daily run can find the sale
)

func sendMode(s string) bool { return s == sendNow || s == sendTwice || s == sendHold || s == sendLose }

// Where a webhook is.
const (
	whPending   = "pending"
	whHeld      = "held"
	whLost      = "lost"
	whDelivered = "delivered"
	whFailed    = "failed" // every try failed
)

// retries is when the shop tries a webhook again after a failed try, from
// the first: Paddle's sandbox tries three times in fifteen minutes.
var retries = []time.Duration{time.Minute, 4 * time.Minute, 10 * time.Minute}

// apiLag is how long Paddle's API may not know a sale its webhook already
// announced, when the dev page says it lags.
const apiLag = 2 * time.Minute

// shop is a pretend Paddle. It sells, takes money back, sends signed
// webhooks with Paddle's names and retries, and answers what the
// dispenser asks it. It knows nothing of the dispenser's rules: what it
// says about a sale goes through licence/paddle, as Paddle's own answers
// will.
type shop struct {
	sending sync.Mutex // one Deliver at a time, so webhooks go in order
	mu      sync.Mutex
	clock   *clock
	secret  string
	target  string // where webhooks go
	client  *http.Client

	sales  map[string]*sale
	order  []string // sales, newest last
	events []*webhook

	api  string // works or down
	lag  bool
	mode string
}

type sale struct {
	Ref   string
	Email string
	Seats int
	At    time.Time
	Adjs  []paddle.Adjustment
}

// TakenBack is what licence/paddle reads from the sale's adjustments.
func (s *sale) TakenBack() bool { return paddle.TakenBack(s.Adjs) }

type webhook struct {
	ID    string
	Type  string
	Ref   string
	Body  []byte
	State string
	Next  time.Time
	Tries []try
}

type try struct {
	At     time.Time
	Code   int
	Answer string
}

func newShop(c *clock, secret, target string) *shop {
	return &shop{
		clock: c, secret: secret, target: target,
		client: &http.Client{Timeout: 10 * time.Second},
		sales:  map[string]*sale{},
		api:    works, mode: sendNow,
	}
}

// paddleID is an ID in Paddle's shape: a prefix and 26 lowercase letters
// and digits.
func paddleID(prefix string) string {
	const chars = "0123456789abcdefghjkmnpqrstvwxyz"
	b := make([]byte, 26)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return prefix + "_" + string(b)
}

// Buy is a checkout that completed.
func (s *shop) Buy(email string, seats int) (string, error) {
	if seats < 1 || seats > dispenser.MaxSeats {
		return "", fmt.Errorf("1 to %d seats", dispenser.MaxSeats)
	}
	if !strings.Contains(email, "@") {
		return "", errors.New("an email address")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	ref := paddleID("txn")
	s.sales[ref] = &sale{Ref: ref, Email: email, Seats: seats, At: now}
	s.order = append(s.order, ref)
	s.queueLocked("transaction.completed", ref, map[string]any{
		"id": ref, "status": "completed", "origin": "web",
		"created_at": now, "updated_at": now, "billed_at": now,
		"items": []map[string]any{{"quantity": seats}},
	})
	return ref, nil
}

// Adjust adds an adjustment to a sale and sends adjustment.created.
func (s *shop) Adjust(ref, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, ok := s.sales[ref]
	if !ok {
		return errors.New("no such sale")
	}
	status := paddle.Approved
	switch action {
	case paddle.Refund:
		// A live account's refund waits for Paddle's approval.
		status = paddle.PendingApproval
	case paddle.Chargeback, paddle.ChargebackWarning:
	case paddle.ChargebackReverse:
		// Paddle reverses a standing chargeback: the reversal is an
		// adjustment of its own, and the chargeback turns reversed.
		i := slices.IndexFunc(sl.Adjs, func(a paddle.Adjustment) bool {
			return a.Action == paddle.Chargeback && a.Status == paddle.Approved
		})
		if i < 0 {
			return errors.New("no chargeback to reverse")
		}
		s.addLocked(sl, action, status)
		return s.setLocked(sl, i, paddle.Reversed)
	default:
		return fmt.Errorf("adjustment %q", action)
	}
	s.addLocked(sl, action, status)
	return nil
}

func (s *shop) addLocked(sl *sale, action, status string) {
	now := s.clock.Now()
	a := paddle.Adjustment{ID: paddleID("adj"), TransactionID: sl.Ref, Action: action, Status: status, At: now}
	sl.Adjs = append(sl.Adjs, a)
	s.queueLocked("adjustment.created", sl.Ref, adjustmentData(a, now))
}

// Decide approves or rejects a refund waiting for approval, and sends
// adjustment.updated.
func (s *shop) Decide(ref, id string, approve bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sl, ok := s.sales[ref]
	if !ok {
		return errors.New("no such sale")
	}
	i := slices.IndexFunc(sl.Adjs, func(a paddle.Adjustment) bool { return a.ID == id })
	if i < 0 || sl.Adjs[i].Status != paddle.PendingApproval {
		return errors.New("no refund waiting for approval")
	}
	status := paddle.Rejected
	if approve {
		status = paddle.Approved
	}
	return s.setLocked(sl, i, status)
}

func (s *shop) setLocked(sl *sale, i int, status string) error {
	sl.Adjs[i].Status = status
	s.queueLocked("adjustment.updated", sl.Ref, adjustmentData(sl.Adjs[i], s.clock.Now()))
	return nil
}

func adjustmentData(a paddle.Adjustment, updated time.Time) map[string]any {
	return map[string]any{
		"id": a.ID, "transaction_id": a.TransactionID, "action": a.Action,
		"status": a.Status, "created_at": a.At, "updated_at": updated,
	}
}

// Replay sends a sale's transaction.completed again, as Paddle's replay
// does.
func (s *shop) Replay(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.events, func(w *webhook) bool { return w.Ref == ref && w.Type == "transaction.completed" })
	if i < 0 {
		return errors.New("no such sale")
	}
	w := *s.events[i]
	w.State, w.Next, w.Tries = whPending, s.clock.Now(), nil
	s.events = append(s.events, &w)
	return nil
}

// Release sends a held webhook.
func (s *shop) Release(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.events {
		if w.ID == id && w.State == whHeld {
			w.State, w.Next = whPending, s.clock.Now()
			return nil
		}
	}
	return errors.New("no such held webhook")
}

// queueLocked makes a webhook the way Paddle writes one, and queues it as
// the dev page says.
func (s *shop) queueLocked(typ, ref string, data map[string]any) {
	now := s.clock.Now()
	body, _ := json.Marshal(map[string]any{
		"event_id": paddleID("evt"), "event_type": typ, "occurred_at": now,
		"notification_id": paddleID("ntf"), "data": data,
	})
	w := &webhook{ID: paddleID("ntf"), Type: typ, Ref: ref, Body: body, State: whPending, Next: now}
	switch s.mode {
	case sendHold:
		w.State = whHeld
	case sendLose:
		w.State = whLost
	}
	s.events = append(s.events, w)
	if s.mode == sendTwice {
		again := *w
		again.ID = paddleID("ntf")
		s.events = append(s.events, &again)
	}
}

// sign is the Paddle-Signature header for body at t. It is written from
// Paddle's docs and not taken from the dispenser, so a mistake in one is
// not hidden by the same mistake in the other.
func sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + ":"))
	mac.Write(body)
	return "ts=" + ts + ";h1=" + hex.EncodeToString(mac.Sum(nil))
}

// Deliver sends every webhook whose time has come, one at a time and in
// the order they were made, and says how many it sent. A second Deliver
// waits for the first, so when it returns, nothing due is still in
// flight.
func (s *shop) Deliver(ctx context.Context) int {
	s.sending.Lock()
	defer s.sending.Unlock()
	n := 0
	for {
		w, body := s.due()
		if w == nil {
			return n
		}
		n++
		code, answer := s.post(ctx, body)
		s.mu.Lock()
		now := s.clock.Now()
		w.Tries = append(w.Tries, try{At: now, Code: code, Answer: answer})
		switch {
		case code == http.StatusOK:
			w.State = whDelivered
		case len(w.Tries) > len(retries):
			w.State = whFailed
		default:
			w.Next = now.Add(retries[len(w.Tries)-1])
		}
		s.mu.Unlock()
	}
}

// due takes the next webhook to send, or nil. A webhook is never sent
// twice at once: it stays pending, and its next time moves past now
// until its try is written down.
func (s *shop) due() (*webhook, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	for _, w := range s.events {
		if w.State == whPending && !w.Next.After(now) {
			w.Next = now.Add(time.Hour)
			return w, w.Body
		}
	}
	return nil, nil
}

func (s *shop) post(ctx context.Context, body []byte) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.target, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Paddle")
	s.mu.Lock()
	req.Header.Set("Paddle-Signature", sign(s.secret, s.clock.Now(), body))
	s.mu.Unlock()
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
	return resp.StatusCode, strings.TrimSpace(string(answer))
}

// The dispenser's questions, as Paddle's API answers them.

var errAPIDown = errors.New("the pretend Paddle API is down")

func (s *shop) Sale(ctx context.Context, ref string) (dispenser.Sale, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api == down {
		return dispenser.Sale{}, errAPIDown
	}
	sl, ok := s.sales[ref]
	if !ok || (s.lag && s.clock.Now().Sub(sl.At) < apiLag) {
		return dispenser.Sale{}, dispenser.ErrNotFound
	}
	return dispenser.Sale{Ref: sl.Ref, Seats: sl.Seats, Email: sl.Email, At: sl.At, TakenBack: sl.TakenBack()}, nil
}

func (s *shop) Since(ctx context.Context, t time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api == down {
		return nil, errAPIDown
	}
	var refs []string
	for _, ref := range s.order {
		sl := s.sales[ref]
		changed := !sl.At.Before(t)
		for _, a := range sl.Adjs {
			changed = changed || !a.At.Before(t)
		}
		if changed {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

func (s *shop) Refs(ctx context.Context, email string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.api == down {
		return nil, errAPIDown
	}
	var refs []string
	for _, ref := range s.order {
		if strings.EqualFold(s.sales[ref].Email, email) {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}

// view is the shop for the dev page, copied so it can be read unlocked.
type shopView struct {
	API, Mode string
	Lag       bool
	Sales     []sale
	Webhooks  []webhook
}

func (s *shop) view() shopView {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := shopView{API: s.api, Mode: s.mode, Lag: s.lag}
	for i := len(s.order) - 1; i >= 0; i-- {
		sl := *s.sales[s.order[i]]
		sl.Adjs = slices.Clone(sl.Adjs)
		v.Sales = append(v.Sales, sl)
	}
	for i := len(s.events) - 1; i >= 0; i-- {
		w := *s.events[i]
		w.Tries = slices.Clone(w.Tries)
		v.Webhooks = append(v.Webhooks, w)
	}
	return v
}

func (s *shop) set(api, mode string, lag bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.api, s.mode, s.lag = api, mode, lag
}
