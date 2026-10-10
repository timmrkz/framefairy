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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

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
		select {
		case c <- body:
		default:
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
// says how long a call of a name waits before it reaches the service, see
// /hold.
func callHandler(svc func() *FrameFairy, own map[string]func() (any, error),
	held func(name string) time.Duration) http.Handler {
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
		if wait := held(name); wait > 0 {
			time.Sleep(wait)
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
		if outside[name] && !(walkedUpdates[name] && svc().updates != nil) {
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
				answer = standInsSetup(out[0].Interface().(SetupState))
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
}

// walkedUpdates are the calls of the Updates page a walk can have answered
// by the app's own updating, once it has asked for one, see /updates.
// Until then they are outside like the rest. Relaunch is outside always.
var walkedUpdates = map[string]bool{"Updates": true, "FollowChannel": true, "CheckForUpdates": true}

// standInsSetup is the setup as the bridge's machine has it: the real
// answer, with the two stand-in models counted as there, since they are
// what hears and what finds clips here. Without it the app opens on its
// first-run screen, which the bridge is not about.
func standInsSetup(s SetupState) SetupState {
	s.HasSpeech, s.HasLocalModel, s.HasServer, s.Chosen, s.Ready = true, true, true, true, true
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
				return nil, nil
			}
			return b.d.svc.addEpisodes(picked)
		},
	}
	mux.Handle("/call", callHandler(svc, own, b.held))
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
	mux.HandleFunc("/pick", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		path, err := b.pick(q.Get("seconds"), q.Get("rate"), q.Get("switch"), q.Get("timing"), q.Get("codec") == "hevc",
			q.Get("namesake") == "1")
		answer(w, path, err)
	})
	// A file of sound alone, named like a video, which Add has to leave
	// out: the episode's decoder finds no picture in it.
	mux.HandleFunc("/pick-sound", func(w http.ResponseWriter, r *http.Request) {
		path, err := b.pickSound()
		answer(w, path, err)
	})
	mux.HandleFunc("/model", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		b.d.model.hangs(q.Get("hang") == "1")
		b.d.model.fails(q.Get("fail") == "1")
		answer(w, nil, nil)
	})
	mux.HandleFunc("/speech", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ms, _ := strconv.Atoi(q.Get("ms"))
		b.words.pace.Store(int64(min(max(ms, 0), 5000)))
		if from, err := strconv.Atoi(q.Get("from")); err == nil && from >= 0 {
			b.words.from(from)
		}
		answer(w, nil, nil)
	})
	mux.HandleFunc("/hold", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ms, _ := strconv.Atoi(q.Get("ms"))
		b.mu.Lock()
		b.holds[q.Get("call")] = time.Duration(min(max(ms, 0), 10000)) * time.Millisecond
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
		answer(w, nil, b.updatesAs(q.Get("from"), q.Get("stored"), list, conflict))
	})
	mux.HandleFunc("/reopen", func(w http.ResponseWriter, r *http.Request) {
		closed, err := b.reopen()
		answer(w, closed, err)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaMiddleware(b.d.svc.store)(http.FileServer(http.Dir(dist))).ServeHTTP(w, r)
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
	// pace is how long it takes over each piece of audio it hears, in
	// milliseconds, so a walk can see a transcript grow, and cancel it.
	pace atomic.Int64
}

var proseText = strings.Fields(`Ich war vielleicht sechs Jahre alt, als mich auf dem Schulhof
irgendein Typ geschubst hat. Mein Arm ist dabei zersprungen, so hat es sich
jedenfalls angefühlt. Meine Mutter hat mich abgeholt und wir sind sofort ins
Krankenhaus gefahren. Der Arzt hat gelacht und gesagt, das ist nur
verstaucht. Ich habe trotzdem eine Woche lang einen Verband getragen und
allen erzählt, er wäre gebrochen. Das war meine erste richtige Lüge, glaube
ich. Und eigentlich erinnere ich mich an die Lüge besser als an den Arm.`)

func (p *prose) Recognize(samples []float32, rate int) []engine.Token {
	time.Sleep(time.Duration(p.pace.Load()) * time.Millisecond)
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
	// holds is how long the next call of a name waits before it reaches
	// the service, the way a busy machine delivers it late, see /hold.
	holds map[string]time.Duration
	// channels serves the channel list and the builds the Updates page is
	// walked with, see /updates.
	channels *channelServer
}

// updatesAs gives the app updates the way a walk asks for them: a build
// from the channel from, or one made on the Mac when from is empty, with
// stored in updates.json, picked by a run before, and these channels on
// the list, those in conflict said to no longer merge into main. It is
// the app's own updating and Wails' updater, reading a
// channel list served on this machine, the one the Go tests of updates
// use, so nothing reaches the network. Every channel has a build newer
// than the one running, which downloads in a moment. The app opened again
// has none, the way the bridge started.
func (b *bridge) updatesAs(from, stored string, list, conflict []string) error {
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
		busy: b.d.svc.jobs.busy,
		emit: func(s UpdateState) { b.d.bus.send("updates", s) },
		load: func() string { return stored },
		save: func(string) error { return nil },
	}
	src := &updates.Source{URL: cs.srv.URL + "/channels.json", Client: cs.srv.Client(), Cache: b.t.TempDir()}
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

// held is how long a call of this name waits before it reaches the
// service. A hold is for the next call of the name only.
func (b *bridge) held(name string) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	wait := b.holds[name]
	delete(b.holds, name)
	return wait
}

// openBridge opens a desk whose Go side tells a browser what it does, with
// one episode in it, heard and searched, so the workspace has a clip.
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
	// The desk is started again with a bus, before anything is in it.
	d.svc.jobs.shutDown()
	d.bus = newBus()
	d.start(d.svc.store)
	b := &bridge{t: t, d: d, words: words, first: filepath.Join(d.home, "erste-erinnerung.mp4"), kept: t.TempDir(),
		holds: map[string]time.Duration{}}
	if err := makeEpisode(b.first, 120, 0, false); err != nil {
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

// makeEpisode makes a video of this many seconds. VP9 and Opus rather than
// the path tests' MPEG-4 and AAC, because the Chromium a walk drives is
// built without the proprietary codecs and would not play it at all. The
// picture says which frame it is, the recipe of the harness's own episodes
// in frontend/preview/open.mjs: a strip along its top, 12 pixels high, is
// the frame number in ten bars of 32 pixels, the highest bit on the left,
// light for one and dark for nought, so a walk reads back from the video
// preview's canvas the frame on screen, see recordFrames in
// walks/bridge.mjs. Below it is a grey that grows lighter. The strip is
// thin so that no two frames differ by a camera switch, which a half of
// bars did every 16 frames, and the search then split its clip into
// pieces at every one. The bars are worked out on a picture one pixel a
// bit and made large without smoothing.
//
// With a switch, at a moment on a frame, the episode is two cameras
// instead, see twoCameras. With hevc its picture is HEVC with 10-bit
// colour, which WebKit says it decodes and then fails on, and which the
// Chromium a walk drives cannot decode at all: the Go side decodes it, see
// frames.go. Its GOPs are open, the way x265 writes them and DaVinci
// Resolve wrote Tim's start.mp4: a key frame every twelve frames, the last
// of a run of four, so the three frames shown before it are decoded after
// it. With a key frame every ten, every run of four ended on the frame
// before a key frame, and no frame led up to one. Plan row 2.186.
func makeEpisode(path string, seconds int, switchAt float64, hevc bool) error {
	d := strconv.Itoa(seconds)
	below, sound := "[1]geq=lum='30+180*T/"+d+"':cb=128:cr=128,scale=320:168:flags=neighbor[g]", "220"
	size, rate, heard := "2x2", "100k", []string{"libopus", "-b:a", "24k"}
	if switchAt > 0 {
		// A chequerboard takes more to keep sharp than a grey. The sound
		// is plain samples, chosen when a render read each piece's sound
		// from where the piece starts: Opus needs 80 ms decoded ahead of
		// a seek, so every piece started a little quiet, and a walk
		// listening for a dip heard that. A render now reads the sound a
		// fifth of a second early, soundLead, which
		// TestEveryPieceIsHeardFromItsFirstMoment in engine/preroll_test.go
		// holds, so Opus would do here too. It has not been changed back.
		below, sound, size, rate = twoCameras(int(math.Round(switchAt*5))), "440", "320x168", "300k"
		heard = []string{"pcm_s16le"}
	}
	args := []string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=10x2:r=5:d=" + d,
		"-f", "lavfi", "-i", "color=c=black:s=" + size + ":r=5:d=" + d,
		"-f", "lavfi", "-i", "sine=f=" + sound + ":sample_rate=48000:d=" + d,
		"-filter_complex",
		"[0]geq=lum='if(bitand(N\\,pow(2\\,9-X))\\,220\\,30)':cb=128:cr=128,scale=320:12:flags=neighbor[b];" +
			below + ";[b][g]vstack,format=yuv420p[v]",
		"-map", "[v]", "-map", "2",
		"-shortest"}
	if hevc {
		args = append(args, "-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1",
			"-x265-params", "bframes=3:keyint=12:log-level=error")
	} else {
		args = append(args, "-c:v", "libvpx-vp9", "-b:v", rate, "-deadline", "realtime", "-cpu-used", "8", "-g", "10")
	}
	args = append(args, "-c:a")
	out, err := exec.Command("ffmpeg", append(append(args, heard...), path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// twoCameras is the picture below the strip of an episode filmed by two
// cameras, the first up to the frame it switches on and the second from
// it. Each looks at a subject, a chequerboard, on its own side of the
// picture, the first on the left and the second on the right, so the
// search frames each shot on its own subject and the two crops differ.
// Around the subject is a plain backdrop, dark grey for the first camera
// and a light red for the second, so a short tells the shots apart by
// their colour, and a shot framed by the other's crop shows only its
// backdrop. The switch changes the whole picture at once, which is what
// the search reads as a camera switch.
func twoCameras(frame int) string {
	n := strconv.Itoa(frame)
	checks := "if(mod(floor(X/4)+floor(Y/4)\\,2)\\,235\\,20)"
	return "[1]geq=lum='if(lt(N\\," + n + ")\\," +
		"if(between(X\\,20\\,99)\\," + checks + "\\,60)\\," +
		"if(between(X\\,220\\,299)\\," + checks + "\\,170))'" +
		":cb=128:cr='if(lt(N\\," + n + ")\\,128\\,180)'[g]"
}

// filmedBits is how many bits the frame number of a filmed episode has, so
// it says the number of every frame up to 2048, more than a minute at
// 29.97 fps. Its bands are bandHeight pixels high each, of a picture 180
// high. The walks read them back with the same numbers, see shortFrames in
// walks/bridge.mjs.
const filmedBits, bandHeight = 11, 4

// makeFilmedEpisode makes a video of this many seconds at the frame rate a
// camera records at, rate as ffmpeg takes it, 30000/1001 for 29.97 fps, so
// a walk can render a clip of it and read the short back. Its picture says
// which frame it is in a way the render keeps: a short is a narrow part of
// the picture made larger, so bars side by side would be cut away. The
// frame number is bands one above the other across the whole width, the
// highest bit at the top, each bandHeight pixels high, light for one and
// dark for nought, and whatever part of the width the crop keeps holds
// every band. They take 44 of the picture's 180 pixels, a quarter, and
// light and dark are only 100 apart, so no two frames differ by a camera
// switch, see makeEpisode: bands of 220 and 30 made six of them in 45
// seconds. They are worked out two pixels a bit, since a picture is an even
// number of pixels high. Below them is the grey that grows lighter. The
// sound is a steady tone, so padded silence at a cut can be heard.
//
// The frames come at the rate, unless timing says otherwise: "uneven" is
// a phone's frames, each 0, 8 or 16 ms late in ticks of 1/600 s, and
// "late" is a picture that starts a quarter of a second after the sound,
// between two of its frames, by an edit list. Either way each frame keeps
// its number.
func makeFilmedEpisode(path string, seconds int, rate, timing string) error {
	d := strconv.Itoa(seconds)
	bands := filmedBits * bandHeight
	shift, keep := "", []string{}
	switch timing {
	case "uneven":
		shift = fmt.Sprintf(",settb=1/600,setpts='(N/(%s)+0.008*mod(N\\,3))/TB'", rate)
		keep = []string{"-fps_mode", "passthrough", "-enc_time_base", "1/600", "-video_track_timescale", "600"}
	case "late":
		// On a clock of 1/90000 s, so the picture starts a true quarter
		// second in, 7.49 frames at 29.97 a second. On the rate's own
		// clock the quarter second came out a whole 7 frames, 0.233 s,
		// on the grid of the file's start, where frames counted from the
		// file and from the picture are the same frames and a mistake
		// between the two could not be seen.
		shift = ",settb=1/90000,setpts=PTS+0.25/TB"
		keep = []string{"-fps_mode", "passthrough", "-enc_time_base", "1/90000", "-video_track_timescale", "90000"}
	}
	args := []string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=2x%d:r=%s:d=%s", 2*filmedBits, rate, d),
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=2x2:r=%s:d=%s", rate, d),
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=" + d,
		"-filter_complex",
		fmt.Sprintf("[0]geq=lum='if(bitand(N\\,pow(2\\,%d-floor(Y/2)))\\,160\\,60)':cb=128:cr=128,scale=320:%d:flags=neighbor[b];", filmedBits-1, bands) +
			fmt.Sprintf("[1]geq=lum='30+180*T/%s':cb=128:cr=128,scale=320:%d:flags=neighbor[g];", d, 180-bands) +
			"[b][g]vstack,format=yuv420p" + shift + "[v]",
		"-map", "[v]", "-map", "2"}
	args = append(args, keep...)
	out, err := exec.Command("ffmpeg", append(args,
		"-shortest", "-c:v", "libvpx-vp9", "-crf", "30", "-b:v", "0", "-deadline", "realtime", "-cpu-used", "8",
		"-g", "15", "-c:a", "libopus", "-b:a", "64k", path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// frameRate is a frame rate as ffmpeg takes it, a whole number or one
// whole number over another, and nothing else, since it goes into a
// filter.
var frameRate = regexp.MustCompile(`^[1-9][0-9]{0,5}(/[1-9][0-9]{0,4})?$`)

// pick makes a new video, not in the library, and has the Add button's box
// hand it over next. Without a rate it is the bridge's own kind of
// episode, at five frames a second, and with one it is filmed at that
// rate, see makeFilmedEpisode. With a switch, in seconds, the bridge's
// own kind is filmed by two cameras that switch then, see twoCameras, and
// with hevc it is HEVC with 10-bit colour.
func (b *bridge) pick(seconds, rate, switchAt, timing string, hevc, namesake bool) (string, error) {
	n, err := strconv.Atoi(seconds)
	if err != nil || n < 10 || n > 3600 {
		n = 90
	}
	if rate != "" && !frameRate.MatchString(rate) {
		return "", fmt.Errorf("%q is no frame rate", rate)
	}
	if timing != "" && timing != "uneven" && timing != "late" {
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
	if rate != "" {
		err = makeFilmedEpisode(path, n, rate, timing)
	} else {
		err = makeEpisode(path, n, at, hevc)
	}
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	b.picks = append(b.picks, path)
	b.mu.Unlock()
	return path, nil
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
func (b *bridge) reopen() ([]Job, error) {
	if !b.d.svc.jobs.shutDown() {
		return nil, fmt.Errorf("the app did not stop its work in time")
	}
	closed := b.d.svc.jobs.list()
	b.d.svc.levels.shutDown()
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
func (b *bridge) reset() error {
	b.words.pace.Store(0)
	b.d.model.hangs(false)
	b.d.model.fails(false)
	b.mu.Lock()
	b.picks = nil
	clear(b.holds)
	b.mu.Unlock()
	if !b.d.svc.jobs.shutDown() {
		return fmt.Errorf("the app did not stop its work in time")
	}
	b.d.svc.levels.shutDown()
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
	b.d.reopen()
	return nil
}

// bridgeDist is where the interface built for the bridge is.
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
// They run side by side, as many at once as the machine has cores, or
// WALKERS. Each runs against a bridge of its own, started as a process of
// its own: a desk sets HOME for the whole process it is in, so two bridges
// in one would share the app's settings. One after another they took nine
// minutes.
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
	}
	runs := []walkRun{{script: "sequences.mjs"}}
	walksWanted := envNumber("WALKS", 3)
	// Each walk takes its own number of steps unless STEPS says.
	var steps []string
	if n := envNumber("STEPS", 0); n > 0 {
		steps = append(steps, fmt.Sprintf("STEPS=%d", n))
	}
	// Every walk in the folder, each for the same seeds.
	for seed := 1; seed <= walksWanted; seed++ {
		for _, script := range walkScripts(t, walks) {
			runs = append(runs, walkRun{script, append([]string{fmt.Sprintf("SEED=%d", seed)}, steps...)})
		}
		// Playback again on an episode whose picture the Go side decodes,
		// HEVC in 10-bit colour, which Chromium cannot.
		runs = append(runs, walkRun{"playback.mjs", append([]string{fmt.Sprintf("SEED=%d", seed), "HEVC=1"}, steps...)})
	}

	walkers := min(envNumber("WALKERS", runtime.NumCPU()), len(runs))
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
			url, err := startBridge(t)
			if err != nil {
				t.Error(err)
				return
			}
			for r := range todo {
				began := time.Now()
				cmd := exec.Command(node, filepath.Join(walks, r.script))
				cmd.Dir = walks
				cmd.Env = append(append(os.Environ(), "BRIDGE_URL="+url), r.env...)
				out, err := cmd.CombinedOutput()
				what := fmt.Sprintf("%s (%.0f s)", strings.TrimSpace(r.script+" "+strings.Join(r.env, " ")), time.Since(began).Seconds())
				if err != nil {
					t.Errorf("%s:\n%s", what, out)
					continue
				}
				t.Logf("%s:\n%s", what, out)
			}
		})
	}
	wg.Wait()
	if left := len(todo); left > 0 {
		t.Errorf("%d runs were not walked, since no bridge started", left)
	}
}

// startBridge starts TestBridge in a process of its own, on a free port,
// and gives back where it answers. It is stopped when the test ends.
func startBridge(t *testing.T) (string, error) {
	cmd := exec.Command(os.Args[0], "-test.run", "^TestBridge$", "-test.timeout", "0")
	cmd.Env = append(os.Environ(), "FRAMEFAIRY_BRIDGE=127.0.0.1:0", "FRAMEFAIRY_BRIDGE_WALKER=1", "FRAMEFAIRY_WALKS=")
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

// walkScripts are the walks in the folder: every script that walks with
// walk.mjs, so a new walk runs without being named anywhere else.
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

func envNumber(name string, otherwise int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return otherwise
}
