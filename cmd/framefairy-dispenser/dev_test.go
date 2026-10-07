package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/licence/dispenser"
	"framefairy/licence/paddle"
)

// world is the local dispenser on a test server, driven the way a person
// drives it: through the checkout, the buyer's pages and the dev page.
type world struct {
	t      *testing.T
	d      *dev
	base   string
	client *http.Client

	mu     sync.Mutex
	nonces map[string]string // the nonce each sale's checkout page made
}

// nonce is one as the checkout page makes it.
func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// checkout is the checkout page paying, with the nonce it passes as
// custom data. It gives the answer, the sale's reference and the nonce.
func checkout(client *http.Client, base, email string, seats int) (*http.Response, string, string, error) {
	n := nonce()
	resp, err := client.PostForm(base+"/shop/buy", url.Values{"email": {email}, "seats": {itoa(seats)}, "thanks": {n}})
	if err != nil {
		return nil, "", "", err
	}
	resp.Body.Close()
	ref, _ := strings.CutPrefix(resp.Header.Get("Location"), "/thanks?txn=")
	return resp, ref, n, nil
}

func newWorld(t *testing.T, batch int) *world {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	base := "http://" + srv.Listener.Addr().String()
	d, err := newDev(base, t.TempDir(), batch, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = d.handler()
	srv.Start()
	t.Cleanup(srv.Close)
	w := &world{t: t, d: d, base: base, client: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	w.tick()
	return w
}

func (w *world) tick() { w.d.Tick(context.Background()) }

// press is a button on the dev page. It checks the dev page takes it and
// gives back what the dispenser answered.
func (w *world) press(action string, form url.Values) answer {
	w.t.Helper()
	w.d.mu.Lock()
	w.d.answer = nil
	w.d.mu.Unlock()
	resp, err := w.client.PostForm(w.base+"/dev/"+action, form)
	if err != nil {
		w.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		w.t.Fatalf("%s: %d", action, resp.StatusCode)
	}
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	if w.d.answer == nil {
		w.t.Fatalf("%s showed nothing", action)
	}
	return *w.d.answer
}

func (w *world) buy(email string, seats int) string {
	w.t.Helper()
	resp, ref, n, err := checkout(w.client, w.base, email, seats)
	if err != nil {
		w.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther || ref == "" {
		w.t.Fatalf("checkout answered %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	w.remember(ref, n)
	w.d.shop.Deliver(context.Background())
	return ref
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// page is a page as the browser gets it.
func (w *world) page(path string) string {
	w.t.Helper()
	resp, err := w.client.Get(w.base + path)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		w.t.Fatalf("%s: %d", path, resp.StatusCode)
	}
	return string(b)
}

// get asks the dispenser as a browser on our website would.
func (w *world) get(path string) (int, map[string]any) {
	w.t.Helper()
	resp, err := w.client.Get(w.base + path)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	b, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(b, &body)
	return resp.StatusCode, body
}

func (w *world) remember(ref, nonce string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.nonces == nil {
		w.nonces = map[string]string{}
	}
	w.nonces[ref] = nonce
}

// thanks is the thank-you page in the tab that paid for ref.
func (w *world) thanks(ref string) (int, []string) {
	w.mu.Lock()
	n := w.nonces[ref]
	w.mu.Unlock()
	return w.thanksWith(ref, n)
}

// thanksWith asks for the keys of ref with any nonce.
func (w *world) thanksWith(ref, nonce string) (int, []string) {
	w.t.Helper()
	b, _ := json.Marshal(map[string]string{"nonce": nonce})
	resp, err := w.client.Post(w.base+"/v1/thanks/"+ref, "application/json", bytes.NewReader(b))
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &body)
	code := resp.StatusCode
	var keys []string
	for _, k := range asList(body["keys"]) {
		keys = append(keys, k.(string))
	}
	return code, keys
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// revoked is the revocation list of the update feed.
func (w *world) revoked() []string {
	w.t.Helper()
	a := w.press("feed", nil)
	var feed struct {
		Revoked []string `json:"revoked"`
	}
	if err := json.Unmarshal([]byte(a.Body), &feed); err != nil || a.Code != http.StatusOK {
		w.t.Fatalf("feed: %d %s", a.Code, a.Body)
	}
	return feed.Revoked
}

// keysOf is what the dispenser holds for a sale, as the dev page reads it.
func (w *world) keysOf(ref string) []heldKey {
	for _, s := range w.d.look(context.Background()).Sales {
		if s.Ref == ref {
			return s.Keys
		}
	}
	return nil
}

func (w *world) revokedCount(ref string) int {
	n := 0
	for _, k := range w.keysOf(ref) {
		if k.Revoked {
			n++
		}
	}
	return n
}

func (w *world) audit() {
	w.t.Helper()
	if a := w.press("audit", nil); a.Code != http.StatusOK {
		w.t.Fatalf("audit: %d %s", a.Code, a.Body)
	}
}

func (w *world) lastWebhook() webhook {
	return w.d.shop.view().Webhooks[0]
}

func TestASaleFromCheckoutToChargeback(t *testing.T) {
	w := newWorld(t, 10)
	ref := w.buy("anna@example.com", 2)

	code, keys := w.thanks(ref)
	if code != http.StatusOK || len(keys) != 2 {
		t.Fatalf("thank-you page: %d %v", code, keys)
	}
	_, letters, _ := w.d.mail.read()
	if len(letters) != 1 || letters[0].To != "anna@example.com" || !slices.Equal(stringsOf(letters[0].Keys), keys) {
		t.Fatalf("letters %+v", letters)
	}
	// Each key in the letter opens the app with it. The template must not
	// take the framefairy:// link for an unsafe one.
	page := w.page("/dev")
	for _, k := range keys {
		if !strings.Contains(page, `href="framefairy://unlock?key=`+k+`"`) {
			t.Fatalf("the letter on the dev page has no Unlock link for %s", k)
		}
	}

	// A refund waits for Paddle's approval and takes nothing back until
	// it is approved.
	if a := w.press("adjust", url.Values{"ref": {ref}, "do": {paddle.Refund}}); a.Code != http.StatusOK {
		t.Fatalf("refund: %+v", a)
	}
	if n := w.revokedCount(ref); n != 0 {
		t.Fatalf("a refund waiting for approval revoked %d keys", n)
	}
	v := w.d.look(context.Background())
	pending := v.Sales[0].Pending
	if len(pending) != 1 {
		t.Fatalf("pending refunds %v", pending)
	}
	w.press("decide", url.Values{"ref": {ref}, "id": {pending[0].ID}, "approve": {"no"}})
	if n := w.revokedCount(ref); n != 0 {
		t.Fatalf("a rejected refund revoked %d keys", n)
	}

	// A chargeback revokes both keys, and its reversal brings them back.
	w.press("adjust", url.Values{"ref": {ref}, "do": {paddle.Chargeback}})
	if n := w.revokedCount(ref); n != 2 || len(w.revoked()) != 2 {
		t.Fatalf("after a chargeback %d keys revoked, the feed lists %d", n, len(w.revoked()))
	}
	w.press("adjust", url.Values{"ref": {ref}, "do": {paddle.ChargebackReverse}})
	if n := w.revokedCount(ref); n != 0 || len(w.revoked()) != 0 {
		t.Fatalf("after the reversal %d keys revoked", n)
	}
	for _, wh := range w.d.shop.view().Webhooks {
		if wh.State != whDelivered {
			t.Fatalf("webhook %s %s", wh.Type, wh.State)
		}
	}
	w.audit()
}

func stringsOf[T ~string](l []T) []string {
	out := make([]string, len(l))
	for i, s := range l {
		out[i] = string(s)
	}
	return out
}

func TestWebhooksInAnotherOrderTwiceOrLost(t *testing.T) {
	w := newWorld(t, 10)
	w.press("paddle", url.Values{"mode": {sendHold}})
	ref := w.buy("ben@example.com", 1)
	w.press("adjust", url.Values{"ref": {ref}, "do": {paddle.Chargeback}})
	if code, _ := w.thanks(ref); code != http.StatusAccepted {
		t.Fatalf("thank-you page before any webhook came: %d", code)
	}
	// The chargeback's webhook comes first, and it settles the sale as
	// Paddle has it now: sold, and taken back.
	held := w.d.shop.view().Webhooks
	w.press("release", url.Values{"id": {held[0].ID}})
	if n := w.revokedCount(ref); n != 1 {
		t.Fatalf("the chargeback that came first left %d keys revoked", n)
	}
	w.press("release", url.Values{"id": {held[1].ID}})
	if n := w.revokedCount(ref); n != 1 {
		t.Fatalf("the sale that came after its chargeback left %d keys revoked", n)
	}

	// Twice: the same keys and one letter.
	w.press("paddle", url.Values{"mode": {sendTwice}})
	twice := w.buy("cleo@example.com", 1)
	_, letters, _ := w.d.mail.read()
	n := 0
	for _, l := range letters {
		if l.Ref == twice {
			n++
		}
	}
	if n != 1 || len(w.keysOf(twice)) != 1 {
		t.Fatalf("a webhook sent twice: %d letters, %d keys", n, len(w.keysOf(twice)))
	}

	// Lost: only the daily run finds it.
	w.press("paddle", url.Values{"mode": {sendLose}})
	lost := w.buy("dora@example.com", 1)
	if len(w.keysOf(lost)) != 0 {
		t.Fatal("a lost webhook assigned keys")
	}
	if a := w.press("cron-daily", nil); a.Code != http.StatusOK {
		t.Fatalf("daily run: %+v", a)
	}
	if len(w.keysOf(lost)) != 1 {
		t.Fatal("the daily run did not find the sale whose webhook was lost")
	}
	w.audit()
}

func TestWhenThingsFail(t *testing.T) {
	w := newWorld(t, 10)

	// The mail service is down: the keys wait in the queue and go when
	// it is back and the mail run comes.
	w.press("mail", url.Values{"mode": {down}})
	ref := w.buy("eve@example.com", 1)
	if code, keys := w.thanks(ref); code != http.StatusOK || len(keys) != 1 {
		t.Fatalf("the thank-you page with the mail down: %d", code)
	}
	if v := w.d.look(context.Background()); len(v.Queue) != 1 {
		t.Fatalf("queue %v", v.Queue)
	}
	w.press("mail", url.Values{"mode": {works}})
	w.press("clock", url.Values{"by": {"5m"}})
	if _, letters, _ := w.d.mail.read(); len(letters) != 1 || letters[0].To != "eve@example.com" {
		t.Fatalf("after the mail came back: %+v", letters)
	}

	// The database is down: Paddle's webhook fails and is sent again.
	w.press("db", url.Values{"mode": {down}})
	ref = w.buy("finn@example.com", 1)
	if wh := w.lastWebhook(); wh.State != whPending || len(wh.Tries) != 1 || wh.Tries[0].Code != http.StatusInternalServerError {
		t.Fatalf("webhook with the database down: %+v", wh)
	}
	if code, _ := w.d.call(context.Background(), "", http.MethodGet, "/healthz", nil); code.Code == http.StatusOK {
		t.Fatal("healthy with the database down")
	}
	if v := w.d.look(context.Background()); v.ReadErr != "" {
		t.Fatalf("the dev page cannot read with the database down: %s", v.ReadErr)
	}
	w.press("db", url.Values{"mode": {works}})
	w.press("clock", url.Values{"by": {"1m"}})
	if wh := w.lastWebhook(); wh.State != whDelivered || len(w.keysOf(ref)) != 1 {
		t.Fatalf("after the database came back: %+v", wh)
	}

	// The database commits and loses the answer: Paddle sends again, the
	// letter waits for the mail run, and the sale ends with one key and
	// one letter.
	w.press("db", url.Values{"mode": {losesWord}})
	ref = w.buy("gus@example.com", 1)
	w.press("db", url.Values{"mode": {works}})
	w.press("clock", url.Values{"by": {"1m"}})
	w.press("clock", url.Values{"by": {"5m"}})
	_, letters, _ := w.d.mail.read()
	sent := 0
	for _, l := range letters {
		if l.Ref == ref {
			sent++
		}
	}
	if len(w.keysOf(ref)) != 1 || sent != 1 {
		t.Fatalf("after a lost answer: %d keys, %d letters", len(w.keysOf(ref)), sent)
	}

	// Paddle's API lags its webhook: the dispenser answers not yet, and
	// the retry after it caught up settles the sale.
	w.press("paddle", url.Values{"lag": {"on"}})
	ref = w.buy("hal@example.com", 1)
	if wh := w.lastWebhook(); len(wh.Tries) != 1 || wh.Tries[0].Code != http.StatusServiceUnavailable {
		t.Fatalf("webhook ahead of the API: %+v", wh)
	}
	w.press("clock", url.Values{"by": {"1m"}})
	w.press("clock", url.Values{"by": {"4m"}})
	if len(w.keysOf(ref)) != 1 {
		t.Fatalf("after the API caught up: %+v", w.lastWebhook())
	}
	w.press("paddle", url.Values{"lag": {"off"}})

	// The pool runs empty: the webhook fails, the thank-you page waits,
	// and the sale is settled once the signer has refilled the pool.
	w.press("auto", url.Values{"refill": {"off"}})
	ref = w.buy("ida@example.com", 20)
	if wh := w.lastWebhook(); len(wh.Tries) != 1 || wh.Tries[0].Code != http.StatusServiceUnavailable || !strings.Contains(wh.Tries[0].Answer, "pool_empty") {
		t.Fatalf("webhook with the pool empty: %+v", wh)
	}
	if code, _ := w.thanks(ref); code != http.StatusAccepted {
		t.Fatalf("thank-you page with the pool empty: %d", code)
	}
	w.press("sign", url.Values{"n": {"30"}})
	w.press("clock", url.Values{"by": {"1m"}})
	if code, keys := w.thanks(ref); code != http.StatusOK || len(keys) != 20 {
		t.Fatalf("after the refill: %d, %d keys", code, len(keys))
	}

	// Mail to us: the pool warning, after the daily run, once the pool
	// is below a fifth of a batch.
	l, err := w.d.engine.PoolLevel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.buy("jan@example.com", l.Left-1)
	w.press("cron-daily", nil)
	if _, _, notes := w.d.mail.read(); len(notes) == 0 {
		t.Fatal("no warning about the pool")
	}
	w.audit()
}

func TestThanksWindowAndLostKeys(t *testing.T) {
	w := newWorld(t, 10)
	ref := w.buy("jo@example.com", 1)
	if code, keys := w.thanks(ref); code != http.StatusOK || len(keys) != 1 {
		t.Fatalf("the tab that paid: %d %v", code, keys)
	}
	// The thank-you link opened anywhere else: the dev page's own link, a
	// receipt, a support mail.
	if code, keys := w.thanksWith(ref, nonce()); code != http.StatusAccepted || keys != nil {
		t.Fatalf("another browser with the reference: %d %v", code, keys)
	}
	if code, _ := w.get("/v1/thanks/" + ref); code == http.StatusOK {
		t.Fatalf("the reference alone: %d", code)
	}
	w.press("clock", url.Values{"by": {"1h"}})
	if code, _ := w.thanks(ref); code != http.StatusGone {
		t.Fatalf("thank-you page after an hour: %d", code)
	}
	lost := func(email string) int {
		req, _ := http.NewRequest(http.MethodPost, w.base+"/v1/lost", strings.NewReader(`{"email":"`+email+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", w.base)
		resp, err := w.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := lost("jo@example.com"); code != http.StatusAccepted {
		t.Fatalf("lost key: %d", code)
	}
	if code := lost("nobody@example.com"); code != http.StatusAccepted {
		t.Fatalf("lost key of an address that bought nothing: %d", code)
	}
	_, letters, _ := w.d.mail.read()
	if len(letters) != 2 || letters[1].Kind != "resend" || letters[1].To != "jo@example.com" {
		t.Fatalf("letters %+v", letters)
	}
	lost("jo@example.com")
	lost("jo@example.com")
	if code := lost("jo@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("the fourth ask in an hour: %d", code)
	}
}

// Every button on the dev page takes its click and the page reads back,
// with every switch in every position.
func TestEveryButton(t *testing.T) {
	w := newWorld(t, 10)
	ref := w.buy("kai@example.com", 1)
	fp := w.keysOf(ref)[0].Fingerprint
	presses := []struct {
		action string
		form   url.Values
	}{
		{"clock", url.Values{"by": {"5m"}}},
		{"clock", url.Values{"by": {"-1h"}}},
		{"paddle", url.Values{"api": {down}}},
		{"paddle", url.Values{"api": {works}, "mode": {sendNow}, "lag": {"off"}}},
		{"paddle", url.Values{"api": {"sideways"}}},
		{"mail", url.Values{"mode": {losesWord}}},
		{"mail", url.Values{"mode": {works}}},
		{"db", url.Values{"mode": {works}}},
		{"auto", url.Values{"refill": {"on"}, "schedule": {"off"}}},
		{"adjust", url.Values{"ref": {ref}, "do": {paddle.ChargebackWarning}}},
		{"adjust", url.Values{"ref": {ref}, "do": {paddle.ChargebackReverse}}},
		{"adjust", url.Values{"ref": {"txn_none"}, "do": {paddle.Refund}}},
		{"decide", url.Values{"ref": {ref}, "id": {"adj_none"}}},
		{"replay", url.Values{"ref": {ref}}},
		{"release", url.Values{"id": {"ntf_none"}}},
		{"sign", url.Values{"n": {"3"}}},
		{"sign", url.Values{"n": {"3"}, "generation": {"0123456789abcdef"}}},
		{"refill", nil},
		{"cron-mail", nil},
		{"cron-daily", nil},
		{"feed", nil},
		{"health", nil},
		{"replace", url.Values{"fingerprint": {fp}, "why": {"posted in public"}}},
		{"revoke", url.Values{"source": {"paddle"}, "ref": {ref}, "why": {"trying"}}},
		{"restore", url.Values{"source": {"paddle"}, "ref": {ref}, "why": {"trying"}}},
		{"revoke", url.Values{"fingerprint": {"not hex"}, "why": {"trying"}}},
		{"settle", url.Values{"ref": {ref}}},
		{"reconcile", url.Values{"days": {"7"}}},
		{"reconcile", url.Values{"days": {"x"}}},
		{"audit", nil},
		{"partner-order", url.Values{"ref": {"bh-1"}, "seats": {"2"}, "email": {"lu@example.com"}}},
		{"partner-keys", url.Values{"ref": {"bh-1"}}},
		{"partner-revoke", url.Values{"ref": {"bh-1"}, "why": {"refunded"}}},
		{"revoke-unsold", url.Values{"why": {"trying"}}},
		{"retire", url.Values{"why": {"trying"}}},
		{"nonsense", nil},
	}
	for _, p := range presses {
		w.press(p.action, p.form)
		for _, db := range []string{works, down} {
			w.d.db.set(db)
			resp, err := w.client.Get(w.base + "/dev")
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), "</html>") {
				t.Fatalf("after %s, with the database %s, the dev page answered %d", p.action, db, resp.StatusCode)
			}
		}
		w.d.db.set(works)
	}
	for _, l := range w.d.log.read() {
		if strings.Contains(l.Text, "dev page failed") || strings.Contains(l.Text, "panic") {
			t.Fatal(l.Text)
		}
	}
	for _, page := range []string{"/shop", "/thanks?txn=" + ref, "/lost", "/web/style.css"} {
		resp, err := w.client.Get(w.base + page)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", page, resp.StatusCode)
		}
	}
	w.audit()
}

// The checkout page and the thank-you page agree with the dispenser on
// the nonce: made at random in the browser, kept in the tab, passed under
// the name the dispenser reads, and sent in a body, never in the address.
func TestPagesCarryTheNonce(t *testing.T) {
	w := newWorld(t, 10)
	shop, thanks := w.page("/shop"), w.page("/thanks?txn=txn_1")
	for _, want := range []string{
		`name="` + dispenser.ThanksField + `"`,
		"crypto.getRandomValues(b)",
		"new Uint8Array(" + itoa(dispenser.NonceLen/2) + ")",
		`sessionStorage.setItem("thanks", n)`,
	} {
		if !strings.Contains(shop, want) {
			t.Errorf("the checkout page has no %s", want)
		}
	}
	for _, want := range []string{`sessionStorage.getItem("thanks")`, `method: "POST"`, "body: JSON.stringify({nonce: nonce})"} {
		if !strings.Contains(thanks, want) {
			t.Errorf("the thank-you page has no %s", want)
		}
	}
}

// Buyers, Paddle, the signer, the scheduled runs and someone at the
// dev page, all at once.
func TestAllAtOnce(t *testing.T) {
	w := newWorld(t, 20)
	var wg sync.WaitGroup
	refs := make(chan string, 64)
	for i := range 4 {
		wg.Go(func() {
			for j := range 6 {
				_, ref, n, err := checkout(w.client, w.base, "p"+itoa(i)+"@example.com", 1+j%3)
				if err != nil {
					t.Error(err)
					return
				}
				w.remember(ref, n)
				refs <- ref
				_, _ = w.thanks(ref)
			}
		})
	}
	wg.Go(func() {
		for range 20 {
			w.tick()
		}
	})
	wg.Go(func() {
		for range 20 {
			resp, err := w.client.Get(w.base + "/dev")
			if err == nil {
				resp.Body.Close()
			}
		}
	})
	wg.Go(func() {
		for range 10 {
			resp, err := w.client.PostForm(w.base+"/dev/cron-mail", nil)
			if err == nil {
				resp.Body.Close()
			}
		}
	})
	wg.Wait()
	close(refs)
	for range 5 {
		w.press("clock", url.Values{"by": {"5m"}})
	}
	for ref := range refs {
		if len(w.keysOf(ref)) == 0 {
			t.Errorf("%s has no keys", ref)
		}
	}
	w.audit()
}

func TestRunRefusesAnOutsideAddress(t *testing.T) {
	if err := run([]string{"dev", "-addr", "0.0.0.0:0"}, io.Discard); err == nil {
		t.Fatal("served on every address")
	}
	if err := run(nil, io.Discard); err == nil || !strings.Contains(err.Error(), "framefairy-dispenser dev") {
		t.Fatalf("no usage: %v", err)
	}
}

// The first buyer, the moment the address is printed, finds keys.
func TestTheFirstBuyerFindsKeys(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	printed := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, 10, writerFunc(func(p []byte) { close(printed) })) }()
	<-printed
	base := "http://" + ln.Addr().String()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	_, ref, n, err := checkout(client, base, "first@example.com", 3)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := client.Post(base+"/v1/thanks/"+ref, "application/json", strings.NewReader(`{"nonce":"`+n+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			break
		}
		if resp.StatusCode != http.StatusAccepted || time.Now().After(deadline) {
			t.Fatalf("the first buyer's thank-you page answered %d", resp.StatusCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
	client.CloseIdleConnections()
	start := time.Now()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("stopping took %v", took)
	}
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) { f(p); return len(p), nil }
