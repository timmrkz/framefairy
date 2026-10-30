package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// fakeReader is a llama-server in miniature that lays a prompt out and
// splits it into tokens the way b11430 does, and keeps what it read the
// way one slot keeps it. The template is Gemma's, and like the real
// server it leaves the template's BOS out of the prompt it lays out and
// puts it back when it splits with special tokens added. A token is a
// character, but two line breaks are one token, so where a prompt is cut
// changes how its end is split, as it does with a real vocabulary.
type fakeReader struct {
	mu   sync.Mutex
	held []int
	// reads are the reads ahead, and asks the searches' asks, as they come.
	reads chan []int
	asks  chan fakeAsk
	// stuck, when set, is told of a read ahead, which then reads until it
	// is called off, and calledOff is told when it is.
	stuck     chan struct{}
	calledOff chan struct{}
	// requests counts every call.
	requests int
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
	// The server takes the template's BOS off and adds its own when it
	// splits the prompt, common/chat.cpp.
	return strings.TrimPrefix(b.String(), "<bos>")
}

// tokenize splits text, with the BOS in front when special tokens are
// added, and the turns read as one token each only when special tokens
// are parsed. Otherwise they are text, character by character.
func (f *fakeReader) tokenize(text string, addSpecial, parseSpecial bool) []int {
	var tokens []int
	if addSpecial {
		tokens = append(tokens, fakeBOS)
	}
	for len(text) > 0 {
		switch {
		case parseSpecial && strings.HasPrefix(text, "<bos>"):
			tokens, text = append(tokens, fakeBOS), text[len("<bos>"):]
		case parseSpecial && strings.HasPrefix(text, "<start_of_turn>"):
			tokens, text = append(tokens, fakeTurnStart), text[len("<start_of_turn>"):]
		case parseSpecial && strings.HasPrefix(text, "<end_of_turn>"):
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
	f.mu.Lock()
	f.requests++
	f.mu.Unlock()
	var ask struct {
		Messages     []chatMessage `json:"messages"`
		Content      string        `json:"content"`
		AddSpecial   bool          `json:"add_special"`
		ParseSpecial bool          `json:"parse_special"`
		Prompt       []int         `json:"prompt"`
		Predict      *int          `json:"n_predict"`
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
		answer(map[string]any{"tokens": f.tokenize(ask.Content, ask.AddSpecial, ask.ParseSpecial)})
	case "/completion":
		if ask.Predict == nil || *ask.Predict != 0 {
			http.Error(w, "a read writes nothing", http.StatusBadRequest)
			return
		}
		if f.stuck != nil {
			f.stuck <- struct{}{}
			<-r.Context().Done()
			if f.calledOff != nil {
				f.calledOff <- struct{}{}
			}
			return
		}
		f.mu.Lock()
		fresh := len(ask.Prompt) - commonPrefix(ask.Prompt, f.held)
		f.held = append([]int(nil), ask.Prompt...)
		f.mu.Unlock()
		f.reads <- ask.Prompt
		answer(map[string]any{"timings": map[string]any{"prompt_n": fresh}})
	case "/v1/chat/completions":
		// The chat path splits with special tokens added and parsed.
		tokens := f.tokenize(f.template(ask.Messages), true, true)
		f.mu.Lock()
		held := commonPrefix(tokens, f.held)
		f.held = tokens
		f.mu.Unlock()
		f.asks <- fakeAsk{tokens, held}
		// What it is asked is all a test looks at, so it answers nothing
		// that would have to be framed.
		w.WriteHeader(http.StatusServiceUnavailable)
		answer(map[string]any{"error": map[string]any{"message": "the test has its ask"}})
	default:
		http.NotFound(w, r)
	}
}

// spokenPart is what the speaker says from from to to of a two minute video:
// a word every half second, a sentence of six, and a pause of a second
// and a bit after each, with the loudness of every frame, loud in a word
// and quiet between.
func spokenPart(from, to float64) ([]timed.Cue, []float32) {
	var words []timed.Cue
	at := 0.1
	for n := 1; at < 120; n++ {
		text := fmt.Sprintf("wort%d", n)
		if n%6 == 0 {
			text += "."
		}
		if at >= from && at+0.35 <= to {
			words = append(words, timed.Cue{Start: at, End: at + 0.35, Text: text})
		}
		at += 0.5
		if n%6 == 0 {
			at += 1.3
		}
	}
	frames := make([]float32, int((to-from)/timed.FrameSeconds+0.5))
	for i := range frames {
		frames[i] = -80
		t := from + (float64(i)+0.5)*timed.FrameSeconds
		for _, w := range words {
			if t >= w.Start && t < w.End {
				frames[i] = -20
			}
		}
	}
	return words, frames
}

// readAheadOf is a video in memory whose search asks fake, read ahead of
// by its own engine the way the app reads ahead of a search.
type readAheadOf struct {
	t     *testing.T
	p     *Project
	local LocalModel
	files afero.Fs
	path  string
	stamp sourceStamp
	// speak is what is heard of a part, spokenPart unless a test says
	// otherwise.
	speak func(from, to float64) ([]timed.Cue, []float32)
}

// hostedFake has the model this program loads be fake, so what is read
// ahead goes through the one model the program holds, as in the app. It
// changes launch and the model host, which the whole package shares, so
// a test that uses it does not run beside others.
func hostedFake(t *testing.T, fake *fakeReader) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	was := launch
	launch = func(_ *Engine, _ context.Context, _ LocalModel, _ int, _ string) (string, func(), error) {
		return server.URL, func() {}, nil
	}
	StopModels()
	t.Cleanup(func() {
		StopModels()
		launch = was
		server.Close()
	})
}

// videoReadAhead is a video named name in files, searched with the
// hosted fake.
func videoReadAhead(t *testing.T, files afero.Fs, name string) *readAheadOf {
	t.Helper()
	source := "/videos/" + name + ".mp4"
	if err := afero.WriteFile(files, source, []byte("the video"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(NewLog(io.Discard, false, false), files)
	base := DefaultOptions()
	base.Planner, base.LLMModel, base.ASRModel = "local", "/models/gemma.gguf", "/models/parakeet"
	p := NewProject(e, source, base)
	stamp, err := stampOf(files, source)
	if err != nil {
		t.Fatal(err)
	}
	return &readAheadOf{t: t, p: p, local: LocalModel{Model: base.LLMModel}, files: files,
		path: filepath.Join(p.LogsDir(), TranscriptName(nil)), stamp: stamp, speak: spokenPart}
}

// hear writes the part from from to to into the transcript, the way the
// speech model writes each part it has heard.
func (v *readAheadOf) hear(from, to float64) {
	v.t.Helper()
	words, frames := v.speak(from, to)
	if err := addHeard(v.files, v.path, v.stamp, "parakeet", 120, timed.Window{Start: from, End: to}, words, frames,
		from); err != nil {
		v.t.Fatal(err)
	}
}

// warm reads ahead of a search of req until it is over, and sends what
// was read.
func (v *readAheadOf) warm(ctx context.Context, req PlanRequest) <-chan ReadAhead {
	warmed := make(chan ReadAhead, 1)
	go func() {
		ahead, err := v.p.warm(ctx, req, v.local)
		if err != nil {
			v.t.Error(err)
		}
		warmed <- ahead
	}()
	return warmed
}

// ask is the search's ask about the window of req once it is heard: its
// lines as the search loads them, asked through BuildPlan, the way
// runner.plan asks. The fake turns the ask down once it has it.
func (v *readAheadOf) ask(ctx context.Context, req PlanRequest) error {
	v.t.Helper()
	window := inMilliseconds(&timed.Window{Start: req.From, End: req.To})
	opts, _ := v.p.planOptions(req, window)
	opts = opts.forSpan(*window)
	e := v.p.engine
	transcript, err := e.LoadTranscript(ctx, v.p.Source, *window, 120, v.p.LogsDir(), speechModel(opts),
		opts.SilenceDB)
	if err != nil {
		v.t.Fatal(err)
	}
	asked := askedWith(opts)
	asked.Model, asked.Local, asked.LogDir, asked.Window = plannerName(opts), &v.local, v.p.LogsDir(), window
	_, err = e.BuildPlan(ctx, v.p.Source, SourceInfo{Width: 1920, Height: 1080, Duration: 120},
		searchLines(transcript, opts), asked)
	return err
}

// A search asked for while its window is still being heard finds the
// transcript already read: the model reads what of it is settled as it
// is heard, each read the start of the search's own prompt token for
// token, and the search reads only the rest. Reading ahead stops once
// the window is heard, before the search asks. Issue #242.
func TestTheTranscriptIsReadAheadOfTheSearch(t *testing.T) {
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	hostedFake(t, fake)
	v := videoReadAhead(t, afero.NewMemMapFs(), t.Name())
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warmed := v.warm(ctx, req)

	// Each part heard is read as soon as it is written.
	read := func() []int {
		t.Helper()
		select {
		case tokens := <-fake.reads:
			return tokens
		case ahead := <-warmed:
			t.Fatalf("reading ahead ended early, with %+v", ahead)
			return nil
		}
	}
	v.hear(0, 20)
	first := read()
	if len(first) == 0 || first[0] != fakeBOS || commonPrefix(first[1:], []int{fakeBOS}) != 0 {
		t.Fatalf("the first read does not start with one BOS: %v", first[:min(len(first), 4)])
	}
	v.hear(20, 40)
	second := read()
	if len(second) <= len(first) || commonPrefix(first, second) != len(first) {
		t.Fatalf("the second read, %d tokens, does not go on from the first, %d tokens: they share %d",
			len(second), len(first), commonPrefix(first, second))
	}

	// The window is heard, and reading ahead ends without another read.
	v.hear(40, 60)
	ahead := <-warmed
	select {
	case more := <-fake.reads:
		t.Fatalf("a read of %d tokens after the window was heard", len(more))
	default:
	}
	if ahead.Reads != 2 || ahead.Tokens != len(second) || ahead.Rewritten != 0 {
		t.Errorf("the reads ahead are said to be %+v, want 2 reads holding %d tokens", ahead, len(second))
	}

	// The search asks about the whole window.
	_ = v.ask(ctx, req)
	search := <-fake.asks
	if search.held != len(second) {
		t.Errorf("the search found %d tokens of its %d held, want the %d read ahead", search.held,
			len(search.tokens), len(second))
	}
}

// A part heard later can write a line read before out differently: the
// loudness a pause is told from is the mean of all that is heard, so a
// quieter part moves it, and with it where a word after a pause starts,
// and the pause the request marks. The read after it says so, reads from
// where the two part, and the search still finds the last read held.
// Issue #437 is about keeping the line as it was.
func TestALineReadBeforeAndWrittenOutAgainIsReadAgain(t *testing.T) {
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	hostedFake(t, fake)
	v := videoReadAhead(t, afero.NewMemMapFs(), t.Name())
	v.speak = func(from, to float64) ([]timed.Cue, []float32) {
		words, frames := spokenPart(from, to)
		if from < 20 {
			// Loud, in a room with a hum, and the word after each pause
			// stamped before its sound begins.
			for i := 1; i < len(words); i++ {
				if endsSentence(words[i-1].Text) {
					words[i].Start -= 0.55
				}
			}
			for i, f := range frames {
				if f == -80 {
					frames[i] = -45
				}
			}
			return words, frames
		}
		// Quiet, so the mean falls and the hum of the first part is
		// heard as sound.
		for i, f := range frames {
			if f == -20 {
				frames[i] = -50
			}
		}
		return words, frames
	}
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warmed := v.warm(ctx, req)

	v.hear(0, 20)
	first := <-fake.reads
	v.hear(20, 40)
	second := <-fake.reads
	if commonPrefix(first, second) >= len(first) {
		t.Fatalf("the quiet part changed nothing read before, so this test proves nothing")
	}
	v.hear(40, 60)
	ahead := <-warmed
	if ahead.Rewritten != 1 || ahead.Read <= ahead.Tokens || ahead.Tokens != len(second) {
		t.Errorf("the reads ahead are said to be %+v, want one written out again, more read than "+
			"held, and %d held", ahead, len(second))
	}
	_ = v.ask(ctx, req)
	if search := <-fake.asks; search.held != len(second) {
		t.Errorf("the search found %d tokens of its %d held, want the %d read last", search.held,
			len(search.tokens), len(second))
	}
}

// A search that asks while the model is still reading ahead calls the
// read off, so it waits for nothing, and nothing is read after it: a read
// after the ask would take the server back to the start of the search's
// prompt. A read that is not called off never ends here, and the test
// with it.
func TestASearchCallsOffWhatIsReadAhead(t *testing.T) {
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4),
		stuck: make(chan struct{}, 4)}
	hostedFake(t, fake)
	v := videoReadAhead(t, afero.NewMemMapFs(), t.Name())
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	warmed := v.warm(ctx, req)

	v.hear(0, 20)
	<-fake.stuck
	v.hear(20, 60)
	_ = v.ask(ctx, req)
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

// A search of another video that wants the model while it reads ahead of
// this video's search has it at once: the read is called off rather than
// held up for, and this video goes on being read ahead of. A read that is
// not called off never ends here, and the test with it.
func TestASearchOfAnotherVideoTakesTheModelFromARead(t *testing.T) {
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4),
		stuck: make(chan struct{}, 4), calledOff: make(chan struct{}, 4)}
	hostedFake(t, fake)
	files := afero.NewMemMapFs()
	reading := videoReadAhead(t, files, t.Name()+"-reading")
	asking := videoReadAhead(t, files, t.Name()+"-asking")
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	asking.hear(0, 60)
	warmed := reading.warm(ctx, req)
	reading.hear(0, 20)
	<-fake.stuck

	asked := make(chan error, 1)
	go func() { asked <- asking.ask(ctx, req) }()
	<-fake.calledOff
	<-fake.asks
	<-asked
	// This video's reading ahead goes on until its window is heard.
	select {
	case ahead := <-warmed:
		t.Fatalf("reading ahead ended with the other video's search: %+v", ahead)
	default:
	}
	reading.hear(20, 60)
	if ahead := <-warmed; ahead.Reads != 0 {
		t.Errorf("a read that was called off is counted: %+v", ahead)
	}
}

// A search that ends without asking, cancelled here, ends what is read
// ahead of it too, which otherwise waits for a transcript that may never
// grow again.
func TestReadingAheadEndsWithTheSearch(t *testing.T) {
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	hostedFake(t, fake)
	v := videoReadAhead(t, afero.NewMemMapFs(), t.Name())
	req := PlanRequest{From: 0, To: 60, Count: 1, Pass: 1}
	warmed := v.warm(context.Background(), req)
	v.hear(0, 20)
	<-fake.reads
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.p.Search(cancelled, req, nil); !errors.Is(err, ErrCancelled) {
		t.Fatalf("the cancelled search says %v", err)
	}
	// It returns, though the window is not heard.
	<-warmed
}

// A server that runs already, --llm-url, is left alone until the search
// asks, as it always was: it may be far away, or serving others.
func TestNothingIsReadAheadIntoARunningServer(t *testing.T) {
	t.Parallel()
	fake := &fakeReader{reads: make(chan []int, 16), asks: make(chan fakeAsk, 4)}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	defer server.Close()
	files := afero.NewMemMapFs()
	if err := afero.WriteFile(files, "/videos/running.mp4", []byte("the video"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := DefaultOptions()
	base.Planner, base.LLMURL = "local", server.URL
	p := NewProject(NewEngine(NewLog(io.Discard, false, false), files), "/videos/running.mp4", base)
	ahead, err := p.WarmModel(context.Background(), PlanRequest{From: 0, To: 60, Count: 1, Pass: 1})
	if err != nil || ahead != (ReadAhead{}) {
		t.Errorf("warming a running server read %+v and said %v", ahead, err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.requests != 0 {
		t.Errorf("the running server was called %d times", fake.requests)
	}
}

// Only a request that starts with the transcript is read ahead of, which
// of the recipes there are is middle, the one a search asks with. The
// heart recipes say first how many lines there are, and lines writes
// each line out by how loud it is beside all the others, so their
// requests change from the start with every line heard. The others are
// experiments, which are not read ahead of.
func TestOnlyARequestThatStartsWithTheTranscriptIsReadAhead(t *testing.T) {
	t.Parallel()
	words, _ := spokenPart(0, 30)
	lines := BuildLines(words, nil, nil)
	for _, recipe := range RecipeNames() {
		want := recipe == DefaultRecipe
		if got := readsAhead(lines, PlanOptions{Recipe: recipe, Count: 1, MinLen: 20, MaxLen: 30}); got != want {
			t.Errorf("the %s recipe is read ahead: %v, want %v", recipe, got, want)
		}
	}
}
