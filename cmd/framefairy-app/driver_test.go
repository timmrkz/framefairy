package main

// The driver for the path tests in paths_test.go.
//
// A path test is what a person does, in the words of the app: add a video,
// press Search, press Cancel, close the app and open it again, press
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
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"framefairy/engine"
	"framefairy/engine/fileio"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
	"framefairy/internal/hangdump"
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
	// still says when a recording nobody talks into has been quiet long
	// enough, when set, see listening.still, and most is the longest a
	// recording is, in samples, in every opening of the app, 0 for as
	// long as in the app.
	still chan struct{}
	most  int
	// beats, when set, are the clocks of every recording, see
	// listening.beats, and told hears each Dictation the app tells the
	// window, so the bridge knows what a step of a recording told.
	beats *listenBeats
	// fakes readies every engine of every opening of the app with the
	// stand-ins, see store.standIns, and fit hands every opening what
	// the bridge stands in for, the microphone, the report's folder and
	// the downloads of the models, when set.
	fakes func(*engine.Engine, *engine.Options)
	fit   func(*FrameFairy)
	told  func(Dictation)
	// time, when set, is the machine's clock in every opening of the
	// app, see engine.Clock.
	time *walkTime
	// The report windows Bugs made and brought forward, the window the
	// app would have made, and the pictures its open box hands over
	// next, see bugs.go.
	reportMade, reportFronted atomic.Int32
	pictureMu                 sync.Mutex
	pictures                  []string
	// clipboard is what the app last put on the clipboard.
	clipboard atomic.Pointer[string]
	// exported is where the app last saved a file through the system's
	// save box, which the desk stands in for, see start.
	exported atomic.Pointer[string]
	// setup, when set, is how the desk's machine shows the setup, the
	// way every call that answers with it shows it, see standInsSetup.
	// What the app sends by itself when the models change is shown the
	// same way.
	setup func(SetupState) SetupState
}

// open starts the app over a new, empty library. Adding a video asks the
// episode's decoder for its first frame, so the desk needs it.
//
// On the real disk: ffmpeg and the episode's decoder read the videos added to this library.
func open(t *testing.T) *desk {
	t.Helper()
	ffmpegtest.Need(t)
	needDecoder(t)
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
	d.fakes = fakes
	st.standIns = fakes
	d.start(st)
	return d
}

// start runs the app over a store.
func (d *desk) start(st *store) {
	d.svc = &FrameFairy{store: st}
	if d.time != nil {
		st.machine.Clock = d.time
	}
	svc := d.svc
	svc.newReport()
	if d.bus != nil {
		// The bridge tells its pages what the app tells its window, with
		// the app's own wiring, see events.go.
		svc.tellWindow(d)
	} else {
		// A path test has no window to tell.
		svc.jobs = newQueue(st, func(JobUpdate) {}, func(string) {})
	}
	d.svc.listening.still = d.still
	d.svc.listening.beats = d.beats
	d.svc.listening.most = d.most
	// The clipboard is the desk's, so a walk can read what was copied.
	d.svc.clipboard = func(text string) bool {
		d.clipboard.Store(&text)
		return true
	}
	// The save box is the desk's: it saves on the desk's Desktop under the
	// name the app offers, in the store's files, so a walk can read what
	// was exported.
	d.svc.saveBox = func(name string) (string, error) {
		desktop := filepath.Join(d.home, "Desktop")
		if err := st.files.MkdirAll(desktop, 0o755); err != nil {
			return "", err
		}
		path := filepath.Join(desktop, name)
		d.exported.Store(&path)
		return path, nil
	}
	d.svc.report.make = func(closed func()) (func(), func()) {
		d.reportMade.Add(1)
		return func() { d.reportFronted.Add(1) }, closed
	}
	d.svc.report.pick = func() ([]string, error) {
		d.pictureMu.Lock()
		defer d.pictureMu.Unlock()
		picked := d.pictures
		d.pictures = nil
		return picked, nil
	}
	if d.bus != nil {
		// The library is on the real disk, so the walks change its
		// folders from outside the app the way Finder does. The watch
		// starts once every field it can reach is set, the way main
		// starts it.
		svc.watchTheDisk()
	}
	// Whatever is still running when the test ends is stopped before its
	// folder goes.
	d.t.Cleanup(func() {
		svc.watch.stop()
		svc.news.stop()
		svc.jobs.shutDown()
	})
	logJobsOnFailure(d.t, svc.jobs)
	if d.fit != nil {
		d.fit(d.svc)
	}
}

// Emit tells the pages on the bridge's bus what the app tells its window,
// the setup as the bridge's machine has it, see standInsSetup, and a
// recording's news to the bridge's microphone too, see told.
func (d *desk) Emit(name string, data ...any) bool {
	if len(data) == 1 {
		switch v := data[0].(type) {
		case SetupState:
			if d.setup != nil {
				data[0] = d.setup(v)
			}
		case Dictation:
			if d.told != nil {
				d.told(v)
			}
		}
	}
	return d.bus.Emit(name, data...)
}

// episode puts a copy of the video of this many seconds in the library's
// folder under name, see grey in internal/fixtures.
//
// On the real disk: ffmpeg and the episode's decoder read the video.
func (d *desk) episode(name string, seconds string) string {
	d.t.Helper()
	return fixtures.Copy(d.t, "grey-"+seconds+"s.mp4", filepath.Join(d.home, name+".mp4"))
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
	// A test that hangs on the programs it started, the decoder or
	// ffmpeg, leaves what each of them was doing in the log.
	flag.Parse()
	stop := hangdump.BeforeTimeout()
	code := m.Run()
	stop()
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

// search presses Search on a window.
func (d *desk) search(path string, w window) {
	d.svc.Search(path, w.request(), "")
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
	d.svc.Continue(j.ID, "")
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

// press presses Search on a window, as the press named click, and gives the
// job the Go side answers with.
func (d *desk) press(path string, w window, click string) Job {
	return d.svc.Search(path, w.request(), click)
}

// stop presses Cancel at the head of the clip list, after the press named
// click: all the work on the episode's clips stops.
func (d *desk) stop(path, click string) {
	d.svc.StopClipWork(path, click)
}

// in presses I with the playhead at a moment, and out presses O.
func (d *desk) in(path string, at float64) Job  { return d.svc.MakeClip(path, at, false) }
func (d *desk) out(path string, at float64) Job { return d.svc.MakeClip(path, at, true) }

// render presses Render on every clip of a plan.
func (d *desk) render(path, plan string) {
	d.svc.Render(path, engine.RenderRequest{Plan: plan})
}

// close quits the app, the way the window's close button does, through
// the one way the app closes itself, see closeDown.
func (d *desk) close() {
	d.t.Helper()
	if !d.svc.closeDown() {
		d.t.Fatal("the app did not stop its work in time")
	}
}

// closeWhileIn closes the app while j hears a piece the speech model
// holds, see speech.hold, and lets the piece end once j is called off, the
// way a real model finishes the piece it is in after the app asked it to
// stop.
func (d *desk) closeWhileIn(j Job, let func()) {
	d.t.Helper()
	shut := make(chan bool, 1)
	go func() { shut <- d.svc.jobs.shutDown() }()
	<-j.ctx.Done()
	let()
	if !<-shut {
		d.t.Fatal("the app did not stop its work in time")
	}
}

// copied is what the app last put on the clipboard, nothing before.
func (d *desk) copied() string {
	if p := d.clipboard.Load(); p != nil {
		return *p
	}
	return ""
}

// reopen starts the app again over the same library, the way main does.
func (d *desk) reopen() {
	d.shut()
	d.open()
}

// shut is the app closing: it sends nothing more, as it does when it
// quits, and hears nothing more of the disk, so what changes on the disk
// until open is a change made while the app was closed.
func (d *desk) shut() {
	d.svc.watch.stop()
	d.svc.news.stop()
}

// open starts the app over the library on the disk, the way main does.
func (d *desk) open() {
	d.openOn(openStore())
}

// openAgainOnTheSameComputer closes the app and opens it again, as reopen
// does, on a machine that knows what the one before learned about this
// computer's programs, see engine.Machine.LearnFrom: the bridge's reset
// starts a walk on a fresh app, not on a fresh computer. Opened on a new
// machine, every walk asked ffmpeg again what the walk before had asked,
// a third of the processor time of opening a video.
func (d *desk) openAgainOnTheSameComputer() {
	before := d.svc.store.machine
	d.shut()
	st := openStore()
	st.machine.LearnFrom(before)
	d.openOn(st)
}

// openOn starts the app over st, with the desk's stand-ins.
func (d *desk) openOn(st *store) {
	st.standIns = d.fakes
	d.start(st)
	d.svc.restoreJobs(st.Episodes())
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
	case JobStopped:
		return outcome{stopped: true, window: w}
	case JobInterrupted:
		return outcome{interrupted: true, window: w}
	case JobFailed:
		return outcome{failed: j.Error, window: w}
	}
	return outcome{}
}

// idle waits until nothing of the video is queued or running, told by
// the queue the moment its last job ends. Work that never ends hangs the
// test until go test's own -timeout, which shows every goroutine.
func (d *desk) idle(path string) {
	d.t.Helper()
	// A video just added has its picture checked behind the answer, and
	// its first search starts once it is.
	<-d.svc.checked(path)
	d.svc.jobs.until(func(jobs []*Job) bool {
		return !slices.ContainsFunc(jobs, func(j *Job) bool { return j.Episode == path && j.going() })
	}, nil)
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
//
// On the real disk: the transcript lies beside an episode ffmpeg reads.
func (d *desk) heard(path string) (float64, bool) {
	return engine.Coverage(disk, path, d.svc.store.Settings().ASRModel)
}

// shorts are the finished shorts of the library, each where its path
// really leads, the way the engine names a short. On macOS the temp folder
// is under /var, which is a link to /private/var.
func (d *desk) shorts() []string {
	var found []string
	_ = filepath.WalkDir(d.svc.store.Settings().OutputDir, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.HasSuffix(p, ".mp4") {
			found = append(found, fileio.ResolvePath(p))
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
// holds a piece for as long as a test says, so the test can close the app
// or press Cancel while it hears, and counts the seconds of audio it was
// given, so a test can tell whether anything was heard twice.
type speech struct {
	mu      sync.Mutex
	broken  error
	started chan struct{}
	// skip is how many pieces are heard before started is closed.
	skip    int
	gate    chan struct{}
	samples atomic.Int64
}

// breaks makes the speech model fail to load, the way a missing or damaged
// model does.
func (s *speech) breaks(err error) {
	s.mu.Lock()
	s.broken = err
	s.mu.Unlock()
}

// hearingAfter gives a channel that is closed when the speech model is
// given audio once more after heard pieces, so those are heard to their
// end. A piece still being heard when the work stops is let go, so a test
// about what a stop keeps stops it after a piece was heard.
func (s *speech) hearingAfter(heard int) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = make(chan struct{})
	s.skip = heard
	return s.started
}

// hold holds the next piece the speech model is given until let is
// called, the way a real model is inside one call that takes no stop. in
// is closed once that piece is in.
func (s *speech) hold() (in <-chan struct{}, let func()) {
	return s.holdAfter(0)
}

// holdAfter holds the piece after heard pieces, which are heard to their
// end, so a test about what a stop keeps has something heard to keep.
func (s *speech) holdAfter(heard int) (in <-chan struct{}, let func()) {
	in = s.hearingAfter(heard)
	gate := make(chan struct{})
	s.mu.Lock()
	s.gate = gate
	s.mu.Unlock()
	return in, sync.OnceFunc(func() { close(gate) })
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
	var gate chan struct{}
	if s.started != nil && s.skip > 0 {
		s.skip--
	} else {
		if s.started != nil {
			close(s.started)
			s.started = nil
		}
		gate, s.gate = s.gate, nil
	}
	s.mu.Unlock()
	if gate != nil {
		<-gate
	}
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
// hold its answers until it is let go of, or to fail.
type model struct {
	server *httptest.Server
	mu     sync.Mutex
	hang   bool
	fail   bool
	asked  chan struct{}
	calls  atomic.Int32
	// holding is how many answers it holds now, for a walk to know that a
	// search waiting on it has nothing to report, see /model.
	holding atomic.Int32
	// let is closed when the model stops holding its answers, and gives
	// every answer it held, see hangs.
	let chan struct{}
	// done lets go of every answer held, when the test ends.
	done chan struct{}
}

func newModel(t *testing.T) *model {
	m := &model{let: make(chan struct{}), done: make(chan struct{})}
	m.server = httptest.NewServer(http.HandlerFunc(m.answer))
	t.Cleanup(m.server.Close)
	t.Cleanup(func() { close(m.done) })
	return m
}

// hangs makes the model hold every answer until the search is stopped or
// the model is told to answer again. Told to answer, it answers what it
// held too, the way a model that was slow answers in the end: a search
// asked while it held went on finding for good, and a video added after
// it never got its turn to be heard, plan row 2.222.
func (m *model) hangs(on bool) {
	m.mu.Lock()
	if m.hang && !on {
		close(m.let)
		m.let = make(chan struct{})
	}
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
	hang, fail, let := m.hang, m.fail, m.let
	if m.asked != nil {
		close(m.asked)
		m.asked = nil
	}
	m.mu.Unlock()
	if hang && !fail {
		m.holding.Add(1)
		defer m.holding.Add(-1)
		select {
		case <-r.Context().Done():
			return
		case <-m.done:
			return
		case <-let:
		}
		// Let go of, it answers the way it is set now.
		m.mu.Lock()
		fail = m.fail
		m.mu.Unlock()
	}
	if fail {
		http.Error(w, "the model fell over", http.StatusInternalServerError)
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

var errBroken = errors.New("the speech model could not be loaded")

// toolsFromThePath names the ffmpeg, ffprobe and llama-server on the search
// path in the environment, where the programs take a tool from by choice.
// A program never falls back to the search path by itself, see
// tool.Finder, and a test binary has nothing beside it. One already
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
