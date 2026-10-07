// Command framefairy-dispenser serves the dispenser of licence keys.
//
//	framefairy-dispenser dev [-addr 127.0.0.1:8090] [-batch 10]
//
// dev runs it on this machine with a pretend world around it: a pretend
// Paddle that sells, refunds and sends signed webhooks, a mail service
// that keeps every letter to be read, a database and both services that
// can be switched to failing, the test signer refilling the pool, and a
// clock that can be moved forward. Its dev page, at /dev, shows both
// sides and does everything through the dispenser's own endpoints. It
// keeps nothing: every start is an empty shop.
//
// Serving the real dispenser comes with the PostgreSQL store. See
// docs/LICENCE.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"framefairy/licence/dispenser"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "framefairy-dispenser:", err)
		os.Exit(1)
	}
}

const usage = `framefairy-dispenser dev [-addr 127.0.0.1:8090] [-batch 10]

  dev   the dispenser on this machine, with a pretend Paddle, mail service
        and signer, and a dev page at /dev to try every workflow by hand`

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "dev" {
		return fmt.Errorf("\n%s", usage)
	}
	fl := flag.NewFlagSet("dev", flag.ContinueOnError)
	addr := fl.String("addr", "127.0.0.1:8090", "where to serve, on this machine only")
	batch := fl.Int("batch", 10, "how many keys the signer signs at a time")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("%q is not a flag", fl.Arg(0))
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		// The pretend world has no secrets worth keeping, but its dev page
		// does whatever is asked, so it answers this machine only.
		return errors.New("-addr is on this machine only, 127.0.0.1 or localhost")
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, ln, *batch, out)
}

// serve runs the dispenser and its world on ln until ctx ends. The pool is
// filled before the address is printed, so the first buyer finds keys.
func serve(ctx context.Context, ln net.Listener, batch int, out io.Writer) error {
	base := "http://" + ln.Addr().String()
	dir, err := os.MkdirTemp("", "framefairy-dispenser-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	d, err := newDev(base, dir, batch, stderr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: d.handler(), ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	d.Tick(ctx)
	go d.run(ctx)
	fmt.Fprintf(out, "The dispenser runs at %s\n  dev page  %s/dev\n  checkout  %s/shop\nCtrl-C stops it, and it forgets everything.\n", base, base, base)
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	// The pretend world's own connections to the dispenser would hold the
	// shutdown up while they sit unused, so they go first. Whatever is
	// still running after a few seconds is cut off: nothing here is kept.
	d.client.CloseIdleConnections()
	d.shop.client.CloseIdleConnections()
	shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(shut); err != nil {
		return srv.Close()
	}
	return nil
}

// handler is the dispenser's endpoints with the dev page and the buyer's
// pages beside them, all on one address, the way our website and the
// dispenser will share their origin rules.
func (d *dev) handler() http.Handler {
	mux := http.NewServeMux()
	page := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			b, err := fs.ReadFile(web, "web/"+name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(b)
		}
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/dev", http.StatusFound) })
	mux.HandleFunc("GET /dev", d.page)
	mux.HandleFunc("POST /dev/{action}", d.act)
	mux.HandleFunc("GET /shop", page("shop.html"))
	mux.HandleFunc("GET /thanks", page("thanks.html"))
	mux.HandleFunc("GET /lost", page("lost.html"))
	mux.HandleFunc("GET /web/style.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := fs.ReadFile(web, "web/style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /shop/buy", d.buy)
	mux.Handle("/", d.api)
	return d.sameHost(mux)
}

// buy is the pretend checkout completing: Paddle sends its webhook, and
// the buyer lands on the thank-you page with the sale's reference, as
// Paddle's checkout hands it over. The checkout page passes its nonce as
// custom data, the way it goes to Paddle.js.
func (d *dev) buy(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	seats, err := strconv.Atoi(r.Form.Get("seats"))
	if err != nil {
		http.Error(w, "seats", http.StatusBadRequest)
		return
	}
	ref, err := d.shop.Buy(strings.TrimSpace(r.Form.Get("email")), seats, r.Form.Get(dispenser.ThanksField))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go d.shop.Deliver(context.WithoutCancel(r.Context()))
	http.Redirect(w, r, "/thanks?txn="+ref, http.StatusSeeOther)
}

// sameHost sends a browser that came by another name, localhost for
// 127.0.0.1, to the address the dispenser knows as its website: the
// thank-you and lost-key endpoints answer that origin only.
func (d *dev) sameHost(next http.Handler) http.Handler {
	want := strings.TrimPrefix(d.base, "http://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != want && r.Method == http.MethodGet {
			http.Redirect(w, r, d.base+r.URL.RequestURI(), http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
