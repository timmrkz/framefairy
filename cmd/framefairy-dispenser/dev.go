package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"framefairy/licence"
	"framefairy/licence/dispenser"
	"framefairy/licence/signer"
)

// The pretend partner, whose orders the dev page makes through the
// partner API.
const partnerName = "bundle-hunt"

// dev is the dispenser with a pretend world around it: Paddle, the mail
// service, the database, the signer, the scheduled runs and the clock.
// Everything the dev page does goes through the dispenser's own endpoints,
// with the tokens a real caller would have.
type dev struct {
	base   string // where it is served, http://host:port
	clock  *clock
	db     *database
	mail   *outbox
	shop   *shop
	signer *signer.Signer
	engine *dispenser.Engine
	api    http.Handler
	log    *ring
	client *http.Client
	tokens map[string]string // by caller: signer, release, cron, admin, partner

	mu        sync.Mutex
	refill    bool // the signer refills the pool by itself
	schedule  bool // the mail and daily runs go by themselves
	lastMail  time.Time
	lastDaily time.Time
	answer    *answer // the last thing the dev page did, to show
}

// answer is what one call to the dispenser answered, for the dev page.
type answer struct {
	What string
	Code int
	Body string
}

// newDev builds the world. dir holds the test signer's record and is the
// caller's to remove.
func newDev(base, dir string, batch int, logTo io.Writer) (*dev, error) {
	d := &dev{
		base: strings.TrimSuffix(base, "/"), clock: &clock{},
		client: &http.Client{Timeout: 30 * time.Second},
		tokens: map[string]string{}, refill: true, schedule: true,
	}
	d.log = newRing(200, logTo)
	d.db = &database{Memory: &dispenser.Memory{}, mode: works}
	d.mail = &outbox{clock: d.clock, mode: works}
	secret := randomHex(32)
	d.shop = newShop(d.clock, secret, d.base+"/paddle")

	record, err := signer.OpenRecord(filepath.Join(dir, "record.jsonl"))
	if err != nil {
		return nil, err
	}
	key := ed25519.NewKeyFromSeed(signer.TestSeed())
	if d.signer, err = signer.New(0, key, record, signer.WithClock(d.clock.Now)); err != nil {
		return nil, err
	}
	d.engine, err = dispenser.New(d.db, dispenser.Config{
		Signers: map[uint8]ed25519.PublicKey{0: d.signer.Public()},
		Batch:   batch, Now: d.clock.Now, Mailer: d.mail, Orders: d.shop,
	})
	if err != nil {
		return nil, err
	}
	for _, who := range []string{"signer", "release", "cron", "admin", "partner"} {
		d.tokens[who] = randomHex(16)
	}
	d.api, err = d.engine.Handler(dispenser.APIConfig{
		PaddleSecrets: []string{secret},
		Partners:      map[string]string{dispenser.TokenHash(d.tokens["partner"]): partnerName},
		Signer:        dispenser.TokenHash(d.tokens["signer"]),
		Release:       dispenser.TokenHash(d.tokens["release"]),
		Cron:          dispenser.TokenHash(d.tokens["cron"]),
		Admin:         dispenser.TokenHash(d.tokens["admin"]),
		Website:       d.base,
		Leaked:        []uint8{0},
		Log:           slog.New(d.log),
	})
	if err != nil {
		return nil, err
	}
	d.lastMail, d.lastDaily = d.clock.Now(), d.clock.Now()
	return d, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// call sends a request to the dispenser as a caller, and keeps the answer
// for the dev page.
func (d *dev) call(ctx context.Context, who, method, path string, body any) (answer, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return answer{}, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, r)
	if err != nil {
		return answer{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t := d.tokens[who]; t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return answer{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return answer{}, err
	}
	return answer{What: method + " " + path, Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}, nil
}

// show keeps what an action on the dev page answered.
func (d *dev) show(a answer) {
	d.mu.Lock()
	d.answer = &a
	d.mu.Unlock()
}

// Refill is the signer: when the pool is below one batch, it signs a
// batch of the generation the pool asks for and hands it over, as the
// real signer will.
func (d *dev) Refill(ctx context.Context) (answer, error) {
	a, err := d.call(ctx, "signer", http.MethodGet, "/v1/pool", nil)
	if err != nil || a.Code != http.StatusOK {
		return a, err
	}
	var level struct {
		Left       int    `json:"left"`
		Batch      int    `json:"batch"`
		Generation string `json:"generation"`
	}
	if err := json.Unmarshal([]byte(a.Body), &level); err != nil {
		return a, err
	}
	if level.Left >= level.Batch {
		return a, nil
	}
	return d.Sign(ctx, level.Batch, level.Generation)
}

// Sign signs n keys and hands them over for generation.
func (d *dev) Sign(ctx context.Context, n int, generation string) (answer, error) {
	keys, err := d.signer.Pool(n, 0)
	if err != nil {
		return answer{}, err
	}
	return d.call(ctx, "signer", http.MethodPost, "/v1/pool", map[string]any{"generation": generation, "keys": keys})
}

// Tick is what happens by itself as time goes by: webhooks go out and
// come back, the signer refills the pool, and the scheduled runs run when
// their time on the pretend clock has come.
func (d *dev) Tick(ctx context.Context) {
	d.shop.Deliver(ctx)
	d.mu.Lock()
	refill, schedule := d.refill, d.schedule
	now := d.clock.Now()
	runMail := schedule && now.Sub(d.lastMail) >= 5*time.Minute
	runDaily := schedule && now.Sub(d.lastDaily) >= 24*time.Hour
	if runMail {
		d.lastMail = now
	}
	if runDaily {
		d.lastDaily = now
	}
	d.mu.Unlock()
	if refill {
		_, _ = d.Refill(ctx)
	}
	if runMail {
		_, _ = d.call(ctx, "cron", http.MethodPost, "/v1/cron/mail", nil)
	}
	if runDaily {
		_, _ = d.call(ctx, "cron", http.MethodPost, "/v1/cron/daily", nil)
	}
}

// run ticks every second until ctx ends.
func (d *dev) run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.Tick(ctx)
		}
	}
}

// keysOf is what the dispenser holds for a sale, read straight from the
// engine, with whether each key is on the revocation list.
type heldKey struct {
	Key         licence.Key
	Fingerprint string
	Revoked     bool
}

func (d *dev) keysOf(ctx context.Context, source, ref string, revoked map[string]bool) ([]heldKey, error) {
	keys, err := d.engine.Keys(ctx, source, ref)
	if errors.Is(err, dispenser.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]heldKey, len(keys))
	for i, k := range keys {
		f := k.Fingerprint().String()
		out[i] = heldKey{Key: k, Fingerprint: f, Revoked: revoked[f]}
	}
	return out, nil
}

// ring keeps the last log lines for the dev page and writes them on.
type ring struct {
	lines *lines
	attrs []slog.Attr
	next  slog.Handler
}

type lines struct {
	mu   sync.Mutex
	size int
	all  []logLine
}

type logLine struct {
	At    time.Time
	Level string
	Text  string
}

func newRing(size int, w io.Writer) *ring {
	if w == nil {
		w = io.Discard
	}
	return &ring{lines: &lines{size: size}, next: slog.NewTextHandler(w, nil)}
}

func (r *ring) Enabled(ctx context.Context, l slog.Level) bool { return true }

func (r *ring) Handle(ctx context.Context, rec slog.Record) error {
	var b strings.Builder
	b.WriteString(rec.Message)
	add := func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	}
	for _, a := range r.attrs {
		add(a)
	}
	rec.Attrs(add)
	l := r.lines
	l.mu.Lock()
	l.all = append(l.all, logLine{At: rec.Time, Level: rec.Level.String(), Text: b.String()})
	if len(l.all) > l.size {
		l.all = l.all[len(l.all)-l.size:]
	}
	l.mu.Unlock()
	return r.next.Handle(ctx, rec)
}

func (r *ring) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ring{lines: r.lines, attrs: append(slices.Clip(r.attrs), attrs...), next: r.next.WithAttrs(attrs)}
}

func (r *ring) WithGroup(name string) slog.Handler { return r }

// read is the log, newest first.
func (r *ring) read() []logLine {
	l := r.lines
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]logLine, len(l.all))
	for i, line := range l.all {
		out[len(out)-1-i] = line
	}
	return out
}

// stderr is where the log goes as well, so the terminal shows it.
var stderr io.Writer = os.Stderr
