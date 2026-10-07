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
	"strconv"
	"sync"
	"time"

	"framefairy/engine"
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
	// Settings is whether the settings are on screen: the Licence card is.
	Settings bool `json:"settings"`
	// Head is the name of the row, Licence key, or Licensed once a key is
	// kept.
	Head string `json:"head"`
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
	// Searches is every search the app knows of, as the clip list reads
	// them: running, or how one ended.
	Searches []SearchView `json:"searches"`
	// Problem says why the page could not be read, if it could not.
	Problem string `json:"problem,omitempty"`
}

// SearchView is one search, the way the clip list reads it.
type SearchView struct {
	Episode string  `json:"episode"`
	State   string  `json:"state"`
	Step    string  `json:"step,omitempty"`
	From    float64 `json:"from"`
	To      float64 `json:"to"`
	Error   string  `json:"error,omitempty"`
}

type probe struct {
	svc *FrameFairy

	mu      sync.Mutex
	links   int
	seq     int
	waiting map[int]chan json.RawMessage
}

var theProbe = &probe{waiting: map[int]chan json.RawMessage{}}

// probeLink counts a link the app was handed.
func probeLink() {
	theProbe.mu.Lock()
	theProbe.links++
	theProbe.mu.Unlock()
}

// probeMiddleware takes the page's answers to what the probe asked it,
// and passes everything else on.
func probeMiddleware(next application.Middleware) application.Middleware {
	return func(h http.Handler) http.Handler {
		inner := next(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != probePage {
				inner.ServeHTTP(w, r)
				return
			}
			var got struct {
				Seq    int             `json:"seq"`
				Answer json.RawMessage `json:"answer"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&got); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			theProbe.mu.Lock()
			ch := theProbe.waiting[got.Seq]
			delete(theProbe.waiting, got.Seq)
			theProbe.mu.Unlock()
			if ch != nil {
				ch <- got.Answer
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
	mux.HandleFunc("POST /add", theProbe.add)
	mux.HandleFunc("POST /search", theProbe.search)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	log.Printf("outside probe on %s", probeAddr)
}

func (p *probe) state(w http.ResponseWriter, _ *http.Request) {
	s := ProbeState{Licence: p.svc.Licence(), Searches: []SearchView{}}
	for _, j := range p.svc.jobs.list() {
		if j.Kind == engine.JobSearch {
			s.Searches = append(s.Searches, SearchView{Episode: j.Episode, State: j.State, Step: j.Step,
				From: j.From, To: j.To, Error: j.Error})
		}
	}
	p.mu.Lock()
	s.Links = p.links
	p.mu.Unlock()
	if p.svc.window != nil {
		s.Focused = p.svc.window.IsFocused()
	}
	answer, err := p.ask(pageScript)
	if err == nil {
		err = json.Unmarshal(answer, &s.Page)
	}
	if err != nil {
		s.Problem = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

// ask runs a script in the page and waits a moment for its answer, which
// comes back through probeMiddleware. The script is a function of the
// page that gives back what it found. It is passed the answer's number
// and the route to send it to.
func (p *probe) ask(script string) (json.RawMessage, error) {
	if p.svc.window == nil {
		return nil, errors.New("no window yet")
	}
	ch := make(chan json.RawMessage, 1)
	p.mu.Lock()
	p.seq++
	seq := p.seq
	p.waiting[seq] = ch
	p.mu.Unlock()
	p.svc.window.ExecJS(fmt.Sprintf(`((seq, route) => {
  let answer = null;
  try { answer = (%s)(); } catch (e) { answer = { problem: String(e) }; }
  fetch(route, { method: "POST", body: JSON.stringify({ seq, answer }) });
})(%d, %q);`, script, seq, probePage))
	select {
	case a := <-ch:
		return a, nil
	case <-time.After(2 * time.Second):
		p.mu.Lock()
		delete(p.waiting, seq)
		p.mu.Unlock()
		return nil, errors.New("the page did not answer")
	}
}

// pageScript reads the Licence row, found by the head of its group. Its
// field is there only while a key is asked for, and reads as empty when
// it is not.
const pageScript = `() => {
  const group = [...document.querySelectorAll(".group")].find(
    (g) => g.querySelector("h2")?.textContent.trim() === "Licence",
  );
  const card = group ? group.querySelector(".card") : null;
  const pick = (s) => (card ? card.querySelector(s) : null);
  const field = pick('input[aria-label="Licence key"]');
  const line = pick(".words .line");
  const mark = pick(".mark");
  const button = pick("button.unlock");
  return {
    settings: !!card,
    head: pick(".words .head")?.textContent.trim() ?? "",
    key: field ? field.value : "",
    line: line ? line.textContent.trim() : "",
    mark: mark ? (mark.classList.contains("ok") ? "ok" : mark.classList.contains("err") ? "err" : "") : "",
    unlock: button ? button.textContent.trim() : "",
    unlockOff: button ? button.disabled : true,
  };
}`

// clickScript presses the button on screen whose words are the ones
// given, the way a click does, and says whether there was one to press.
const clickScript = `() => {
  const words = %s;
  const button = [...document.querySelectorAll("button")].find(
    (b) => b.textContent.trim() === words && b.offsetParent !== null && !b.disabled,
  );
  if (button) button.click();
  return { clicked: !!button };
}`

// click presses a button, named by its words: POST /click?button=Unlock.
// It answers 404 when no button on screen says that and can be pressed.
func (p *probe) click(w http.ResponseWriter, r *http.Request) {
	words := r.URL.Query().Get("button")
	if words == "" {
		http.Error(w, "say which button, by its words", http.StatusBadRequest)
		return
	}
	quoted, _ := json.Marshal(words)
	answer, err := p.ask(fmt.Sprintf(clickScript, quoted))
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	var got struct {
		Clicked bool `json:"clicked"`
	}
	if json.Unmarshal(answer, &got) != nil || !got.Clicked {
		http.Error(w, "no button on screen says "+words, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (p *probe) quit(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		p.quitNow()
	}()
}

// quitNow quits the app the way the second Cmd+Q does: everything stops,
// and then the app goes. app.Quit alone asks ShouldQuit, which takes it for
// a first Cmd+Q and only asks, so the app stayed open.
func (p *probe) quitNow() {
	l := p.svc.leave
	l.stop()
	l.mu.Lock()
	l.going, l.gone = true, true
	l.mu.Unlock()
	p.svc.app.Quit()
}

// add adds a video to the library, the way Add does once the system's box
// hands it over: POST /add?video=path. Its first search starts by itself.
func (p *probe) add(w http.ResponseWriter, r *http.Request) {
	video := r.URL.Query().Get("video")
	added, err := p.svc.addEpisodes([]string{video})
	if err != nil || len(added) != 1 {
		http.Error(w, fmt.Sprintf("%s was not added: %v", video, err), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// search asks for a search of a window of a video, the way New does:
// POST /search?video=path&from=30&to=60. With quit=1 the app quits in the
// same moment, the way the second Cmd+Q does: everything is stopped first,
// then the app goes. The walk searching.mjs found a search asked for then
// was lost, see TestPathClosedTheMomentASearchIsAsked.
func (p *probe) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err1 := strconv.ParseFloat(q.Get("from"), 64)
	to, err2 := strconv.ParseFloat(q.Get("to"), 64)
	if err1 != nil || err2 != nil {
		http.Error(w, "say the window, from and to, in seconds", http.StatusBadRequest)
		return
	}
	set := p.svc.store.Settings()
	job := p.svc.Search(q.Get("video"), engine.PlanRequest{From: from, To: to, Count: 1, Min: set.Min, Max: set.Max})
	if q.Get("quit") == "1" {
		// Stopped before the answer goes, so the search is cut off where
		// it stands, in the moment it was asked for.
		p.svc.leave.stop()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if q.Get("quit") == "1" {
		go p.quitNow()
	}
}
