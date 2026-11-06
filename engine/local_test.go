package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/standin"
)

func TestPlanSchemaIsValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(planSchema(120, 12)), &schema); err != nil {
		t.Fatal(err)
	}
	// Property order is the order the model writes in, so it must survive.
	s := planSchema(120, 12)
	if !(strings.Index(s, `"slug"`) < strings.Index(s, `"title"`) &&
		strings.Index(s, `"title"`) < strings.Index(s, `"reason"`) &&
		strings.Index(s, `"reason"`) < strings.Index(s, `"keep"`)) {
		t.Errorf("properties out of order")
	}
	if !strings.Contains(s, `"maximum": 120`) || !strings.Contains(s, `"maxItems": 12`) {
		t.Errorf("limits missing: %s", s)
	}
}

func TestContextFor(t *testing.T) {
	if got := contextFor(1000, 48000); got != 32768 {
		t.Errorf("small prompt got %d", got)
	}
	// An hour of German transcript, roughly 90,000 characters.
	if got := contextFor(90000, 48000); got != 65536 {
		t.Errorf("hour got %d", got)
	}
	if got := contextFor(10_000_000, 48000); got != 262144 {
		t.Errorf("cap got %d", got)
	}
}

func TestCallLocalWithRunningServer(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &request)
		writeLocalStream(w, `{"clips":[]}`, 4)
	}))
	defer server.Close()

	e := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs()))
	var heard strings.Builder
	answer, err := e.CallLocal(context.Background(), LocalModel{URL: server.URL, Think: 2048}, linesRecipe, "Transcript:", 40, 12, 1000, "",
		&Listener{Text: func(p string) { heard.WriteString(p) }})
	if err != nil {
		t.Fatal(err)
	}
	reply := answer.Content
	if reply != `{"clips":[]}` || heard.String() != reply {
		t.Errorf("reply %q, heard %q", reply, heard.String())
	}
	if request["stream"] != true || request["return_progress"] != true {
		t.Errorf("not asked for a stream: %v", request)
	}
	if request["reasoning_budget_tokens"] != 2048.0 || request["reasoning_budget_message"] != thinkEnough {
		t.Errorf("thinking not held to its budget: %v", request)
	}
	format := request["response_format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Errorf("response_format %v", format)
	}
	messages := request["messages"].([]any)
	if messages[0].(map[string]any)["content"] != SystemPrompt {
		t.Errorf("system prompt not sent")
	}
}

func TestCallLocalReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":{"message":"the request exceeds the available context size"}}`)
	}))
	defer server.Close()
	e := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs()))
	_, err := e.CallLocal(context.Background(), LocalModel{URL: server.URL}, linesRecipe, "x", 1, 1, 10, "", nil)
	if err == nil || !strings.Contains(err.Error(), "context size") {
		t.Errorf("err = %v", err)
	}
}

func TestServerStamp(t *testing.T) {
	got, ok := serverStamp("0.19.335.706 I srv    load_model: loading model '/models/gemma.gguf'")
	if !ok || got < 19.3357 || got > 19.3358 {
		t.Errorf("read %v %v", got, ok)
	}
	if got, _ := serverStamp("1.02.949.664 I slot print_timing"); got < 62.94 || got > 62.95 {
		t.Errorf("a minute in read %v", got)
	}
	for _, bad := range []string{"", "load_model: loading", "a.b.c.d x", "0.19.335 x"} {
		if _, ok := serverStamp(bad); ok {
			t.Errorf("%q read as a time", bad)
		}
	}
}

// heardSaying is a machine whose programs say their lines into the
// lines it returns, one "from pid level: line" each, see ProgramSaid.
//
// On the real disk: it starts stand-in programs, which read their model
// from the disk.
func heardSaying() (*Machine, func() []string) {
	m := memoryUnasked(NewMachine(disk))
	var mu sync.Mutex
	var lines []string
	m.Said = func(from, video string, pid int, level slog.Level, line string) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf("%s %d %s: %s", from, pid, level, line))
	}
	return m, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(lines)
	}
}

// Every line llama-server writes reaches the program's log as
// "llama-server", with its process, at the level llama.cpp marks it with,
// without the time it puts before it, and with the model's description
// when the load fails. The error quotes its last lines, since a report of
// a problem holds the log and nothing else, #664.
//
// On the real disk: the stand-in llama-server is a program that is run.
func TestWhatTheServerSaysIsInTheLog(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for llama-server is a shell script")
	}
	dir := t.TempDir()
	server := filepath.Join(dir, "llama-server")
	// It says what it was started with and ends, so the start is over
	// once it has said it.
	script := "#!/bin/sh\n" +
		"echo \"0.00.000.100 I build: 11430 args $*\"\n" +
		"echo \"0.00.001.000 I print_info: file type   = Q4_K - Medium\"\n" +
		"echo \"0.00.002.000 W llama_context: n_ctx_seq (16384) < n_ctx_train\"\n" +
		"echo \"0.00.003.000 E llama_model_load: error loading model\" >&2\n" +
		"printf 'srv    main: exiting'\n"
	if err := standin.Write(server, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m, said := heardSaying()
	e := NewEngine(runlog.New(io.Discard, false, false), m)
	_, _, err := e.startServer(context.Background(), LocalModel{Server: server, Model: model}, 16384)
	if err == nil {
		t.Fatal("a server that ended while it loaded was taken for loaded")
	}
	lines := said()
	if len(lines) == 0 {
		t.Fatal("the server said nothing into the log")
	}
	var pid int
	if _, err := fmt.Sscanf(lines[0], "llama-server %d DEBUG: started, process", &pid); err != nil || pid <= 0 {
		t.Fatalf("the first line says no start with its process: %q", lines[0])
	}
	for _, want := range []string{
		"DEBUG: build: 11430 args ",
		"DEBUG: print_info: file type   = Q4_K - Medium",
		"WARN: llama_context: n_ctx_seq (16384) < n_ctx_train",
		"ERROR: llama_model_load: error loading model",
		"DEBUG: srv    main: exiting",
	} {
		prefix := fmt.Sprintf("llama-server %d %s", pid, want)
		if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, prefix) }) {
			t.Errorf("no line %q in %q", want, lines)
		}
	}
	// What the model took from memory is only said from level 4, and at
	// level 5 the server writes every request and answer whole, which is
	// the transcript, and a report must never hold it.
	if !strings.Contains(strings.Join(lines, "\n"), " -lv 4 ") {
		t.Errorf("llama-server was not started at level 4: %q", lines)
	}
	if !strings.Contains(err.Error(), "error loading model") {
		t.Errorf("the error does not say what the server said last: %v", err)
	}
}

// The description of a model file the app knows is left out of the log
// of a load that works: it is the same on every load, and the file is
// held to its SHA256 before it starts. It is said when the load fails.
// Everything else is said as it comes, and a mark that the server is
// ready is heard however its line is split over writes.
func TestTheModelsDescriptionIsSaidOnlyWhenTheLoadFails(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		var lines []string
		ready := make(chan struct{}, 8)
		o := &serverOutput{ready: ready, said: func(_ int, level slog.Level, line string) {
			lines = append(lines, level.String()+": "+line)
		}}
		o.begin(7)
		lines = nil
		for _, w := range []string{
			"0.00.001.000 I llama_model_loader: - kv   0: general.architecture str = gemma3\n",
			"0.00.002.000 I print_info: n_ctx_train = 131072\n0.00.003.000 I load_tensors: loading model ",
			"tensors\n0.00.004.000 I main: model lo", "aded\n",
		} {
			if n, err := o.Write([]byte(w)); n != len(w) || err != nil {
				t.Fatalf("Write(%q) = %d, %v", w, n, err)
			}
		}
		o.settle(failed)
		_, _ = o.Write([]byte("0.00.005.000 I print_info: said after the load\n"))
		want := []string{"DEBUG: load_tensors: loading model tensors", "DEBUG: main: model loaded"}
		if failed {
			want = append(want, "DEBUG: llama_model_loader: - kv   0: general.architecture str = gemma3",
				"DEBUG: print_info: n_ctx_train = 131072")
		}
		want = append(want, "DEBUG: print_info: said after the load")
		if !slices.Equal(lines, want) {
			t.Errorf("failed %v: the log holds %q, want %q", failed, lines, want)
		}
		if len(ready) != 1 {
			t.Errorf("failed %v: heard ready %d times, want once", failed, len(ready))
		}
		if got := o.loadLines(); len(got) != 2 {
			t.Errorf("failed %v: the lines that begin a part of the load are %q", failed, got)
		}
	}
}

// A llama-server that will not go when asked is killed a moment later.
// The app waited five seconds for one, frozen, on Cmd+Q and on removing an
// episode while a search ran.
//
// On the real disk: the stand-in llama-server is a program that is run, with its model as a file.
func TestAServerThatWillNotStopIsKilledAtOnce(t *testing.T) {
	machine, _ := noteIn(t)
	// Not parallel: t.Setenv rules it out.
	t.Setenv("FRAMEFAIRY_STUBBORN_SERVER", "1")
	// stopGrace is up the moment it is asked about, on the engine's own
	// runner.
	up := make(chan time.Time)
	close(up)
	// The start ends on the server's word that it is listening, and never
	// on a look of the clock, which here never comes, #550.
	machine.llama.lookAgain = func(time.Duration) <-chan time.Time { return nil }
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(runlog.New(io.Discard, false, false), machine)
	e.Runner.Leave = func(time.Duration) <-chan time.Time { return up }
	_, stop, err := e.startServer(context.Background(), LocalModel{Server: os.Args[0], Model: model}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	// A server that is never killed hangs here until go test's -timeout.
	// It is killed once stopGrace is up, at once here, rather than after
	// half a second of the clock.
	stop()
}

// The wait for a loading server ends on its own word that it may be ready,
// however its output is split into writes, and each such line is heard
// once. Everything it says still reaches the log.
func TestTheServerIsHeardSayingItIsReady(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		writes []string
		heard  int
	}{
		{"nothing of note", []string{"load_tensors: loading model tensors\n", "warming up the model\n"}, 0},
		{"one line", []string{"main: model loaded\n"}, 1},
		{"split over writes", []string{"main: model lo", "aded\n"}, 1},
		{"a byte at a time", strings.Split("main: server is listening on http://127.0.0.1:1\n", ""), 1},
		{"both lines", []string{"main: model loaded\nmain: server is listening on http://127.0.0.1:1\n"}, 2},
		{"both, apart", []string{"main: model loaded\n", "srv init: ready\n", "main: server is listening\n"}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var log strings.Builder
			ready := make(chan struct{}, 8)
			o := &serverOutput{ready: ready, said: func(_ int, _ slog.Level, line string) { log.WriteString(line + "\n") }}
			o.begin(7)
			log.Reset()
			for _, w := range c.writes {
				if n, err := o.Write([]byte(w)); n != len(w) || err != nil {
					t.Fatalf("Write(%q) = %d, %v", w, n, err)
				}
			}
			if len(ready) != c.heard {
				t.Errorf("heard %d times, want %d", len(ready), c.heard)
			}
			if want := strings.Join(c.writes, ""); log.String() != want {
				t.Errorf("the log holds %q, want %q", log.String(), want)
			}
		})
	}
}

// The name of a model is enough on the command line. A bare file name that
// is not in the working folder is found in the models folder, and anything
// else is left as it was given, to be refused with its own name.
//
// On the real disk: the models are on the disk, where llama-server reads them.
func TestAModelIsFoundByItsName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(ModelsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(ModelsDir(), "gemma.gguf")
	if err := os.WriteFile(installed, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LocalModelPath("gemma.gguf"); got != installed {
		t.Errorf("gemma.gguf is %s", got)
	}
	// One in the working folder is the one named.
	if err := os.WriteFile("gemma.gguf", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LocalModelPath("gemma.gguf"); got != "gemma.gguf" {
		t.Errorf("gemma.gguf beside us is %s", got)
	}
	for _, named := range []string{"", "missing.gguf", "sub/gemma.gguf"} {
		if got := LocalModelPath(named); got != named {
			t.Errorf("%q became %s", named, got)
		}
	}
}

// llama.cpp answers "Compute error." when the graphics memory runs out
// partway, which says nothing to somebody who only asked for clips. The
// error says what it most likely means and what to do.
func TestAComputeErrorSaysTheMemoryRanOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"code":500,"message":"Compute error.","type":"server_error"}}`)
	}))
	defer server.Close()
	e := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs()))
	_, err := e.CallLocal(context.Background(), LocalModel{URL: server.URL}, linesRecipe, "Transcript:",
		40, 12, 1000, "", nil)
	if err == nil || !strings.Contains(err.Error(), "ran out of memory") ||
		!strings.Contains(err.Error(), "another language model") {
		t.Errorf("got %v", err)
	}
}

// A server that takes the connection and never answers holds a look at
// its health no longer than healthWait, and not a moment once the load is
// cancelled. Without a limit, a load that hung there outlived Cancel and
// quitting.
//
// It runs in a bubble, with healthWait as the app has it, on the bubble's
// clock, and its machine reaches llama-server through a transport that
// never answers.
func TestAHealthCheckThatGetsNoAnswerGivesUp(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := NewMachine(afero.NewMemMapFs())
		m.llama.client = &http.Client{Transport: neverAnswers{}}

		began := time.Now()
		if m.healthy(context.Background(), "http://llama-server") {
			t.Error("a server that never answered was taken for ready")
		}
		if took := time.Since(began); took != healthWait { //nolint:forbidigo // synctest bubble
			t.Errorf("a look at a server that never answers took %s on the bubble's clock, want %s", took, healthWait)
		}

		ctx, cancel := context.WithCancel(context.Background())
		looked := make(chan bool)
		began = time.Now()
		go func() { looked <- m.healthy(ctx, "http://llama-server") }()
		// Cancelled while it waits for the answer, not before it asks.
		synctest.Wait()
		cancel()
		if <-looked {
			t.Error("a server that never answered was taken for ready once the load was cancelled")
		}
		if took := time.Since(began); took != 0 { //nolint:forbidigo // synctest bubble
			t.Errorf("a cancelled look waited %s on the bubble's clock, want none", took)
		}
	})
}

// neverAnswers takes every request and answers none, until the request is
// called off. No network, so it runs in a bubble.
type neverAnswers struct{}

func (neverAnswers) RoundTrip(r *http.Request) (*http.Response, error) {
	<-r.Context().Done()
	return nil, r.Context().Err()
}

// launchStandIn starts a stand-in llama-server that says what it was
// handed and ends, and returns what it said. Each name is a launch of its
// own.
//
// On the real disk: the stand-in llama-server is a program that is run, and its log is a file.
func launchStandIn(t *testing.T, dir, name string) string {
	t.Helper()
	server := filepath.Join(dir, "llama-server")
	if _, err := os.Stat(server); err != nil {
		// It says what it was handed and ends, so the start is over once
		// it has said it.
		script := "#!/bin/sh\necho \"key=$LLAMA_API_KEY args=$*\"\n"
		if err := standin.Write(server, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m, said := heardSaying()
	e := NewEngine(runlog.New(io.Discard, false, false), m)
	_, _, _ = e.startServer(context.Background(), LocalModel{Server: server, Model: model}, 16384)
	return strings.Join(said(), "\n")
}

// A llama-server the app starts answers only to the app: it is handed a
// key of its own in its environment, a new one every launch, never on its
// command line, and it lists nothing of what it is working on.
//
// On the real disk: the stand-in llama-server is a program that is run, and its log is a file.
func TestTheServerIsHandedAKeyOfItsOwn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for llama-server is a shell script")
	}
	dir := t.TempDir()
	keyOf := func(out string) string {
		_, after, _ := strings.Cut(out, "key=")
		key, _, _ := strings.Cut(after, " ")
		return key
	}
	first, second := launchStandIn(t, dir, "one"), launchStandIn(t, dir, "two")
	key := keyOf(first)
	if len(key) != 64 {
		t.Fatalf("the server was handed no key of 64 hex digits: %q", first)
	}
	if strings.Contains(first[strings.Index(first, "args="):], key) {
		t.Errorf("the key is on the command line, where any program can read it: %q", first)
	}
	if !strings.Contains(first, "--no-slots") {
		t.Errorf("the server lists what it is working on: %q", first)
	}
	if !strings.Contains(first, "-b "+itoa(promptBatch)) {
		t.Errorf("the server reads the prompt in batches of its own size, and says how far it is only after each: %q", first)
	}
	if other := keyOf(second); other == key || len(other) != 64 {
		t.Errorf("a second launch was handed %q after %q, not a key of its own", other, key)
	}
}

// The server keeps no copies of earlier prompts in ordinary memory. Left
// to itself it keeps up to 8 GiB of them, one each time a search of the
// next window starts, which the memory a model is judged by does not
// count and nothing asks for again. Issue #433.
//
// On the real disk: the stand-in llama-server is a program that is run, and its log is a file.
func TestTheServerKeepsNoCopiesOfEarlierPrompts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for llama-server is a shell script")
	}
	out := launchStandIn(t, t.TempDir(), "one")
	if !strings.Contains(out, " --cache-ram 0") {
		t.Errorf("the server keeps copies of earlier prompts, up to 8 GiB of them by itself: %q", out)
	}
	if !strings.Contains(out, " --no-cache-idle-slots") {
		t.Errorf("the server warns at every start that it has no cache to keep idle slots in: %q", out)
	}
}

// The key goes with a request to the server it belongs to and to no other
// address, and it is gone once that server is.
func TestTheKeyGoesOnlyToItsServer(t *testing.T) {
	var got []string
	answer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
	}))
	defer answer.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, "other:"+r.Header.Get("Authorization"))
	}))
	defer other.Close()

	m := NewMachine(afero.NewMemMapFs())
	m.llama.keys.Store(answer.URL, "the-key")
	m.healthy(context.Background(), answer.URL)
	m.healthy(context.Background(), other.URL)
	m.llama.keys.Delete(answer.URL)
	m.healthy(context.Background(), answer.URL)

	want := []string{"Bearer the-key", "other:", ""}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the requests carried %q, want %q", got, want)
	}
}

// A llama-server that stops answering partway is given up on, and the
// server is stopped even when the search asked to keep it, so the next
// search loads one that works. Before, the search waited for good and held
// every video's transcription up behind it (2.224).
//
// It runs in a bubble, with localStall as the app has it, on the bubble's
// clock, and its machine reaches llama-server through a transport that
// stops answering.
func TestALocalModelThatStopsAnsweringIsGivenUpOn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		e := quietEngine()
		e.llama.client = &http.Client{Transport: stopsAfter(`data: {"choices":[],"prompt_progress":{"total":100,"cache":0,"processed":50}}` + "\n\n")}
		var stopped atomic.Int32
		e.host.launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int) (string, func(), error) {
			var once sync.Once
			return "http://llama-server", func() { once.Do(func() { stopped.Add(1) }) }, nil
		}
		defer e.StopModels()

		began := time.Now()
		_, err := e.CallLocal(context.Background(), LocalModel{Model: "stalls.gguf", Keep: time.Hour},
			linesRecipe, "Transcript:", 40, 12, 1000, "", nil)
		if err == nil || !strings.Contains(err.Error(), "stopped answering") {
			t.Fatalf("err = %v", err)
		}
		if took := time.Since(began); took != localStall { //nolint:forbidigo // synctest bubble
			t.Errorf("a model that stopped answering was given up on after %s on the bubble's clock, want %s", took, localStall)
		}
		if stopped.Load() != 1 {
			t.Errorf("the server that stopped answering was stopped %d times, want once", stopped.Load())
		}
	})
}

// stopsAfter answers every request with first and then nothing more, until
// the request is called off, the way a server that stopped answering
// keeps its connection open. No network and no goroutine, so it runs in a
// bubble.
type stopsAfter string

func (first stopsAfter) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: io.NopCloser(io.MultiReader(strings.NewReader(string(first)), calledOff{r.Context()})), Request: r}, nil
}

// calledOff is a body that brings nothing until its request is called off.
type calledOff struct{ ctx context.Context }

func (c calledOff) Read([]byte) (int, error) {
	<-c.ctx.Done()
	return 0, c.ctx.Err()
}

// A machine that reads the prompt slowly is not taken for a stalled
// model: llama-server says how far it has read once a batch, and a batch
// is read inside localStall by any machine that reads slowestReading
// tokens a second. With llama.cpp's own batch of 2048 a machine had to
// read 17 a second, and a processor without a graphics chip llama.cpp can
// use might not.
func TestASlowMachineIsNotTakenForAStalledModel(t *testing.T) {
	if need := float64(promptBatch) / localStall.Seconds(); need >= slowestReading {
		t.Errorf("a batch of %d tokens read in %s needs %.1f tokens a second, want under %.1f",
			promptBatch, localStall, need, slowestReading)
	}
}

// A server that takes longer than localStall to answer, but says something
// all the while, is not a server that stopped answering.
//
// It runs in a bubble, with localStall as the app has it, on the bubble's
// clock, and its machine reaches llama-server through a transport that
// streams the answer.
func TestALocalModelThatKeepsTalkingIsWaitedFor(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		e := quietEngine()
		e.llama.client = &http.Client{Transport: streamFrom(func(w http.ResponseWriter) {
			// Ten lines, each half of localStall after the one before, five
			// times localStall in all.
			for done := 0; done < 100; done += 10 {
				fmt.Fprintf(w, `data: {"choices":[],"prompt_progress":{"total":100,"cache":0,"processed":%d}}`+"\n\n", done)
				time.Sleep(localStall / 2) //nolint:forbidigo // synctest bubble
			}
			writeLocalStream(w, `{"clips":[]}`, 4)
		})}

		began := time.Now()
		answer, err := e.CallLocal(context.Background(), LocalModel{URL: "http://llama-server"},
			linesRecipe, "Transcript:", 40, 12, 1000, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if answer.Content != `{"clips":[]}` {
			t.Errorf("answer %q", answer.Content)
		}
		if took := time.Since(began); took != 5*localStall { //nolint:forbidigo // synctest bubble
			t.Errorf("the answer took %s on the bubble's clock, want %s", took, 5*localStall)
		}
	})
}

// streamFrom answers every request with what write writes, as it writes
// it, the way llama-server streams an answer. A request called off ends
// what is read, as a closed connection does. No network, so it runs in a
// bubble.
type streamFrom func(w http.ResponseWriter)

func (write streamFrom) RoundTrip(r *http.Request) (*http.Response, error) {
	read, w := io.Pipe()
	stop := context.AfterFunc(r.Context(), func() { w.CloseWithError(r.Context().Err()) })
	go func() {
		defer stop()
		write(pipeWriter{w, http.Header{}})
		w.Close()
	}()
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: read, Request: r}, nil
}

// pipeWriter is a ResponseWriter over a pipe, for streamFrom.
type pipeWriter struct {
	*io.PipeWriter
	header http.Header
}

func (w pipeWriter) Header() http.Header { return w.header }
func (w pipeWriter) WriteHeader(int)     {}

// The time a model took to load is told in its parts, from the lines
// llama-server writes as it begins each, so the part to make shorter is
// known. A part whose line is missing goes to the part before it.
func TestLoadingIsToldInItsParts(t *testing.T) {
	t.Parallel()
	lines := []string{
		"0.01.500.000 I srv    load_model: loading model '/models/gemma.gguf'",
		"0.02.000.000 I load_tensors: loading model tensors, this can take a while... (load_mode = mmap)",
		"0.17.000.000 I llama_context: constructing llama_context",
		"0.18.000.000 I common_init_from_params: warming up the model with an empty run - please wait",
		"0.23.500.000 I srv    main: model loaded",
	}
	got := loadPhases(lines, 24)
	want := []loadPhase{{"starting", 2}, {"reading the model", 15}, {"making room for the context", 1},
		{"warming up", 5.5}, {"getting ready to answer", 0.5}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the parts are %v, want %v", got, want)
	}
	// No warm-up line: its time is the context's.
	short := slices.Clone(lines)
	short[3] = strings.Replace(short[3], "warming up the model", "something else", 1)
	if got := loadPhases(short, 24); len(got) != 4 || got[2].seconds != 6.5 {
		t.Errorf("without the warm-up's line the parts are %v", got)
	}
	if loadPhases(nil, 24) != nil {
		t.Error("a load that said nothing has parts")
	}
}
