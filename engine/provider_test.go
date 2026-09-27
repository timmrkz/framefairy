package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestProviderFor(t *testing.T) {
	for model, want := range map[string]string{
		"claude-sonnet-5":           "anthropic",
		"claude-haiku-4-5-20251001": "anthropic",
		"gpt-5.6-terra":             "openai",
		"gpt-5.6-luna":              "openai",
		"o3-mini":                   "openai",
		// A model nobody has heard of is Anthropic's, which is what every
		// model was before there was a choice.
		"something-else": "anthropic",
		"":               "anthropic",
	} {
		if got := ProviderFor(model).Name; got != want {
			t.Errorf("%q belongs to %s, want %s", model, got, want)
		}
	}
	// Every model the app offers by name belongs to the provider it says.
	for _, m := range CloudModels() {
		if got := ProviderFor(m.Model).Name; got != m.Provider {
			t.Errorf("%s is offered as %s's but goes to %s", m.Model, m.Provider, got)
		}
		if !FactsFor(m.Model).Priced {
			t.Errorf("%s is offered without a price, so a search could not say what it costs", m.Model)
		}
	}
}

// An answer from OpenAI as it streams, put back together the way
// Anthropic's is, so everything after reading works on one kind of answer.
func TestReadOpenAIStream(t *testing.T) {
	lines := []string{
		`data: {"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"{\"clips\": "},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{"content":"[]}"},"finish_reason":null}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":1200,"completion_tokens":40,"prompt_tokens_details":{"cached_tokens":100},"completion_tokens_details":{"reasoning_tokens":30}}}`,
		`data: [DONE]`,
	}
	stream := strings.Join(lines, "\n\n") + "\n\n"
	var heard strings.Builder
	reply, spoke, err := readOpenAIStream(strings.NewReader(stream), &Listener{
		Text: func(p string) { heard.WriteString(p) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !spoke || heard.String() != `{"clips": []}` || replyText(reply) != `{"clips": []}` {
		t.Errorf("heard %q, reply %q", heard.String(), replyText(reply))
	}
	if reply.StopReason != "end_turn" || reply.Usage.InputTokens != 1200 ||
		reply.Usage.OutputTokens != 40 || reply.Usage.CacheReadInputTokens != 100 {
		t.Errorf("reply %+v", reply)
	}
	if len(reply.Content) != 2 || reply.Content[0].Type != "thinking" {
		t.Errorf("blocks %+v", reply.Content)
	}

	// Cut before the end, it did not finish, and says whether anything had
	// been heard by then.
	cut := strings.Join(lines[:3], "\n\n") + "\n\n"
	if _, spoke, err := readOpenAIStream(strings.NewReader(cut), nil); err == nil || !spoke {
		t.Errorf("a cut stream: spoke %v, err %v", spoke, err)
	}
	if _, spoke, err := readOpenAIStream(strings.NewReader(lines[0]+"\n\n"), nil); err == nil || spoke {
		t.Errorf("cut before a word: spoke %v, err %v", spoke, err)
	}

	// A ceiling spent thinking reads as Anthropic's does: a thinking block
	// and no text, stopped at max_tokens.
	thought := strings.Join([]string{
		`data: {"choices":[{"delta":{},"finish_reason":"length"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":500,"completion_tokens_details":{"reasoning_tokens":500}}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	reply, _, err = readOpenAIStream(strings.NewReader(thought), nil)
	if err != nil || reply.StopReason != "max_tokens" || len(reply.Content) != 1 || reply.Content[0].Type != "thinking" {
		t.Errorf("spent thinking: %+v, %v", reply, err)
	}

	// An error in the middle says whether it is worth asking again.
	for body, again := range map[string]bool{
		`data: {"error":{"type":"server_error","message":"try later"}}`:                        true,
		`data: {"error":{"type":"tokens","code":"rate_limit_exceeded","message":"slow down"}}`: true,
		`data: {"error":{"type":"invalid_request_error","code":null,"message":"bad request"}}`: false,
	} {
		_, _, err := readOpenAIStream(strings.NewReader(body+"\n\n"), nil)
		if refused, ok := asStreamError(err); !ok || refused.transient() != again {
			t.Errorf("%s: %v", body, err)
		}
	}
}

// fakeCloud stands in for both providers at once. It records what each
// request carried and answers from a queue, one answer per request.
type fakeCloud struct {
	mu       sync.Mutex
	requests []seenRequest
	answers  []func(w http.ResponseWriter)
}

type seenRequest struct {
	path    string
	headers http.Header
	body    map[string]any
}

func (f *fakeCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.requests = append(f.requests, seenRequest{r.URL.Path, r.Header.Clone(), body})
	var answer func(w http.ResponseWriter)
	if len(f.answers) > 0 {
		answer, f.answers = f.answers[0], f.answers[1:]
	}
	f.mu.Unlock()
	if answer == nil {
		http.Error(w, `{"error":{"message":"no answer queued"}}`, http.StatusTeapot)
		return
	}
	answer(w)
}

func (f *fakeCloud) seen() []seenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenRequest(nil), f.requests...)
}

// cloud points both providers at one fake server for the length of a test,
// with a key for each in the environment.
func cloud(t *testing.T, answers ...func(w http.ResponseWriter)) (*fakeCloud, *Engine) {
	t.Helper()
	fake := &fakeCloud{answers: answers}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	saved := providers
	providers = nil
	for _, p := range saved {
		p.URL = server.URL + "/" + p.Name
		providers = append(providers, p)
	}
	t.Cleanup(func() { providers = saved })
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("OPENAI_API_KEY", "sk-proj-test")
	return fake, NewEngine(NewLog(&bytes.Buffer{}, false, false))
}

func openAIStream(pieces []string, finish string, usage string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("content-type", "text/event-stream")
		for _, p := range pieces {
			chunk, _ := json.Marshal(map[string]any{"choices": []any{
				map[string]any{"delta": map[string]any{"content": p}, "finish_reason": nil}}})
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":%q}]}\n\n", finish)
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":%s}\n\n", usage)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

func claudeStream(text string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("content-type", "text/event-stream")
		delta, _ := json.Marshal(text)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", delta)
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}
}

// Each provider is asked at its own address, with its own key in its own
// header and in its own shape, and the answer comes back the same either
// way, heard as it is written.
func TestCallAPIAsksEachProviderItsOwnWay(t *testing.T) {
	const answer = `{"clips": []}`
	fake, e := cloud(t,
		openAIStream([]string{`{"clips": `, `[]}`}, "stop", `{"prompt_tokens":100,"completion_tokens":20}`),
		claudeStream(answer),
	)
	var heard strings.Builder
	listen := &Listener{Text: func(p string) { heard.WriteString(p) }}

	text, err := e.CallAPI(context.Background(), "the transcript", "gpt-5.6-terra", 4000, "", "plan", "", listen)
	if err != nil || text != answer || heard.String() != answer {
		t.Fatalf("openai: %q, heard %q, %v", text, heard.String(), err)
	}
	e.Prefill = true
	heard.Reset()
	text, err = e.CallAPI(context.Background(), "the transcript", "claude-sonnet-5", 4000, "", "plan", "", listen)
	// A prefilled brace is sent and not sent back, so it is put in front.
	if err != nil || text != "{"+answer || heard.String() != "{"+answer {
		t.Fatalf("anthropic: %q, heard %q, %v", text, heard.String(), err)
	}

	seen := fake.seen()
	if len(seen) != 2 {
		t.Fatalf("%d requests", len(seen))
	}
	gpt, claude := seen[0], seen[1]
	if gpt.path != "/openai" || gpt.headers.Get("authorization") != "Bearer sk-proj-test" ||
		gpt.headers.Get("x-api-key") != "" {
		t.Errorf("openai was asked at %s with %v", gpt.path, gpt.headers)
	}
	if gpt.body["model"] != "gpt-5.6-terra" || gpt.body["max_completion_tokens"] != float64(4000) ||
		gpt.body["max_tokens"] != nil || gpt.body["stream"] != true {
		t.Errorf("openai body %v", gpt.body)
	}
	if opts, _ := gpt.body["stream_options"].(map[string]any); opts["include_usage"] != true {
		t.Errorf("openai was not asked what the call cost: %v", gpt.body["stream_options"])
	}
	msgs, _ := gpt.body["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "developer" ||
		msgs[1].(map[string]any)["content"] != "the transcript" {
		t.Errorf("openai messages %v", msgs)
	}
	if claude.path != "/anthropic" || claude.headers.Get("x-api-key") != "sk-ant-test" ||
		claude.headers.Get("anthropic-version") == "" || claude.headers.Get("authorization") != "" {
		t.Errorf("anthropic was asked at %s with %v", claude.path, claude.headers)
	}
	if claude.body["system"] == nil || claude.body["max_tokens"] != float64(4000) {
		t.Errorf("anthropic body %v", claude.body)
	}
	// OpenAI takes no half-written answer, so a prefill never reaches it,
	// and Anthropic gets its brace.
	cmsgs, _ := claude.body["messages"].([]any)
	if len(cmsgs) != 2 || cmsgs[1].(map[string]any)["content"] != "{" {
		t.Errorf("anthropic messages %v", cmsgs)
	}
}

// OpenAI asking to be asked again later is asked again, a key it refuses is
// said to be refused, with where to get another, and a model that spent
// the whole ceiling thinking is given more room, the same as Anthropic's.
func TestCallAPIOpenAIRetriesRefusalsAndHeadroom(t *testing.T) {
	busy := func(w http.ResponseWriter) {
		w.Header().Set("retry-after", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"slow down","type":"requests"}}`)
	}
	fake, e := cloud(t, busy, openAIStream([]string{`{"clips": []}`}, "stop", `{"prompt_tokens":1,"completion_tokens":1}`))
	if text, err := e.CallAPI(context.Background(), "p", "gpt-5.6-terra", 100, "", "plan", "", nil); err != nil || text != `{"clips": []}` {
		t.Errorf("after a 429: %q, %v", text, err)
	}
	if n := len(fake.seen()); n != 2 {
		t.Errorf("%d requests for one 429 and an answer", n)
	}

	refused := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`)
	}
	_, e = cloud(t, refused)
	_, err := e.CallAPI(context.Background(), "p", "gpt-5.6-terra", 100, "", "plan", "", nil)
	if err == nil || !strings.Contains(err.Error(), "platform.openai.com") {
		t.Errorf("a refused key: %v", err)
	}

	spent := openAIStream(nil, "length", `{"prompt_tokens":1,"completion_tokens":100,"completion_tokens_details":{"reasoning_tokens":100}}`)
	fake, e = cloud(t, spent, openAIStream([]string{`{"clips": []}`}, "stop", `{"prompt_tokens":1,"completion_tokens":150}`))
	text, err := e.CallAPIWithHeadroom(context.Background(), "p", "gpt-5.6-terra", 100, "", "plan", nil)
	if err != nil || text != `{"clips": []}` {
		t.Fatalf("after thinking the ceiling away: %q, %v", text, err)
	}
	seen := fake.seen()
	if len(seen) != 2 || seen[1].body["max_completion_tokens"] != float64(200) {
		t.Errorf("the second ask had %v", seen[len(seen)-1].body["max_completion_tokens"])
	}
}

// A repair is asked without streaming, and OpenAI's plain answer is read
// the same way as its streamed one.
func TestRepairJSONAsksOpenAIPlainly(t *testing.T) {
	plain := func(w http.ResponseWriter) {
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"clips\": []}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}`)
	}
	fake, e := cloud(t, plain)
	text, err := e.RepairJSON(context.Background(), `{"clips": [`, "gpt-5.6-terra", "")
	if err != nil || text != `{"clips": []}` {
		t.Fatalf("repair: %q, %v", text, err)
	}
	if body := fake.seen()[0].body; body["stream"] != nil || body["model"] != "gpt-5.6-terra" {
		t.Errorf("repair body %v", body)
	}
}

// A search on a model in the cloud reports that model's company's key as
// missing, in that company's words, before anything is sent.
func TestReadAPIKeyNamesTheProvider(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	p, _ := ProviderNamed("openai")
	_, err := ReadAPIKey(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") || !strings.Contains(err.Error(), "OpenAI") {
		t.Errorf("no key: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "sk-proj-a\nb")
	if _, err := ReadAPIKey(context.Background(), p); err == nil {
		t.Error("a key with a newline in it was taken")
	}
}

// The whole of a search on a model in the cloud from OpenAI: the episode is
// transcribed, the transcript goes to OpenAI with OpenAI's key, the answer
// arrives as it is written, and the plan it makes is written like any other.
// This is the journey somebody with an OpenAI key takes, end to end, with
// the only thing that is not real being the far end of the call.
func TestProjectPlansWithOpenAI(t *testing.T) {
	source := testEpisode(t, "40")
	SetTrainingDir(t.TempDir())
	plan := `{"clips": [{"slug": "erste", "title": "Erste", "reason": "Test", "keep": [[1, 1]]}]}`
	fake, e := cloud(t, openAIStream([]string{plan[:30], plan[30:]}, "stop",
		`{"prompt_tokens":900,"completion_tokens":60}`))
	var heard int32
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }

	base := DefaultOptions()
	base.Planner = "api"
	base.Model = "gpt-5.6-terra"
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Preset = "ultrafast"
	p := NewProject(e, source, base)
	ctx := context.Background()
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("transcribe: %v %s", err, p.LastError())
	}
	path, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	_, clips, err := LoadClips(path)
	if err != nil || len(clips) != 1 || clips[0].Title != "Erste" {
		t.Fatalf("the plan from OpenAI's answer: %+v, %v", clips, err)
	}
	seen := fake.seen()
	if len(seen) != 1 || seen[0].path != "/openai" || seen[0].body["model"] != "gpt-5.6-terra" {
		t.Fatalf("asked %d times, first at %v", len(seen), seen)
	}
	msgs, _ := seen[0].body["messages"].([]any)
	if len(msgs) != 2 || !strings.Contains(fmt.Sprint(msgs[1]), "wort") {
		t.Errorf("the transcript did not go with the request: %v", msgs)
	}
}

// Whatever OpenAI sends is read without a panic, and an answer read whole
// is exactly what was heard of it on the way, because the clips taken as it
// arrived and the answer read at the end have to be the same answer. The
// plain reply of a repair is read the same way.
func FuzzReadOpenAIStream(f *testing.F) {
	f.Add("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"clips\\\": []}\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	f.Add("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"completion_tokens_details\":{\"reasoning_tokens\":2}}}\n\ndata: [DONE]\n\n")
	f.Add("data: {\"error\":{\"type\":\"server_error\",\"code\":7,\"message\":\"x\"}}\n\n")
	f.Add("data: {\"choices\":[{\"delta\":{\"content\":null},\"finish_reason\":null}]}\n\n")
	f.Add(`{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`)
	f.Fuzz(func(t *testing.T, stream string) {
		var heard strings.Builder
		reply, _, err := readOpenAIStream(strings.NewReader(stream), &Listener{
			Text: func(p string) { heard.WriteString(p) },
		})
		if err == nil && replyText(reply) != heard.String() {
			t.Errorf("heard %q, the reply says %q", heard.String(), replyText(reply))
		}
		_, _ = readOpenAIReply([]byte(stream))
	})
}
