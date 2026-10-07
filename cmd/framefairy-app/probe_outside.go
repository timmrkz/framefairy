//go:build outside

package main

// The probe the checks from outside the app drive it through, see From
// outside the app in docs/TESTING.md. Only a build with the tag outside
// has it, and make outside is the only thing that makes one: it answers
// anybody on this machine who asks, and it presses buttons.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// probeAddr is where the probe listens. Fixed, because the test cannot
// hand an app that macOS starts anything to read a port from.
const probeAddr = "127.0.0.1:47290"

// An app macOS starts has nowhere to print to, so this build writes its
// log to a file as well, from the first line on. The test shows it when a
// check fails.
func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, "Library", "Logs")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "FrameFairy-outside.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
}

// probePage is the route the page sends what it shows to. It is on the
// app's own address, which the page can always reach.
const probePage = "/__outside/page"

// PageView is what the settings show, read out of the page itself.
type PageView struct {
	Seq int `json:"seq"`
	// Settings is whether the settings are on screen: the Licence field is.
	Settings bool `json:"settings"`
	// Key is what the Licence field holds.
	Key string `json:"key"`
	// Line is the words under Licence key.
	Line string `json:"line"`
	// Mark is the mark beside it: ok, err, or nothing.
	Mark   string `json:"mark"`
	Unlock string `json:"unlock"`
	// UnlockOff is whether Unlock can be pressed.
	UnlockOff bool `json:"unlockOff"`
}

// ProbeState is everything the probe says about the app.
type ProbeState struct {
	// Links is how many framefairy:// links the app was handed.
	Links   int          `json:"links"`
	Licence LicenceState `json:"licence"`
	// Focused is whether the app's window is the one in front.
	Focused bool     `json:"focused"`
	Page    PageView `json:"page"`
	// Problem says why the page could not be read, if it could not.
	Problem string `json:"problem,omitempty"`
}

type probe struct {
	svc *FrameFairy

	mu      sync.Mutex
	links   int
	seq     int
	waiting map[int]chan PageView
}

var theProbe = &probe{waiting: map[int]chan PageView{}}

// probeLink counts a link the app was handed.
func probeLink() {
	theProbe.mu.Lock()
	theProbe.links++
	theProbe.mu.Unlock()
}

// probeMiddleware takes what the page sends about itself and passes
// everything else on.
func probeMiddleware(next application.Middleware) application.Middleware {
	return func(h http.Handler) http.Handler {
		inner := next(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != probePage {
				inner.ServeHTTP(w, r)
				return
			}
			var view PageView
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&view); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			theProbe.mu.Lock()
			ch := theProbe.waiting[view.Seq]
			delete(theProbe.waiting, view.Seq)
			theProbe.mu.Unlock()
			if ch != nil {
				ch <- view
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// startProbe listens for the test. The app does not wait for it: a probe
// that cannot listen says so in the log, and the test finds nobody there.
func startProbe(svc *FrameFairy) {
	theProbe.svc = svc
	ln, err := net.Listen("tcp", probeAddr)
	if err != nil {
		log.Printf("outside probe: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", theProbe.state)
	mux.HandleFunc("POST /click", theProbe.click)
	mux.HandleFunc("POST /quit", theProbe.quit)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	log.Printf("outside probe on %s", probeAddr)
}

func (p *probe) state(w http.ResponseWriter, _ *http.Request) {
	s := ProbeState{Licence: p.svc.Licence()}
	p.mu.Lock()
	s.Links = p.links
	p.mu.Unlock()
	if p.svc.window != nil {
		s.Focused = p.svc.window.IsFocused()
	}
	view, err := p.readPage()
	if err != nil {
		s.Problem = err.Error()
	}
	s.Page = view
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

// readPage asks the page what it shows, and waits a moment for the
// answer, which comes back through probeMiddleware.
func (p *probe) readPage() (PageView, error) {
	if p.svc.window == nil {
		return PageView{}, errors.New("no window yet")
	}
	ch := make(chan PageView, 1)
	p.mu.Lock()
	p.seq++
	seq := p.seq
	p.waiting[seq] = ch
	p.mu.Unlock()
	p.svc.window.ExecJS(fmt.Sprintf(pageScript, seq, probePage))
	select {
	case v := <-ch:
		return v, nil
	case <-time.After(2 * time.Second):
		p.mu.Lock()
		delete(p.waiting, seq)
		p.mu.Unlock()
		return PageView{}, errors.New("the page did not answer")
	}
}

// pageScript reads the Licence row, found by its field's label: the
// settings have another field of the same kind, for the API keys.
const pageScript = `(() => {
  const field = document.querySelector('input[aria-label="Licence key"]');
  const card = field ? field.closest(".card") : null;
  const pick = (s) => (card ? card.querySelector(s) : null);
  const line = pick(".words .line");
  const mark = pick(".mark");
  const button = pick("button.unlock");
  const view = {
    seq: %d,
    settings: !!field,
    key: field ? field.value : "",
    line: line ? line.textContent.trim() : "",
    mark: mark ? (mark.classList.contains("ok") ? "ok" : mark.classList.contains("err") ? "err" : "") : "",
    unlock: button ? button.textContent.trim() : "",
    unlockOff: button ? button.disabled : true,
  };
  fetch(%q, { method: "POST", body: JSON.stringify(view) });
})();`

// click presses the button a selector names, the way a click does.
func (p *probe) click(w http.ResponseWriter, r *http.Request) {
	sel := r.URL.Query().Get("selector")
	if sel == "" || strings.ContainsAny(sel, "\n\r") || p.svc.window == nil {
		http.Error(w, "a selector, and a window to press it in", http.StatusBadRequest)
		return
	}
	quoted, _ := json.Marshal(sel)
	p.svc.window.ExecJS(fmt.Sprintf(`document.querySelector(%s)?.click();`, quoted))
	w.WriteHeader(http.StatusNoContent)
}

func (p *probe) quit(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		p.svc.app.Quit()
	}()
}
