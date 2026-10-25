package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

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

	e := NewEngine(NewLog(io.Discard, false, false), afero.NewMemMapFs())
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
	e := NewEngine(NewLog(io.Discard, false, false), afero.NewMemMapFs())
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

// The model is loaded ahead of the first search of a new episode, while
// nothing has made the episode's logs folder yet. The server's output is
// the only record of what the model took from memory, so it is kept even
// then.
//
// On the real disk: the stand-in llama-server is a program that is run, and its log is a file.
func TestTheServerLogIsKeptBeforeTheLogsFolderExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for llama-server is a shell script")
	}
	dir := t.TempDir()
	server := filepath.Join(dir, "llama-server")
	// It says what it was started with and ends, so the start is over
	// once it has said it.
	script := "#!/bin/sh\necho \"buffer size stand-in $*\"\n"
	if err := standin.Write(server, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "episode.framefairy", "logs")
	logFile := filepath.Join(logDir, "llm-server.log")

	e := NewEngine(NewLog(io.Discard, false, false), disk)
	if _, _, err := e.startServer(context.Background(), LocalModel{Server: server, Model: model}, 16384, logDir); err == nil {
		t.Fatal("a server that ended while it loaded was taken for loaded")
	}
	body, _ := os.ReadFile(logFile)
	if !strings.Contains(string(body), "buffer size") {
		t.Fatalf("no server output in %s, got %q", logFile, body)
	}
	// What the model took from memory is only in the log from level 4.
	if !strings.Contains(string(body), "-lv 4") {
		t.Errorf("llama-server was not asked to say what it took from memory: %q", body)
	}
}

// A llama-server that will not go when asked is killed a moment later.
// The app waited five seconds for one, frozen, on Cmd+Q and on removing an
// episode while a search ran.
//
// On the real disk: the stand-in llama-server is a program that is run, with its model as a file.
func TestAServerThatWillNotStopIsKilledAtOnce(t *testing.T) {
	noteIn(t)
	t.Setenv("FRAMEFAIRY_STUBBORN_SERVER", "1")
	// stopGrace is up the moment it is asked about. Not parallel: leaving
	// is a package variable, and t.Setenv rules it out too.
	was := leaving
	up := make(chan time.Time)
	close(up)
	leaving = func(time.Duration) <-chan time.Time { return up }
	t.Cleanup(func() { leaving = was })
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(NewLog(io.Discard, false, false), disk)
	_, stop, err := e.startServer(context.Background(), LocalModel{Server: os.Args[0], Model: model}, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	// A server that is never killed hangs here until go test's -timeout.
	// It is killed once stopGrace is up, at once here, rather than after
	// half a second of the clock.
	stop()
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
	e := NewEngine(NewLog(io.Discard, false, false), afero.NewMemMapFs())
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
// clock. Changes localClient, which the whole package shares, so it does
// not run in parallel.
func TestAHealthCheckThatGetsNoAnswerGivesUp(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		was := localClient
		localClient = &http.Client{Transport: neverAnswers{}}
		defer func() { localClient = was }()

		began := time.Now()
		if healthy(context.Background(), "http://llama-server") {
			t.Error("a server that never answered was taken for ready")
		}
		if took := time.Since(began); took != healthWait { //nolint:forbidigo // synctest bubble
			t.Errorf("a look at a server that never answers took %s on the bubble's clock, want %s", took, healthWait)
		}

		ctx, cancel := context.WithCancel(context.Background())
		looked := make(chan bool)
		began = time.Now()
		go func() { looked <- healthy(ctx, "http://llama-server") }()
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
	server := filepath.Join(dir, "llama-server")
	// It says what it was handed and ends, so the start is over once it
	// has said it.
	script := "#!/bin/sh\necho \"key=$LLAMA_API_KEY args=$*\"\n"
	if err := standin.Write(server, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	launch := func(name string) string {
		logDir := filepath.Join(dir, name)
		e := NewEngine(NewLog(io.Discard, false, false), disk)
		_, _, _ = e.startServer(context.Background(), LocalModel{Server: server, Model: model}, 16384, logDir)
		body, _ := os.ReadFile(filepath.Join(logDir, "llm-server.log"))
		return string(body)
	}
	keyOf := func(out string) string {
		_, after, _ := strings.Cut(out, "key=")
		key, _, _ := strings.Cut(after, " ")
		return key
	}
	first, second := launch("one"), launch("two")
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

	serverKeys.Store(answer.URL, "the-key")
	healthy(context.Background(), answer.URL)
	healthy(context.Background(), other.URL)
	serverKeys.Delete(answer.URL)
	healthy(context.Background(), answer.URL)

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
// clock. Changes localClient and launch, which the whole package shares,
// so it does not run in parallel.
func TestALocalModelThatStopsAnsweringIsGivenUpOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := localClient
		localClient = &http.Client{Transport: stopsAfter(`data: {"choices":[],"prompt_progress":{"total":100,"cache":0,"processed":50}}` + "\n\n")}
		var stopped atomic.Int32
		was := launch
		launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int, _ string) (string, func(), error) {
			var once sync.Once
			return "http://llama-server", func() { once.Do(func() { stopped.Add(1) }) }, nil
		}
		t.Cleanup(func() {
			StopModels()
			launch, localClient = was, client
		})
		StopModels()

		began := time.Now()
		_, err := quietEngine().CallLocal(context.Background(), LocalModel{Model: "stalls.gguf", Keep: time.Hour},
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
// clock. Changes localClient, which the whole package shares, so it does
// not run in parallel.
func TestALocalModelThatKeepsTalkingIsWaitedFor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		was := localClient
		localClient = &http.Client{Transport: streamFrom(func(w http.ResponseWriter) {
			// Ten lines, each half of localStall after the one before, five
			// times localStall in all.
			for done := 0; done < 100; done += 10 {
				fmt.Fprintf(w, `data: {"choices":[],"prompt_progress":{"total":100,"cache":0,"processed":%d}}`+"\n\n", done)
				time.Sleep(localStall / 2) //nolint:forbidigo // synctest bubble
			}
			writeLocalStream(w, `{"clips":[]}`, 4)
		})}
		defer func() { localClient = was }()

		began := time.Now()
		answer, err := quietEngine().CallLocal(context.Background(), LocalModel{URL: "http://llama-server"},
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
	// On the real disk: loadPhases reads the log llama-server writes
	// itself, which is always on the disk, see startServer.
	path := filepath.Join(t.TempDir(), "llm-server.log")
	log := strings.Join([]string{
		"0.00.012.000 I build: 11430",
		"0.01.500.000 I srv    load_model: loading model '/models/gemma.gguf'",
		"0.02.000.000 I load_tensors: loading model tensors, this can take a while... (load_mode = mmap)",
		"0.17.000.000 I llama_context: constructing llama_context",
		"0.18.000.000 I common_init_from_params: warming up the model with an empty run - please wait",
		"0.23.500.000 I srv    main: model loaded",
	}, "\n")
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadPhases(path, 24)
	want := []loadPhase{{"starting", 2}, {"reading the model", 15}, {"making room for the context", 1},
		{"warming up", 5.5}, {"getting ready to answer", 0.5}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the parts are %v, want %v", got, want)
	}
	// No warm-up line: its time is the context's.
	short := strings.Replace(log, "warming up the model", "something else", 1)
	if err := os.WriteFile(path, []byte(short), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadPhases(path, 24); len(got) != 4 || got[2].seconds != 6.5 {
		t.Errorf("without the warm-up's line the parts are %v", got)
	}
	if loadPhases(filepath.Join(t.TempDir(), "none.log"), 24) != nil {
		t.Error("a log that is not there has parts")
	}
}
