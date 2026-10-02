package dispenser

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/licence"
)

const (
	paddleSecret = "pdl_ntfset_test_secret_0123456789"
	signerToken  = "signer-token"
	releaseToken = "release-token"
	cronToken    = "cron-token"
	adminToken   = "admin-token"
	bundleToken  = "bundle-hunt-token"
	stackToken   = "stacksocial-token"
	website      = "https://framefairy.app"
)

type web struct {
	*fixture
	h http.Handler
}

func newWeb(t testing.TB, twice bool) *web {
	f := newFixture(t, twice)
	h, err := f.engine.Handler(APIConfig{
		PaddleSecrets: []string{paddleSecret},
		Partners:      map[string]string{TokenHash(bundleToken): "bundle-hunt", TokenHash(stackToken): "stacksocial"},
		Signer:        TokenHash(signerToken),
		Release:       TokenHash(releaseToken),
		Cron:          TokenHash(cronToken),
		Admin:         TokenHash(adminToken),
		Website:       website,
		Leaked:        []uint8{0},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &web{fixture: f, h: h}
}

// call makes one request and returns the status and the decoded answer.
func (w *web) call(method, path, token string, body any, header ...string) (int, map[string]any) {
	w.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	case []byte:
		r = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		r = bytes.NewReader(j)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Header().Get("Cache-Control") != "no-store" {
		w.t.Fatalf("%s %s answered without no-store", method, path)
	}
	return rec.Code, out
}

// webhook is Paddle sending event about ref, signed at the time given.
func (w *web) webhook(event, ref string, at time.Time) (int, map[string]any) {
	w.t.Helper()
	field := "id"
	if strings.HasPrefix(event, "adjustment.") {
		field = "transaction_id"
	}
	body := []byte(`{"event_id":"evt_1","event_type":"` + event + `","occurred_at":"2026-10-28T14:05:09Z","data":{"` + field + `":"` + ref + `","status":"completed","items":[{"price":{"id":"pri_1"},"quantity":1}]}}`)
	return w.call("POST", "/paddle", "", body, "Paddle-Signature", signPaddle(body, paddleSecret, at))
}

func keysIn(t testing.TB, m map[string]any) []licence.Key {
	t.Helper()
	raw, _ := m["keys"].([]any)
	var out []licence.Key
	for _, k := range raw {
		out = append(out, licence.Key(k.(string)))
	}
	return out
}

func TestHandlerRefusesItsConfig(t *testing.T) {
	e := newFixture(t, false).engine
	good := APIConfig{PaddleSecrets: []string{paddleSecret}}
	for name, change := range map[string]func(*APIConfig){
		"no Paddle secret":    func(c *APIConfig) { c.PaddleSecrets = nil },
		"a short secret":      func(c *APIConfig) { c.PaddleSecrets = []string{"abc"} },
		"a token, not a hash": func(c *APIConfig) { c.Admin = adminToken },
		"a hash in capitals":  func(c *APIConfig) { c.Admin = strings.ToUpper(TokenHash(adminToken)) },
		"one token twice":     func(c *APIConfig) { c.Admin, c.Cron = TokenHash("x"), TokenHash("x") },
		"a bad partner name":  func(c *APIConfig) { c.Partners = map[string]string{TokenHash("p"): "Bundle Hunt"} },
	} {
		c := good
		change(&c)
		if _, err := e.Handler(c); err == nil {
			t.Errorf("%s: a handler was made", name)
		}
	}
	if _, err := e.Handler(good); err != nil {
		t.Fatalf("the good config: %v", err)
	}
}

// Use case 1 through Paddle's webhook: signed, settled with what Paddle
// says, the same webhook twice changes nothing.
func TestPaddleWebhook(t *testing.T) {
	w := newWeb(t, false)
	w.stock(10)
	w.shop.put(Sale{Ref: "txn_01", Seats: 2, Email: "anna@example.com", At: now}, now)
	code, out := w.webhook("transaction.completed", "txn_01", now)
	if code != 200 || out["settled"] != true {
		t.Fatalf("%d %v", code, out)
	}
	w.webhook("transaction.completed", "txn_01", now)
	keys, _ := w.engine.Keys(w.ctx, "paddle", "txn_01")
	if len(keys) != 2 || len(w.mail.sent()) != 1 {
		t.Fatalf("%d keys, %d letters", len(keys), len(w.mail.sent()))
	}
	// A chargeback, then its reversal, each from its own webhook.
	w.shop.put(Sale{Ref: "txn_01", Seats: 2, Email: "anna@example.com", At: now, TakenBack: true}, now)
	w.webhook("adjustment.created", "txn_01", now)
	if len(w.revoked()) != 2 {
		t.Fatal("a chargeback webhook did not revoke")
	}
	w.shop.put(Sale{Ref: "txn_01", Seats: 2, Email: "anna@example.com", At: now}, now)
	w.webhook("adjustment.updated", "txn_01", now)
	if len(w.revoked()) != 0 {
		t.Fatal("a reversal webhook did not restore")
	}
	w.audit()
}

func TestPaddleWebhookRefuses(t *testing.T) {
	w := newWeb(t, false)
	w.stock(10)
	w.shop.put(Sale{Ref: "txn_01", Seats: 1, Email: "anna@example.com", At: now}, now)
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_01"}}`)
	good := signPaddle(body, paddleSecret, now)
	cases := map[string]string{
		"no signature":                "",
		"another secret":              signPaddle(body, "pdl_ntfset_someone_elses_secret", now),
		"six minutes old":             signPaddle(body, paddleSecret, now.Add(-6*time.Minute)),
		"six minutes ahead":           signPaddle(body, paddleSecret, now.Add(6*time.Minute)),
		"no time":                     good[strings.Index(good, ";")+1:],
		"no hash":                     good[:strings.Index(good, ";")],
		"a hash cut short":            good[:len(good)-2],
		"a hash not hexadecimal":      good[:len(good)-1] + "z",
		"two times":                   good + ";ts=1",
		"a time with a sign":          strings.Replace(good, "ts=", "ts=+", 1),
		"garbage":                     "hello",
		"a signature of another body": signPaddle([]byte(`{"event_type":"transaction.completed","data":{"id":"txn_02"}}`), paddleSecret, now),
	}
	for name, sig := range cases {
		code, _ := w.call("POST", "/paddle", "", body, "Paddle-Signature", sig)
		if code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, code)
		}
	}
	if _, err := w.engine.Keys(w.ctx, "paddle", "txn_01"); err == nil {
		t.Fatal("a webhook that does not verify assigned keys")
	}
	// Signed and in time, it goes through, also within five minutes either way.
	for _, at := range []time.Time{now.Add(-4 * time.Minute), now.Add(4 * time.Minute)} {
		if code, _ := w.call("POST", "/paddle", "", body, "Paddle-Signature", signPaddle(body, paddleSecret, at)); code != 200 {
			t.Fatalf("a webhook signed at %s: %d", at, code)
		}
	}
	// During a rotation, either secret's signature is accepted.
	two := strings.Replace(good, "h1=", "h1="+strings.Repeat("ab", 32)+";h1=", 1)
	if code, _ := w.call("POST", "/paddle", "", body, "Paddle-Signature", two); code != 200 {
		t.Fatalf("two signatures, one good: %d", code)
	}
}

func TestPaddleWebhookAnswers(t *testing.T) {
	w := newWeb(t, false)
	w.stock(1)
	// An event the dispenser does not act on is taken, so Paddle stops.
	if code, out := w.webhook("subscription.created", "txn_01", now); code != 200 || out["ignored"] != true {
		t.Fatalf("%d %v", code, out)
	}
	// A sale Paddle does not know yet comes again.
	if code, _ := w.webhook("transaction.completed", "txn_unknown", now); code != http.StatusServiceUnavailable {
		t.Fatalf("an unknown sale: %d, want 503", code)
	}
	// A sale the pool cannot fill comes again.
	w.shop.put(Sale{Ref: "txn_big", Seats: 5, Email: "a@example.com", At: now}, now)
	if code, _ := w.webhook("transaction.completed", "txn_big", now); code != http.StatusServiceUnavailable {
		t.Fatalf("an empty pool: %d, want 503", code)
	}
	// Paddle down comes again.
	w.shop.setDown(true)
	if code, _ := w.webhook("transaction.completed", "txn_big", now); code != http.StatusInternalServerError {
		t.Fatalf("Paddle down: %d, want 500", code)
	}
	w.shop.setDown(false)
	// A signed webhook that names no sale is refused.
	if code, _ := w.webhook("transaction.completed", "txn bad", now); code != http.StatusBadRequest {
		t.Fatalf("a bad reference: %d", code)
	}
	body := []byte(`not json`)
	if code, _ := w.call("POST", "/paddle", "", body, "Paddle-Signature", signPaddle(body, paddleSecret, now)); code != http.StatusBadRequest {
		t.Fatalf("a body that is not JSON: %d", code)
	}
	big := bytes.Repeat([]byte("x"), maxPaddle+1)
	if code, _ := w.call("POST", "/paddle", "", big, "Paddle-Signature", signPaddle(big, paddleSecret, now)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a body too large: %d", code)
	}
}

// Use case 2: the page waits, then shows the keys, for a day.
func TestThankYouPage(t *testing.T) {
	w := newWeb(t, false)
	w.stock(5)
	if code, out := w.call("GET", "/v1/thanks/txn_01", "", nil); code != http.StatusAccepted || out["waiting"] != true {
		t.Fatalf("before the sale: %d %v", code, out)
	}
	keys := w.assign("txn_01", 2)
	code, out := w.call("GET", "/v1/thanks/txn_01", "", nil, "Origin", website)
	if code != 200 || !slices.Equal(keysIn(t, out), keys) {
		t.Fatalf("after the sale: %d %v", code, out)
	}
	w.clock.add(ThanksWindow + time.Second)
	if code, out := w.call("GET", "/v1/thanks/txn_01", "", nil); code != http.StatusGone || out["keys"] != nil {
		t.Fatalf("after a day: %d %v", code, out)
	}
	if code, _ := w.call("GET", "/v1/thanks/txn%20bad", "", nil); code != http.StatusBadRequest {
		t.Fatalf("a bad reference: %d", code)
	}
	if code, _ := w.call("GET", "/v1/thanks/txn_01", "", nil, "Origin", "https://evil.example"); code != http.StatusForbidden {
		t.Fatalf("another site: %d", code)
	}
}

func TestCORS(t *testing.T) {
	w := newWeb(t, false)
	req := httptest.NewRequest("OPTIONS", "/v1/lost", nil)
	req.Header.Set("Origin", website)
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != website {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	req = httptest.NewRequest("GET", "/v1/thanks/txn_01", nil)
	req.Header.Set("Origin", website)
	rec = httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != website {
		t.Fatal("our website cannot read the thank-you page")
	}
	// The partner API is for programs, not pages.
	req = httptest.NewRequest("GET", "/v1/orders/1", nil)
	req.Header.Set("Origin", website)
	req.Header.Set("Authorization", "Bearer "+bundleToken)
	rec = httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("a page may read the partner API")
	}
}

// Use case 7 through the page: the same answer whatever the address, and
// limited per address and per caller.
func TestLostKeyPage(t *testing.T) {
	w := newWeb(t, false)
	w.stock(5)
	w.shop.sold("txn_01", "anna@example.com")
	w.assign("txn_01", 1)
	before := len(w.mail.sent())
	codeA, outA := w.call("POST", "/v1/lost", "", map[string]string{"email": "anna@example.com"})
	codeB, outB := w.call("POST", "/v1/lost", "", map[string]string{"email": "nobody@example.com"})
	if codeA != http.StatusAccepted || codeB != codeA || outA["sent"] != outB["sent"] {
		t.Fatalf("the answers differ: %d %v, %d %v", codeA, outA, codeB, outB)
	}
	if got := w.mail.sent()[before:]; len(got) != 1 || got[0].to != "anna@example.com" {
		t.Fatalf("letters %+v", got)
	}
	for range 2 {
		w.call("POST", "/v1/lost", "", map[string]string{"email": "Anna@Example.com"})
	}
	if code, _ := w.call("POST", "/v1/lost", "", map[string]string{"email": "anna@example.com"}); code != http.StatusTooManyRequests {
		t.Fatalf("a fourth ask for one address in an hour: %d", code)
	}
	for i := range 20 {
		w.call("POST", "/v1/lost", "", map[string]string{"email": "x" + string(rune('a'+i)) + "@example.com"})
	}
	if code, _ := w.call("POST", "/v1/lost", "", map[string]string{"email": "fresh@example.com"}); code != http.StatusTooManyRequests {
		t.Fatalf("too many asks from one caller: %d", code)
	}
	w.clock.add(time.Hour)
	if code, _ := w.call("POST", "/v1/lost", "", map[string]string{"email": "anna@example.com"}); code != http.StatusAccepted {
		t.Fatalf("an hour later: %d", code)
	}
	for name, body := range map[string]any{
		"not an address":   map[string]string{"email": "anna"},
		"two addresses":    map[string]string{"email": "a@b.c,d@e.f"},
		"an unknown field": `{"email":"anna@example.com","admin":true}`,
		"two objects":      `{"email":"anna@example.com"}{"email":"x@y.z"}`,
		"not JSON":         `email=anna@example.com`,
		"too large":        `{"email":"` + strings.Repeat("a", maxBody) + `@example.com"}`,
	} {
		if code, _ := w.call("POST", "/v1/lost", "", body); code < 400 || code > 499 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

// Use case 4: a partner's orders through its own token, which reaches no
// other partner's.
func TestPartnerAPI(t *testing.T) {
	w := newWeb(t, false)
	w.stock(10)
	code, out := w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": "1001", "seats": 2, "email": "max@example.com"})
	if code != 200 || len(keysIn(t, out)) != 2 {
		t.Fatalf("%d %v", code, out)
	}
	keys := keysIn(t, out)
	_, again := w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": "1001", "seats": 2, "email": "max@example.com"})
	if !slices.Equal(keysIn(t, again), keys) {
		t.Fatal("the same order twice gave other keys")
	}
	if code, out := w.call("GET", "/v1/orders/1001", bundleToken, nil); code != 200 || !slices.Equal(keysIn(t, out), keys) {
		t.Fatalf("reading it back: %d %v", code, out)
	}
	// Another partner's order of the same reference is another order, and
	// it cannot read or revoke the first.
	if code, _ := w.call("GET", "/v1/orders/1001", stackToken, nil); code != http.StatusNotFound {
		t.Fatalf("another partner read it: %d", code)
	}
	if code, _ := w.call("POST", "/v1/orders/1001/revoke", stackToken, map[string]string{"why": "refund"}); code != http.StatusNotFound {
		t.Fatalf("another partner revoked it: %d", code)
	}
	if len(w.revoked()) != 0 {
		t.Fatal("another partner's revoke revoked something")
	}
	if code, out := w.call("POST", "/v1/orders/1001/revoke", bundleToken, map[string]string{"why": "refund"}); code != 200 || out["revoked"] != 2.0 {
		t.Fatalf("its own revoke: %d %v", code, out)
	}
	if code, _ := w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": "1001", "seats": 3}); code != http.StatusConflict {
		t.Fatalf("the same order with other seats: %d", code)
	}
	if code, _ := w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": "1002", "seats": 50}); code != http.StatusServiceUnavailable {
		t.Fatalf("more seats than the pool: %d", code)
	}
	for _, token := range []string{"", "wrong", signerToken, adminToken} {
		if code, _ := w.call("POST", "/v1/orders", token, map[string]any{"ref": "1003", "seats": 1}); code != http.StatusUnauthorized {
			t.Errorf("token %q: %d", token, code)
		}
	}
	// A partner cannot name its source.
	if code, _ := w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": "1004", "seats": 1, "source": "paddle"}); code != http.StatusBadRequest {
		t.Fatalf("a partner naming the source: %d", code)
	}
	w.audit()
}

// Use case 12 through the signer's endpoints, and a batch of an earlier
// generation refused.
func TestSignerEndpoints(t *testing.T) {
	w := newWeb(t, false)
	code, out := w.call("GET", "/v1/pool", signerToken, nil)
	if code != 200 || out["left"] != 0.0 || out["batch"] != 1000.0 {
		t.Fatalf("%d %v", code, out)
	}
	gen := out["generation"].(string)
	batch := sign(t, 3)
	if code, out := w.call("POST", "/v1/pool", signerToken, map[string]any{"generation": gen, "keys": batch}); code != 200 || out["added"] != 3.0 {
		t.Fatalf("%d %v", code, out)
	}
	w.engine.Retire(w.ctx, "restored")
	if code, out := w.call("POST", "/v1/pool", signerToken, map[string]any{"generation": gen, "keys": sign(t, 2)}); code != http.StatusConflict || out["error"] != "stale" {
		t.Fatalf("an earlier generation: %d %v", code, out)
	}
	if code, _ := w.call("POST", "/v1/pool", signerToken, map[string]any{"generation": w.generation(), "keys": signWith(t, 1, strangerSigner(), 0, "")}); code != http.StatusBadRequest {
		t.Fatalf("a stranger's key: %d", code)
	}
	for _, token := range []string{"", bundleToken, adminToken, releaseToken} {
		if code, _ := w.call("GET", "/v1/pool", token, nil); code != http.StatusUnauthorized {
			t.Errorf("token %q read the pool: %d", token, code)
		}
	}
}

// A full batch fits in a request, and one too large does not.
func TestHandoverSize(t *testing.T) {
	w := newWeb(t, false)
	gen := w.generation()
	batch := sign(t, MaxStock)
	if code, out := w.call("POST", "/v1/pool", signerToken, map[string]any{"generation": gen, "keys": batch}); code != 200 || out["added"] != float64(MaxStock) {
		t.Fatalf("a full batch: %d %v", code, out)
	}
	big := `{"generation":"","keys":["` + strings.Repeat("A", maxHandover) + `"]}`
	if code, _ := w.call("POST", "/v1/pool", signerToken, big); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", code)
	}
}

// Use case 18: the release workflow's lists, and the record's head.
func TestFeed(t *testing.T) {
	w := newWeb(t, false)
	w.stock(5)
	keys := w.assign("txn_01", 2)
	w.engine.Revoke(w.ctx, Target{Key: keys[0].Fingerprint()}, "refund")
	code, out := w.call("GET", "/v1/feed", releaseToken, nil)
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	revoked := out["revoked"].([]any)
	if len(revoked) != 1 || revoked[0] != keys[0].Fingerprint().String() {
		t.Fatalf("revoked %v", revoked)
	}
	genuine := out["genuine"].(map[string]any)["0"].([]any)
	if len(genuine) != 2 {
		t.Fatalf("genuine %v", genuine)
	}
	head, _ := w.engine.VerifyRecord(w.ctx)
	if out["record"].(map[string]any)["seq"] != float64(head.Seq) {
		t.Fatal("the feed's head is not the record's")
	}
	if code, _ := w.call("GET", "/v1/feed", signerToken, nil); code != http.StatusUnauthorized {
		t.Fatalf("the signer read the feed: %d", code)
	}
}

// Use cases 13, 19 and 21: the scheduled runs.
func TestCron(t *testing.T) {
	w := newWeb(t, false)
	w.stock(5)
	w.shop.put(Sale{Ref: "txn_missed", Seats: 1, Email: "a@example.com", At: now}, now)
	code, out := w.call("POST", "/v1/cron/daily", cronToken, nil)
	if code != 200 || out["settled"] != 1.0 {
		t.Fatalf("%d %v", code, out)
	}
	if _, err := w.engine.Keys(w.ctx, "paddle", "txn_missed"); err != nil {
		t.Fatal("the daily run did not catch up a sale")
	}
	if len(w.mail.warnings()) != 1 {
		t.Fatal("the daily run did not warn about a pool of 4")
	}
	w.mail.failing(func(string, Letter) bool { return true })
	w.shop.put(Sale{Ref: "txn_2", Seats: 1, Email: "b@example.com", At: now}, now)
	w.call("POST", "/v1/cron/daily", cronToken, nil)
	w.mail.failing(nil)
	w.clock.add(time.Minute)
	if code, out := w.call("POST", "/v1/cron/mail", cronToken, nil); code != 200 || out["sent"] != 1.0 {
		t.Fatalf("the mail run: %d %v", code, out)
	}
	if code, _ := w.call("POST", "/v1/cron/daily", adminToken, nil); code != http.StatusUnauthorized {
		t.Fatalf("the admin token ran the daily run: %d", code)
	}
}

// A store that does not add up to its record fails the daily run and tells
// us.
func TestCronAuditTellsUs(t *testing.T) {
	w := newWeb(t, false)
	w.stock(2)
	w.store.db.pool[0].State = Sold
	if code, out := w.call("POST", "/v1/cron/daily", cronToken, nil); code != http.StatusInternalServerError || out["audit"] != "failed" {
		t.Fatalf("%d %v", code, out)
	}
	found := false
	for _, m := range w.mail.warnings() {
		found = found || strings.Contains(m, "audit")
	}
	if !found {
		t.Fatalf("warnings %q", w.mail.warnings())
	}
}

// Use cases 11, 14 and 15 by hand.
func TestAdmin(t *testing.T) {
	w := newWeb(t, false)
	w.stock(6)
	w.shop.sold("txn_01", "anna@example.com")
	keys := w.assign("txn_01", 1)
	code, out := w.call("POST", "/v1/admin/replace", adminToken, map[string]string{"fingerprint": keys[0].Fingerprint().String(), "why": "posted on a forum"})
	if code != 200 || out["ref"] != "txn_01" || out["key"] == keys[0] {
		t.Fatalf("replace: %d %v", code, out)
	}
	if code, out := w.call("POST", "/v1/admin/retire", adminToken, map[string]string{"why": "restored"}); code != 200 || out["retired"] != 4.0 {
		t.Fatalf("retire: %d %v", code, out)
	}
	if code, out := w.call("POST", "/v1/admin/audit", adminToken, map[string]string{}); code != 200 || out["seq"] == nil {
		t.Fatalf("audit: %d %v", code, out)
	}
	for name, c := range map[string]struct {
		path string
		body any
		want int
	}{
		"an unknown action":   {"/v1/admin/drop", map[string]string{}, 404},
		"a bad fingerprint":   {"/v1/admin/replace", map[string]string{"fingerprint": "xyz", "why": "x"}, 400},
		"no reason":           {"/v1/admin/revoke-unsold", map[string]string{}, 400},
		"a time that is none": {"/v1/admin/reconcile", map[string]string{"since": "yesterday"}, 400},
		"an unknown field":    {"/v1/admin/retire", map[string]string{"why": "x", "force": "yes"}, 400},
	} {
		if code, _ := w.call("POST", c.path, adminToken, c.body); code != c.want {
			t.Errorf("%s: %d, want %d", name, code, c.want)
		}
	}
	if code, _ := w.call("POST", "/v1/admin/retire", cronToken, map[string]string{"why": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("the cron token used admin: %d", code)
	}
	w.audit()
}

func TestRoutes(t *testing.T) {
	w := newWeb(t, false)
	if code, out := w.call("GET", "/healthz", "", nil); code != 200 || out["status"] != "ok" {
		t.Fatalf("health: %d %v", code, out)
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/paddle"},
		{"DELETE", "/v1/orders/1"},
		{"GET", "/v1/admin/retire"},
		{"GET", "/"},
		{"GET", "/v1/secret"},
	} {
		if code, _ := w.call(c.method, c.path, adminToken, nil); code != 404 && code != 405 {
			t.Errorf("%s %s: %d", c.method, c.path, code)
		}
	}
}

// A store that panics fails that request only.
type panicking struct{ *Memory }

func (panicking) View(ctx context.Context, fn func(Tx) error) error { panic("the store broke") }

func TestPanicIsOneFailedRequest(t *testing.T) {
	f := newFixture(t, false)
	e, _ := New(panicking{f.store}, Config{Signers: f.engine.signers, Batch: 10, Mailer: f.mail, Orders: f.shop})
	h, err := e.Handler(APIConfig{PaddleSecrets: []string{paddleSecret}})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 500 {
		t.Fatalf("%d", rec.Code)
	}
}

// Many callers at once through the endpoints, under the race detector:
// webhooks, partners, the thank-you and lost-key pages, the signer and the
// runs. Then the audit.
func TestEndpointsAtOnce(t *testing.T) {
	w := newWeb(t, true)
	w.stock(300)
	for i := range 40 {
		ref := "txn_" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		w.shop.put(Sale{Ref: ref, Seats: 1 + i%3, Email: "b@example.com", At: now}, now)
	}
	var wg sync.WaitGroup
	for g := range 10 {
		wg.Go(func() {
			for i := range 20 {
				n := (g*20 + i) % 40
				ref := "txn_" + string(rune('a'+n%26)) + string(rune('a'+n/26))
				switch i % 6 {
				case 0, 1:
					w.webhook("transaction.completed", ref, now)
				case 2:
					w.call("GET", "/v1/thanks/"+ref, "", nil)
				case 3:
					w.call("POST", "/v1/orders", bundleToken, map[string]any{"ref": ref, "seats": 1})
				case 4:
					w.call("POST", "/v1/cron/mail", cronToken, nil)
				default:
					w.call("GET", "/v1/pool", signerToken, nil)
				}
			}
		})
	}
	wg.Wait()
	w.audit()
}
