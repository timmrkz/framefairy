package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	answer, err := e.CallLocal(context.Background(), LocalModel{URL: server.URL, Think: 2048}, "Transcript:", 40, 12, 1000, "",
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
	_, err := e.CallLocal(context.Background(), LocalModel{URL: server.URL}, "x", 1, 1, 10, "", nil)
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
