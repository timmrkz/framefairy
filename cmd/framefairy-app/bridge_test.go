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
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"io"
	"maps"
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
	"framefairy/engine/runlog"
	"framefairy/internal/fixtures"
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
// whether the call reaches the service first and only its answer waits,
// and the reason it fails with instead of reaching it, if it does, see
// /hold. fresh is the bridge's machine on its first run, or nil when it
// is not on one, see /fresh.
func callHandler(svc func() *FrameFairy, own map[string]func() (any, error),
	held func(name string) (gate <-chan struct{}, late bool, fails string), fresh func() *freshMachine) http.Handler {
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
		gate, late, fails := held(name)
		opened := func() bool {
			select {
			case <-gate:
				return true
			case <-r.Context().Done():
				return false
			}
		}
		if gate != nil && !late && !opened() {
			return
		}
		if fails != "" {
			http.Error(w, fails, http.StatusInternalServerError)
			return
		}
		if answerOwn, ok := own[name]; ok {
			answer, err := answerOwn()
			if gate != nil && late && !opened() {
				return
			}
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
		// A key saved on the first run is kept by the bridge, never the
		// keychain, and never shown to the company it is for.
		if m := fresh(); name == "SaveAPIKey" && m != nil {
			var provider, key string
			if len(call.Args) != 2 || json.Unmarshal(call.Args[0], &provider) != nil || json.Unmarshal(call.Args[1], &key) != nil {
				http.Error(w, "SaveAPIKey takes a provider and a key", http.StatusBadRequest)
				return
			}
			m.saveKey(provider, key)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nil)
			return
		}
		if outside[name] && !(walkedUpdates[name] && svc().updates != nil) && !(firstRun[name] && fresh() != nil) {
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
		if gate != nil && late && !opened() {
			return
		}
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
// ones the walk let arrive, the machine has the memory the walk gave it,
// so the models offered are the same on every machine the walk runs on,
// and a key is there once the walk saved one.
func standInsSetup(s SetupState, fresh *freshMachine) SetupState {
	s.HasServer = true
	if fresh != nil {
		s.Memory = fresh.memory
		best, hasBest := engine.RecommendedFor(s.Memory)
		for i := range s.Language {
			s.Language[i].Fit = string(s.Language[i].FitsIn(s.Memory))
			s.Language[i].Recommended = hasBest && s.Language[i].Name == best.Name
		}
		s.Keys, s.KeyHints = map[string]string{}, map[string]string{}
		for _, provider := range fresh.keyed() {
			s.Keys[provider], s.KeyHints[provider] = "keychain", "sk-…walk"
		}
		s.HasKey = s.Keys[s.Provider] != ""
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
	mux.Handle("/call", b.calls.serve(callHandler(svc, own, b.held, b.fresh.Load)))
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
		if err := b.reset(); err != nil {
			answer(w, nil, err)
			return
		}
		if !r.URL.Query().Has("seconds") {
			answer(w, nil, nil)
			return
		}
		q := r.URL.Query()
		path, err := b.startWith(q.Get("seconds"), q.Get("rate"), q.Get("timing"))
		answer(w, path, err)
	})
	// A mark sent down the events behind everything the Go side told the
	// interface before it. A page that has heard the mark has heard all
	// of that, since every page hears the events in the order they were
	// sent, so a walk can wait until the interface knows what the engine
	// knew when the walk asked it, see caughtUp in bridge.mjs.
	mux.HandleFunc("/mark", func(w http.ResponseWriter, r *http.Request) {
		sendDue(b.d.svc.news)
		n := b.marks.Add(1)
		b.d.bus.send("mark", n)
		answer(w, n, nil)
	})
	// The machine's clock, see walkTime: held, let go, run on by run
	// milliseconds, with every call due by then made before it answers,
	// the way a walk runs the page's clock on, see runClock in
	// walks/bridge.mjs, or run on to the next call due.
	mux.HandleFunc("/clock", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("hold") {
			b.d.time.hold()
			answer(w, true, nil)
			return
		}
		if q.Has("go") {
			b.d.time.letGo()
			answer(w, true, nil)
			return
		}
		if q.Has("next") {
			ran, err := b.d.time.next(r.Context())
			answer(w, ran.Milliseconds(), err)
			return
		}
		ms, err := strconv.Atoi(q.Get("run"))
		if err != nil || ms < 0 {
			http.Error(w, "run says how many milliseconds to run the clock on", http.StatusBadRequest)
			return
		}
		b.d.time.run(time.Duration(ms) * time.Millisecond)
		answer(w, ms, nil)
	})
	mux.HandleFunc("/pick", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		path, err := b.pick(q.Get("seconds"), q.Get("rate"), q.Get("switch"), q.Get("timing"), q.Get("codec") == "hevc",
			q.Get("box") != "0", cameraHeight(q.Get("size")))
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
	// step. It answers how many steps it took. Or ahead=n hands over n
	// tenths of a second with no pass of the speech model, the person
	// talking faster than it writes down, and answers how many it handed.
	mux.HandleFunc("/microphone", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("silent") {
			b.sound.silent.Store(q.Get("silent") == "1")
			answer(w, nil, nil)
			return
		}
		if n, err := strconv.Atoi(q.Get("ahead")); err == nil {
			handed, err := b.sound.ahead(r.Context(), min(max(n, 0), 100))
			answer(w, handed, err)
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
			if q.Has("heard") {
				b.words.stepHeard(r.Context(), min(max(n, 0), 100))
			} else {
				b.words.step(min(max(n, 0), 100))
			}
			answer(w, nil, nil)
			return
		}
		b.words.release()
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
		// every call when it is not given. answer holds the answer rather
		// than the call: the Go side does what was asked and the answer
		// waits, the way a busy machine hands it back late, after the
		// news of what it set going.
		times, err := strconv.Atoi(q.Get("times"))
		if err != nil {
			times = -1
		}
		late := q.Get("answer") != ""
		b.mu.Lock()
		switch g, shut := b.gates[q.Get("call")]; {
		case q.Get("gate") == "shut" && !shut:
			b.gates[q.Get("call")] = &gate{open: make(chan struct{}), left: min(times, 100), late: late}
		case q.Get("gate") == "shut":
			g.left, g.late = min(times, 100), late
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
		memory, err := strconv.ParseInt(cmp.Or(r.URL.Query().Get("memory"), "32"), 10, 64)
		if err != nil || memory <= 0 {
			http.Error(w, "memory is a number of gigabytes", http.StatusBadRequest)
			return
		}
		answer(w, nil, b.makeFresh(memory<<30))
	})
	// A model deleted outside the app, the way Tim removed one from the
	// models folder in Finder: the first of the kind, "language" or
	// "speech", that is here. It answers the model's name.
	mux.HandleFunc("/model-gone", func(w http.ResponseWriter, r *http.Request) {
		name, err := modelGone(r.URL.Query().Get("kind"))
		answer(w, name, err)
	})
	mux.HandleFunc("/reopen", func(w http.ResponseWriter, r *http.Request) {
		closed, err := b.reopen()
		answer(w, closed, err)
	})
	// The app closed, and opened again later, so a walk changes the disk
	// in between the way a person does while it is closed: the app's
	// watch on the disk does not hear it.
	mux.HandleFunc("/close", func(w http.ResponseWriter, r *http.Request) {
		closed, err := b.close()
		answer(w, closed, err)
	})
	mux.HandleFunc("/open", func(w http.ResponseWriter, r *http.Request) {
		b.d.open()
		answer(w, true, nil)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaMiddleware(b.d.svc)(reportMiddleware(b.d.svc.report)(http.FileServer(http.Dir(dist)))).ServeHTTP(noStore{w}, r)
	}))
	return mux
}

// noStore has the page fetch every file of the bridge again, whatever the
// app says it may keep, like a font for a day. A walker opens sequence
// after sequence on one page, see openAgain in walks/bridge.mjs, and a
// sequence then finds what it needs the way a new page does, whichever
// sequence walked before it.
type noStore struct{ http.ResponseWriter }

func (w noStore) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(code)
}

func (w noStore) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(p)
}

// Unwrap hands http.ResponseController the writer underneath.
func (w noStore) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
	// ended counts the hearings that have ended, done, called off or
	// failed, each of which closes it, so a wait for the next piece
	// ends with the hearing that would have asked for it.
	ended int
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

// stepHeard lets n pieces more be heard, the way step does, and returns
// once they have all been heard and the next piece waits, so a walk can let many pieces on at once and go on when
// they are all in. The engine hears one piece after the other and says
// how far it came after each, before it asks for the next, so by then it
// has said so of all of them. It is for pieces that are not the last: a
// video heard to its end asks for no next piece, and the wait ends with
// the hearing instead, the same as for one that fails or is called off,
// and the walk then finds out how it ended. A walk that gives up on it,
// or a reset, ends the wait too.
// The pieces are let on under the same lock the count of hearings ended
// is read under, so a hearing that ends before the wait starts is not
// missed.
func (p *prose) stepHeard(ctx context.Context, n int) {
	stop := context.AfterFunc(ctx, func() {
		p.gate.Lock()
		defer p.gate.Unlock()
		p.cond().Broadcast()
	})
	defer stop()
	p.gate.Lock()
	defer p.gate.Unlock()
	ended := p.ended
	p.steps += n
	p.cond().Broadcast()
	for (p.steps > 0 || p.hearing > 0 || p.waiting == 0) && p.held && p.ended == ended && ctx.Err() == nil {
		p.cond().Wait()
	}
}

// The walk that lets many pieces on at once goes on once they are heard
// and the next waits, and does not wait for good when the hearing ends
// with no next piece, done, called off or failed.
func TestHeardWaitsForTheNextPieceOrTheEndOfTheHearing(t *testing.T) {
	t.Parallel()
	p := &prose{}
	p.hold()
	samples := make([]float32, 16000)
	hears := func(n int) {
		for range n {
			p.Recognize(samples, 16000)
		}
	}
	// Two pieces let on, and a third asked for, which waits.
	done := make(chan struct{})
	go func() { hears(3); close(done) }()
	p.stepHeard(t.Context(), 2)
	p.gate.Lock()
	waiting, steps := p.waiting, p.steps
	p.gate.Unlock()
	if waiting != 1 || steps != 0 {
		t.Fatalf("heard returned with %d pieces waiting and %d steps left, not 1 and 0", waiting, steps)
	}
	p.release()
	<-done

	// One piece let on, and the hearing ends after it with no next.
	p.hold()
	go func() { hears(1); p.Close() }()
	p.stepHeard(t.Context(), 1)
	p.gate.Lock()
	ended := p.ended
	p.gate.Unlock()
	if ended != 1 {
		t.Fatalf("heard returned before the hearing ended, %d ended", ended)
	}
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

// quick are words said in a hundredth of a second and straight into the
// word after, the way words typed into one short word are: too short for a
// frame of their own, so each is lit on the frame it shares with the
// words beside it, #280. "so" starts a caption between two frames, "sich"
// is in the middle of the same one and "wir" ends the one after, so the
// caption walks meet words that share a frame wherever they go: edges
// dragged, words typed and removed, blocks clicked, Shift and the arrows.
// The word after a quick word starts straight after it and ends where it
// always did, so every other word keeps its time.
var quick = map[string]bool{"so": true, "sich": true, "wir": true}

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
	// The time a quick word leaves over goes to the word after it.
	carry := 0.0
	for at := 0.1; ; {
		word := proseText[p.next%len(proseText)]
		// Longer words take longer to say.
		lasts, after := 0.12+0.05*float64(len([]rune(word))), 0.08
		said := lasts + carry
		if quick[word] {
			said = 0.01
		}
		if at+said > length-0.05 {
			break
		}
		tokens = append(tokens, engine.Token{Text: " " + word, Start: at, Duration: said})
		p.next++
		if quick[word] {
			carry = lasts + after - said
			at += said
		} else {
			at += said + after
			carry = 0
		}
		if strings.HasSuffix(word, ".") {
			at += 0.9
		}
	}
	return tokens
}

// Close is called as each hearing ends, see ended.
func (p *prose) Close() {
	p.gate.Lock()
	defer p.gate.Unlock()
	p.ended++
	p.cond().Broadcast()
}

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
	// no video in the app, see /fresh. Nil when it is not on one.
	fresh atomic.Pointer[freshMachine]
	// arrive lets the download of a kind of model, "speech" or "clip",
	// arrive, closed by the walk, see arriving.
	arrive map[string]chan struct{}
	// calls are the page's calls running in the Go side, which the reset
	// waits for, see pageCalls.
	calls pageCalls
	// prepared is the folder the bridges of the walks keep the videos
	// they started a walk with in, see startWith, or "" for this
	// bridge's own.
	prepared string
}

// pageCalls are the calls of a page the Go side is running. A walk's page
// is closed before the next walk resets the bridge, and a call it had sent
// runs on in the Go side all the same: the page going away does not stop
// a method, and a call held at a gate is let through when the reset opens
// the gates. Such a call wrote into a video's work folder while the reset
// removed it, and the reset failed with "directory not empty", issue
// #709. The reset waits for every call that runs, and a call that comes
// while it resets is the page's before it and is turned away, the way the
// app's process ending takes its calls with it.
type pageCalls struct {
	mu       sync.Mutex
	running  int
	stopping bool
	// idle is closed once the last call that runs has ended, while a
	// reset waits for it.
	idle chan struct{}
}

// serve runs a call, unless a reset is running.
func (c *pageCalls) serve(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		if c.stopping {
			c.mu.Unlock()
			http.Error(w, "the page that made this call was left behind by a reset", http.StatusGone)
			return
		}
		c.running++
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.running--
			if c.running == 0 && c.idle != nil {
				close(c.idle)
				c.idle = nil
			}
		}()
		h.ServeHTTP(w, r)
	})
}

// stop waits for every call that runs to end, and turns away every call
// that comes until done is called.
func (c *pageCalls) stop() (done func()) {
	c.mu.Lock()
	c.stopping = true
	var idle chan struct{}
	if c.running > 0 {
		c.idle = make(chan struct{})
		idle = c.idle
	}
	c.mu.Unlock()
	if idle != nil {
		<-idle
	}
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.stopping = false
	}
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
// left is below nought, or the next left of them. Late, it holds their
// answers instead.
type gate struct {
	open chan struct{}
	left int
	late bool
}

// held is the gate a call of this name waits behind, whether only its
// answer waits, and the reason it fails with instead, if it does, see
// /hold.
func (b *bridge) held(name string) (<-chan struct{}, bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fails := b.fails[name]
	delete(b.fails, name)
	g := b.gates[name]
	if g == nil || g.left == 0 {
		return nil, false, fails
	}
	if g.left > 0 {
		g.left--
	}
	return g.open, g.late, fails
}

// openBridge opens a desk whose Go side tells a browser what it does, with
// one episode in it, heard and searched, so the workspace has a clip.
//
// On the real disk: ffmpeg and the episode's decoder read the episode.
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
	d.fakes = fakes
	// A font installed on the bridge's computer: Anton under a name of its
	// own, so the Font list has a face of the machine to offer, and libass
	// and the word typed are drawn from its copy.
	anton, _ := new(engine.Fonts).Bytes("Anton")
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
	inter, _ := new(engine.Fonts).Bytes(engine.DefaultFont)
	archivo, _ := new(engine.Fonts).Bytes("Archivo Black")
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
	// The desk is started again with a bus, before anything is in it.
	d.svc.jobs.shutDown()
	d.svc.watch.stop()
	d.svc.store.standIns = fakes
	d.bus = newBus()
	// The bridge's microphone talks for as long as a walk takes steps of
	// a recording, so one also stops after a minute, 600 steps, long after
	// every sequence has heard what it waits for, and a walk waiting for
	// more words ends there.
	d.most = 60 * engine.SampleRate
	d.time = newWalkTime()
	b := &bridge{t: t, d: d, words: words, sound: snd, first: filepath.Join(d.home, "erste-erinnerung.mp4"), kept: t.TempDir(),
		gates: map[string]*gate{}, fails: map[string]string{}}
	d.fit = func(svc *FrameFairy) {
		// Every opening of the app finds the fonts installed on the
		// bridge's computer.
		svc.store.machine.Fonts.Use(installed, []string{"/Library/Fonts"})
		svc.listening.mic = snd.open
		// A report goes on the bridge's own Desktop and is shown nowhere.
		svc.reportFolder = func() string {
			desk := filepath.Join(d.home, "Desktop")
			_ = os.MkdirAll(desk, 0o755)
			return desk
		}
		svc.showReport = func(string) error { return nil }
		// What the first run downloads, once a walk made the machine new.
		// What arrives is as much of a model as the app looks for, and it
		// lands the way the engine's download lands it: written under a
		// name of its own and renamed into place whole.
		svc.installSpeech = func(_ *engine.Machine, ctx context.Context, log *runlog.Log, m engine.SpeechModel, dir string) error {
			return b.arriving(ctx, log, "speech", m.Download, func() error {
				staging := filepath.Join(dir, m.Name+".unpacking")
				if err := os.MkdirAll(staging, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(staging, "tokens.txt"), []byte("the stand-in hears\n"), 0o644); err != nil {
					return err
				}
				return os.Rename(staging, filepath.Join(dir, m.Name))
			})
		}
		svc.installLanguage = func(_ *engine.Machine, ctx context.Context, log *runlog.Log, m engine.LanguageModel, dir string) error {
			return b.arriving(ctx, log, "clip", m.Download, func() error {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				part := filepath.Join(dir, m.Name+".part")
				if err := os.WriteFile(part, []byte("GGUF, the stand-in finds clips"), 0o644); err != nil {
					return err
				}
				return os.Rename(part, filepath.Join(dir, m.Name))
			})
		}
	}
	// The setup the app sends when the models change is the bridge's
	// machine's, like the one every call answers with, so the desk starts
	// again only once it knows the bridge.
	d.setup = func(state SetupState) SetupState { return standInsSetup(state, b.fresh.Load()) }
	d.start(d.svc.store)
	b.prepared = os.Getenv("FRAMEFAIRY_BRIDGE_PREPARED")
	fixtures.Copy(t, "walk-120s.mp4", b.first)
	if from := os.Getenv("FRAMEFAIRY_BRIDGE_FROM"); from != "" {
		// The episode as another bridge of the same walks heard and
		// searched it, so the six bridges of a job do that work once.
		if err := b.takeHome(from); err != nil {
			t.Fatal(err)
		}
	} else {
		if added, err := d.svc.addEpisodes([]string{b.first}); err != nil || len(added) != 1 {
			t.Fatalf("adding %s: %v", b.first, err)
		}
		d.idle(b.first)
	}
	if len(d.clips(b.first)) == 0 {
		t.Fatalf("the episode has no clip to work on: %+v", d.svc.jobs.list())
	}
	for _, dir := range []string{workOf(b.first), filepath.Join(d.home, "config")} {
		if err := os.CopyFS(filepath.Join(b.kept, filepath.Base(dir)), os.DirFS(dir)); err != nil {
			t.Fatal(err)
		}
	}
	b.said = words.at()
	if keep := os.Getenv("FRAMEFAIRY_BRIDGE_KEEP"); keep != "" {
		if err := b.keepHome(keep); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// frameTimings are the timings a walk's video may have, each a fixture
// of its own, see filmed in internal/fixtures: a picture a quarter of a
// second or 4 ms after its sound, and a phone's picture stored on its
// side.
var frameTimings = map[string]bool{"late": true, "late4ms": true, "turned": true}

// startWith puts a second video in the app reset a moment ago, heard and
// searched, the way Add and its first search leave it, for a walk that
// only needs a video of this kind to work on: seconds long, filmed at
// rate, with its frames timed by timing, see pick. It says where the video is. The first bridge of the walks to be
// asked for a kind adds it and keeps what the app made of it in the
// folder FRAMEFAIRY_BRIDGE_PREPARED names, and every bridge after it
// puts that back, so the work of hearing and searching it is done once
// for all of them. Two bridges asked at once both add it, and the folder
// keeps the first, which holds the same. Either way the app is opened
// again on what is on the disk, so a walk finds the same app whichever
// bridge made the video.
//
// On the real disk: the bridges of the walks are processes of their own, and ffmpeg reads the video.
func (b *bridge) startWith(seconds, rate, timing string) (string, error) {
	n, err := strconv.Atoi(seconds)
	if err != nil || n < 10 || n > 3600 {
		return "", fmt.Errorf("%q is no length of a video to start with", seconds)
	}
	if rate != "" && !frameRate.MatchString(rate) {
		return "", fmt.Errorf("%q is no frame rate", rate)
	}
	if timing != "" && !frameTimings[timing] {
		return "", fmt.Errorf("%q is no timing of frames", timing)
	}
	file := walkFixture(n, rate, 0, timing, false)
	// Named by its kind, so it never meets a video a walk picks next,
	// folge-1 and on.
	path := filepath.Join(b.d.home, file)
	if err := fixtures.CopyTo(file, path); err != nil {
		return "", err
	}
	prepared := b.prepared
	if prepared == "" {
		prepared = filepath.Join(b.kept, "prepared")
	}
	if err := os.MkdirAll(prepared, 0o755); err != nil {
		return "", err
	}
	kept := filepath.Join(prepared, file)
	work := filepath.Base(workOf(path))
	if _, err := os.Stat(kept); err == nil {
		if err := b.stopForTheDisk(); err != nil {
			return "", err
		}
		home, err := os.ReadFile(filepath.Join(kept, "home"))
		if err != nil {
			return "", err
		}
		said, err := os.ReadFile(filepath.Join(kept, "said"))
		if err != nil {
			return "", err
		}
		next, err := strconv.Atoi(string(said))
		if err != nil {
			return "", err
		}
		// The library names both videos, so the settings come back whole.
		if err := os.RemoveAll(filepath.Join(b.d.home, "config")); err != nil {
			return "", err
		}
		for _, dir := range []string{work, "config"} {
			if err := copyHomed(filepath.Join(kept, dir), filepath.Join(b.d.home, dir), string(home), b.d.home); err != nil {
				return "", err
			}
		}
		if err := takeStamp(kept, path); err != nil {
			return "", err
		}
		b.words.from(next)
	} else {
		added := b.d.svc.takeFiles("add", []string{path})
		if added.Error != "" || len(added.Added) != 1 {
			return "", fmt.Errorf("adding %s: %s", file, added.Error)
		}
		b.d.idle(path)
		if len(b.d.clips(path)) == 0 {
			return "", fmt.Errorf("%s has no clip once searched: %+v", file, b.d.svc.jobs.list())
		}
		if err := b.stopForTheDisk(); err != nil {
			return "", err
		}
		staging, err := os.MkdirTemp(filepath.Dir(kept), file+".")
		if err != nil {
			return "", err
		}
		for _, dir := range []string{work, "config"} {
			if err := os.CopyFS(filepath.Join(staging, dir), os.DirFS(filepath.Join(b.d.home, dir))); err != nil {
				return "", err
			}
		}
		if err := os.WriteFile(filepath.Join(staging, "said"), []byte(strconv.Itoa(b.words.at())), 0o644); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(staging, "home"), []byte(b.d.home), 0o644); err != nil {
			return "", err
		}
		if err := keepStamp(staging, path); err != nil {
			return "", err
		}
		// Whole or not at all. Another bridge that kept it first keeps
		// its own, which holds the same.
		if err := os.Rename(staging, kept); err != nil {
			_ = os.RemoveAll(staging)
		}
	}
	b.d.openAgainOnTheSameComputer()
	b.d.idle(path)
	return path, nil
}

// stopForTheDisk stops the work and the watch on the folders of the app,
// the way reset does before it puts the folders back, so the app does not
// hear its own folders being written.
func (b *bridge) stopForTheDisk() error {
	if !b.d.svc.jobs.shutDown() {
		return fmt.Errorf("the app did not stop its work in time")
	}
	b.d.svc.levels.shutDown()
	b.d.svc.watch.stop()
	return nil
}

// keepHome writes the episode's work folder and the app's settings as
// they were once the episode was first searched into dir, with the word
// the speech stand-in says next then and the home they name, for the
// other bridges of the walks to start from, see takeHome.
//
// On the real disk: the bridges of the walks are processes of their own.
func (b *bridge) keepHome(dir string) error {
	staging := dir + ".writing"
	if err := os.CopyFS(staging, os.DirFS(b.kept)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "said"), []byte(strconv.Itoa(b.said)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(staging, "home"), []byte(b.d.home), 0o644); err != nil {
		return err
	}
	if err := keepStamp(staging, b.first); err != nil {
		return err
	}
	// Whole or not at all: a bridge that finds the folder finds all of it.
	return os.Rename(staging, dir)
}

// takeHome puts the episode's work folder and the app's settings another
// bridge kept with keepHome in this bridge's home, the home they name
// written as this one's, and opens the app again on them, with the speech
// stand-in going on from the word that bridge's went on from.
//
// On the real disk: the bridges of the walks are processes of their own.
func (b *bridge) takeHome(dir string) error {
	said, err := os.ReadFile(filepath.Join(dir, "said"))
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(string(said))
	if err != nil {
		return err
	}
	home, err := os.ReadFile(filepath.Join(dir, "home"))
	if err != nil {
		return err
	}
	b.d.svc.watch.stop()
	if err := takeStamp(dir, b.first); err != nil {
		return err
	}
	for _, name := range []string{filepath.Base(workOf(b.first)), "config"} {
		if err := copyHomed(filepath.Join(dir, name), filepath.Join(b.d.home, name), string(home), b.d.home); err != nil {
			return err
		}
	}
	b.words.from(n)
	b.d.openAgainOnTheSameComputer()
	b.d.idle(b.first)
	return nil
}

// keepStamp writes when the video was last changed into dir, for
// takeStamp. The transcript is stamped with it, so a copy of the video
// with a time of its own is a video never heard.
//
// On the real disk: the bridges of the walks are processes of their own.
func keepStamp(dir, video string) error {
	info, err := os.Stat(video)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "modified"), []byte(strconv.FormatInt(info.ModTime().UnixNano(), 10)), 0o644)
}

// takeStamp gives the video the time keepStamp wrote into dir.
//
// On the real disk: the bridges of the walks are processes of their own.
func takeStamp(dir, video string) error {
	kept, err := os.ReadFile(filepath.Join(dir, "modified"))
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(string(kept), 10, 64)
	if err != nil {
		return err
	}
	at := time.Unix(0, n)
	return os.Chtimes(video, at, at)
}

// copyHomed copies the folder from to the folder to, with every file
// that names the home it was made in, from, naming home instead: the
// library and the settings hold the paths of the videos and folders in
// it.
//
// On the real disk: it copies what one bridge made into another's home.
func copyHomed(from, to, was, home string) error {
	return filepath.WalkDir(from, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if e.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, bytes.ReplaceAll(data, []byte(was), []byte(home)), 0o644)
	})
}

func workOf(video string) string {
	return strings.TrimSuffix(video, filepath.Ext(video)) + ".framefairy"
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

// pick puts a new video, not in the library, in the bridge's home, and
// has the Add button's box hand it over next. Without a rate it is the
// bridge's own kind of video, at five frames a second, and with one it is
// filmed at that rate. With a switch, in seconds, the bridge's own kind is
// filmed by two cameras that switch then, and with hevc it is HEVC with
// 10-bit colour. Each is a copy of a video committed in internal/fixtures, see
// walkFixture, so one that is not there is refused with its name. With a
// height it is a camera's picture of that height instead, see
// makeCameraEpisode.
//
// On the real disk: the app adds the video from the disk.
func (b *bridge) pick(seconds, rate, switchAt, timing string, hevc, box bool, height int) (string, error) {
	n, err := strconv.Atoi(seconds)
	if err != nil || n < 10 || n > 3600 {
		n = 90
	}
	if rate != "" && !frameRate.MatchString(rate) {
		return "", fmt.Errorf("%q is no frame rate", rate)
	}
	if timing != "" && !frameTimings[timing] {
		return "", fmt.Errorf("%q is no timing of frames", timing)
	}
	at, err := strconv.ParseFloat(switchAt, 64)
	if err != nil || at <= 0 || at >= float64(n) {
		at = 0
	}
	b.mu.Lock()
	b.made++
	path := filepath.Join(b.d.home, fmt.Sprintf("folge-%d.mp4", b.made))
	b.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if height > 0 {
		err = makeCameraEpisode(path, n, height, hevc)
	} else {
		err = fixtures.CopyTo(walkFixture(n, rate, at, timing, hevc), path)
	}
	if err != nil {
		return "", err
	}
	if box {
		b.mu.Lock()
		b.picks = append(b.picks, path)
		b.mu.Unlock()
	}
	return path, nil
}

// walkFixture names the video committed in internal/fixtures that a pick
// asks for: walk, cameras or filmed and its length, and then its switch,
// its frame rate and its timing where it has them.
func walkFixture(seconds int, rate string, switchAt float64, timing string, hevc bool) string {
	name := fmt.Sprintf("walk-%ds", seconds)
	switch {
	case rate != "":
		name = fmt.Sprintf("filmed-%ds-%s", seconds, strings.NewReplacer("30000/1001", "29.97", "24000/1001", "23.976").Replace(rate))
		if timing != "" {
			name += "-" + timing
		}
	case switchAt > 0:
		name = fmt.Sprintf("cameras-%ds-%s", seconds, strconv.FormatFloat(switchAt, 'g', -1, 64))
	}
	if hevc && rate == "" {
		name += "-hevc"
	}
	return name + ".mp4"
}

// pickPicture makes a picture of w by h pixels in the bridge's home, a
// PNG with a gradient so it is not empty, or a file of text named like a
// picture, or puts a recording of one second there, and has the
// paperclip's box hand it over next when box.
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
		// A recording in a MOV, the way the Mac's own recorder makes one,
		// with a sound.
		if err := fixtures.CopyTo("recording.mov", path); err != nil {
			return "", err
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

// pickSound puts a file of sound with no picture there, named like a video,
// and has the Add button's box hand it over next.
func (b *bridge) pickSound() (string, error) {
	b.mu.Lock()
	b.made++
	path := filepath.Join(b.d.home, fmt.Sprintf("nur-ton-%d.mp4", b.made))
	b.mu.Unlock()
	if err := fixtures.CopyTo("tone-then-silence.m4a", path); err != nil {
		return "", err
	}
	b.mu.Lock()
	b.picks = append(b.picks, path)
	b.mu.Unlock()
	return path, nil
}

// makeFresh makes the bridge's machine one the app has never run on, see
// /fresh, with memory bytes of memory. The video leaves the library with
// its work folder kept, the way Remove leaves it, and reset brings both
// back.
//
// On the real disk: the models folder is the one the app looks in for its models, in the bridge's home.
func (b *bridge) makeFresh(memory int64) error {
	b.mu.Lock()
	b.arrive = map[string]chan struct{}{"speech": make(chan struct{}), "clip": make(chan struct{})}
	b.mu.Unlock()
	b.fresh.Store(&freshMachine{memory: memory})
	// The folder over the models goes too: the app never ran here, so
	// the first run's download makes both, see modelsMade.
	if err := os.RemoveAll(filepath.Dir(engine.ModelsDir())); err != nil {
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

// modelGone deletes the first model of a kind that is here, "language" or
// "speech", from the models folder, the way Finder does, and says which.
//
// On the real disk: the models folder is the one the app watches, in the
// bridge's home.
func modelGone(kind string) (string, error) {
	dir := engine.ModelsDir()
	switch kind {
	case "language":
		for _, m := range engine.LanguageModels() {
			if m.Installed(dir) {
				return m.Name, os.Remove(filepath.Join(dir, m.Name))
			}
		}
	case "speech":
		for _, m := range engine.SpeechModels() {
			if m.Installed(dir) {
				return m.Name, os.RemoveAll(filepath.Join(dir, m.Name))
			}
		}
	default:
		return "", fmt.Errorf("no kind of model is called %q", kind)
	}
	return "", fmt.Errorf("no %s model is here", kind)
}

// freshMachine is the bridge's machine on its first run, see /fresh: the
// memory it has and the companies a key was saved for.
type freshMachine struct {
	memory int64
	mu     sync.Mutex
	keys   []string
}

// saveKey keeps a key for a provider the way the keychain would, or takes
// it away when the key is empty.
func (m *freshMachine) saveKey(provider, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys = slices.DeleteFunc(m.keys, func(p string) bool { return p == provider })
	if strings.TrimSpace(key) != "" {
		m.keys = append(m.keys, provider)
	}
}

// keyed are the providers a key is kept for.
func (m *freshMachine) keyed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.keys)
}

// letArrive lets the download of a kind of model on its way arrive, see
// arriving. A download of that kind pressed after it waits part way
// again, so a walk can look at it on its way in the settings, and a walk
// lets it arrive the same way or cancels it.
func (b *bridge) letArrive(kind string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	gate, ok := b.arrive[kind]
	if !ok {
		return fmt.Errorf("no download of a %q model to let arrive, the bridge is not on its first run", kind)
	}
	close(gate)
	b.arrive[kind] = make(chan struct{})
	return nil
}

// arriving is a download of the first run on the bridge, see /fresh. It
// says it has fetched two fifths, in the words the engine's download
// says it in, and holds there until the walk lets it arrive or it is
// cancelled. Then put lays the model down the way an install leaves it.
func (b *bridge) arriving(ctx context.Context, log *runlog.Log, kind string, size int64, put func() error) error {
	b.mu.Lock()
	gate := b.arrive[kind]
	b.mu.Unlock()
	if gate == nil {
		return fmt.Errorf("the bridge downloads nothing outside its first run")
	}
	// In the engine's words, see inMB there: gigabytes from a thousand
	// megabytes up.
	said := func(n int64) string {
		if n >= 1e9 {
			return fmt.Sprintf("%.1f GB", float64(n)/1e9)
		}
		return fmt.Sprintf("%d MB", n/1e6)
	}
	const rate = 120e6
	log.ProgressOf(fmt.Sprintf("fetching %s of %s at %s a second", said(size*2/5), said(size), said(rate)),
		0.4, float64(size)*3/5/rate)
	select {
	case <-ctx.Done():
		return engine.ErrCancelled
	case <-gate:
	}
	log.ClearProgress()
	return put()
}

// reopen closes the app and opens it again, the way quitting and starting
// it does: the work stops, and what it did and how it ended is read back.
// It gives back the jobs as the app left them, once nothing ran any more,
// so a walk knows how each ended on the engine's word: a search the
// closing cut off says cancelled, one that finished as the app closed says
// done. What a walk read off the screen before it asked is a moment older,
// and a search the model answers at once can end in that moment.
func (b *bridge) reopen() ([]Job, error) {
	closed, err := b.close()
	if err != nil {
		return nil, err
	}
	b.d.open()
	return closed, nil
}

// close closes the app the way quitting does, and says the jobs it
// closed on. Nothing runs and nothing hears the disk until open.
func (b *bridge) close() ([]Job, error) {
	if !b.words.whileClosing(func() bool { return quit(b.d.svc) }) {
		return nil, fmt.Errorf("the app did not stop its work in time")
	}
	return b.d.svc.jobs.list(), nil
}

// quit closes the app the one way it closes itself, closeDown, and then
// stops what the end of the app's process stops in the app, since the
// bridge opens the app again in the same process. The bridge's reset
// named the app's parts one by one before, and each time a part it did
// not name wrote into a work folder while the reset removed it and failed
// with "directory not empty": a caption file of the video preview, issue
// #561, a draft of Report a Problem, #632, a call of the page, #709, so
// it closes the app whole now, issue #713. It says whether the jobs
// stopped in time.
//
// What the process ending stops besides: a recording, which tells the bus
// every desk shares what it hears, and the app opened again numbers its
// recordings from 1, so the next walk's Dictate would take those words
// for its own. The caption files of the video preview, which go on being
// made after the request that asked is gone. The report, whose draft the
// page saves as a walk ends. The page's calls are the reset's, see
// pageCalls.
func quit(svc *FrameFairy) bool {
	stopped := svc.closeDown()
	svc.CancelListening()
	svc.store.machine.ClosePreviews()
	svc.report.end()
	return stopped
}

// reset puts the app back the way it was once the first episode was
// searched: the model answers again, every episode a walk added is gone,
// the episode's work folder and the app's settings are what they were, the
// speech stand-in says next the word it said next then, and the app is
// opened again on them, so nothing a walk did is kept in memory either.
// Only what the app learned about this computer's programs is kept, the
// way it is kept across walks on one computer, see
// desk.openAgainOnTheSameComputer.
// Every walk starts from the same place, and a seed walks the same way
// every time: a video it adds is heard with the same words, however many
// words the walks before it had heard.
//
// On the real disk: it puts back the library where ffmpeg and the episode's decoder read the episode.
func (b *bridge) reset() error {
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
	b.fresh.Store(nil)
	b.d.pictureMu.Lock()
	b.d.pictures = nil
	b.d.pictureMu.Unlock()
	b.d.reportMade.Store(0)
	b.d.reportFronted.Store(0)
	b.d.exported.Store(nil)
	// The app is closed the way it quits, and what its process ending
	// stops is stopped too, so no part of it writes while its folders go,
	// see quit.
	if !quit(b.d.svc) {
		return fmt.Errorf("the app did not stop its work in time")
	}
	// A log a walk started lives in the home, which goes next.
	theLog.close()
	// What the process ending does besides: the page's calls the Go side
	// still runs end with it. stop waits for every call still running
	// here and now, once the gates are open and the app has closed, so
	// nothing a call waits on is still held, and turns away the calls that
	// come after it. What it returns lets calls through again as the reset
	// ends.
	defer b.calls.stop()()
	entries, err := os.ReadDir(b.d.home)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == filepath.Base(b.first) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(b.d.home, e.Name())); err != nil {
			return fmt.Errorf("%w, and in it now %s", err, leftIn(filepath.Join(b.d.home, e.Name())))
		}
	}
	for _, dir := range []string{workOf(b.first), filepath.Join(b.d.home, "config")} {
		if err := os.CopyFS(dir, os.DirFS(filepath.Join(b.kept, filepath.Base(dir)))); err != nil {
			return err
		}
	}
	// After the work has stopped, so nothing hears on past it.
	b.words.from(b.said)
	freshDecoders(&b.d.svc.previews)
	b.d.openAgainOnTheSameComputer()
	return nil
}

// leftIn names what is in a folder the reset could not remove, each file
// with how long ago it was written, so a failed reset says what wrote into
// the folder while it was removed, and when.
//
// On the real disk: it reads the folder reset could not remove there.
func leftIn(dir string) string {
	var left []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if info, err := d.Info(); err == nil && !d.IsDir() {
			rel += fmt.Sprintf(" (%d bytes, written %d ms ago)", info.Size(), time.Since(info.ModTime()).Milliseconds()) //nolint:forbidigo // how long ago, said in an error, waits on nothing
		}
		left = append(left, rel)
		return nil
	})
	if len(left) == 0 {
		return "nothing"
	}
	return strings.Join(left, ", ")
}

// freshDecoders closes the streams and the episode's decoders the walk
// before left. A decoder keeps its cursors where that walk left them, and
// a jump of the next walk then went on from one of them rather than
// seek, so what a walk saw depended on the walks before it: a drag that
// should have shown its key frame showed none, issue #552. The next
// stream starts a decoder again, with its cursors warmed as the app's
// first are. A hold the walk before took is kept, so its release finds it.
func freshDecoders(p *previews) {
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
// asked, because it needs Node, Playwright's Chromium and WebKit, and the
// build:
//
//	make walks
//	make walks WALKS=40 STEPS=80   to look further
//
// Each walk takes WALKS seeds. With WALKSEED, which CI sets to the
// commit, the seeds are worked out from it, so every push walks ground no
// push walked before, and every share of one run takes the same seeds.
// Without it they are 1 to WALKS, the same every time. A walk prints its
// seed, so SEED=n walks a failure again step for step.
//
// They run side by side, half as many again as the machine has cores, or
// WALKERS. Each runs against a bridge of its own, started as a process of
// its own: a desk sets HOME for the whole process it is in, so two bridges
// in one would share the app's settings. One after another they took nine
// minutes. A run spent most of its time waiting then, on a clip that
// played in real time and on the quiet after every step, so four bridges kept four
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
	// Every walk in the folder, each for the same seeds, and the seeds
	// kept because they found a bug, each for its own walk.
	scripts := walkScripts(t, walks)
	for _, seed := range walkSeeds(os.Getenv("WALKSEED"), walksWanted) {
		for _, script := range scripts {
			runs = append(runs, walkRun{script: script, env: append([]string{fmt.Sprintf("SEED=%d", seed)}, steps...)})
		}
	}
	// On the real disk: the kept seeds are a file beside the walks.
	keptFile, err := os.ReadFile(filepath.Join(walks, "kept-seeds.txt"))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := keptSeeds(string(keptFile), scripts)
	if err != nil {
		t.Fatal("kept-seeds.txt: ", err)
	}
	for _, k := range kept {
		runs = append(runs, walkRun{script: k.script, env: append([]string{fmt.Sprintf("SEED=%d", k.seed)}, steps...)})
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
	runs = shareOf(t, all, func(r walkRun) string { return timedAs(name(r)) }, times, os.Getenv("WALKSHARD"))
	if len(runs) == 0 {
		t.Skip("this share has no runs")
	}
	var tookMu sync.Mutex
	took := map[string]float64{}
	// The processor time of the bridges, with what they started, see
	// cpuOf, held under tookMu too.
	var bridged time.Duration

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

	walkers := min(envNumber("WALKERS", runtime.NumCPU()*3/2), len(runs))
	// The machine's processor time while the walks run, the browsers'
	// renderers in it, which no program that waits for its children
	// counts: Chromium's zygote starts them.
	busyBefore, busy := machineCPU()
	started := time.Now()
	todo := make(chan walkRun, len(runs))
	for _, r := range runs {
		todo <- r
	}
	close(todo)
	// The first bridge hears and searches its episode and keeps what the
	// app made of it, and the others start from a copy, so the six of a
	// job do that work once. What a walk starts with is kept for all of
	// them too, see startWith. The bridges are processes of their own,
	// so what they share is in folders.
	home := filepath.Join(t.TempDir(), "home")
	prepared := t.TempDir()
	first := make(chan struct{})
	firstFailed := false
	var wg sync.WaitGroup
	for i := range walkers {
		wg.Go(func() {
			starts := []string{"FRAMEFAIRY_BRIDGE_PREPARED=" + prepared}
			if i == 0 {
				starts = append(starts, "FRAMEFAIRY_BRIDGE_KEEP="+home)
			} else if <-first; !firstFailed {
				starts = append(starts, "FRAMEFAIRY_BRIDGE_FROM="+home)
			}
			url, stopBridge, err := startBridge(t, starts...)
			if i == 0 {
				firstFailed = err != nil
				close(first)
			}
			if err != nil {
				t.Error(err)
				return
			}
			env := append(os.Environ(), "BRIDGE_URL="+url)
			// The walker's sequences, walked one after another by one
			// node, see sequencer.
			seqs := &sequencer{run: []string{node, filepath.Join(walks, "sequences.mjs"), "--serve"}, dir: walks, env: env}
			defer func() {
				seqs.stop(ctx)
				cpu := stopBridge()
				tookMu.Lock()
				defer tookMu.Unlock()
				bridged += cpu
			}()
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
	// Where this machine's time went, every run by what it took, so a slow
	// walks job says which runs made it slow. Written to the file
	// WALKSUMMARY names, which make walks prints after go test, since go
	// test prints nothing a passing test said. On the clock only to be
	// read, never waited on.
	//
	// On the real disk: make walks reads the file once go test has ended.
	if file := os.Getenv("WALKSUMMARY"); file != "" {
		spent := map[string]float64{bridgesSpent: bridged.Seconds()}
		if after, ok := machineCPU(); ok && busy {
			spent[machineSpent] = (after - busyBefore).Seconds()
		}
		summary := walkSummary(took, times, spent, len(runs), walkers, time.Since(started).Seconds()) //nolint:forbidigo // the seconds the walks took, reported, never waited on
		if err := os.WriteFile(file, []byte(summary), 0o644); err != nil {
			t.Error(err)
		}
	}
	if left := len(todo); left > 0 {
		t.Errorf("%d runs were not walked, since no bridge started", left)
	}
	if n := unwalked.Load(); n > 0 {
		t.Errorf("%d runs were not walked, since the test's time ran out", n)
	}
	// WALKTIMES=1 keeps what the runs took for the next split, and drops
	// the runs that are gone.
	if os.Getenv("WALKTIMES") != "" {
		var names []string
		for _, r := range all {
			if n := timedAs(name(r)); !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
		if err := os.WriteFile(timesFile, []byte(writeWalkTimes(names, times, timedTook(took))), 0o644); err != nil {
			t.Error(err)
		}
	}
}

// startBridge starts TestBridge in a process of its own, on a free port,
// and gives back where it answers, and stop, which ends it and says what
// processor time it took, see cpuOf. It is stopped when the test ends,
// if stop was not called before.
func startBridge(t *testing.T, env ...string) (string, func() time.Duration, error) {
	cmd := exec.Command(os.Args[0], bridgeArgs(flag.Lookup("test.gocoverdir").Value.String())...)
	cmd.Env = append(append(os.Environ(), "FRAMEFAIRY_BRIDGE=127.0.0.1:0", "FRAMEFAIRY_BRIDGE_WALKER=1", "FRAMEFAIRY_WALKS="), env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", nil, err
	}
	// What it says, read until it says where it answers and let go of
	// after that.
	said, says := io.Pipe()
	cmd.Stdout, cmd.Stderr = says, says
	if err := cmd.Start(); err != nil {
		return "", nil, err
	}
	ended := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = says.Close()
		close(ended)
	}()
	stop := sync.OnceValue(func() time.Duration {
		_ = stdin.Close()
		<-ended
		return cpuOf(cmd)
	})
	t.Cleanup(func() { stop() })
	lines := bufio.NewScanner(said)
	var before []string
	for lines.Scan() {
		line := lines.Text()
		if rest, ok := strings.CutPrefix(line, "bridge on "); ok {
			url, _, _ := strings.Cut(rest, " ")
			go func() { _, _ = io.Copy(io.Discard, said) }()
			return url + "/", stop, nil
		}
		before = append(before, line)
	}
	return "", nil, fmt.Errorf("the bridge did not start:\n%s", strings.Join(before, "\n"))
}

// bridgeArgs is how TestWalks starts a bridge. When go test counts the
// lines the walks run, make walks COVERAGE=, it hands the test the folder
// it gathers the counts from, coverdir, and the bridge, the same program,
// writes its own counts there when it stops, so the lines of the app and
// the engine the walks reach are counted. See Coverage in
// docs/TESTING.md.
func bridgeArgs(coverdir string) []string {
	args := []string{"-test.run", "^TestBridge$", "-test.timeout", "0"}
	if coverdir != "" {
		args = append(args, "-test.gocoverdir="+coverdir)
	}
	return args
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

func TestAWalkersNodeThatEndsOnItsOwnIsHeardToItsLastLine(t *testing.T) {
	// A node that ended on its own lost what it said last, issue #684:
	// what it printed went through a copy that cmd.Wait cut off a moment
	// after the node ended, and the copy waited on the reading. Here the
	// node begins, is handed its sequence, says its last line and ends,
	// all before anything it said is read.
	s := &sequencer{run: []string{"sh", "-c", `printf '\036walking\n'; read -r name; echo "$name said this last"`}}
	defer s.stop(context.Background())
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(s.in, "ends\n"); err != nil {
		t.Fatal(err)
	}
	<-s.ended
	var out bytes.Buffer
	err := s.walk(context.Background(), "ends", &out)
	if err == nil || !strings.Contains(err.Error(), "its node ended before it had walked") {
		t.Errorf("a sequence whose node ended in it gives %v, want that its node ended before it had walked", err)
	}
	if out.String() != "ends said this last\n" {
		t.Errorf("a sequence whose node ended in it printed %q, want what the node said last", out.String())
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
	// is not waited for.
	s := &sequencer{run: []string{"sh", "-c", serve}}
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
	cmd *exec.Cmd
	in  io.WriteCloser
	// lines is what the node prints, a line at a time, closed once
	// everything it printed before it ended has been read.
	lines chan string
	// ended is closed when the node has ended, before what it printed
	// last is read.
	ended chan struct{}
	// last is the sequence the node walked last, which a node that ends
	// between two sequences is told by.
	last string
}

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
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	// The node prints into a pipe of the app's own, not through a copy
	// that cmd.Wait stops: that copy was cut off a moment after the node
	// ended, with what it said last still on its way, whenever the
	// reading was slower than the moment, issue #684.
	said, says, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = says, says
	if err := cmd.Start(); err != nil {
		_, _ = said.Close(), says.Close()
		return err
	}
	lines, ended := make(chan string), make(chan struct{})
	go func() {
		defer said.Close()
		defer close(lines)
		read := bufio.NewReader(said)
		for {
			l, err := read.ReadString('\n')
			l, last := strings.CutSuffix(strings.TrimSuffix(l, "\n"), nodeEndedLine)
			if l != "" || (err == nil && !last) {
				lines <- l
			}
			if last || err != nil {
				return
			}
		}
	}()
	// Once the node has ended, everything it printed is in the pipe, and
	// the line put in after it is read after all of it. What a child of
	// the node still says after that, a browser a stopped walk started,
	// is not waited for: the reading ends at that line, whoever else
	// holds the pipe open.
	go func() {
		_ = cmd.Wait()
		close(ended)
		_, _ = io.WriteString(says, nodeEndedLine+"\n")
		_ = says.Close()
	}()
	s.cmd, s.in, s.lines, s.ended, s.last = cmd, in, lines, ended, ""
	return nil
}

// nodeEndedLine is the line put into a node's output after it ended, see
// start, which ends the reading of what it said.
const nodeEndedLine = "\x1enode ended"

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

// cpuOf is the processor time of a program that ended. On Linux it holds
// the programs it started and waited for too: a bridge's ffmpeg and
// decoders. Not a node's browser's renderers, which Chromium's zygote
// starts, see machineCPU.
func cpuOf(cmd *exec.Cmd) time.Duration {
	if cmd.ProcessState == nil {
		return 0
	}
	return cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()
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

// walkSeeds gives the n seeds each walk takes. Worked out from commit
// when there is one, so the shares of one run agree and the next commit
// walks others, and 1 to n when there is none.
func walkSeeds(commit string, n int) []int {
	seeds := make([]int, n)
	for i := range seeds {
		seeds[i] = i + 1
		if commit != "" {
			h := fnv.New32a()
			fmt.Fprintf(h, "%s %d", commit, i)
			seeds[i] = int(h.Sum32()%999_999) + 1
		}
	}
	return seeds
}

// keptSeed is a seed that found a bug in a walk, walked on every push
// from then on.
type keptSeed struct {
	script string
	seed   int
}

// keptSeeds reads kept-seeds.txt: a line for each seed, the walk's file,
// a space and the seed, and lines that start with # say why. A seed for
// a walk that is not in scripts is refused, so a walk renamed or removed
// does not quietly drop the seeds kept for it.
func keptSeeds(text string, scripts []string) ([]keptSeed, error) {
	var out []keptSeed
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		script, n, ok := strings.Cut(line, " ")
		seed, err := strconv.Atoi(strings.TrimSpace(n))
		if !ok || err != nil || seed < 1 {
			return nil, fmt.Errorf("line %d, %q, is not a walk's file and a seed", i+1, line)
		}
		if !slices.Contains(scripts, script) {
			return nil, fmt.Errorf("line %d keeps a seed for %s, which is no walk", i+1, script)
		}
		out = append(out, keptSeed{script, seed})
	}
	return out, nil
}

// timedAs is the name a run is timed under in walk-times.tsv: a
// sequence by its own name, a walk by its file, whatever its seed, since
// the seeds change with every commit.
func timedAs(run string) string {
	return seedOf.ReplaceAllString(run, "")
}

var seedOf = regexp.MustCompile(` SEED=\d+`)

// timedTook is what the runs took by the name they are timed under, the
// mean of the seeds a walk took.
func timedTook(took map[string]float64) map[string]float64 {
	sum, count := map[string]float64{}, map[string]float64{}
	for n, s := range took {
		sum[timedAs(n)] += s
		count[timedAs(n)]++
	}
	for n := range sum {
		sum[n] /= count[n]
	}
	return sum
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

// The kinds of program whose processor time the walks summary gives.
const (
	machineSpent = "the whole machine, the browsers in it"
	bridgesSpent = "the bridges, Go with ffmpeg and the decoder"
)

// machineCPU is the processor time the machine has been busy since it
// started, from /proc/stat, on Linux only. CI's machine runs the walks
// and nothing else, so what it spent while they ran is what they cost.
//
// On the real disk: /proc is the kernel's.
func machineCPU() (time.Duration, bool) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, false
	}
	return busyOf(string(stat))
}

// busyOf reads the time the machine was busy from the first line of
// /proc/stat, in the kernel's ticks of a hundredth of a second: user,
// nice, system, irq, softirq and steal, but not idle and iowait.
func busyOf(stat string) (time.Duration, bool) {
	line, _, _ := strings.Cut(stat, "\n")
	f := strings.Fields(line)
	if len(f) < 9 || f[0] != "cpu" {
		return 0, false
	}
	var ticks int64
	for _, i := range []int{1, 2, 3, 6, 7, 8} {
		n, err := strconv.ParseInt(f[i], 10, 64)
		if err != nil {
			return 0, false
		}
		ticks += n
	}
	return time.Duration(ticks) * 10 * time.Millisecond, true
}

// walkSummary says where a machine's walks spent their time: how many
// runs its walkers walked, the seconds they took together against the
// seconds the walkers had, the processor time of each kind of program
// in spent, and what every run took, slowest first. Every number stands
// right in its column, so the tables read in a terminal as they do on
// GitHub. The processor time varies less from one run to the next than
// the seconds do, so it is what a change to the speed of the walks is
// compared by. It also says how long walk-times.tsv, kept, says the same
// runs took last time, so a change that makes the walks slower shows in
// the log of its walks job. It only says so and does not fail: how long a
// run takes depends on how busy CI's machine is.
func walkSummary(took, kept, spent map[string]float64, runs, walkers int, wall float64) string {
	sum, before := 0.0, 0.0
	for n, s := range took {
		sum += s
		before += kept[timedAs(n)]
	}
	busy := 0.0
	if wall > 0 && walkers > 0 {
		busy = 100 * sum / (wall * float64(walkers))
	}
	var out strings.Builder
	several := "walkers"
	if walkers == 1 {
		several = "walker"
	}
	fmt.Fprintf(&out, "%d of %d runs on %d %s in %.0f s, %.0f s of walking together, the walkers busy %.0f %% of the time\n", len(took), runs, walkers, several, wall, sum, busy)
	if before > 0 {
		way := "slower"
		if sum < before {
			way = "faster"
		}
		fmt.Fprintf(&out, "walk-times.tsv says these runs took %.0f s last time, so this time they were %.0f %% %s\n", before, math.Abs(100*(sum-before)/before), way)
	}
	table(&out, "CPU (s)", "Programs", spent)
	table(&out, "Seconds", "Run", took)
	return out.String()
}

// table writes rows as a markdown table, the largest number first, under
// the heads number and name. It writes nothing when there are no rows.
func table(out io.Writer, number, name string, rows map[string]float64) {
	type row struct {
		name  string
		value float64
	}
	var all []row
	for n, v := range rows {
		all = append(all, row{n, v})
	}
	if len(all) == 0 {
		return
	}
	slices.SortFunc(all, func(a, b row) int {
		return cmp.Or(cmp.Compare(b.value, a.value), strings.Compare(a.name, b.name))
	})
	width := max(len(number), len(fmt.Sprintf("%.0f", all[0].value)))
	names := len(name)
	for _, r := range all {
		names = max(names, len(r.name))
	}
	fmt.Fprintf(out, "| %*s | %-*s |\n", width, number, names, name)
	fmt.Fprintf(out, "| %s: | %s |\n", strings.Repeat("-", width-1), strings.Repeat("-", names))
	for _, r := range all {
		fmt.Fprintf(out, "| %*.0f | %-*s |\n", width, r.value, names, r.name)
	}
}

func envNumber(name string, otherwise int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return otherwise
}

// The summary of a machine's walks counts what its walkers did against the
// time they had, gives the processor time of each kind of program, and
// lists the slowest runs first, each column lined up.
func TestTheWalksSummarySaysWhereTheTimeWent(t *testing.T) {
	took := map[string]float64{"sequences.mjs SEQUENCE=short": 4.4, "playback.mjs SEED=1": 83, "timeline.mjs SEED=2": 9.6, "words.mjs SEED=1": 12}
	spent := map[string]float64{machineSpent: 151.4, bridgesSpent: 38}
	kept := map[string]float64{"playback.mjs": 60, "timeline.mjs": 10, "words.mjs": 12, "sequences.mjs SEQUENCE=short": 4, "gone.mjs": 50}
	got := walkSummary(took, kept, spent, 5, 2, 60)
	want := "4 of 5 runs on 2 walkers in 60 s, 109 s of walking together, the walkers busy 91 % of the time\n" +
		"walk-times.tsv says these runs took 86 s last time, so this time they were 27 % slower\n" +
		"| CPU (s) | Programs                                    |\n" +
		"| ------: | ------------------------------------------- |\n" +
		"|     151 | the whole machine, the browsers in it       |\n" +
		"|      38 | the bridges, Go with ffmpeg and the decoder |\n" +
		"| Seconds | Run                          |\n" +
		"| ------: | ---------------------------- |\n" +
		"|      83 | playback.mjs SEED=1          |\n" +
		"|      12 | words.mjs SEED=1             |\n" +
		"|      10 | timeline.mjs SEED=2          |\n" +
		"|       4 | sequences.mjs SEQUENCE=short |\n"
	if got != want {
		t.Errorf("the summary is\n%s\nwant\n%s", got, want)
	}
	if got := walkSummary(nil, nil, nil, 3, 2, 0); got != "0 of 3 runs on 2 walkers in 0 s, 0 s of walking together, the walkers busy 0 % of the time\n" {
		t.Errorf("with no runs walked the summary is %q", got)
	}
}

// What one bridge kept is copied into another's home with every path to
// the first home naming the second, in folders and files alike, and the
// video keeps the time it was last changed, which its transcript is
// stamped with.
//
// On the real disk: the bridges of the walks are processes of their own,
// and share what they keep through folders.
func TestAHomeKeptByOneBridgeIsTakenByAnother(t *testing.T) {
	t.Parallel()
	was, home := t.TempDir(), t.TempDir()
	kept := filepath.Join(t.TempDir(), "kept")
	library := `["` + was + `/a.mp4","` + was + `/b.mp4"]`
	if err := os.MkdirAll(filepath.Join(kept, "config", "FrameFairy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "config", "FrameFairy", "library.json"), []byte(library), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyHomed(filepath.Join(kept, "config"), filepath.Join(home, "config"), was, home); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "config", "FrameFairy", "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `["` + home + `/a.mp4","` + home + `/b.mp4"]`; string(got) != want {
		t.Errorf("the library taken is %s, want %s", got, want)
	}

	video := filepath.Join(was, "a.mp4")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	then := time.Date(2026, 11, 6, 9, 30, 0, 123456789, time.UTC)
	if err := os.Chtimes(video, then, then); err != nil {
		t.Fatal(err)
	}
	if err := keepStamp(kept, video); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(home, "a.mp4")
	if err := os.WriteFile(copied, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := takeStamp(kept, copied); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(copied)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(then) {
		t.Errorf("the video copied was last changed %s, want %s as the one kept", info.ModTime(), then)
	}
}

// A program's processor time holds what the programs it started and
// waited for took: the walks summary counts a bridge with its ffmpeg.
func TestTheProcessorTimeOfAProgramHoldsWhatItStarted(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the walks summary counts the programs a program started on Linux")
	}
	// The shell only waits, its child counts to 200000.
	cmd := exec.Command("sh", "-c", `sh -c 'i=0; while [ $i -lt 200000 ]; do i=$((i+1)); done' & wait`)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if cpu := cpuOf(cmd); cpu < 20*time.Millisecond {
		t.Errorf("a shell whose child counted took %s, want the child's counting in it", cpu)
	}
}

// The machine's busy time is read from the first line of /proc/stat, in
// hundredths of a second, without the time it was idle or waited on the
// disk, and nothing else passes for it.
func TestTheMachinesBusyTimeIsReadFromProcStat(t *testing.T) {
	t.Parallel()
	got, ok := busyOf("cpu  100 20 30 5000 400 6 7 8 0 0\ncpu0 50 10 15 2500 200 3 3 4 0 0\n")
	if want := 1710 * time.Millisecond; !ok || got != want {
		t.Errorf("the machine was busy %s, %v, want %s", got, ok, want)
	}
	for _, stat := range []string{"", "cpu0 1 2 3 4 5 6 7 8\n", "cpu  1 2 x 4 5 6 7 8\n", "cpu  1 2 3\n"} {
		if _, ok := busyOf(stat); ok {
			t.Errorf("%q was read as the machine's busy time", stat)
		}
	}
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

// Every commit walks seeds of its own, the same in every share of its
// run, beside the seeds kept because they found a bug, and a walk is
// timed by its file whatever seed it took.
func TestEveryCommitWalksSeedsOfItsOwn(t *testing.T) {
	if got := walkSeeds("", 3); !slices.Equal(got, []int{1, 2, 3}) {
		t.Errorf("with no commit the seeds are %v, not 1 to 3", got)
	}
	one, again, next := walkSeeds("18aeb49", 3), walkSeeds("18aeb49", 3), walkSeeds("91c88dc", 3)
	if !slices.Equal(one, again) {
		t.Errorf("one commit took %v and then %v, so two shares of its run would walk different seeds", one, again)
	}
	for _, s := range one {
		if slices.Contains(next, s) || s < 1 || s > 999_999 {
			t.Errorf("seed %d of %v: the next commit's seeds %v hold it, or it is no seed a walk takes", s, one, next)
		}
	}
	if len(slices.Compact(slices.Sorted(slices.Values(one)))) != 3 {
		t.Errorf("one commit's seeds %v repeat", one)
	}
	kept, err := keptSeeds("# found the hang of #1\nplayback.mjs 328041\n\nwords.mjs 7\n", []string{"playback.mjs", "words.mjs"})
	if err != nil || !slices.Equal(kept, []keptSeed{{"playback.mjs", 328041}, {"words.mjs", 7}}) {
		t.Errorf("kept %v, %v, not the two seeds the file names", kept, err)
	}
	for _, wrong := range []string{"gone.mjs 3", "playback.mjs", "playback.mjs three", "playback.mjs 0"} {
		if _, err := keptSeeds(wrong, []string{"playback.mjs"}); err == nil {
			t.Errorf("kept %q, which names no walk or no seed", wrong)
		}
	}
	took := timedTook(map[string]float64{"playback.mjs SEED=815": 30, "playback.mjs SEED=17": 20, "sequences.mjs SEQUENCE=a short": 5})
	if !maps.Equal(took, map[string]float64{"playback.mjs": 25, "sequences.mjs SEQUENCE=a short": 5}) {
		t.Errorf("timed %v, not a walk by its file with the mean of its seeds", took)
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

// sendDue sends at once the state of every video that is due to be sent,
// and waits for the sends under way, so a mark sent after it comes behind
// all of them: what the Go side had to say when a walk asks is then what
// the interface has heard by the mark, see caughtUp in bridge.mjs.
func sendDue(n *videoNews) {
	if n == nil {
		return
	}
	n.mu.Lock()
	now := map[string]*telling{}
	var going []chan struct{}
	for path, t := range n.due {
		// The send not begun yet is the one asked for last.
		if t.timer != nil && t.timer.Stop() {
			t.timer = nil
			close(t.sends[len(t.sends)-1])
			t.sends = t.sends[:len(t.sends)-1]
			now[path] = t
		}
		going = append(going, t.sends...)
	}
	n.mu.Unlock()
	for path, t := range now {
		n.tell(path, t)
	}
	for _, sent := range going {
		<-sent
	}
}
