package dispenser

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// paddle takes Paddle's webhooks. Only one that Paddle signed in the last
// five minutes is read, and of it only the reference of the sale: the
// dispenser then asks Paddle what the sale is, see Settle. A webhook that
// cannot be settled yet is answered with a failure, so Paddle sends it
// again. One the dispenser has nothing to do with is answered with
// success, so Paddle does not.
func (a *api) paddle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPaddle))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	if err := verifyPaddle(r.Header.Get("Paddle-Signature"), body, a.c.PaddleSecrets, a.e.now()); err != nil {
		a.c.Log.Warn("a Paddle webhook that does not verify", "why", err.Error())
		fail(w, http.StatusUnauthorized, "signature")
		return
	}
	ref, ok, err := paddleRef(body)
	if err != nil {
		a.c.Log.Error("a signed Paddle webhook that does not read", "error", err.Error())
		fail(w, http.StatusBadRequest, "invalid")
		return
	}
	if !ok {
		reply(w, http.StatusOK, map[string]bool{"ignored": true})
		return
	}
	if err := a.e.Settle(r.Context(), ref); err != nil {
		if errors.Is(err, ErrInvalid) {
			// It will never settle, so Paddle should stop sending it.
			a.c.Log.Error("a Paddle sale that cannot be settled", "ref", ref, "error", err.Error())
			reply(w, http.StatusOK, map[string]bool{"ignored": true})
			return
		}
		if errors.Is(err, ErrNotFound) {
			w.Header().Set("Retry-After", "60")
			fail(w, http.StatusServiceUnavailable, "not_yet")
			return
		}
		a.answer(w, r, err)
		return
	}
	reply(w, http.StatusOK, map[string]bool{"settled": true})
}

var (
	errNoSignature = errors.New("no signature")
	errSignature   = errors.New("the signature does not match")
	errStale       = errors.New("the signature is not of the last five minutes")
)

// verifyPaddle checks the Paddle-Signature header, ts=<unix>;h1=<hex>, an
// HMAC-SHA256 over "<ts>:<body>" with one of the secrets. More than one h1
// is accepted while Paddle rotates the secret. The time must be within
// five minutes of now either way, so an old webhook replayed is refused.
// Paddle's own libraries allow five seconds, but a replay inside the five
// minutes only makes Settle ask Paddle again. See
// https://developer.paddle.com/webhooks/about/signature-verification
func verifyPaddle(header string, body []byte, secrets []string, now time.Time) error {
	if header == "" || len(header) > 1024 {
		return errNoSignature
	}
	var ts string
	var sigs [][]byte
	for _, part := range strings.Split(header, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return errNoSignature
		}
		switch k {
		case "ts":
			if ts != "" {
				return errNoSignature
			}
			ts = v
		case "h1":
			sig, err := hex.DecodeString(v)
			if err != nil || len(sig) != sha256.Size {
				return errNoSignature
			}
			sigs = append(sigs, sig)
		}
	}
	if ts == "" || len(sigs) == 0 || len(sigs) > 4 {
		return errNoSignature
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || strconv.FormatInt(sec, 10) != ts {
		return errNoSignature
	}
	at := time.Unix(sec, 0)
	if now.Sub(at) > paddleWindow || at.Sub(now) > paddleWindow {
		return errStale
	}
	for _, secret := range secrets {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(ts))
		mac.Write([]byte(":"))
		mac.Write(body)
		want := mac.Sum(nil)
		for _, sig := range sigs {
			if hmac.Equal(sig, want) {
				return nil
			}
		}
	}
	return errSignature
}

// paddleRef is the sale a webhook is about, when it is an event the
// dispenser acts on.
func paddleRef(body []byte) (ref string, ok bool, err error) {
	var ev struct {
		EventType string `json:"event_type"`
		Data      struct {
			ID            string `json:"id"`
			TransactionID string `json:"transaction_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		return "", false, err
	}
	switch ev.EventType {
	case "transaction.completed":
		ref = ev.Data.ID
	case "adjustment.created", "adjustment.updated":
		ref = ev.Data.TransactionID
	default:
		return "", false, nil
	}
	if checkSale("paddle", ref) != nil {
		return "", false, errors.New("the webhook names no sale")
	}
	return ref, true, nil
}
