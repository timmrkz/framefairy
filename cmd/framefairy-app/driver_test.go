package main

// The driver for the path tests in paths_test.go.
//
// A path test is what a person does, in the words of the app: add a video,
// press New, press Cancel, close the app and open it again, press
// Continue, press Render. This file is the one place that knows which calls
// those are, so the way work is run underneath can change and the path
// tests stay as they are. See docs/JOBS.md.
//
// Everything but the two models is real: the queue, the engine, ffmpeg and
// the files in the work folder. The speech model is a stand-in that hears a
// word every 0.6 seconds, and the language model is a stand-in server that
// answers like llama-server.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"framefairy/engine"
	"framefairy/internal/ffmpegtest"
)

// desk is the app as a person uses it, over one library.
type desk struct {
	t    *testing.T
	home string
	svc  *FrameFairy
	// speech and model are the two stand-ins, shared by every opening of
	// the app, the way the models on disk would be.
	speech *speech
	model  *model
	// bus carries what the Go side tells the interface, when a browser
	// is listening, see bridge_test.go.
	bus *bus
}

// open starts the app over a new, empty library.
func open(t *testing.T) *desk {
	t.Helper()
	ffmpegtest.Need(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	d := &desk{t: t, home: home, speech: &speech{}, model: newModel(t)}
	st := openStore()
	if err := st.UpdateSettings(func(s *Settings) {
		s.ASRModel = filepath.Join(home, "speech-model")
		s.OutputDir = filepath.Join(home, "shorts")
		s.Planner = "local"
	}); err != nil {
		t.Fatal(err)
	}
	fakes := func(e *engine.Engine, o *engine.Options) {
		e.OpenRecognizer = d.speech.open
		o.LLMURL = d.model.server.URL
		o.Width, o.Height = 360, 640
		o.Preset = "ultrafast"
	}
	standIns.Store(&fakes)
	t.Cleanup(func() { standIns.Store(nil) })
	d.start(st)
	return d
}

// start runs the app over a store.
func (d *desk) start(st *store) {
	d.svc = &FrameFairy{store: st}
	d.svc.jobs = newQueue(st, func(u JobUpdate) { d.bus.send("job", u) },
		func(episode string) { d.bus.send("episode", episode) })
	if d.bus != nil {
		d.svc.levels = newMeasuring(func(episode string) { d.bus.send("levels", episode) })
	}
	svc := d.svc
	// Whatever is still running when the test ends is stopped before its
	// folder goes.
	d.t.Cleanup(func() { svc.jobs.shutDown() })
}

// episode puts a video of this many seconds in the library's folder, with
// a tone for its sound. Each video is made once for all the tests and
// copied, because making one takes seconds. Videos of different names have
// different tones, so they are different episodes.
func (d *desk) episode(name string, seconds string) string {
	d.t.Helper()
	made, err := videos.make(name, seconds)
	if err != nil {
		d.t.Fatal(err)
	}
	body, err := os.ReadFile(made)
	if err != nil {
		d.t.Fatal(err)
	}
	path := filepath.Join(d.home, name+".mp4")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		d.t.Fatal(err)
	}
	return path
}

// videos are the videos made for this run of the tests, in a folder of
// their own that TestMain takes away at the end.
var videos = &made{files: map[string]string{}}

type made struct {
	mu    sync.Mutex
	dir   string
	files map[string]string
	tone  int
}

func (m *made) make(name, seconds string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name + "-" + seconds
	if path, ok := m.files[key]; ok {
		return path, nil
	}
	if m.dir == "" {
		dir, err := os.MkdirTemp("", "framefairy-videos-")
		if err != nil {
			return "", err
		}
		m.dir = dir
	}
	m.tone++
	path := filepath.Join(m.dir, key+".mp4")
	// Five frames a second of grey and a 16 kHz tone: what the speech
	// model hears and what a render cuts, and nothing that takes long to
	// make.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=gray:s=320x180:r=5:d="+seconds,
		"-f", "lavfi", "-i", "sine=f="+strconv.Itoa(200+20*m.tone)+":sample_rate=16000:d="+seconds,
		"-shortest", "-c:v", "mpeg4", "-g", "50", "-c:a", "aac", "-b:a", "32k", path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("making the video: %s %s", err, out)
	}
	m.files[key] = path
	return path, nil
}

// updaterHelper is what Wails' updater sets in the copy of the app's own
// program it starts on a restart, see updating.Restart.
const updaterHelper = "WAILS_UPDATER_HELPER"

func TestMain(m *testing.M) {
	// A restart hands over to a copy of the app's own program started as
	// the updater's helper. The app answers that in application.New, but
	// in a test the program is the test binary, which ran every test
	// again, renders included, and could start copies of its own: a
	// binary kept after the run started them without end. A copy started
	// as the helper leaves at once, before any test.
	if os.Getenv(updaterHelper) == "1" {
		os.Exit(0)
	}
	toolsFromThePath()
	// The face detector finds no face in a grey test video and framing
	// falls back to what is in focus, after some 50 milliseconds a frame of
	// looking. The engine's TestProjectSteps frames with it.
	os.Setenv("FRAMEFAIRY_NO_FACES", "1")
	code := m.Run()
	if videos.dir != "" {
		_ = os.RemoveAll(videos.dir)
	}
	os.Exit(code)
}

// firstWindow is the window of an episode's first search: the first half
// hour, which for the short episodes of these tests is all of it.
var firstWindow = window{0, 0}

type window struct{ from, to float64 }

// numbers is what the tests ask every search for.
func (w window) request() engine.PlanRequest {
	return engine.PlanRequest{From: w.from, To: w.to, Count: 1, Min: 5, Max: 30}
}

// add adds a video to the library, the way the Add button does. The app
// starts its first search by itself.
func (d *desk) add(name, seconds string) string {
	d.t.Helper()
	path := d.episode(name, seconds)
	if added, err := d.svc.addEpisodes([]string{path}); err != nil || len(added) != 1 {
		d.t.Fatalf("adding %s: %v", path, err)
	}
	return path
}

// search presses New on a window.
func (d *desk) search(path string, w window) {
	d.svc.Search(path, w.request())
}

// carryOn presses Continue on the search the episode says was cut off or
// failed.
func (d *desk) carryOn(path string) {
	d.t.Helper()
	o := d.outcome(path)
	if !o.interrupted && !o.stopped && o.failed == "" {
		d.t.Fatalf("Continue on a search that did not stop: %+v", o)
	}
	j, _ := d.svc.jobs.find(path, "search")
	d.svc.Continue(j.ID)
}

// cancel presses Cancel on the episode's search.
func (d *desk) cancel(path string) {
	d.t.Helper()
	j, ok := d.svc.jobs.find(path, "search")
	if !ok {
		d.t.Fatal("no search to call off")
	}
	d.svc.CancelJob(j.ID)
}

// in presses I with the playhead at a moment, and out presses O.
func (d *desk) in(path string, at float64) Job  { return d.svc.MakeClip(path, at, false) }
func (d *desk) out(path string, at float64) Job { return d.svc.MakeClip(path, at, true) }

// render presses Render on every clip of a plan.
func (d *desk) render(path, plan string) {
	d.svc.Render(path, engine.RenderRequest{Plan: plan})
}

// close quits the app, the way the window's close button does.
func (d *desk) close() {
	d.t.Helper()
	if !d.svc.jobs.shutDown() {
		d.t.Fatal("the app did not stop its work in time")
	}
}

// reopen starts the app again over the same library, the way main does.
func (d *desk) reopen() {
	st := openStore()
	d.start(st)
	d.svc.jobs.restore(st.Episodes())
}

// outcome is what the app says about an episode's last search where its
// clips would be: nothing, interrupted or stopped with Continue, or failed
// with the reason.
type outcome struct {
	interrupted bool
	// stopped is a search called off with Cancel, which says so the same
	// way, with Continue.
	stopped bool
	failed  string
	window  window
}

func (d *desk) outcome(path string) outcome {
	j, ok := d.svc.jobs.find(path, "search")
	if !ok {
		return outcome{}
	}
	w := window{j.From, j.To}
	switch j.State {
	case JobInterrupted:
		if j.Step == engine.StepStopped {
			return outcome{stopped: true, window: w}
		}
		return outcome{interrupted: true, window: w}
	case JobFailed:
		return outcome{failed: j.Error, window: w}
	}
	return outcome{}
}

// idle waits until nothing of the episode is queued or running.
func (d *desk) idle(path string) {
	d.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		busy := false
		for _, j := range d.svc.jobs.list() {
			if j.Episode == path && (j.State == JobQueued || j.State == JobRunning) {
				busy = true
			}
		}
		if !busy {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("the work on %s never ended: %+v", filepath.Base(path), d.svc.jobs.list())
}

// clips are the clips in the episode's list.
func (d *desk) clips(path string) []ClipEntry {
	d.t.Helper()
	clips, err := d.svc.Clips(context.Background(), path)
	if err != nil {
		d.t.Fatal(err)
	}
	return clips
}

// heard is how far the episode's transcript reaches, and whether it is all
// of it.
func (d *desk) heard(path string) (float64, bool) {
	return engine.Coverage(path, d.svc.store.Settings().ASRModel)
}

// shorts are the finished shorts of the library, each where its path
// really leads, the way the engine names a short. On macOS the temp folder
// is under /var, which is a link to /private/var.
func (d *desk) shorts() []string {
	var found []string
	_ = filepath.WalkDir(d.svc.store.Settings().OutputDir, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(p, ".mp4") {
			found = append(found, engine.ResolvePath(p))
		}
		return nil
	})
	return found
}

// failedRender is the reason the last render of the episode failed, or "".
func (d *desk) failedRender(path string) string {
	if j, ok := d.svc.jobs.find(path, "render"); ok && j.State == JobFailed {
		return j.Error
	}
	return ""
}

// speech is the stand-in speech model. It hears a word every 0.6 seconds,
// takes a moment over every chunk so a test can close the app while it
// hears, and counts the seconds of audio it was given, so a test can tell
// whether anything was heard twice.
type speech struct {
	mu      sync.Mutex
	broken  error
	started chan struct{}
	samples atomic.Int64
}

// breaks makes the speech model fail to load, the way a missing or damaged
// model does.
func (s *speech) breaks(err error) {
	s.mu.Lock()
	s.broken = err
	s.mu.Unlock()
}

// hearing gives a channel that is closed when the speech model is next
// given audio.
func (s *speech) hearing() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = make(chan struct{})
	return s.started
}

func (s *speech) open(string) (engine.Recognizer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken != nil {
		return nil, s.broken
	}
	return s, nil
}

// seconds is how much audio the speech model has been given in all.
func (s *speech) seconds() float64 {
	return float64(s.samples.Load()) / engine.SampleRate
}

func (s *speech) Recognize(samples []float32, rate int) []engine.Token {
	s.samples.Add(int64(len(samples)))
	s.mu.Lock()
	if s.started != nil {
		close(s.started)
		s.started = nil
	}
	s.mu.Unlock()
	time.Sleep(40 * time.Millisecond)
	length := float64(len(samples)) / float64(rate)
	var tokens []engine.Token
	for at := 0.1; at+0.35 < length; at += 0.6 {
		tokens = append(tokens, engine.Token{Text: " wort", Start: at, Duration: 0.3})
	}
	return tokens
}

func (*speech) Close() {}

// model is the stand-in language model, a server that answers like
// llama-server with one clip made of the first line. It can be told to
// hang until it is let go of, or to fail.
type model struct {
	server *httptest.Server
	mu     sync.Mutex
	hang   bool
	fail   bool
	asked  chan struct{}
	calls  atomic.Int32
	// done lets go of every answer held, when the test ends.
	done chan struct{}
}

func newModel(t *testing.T) *model {
	m := &model{done: make(chan struct{})}
	m.server = httptest.NewServer(http.HandlerFunc(m.answer))
	t.Cleanup(m.server.Close)
	t.Cleanup(func() { close(m.done) })
	return m
}

// hangs makes the model hold every answer until the search is stopped.
func (m *model) hangs(on bool) {
	m.mu.Lock()
	m.hang = on
	m.mu.Unlock()
}

// fails makes the model answer with an error.
func (m *model) fails(on bool) {
	m.mu.Lock()
	m.fail = on
	m.mu.Unlock()
}

// asking gives a channel that is closed when the model is next asked.
func (m *model) asking() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.asked = make(chan struct{})
	return m.asked
}

func (m *model) answer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusOK)
		return
	}
	m.calls.Add(1)
	m.mu.Lock()
	hang, fail := m.hang, m.fail
	if m.asked != nil {
		close(m.asked)
		m.asked = nil
	}
	m.mu.Unlock()
	if fail {
		http.Error(w, "the model fell over", http.StatusInternalServerError)
		return
	}
	if hang {
		select {
		case <-r.Context().Done():
		case <-m.done:
		}
		return
	}
	// One story, the first line, as middle answers: where it starts, a
	// line inside it and where it ends.
	stream(w, "1 1 1\n")
}

// stream sends an answer the way llama-server streams one.
func stream(w http.ResponseWriter, answer string) {
	w.Header().Set("content-type", "text/event-stream")
	send := func(body string) {
		_, _ = w.Write([]byte("data: " + body + "\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	quoted, _ := json.Marshal(answer)
	send(`{"choices":[{"delta":{"content":` + string(quoted) + `},"finish_reason":null}]}`)
	send(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	send(`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":40}}`)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

// within waits for a channel, or fails the test.
func within(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(30 * time.Second):
		t.Fatalf("%s never happened", what)
	}
}

var errBroken = errors.New("the speech model could not be loaded")

// toolsFromThePath names the ffmpeg, ffprobe and llama-server on the search
// path in the environment, where the programs take a tool from by choice.
// A program never falls back to the search path by itself, see
// engine.FindTool, and a test binary has nothing beside it. One already
// named is left as it is.
func toolsFromThePath() {
	for env, name := range map[string]string{
		"FRAMEFAIRY_FFMPEG":       "ffmpeg",
		"FRAMEFAIRY_FFPROBE":      "ffprobe",
		"FRAMEFAIRY_LLAMA_SERVER": "llama-server",
	} {
		if os.Getenv(env) != "" {
			continue
		}
		if found, err := exec.LookPath(name); err == nil {
			_ = os.Setenv(env, found)
		}
	}
}
