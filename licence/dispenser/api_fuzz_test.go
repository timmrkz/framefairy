package dispenser

import (
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
