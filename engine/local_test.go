package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

	e := NewEngine(NewLog(io.Discard, false, false))
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
	e := NewEngine(NewLog(io.Discard, false, false))
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
func TestTheServerLogIsKeptBeforeTheLogsFolderExists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for llama-server is a shell script")
	}
	dir := t.TempDir()
	server := filepath.Join(dir, "llama-server")
	script := "#!/bin/sh\necho \"buffer size stand-in $*\"\nexec sleep 30\n"
	if err := os.WriteFile(server, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "episode.framefairy", "logs")
	logFile := filepath.Join(logDir, "llm-server.log")

	// The stand-in never answers, so the start is given up once its
	// output has been seen, or after ten seconds.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		e := NewEngine(NewLog(io.Discard, false, false))
		_, _, _ = e.startServer(ctx, LocalModel{Server: server, Model: model}, 16384, logDir)
	}()
	var body []byte
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if body, _ = os.ReadFile(logFile); strings.Contains(string(body), "buffer size") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	<-done
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
func TestAServerThatWillNotStopIsKilledAtOnce(t *testing.T) {
	was := serverNoteFile
	note := filepath.Join(t.TempDir(), "llama-server.json")
	serverNoteFile = func() string { return note }
	t.Cleanup(func() { serverNoteFile = was })
	t.Setenv("FRAMEFAIRY_STUBBORN_SERVER", "1")
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(NewLog(io.Discard, false, false))
	_, stop, err := e.startServer(context.Background(), LocalModel{Server: os.Args[0], Model: model}, 4096, "")
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	stop()
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("stopping a server that ignores the interrupt took %s", took)
	}
}

// The name of a model is enough on the command line. A bare file name that
// is not in the working folder is found in the models folder, and anything
// else is left as it was given, to be refused with its own name.
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
