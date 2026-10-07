package dispenser

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"framefairy/licence"
)

// APIConfig is how the web endpoints are set up. Tokens are never kept as
// they are: each is the SHA-256 of the token in hexadecimal, so the
// dispenser's settings hold nothing a caller could use.
type APIConfig struct {
	// PaddleSecrets are the secret keys Paddle signs its webhooks with.
	// Two while one replaces the other.
	PaddleSecrets []string
	// Partners maps a partner's token hash to its name, lowercase letters,
	// digits and dashes.
	Partners map[string]string
	// Signer, Release, Cron and Admin are the token hashes of the signer,
	// the release workflow, the scheduled runs and us.
	Signer, Release, Cron, Admin string
	// Website is the origin of our website, the only one whose pages may
	// call the thank-you and lost-key endpoints.
	Website string
	// Leaked are the signers whose genuine lists the feed carries.
	Leaked []uint8
	// ClientIP says which address a request came from, for the lost-key
	// page's limits. Without it, the address the connection came from.
	ClientIP func(*http.Request) string
	// Log is where requests are logged, without their bodies and without
	// any address of a buyer. Without it, nothing is logged.
	Log *slog.Logger
}

// How long the thank-you page shows a sale's keys after the sale. After
// that, the lost-key page sends them to the buyer's address instead, so a
// reference that got out shows nothing.
const ThanksWindow = 24 * time.Hour

// Limits on what one request may carry.
const (
	maxBody      = 16 << 10 // a request of ours
	maxPaddle    = 1 << 20  // a Paddle webhook
	maxHandover  = 2 << 20  // a batch from the signer, MaxStock keys
	paddleWindow = 5 * time.Minute
)

// Handler is every web endpoint of the dispenser.
func (e *Engine) Handler(c APIConfig) (http.Handler, error) {
	// A caller without a token has its endpoints switched off.
	seen := map[string]bool{}
	for _, h := range append([]string{c.Signer, c.Release, c.Cron, c.Admin}, keys(c.Partners)...) {
		if h == "" {
			continue
		}
		if !tokenHash(h) {
			return nil, errors.New("every token is given as the hexadecimal SHA-256 of the token")
		}
		if seen[h] {
			return nil, errors.New("two callers share one token")
		}
		seen[h] = true
	}
	for _, name := range c.Partners {
		if !partnerName(name) {
			return nil, fmt.Errorf("partner name %q", name)
		}
	}
	if len(c.PaddleSecrets) == 0 {
		return nil, errors.New("the Paddle webhook needs its secret")
	}
	for _, s := range c.PaddleSecrets {
		if len(s) < 16 {
			return nil, errors.New("a Paddle secret is too short to be one")
		}
	}
	if c.ClientIP == nil {
		c.ClientIP = remoteIP
	}
	if c.Log == nil {
		c.Log = slog.New(slog.DiscardHandler)
	}
	a := &api{e: e, c: c, byEmail: newLimiter(3, time.Hour), byCaller: newLimiter(20, time.Hour)}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /paddle", a.paddle)
	mux.HandleFunc("GET /v1/thanks/{ref}", a.public(a.thanks))
	mux.HandleFunc("POST /v1/lost", a.public(a.lost))
	mux.HandleFunc("OPTIONS /v1/thanks/{ref}", a.public(nil))
	mux.HandleFunc("OPTIONS /v1/lost", a.public(nil))
	mux.HandleFunc("POST /v1/orders", a.partner(a.order))
	mux.HandleFunc("GET /v1/orders/{ref}", a.partner(a.orderKeys))
	mux.HandleFunc("POST /v1/orders/{ref}/revoke", a.partner(a.orderRevoke))
	mux.HandleFunc("GET /v1/pool", a.only(c.Signer, a.poolLevel))
	mux.HandleFunc("POST /v1/pool", a.only(c.Signer, a.handover))
	mux.HandleFunc("GET /v1/feed", a.only(c.Release, a.feed))
	mux.HandleFunc("POST /v1/cron/mail", a.only(c.Cron, a.cronMail))
	mux.HandleFunc("POST /v1/cron/daily", a.only(c.Cron, a.cronDaily))
	mux.HandleFunc("POST /v1/admin/{action}", a.only(c.Admin, a.admin))
	mux.HandleFunc("GET /healthz", a.health)
	return a.wrap(mux), nil
}

type api struct {
	e        *Engine
	c        APIConfig
	byEmail  *limiter
	byCaller *limiter
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func tokenHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil && strings.ToLower(h) == h
}

// TokenHash is the hash a token is configured by.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// wrap puts what every response needs around every endpoint: no caching,
// no guessing of types, a panic answered as a failure, and a log line.
func (a *api) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &status{ResponseWriter: w, code: http.StatusOK}
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		defer func() {
			if p := recover(); p != nil {
				a.c.Log.Error("panic", "path", r.URL.Path, "panic", fmt.Sprint(p))
				if !rec.wrote {
					fail(rec, http.StatusInternalServerError, "internal")
				}
			}
			a.c.Log.Info("request", "method", r.Method, "path", r.Pattern, "status", rec.code, "took", time.Since(start).Round(time.Millisecond))
		}()
		next.ServeHTTP(rec, r)
	})
}

type status struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (s *status) WriteHeader(code int) {
	s.code, s.wrote = code, true
	s.ResponseWriter.WriteHeader(code)
}

func (s *status) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// reply writes v as JSON.
func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// fail writes an error as a short code a caller can act on, and never the
// error itself, which may say more about us than a caller needs.
func fail(w http.ResponseWriter, code int, what string) {
	reply(w, code, map[string]string{"error": what})
}

// answer turns an error of the engine into a response, and logs what is
// not the caller's fault.
func (a *api) answer(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		fail(w, http.StatusBadRequest, "invalid")
	case errors.Is(err, ErrNotFound):
		fail(w, http.StatusNotFound, "not_found")
	case errors.Is(err, ErrConflict):
		fail(w, http.StatusConflict, "conflict")
	case errors.Is(err, ErrStale):
		fail(w, http.StatusConflict, "stale")
	case errors.Is(err, ErrPoolEmpty):
		a.c.Log.Error("the pool is empty", "path", r.Pattern)
		w.Header().Set("Retry-After", "300")
		fail(w, http.StatusServiceUnavailable, "pool_empty")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		fail(w, http.StatusServiceUnavailable, "timeout")
	default:
		a.c.Log.Error("failed", "path", r.Pattern, "error", err.Error())
		fail(w, http.StatusInternalServerError, "internal")
	}
}

// read decodes one JSON object of at most limit bytes into v, and refuses
// anything else: an unknown field, a second object, a body too large.
func read(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		fail(w, http.StatusUnsupportedMediaType, "json_only")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "too_large")
		} else {
			fail(w, http.StatusBadRequest, "invalid")
		}
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		fail(w, http.StatusBadRequest, "invalid")
		return false
	}
	return true
}

// bearer is the token a request carries, or "".
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok && t != "" {
		return t
	}
	return ""
}

// only lets a request through when it carries the token whose hash is want.
func (a *api) only(want string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := bearer(r)
		if want == "" || t == "" || subtle.ConstantTimeCompare([]byte(TokenHash(t)), []byte(want)) != 1 {
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

type partnerKey struct{}

// partner lets a request through when it carries a partner's token, and
// gives the handler that partner's source, so it reaches only its own
// orders.
func (a *api) partner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := bearer(r)
		name, ok := a.c.Partners[TokenHash(t)]
		if t == "" || !ok {
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), partnerKey{}, "partner:"+name)))
	}
}

// public is an endpoint our website's pages call: only that origin may
// read the answer.
func (a *api) public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			if a.c.Website == "" || o != a.c.Website {
				fail(w, http.StatusForbidden, "origin")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions || next == nil {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "3600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	if _, err := a.e.PoolLevel(r.Context()); err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]string{"status": "ok"})
}

// thanks shows a sale's keys on the thank-you page, from the moment they
// are assigned and for a day, with the key ID of each, in the same order,
// which is how the page and the app name a key.
func (a *api) thanks(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	keys, at, err := a.e.keysAt(r.Context(), "paddle", ref)
	switch {
	case errors.Is(err, ErrNotFound):
		reply(w, http.StatusAccepted, map[string]bool{"waiting": true})
	case err != nil:
		a.answer(w, r, err)
	case a.e.now().Sub(at) > ThanksWindow:
		fail(w, http.StatusGone, "expired")
	default:
		ids := make([]string, len(keys))
		for i, k := range keys {
			id, err := k.ID()
			if err != nil {
				a.answer(w, r, err)
				return
			}
			ids[i] = id.String()
		}
		reply(w, http.StatusOK, map[string]any{"keys": keys, "ids": ids})
	}
}

// lost sends a buyer's keys to the address given. It answers the same
// whether the address bought anything, and limits how often an address
// and a caller can ask.
func (a *api) lost(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !read(w, r, maxBody, &req) {
		return
	}
	if err := checkEmail(req.Email); err != nil {
		fail(w, http.StatusBadRequest, "invalid")
		return
	}
	now := a.e.now()
	if !a.byCaller.allow(a.c.ClientIP(r), now) || !a.byEmail.allow(strings.ToLower(req.Email), now) {
		w.Header().Set("Retry-After", "3600")
		fail(w, http.StatusTooManyRequests, "too_many")
		return
	}
	if err := a.e.Resend(r.Context(), req.Email); err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusAccepted, map[string]bool{"sent": true})
}

func source(r *http.Request) string { return r.Context().Value(partnerKey{}).(string) }

func (a *api) order(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref   string `json:"ref"`
		Seats int    `json:"seats"`
		Email string `json:"email"`
	}
	if !read(w, r, maxBody, &req) {
		return
	}
	keys, err := a.e.Assign(r.Context(), Order{Source: source(r), Ref: req.Ref, Seats: req.Seats, Email: req.Email, At: a.e.now()})
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"keys": keys})
}

func (a *api) orderKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.e.Keys(r.Context(), source(r), r.PathValue("ref"))
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"keys": keys})
}

func (a *api) orderRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Why string `json:"why"`
	}
	if !read(w, r, maxBody, &req) {
		return
	}
	n, err := a.e.Revoke(r.Context(), Target{Source: source(r), Ref: r.PathValue("ref")}, req.Why)
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]int{"revoked": n})
}

func (a *api) poolLevel(w http.ResponseWriter, r *http.Request) {
	l, err := a.e.PoolLevel(r.Context())
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"left": l.Left, "batch": l.Batch, "generation": l.Generation})
}

func (a *api) handover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Generation string        `json:"generation"`
		Keys       []licence.Key `json:"keys"`
	}
	if !read(w, r, maxHandover, &req) {
		return
	}
	added, err := a.e.Stock(r.Context(), req.Generation, req.Keys)
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]int{"added": added})
}

// feed is what the release workflow puts in the update feed: the
// revocation list, the genuine lists of leaked signers, and the head of
// the record, checked from its first line.
func (a *api) feed(w http.ResponseWriter, r *http.Request) {
	head, err := a.e.VerifyRecord(r.Context())
	if err != nil {
		a.answer(w, r, err)
		return
	}
	revoked, err := a.e.Revocations(r.Context())
	if err != nil {
		a.answer(w, r, err)
		return
	}
	genuine := map[string][]string{}
	for _, s := range a.c.Leaked {
		list, err := a.e.Genuine(r.Context(), s)
		if err != nil {
			a.answer(w, r, err)
			return
		}
		genuine[strconv.Itoa(int(s))] = hexes(list)
	}
	reply(w, http.StatusOK, map[string]any{
		"revoked": hexes(revoked),
		"genuine": genuine,
		"record":  map[string]any{"seq": head.Seq, "head": hex.EncodeToString(head.Hash[:])},
	})
}

func hexes(fs []licence.Fingerprint) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.String()
	}
	return out
}

func (a *api) cronMail(w http.ResponseWriter, r *http.Request) {
	sent, failed, err := a.e.SendMail(r.Context())
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]int{"sent": sent, "failed": failed})
}

// cronDaily is the daily run: catch up with Paddle over the last week, warn
// about the pool, and audit the store against its record. An audit that
// fails tells us at once.
func (a *api) cronDaily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settled, recErr := a.e.Reconcile(ctx, a.e.now().Add(-7*24*time.Hour))
	poolErr := a.e.CheckPool(ctx)
	head, auditErr := a.e.Audit(ctx)
	if auditErr != nil && (errors.Is(auditErr, ErrAudit) || errors.Is(auditErr, ErrBrokenRecord)) {
		_ = a.e.mailer.Us(ctx, "The audit of the dispenser failed", auditErr.Error())
	}
	res := map[string]any{"settled": settled, "seq": head.Seq}
	code := http.StatusOK
	for name, err := range map[string]error{"reconcile": recErr, "pool": poolErr, "audit": auditErr} {
		if err != nil {
			a.c.Log.Error("the daily run", "part", name, "error", err.Error())
			res[name] = "failed"
			code = http.StatusInternalServerError
		}
	}
	reply(w, code, res)
}

// admin is us, by hand, with the token kept offline.
func (a *api) admin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source      string `json:"source"`
		Ref         string `json:"ref"`
		Fingerprint string `json:"fingerprint"`
		Why         string `json:"why"`
		Since       string `json:"since"`
	}
	if !read(w, r, maxBody, &req) {
		return
	}
	var f licence.Fingerprint
	if req.Fingerprint != "" {
		var err error
		if f, err = licence.ParseFingerprint(req.Fingerprint); err != nil {
			fail(w, http.StatusBadRequest, "invalid")
			return
		}
	}
	ctx := r.Context()
	var res any
	var err error
	switch r.PathValue("action") {
	case "replace":
		var k licence.Key
		var s Seat
		k, s, err = a.e.Replace(ctx, f, req.Why)
		res = map[string]any{"key": k, "source": s.Source, "ref": s.Ref, "seat": s.Seat}
	case "revoke":
		var n int
		n, err = a.e.Revoke(ctx, Target{Source: req.Source, Ref: req.Ref, Key: f}, req.Why)
		res = map[string]int{"revoked": n}
	case "restore":
		var n int
		n, err = a.e.Restore(ctx, Target{Source: req.Source, Ref: req.Ref, Key: f}, req.Why)
		res = map[string]int{"restored": n}
	case "revoke-unsold":
		var n int
		n, err = a.e.RevokeUnsold(ctx, req.Why)
		res = map[string]int{"burned": n}
	case "retire":
		var n int
		n, err = a.e.Retire(ctx, req.Why)
		res = map[string]int{"retired": n}
	case "settle":
		err = a.e.Settle(ctx, req.Ref)
		res = map[string]bool{"settled": err == nil}
	case "reconcile":
		since, perr := time.Parse(time.RFC3339, req.Since)
		if perr != nil {
			fail(w, http.StatusBadRequest, "invalid")
			return
		}
		var n int
		n, err = a.e.Reconcile(ctx, since)
		res = map[string]any{"settled": n}
	case "audit":
		var head Anchor
		head, err = a.e.Audit(ctx)
		res = map[string]any{"seq": head.Seq, "head": hex.EncodeToString(head.Hash[:])}
	default:
		fail(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, res)
}
