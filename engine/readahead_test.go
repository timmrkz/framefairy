package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"
)

// fakeReader is a llama-server in miniature that lays a prompt out and
// splits it into tokens the way the real one does, and keeps what it read
// the way one slot keeps it. The template is Gemma's. A token is a
// character, but two line breaks are one token, so where a prompt is cut
// changes how its end is split, as it does with a real vocabulary.
type fakeReader struct {
	mu   sync.Mutex
	held []int
	// reads are the reads ahead, and asks the searches' asks, as they come.
	reads chan []int
	asks  chan fakeAsk
	// stuck, when set, is told of a read ahead, which then reads until it
	// is called off.
	stuck chan struct{}
}

// fakeAsk is a search's ask: its tokens, and how many of them the server
// held already.
type fakeAsk struct {
	tokens []int
	held   int
}

const (
	fakeBOS       = 2
	fakeTurnStart = 105
	fakeTurnEnd   = 106
	fakeTwoBreaks = 108
)

func (f *fakeReader) template(messages []chatMessage) string {
	var b strings.Builder
	b.WriteString("<bos>")
	for _, m := range messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		b.WriteString("<start_of_turn>" + role + "\n" + m.Content + "<end_of_turn>\n")
	}
	b.WriteString("<start_of_turn>model\n")
	return b.String()
}

func (f *fakeReader) tokenize(text string) []int {
	var tokens []int
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "<bos>"):
			tokens, text = append(tokens, fakeBOS), text[len("<bos>"):]
		case strings.HasPrefix(text, "<start_of_turn>"):
			tokens, text = append(tokens, fakeTurnStart), text[len("<start_of_turn>"):]
		case strings.HasPrefix(text, "<end_of_turn>"):
			tokens, text = append(tokens, fakeTurnEnd), text[len("<end_of_turn>"):]
		case strings.HasPrefix(text, "\n\n"):
			tokens, text = append(tokens, fakeTwoBreaks), text[2:]
		default:
			r := []rune(text)[0]
			tokens, text = append(tokens, 1000+int(r)), text[len(string(r)):]
		}
	}
	return tokens
}

func (f *fakeReader) serve(w http.ResponseWriter, r *http.Request) {
	var ask struct {
		Messages []chatMessage `json:"messages"`
		Content  string        `json:"content"`
		Prompt   []int         `json:"prompt"`
		Predict  *int          `json:"n_predict"`
	}
	_ = json.NewDecoder(r.Body).Decode(&ask)
	answer := func(v any) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	switch r.URL.Path {
	case "/apply-template":
		answer(map[string]any{"prompt": f.template(ask.Messages)})
	case "/tokenize":
		answer(map[string]any{"tokens": f.tokenize(ask.Content)})
	case "/completion":
		if ask.Predict == nil || *ask.Predict != 0 {
			http.Error(w, "a read writes nothing", http.StatusBadRequest)
			return
		}
		if f.stuck != nil {
			f.stuck <- struct{}{}
			<-r.Context().Done()
			return
		}
		f.mu.Lock()
		fresh := len(ask.Prompt) - commonPrefix(ask.Prompt, f.held)
		f.held = append([]int(nil), ask.Prompt...)
		f.mu.Unlock()
		f.reads <- ask.Prompt
		answer(map[string]any{"timings": map[string]any{"prompt_n": fresh}})
	case "/v1/chat/completions":
		tokens := f.tokenize(f.template(ask.Messages))
		f.mu.Lock()
		held := commonPrefix(tokens, f.held)
		// It holds the ask and what it wrote after it.
		f.held = append(append([]int(nil), tokens...), f.tokenize("1 1 1\n")...)
		f.mu.Unlock()
		f.asks <- fakeAsk{tokens, held}
		writeLocalStream(w, "1 1 1\n", 7)
	default:
		http.NotFound(w, r)
	}
}

// spokenPart is what the speaker says from from to to of a two minute video:
// a word every half second, a sentence of six, and a pause of a second
// and a bit after each, with the loudness of every frame, loud in a word
// and quiet between.
func spokenPart(from, to float64) ([]Cue, []float32) {
	var words []Cue
	at := 0.1
	for n := 1; at < 120; n++ {
		text := fmt.Sprintf("wort%d", n)
		if n%6 == 0 {
			text += "."
		}
		if at >= from && at+0.35 <= to {
			words = append(words, Cue{at, at + 0.35, text})
		}
		at += 0.5
		if n%6 == 0 {
			at += 1.3
		}
	}
	frames := make([]float32, int((to-from)/FrameSeconds+0.5))
	for i := range frames {
		frames[i] = -80
		t := from + (float64(i)+0.5)*FrameSeconds
		for _, w := range words {
			if t >= w.Start && t < w.End {
				frames[i] = -20
			}
		}
	}
	return words, frames
}

// A search asked for while its window is still being heard finds the
// transcript already read: the model reads what of it is settled as it
// is heard, each read the start of the search's own prompt token for
// token, and the search reads only the rest. Reading ahead stops once
// the window is heard, before the search asks. Issue #242.
func TestTheTranscriptIsReadAheadOfTheSearch(t *testing.T) {
	t.Parallel()
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	p, hear, ask := readingAhead(t, fake)
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warmed := make(chan ReadAhead, 1)
	go func() {
		ahead, err := p.WarmModel(ctx, req)
		if err != nil {
			t.Error(err)
		}
		warmed <- ahead
	}()

	// Each part heard is read as soon as it is written.
	hear(0, 20)
	first := <-fake.reads
	hear(20, 40)
	second := <-fake.reads
	if len(second) <= len(first) || commonPrefix(first, second) != len(first) {
		t.Fatalf("the second read, %d tokens, does not go on from the first, %d tokens: they share %d",
			len(second), len(first), commonPrefix(first, second))
	}

	// The window is heard, and reading ahead ends without another read.
	hear(40, 60)
	ahead := <-warmed
	select {
	case more := <-fake.reads:
		t.Fatalf("a read of %d tokens after the window was heard", len(more))
	default:
	}
	if ahead.Reads != 2 || ahead.Tokens != len(second) {
		t.Errorf("the reads ahead are said to be %+v, want 2 reads holding %d tokens", ahead, len(second))
	}

	// The search asks about the whole window, the way BuildPlan asks.
	ask(ctx, req)
	search := <-fake.asks
	if search.held != len(second) {
		t.Errorf("the search found %d tokens of its %d held, want the %d read ahead", search.held,
			len(search.tokens), len(second))
	}
}

// readingAhead is a video in memory whose search asks fake, how to hear a
// part of it, and how its search asks about its window, the way BuildPlan
// asks about it.
func readingAhead(t *testing.T, fake *fakeReader) (*Project, func(from, to float64),
	func(ctx context.Context, req PlanRequest)) {
	t.Helper()
	files := afero.NewMemMapFs()
	source := "/videos/episode.mp4"
	if err := afero.WriteFile(files, source, []byte("the video"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)

	e := NewEngine(NewLog(io.Discard, false, false), files)
	base := DefaultOptions()
	base.Planner, base.LLMURL, base.ASRModel = "local", server.URL, "/models/parakeet"
	p := NewProject(e, source, base)
	stamp, err := stampOf(files, source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.LogsDir(), TranscriptName(nil))
	hear := func(from, to float64) {
		t.Helper()
		words, frames := spokenPart(from, to)
		if err := addHeard(files, path, stamp, "parakeet", 120, Window{from, to}, words, frames, from); err != nil {
			t.Fatal(err)
		}
	}
	ask := func(ctx context.Context, req PlanRequest) {
		t.Helper()
		window := inMilliseconds(&Window{req.From, req.To})
		opts, _ := p.planOptions(req, window)
		opts = opts.forSpan(*window)
		heard, ok := sliceTranscript(files, path, stamp, "parakeet", *window, nil)
		if !ok {
			t.Fatal("the window's transcript cannot be read")
		}
		lines := BuildLines(heard.Words, heard.Levels(), opts.MaxPause)
		asked := askedWith(opts)
		recipe, units, prompt := searchRequest(lines, asked)
		local, err := resolveLocal(opts)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.CallLocal(ctx, *local, recipe, prompt, len(units), asked.Count, asked.MaxTokens,
			"", nil); err != nil {
			t.Fatal(err)
		}
	}
	return p, hear, ask
}

// A search that asks while the model is still reading ahead calls the
// read off, so it waits for nothing, and nothing is read after it: a read
// after the ask would take the server back to the start of the search's
// prompt. A read that is not called off never ends here, and the test
// with it.
func TestASearchCallsOffWhatIsReadAhead(t *testing.T) {
	t.Parallel()
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4),
		stuck: make(chan struct{}, 4)}
	p, hear, ask := readingAhead(t, fake)
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warmed := make(chan ReadAhead, 1)
	go func() {
		ahead, err := p.WarmModel(ctx, req)
		if err != nil {
			t.Error(err)
		}
		warmed <- ahead
	}()

	hear(0, 20)
	<-fake.stuck
	hear(20, 60)
	ask(ctx, req)
	<-fake.asks
	// Reading ahead has ended, though the window was heard only after
	// the read began.
	if ahead := <-warmed; ahead.Reads != 0 {
		t.Errorf("a read that was called off is counted: %+v", ahead)
	}
	select {
	case <-fake.stuck:
		t.Error("the model was given more to read after the search asked")
	default:
	}
}

// A search that ends without asking, a saved answer or a failure while
// it hears, ends what is read ahead of it too, which otherwise waits for
// a transcript that may never grow again.
func TestReadingAheadEndsWithTheSearch(t *testing.T) {
	t.Parallel()
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	p, hear, _ := readingAhead(t, fake)
	warmed := make(chan ReadAhead, 1)
	go func() {
		ahead, err := p.WarmModel(context.Background(), PlanRequest{From: 0, To: 60, Count: 1, Pass: 1})
		if err != nil {
			t.Error(err)
		}
		warmed <- ahead
	}()
	hear(0, 20)
	<-fake.reads
	p.engine.ahead.close()
	// It returns, though the window is not heard.
	<-warmed
}
