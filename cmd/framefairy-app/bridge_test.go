package main

// The bridge: the interface in a browser, against the real Go side.
//
// The preview in frontend/preview answers the interface's calls with a
// stand-in written in TypeScript, wails-stub.ts. It is quick and it can be
// made to do anything, but it is a second engine, and it drifts from the
// first: it laid captions out eight words at a time and knew nothing of
// pauses, so a caption breaking where a removed word had been could never
// be seen there. The bridge answers the same calls with the service the
// app runs, over a desk from the path tests: the real queue, engine, words
// and ffmpeg, with stand-ins only for the two models. See
// docs/TESTING.md.
//
// The browser side is frontend/preview/wails-bridge.ts, which takes the
// place of the Wails runtime: a call is a POST to /call, and what the Go
// side tells the interface comes as server-sent events on /events.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"framefairy/engine"
	"framefairy/updates"
)

// bus is what the Go side tells the interface, Wails's app.Event.Emit, for
// every browser listening. A nil bus tells nobody, which is the path
// tests.
type bus struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func newBus() *bus { return &bus{subs: map[chan []byte]struct{}{}} }

func (b *bus) send(name string, data any) {
	if b == nil {
		return
	}
	body, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.subs {
		// A browser that does not keep up misses an event rather than
		// holding up the queue that sent it, which is what Wails does too.
		// It is said, since a walk waiting on that event then waits for
		// good, and the reason is in what the bridge printed.
		select {
		case c <- body:
		default:
			fmt.Fprintf(os.Stderr, "the bridge dropped %q for a page that did not keep up\n", name)
		}
	}
}

func (b *bus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	c := make(chan []byte, 256)
	b.mu.Lock()
	b.subs[c] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.subs, c)
		b.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(": open\n\n"))
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case body := <-c:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
			flusher.Flush()
		}
	}
}

// callHandler answers a call the way Wails does: the method of the service
// by its name, the arguments by position, a leading context filled in, and
// the error, when there is one, as a call that failed with its message.
//
// svc is the service now: a restart of the app is a new one. own are the
// calls the bridge answers itself, a box from the system among them. held
// says the gate a call of a name waits behind until the walk opens it,
// and the reason it fails with instead of reaching it, if it does, see
// /hold. fresh says the bridge's machine is on its first run, see /fresh.
func callHandler(svc func() *FrameFairy, own map[string]func() (any, error),
	held func(name string) (<-chan struct{}, string), fresh func() bool) http.Handler {
	contextType := reflect.TypeFor[context.Context]()
	errorType := reflect.TypeFor[error]()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Name string            `json:"name"`
			Args []json.RawMessage `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name := call.Name[strings.LastIndex(call.Name, ".")+1:]
		gate, fails := held(name)
		if gate != nil {
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		if fails != "" {
			http.Error(w, fails, http.StatusInternalServerError)
			return
		}
		if answerOwn, ok := own[name]; ok {
			answer, err := answerOwn()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(answer)
			return
		}
		method := reflect.ValueOf(svc()).MethodByName(name)
		if !method.IsValid() {
			http.Error(w, "the service has no method "+name, http.StatusNotFound)
			return
		}
		kind := method.Type()
		if outside[name] && !(walkedUpdates[name] && svc().updates != nil) && !(firstRun[name] && fresh()) {
			// Answered as having nothing to say, in the shape the
			// interface expects.
			var answer any
			if kind.NumOut() > 0 && kind.Out(0) != errorType {
				answer = reflect.Zero(kind.Out(0)).Interface()
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(answer)
			return
		}
		var in []reflect.Value
		next := 0
		for i := range kind.NumIn() {
			at := kind.In(i)
			if i == 0 && at == contextType {
				in = append(in, reflect.ValueOf(r.Context()))
				continue
			}
			v := reflect.New(at)
			if next < len(call.Args) {
				if err := json.Unmarshal(call.Args[next], v.Interface()); err != nil {
					http.Error(w, fmt.Sprintf("%s argument %d: %v", name, next+1, err), http.StatusBadRequest)
					return
				}
			}
			next++
			in = append(in, v.Elem())
		}
		if next != len(call.Args) {
			http.Error(w, fmt.Sprintf("%s takes %d arguments, got %d", name, next, len(call.Args)), http.StatusBadRequest)
			return
		}
		out, failed := callSafely(method, in)
		if failed != "" {
			http.Error(w, failed, http.StatusInternalServerError)
			return
		}
		var answer any
		if name == "Setup" || name == "CheckSetup" {
			if name == "Setup" {
				answer = standInsSetup(out[0].Interface().(SetupState), fresh())
			} else {
				answer = standInsChecks(out[0].Interface().([]Check))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(answer)
			return
		}
		for _, v := range out {
			if v.Type() == errorType {
				if !v.IsNil() {
					http.Error(w, v.Interface().(error).Error(), http.StatusInternalServerError)
					return
				}
				continue
			}
			answer = v.Interface()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(answer)
	})
}

// outside are the calls that reach beyond the work folder: the network,
// the keychain, a box from the system, another app. The bridge answers
// them itself, with nothing, so no walk ever downloads a model, installs a
// build or opens Finder, whatever it presses. A bridge that answered the
// first-run screen with the real thing started fetching the speech model,
// half a gigabyte, the moment the page opened.
var outside = map[string]bool{
	"Updates": true, "FollowChannel": true, "CheckForUpdates": true, "RestartToUpdate": true,
	"InstallSpeechModel": true, "InstallLanguageModel": true,
	"OpenKeysPage": true, "SaveAPIKey": true, "OpenCommit": true,
	"Reveal": true, "ChooseFolder": true, "Chrome": true, "StayOpen": true,
	"ShowAppLog": true,
}

// walkedUpdates are the calls of the Updates page a walk can have answered
// by the app's own updating, once it has asked for one, see /updates.
// Until then they are outside like the rest. Relaunch is outside always.
var walkedUpdates = map[string]bool{"Updates": true, "FollowChannel": true, "CheckForUpdates": true}

// firstRun are the downloads of the first run, which reach the app once a
// walk has made the bridge's machine new, see /fresh. Each then downloads
// from the bridge's stand-in, see arriving, never the internet.
var firstRun = map[string]bool{"InstallSpeechModel": true, "InstallLanguageModel": true}

// standInsSetup is the setup as the bridge's machine has it: the real
// answer, with the two stand-in models counted as there, since they are
// what hears and what finds clips here. Without it the app opens on its
// first-run screen, which the bridge is only about once a walk made it
// new, see /fresh. Then the models are what the real answer says, the
// ones the walk let arrive, and the machine is Tim's Mac, 32 GB, so the
// models offered are the same on every machine the walk runs on.
func standInsSetup(s SetupState, fresh bool) SetupState {
	s.HasServer = true
	if fresh {
		s.Memory = 32 << 30
		for i := range s.Language {
			s.Language[i].Fit = string(s.Language[i].FitsIn(s.Memory))
		}
		return s
	}
	s.HasSpeech, s.HasLocalModel, s.Chosen, s.Ready = true, true, true, true
	if len(s.Speech) > 0 {
		s.Speech[0].Installed = true
	}
	// The stand-in finds the clips, so a model is chosen, the way a
	// machine that has searched has one: the one the settings name, or
	// the one the app offers. Without it the settings ask for a choice,
	// and hold the app until one is made.
	chosen := slices.IndexFunc(s.Language, func(m LanguageModelView) bool { return m.InUse })
	if chosen < 0 {
		chosen = slices.IndexFunc(s.Language, func(m LanguageModelView) bool { return m.Recommended })
	}
	for i := range s.Language {
		s.Language[i].InUse = i == chosen
		s.Language[i].Installed = i == chosen
	}
	return s
}

// standInsChecks is the setup check as the bridge's machine has it: the
// two models are there, and what runs the language model, since the
// stand-ins hear and find the clips.
func standInsChecks(checks []Check) []Check {
	for i := range checks {
		switch checks[i].Name {
		case "Speech model", "Language model", "llama-server":
			checks[i].OK, checks[i].Detail = true, ""
		}
	}
	return checks
}

// callSafely calls a method and turns a panic into the reason the call
// failed, so a method that needs what the bridge does not have, the
// updater among them, fails that call and not the bridge.
func callSafely(method reflect.Value, in []reflect.Value) (out []reflect.Value, failed string) {
	defer func() {
		if r := recover(); r != nil {
			failed = fmt.Sprintf("the call panicked: %v", r)
		}
	}()
	return method.Call(in), ""
}

// bridgeHandler is everything the browser asks for: the calls, the events,
// the episode's files the way the app serves them, the interface itself,
// built into dist, and what a walk asks of the bridge, see bridge.
func bridgeHandler(b *bridge, dist string) http.Handler {
	mux := http.NewServeMux()
	svc := func() *FrameFairy { return b.d.svc }
	// The Add button asks the system for files, and the system's box is
	// the bridge: it hands over what a walk picked, see /pick, or nothing,
	// the way the box answers Cancel.
	own := map[string]func() (any, error){
		"AddEpisodes": func() (any, error) {
			b.mu.Lock()
			picked := b.picks
			b.picks = nil
			b.mu.Unlock()
			if len(picked) == 0 {
				return Added{Added: []string{}}, nil
			}
			return b.d.svc.takeFiles("add", picked), nil
		},
		// The title bar, as macOS answered it if a walk said so, see
		// /chrome, or nothing, the way every other system answers.
		"Chrome": func() (any, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			return b.chrome, nil
		},
	}
	mux.Handle("/call", callHandler(svc, own, b.held, b.fresh.Load))
	mux.Handle("/events", b.d.bus)
	answer := func(w http.ResponseWriter, v any, err error) {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		answer(w, nil, b.reset())
	})
	// A mark sent down the events behind everything the Go side told the
	// interface before it. A page that has heard the mark has heard all
	// of that, since every page hears the events in the order they were
	// sent, so a walk can wait until the interface knows what the engine
	// knew when the walk asked it, see caughtUp in bridge.mjs.
	mux.HandleFunc("/mark", func(w http.ResponseWriter, r *http.Request) {
		n := b.marks.Add(1)
		b.d.bus.send("mark", n)
		answer(w, n, nil)
	})
	mux.HandleFunc("/pick", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		path, err := b.pick(q.Get("seconds"), q.Get("rate"), q.Get("switch"), q.Get("timing"), q.Get("codec") == "hevc",
			q.Get("namesake") == "1", q.Get("box") != "0", q.Get("in") == "mkv", cameraHeight(q.Get("size")))
		answer(w, path, err)
	})
	// Files dropped on the app, which the window hands over the way the
	// runtime does once the drop landed on a drop target, see
	// wails-bridge.ts: what WindowFilesDropped does in main.go.
	mux.HandleFunc("/drop", func(w http.ResponseWriter, r *http.Request) {
		var files []string
		if err := json.NewDecoder(r.Body).Decode(&files); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dropped := b.d.svc.dropFiles(files)
		b.d.bus.send("dropped", dropped)
		answer(w, dropped, nil)
	})
	// Files dropped on the report window, which bugs.go attaches and
	// tells the window of, what WindowFilesDropped does on it in main.go.
	mux.HandleFunc("/drop-report", func(w http.ResponseWriter, r *http.Request) {
		var files []string
		if err := json.NewDecoder(r.Body).Decode(&files); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.d.svc.report.dropped(files)
		answer(w, nil, nil)
	})
	// The report window, which Bugs opens and the walk opens as a second
	// page on ?window=report: how many the app made and brought forward,
	// whether one is open, and with ?close=1 it closes, the way Cmd+W
	// does. See bugs.go.
	mux.HandleFunc("/report-window", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("close") == "1" {
			b.d.svc.report.closeWindow("window")
		}
		answer(w, map[string]any{
			"made": b.d.reportMade.Load(), "fronted": b.d.reportFronted.Load(), "open": b.d.svc.ReportWindowOpen(),
		}, nil)
	})
	// A picture of w by h pixels, for the paperclip's box to hand over
	// next, or with &box=0 only made, for a walk to drop. With
	// &kind=text it is a file of text named like a picture.
	mux.HandleFunc("/pick-picture", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		path, err := b.pickPicture(q.Get("name"), q.Get("w"), q.Get("h"), q.Get("kind"), q.Get("box") != "0")
		answer(w, path, err)
	})
	// What the last report on the bridge's Desktop holds, by name.
	mux.HandleFunc("/report-made", func(w http.ResponseWriter, r *http.Request) {
		if file := r.URL.Query().Get("file"); file != "" {
			text, err := lastReportFile(filepath.Join(b.d.home, "Desktop"), file)
			answer(w, text, err)
			return
		}
		names, err := lastReport(filepath.Join(b.d.home, "Desktop"))
		answer(w, names, err)
	})
	// A file of sound alone, named like a video, which Add has to leave
	// out: the episode's decoder finds no picture in it.
	mux.HandleFunc("/pick-sound", func(w http.ResponseWriter, r *http.Request) {
		path, err := b.pickSound()
		answer(w, path, err)
	})
	mux.HandleFunc("/model", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// How it answers first, so what it held answers that way.
		b.d.model.fails(q.Get("fail") == "1")
		b.d.model.hangs(q.Get("hang") == "1")
		answer(w, nil, nil)
	})
	// How many answers the model holds now, so a walk knows how many
	// searches wait on it and have nothing to report.
	mux.HandleFunc("/model-holding", func(w http.ResponseWriter, r *http.Request) {
		answer(w, b.d.model.holding.Load(), nil)
	})
	// The person dictating stops talking, and the microphone hears
	// nothing more once the phrase it is on has been said. Or the
	// recording in hand goes on, a step at a time, until what the walk
	// waits for has been told the window, see sound: until=hearing until
	// words are told as being heard, until=said until more words heard
	// are settled, until=settled until some are, now or before,
	// until=ended until the recording is over, and until=tenth for one
	// step. It answers how many steps it took.
	mux.HandleFunc("/microphone", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("silent") {
			b.sound.silent.Store(q.Get("silent") == "1")
			answer(w, nil, nil)
			return
		}
		steps, err := b.sound.until(r.Context(), q.Get("until"))
		answer(w, steps, err)
	})
	mux.HandleFunc("/speech", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("wordless") {
			b.words.wordless.Store(q.Get("wordless") == "1")
			answer(w, nil, nil)
			return
		}
		if q.Get("hold") == "one" {
			b.words.holdOne()
			answer(w, nil, nil)
			return
		}
		if q.Has("hold") {
			b.words.hold()
			answer(w, nil, nil)
			return
		}
		if n, err := strconv.Atoi(q.Get("step")); err == nil {
			b.words.save.Store(q.Has("save"))
			b.words.step(min(max(n, 0), 100))
			answer(w, nil, nil)
			return
		}
		b.words.release()
		if from, err := strconv.Atoi(q.Get("from")); err == nil && from >= 0 {
			b.words.from(from)
		}
		answer(w, nil, nil)
	})
	// The app's window as macOS lays it out, in points: the title bar's
	// height and where its buttons start and end. It goes through what
	// the Go side does with the answer and is told the way a resize is.
	mux.HandleFunc("/chrome", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		num := func(k string) float64 { v, _ := strconv.ParseFloat(q.Get(k), 64); return v }
		now := (&chromeWatch{}).settle(chromeRaw{bar: num("bar"), left: num("left"), right: num("right")})
		b.mu.Lock()
		b.chrome = now
		b.mu.Unlock()
		b.d.bus.send(chromeEvent("app"), now)
		answer(w, now, nil)
	})
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// times is how many calls of the name from now on the gate is for,
		// every call when it is not given.
		times, err := strconv.Atoi(q.Get("times"))
		if err != nil {
			times = -1
		}
		b.mu.Lock()
		switch g, shut := b.gates[q.Get("call")]; {
		case q.Get("gate") == "shut" && !shut:
			b.gates[q.Get("call")] = &gate{open: make(chan struct{}), left: min(times, 100)}
		case q.Get("gate") == "shut":
			g.left = min(times, 100)
		case q.Get("gate") == "open" && shut:
			close(g.open)
			delete(b.gates, q.Get("call"))
		}
		if reason := q.Get("fail"); reason != "" {
			b.fails[q.Get("call")] = reason
		}
		b.mu.Unlock()
		answer(w, nil, nil)
	})
	mux.HandleFunc("/updates", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var list []string
		if l := q.Get("list"); l != "" {
			list = strings.Split(l, ",")
		}
		var conflict []string
		if c := q.Get("conflict"); c != "" {
			conflict = strings.Split(c, ",")
		}
		answer(w, nil, b.updatesAs(q.Get("from"), q.Get("stored"), list, conflict, q.Get("offline") != ""))
	})
	// What the app last put on the clipboard, which the bridge stands in
	// for, see desk.start.
	mux.HandleFunc("/clipboard", func(w http.ResponseWriter, r *http.Request) {
		answer(w, b.d.copied(), nil)
	})
	// The file the app last saved through the save box, its name and what
	// it holds, or nothing, see desk.start.
	mux.HandleFunc("/exported", func(w http.ResponseWriter, r *http.Request) {
		p := b.d.exported.Load()
		if p == nil {
			answer(w, nil, nil)
			return
		}
		body, err := afero.ReadFile(b.d.svc.store.files, *p)
		answer(w, map[string]string{"name": filepath.Base(*p), "text": string(body)}, err)
	})
	// The app's log, which the bridge keeps none of until a walk asks:
	// with ?open=1 it starts one in the bridge's home with a few lines and
	// an older file beside it, the way a log that has turned over once
	// looks. Without it, what the log holds. Settings, Advanced shows it,
	// and the next walk starts with none again, see reset.
	mux.HandleFunc("/app-log", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("open") == "1" {
			answer(w, nil, b.openLog())
			return
		}
		answer(w, theLog.status(), nil)
	})
	// The bridge's machine on its first run: the models gone, no video in
	// the app and nothing yet said about how clips are found, so the app
	// opened again shows its setup. A download pressed there waits part
	// way until the walk lets it arrive with ?arrive=speech or
	// ?arrive=clip, see arriving. The next reset makes the machine the
	// bridge's again.
	mux.HandleFunc("/fresh", func(w http.ResponseWriter, r *http.Request) {
		if kind := r.URL.Query().Get("arrive"); kind != "" {
			answer(w, nil, b.letArrive(kind))
			return
		}
		answer(w, nil, b.makeFresh())
	})
	mux.HandleFunc("/reopen", func(w http.ResponseWriter, r *http.Request) {
		closed, err := b.reopen()
		answer(w, closed, err)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaMiddleware(b.d.svc.store)(reportMiddleware(b.d.svc.report)(http.FileServer(http.Dir(dist)))).ServeHTTP(w, r)
	}))
	return mux
}

// prose is the stand-in speech model of the bridge. It says sentences
// rather than one word over and over, so a word on screen twice, or one
// in the wrong place, can be told from the words around it. Each sentence
// ends in a pause long enough to end a caption.
type prose struct {
	mu sync.Mutex
	// next is the word it says next, counted on through proseText.
	next int
	// wordless hears no word at all, the way a video with no speech is
	// heard, so a walk can make a clip by hand that fails. Not the
	// bridge's silent, which is the microphone's.
	wordless atomic.Bool
	// save has what is heard so far saved once the next piece is heard,
	// asked for with a step, see SaveHeard. Otherwise a transcript is
	// saved only once it is all heard, never by the clock.
	save atomic.Bool
	// held keeps it from hearing anything more, so a walk can look at a
	// transcript part way that stays where it is, or press something
	// while it hears. /speech?go or a reset lets it go on. steps is how
	// many pieces it may still hear while it is held, so a walk can let
	// the hearing on one piece at a time.
	// hearing counts the pieces it is hearing now.
	gate    sync.Mutex
	turn    *sync.Cond
	held    bool
	steps   int
	hearing int
	// waiting counts the pieces handed to it while it is held, which the
	// engine is part way through as far as it knows.
	waiting int
	// closing lets every piece through while the app closes, so a piece
	// the walk holds ends the way a piece being heard ends as the app
	// quits. It stays held after.
	closing bool
}

func (p *prose) cond() *sync.Cond {
	if p.turn == nil {
		p.turn = sync.NewCond(&p.gate)
	}
	return p.turn
}

// hold stops it hearing and returns once the pieces it was hearing are
// heard, so the edge of what is heard stands still from then on.
func (p *prose) hold() {
	p.gate.Lock()
	defer p.gate.Unlock()
	p.held, p.steps = true, 0
	for p.hearing > 0 {
		p.cond().Wait()
	}
}

// holdOne holds it and returns once a piece is waiting in it, so a walk
// can stop the work while a piece is being heard, the way a person does.
func (p *prose) holdOne() {
	p.hold()
	p.gate.Lock()
	defer p.gate.Unlock()
	for p.held && p.waiting == 0 {
		p.cond().Wait()
	}
}

func (p *prose) release() {
	p.gate.Lock()
	defer p.gate.Unlock()
	p.held, p.steps = false, 0
	p.cond().Broadcast()
}

// step lets n pieces more be heard while it is held, by whatever hears
// next. It does not wait for them: a hearing called off between two
// pieces never asks for the next, and a walk waits on the news of the
// work instead.
func (p *prose) step(n int) {
	p.gate.Lock()
	defer p.gate.Unlock()
	p.steps += n
	p.cond().Broadcast()
}

// whileClosing lets every piece through for as long as close takes, and
// hands back what it said.
func (p *prose) whileClosing(close func() bool) bool {
	p.gate.Lock()
	p.closing = true
	p.cond().Broadcast()
	p.gate.Unlock()
	defer func() {
		p.gate.Lock()
		p.closing = false
		p.gate.Unlock()
	}()
	return close()
}

var proseText = strings.Fields(`Ich war vielleicht sechs Jahre alt, als mich auf dem Schulhof
irgendein Typ geschubst hat. Mein Arm ist dabei zersprungen, so hat es sich
jedenfalls angefühlt. Meine Mutter hat mich abgeholt und wir sind sofort ins
Krankenhaus gefahren. Der Arzt hat gelacht und gesagt, das ist nur
verstaucht. Ich habe trotzdem eine Woche lang einen Verband getragen und
allen erzählt, er wäre gebrochen. Das war meine erste richtige Lüge, glaube
ich. Und eigentlich erinnere ich mich an die Lüge besser als an den Arm.`)

func (p *prose) Recognize(samples []float32, rate int) []engine.Token {
	p.gate.Lock()
	if p.held && p.steps == 0 && !p.closing {
		p.waiting++
		p.cond().Broadcast()
		for p.held && p.steps == 0 && !p.closing {
			p.cond().Wait()
		}
		p.waiting--
	}
	if p.held && !p.closing {
		p.steps--
	}
	p.hearing++
	p.gate.Unlock()
	defer func() {
		p.gate.Lock()
		p.hearing--
		p.cond().Broadcast()
		p.gate.Unlock()
	}()
	if p.wordless.Load() {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	length := float64(len(samples)) / float64(rate)
	var tokens []engine.Token
	for at := 0.1; ; {
		word := proseText[p.next%len(proseText)]
		// Longer words take longer to say.
		lasts := 0.12 + 0.05*float64(len([]rune(word)))
		if at+lasts > length-0.05 {
			break
		}
		tokens = append(tokens, engine.Token{Text: " " + word, Start: at, Duration: lasts})
		p.next++
		at += lasts + 0.08
		if strings.HasSuffix(word, ".") {
			at += 0.9
		}
	}
	return tokens
}

func (*prose) Close() {}

// SaveHeard says whether what is heard so far is saved now, which the
// engine asks in place of its clock: once after a step that asked for it.
func (p *prose) SaveHeard() bool { return p.save.Swap(false) }

// at is the word it says next.
func (p *prose) at() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.next
}

// from makes word n the one it says next.
func (p *prose) from(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next = n
}

// TestBridge serves the interface against the real Go side until it is
// stopped, for a person or a probe to look at. It only runs when asked:
//
//	FRAMEFAIRY_BRIDGE=127.0.0.1:8123 go test -run TestBridge -timeout 0 ./cmd/framefairy-app
//
// with the interface built for it first:
//
//	cd frontend && npx vite build --config preview/bridge.config.ts
func TestBridge(t *testing.T) {
	addr := os.Getenv("FRAMEFAIRY_BRIDGE")
	if addr == "" {
		t.Skip("FRAMEFAIRY_BRIDGE is not set")
	}
	b := openBridge(t)
	server := &http.Server{Handler: bridgeHandler(b, bridgeDist(t))}
	// Port 0 is any free port, and the line below says which one.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	// Stopped, it closes, so the test ends and takes its folder with it.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		_ = server.Close()
	}()
	// Started by TestWalks, it also closes when its standard input does,
	// so a walk run that was killed leaves no bridge behind.
	if os.Getenv("FRAMEFAIRY_BRIDGE_WALKER") != "" {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			_ = server.Close()
		}()
	}
	fmt.Printf("bridge on http://%s with %s\n", listener.Addr(), b.first)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}

// bridge is the app on a desk, with one episode heard and searched, and
// what a walk can ask of it besides what the interface asks: a file for
// the Add button's box, a language model that holds its answers or fails,
// the app closed and opened again, and everything put back the way it was
// first searched.
type bridge struct {
	t     *testing.T
	d     *desk
	words *prose
	first string
	// kept is the episode's work folder and the app's settings as they
	// were once the episode was first searched.
	kept string
	// said is the word the speech stand-in was to say next then.
	said  int
	mu    sync.Mutex
	picks []string
	made  int
	// videos are the videos made for walks before, see /pick.
	videos *made
	// gates are shut for the calls of a name, every one or the next so
	// many, which wait until the walk opens it, the way a busy machine
	// delivers them late, see /hold. A walk that holds a call by a time
	// proves only that the time was long enough on that machine, so it
	// holds it until it has looked. Each call waiting holds one of the
	// browser's six connections to the bridge, and the events hold
	// another, so a walk shuts a gate for every call only on a call the
	// page makes a few of at a time, like Captions.
	gates map[string]*gate
	// fails is the reason the next call of a name fails with instead of
	// reaching the service, the way a full disk answers a save, see /hold.
	fails map[string]string
	// chrome is the app's title bar as macOS answers it, once a walk
	// asked for one, see /chrome.
	chrome Chrome
	// channels serves the channel list and the builds the Updates page is
	// walked with, see /updates.
	channels *channelServer
	// sound is the bridge's microphone and the clocks of a recording,
	// which a walk drives, see /microphone.
	sound *sound
	// marks counts the marks sent down the events, see /mark.
	marks atomic.Int64
	// fresh is the bridge's machine on its first run: no model on it and
	// no video in the app, see /fresh.
	fresh atomic.Bool
	// arrive lets the download of a kind of model, "speech" or "clip",
	// arrive, closed by the walk, see arriving.
	arrive map[string]chan struct{}
}

// updatesAs gives the app updates the way a walk asks for them: a build
// from the channel from, or one made on the Mac when from is empty, with
// stored in updates.json, picked by a run before, and these channels on
// the list, those in conflict said to no longer merge into main. It is
// the app's own updating and Wails' updater, reading a
// channel list served on this machine, the one the Go tests of updates
// use, so nothing reaches the network. Every channel has a build newer
// than the one running, which downloads in a moment. The app opened again
// has none, the way the bridge started. Offline, the list is read from a
// port nothing listens on, the way a Mac without a network answers.
//
// On the real disk: an update is a program that is downloaded, kept and run.
func (b *bridge) updatesAs(from, stored string, list, conflict []string, offline bool) error {
	for _, ch := range append(append([]string{from, stored}, list...), conflict...) {
		if ch != "" && !updates.ValidChannel(ch) {
			return fmt.Errorf("%q is not a channel", ch)
		}
	}
	b.mu.Lock()
	if b.channels == nil {
		b.channels = newChannelServer(b.t)
	}
	cs := b.channels
	b.mu.Unlock()
	cs.mu.Lock()
	cs.conflict = map[string]bool{}
	for _, ch := range conflict {
		cs.conflict[ch] = true
	}
	cs.mu.Unlock()
	var builds [][3]string
	for _, ch := range list {
		builds = append(builds, [3]string{ch, "0.3.0-" + strings.ReplaceAll(ch, "-", "") + ".1", ch})
	}
	cs.publish(b.t, builds...)
	c := &updating{
		own:  from,
		idle: b.d.svc.jobs.closeIfIdle,
		emit: func(s UpdateState) { b.d.bus.send("updates", s) },
		load: func() string { return stored },
		save: func(string) error { return nil },
	}
	src := &updates.Source{URL: cs.srv.URL + "/channels.json", Client: cs.srv.Client(), Cache: b.t.TempDir()}
	if offline {
		gone, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		src.URL = "https://" + gone.Addr().String() + "/channels.json"
		_ = gone.Close()
	}
	c.setUp(updater.New(quietHost{}), cs.publicKey(), src, inApp, "darwin")
	if c.state.Off != "" {
		return errors.New(c.state.Off)
	}
	b.t.Cleanup(func() {
		c.shutDown()
		if p := c.staged(); p != "" {
			_ = os.RemoveAll(filepath.Dir(p))
		}
	})
	c.start()
	b.d.svc.updates = c
	return nil
}

// gate holds the calls of a name until a walk opens it: every call when
// left is below nought, or the next left of them.
type gate struct {
	open chan struct{}
	left int
}

// held is the gate a call of this name waits behind, and the reason it
// fails with instead, if it does, see /hold.
func (b *bridge) held(name string) (<-chan struct{}, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fails := b.fails[name]
	delete(b.fails, name)
	g := b.gates[name]
	if g == nil || g.left == 0 {
		return nil, fails
	}
	if g.left > 0 {
		g.left--
	}
	return g.open, fails
}

// openBridge opens a desk whose Go side tells a browser what it does, with
// one episode in it, heard and searched, so the workspace has a clip.
//
// On the real disk: ffmpeg makes the episode, and ffmpeg and the episode's decoder read it.
func openBridge(t *testing.T) *bridge {
	t.Helper()
	d := open(t)
	words := &prose{}
	fakes := func(e *engine.Engine, o *engine.Options) {
		e.OpenRecognizer = func(string) (engine.Recognizer, error) { return words, nil }
		o.LLMURL = d.model.server.URL
		o.Width, o.Height = 360, 640
		o.Preset = "ultrafast"
	}
	standIns.Store(&fakes)
	// A font installed on the bridge's computer: Anton under a name of its
	// own, so the Font list has a face of the machine to offer, and libass
	// and the word typed are drawn from its copy.
	anton, _ := engine.FontBytes("Anton")
	narrow, err := engine.NamedFace(anton, "Bridge Narrow")
	if err != nil {
		t.Fatal(err)
	}
	installed := afero.NewMemMapFs()
	if err := afero.WriteFile(installed, "/Library/Fonts/Bridge Narrow.ttf", narrow, 0o644); err != nil {
		t.Fatal(err)
	}
	// A family of two styles, Regular and Heavy, out of Archivo Black and
	// Inter Black, so picking it keeps the look of the face it replaces.
	// And enough families after it for the list to be longer than it may
	// be tall, the way a Mac's is, which is when its rows were squeezed.
	inter, _ := engine.FontBytes(engine.DefaultFont)
	archivo, _ := engine.FontBytes("Archivo Black")
	faces := map[string][]byte{}
	if faces["Bridge Sans.ttf"], err = engine.StyledFace(archivo, "Bridge Sans", "Regular"); err != nil {
		t.Fatal(err)
	}
	if faces["Bridge Sans Heavy.ttf"], err = engine.StyledFace(inter, "Bridge Sans", "Heavy"); err != nil {
		t.Fatal(err)
	}
	for i := range 16 {
		name := fmt.Sprintf("Bridge Spare %02d", i+1)
		if faces[name+".ttf"], err = engine.NamedFace(anton, name); err != nil {
			t.Fatal(err)
		}
	}
	for name, face := range faces {
		if err := afero.WriteFile(installed, "/Library/Fonts/"+name, face, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	engine.UseFontFolders(installed, []string{"/Library/Fonts"})
	t.Cleanup(func() { engine.UseFontFolders(nil, nil) })
	// Dictate on Report a Problem records the bridge's microphone, at the
	// pace the walk asks for it, see sound and /microphone. Once the
	// person has fallen silent and the phrase they were on is said, the
	// quiet is over at once, d.still, rather than after the two minutes
	// of the app's clock. That a recording waits its two minutes is
	// TestListeningStopsByItself's to prove.
	snd := newSound()
	d.still = snd.still
	d.beats = snd.beats()
	d.told = snd.tell
	openMicrophone = snd.open
	// The desk is started again with a bus, before anything is in it.
	d.svc.jobs.shutDown()
	d.bus = newBus()
	// The bridge's microphone talks for as long as a walk takes steps of
	// a recording, so one also stops after a minute, 600 steps, long after
	// every sequence has heard what it waits for, and a walk waiting for
	// more words ends there.
	d.most = 60 * engine.SampleRate
	d.start(d.svc.store)
	// A report goes on the bridge's own Desktop and is shown nowhere.
	reportFolder = func() string {
		desk := filepath.Join(d.home, "Desktop")
		_ = os.MkdirAll(desk, 0o755)
		return desk
	}
	showReport = func(string) error { return nil }
	b := &bridge{t: t, d: d, words: words, sound: snd, first: filepath.Join(d.home, "erste-erinnerung.mp4"), kept: t.TempDir(),
		gates: map[string]*gate{}, fails: map[string]string{},
		videos: walkVideos(t)}
	// What the first run downloads, once a walk made the machine new.
	// What arrives is as much of a model as the app looks for.
	installSpeech = func(ctx context.Context, log *engine.Log, m engine.SpeechModel, dir string) error {
		return b.arriving(ctx, log, "speech", m.Download, func() error {
			if err := os.MkdirAll(filepath.Join(dir, m.Name), 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, m.Name, "tokens.txt"), []byte("the stand-in hears\n"), 0o644)
		})
	}
	installLanguage = func(ctx context.Context, log *engine.Log, m engine.LanguageModel, dir string) error {
		return b.arriving(ctx, log, "clip", m.Download, func() error {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, m.Name), []byte("GGUF, the stand-in finds clips"), 0o644)
		})
	}
	if err := b.videos.copy(b.first, pickedAs("120", "", "0", "", false, 0), func(path string) error {
		return makeEpisode(path, 120, 0, false)
	}); err != nil {
		t.Fatal(err)
	}
	if added, err := d.svc.addEpisodes([]string{b.first}); err != nil || len(added) != 1 {
		t.Fatalf("adding %s: %v", b.first, err)
	}
	d.idle(b.first)
	if len(d.clips(b.first)) == 0 {
		t.Fatalf("the episode has no clip to work on: %+v", d.svc.jobs.list())
	}
	for _, dir := range []string{workOf(b.first), filepath.Join(d.home, "config")} {
		if err := os.CopyFS(filepath.Join(b.kept, filepath.Base(dir)), os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
	}
	b.said = words.at()
	return b
}

func workOf(video string) string {
	return strings.TrimSuffix(video, filepath.Ext(video)) + ".framefairy"
}

// intoMatroska copies a video's streams into Matroska beside it, under
// the same name ending in .mkv, and removes the MP4. A picture that starts
// late by an edit list starts late by its first timestamp there.
//
// On the real disk: ffmpeg copies the video into Matroska.
func intoMatroska(path string) (string, error) {
	mkv := strings.TrimSuffix(path, filepath.Ext(path)) + ".mkv"
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-i", path, "-map", "0", "-c", "copy", mkv).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("copying %s into Matroska: %s %s", filepath.Base(path), err, out)
	}
	return mkv, os.Remove(path)
}

// frameRate is a frame rate as ffmpeg takes it, a whole number or one
// whole number over another, and nothing else, since it goes into a
// filter.
var frameRate = regexp.MustCompile(`^[1-9][0-9]{0,5}(/[1-9][0-9]{0,4})?$`)

// cameraHeight is the height of the camera's picture a pick asks for with
// size, 1080 or 2160, and 0 for the bridge's own kind of episode.
func cameraHeight(size string) int {
	switch size {
	case "1080":
		return 1080
	case "2160":
		return 2160
	}
	return 0
}

// pick makes a new video, not in the library, and has the Add button's box
// hand it over next. Without a rate it is the bridge's own kind of
// episode, at five frames a second, and with one it is filmed at that
// rate, see makeFilmedEpisode. With a switch, in seconds, the bridge's
// own kind is filmed by two cameras that switch then, see twoCameras, and
// with hevc it is HEVC with 10-bit colour. With mkv it is the same video
// in Matroska, the way OBS records, its streams copied as they are. With
// a height it is a camera's picture of that height instead, see
// makeCameraEpisode.
//
// On the real disk: ffmpeg makes the video the app adds.
func (b *bridge) pick(seconds, rate, switchAt, timing string, hevc, namesake, box, mkv bool, height int) (string, error) {
	n, err := strconv.Atoi(seconds)
	if err != nil || n < 10 || n > 3600 {
		n = 90
	}
	if rate != "" && !frameRate.MatchString(rate) {
		return "", fmt.Errorf("%q is no frame rate", rate)
	}
	if timing != "" && timing != "uneven" && timing != "late" && timing != "turned" {
		return "", fmt.Errorf("%q is no timing of frames", timing)
	}
	at, err := strconv.ParseFloat(switchAt, 64)
	if err != nil || at <= 0 || at >= float64(n) {
		at = 0
	}
	b.mu.Lock()
	b.made++
	path := filepath.Join(b.d.home, fmt.Sprintf("folge-%d.mp4", b.made))
	if namesake {
		// The name of the bridge's episode, in a folder of its own.
		path = filepath.Join(b.d.home, fmt.Sprintf("folge-%d", b.made), filepath.Base(b.first))
	}
	b.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	err = b.videos.copy(path, pickedAs(strconv.Itoa(n), rate, strconv.FormatFloat(at, 'g', -1, 64), timing, hevc, height), func(path string) error {
		return makePicked(path, n, rate, at, timing, hevc, height)
	})
	if err != nil {
		return "", err
	}
	if mkv {
		if path, err = intoMatroska(path); err != nil {
			return "", err
		}
	}
	if box {
		b.mu.Lock()
		b.picks = append(b.picks, path)
		b.mu.Unlock()
	}
	return path, nil
}

// pickPicture makes a picture of w by h pixels in the bridge's home, a
// PNG with a gradient so it is not empty, or a file of text named like a
// picture, and has the paperclip's box hand it over next when box.
//
// On the real disk: the open box and a drop hand over paths on the disk.
func (b *bridge) pickPicture(name, w, h, kind string, box bool) (string, error) {
	width, _ := strconv.Atoi(w)
	height, _ := strconv.Atoi(h)
	if width <= 0 || height <= 0 {
		width, height = 320, 200
	}
	if name == "" {
		b.mu.Lock()
		b.made++
		name = fmt.Sprintf("Screenshot %d.png", b.made)
		if kind == "recording" {
			name = fmt.Sprintf("Screen Recording %d.mov", b.made)
		}
		b.mu.Unlock()
	}
	dir := filepath.Join(b.d.home, "pictures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filepath.Base(name))
	var data []byte
	if kind == "recording" {
		// A recording of three seconds in a MOV, the way the Mac's own
		// recorder makes one, with a sound. MPEG-4 and not H.264, which
		// the ffmpeg the app ships cannot make.
		if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d:rate=30:duration=3", width, height),
			"-f", "lavfi", "-i", "sine=f=440:d=3", "-c:v", "mpeg4", "-c:a", "aac",
			"-shortest", "-f", "mov", path).CombinedOutput(); err != nil {
			return "", fmt.Errorf("ffmpeg: %v: %s", err, out)
		}
	} else if kind == "text" {
		data = []byte("not a picture\n")
	} else {
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := range height {
			for x := range width {
				img.Set(x, y, color.RGBA{uint8(255 * x / width), uint8(255 * y / height), 146, 255})
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return "", err
		}
		data = buf.Bytes()
	}
	if data != nil {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return "", err
		}
	}
	if box {
		b.d.pictureMu.Lock()
		b.d.pictures = append(b.d.pictures, path)
		b.d.pictureMu.Unlock()
	}
	return path, nil
}

// lastReport names what the newest report in dir holds.
//
// On the real disk: a report is a zip on the Desktop.
func lastReport(dir string) ([]string, error) {
	z, err := newestReport(dir)
	if z == nil || err != nil {
		return []string{}, err
	}
	defer z.Close()
	names := []string{}
	for _, f := range z.File {
		names = append(names, f.Name)
	}
	return names, nil
}

// lastReportFile is what the file whose name ends in name holds in the
// newest report, so a walk can read what the report says was written.
//
// On the real disk: a report is a zip on the Desktop.
func lastReportFile(dir, name string) (string, error) {
	z, err := newestReport(dir)
	if z == nil || err != nil {
		return "", err
	}
	defer z.Close()
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, name) {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		return string(b), err
	}
	return "", fmt.Errorf("the report holds no %s", name)
}

// newestReport opens the zip last made in dir, or none when there is none.
//
// On the real disk: a report is a zip on the Desktop.
func newestReport(dir string) (*zip.ReadCloser, error) {
	zips, _ := filepath.Glob(filepath.Join(dir, "*.zip"))
	if len(zips) == 0 {
		return nil, nil
	}
	sort.Slice(zips, func(i, j int) bool {
		a, _ := os.Stat(zips[i])
		c, _ := os.Stat(zips[j])
		return a.ModTime().Before(c.ModTime())
	})
	return zip.OpenReader(zips[len(zips)-1])
}

// pickSound makes a file of sound with no picture, named like a video,
// and has the Add button's box hand it over next.
func (b *bridge) pickSound() (string, error) {
	b.mu.Lock()
	b.made++
	path := filepath.Join(b.d.home, fmt.Sprintf("nur-ton-%d.mp4", b.made))
	b.mu.Unlock()
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=f=440:d=12",
		"-c:a", "aac", path).CombinedOutput(); err != nil {
		return "", fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	b.mu.Lock()
	b.picks = append(b.picks, path)
	b.mu.Unlock()
	return path, nil
}

// reopen closes the app and opens it again, the way quitting and starting
// it does: the work stops, and what it did and how it ended is read back.
// It gives back the jobs as the app left them, once nothing ran any more,
// so a walk knows how each ended on the engine's word: a search the
// closing cut off says cancelled, one that finished as the app closed says
// done. What a walk read off the screen before it asked is a moment older,
// and a search the model answers at once can end in that moment.
// makeFresh makes the bridge's machine one the app has never run on, see
// /fresh. The video leaves the library with its work folder kept, the
// way Remove leaves it, and reset brings both back.
//
// On the real disk: the models folder is the one the app looks in for its models, in the bridge's home.
func (b *bridge) makeFresh() error {
	b.mu.Lock()
	b.arrive = map[string]chan struct{}{"speech": make(chan struct{}), "clip": make(chan struct{})}
	b.mu.Unlock()
	b.fresh.Store(true)
	if err := os.RemoveAll(engine.ModelsDir()); err != nil {
		return err
	}
	if err := b.d.svc.RemoveEpisode(b.first, false); err != nil {
		return err
	}
	planner := defaultSettings().Planner
	return b.d.svc.store.UpdateSettings(func(s *Settings) {
		s.Planner, s.Chosen, s.LLMModel = planner, false, ""
	})
}

// letArrive lets the download of a kind of model arrive, see arriving.
func (b *bridge) letArrive(kind string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	gate, ok := b.arrive[kind]
	if !ok {
		return fmt.Errorf("no download of a %q model to let arrive, the bridge is not on its first run", kind)
	}
	select {
	case <-gate:
	default:
		close(gate)
	}
	return nil
}

// arriving is a download of the first run on the bridge, see /fresh. It
// says it has fetched two fifths, in the words the engine's download
// says it in, and holds there until the walk lets it arrive or it is
// cancelled. Then put lays the model down the way an install leaves it.
func (b *bridge) arriving(ctx context.Context, log *engine.Log, kind string, size int64, put func() error) error {
	b.mu.Lock()
	gate := b.arrive[kind]
	b.mu.Unlock()
	if gate == nil {
		return fmt.Errorf("the bridge downloads nothing outside its first run")
	}
	const rate = 120e6
	log.ProgressOf(fmt.Sprintf("fetching %d MB of %d MB at %d MB a second", size*2/5/1e6, size/1e6, int64(rate/1e6)),
		0.4, float64(size)*3/5/rate)
	select {
	case <-ctx.Done():
		return engine.ErrCancelled
	case <-gate:
	}
	log.ClearProgress()
	return put()
}

func (b *bridge) reopen() ([]Job, error) {
	if !b.words.whileClosing(b.d.svc.jobs.shutDown) {
		return nil, fmt.Errorf("the app did not stop its work in time")
	}
	closed := b.d.svc.jobs.list()
	b.d.svc.levels.shutDown()
	b.d.svc.CancelListening()
	b.d.reopen()
	return closed, nil
}

// reset puts the app back the way it was once the first episode was
// searched: the model answers again, every episode a walk added is gone,
// the episode's work folder and the app's settings are what they were, the
// speech stand-in says next the word it said next then, and the app is
// opened again on them, so nothing a walk did is kept in memory either.
// Every walk starts from the same place, and a seed walks the same way
// every time: a video it adds is heard with the same words, however many
// words the walks before it had heard.
//
// On the real disk: it puts back the library where ffmpeg and the episode's decoder read the episode.
func (b *bridge) reset() error {
	// A recording the walk before left running is still in hand, and
	// tells the bus every desk shares what it hears. The app opened again numbers
	// its recordings from 1, so the next walk's Dictate would take those
	// words for its own.
	b.d.svc.CancelListening()
	b.sound.silent.Store(false)
	b.words.release()
	b.words.wordless.Store(false)
	b.words.save.Store(false)
	b.d.model.hangs(false)
	b.d.model.fails(false)
	b.mu.Lock()
	b.picks = nil
	for _, g := range b.gates {
		close(g.open)
	}
	clear(b.gates)
	clear(b.fails)
	b.chrome = Chrome{}
	b.arrive = nil
	b.mu.Unlock()
	b.fresh.Store(false)
	b.d.pictureMu.Lock()
	b.d.pictures = nil
	b.d.pictureMu.Unlock()
	b.d.reportMade.Store(0)
	b.d.reportFronted.Store(0)
	b.d.exported.Store(nil)
	if !b.d.svc.jobs.shutDown() {
		return fmt.Errorf("the app did not stop its work in time")
	}
	b.d.svc.levels.shutDown()
	// A log a walk started lives in the home, which goes next.
	theLog.close()
	entries, err := os.ReadDir(b.d.home)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == filepath.Base(b.first) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(b.d.home, e.Name())); err != nil {
			return err
		}
	}
	for _, dir := range []string{workOf(b.first), filepath.Join(b.d.home, "config")} {
		if err := os.CopyFS(dir, os.DirFS(filepath.Join(b.kept, filepath.Base(dir)))); err != nil {
			return err
		}
	}
	// After the work has stopped, so nothing hears on past it.
	b.words.from(b.said)
	freshDecoders()
	b.d.reopen()
	return nil
}

// freshDecoders closes the streams and the episode's decoders the walk
// before left. A decoder keeps its cursors where that walk left them, and
// a jump of the next walk then went on from one of them rather than
// seek, so what a walk saw depended on the walks before it: a drag that
// should have shown its key frame showed none, issue #552. The next
// stream starts a decoder again, with its cursors warmed as the app's
// first are. A hold the walk before took is kept, so its release finds it.
func freshDecoders() {
	p := openPreviews
	p.mu.Lock()
	streams, decoders := p.open, p.decoders
	p.open, p.decoders = nil, nil
	p.mu.Unlock()
	for _, s := range streams {
		s.stop()
	}
	for _, d := range decoders {
		d.Close()
	}
}

// bridgeDist is where the interface built for the bridge is.
//
// On the real disk: the interface for the bridge is a folder the build wrote.
func bridgeDist(t *testing.T) string {
	t.Helper()
	dist, err := filepath.Abs("../../frontend/preview/dist-bridge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Skip("the interface is not built for the bridge: cd frontend && npx vite build --config preview/bridge.config.ts")
	}
	return dist
}

// TestWalks runs the sequences and a set of walks in frontend/preview/walks
// against the bridge, and fails with what they printed when one breaks a
// rule or a sequence does not come out as it should. make walks runs it,
// with the interface built for the bridge first, and it only runs when
// asked, because it needs Node, Playwright's Chromium and the build:
//
//	make walks
//	make walks WALKS=40 STEPS=80   to look further
//
// The walks are seeds 1 to WALKS, the same every time, so a walk that
// breaks a rule breaks it again on the next run, and a change that only
// moves what a seed walks into is not a red run of its own.
//
// They run side by side, half as many again as the machine has cores, or
// WALKERS. Each runs against a bridge of its own, started as a process of
// its own: a desk sets HOME for the whole process it is in, so two bridges
// in one would share the app's settings. One after another they took nine
// minutes. A run spends most of its time waiting, on a clip that plays in
// real time and on the quiet after every step, so four bridges kept four
// cores busy only part of the time: six on the four cores of CI and the
// cloud took the whole set from six and a half minutes to five and a half,
// with the cores busy four fifths of it.
//
// Every sequence is a run of its own, handed to whichever bridge is free
// next, the same as a walk. Run as one, the sixty-six sequences took
// twelve minutes on one bridge while the other three had finished their
// walks in four and waited. Each sequence opens the app afresh with
// /reset, so it walks the same alone as after the one before it. The
// longest runs go first and the short ones fill in behind them, so no
// bridge is left with a long run once the others are done. How long each
// run takes is in walk-times.tsv beside the walks, written by
// WALKTIMES=1, see shareOf.
//
// On the real disk: Node runs the walks against the interface the build wrote.
func TestWalks(t *testing.T) {
	if os.Getenv("FRAMEFAIRY_WALKS") == "" {
		t.Skip("make walks runs these")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("the walks need Node")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal("the walks need ffmpeg")
	}
	dist, err := filepath.Abs("../../frontend/preview/dist-bridge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Fatal("the interface is not built for the bridge, which make walks does")
	}
	walks, err := filepath.Abs("../../frontend/preview/walks")
	if err != nil {
		t.Fatal(err)
	}

	type walkRun struct {
		script string
		env    []string
		// sequence is the sequence the run walks, if it is one.
		sequence string
	}
	// The name a run is logged and timed under.
	name := func(r walkRun) string {
		return strings.TrimSpace(r.script + " " + strings.Join(r.env, " "))
	}
	var runs []walkRun
	walksWanted := envNumber("WALKS", 3)
	// Each walk takes its own number of steps unless STEPS says.
	var steps []string
	if n := envNumber("STEPS", 0); n > 0 {
		steps = append(steps, fmt.Sprintf("STEPS=%d", n))
	}
	// Every walk in the folder, each for the same seeds.
	for seed := 1; seed <= walksWanted; seed++ {
		for _, script := range walkScripts(t, walks) {
			runs = append(runs, walkRun{script: script, env: append([]string{fmt.Sprintf("SEED=%d", seed)}, steps...)})
		}
		// Playback again on an episode whose picture the Go side decodes,
		// HEVC in 10-bit colour, which Chromium cannot.
		runs = append(runs, walkRun{script: "playback.mjs", env: append([]string{fmt.Sprintf("SEED=%d", seed), "HEVC=1"}, steps...)})
	}
	for _, name := range sequenceNames(t, node, walks) {
		runs = append(runs, walkRun{script: "sequences.mjs", env: []string{"SEQUENCE=" + name}, sequence: name})
	}

	// One share of the runs when CI spreads them over machines,
	// WALKSHARD=1/2, each share as long as the others by the times the
	// runs took last, and the longest first in every share.
	//
	// On the real disk: the times are a file beside the walks.
	timesFile := filepath.Join(walks, "walk-times.tsv")
	before, err := os.ReadFile(timesFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	times := readWalkTimes(string(before))
	all := runs
	runs = shareOf(t, all, name, times, os.Getenv("WALKSHARD"))
	if len(runs) == 0 {
		t.Skip("this share has no runs")
	}
	var tookMu sync.Mutex
	took := map[string]float64{}

	// The traces of the runs that fail, see keepTraces in walks/bridge.mjs.
	// Those a run before kept are let go of, so the folder holds this
	// run's and no other.
	if dir := os.Getenv("WALKTRACES"); dir != "" {
		old, _ := filepath.Glob(filepath.Join(dir, "*.zip"))
		heard, _ := filepath.Glob(filepath.Join(dir, "*.requests.txt"))
		for _, f := range append(old, heard...) {
			if err := os.Remove(f); err != nil {
				t.Fatal(err)
			}
		}
	}

	// A walk waits on signals, so one whose signal never comes runs until
	// go test's -timeout ends the whole binary, and that prints neither
	// which walk it was nor anything the runs said through t. So the runs
	// still going a minute before then are stopped and reported, with
	// what they printed, and every run says on stderr, which is not held
	// back, when it starts and ends. Issue #443.
	ctx := context.Background()
	if end, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, end.Add(-walkGrace)) //nolint:forbidigo // reports the walks still running before go test's -timeout ends them, #443
		defer cancel()
	}
	var unwalked atomic.Int32

	// The videos the bridges make, kept for the next walk that asks for
	// the same, see walkVideos. Those another recipe made go first.
	//
	// On the real disk: the bridges are programs of their own, which share the folder.
	kept := os.Getenv("FRAMEFAIRY_WALK_VIDEOS")
	if kept == "" {
		kept = t.TempDir()
	}
	if err := (&made{fs: afero.NewOsFs(), dir: kept, recipe: videoRecipe()}).prune(); err != nil {
		t.Fatal(err)
	}

	walkers := min(envNumber("WALKERS", runtime.NumCPU()*3/2), len(runs))
	todo := make(chan walkRun, len(runs))
	for _, r := range runs {
		todo <- r
	}
	close(todo)
	// The bridges start at once, each in its walker, since each takes a
	// few seconds to hear and search its episode.
	var wg sync.WaitGroup
	for range walkers {
		wg.Go(func() {
			url, err := startBridge(t, kept)
			if err != nil {
				t.Error(err)
				return
			}
			env := append(os.Environ(), "BRIDGE_URL="+url)
			// The walker's sequences, walked one after another by one
			// node, see sequencer.
			seqs := &sequencer{run: []string{node, filepath.Join(walks, "sequences.mjs"), "--serve"}, dir: walks, env: env}
			defer seqs.stop(ctx)
			for r := range todo {
				if ctx.Err() != nil {
					unwalked.Add(1)
					continue
				}
				fmt.Fprintf(os.Stderr, "walking %s\n", name(r))
				began := time.Now()
				var out bytes.Buffer
				var err error
				if r.sequence != "" {
					err = seqs.walk(ctx, r.sequence, &out)
				} else {
					err = walk(ctx, node, []string{filepath.Join(walks, r.script)}, walks, append(env, r.env...), &out)
				}
				seconds := time.Since(began).Seconds() //nolint:forbidigo // the seconds a walk took, reported, never waited on
				// A run the test's time stopped says nothing of how long it
				// takes, so walk-times.tsv keeps the time it had for it.
				if err == nil || ctx.Err() == nil {
					tookMu.Lock()
					took[name(r)] = seconds
					tookMu.Unlock()
				}
				what := fmt.Sprintf("%s (%.0f s)", name(r), seconds)
				if err != nil {
					// A walk that was stopped says what it printed at once
					// too, since the bridges may not end in time either,
					// and then t holds it back for good.
					if ctx.Err() != nil {
						fmt.Fprintf(os.Stderr, "failed %s: %v\n%s\n", what, err, out.Bytes())
					} else {
						fmt.Fprintf(os.Stderr, "failed %s\n", what)
					}
					t.Errorf("%s: %v\n%s", what, err, out.Bytes())
					continue
				}
				fmt.Fprintf(os.Stderr, "walked %s\n", what)
				t.Logf("%s:\n%s", what, out.Bytes())
			}
		})
	}
	wg.Wait()
	if left := len(todo); left > 0 {
		t.Errorf("%d runs were not walked, since no bridge started", left)
	}
	if n := unwalked.Load(); n > 0 {
		t.Errorf("%d runs were not walked, since the test's time ran out", n)
	}
	// WALKTIMES=1 keeps what the runs took for the next split, and drops
	// the runs that are gone.
	if os.Getenv("WALKTIMES") != "" {
		names := make([]string, len(all))
		for i, r := range all {
			names[i] = name(r)
		}
		if err := os.WriteFile(timesFile, []byte(writeWalkTimes(names, times, took)), 0o644); err != nil {
			t.Error(err)
		}
	}
}

// startBridge starts TestBridge in a process of its own, on a free port,
// keeping the videos it makes in the folder videos, and gives back where
// it answers. It is stopped when the test ends.
func startBridge(t *testing.T, videos string) (string, error) {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestBridge$", "-test.timeout", "0")
	cmd.Env = append(os.Environ(), "FRAMEFAIRY_BRIDGE=127.0.0.1:0", "FRAMEFAIRY_BRIDGE_WALKER=1", "FRAMEFAIRY_WALKS=",
		"FRAMEFAIRY_WALK_VIDEOS="+videos)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	// What it says, read until it says where it answers and let go of
	// after that.
	said, says := io.Pipe()
	cmd.Stdout, cmd.Stderr = says, says
	if err := cmd.Start(); err != nil {
		return "", err
	}
	ended := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = says.Close()
		close(ended)
	}()
	t.Cleanup(func() {
		_ = stdin.Close()
		<-ended
	})
	lines := bufio.NewScanner(said)
	var before []string
	for lines.Scan() {
		line := lines.Text()
		if rest, ok := strings.CutPrefix(line, "bridge on "); ok {
			url, _, _ := strings.Cut(rest, " ")
			go func() { _, _ = io.Copy(io.Discard, said) }()
			return url + "/", nil
		}
		before = append(before, line)
	}
	return "", fmt.Errorf("the bridge did not start:\n%s", strings.Join(before, "\n"))
}

// A walk that hangs is stopped when the test's time runs out, and what it
// printed until then is kept, so the report says which walk it was and
// how far it got, issue #443.
func TestAWalkThatHangsIsStoppedWithWhatItSaid(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	out := &saying{what: "halfway", said: make(chan struct{})}
	go func() {
		<-out.said
		stop()
	}()
	// tail -f waits for more forever, the way a walk waits on a signal
	// that never comes.
	err := walk(ctx, "sh", []string{"-c", "echo halfway; exec tail -f /dev/null"}, "", nil, out)
	if err == nil || !strings.Contains(err.Error(), "time ran out") {
		t.Errorf("a walk that hangs ends with %v, want that its time ran out", err)
	}
	if !strings.Contains(out.String(), "halfway") {
		t.Errorf("what the walk printed before it was stopped is %q", out.String())
	}
	// A walk is stopped with an interrupt, and what it says on its way
	// out, where it kept its traces, is kept too. A walk on its own
	// answers it at once, and one that records traces once they are kept.
	ctx, stop = context.WithCancel(context.Background())
	defer stop()
	out = &saying{what: "halfway", said: make(chan struct{})}
	go func() {
		<-out.said
		stop()
	}()
	err = walk(ctx, "sh", []string{"-c", `trap 'kill $!; echo "trace: kept"; exit 1' INT; echo halfway; tail -f /dev/null >/dev/null & wait`}, "", nil, out)
	if err == nil || !strings.Contains(err.Error(), "time ran out") {
		t.Errorf("a walk that hangs and keeps its traces ends with %v, want that its time ran out", err)
	}
	if !strings.Contains(out.String(), "trace: kept") {
		t.Errorf("what a walk that was stopped said on its way out is not kept: %q", out.String())
	}
	if err := walk(context.Background(), "sh", []string{"-c", "echo done"}, "", nil, io.Discard); err != nil {
		t.Errorf("a walk that ends gives %v", err)
	}
	if err := walk(context.Background(), "sh", []string{"-c", "exit 1"}, "", nil, io.Discard); err == nil || strings.Contains(err.Error(), "time ran out") {
		t.Errorf("a walk that fails gives %v, want its own failure", err)
	}
}

// A walker's sequences are walked by one node, one after another, and
// each says how it went: passed, failed, or threw, which ends the node, so
// the next starts in a new one. A node that ends before its sequence is
// walked, or that is still walking when the test's time runs out, fails
// that sequence, with what it printed, and the next starts in a new one.
// A shell stands in for node, saying what sequences.mjs --serve says.
func TestAWalkersSequencesAreWalkedByOneNode(t *testing.T) {
	serve := `while read -r name; do
		printf '\036walking\n'
		echo "$name in $$"
		case "$name" in
		'"fails"') printf '\036walked FAIL\n' ;;
		'"throws"') printf '\036walked threw\n'; exit 1 ;;
		'"ends"') exit 0 ;;
		'"passes, and its node ends"') printf '\036walked ok\n'; echo "a late error"; exit 1 ;;
		'"hangs"') sleep 1000 & echo "halfway, $! holds the output"; exec tail -f /dev/null ;;
		'"hangs, and keeps its traces"') trap 'kill $!; echo "trace: kept"; exit 1' INT; echo halfway; tail -f /dev/null >/dev/null & wait ;;
		*) printf '\036walked ok\n' ;;
		esac
	done`
	// The node's child that holds its output, killed with nothing else,
	// lingers no longer than it takes to see that it is let go of.
	s := &sequencer{run: []string{"sh", "-c", serve}, linger: time.Millisecond}
	defer s.stop(context.Background())
	walked := func(name string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		err := s.walk(context.Background(), name, &out)
		said, in, _ := strings.Cut(strings.TrimSpace(out.String()), " in ")
		if said != `"`+name+`"` {
			t.Errorf("walking %s printed %q", name, out.String())
		}
		return in, err
	}
	first, err := walked("passes")
	if err != nil {
		t.Fatalf("a sequence that passed gives %v", err)
	}
	if node, err := walked("fails"); err == nil || node != first {
		t.Errorf("a sequence that failed after one that passed gives %v in node %s, want a failure in node %s", err, node, first)
	}
	if node, err := walked("throws"); err == nil || node != first {
		t.Errorf("a sequence that threw gives %v in node %s, want a failure in node %s", err, node, first)
	}
	second, err := walked("passes")
	if err != nil || second == first {
		t.Errorf("the sequence after one that threw gives %v in node %s, want it passed in a new node", err, second)
	}
	if _, err := walked("ends"); err == nil {
		t.Error("a sequence whose node ended before it was walked passed")
	}
	third, err := walked("passes")
	if err != nil || third == second {
		t.Errorf("the sequence after a node that ended gives %v in node %s, want it passed in a new node", err, third)
	}
	if _, err := walked("passes, and its node ends"); err != nil {
		t.Errorf("a sequence that passed before its node ended gives %v", err)
	}
	// The node ended after it, so the next sequence was never begun: it
	// says so, with what the node said on its way out, and blames the
	// node, not itself.
	var late bytes.Buffer
	err = s.walk(context.Background(), "passes", &late)
	if err == nil || !strings.Contains(err.Error(), `after passes, and its node ends, before this one began`) {
		t.Errorf("a sequence after its node ended gives %v, want that the node ended after the one before it", err)
	}
	if !strings.Contains(late.String(), "before it began, its node said: a late error") {
		t.Errorf("a sequence after its node ended printed %q, want what the node said before it began", late.String())
	}
	if fourth, err := walked("passes"); err != nil || fourth == third {
		t.Errorf("the sequence after that gives %v in node %s, want it passed in a new node", err, fourth)
	}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	out := &saying{what: "holds the output", said: make(chan struct{})}
	go func() {
		<-out.said
		stop()
	}()
	if err := s.walk(ctx, "hangs", out); err == nil || !strings.Contains(err.Error(), "time ran out") {
		t.Errorf("a sequence that hangs ends with %v, want that its time ran out", err)
	}
	var child int
	if _, err := fmt.Sscanf(out.buf.String()[strings.Index(out.buf.String(), "halfway, "):], "halfway, %d holds", &child); err == nil {
		if p, err := os.FindProcess(child); err == nil {
			_ = p.Kill()
		}
	}
	if _, err := walked("passes"); err != nil {
		t.Errorf("the sequence after one that hung gives %v", err)
	}

	// A node is stopped with an interrupt, and what it says on its way
	// out, where it kept the traces of the sequence it hung in, is kept.
	ctx, stop = context.WithCancel(context.Background())
	defer stop()
	out = &saying{what: "halfway", said: make(chan struct{})}
	go func() {
		<-out.said
		stop()
	}()
	if err := s.walk(ctx, "hangs, and keeps its traces", out); err == nil || !strings.Contains(err.Error(), "time ran out") {
		t.Errorf("a sequence that hangs and keeps its traces ends with %v, want that its time ran out", err)
	}
	if !strings.Contains(out.String(), "trace: kept") {
		t.Errorf("what a node that was stopped said on its way out is not kept: %q", out.String())
	}
	if _, err := walked("passes"); err != nil {
		t.Errorf("the sequence after one that was stopped gives %v", err)
	}
}

// saying keeps what is written to it and closes said once it holds what.
// The buffer is a field, not embedded, since a bytes.Buffer's ReadFrom
// would read the walk's output past Write.
type saying struct {
	buf  bytes.Buffer
	what string
	said chan struct{}
	once sync.Once
}

func (s *saying) Write(p []byte) (int, error) {
	n, err := s.buf.Write(p)
	if strings.Contains(s.buf.String(), s.what) {
		s.once.Do(func() { close(s.said) })
	}
	return n, err
}

func (s *saying) String() string { return s.buf.String() }

// walkGrace is how long before go test's -timeout the walks still
// running are stopped, time enough to end them and report.
const walkGrace = time.Minute

// walk runs one walk, name with args in dir, and writes what it prints to
// out. One still running when ctx ends is stopped, and says so, and what
// it printed until then is in out.
func walk(ctx context.Context, name string, args []string, dir string, env []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = out, out
	// Stopped with an interrupt, which a walk that records traces answers
	// by keeping them, see keepTraces in walks/bridge.mjs, and killed if
	// it has not ended after walkStopping.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = walkStopping
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return errors.New("it had not ended when the test's time ran out, and was stopped")
	}
	return err
}

// sequencer is one node that walks sequence after sequence on one bridge,
// in one browser, sequences.mjs --serve. A node of its own for each, as
// the walks still have, started node, imported Playwright and launched
// Chromium again every time: 1.45 s and 1.2 s of CPU a sequence, of 205,
// issue #526. Each sequence still starts with /reset and pages of its
// own. A sequence that threw ends the node, and the next starts a new
// one.
type sequencer struct {
	// run is the program that walks them and what it is handed, in dir
	// and with env.
	run []string
	dir string
	env []string
	// linger is how long a node that ended may hold its output open,
	// walkLinger unless set.
	linger time.Duration
	cmd    *exec.Cmd
	in     io.WriteCloser
	// lines is what the node prints, a line at a time, closed when it
	// has ended.
	lines chan string
	// last is the sequence the node walked last, which a node that ends
	// between two sequences is told by.
	last string
}

// walkLinger is how long a walk's node that ended may hold its output
// open. The browser a stopped walk started may hold it open, and what it
// says then is not worth waiting for.
const walkLinger = 5 * time.Second

// walkStopping is how long a walk that was interrupted has to keep its
// traces and end before it is killed.
const walkStopping = 30 * time.Second

// walkingLine is the line the node begins what a sequence prints with,
// and walkedLine starts the line it ends it with, followed by how it
// went, see mark in sequences.mjs.
const walkingLine, walkedLine = "\x1ewalking", "\x1ewalked "

// walk walks the sequence name and writes what it printed to out. It
// fails when the sequence failed, and when it threw or had not ended
// when ctx did, the node is stopped.
func (s *sequencer) walk(ctx context.Context, name string, out io.Writer) error {
	if s.cmd == nil {
		if err := s.start(); err != nil {
			return err
		}
	}
	// A node that ended after the sequence before this one, or never
	// began, is said to have, so this sequence is not blamed for it.
	gone := func() error {
		s.kill()
		if s.last == "" {
			return errors.New("its node ended before it began")
		}
		return fmt.Errorf("its node ended after %s, before this one began", s.last)
	}
	last := s.last
	s.last = name
	line, _ := json.Marshal(name)
	// A node that has ended refuses the name. What it said on its way out
	// is still to be read, so that is left to the reading below, which
	// then hears it has ended.
	_, _ = s.in.Write(append(line, '\n'))
	begun := false
	for {
		select {
		case l, ok := <-s.lines:
			if !ok {
				if !begun {
					s.last = last
					return gone()
				}
				s.kill()
				return errors.New("its node ended before it had walked")
			}
			if l == walkingLine {
				begun = true
				continue
			}
			how, ended := strings.CutPrefix(l, walkedLine)
			if !ended {
				if !begun {
					l = "before it began, its node said: " + l
				}
				fmt.Fprintln(out, l)
				continue
			}
			switch how {
			case "ok":
				return nil
			case "threw":
				s.stop(ctx)
				return errors.New("it threw, and its node ended")
			}
			return errors.New("it failed")
		case <-ctx.Done():
			s.interrupt(out)
			return errors.New("it had not ended when the test's time ran out, and was stopped")
		}
	}
}

func (s *sequencer) start() error {
	cmd := exec.Command(s.run[0], s.run[1:]...)
	cmd.Dir, cmd.Env = s.dir, s.env
	cmd.WaitDelay = cmp.Or(s.linger, walkLinger)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	said, says := io.Pipe()
	cmd.Stdout, cmd.Stderr = says, says
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := make(chan string)
	go func() {
		read := bufio.NewScanner(said)
		read.Buffer(nil, 1<<24)
		for read.Scan() {
			lines <- read.Text()
		}
		close(lines)
		// What it says after the reading stopped, a line too long, is
		// let go of, so it never waits to be heard.
		_, _ = io.Copy(io.Discard, said)
	}()
	go func() {
		_ = cmd.Wait()
		_ = says.Close()
	}()
	s.cmd, s.in, s.lines, s.last = cmd, in, lines, ""
	return nil
}

// stop has the node end, if one runs, by telling it there is nothing
// more to walk, so it closes its browser itself. It is killed if that has
// not happened when ctx ends.
func (s *sequencer) stop(ctx context.Context) {
	if s.cmd == nil {
		return
	}
	_ = s.in.Close()
	for {
		select {
		case _, ok := <-s.lines:
			if !ok {
				s.cmd = nil
				return
			}
		case <-ctx.Done():
			s.kill()
			return
		}
	}
}

// interrupt has the node end, if one runs, with an interrupt, which a
// node that records traces answers by keeping those of the sequence it
// hung in, see keepTraces in walks/bridge.mjs. What it says on its way
// out, where the traces are, is written to out. It is killed if it has
// not ended after walkStopping.
func (s *sequencer) interrupt(out io.Writer) {
	if s.cmd == nil {
		return
	}
	_ = s.cmd.Process.Signal(os.Interrupt)
	stopping := time.NewTimer(walkStopping) //nolint:forbidigo // a node that does not end when it is interrupted is killed, and only the clock says when
	defer stopping.Stop()
	for {
		select {
		case l, ok := <-s.lines:
			if !ok {
				_ = s.in.Close()
				s.cmd = nil
				return
			}
			fmt.Fprintln(out, l)
		case <-stopping.C:
			s.kill()
			return
		}
	}
}

// kill ends the node at once, if one runs, and lets go of what it still
// says.
func (s *sequencer) kill() {
	if s.cmd == nil {
		return
	}
	_ = s.in.Close()
	_ = s.cmd.Process.Kill()
	for range s.lines {
	}
	s.cmd = nil
}

// walkScripts are the walks in the folder: every script that walks with
// walk.mjs, so a new walk runs without being named anywhere else.
//
// On the real disk: the walks are the scripts in frontend/preview/walks.
func walkScripts(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".mjs") || e.Name() == "walk.mjs" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `from "./walk.mjs"`) {
			out = append(out, e.Name())
		}
	}
	if len(out) == 0 {
		t.Fatal("no walks found in " + dir)
	}
	return out
}

// sequenceNames is every sequence in sequences.mjs, in the order of the
// file, as the script itself lists them.
func sequenceNames(t *testing.T, node, dir string) []string {
	t.Helper()
	cmd := exec.Command(node, filepath.Join(dir, "sequences.mjs"), "--names")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("listing the sequences: %v\n%s", err, out)
	}
	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(names) == 0 || names[0] == "" {
		t.Fatal("sequences.mjs lists no sequences")
	}
	return names
}

// shareOf gives share i of n of all, "i/n", or all of them when share is
// empty, the longest run first either way. How long a run takes is what
// it took when the times were last written, and a run with no time takes
// as long as the middle one. The runs are handed out longest first, each
// to the share with the least time so far, so the shares end together
// where every third run left one share a minute and more behind the
// others. A share starts its longest runs first, so the short ones fill
// in behind them rather than one long run starting last.
func shareOf[T any](t *testing.T, all []T, name func(T) string, took map[string]float64, share string) []T {
	t.Helper()
	i, n := 1, 1
	if share != "" {
		if _, err := fmt.Sscanf(share, "%d/%d", &i, &n); err != nil || n < 1 || i < 1 || i > n {
			t.Fatalf("WALKSHARD is i/n, like 1/2, not %q", share)
		}
	}
	middle := 1.0
	if len(took) > 0 {
		known := make([]float64, 0, len(took))
		for _, s := range took {
			known = append(known, s)
		}
		slices.Sort(known)
		middle = known[len(known)/2]
	}
	takes := func(x T) float64 {
		if s, ok := took[name(x)]; ok {
			return s
		}
		return middle
	}
	order := slices.Clone(all)
	slices.SortStableFunc(order, func(a, b T) int { return cmp.Compare(takes(b), takes(a)) })
	sums := make([]float64, n)
	var mine []T
	for _, x := range order {
		least := 0
		for k := range sums {
			if sums[k] < sums[least] {
				least = k
			}
		}
		sums[least] += takes(x)
		if least == i-1 {
			mine = append(mine, x)
		}
	}
	return mine
}

// readWalkTimes reads walk-times.tsv: whole seconds, a tab and the name
// of the run, one run to a line. A line it cannot read is left out.
func readWalkTimes(file string) map[string]float64 {
	took := map[string]float64{}
	for line := range strings.Lines(file) {
		seconds, name, ok := strings.Cut(strings.TrimRight(line, "\r\n"), "\t")
		s, err := strconv.ParseFloat(strings.TrimSpace(seconds), 64)
		if !ok || err != nil || s <= 0 || name == "" {
			continue
		}
		took[name] = s
	}
	return took
}

// writeWalkTimes is walk-times.tsv for the runs named, with what a run
// took now where it ran and what it took before where it did not, the
// longest first and padded so the seconds line up.
func writeWalkTimes(names []string, before, now map[string]float64) string {
	type line struct {
		seconds int
		name    string
	}
	var lines []line
	for _, n := range names {
		s, ok := now[n]
		if !ok {
			s, ok = before[n]
		}
		if ok {
			lines = append(lines, line{max(1, int(math.Round(s))), n})
		}
	}
	slices.SortFunc(lines, func(a, b line) int {
		return cmp.Or(cmp.Compare(b.seconds, a.seconds), strings.Compare(a.name, b.name))
	})
	var out strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&out, "%3d\t%s\n", l.seconds, l.name)
	}
	return out.String()
}

func envNumber(name string, otherwise int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return otherwise
}

// Split two ways or three, the shares of the walks hold every run once,
// with times for some of the runs or none.
func TestTheSharesOfTheWalksHoldEveryRunOnce(t *testing.T) {
	all := []string{"a", "b", "c", "d", "e", "f", "g"}
	same := func(x string) string { return x }
	for _, took := range []map[string]float64{nil, {"a": 3, "c": 90, "f": 40, "g": 7}} {
		for _, n := range []int{1, 2, 3} {
			seen := map[string]int{}
			for i := 1; i <= n; i++ {
				for _, x := range shareOf(t, all, same, took, fmt.Sprintf("%d/%d", i, n)) {
					seen[x]++
				}
			}
			for _, x := range all {
				if seen[x] != 1 {
					t.Errorf("times %v, split %d ways, run %s is in %d shares", took, n, x, seen[x])
				}
			}
		}
		if got := shareOf(t, all, same, took, ""); len(got) != len(all) {
			t.Errorf("times %v, with no share, %d runs of %d", took, len(got), len(all))
		}
	}
}

// The shares of the walks end together: a set like the real one, a few
// runs of a minute or more and many of a few seconds, splits three ways
// with no share more than a short run longer than another, every share
// starts with its longest run, and a run with no time counts as the
// middle one.
func TestTheSharesOfTheWalksTakeAsLongAsEachOther(t *testing.T) {
	took := map[string]float64{}
	var all []string
	for k, s := range []float64{92, 70, 69, 63, 61, 59, 56, 54, 51, 49, 48, 47, 43, 38, 37, 37} {
		took[fmt.Sprintf("long %d", k)] = s
	}
	for k := range 60 {
		took[fmt.Sprintf("short %d", k)] = float64(5 + k%20)
	}
	for name := range took {
		all = append(all, name)
	}
	slices.Sort(all)
	all = append(all, "new")
	same := func(x string) string { return x }
	sums := make([]float64, 3)
	for i := range sums {
		share := shareOf(t, all, same, took, fmt.Sprintf("%d/3", i+1))
		for k, x := range share {
			s, ok := took[x]
			if !ok {
				s = took["short 10"]
			}
			sums[i] += s
			if k > 0 && s > took[share[k-1]] && took[share[k-1]] != 0 {
				t.Errorf("share %d runs %s (%.0f s) after %s (%.0f s)", i+1, x, s, share[k-1], took[share[k-1]])
			}
		}
	}
	if spread := slices.Max(sums) - slices.Min(sums); spread > 24 {
		t.Errorf("the shares take %v s, %.0f s apart, more than the longest short run", sums, spread)
	}
	whole := shareOf(t, all, same, took, "")
	if whole[0] != "long 0" {
		t.Errorf("with no share, the first run is %s, not the longest", whole[0])
	}
}

// walk-times.tsv reads back what was written, keeps the old time of a
// run that did not run, drops a run that is gone, and leaves out a line
// it cannot read.
func TestTheTimesOfTheWalksAreKept(t *testing.T) {
	before := readWalkTimes(" 92\tsequences.mjs SEQUENCE=a short\n  7\tgone.mjs SEED=1\nnot a line\n-3\tnegative\n")
	if len(before) != 2 || before["sequences.mjs SEQUENCE=a short"] != 92 {
		t.Fatalf("read %v", before)
	}
	names := []string{"walk.mjs SEED=1", "sequences.mjs SEQUENCE=a short", "never.mjs"}
	file := writeWalkTimes(names, before, map[string]float64{"walk.mjs SEED=1": 0.4})
	want := " 92\tsequences.mjs SEQUENCE=a short\n  1\twalk.mjs SEED=1\n"
	if file != want {
		t.Errorf("wrote\n%q\nwant\n%q", file, want)
	}
	if again := writeWalkTimes(names, readWalkTimes(file), nil); again != file {
		t.Errorf("read and written again\n%q\nwas\n%q", again, file)
	}
}

// openLog starts the app's log in the bridge's home, four lines in the
// file in use and three in an older file beside it, see /app-log.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func (b *bridge) openLog() error {
	dir := filepath.Join(b.d.home, "Logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var older bytes.Buffer
	z := gzip.NewWriter(&older)
	_, _ = z.Write([]byte("{}\n{}\n{}\n"))
	_ = z.Close()
	if err := os.WriteFile(filepath.Join(dir, "app-2026-10-09T10-00-00.000.log.gz"), older.Bytes(), 0o600); err != nil {
		return err
	}
	theLog.open(filepath.Join(dir, "app.log"))
	for i := range 4 {
		theLog.line(zerolog.InfoLevel, "app").Int("i", i).Msg("a line for the walk")
	}
	return nil
}
