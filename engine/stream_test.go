package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// writeLocalStream answers like llama-server does when asked for a stream:
// the reading of the prompt, the answer a few characters at a time, then
// what it cost, then the end marker.
func writeLocalStream(w http.ResponseWriter, answer string, piece int) {
	w.Header().Set("content-type", "text/event-stream")
	send := func(v any) {
		body, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", body)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	for done := 0; done <= 100; done += 50 {
		send(map[string]any{
			"choices":         []any{map[string]any{"delta": map[string]any{"role": "assistant", "content": nil}}},
			"prompt_progress": map[string]any{"total": 100, "cache": 0, "processed": done, "time_ms": done},
		})
	}
	for at := 0; at < len(answer); at += piece {
		end := min(at+piece, len(answer))
		send(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"content": answer[at:end]}, "finish_reason": nil}}})
	}
	send(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "stop"}}})
	send(map[string]any{"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 40},
		"timings": map[string]any{"prompt_n": 100, "prompt_per_second": 500.0, "predicted_n": 40, "predicted_per_second": 50.0}})
	fmt.Fprint(w, "data: [DONE]\n\n")
}

// scanAll feeds an answer to a scanner in pieces of the given size.
func scanAll(answer string, piece int) []string {
	var s clipScanner
	var found []string
	for at := 0; at < len(answer); at += piece {
		found = append(found, s.feed(answer[at:min(at+piece, len(answer))])...)
	}
	return found
}

func TestClipScannerFindsEachClipAsItCloses(t *testing.T) {
	answer := `{"clips": [{"slug": "a", "title": "Ein {Titel}", "reason": "r", "keep": [[1, 2], [4, 5]]}, ` +
		`{"slug": "b", "title": "Zwei \"}]\" ", "reason": "r", "keep": [[7, 9]]}]}`
	want := []string{
		`{"slug": "a", "title": "Ein {Titel}", "reason": "r", "keep": [[1, 2], [4, 5]]}`,
		`{"slug": "b", "title": "Zwei \"}]\" ", "reason": "r", "keep": [[7, 9]]}`,
	}
	// Whatever the pieces the answer arrives in, the same clips come out.
	for _, piece := range []int{1, 2, 3, 7, 50, len(answer)} {
		if got := scanAll(answer, piece); !reflect.DeepEqual(got, want) {
			t.Errorf("pieces of %d: got %q", piece, got)
		}
	}

	// And each one is out the moment its closing brace is in, not when
	// the answer ends.
	var s clipScanner
	first := strings.Index(answer, `]]}`) + 3
	if got := s.feed(answer[:first]); len(got) != 1 {
		t.Errorf("the first clip was not out when it closed: %q", got)
	}
	// The answer ends in the second clip's brace, then the list's, then
	// the answer's own.
	if got := s.feed(answer[first : len(answer)-2]); len(got) != 1 {
		t.Errorf("the second clip was not out when it closed: %q", got)
	}
	if got := s.feed(answer[len(answer)-2:]); len(got) != 0 {
		t.Errorf("the end of the answer made a clip: %q", got)
	}
}

func TestClipScannerOnlyReadsTheClipsList(t *testing.T) {
	cases := map[string]int{
		// A sentence before the answer, with a brace in it.
		`Here it is {not a plan}. {"clips": [{"keep": [[1, 1]]}]}`: 1,
		// A list under another name is not the clips.
		`{"other": [{"keep": [[1, 1]]}], "clips": [{"keep": [[2, 2]]}]}`: 1,
		// A value that happens to say clips is not the key.
		`{"note": "clips", "x": [{"keep": [[1, 1]]}]}`: 0,
		// An object deeper down is part of a clip, not one of its own.
		`{"clips": [{"keep": [[1, 1]], "more": {"deep": {"a": 1}}}]}`: 1,
		// A list on its own is not a plan.
		`[{"clips": [{"keep": [[1, 1]]}]}]`: 0,
		// A clip that never closes is never handed out.
		`{"clips": [{"keep": [[1, 1]]}, {"keep": [[2,`: 1,
		// Brackets that do not match end the reading. A clip that closed
		// before them was whole and has already been handed out.
		`{"clips": [{"keep": [[1, 1]]}], {"keep": [[2, 2]]}]}`: 1,
		`{"clips": [{"keep": [[1, 1]]], {"keep": [[2, 2]]}]}`:  0,
		// Two answers one after the other are both read.
		`{"clips": [{"keep": [[1, 1]]}]} {"clips": [{"keep": [[2, 2]]}]}`: 2,
	}
	for answer, want := range cases {
		if got := scanAll(answer, 3); len(got) != want {
			t.Errorf("%s: %d clip(s), want %d: %q", answer, len(got), want, got)
		}
	}
}

// FuzzClipScanner holds the scanner to two promises whatever a model
// writes: the pieces an answer arrives in never change what is found, and
// everything found is a balanced object that begins and ends with a brace.
func FuzzClipScanner(f *testing.F) {
	f.Add(`{"clips": [{"slug": "a", "keep": [[1, 2]]}, {"slug": "b\"}", "keep": [[3, 4]]}]}`, 3)
	f.Add(`{"clips": [{"a": "\\"}]}`, 1)
	f.Add(`{"clips": [{]}`, 2)
	f.Add(`x{"clips":[{}]}{"clips":[{"b":[{}]}]}`, 5)
	f.Fuzz(func(t *testing.T, answer string, piece int) {
		if piece < 1 || piece > 64 {
			piece = 1 + (piece%64+64)%64
		}
		whole := scanAll(answer, len(answer)+1)
		if len(answer) == 0 {
			whole = nil
		}
		split := scanAll(answer, piece)
		if len(whole) != len(split) {
			t.Fatalf("whole found %d, in pieces of %d found %d", len(whole), piece, len(split))
		}
		for i := range whole {
			if whole[i] != split[i] {
				t.Fatalf("clip %d differs: %q against %q", i, whole[i], split[i])
			}
			if !strings.HasPrefix(whole[i], "{") || !strings.HasSuffix(whole[i], "}") {
				t.Fatalf("not an object: %q", whole[i])
			}
		}
	})
}

func TestReadClaudeStream(t *testing.T) {
	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":1200,"cache_read_input_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}`,
		``,
		`event: ping`,
		`data: {"type": "ping"}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"{\"clips\": "}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"[]}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":40}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var heard strings.Builder
	thought := 0
	reply, spoke, err := readClaudeStream(strings.NewReader(stream), &Listener{
		Text:     func(p string) { heard.WriteString(p) },
		Thinking: func(n int) { thought += n },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !spoke || heard.String() != `{"clips": []}` || replyText(reply) != `{"clips": []}` {
		t.Errorf("heard %q, reply %q", heard.String(), replyText(reply))
	}
	if thought != 3 {
		t.Errorf("thought %d", thought)
	}
	if reply.StopReason != "end_turn" || reply.Usage.InputTokens != 1200 || reply.Usage.OutputTokens != 40 {
		t.Errorf("reply %+v", reply)
	}
	if len(reply.Content) != 2 || reply.Content[0].Type != "thinking" {
		t.Errorf("blocks %+v", reply.Content)
	}

	// A stream that stops without its last word did not finish, and says
	// whether anything had been heard by then.
	cut := stream[:strings.Index(stream, "event: message_delta")]
	if _, spoke, err := readClaudeStream(strings.NewReader(cut), nil); err == nil || !spoke {
		t.Errorf("a cut stream: spoke %v, err %v", spoke, err)
	}
	thinkingOnly := stream[:strings.Index(stream, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1")]
	if _, spoke, err := readClaudeStream(strings.NewReader(thinkingOnly), nil); err == nil || spoke {
		t.Errorf("cut while thinking: spoke %v, err %v", spoke, err)
	}

	// An error the server sends in the middle says what kind it was.
	overloaded := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"
	_, _, err = readClaudeStream(strings.NewReader(overloaded), nil)
	if refused, ok := asStreamError(err); !ok || !refused.transient() {
		t.Errorf("overloaded: %v", err)
	}
	invalid := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"no\"}}\n\n"
	_, _, err = readClaudeStream(strings.NewReader(invalid), nil)
	if refused, ok := asStreamError(err); !ok || refused.transient() {
		t.Errorf("invalid: %v", err)
	}

	// A delta for a block that was never started is a stream this does not
	// understand, not a reason to index past the end.
	stray := "data: {\"type\":\"content_block_delta\",\"index\":3,\"delta\":{\"type\":\"text_delta\",\"text\":\"x\"}}\n\n"
	if _, _, err := readClaudeStream(strings.NewReader(stray), nil); err == nil {
		t.Errorf("a stray delta was taken")
	}
}

func TestReadLocalStream(t *testing.T) {
	answer := `{"clips": [{"slug": "a", "title": "t", "reason": "r", "keep": [[1, 2]]}]}`
	rec := &flushRecorder{}
	writeLocalStream(rec, answer, 5)

	var heard strings.Builder
	var read [][2]int
	got, err := readLocalStream(strings.NewReader(rec.String()), &Listener{
		Text:    func(p string) { heard.WriteString(p) },
		Reading: func(done, total int) { read = append(read, [2]int{done, total}) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != answer || heard.String() != answer {
		t.Errorf("content %q, heard %q", got.Content, heard.String())
	}
	if !reflect.DeepEqual(read, [][2]int{{0, 100}, {50, 100}, {100, 100}}) {
		t.Errorf("reading %v", read)
	}
	if got.FinishReason != "stop" || got.PromptTokens != 100 || got.Written != 40 ||
		got.Timings == nil || got.Timings.PredictedPerSecond != 50 {
		t.Errorf("answer %+v", got)
	}

	cut := rec.String()[:strings.Index(rec.String(), `"finish_reason":"stop"`)-40]
	if _, err := readLocalStream(strings.NewReader(cut), nil); err == nil {
		t.Errorf("a cut stream was taken as an answer")
	}
	failed := "data: {\"error\":{\"message\":\"out of memory\"}}\n\n"
	if _, err := readLocalStream(strings.NewReader(failed), nil); err == nil ||
		!strings.Contains(err.Error(), "out of memory") {
		t.Errorf("error chunk: %v", err)
	}
}

// flushRecorder is a response writer that keeps what was written.
type flushRecorder struct {
	strings.Builder
	header http.Header
}

func (r *flushRecorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}
func (r *flushRecorder) WriteHeader(int) {}
func (r *flushRecorder) Flush()          {}
