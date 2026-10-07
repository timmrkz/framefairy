package dispenser

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// FuzzVerifyPaddle feeds the signature check any header and body. It must
// never accept one that does not carry the true signature of that body.
func FuzzVerifyPaddle(f *testing.F) {
	body := []byte(`{"event_type":"transaction.completed","data":{"id":"txn_01"}}`)
	f.Add(signPaddle(body, paddleSecret, now), body)
	f.Add("ts=1;h1=00", body)
	f.Add("", []byte{})
	f.Add("ts=;h1=;h1=;;;", body)
	f.Fuzz(func(t *testing.T, header string, body []byte) {
		err := verifyPaddle(header, body, []string{paddleSecret}, now)
		if err != nil {
			return
		}
		// Hexadecimal reads the same in either case, so the signature is
		// looked for in lowercase.
		header = strings.ToLower(header)
		want := signPaddle(body, paddleSecret, now)
		mac := want[strings.Index(want, "h1=")+3:]
		if !strings.Contains(header, mac) {
			// Accepted at another second within the window: then that
			// second's signature must be in it.
			found := false
			for d := -paddleWindow; d <= paddleWindow; d += time.Second {
				s := signPaddle(body, paddleSecret, now.Add(d))
				if strings.Contains(header, s[strings.Index(s, "h1=")+3:]) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("accepted %q without its signature", header)
			}
		}
	})
}

// FuzzPaddleRef reads any signed body. It must never panic, and a
// reference it gives is always one the engine takes.
func FuzzPaddleRef(f *testing.F) {
	f.Add([]byte(`{"event_type":"transaction.completed","data":{"id":"txn_01"}}`))
	f.Add([]byte(`{"event_type":"adjustment.created","data":{"transaction_id":"txn_01"}}`))
	f.Add([]byte(`{"event_type":"adjustment.created","data":{"transaction_id":"../x"}}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		ref, ok, err := paddleRef(body)
		if ok && (err != nil || checkSale("paddle", ref) != nil) {
			t.Fatalf("%q gave %q", body, ref)
		}
	})
}

// FuzzRequests sends any body to the endpoints that take one from outside.
// Nothing may answer with a failure of ours.
func FuzzRequests(f *testing.F) {
	f.Add("/v1/lost", `{"email":"anna@example.com"}`)
	f.Add("/v1/orders", `{"ref":"1","seats":1,"email":"a@b.c"}`)
	f.Add("/v1/orders/1/revoke", `{"why":"refund"}`)
	f.Add("/v1/thanks/txn_01", `{"nonce":"`+nonceA+`"}`)
	f.Add("/v1/thanks/txn_01", `{"nonce":"`+nonceA+`","more":1}`)
	f.Add("/v1/lost", `{"email":`)
	f.Add("/v1/orders", `{"seats":1e400}`)
	w := newWeb(f, false)
	w.stock(50)
	f.Fuzz(func(t *testing.T, path, body string) {
		if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "#?%") {
			return
		}
		for i := 0; i < len(path); i++ {
			if path[i] <= ' ' || path[i] >= 0x7f {
				return
			}
		}
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bundleToken)
		rec := httptest.NewRecorder()
		w.h.ServeHTTP(rec, req)
		if rec.Code >= 500 && rec.Code != 503 {
			t.Fatalf("POST %s %q: %d %s", path, body, rec.Code, rec.Body)
		}
	})
}

// FuzzThanks reads any custom data as a sale's nonce, and sends the
// thank-you endpoint any nonce for a sale made with nonceA and one made
// with none. What a sale keeps of its custom data is always a hash or
// nothing, and the keys show only for the very nonce the sale was made
// with.
func FuzzThanks(f *testing.F) {
	f.Add(nonceA, nonceA)
	f.Add(nonceB, nonceB)
	f.Add("", "")
	f.Add(strings.ToUpper(nonceA), nonceA)
	f.Add(nonceA+"\x00", nonceA+" ")
	f.Add(`","x":"`, `","x":"`)
	w := newWeb(f, false)
	w.stock(2)
	w.sell("txn_a", 1, nonceA)
	w.sell("txn_b", 1, "")
	f.Fuzz(func(t *testing.T, custom, nonce string) {
		h := ThanksHash(custom)
		if (h != "") != checkNonce(custom) || (h != "" && (!tokenHash(h) || h != TokenHash(custom))) {
			t.Fatalf("custom data %q is kept as %q", custom, h)
		}
		if checkOrder(Order{Source: "paddle", Ref: "txn_1", Seats: 1, At: now, Thanks: h}) != nil {
			t.Fatalf("custom data %q would stop the sale", custom)
		}
		body, _ := json.Marshal(map[string]string{"nonce": nonce})
		for _, ref := range []string{"txn_a", "txn_b"} {
			req := httptest.NewRequest("POST", "/v1/thanks/"+ref, bytes.NewReader(body))
			rec := httptest.NewRecorder()
			w.h.ServeHTTP(rec, req)
			if (rec.Code == 200) != (ref == "txn_a" && nonce == nonceA) || rec.Code >= 500 {
				t.Fatalf("%s with nonce %q: %d %s", ref, nonce, rec.Code, rec.Body)
			}
		}
	})
}
