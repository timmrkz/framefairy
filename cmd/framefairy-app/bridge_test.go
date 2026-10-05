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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
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
func callHandler(svc *FrameFairy) http.Handler {
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
		method := reflect.ValueOf(svc).MethodByName(name)
		if !method.IsValid() {
			http.Error(w, "the service has no method "+name, http.StatusNotFound)
			return
		}
		kind := method.Type()
		if outside[name] {
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
		if name == "Setup" {
			answer = standInsSetup(out[0].Interface().(SetupState))
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
	"Reveal": true, "ChooseFolder": true, "AddEpisodes": true, "Chrome": true, "StayOpen": true,
}

// standInsSetup is the setup as the bridge's machine has it: the real
// answer, with the two stand-in models counted as there, since they are
// what hears and what finds clips here. Without it the app opens on its
// first-run screen, which the bridge is not about.
func standInsSetup(s SetupState) SetupState {
	s.HasSpeech, s.HasLocalModel, s.HasServer, s.Chosen, s.Ready = true, true, true, true, true
	if len(s.Speech) > 0 {
		s.Speech[0].Installed = true
	}
	for i := range s.Language {
		s.Language[i].Installed = s.Language[i].InUse
	}
	return s
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
// the episode's files the way the app serves them, and the interface
// itself, built into dist.
func bridgeHandler(d *desk, dist string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/call", callHandler(d.svc))
	mux.Handle("/events", d.bus)
	mux.Handle("/", mediaMiddleware(d.svc.store)(http.FileServer(http.Dir(dist))))
	return mux
}

// prose is the stand-in speech model of the bridge. It says sentences
// rather than one word over and over, so a word on screen twice, or one
// in the wrong place, can be told from the words around it. Each sentence
// ends in a pause long enough to end a caption.
type prose struct {
	mu   sync.Mutex
	next int
}

var proseText = strings.Fields(`Ich war vielleicht sechs Jahre alt, als mich auf dem Schulhof
irgendein Typ geschubst hat. Mein Arm ist dabei zersprungen, so hat es sich
jedenfalls angefühlt. Meine Mutter hat mich abgeholt und wir sind sofort ins
Krankenhaus gefahren. Der Arzt hat gelacht und gesagt, das ist nur
verstaucht. Ich habe trotzdem eine Woche lang einen Verband getragen und
allen erzählt, er wäre gebrochen. Das war meine erste richtige Lüge, glaube
ich. Und eigentlich erinnere ich mich an die Lüge besser als an den Arm.`)

func (p *prose) Recognize(samples []float32, rate int) []engine.Token {
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
	d, path := openBridge(t)
	server := &http.Server{Addr: addr, Handler: bridgeHandler(d, bridgeDist(t))}
	fmt.Printf("bridge on http://%s with %s\n", addr, path)
	if err := server.ListenAndServe(); err != nil {
		t.Fatal(err)
	}
}

// openBridge opens a desk whose Go side tells a browser what it does, with
// one episode in it, heard and searched, so the workspace has a clip.
func openBridge(t *testing.T) (*desk, string) {
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
	path := filepath.Join(d.home, "erste-erinnerung.mp4")
	// VP9 and Opus rather than the path tests' MPEG-4 and AAC, because the
	// Chromium a walk drives is built without the proprietary codecs and
	// would not play the episode at all. A grey that grows lighter, so a
	// frame shows where in the episode it is.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=320x180:r=5:d=120,geq=lum='30+180*T/120':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=220:sample_rate=48000:d=120",
		"-shortest", "-c:v", "libvpx-vp9", "-b:v", "40k", "-deadline", "realtime", "-cpu-used", "8",
		"-g", "10", "-pix_fmt", "yuv420p", "-c:a", "libopus", "-b:a", "24k", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the episode: %s %s", err, out)
	}
	if added, err := d.svc.addEpisodes([]string{path}); err != nil || len(added) != 1 {
		t.Fatalf("adding %s: %v", path, err)
	}
	d.idle(path)
	if len(d.clips(path)) == 0 {
		t.Fatalf("the episode has no clip to work on: %+v", d.svc.jobs.list())
	}
	return d, path
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
