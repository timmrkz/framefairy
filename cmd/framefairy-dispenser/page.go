package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"framefairy/licence"
	"framefairy/licence/dispenser"
	"framefairy/licence/paddle"
)

//go:embed web
var web embed.FS

var pages = template.Must(template.New("").Funcs(template.FuncMap{
	"clock": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04:05")
	},
	"short": func(s string) string {
		if len(s) > 14 {
			return s[:14] + "…"
		}
		return s
	},
	"ahead": func(d time.Duration) string {
		if d == 0 {
			return "the real time"
		}
		days := int(d / (24 * time.Hour))
		rest := (d % (24 * time.Hour)).Round(time.Minute)
		if days > 0 {
			return fmt.Sprintf("%d d %s ahead", days, rest)
		}
		return fmt.Sprintf("%s ahead", rest)
	},
	"ok":   func(code int) bool { return code >= 200 && code < 300 },
	"dec":  func(n int) int { return n - 1 },
	"list": func(s ...string) []string { return s },
	"hm": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("Jan 2, 15:04")
	},
}).ParseFS(web, "web/dev.html"))

// The dev page's view of the world, read for one page.
type pageView struct {
	Now     time.Time
	Ahead   time.Duration
	Answer  *answer
	Shop    shopView
	Mail    string
	DB      string
	Refill  bool
	Sched   bool
	Pool    map[string]int
	Level   dispenser.Level
	Sales   []saleRow
	Partner []saleRow
	Queue   []dispenser.Mail
	Letters []letter
	Notes   []note
	Record  []dispenser.Line
	Revoked int
	Log     []logLine
	ReadErr string
	Broken  []string // what is switched away from normal, in words
}

type saleRow struct {
	Sale      sale
	TakenBack bool
	Ref       string
	Keys      []heldKey
	Pending   []paddle.Adjustment // refunds waiting for approval
	Standing  bool                // a chargeback that can be reversed
	Stuck     []webhook           // its webhooks not delivered yet, newest first
}

// look reads everything the dev page shows. It reads the store past the
// database switch, so a database that is down still shows what it holds.
func (d *dev) look(ctx context.Context) pageView {
	v := pageView{Now: d.clock.Now(), Ahead: d.clock.Ahead(), Shop: d.shop.view(), DB: d.db.state(), Log: d.log.read()}
	v.Mail, v.Letters, v.Notes = d.mail.read()
	slices.Reverse(v.Letters)
	slices.Reverse(v.Notes)
	d.mu.Lock()
	v.Answer, v.Refill, v.Sched = d.answer, d.refill, d.schedule
	d.mu.Unlock()
	v.Broken = broken(v)

	keysOf := func(tx dispenser.Tx, revoked map[licence.Fingerprint]dispenser.Revocation, seats []dispenser.Seat) []heldKey {
		var out []heldKey
		for _, s := range seats {
			k, err := tx.Key(s.Key)
			if err != nil {
				continue
			}
			out = append(out, heldKey{Key: k.Key, Fingerprint: s.Key.String(), Revoked: revoked[s.Key] != ""})
		}
		return out
	}
	err := d.db.Memory.View(ctx, func(tx dispenser.Tx) error {
		v.Pool = map[string]int{}
		for _, s := range []dispenser.State{dispenser.Unsold, dispenser.Sold, dispenser.Retired, dispenser.Burned} {
			n, err := tx.Count(s)
			if err != nil {
				return err
			}
			v.Pool[string(s)] = n
		}
		revoked, err := tx.Revoked()
		if err != nil {
			return err
		}
		v.Revoked = len(revoked)
		for _, sl := range v.Shop.Sales {
			seats, err := tx.Seats("paddle", sl.Ref)
			if err != nil && !errors.Is(err, dispenser.ErrNotFound) {
				return err
			}
			row := saleRow{Sale: sl, Ref: sl.Ref, TakenBack: sl.TakenBack(), Keys: keysOf(tx, revoked, seats)}
			for _, a := range sl.Adjs {
				if a.Action == paddle.Refund && a.Status == paddle.PendingApproval {
					row.Pending = append(row.Pending, a)
				}
				if a.Action == paddle.Chargeback && a.Status == paddle.Approved {
					row.Standing = true
				}
			}
			for _, wh := range v.Shop.Webhooks {
				if wh.Ref == sl.Ref && wh.State != whDelivered {
					row.Stuck = append(row.Stuck, wh)
				}
			}
			v.Sales = append(v.Sales, row)
		}
		all, err := tx.AllSeats()
		if err != nil {
			return err
		}
		byRef := map[string][]dispenser.Seat{}
		var refs []string
		for _, s := range all {
			if s.Source != "partner:"+partnerName {
				continue
			}
			if byRef[s.Ref] == nil {
				refs = append(refs, s.Ref)
			}
			byRef[s.Ref] = append(byRef[s.Ref], s)
		}
		for _, ref := range refs {
			v.Partner = append(v.Partner, saleRow{Ref: ref, Keys: keysOf(tx, revoked, byRef[ref])})
		}
		if v.Queue, err = tx.DueMail(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), 100); err != nil {
			return err
		}
		head, err := tx.Head()
		if errors.Is(err, dispenser.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		lines, err := tx.Lines(max(head.Seq-40, 0), 40)
		if err != nil {
			return err
		}
		slices.Reverse(lines)
		v.Record = lines
		return nil
	})
	if err != nil {
		v.ReadErr = err.Error()
	}
	return v
}

// broken says in words what the switches have changed from a world that
// works, so the page can say so at the top.
func broken(v pageView) []string {
	var out []string
	if v.Shop.API != works {
		out = append(out, "Paddle's API is down")
	}
	if v.Shop.Lag {
		out = append(out, "Paddle's API lags")
	}
	switch v.Shop.Mode {
	case sendTwice:
		out = append(out, "Paddle sends every webhook twice")
	case sendHold:
		out = append(out, "Paddle holds its webhooks")
	case sendLose:
		out = append(out, "Paddle loses its webhooks")
	}
	switch v.Mail {
	case down:
		out = append(out, "mail is down")
	case losesWord:
		out = append(out, "mail loses its answers")
	}
	switch v.DB {
	case down:
		out = append(out, "the database is down")
	case losesWord:
		out = append(out, "the database loses its answers")
	}
	if !v.Refill {
		out = append(out, "the signer is off")
	}
	if !v.Sched {
		out = append(out, "the scheduled runs are off")
	}
	return out
}

func (d *dev) page(w http.ResponseWriter, r *http.Request) {
	v := d.look(r.Context())
	if l, err := d.engine.PoolLevel(r.Context()); err == nil {
		v.Level = l
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pages.ExecuteTemplate(w, "dev.html", v); err != nil {
		slog.New(d.log).Error("the dev page failed", "error", err.Error())
	}
}

// act is one button on the dev page. What it did is shown at the top of
// the page it goes back to.
func (d *dev) act(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a, err := d.do(r.Context(), r.PathValue("action"), r.Form)
	if err != nil {
		a = answer{What: r.PathValue("action"), Code: 0, Body: err.Error()}
	}
	if a.What != "" {
		d.show(a)
	}
	http.Redirect(w, r, "/dev#"+r.Form.Get("back"), http.StatusSeeOther)
}

func (d *dev) do(ctx context.Context, action string, f url.Values) (answer, error) {
	get := func(k string) string { return strings.TrimSpace(f.Get(k)) }
	admin := func(name string, body map[string]string) (answer, error) {
		return d.call(ctx, "admin", http.MethodPost, "/v1/admin/"+name, body)
	}
	done := func(what string) (answer, error) { return answer{What: what, Code: http.StatusOK}, nil }
	switch action {
	case "clock":
		dur, err := time.ParseDuration(get("by"))
		if err != nil || dur <= 0 || dur > 30*24*time.Hour {
			return answer{}, errors.New("move the clock by 1m to 720h")
		}
		d.clock.Advance(dur)
		d.Tick(ctx)
		return done("Moved the pretend time " + strings.TrimSuffix(strings.TrimSuffix(dur.String(), "0s"), "0m") + " forward")
	case "paddle":
		v := d.shop.view()
		api, mode, lag := v.API, v.Mode, v.Lag
		if s := get("api"); s != "" {
			api = s
		}
		if s := get("mode"); s != "" {
			mode = s
		}
		if s := get("lag"); s != "" {
			lag = s == "on"
		}
		if (api != works && api != down) || !sendMode(mode) {
			return answer{}, errors.New("no such switch")
		}
		d.shop.set(api, mode, lag)
		return done(fmt.Sprintf("Paddle's API %s, its webhooks %s, lag %v", api, mode, lag))
	case "mail":
		if !behaviour(get("mode")) {
			return answer{}, errors.New("no such switch")
		}
		d.mail.set(get("mode"))
		return done("Mail: " + get("mode"))
	case "db":
		if !behaviour(get("mode")) {
			return answer{}, errors.New("no such switch")
		}
		d.db.set(get("mode"))
		return done("Database: " + get("mode"))
	case "auto":
		d.mu.Lock()
		if s := get("refill"); s != "" {
			d.refill = s == "on"
		}
		if s := get("schedule"); s != "" {
			d.schedule = s == "on"
		}
		d.mu.Unlock()
		return done("Switched")
	case "adjust":
		if err := d.shop.Adjust(get("ref"), get("do")); err != nil {
			return answer{}, err
		}
		d.shop.Deliver(ctx)
		return done(strings.ReplaceAll(strings.ToUpper(get("do")[:1])+get("do")[1:], "_", " ") + " at Paddle")
	case "decide":
		if err := d.shop.Decide(get("ref"), get("id"), get("approve") == "yes"); err != nil {
			return answer{}, err
		}
		d.shop.Deliver(ctx)
		return done("Paddle decided the refund")
	case "replay":
		if err := d.shop.Replay(get("ref")); err != nil {
			return answer{}, err
		}
		d.shop.Deliver(ctx)
		return done("Paddle sent the sale again")
	case "release":
		if err := d.shop.Release(get("id")); err != nil {
			return answer{}, err
		}
		d.shop.Deliver(ctx)
		return done("Paddle sent the held webhook")
	case "sign":
		n, err := strconv.Atoi(get("n"))
		if err != nil {
			return answer{}, errors.New("how many keys")
		}
		l, err := d.engine.PoolLevel(ctx)
		if err != nil {
			return answer{}, err
		}
		gen := get("generation")
		if gen == "" {
			gen = l.Generation
		}
		return d.Sign(ctx, n, gen)
	case "refill":
		return d.Refill(ctx)
	case "cron-mail":
		return d.call(ctx, "cron", http.MethodPost, "/v1/cron/mail", nil)
	case "cron-daily":
		return d.call(ctx, "cron", http.MethodPost, "/v1/cron/daily", nil)
	case "feed":
		return d.call(ctx, "release", http.MethodGet, "/v1/feed", nil)
	case "health":
		return d.call(ctx, "", http.MethodGet, "/healthz", nil)
	case "replace", "revoke", "restore":
		return admin(action, map[string]string{"source": get("source"), "ref": get("ref"), "fingerprint": get("fingerprint"), "why": get("why")})
	case "revoke-unsold", "retire":
		return admin(action, map[string]string{"why": get("why")})
	case "settle":
		return admin(action, map[string]string{"ref": get("ref")})
	case "reconcile":
		days, err := strconv.Atoi(get("days"))
		if err != nil || days < 0 {
			return answer{}, errors.New("how many days back")
		}
		return admin(action, map[string]string{"since": d.clock.Now().AddDate(0, 0, -days).Format(time.RFC3339)})
	case "audit":
		return admin(action, map[string]string{})
	case "partner-order":
		seats, _ := strconv.Atoi(get("seats"))
		return d.call(ctx, "partner", http.MethodPost, "/v1/orders", map[string]any{"ref": get("ref"), "seats": seats, "email": get("email")})
	case "partner-keys":
		return d.call(ctx, "partner", http.MethodGet, "/v1/orders/"+url.PathEscape(get("ref")), nil)
	case "partner-revoke":
		return d.call(ctx, "partner", http.MethodPost, "/v1/orders/"+url.PathEscape(get("ref"))+"/revoke", map[string]string{"why": get("why")})
	}
	return answer{}, fmt.Errorf("no action %q", action)
}
