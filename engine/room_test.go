package engine

import (
	"fmt"
	"strings"
	"testing"
)

func TestLocalContextIsWhatTheMakerPublishes(t *testing.T) {
	for name, want := range map[string]int{
		"/models/gemma-4-26B_q4_0-it.gguf":                 262144,
		"Qwen3-14B-Q4_K_M.gguf":                            40960,
		"gemma-4-12b-it-qat-q4_0.gguf":                     262144,
		"Ministral-3-8B-Instruct-2512-Q4_K_M.gguf":         262144,
		"/somewhere/else/a-model-nobody-has-heard-of.gguf": unknownContext,
		"": unknownContext,
	} {
		if got := LocalContext(name); got != want {
			t.Errorf("%q holds %d, want %d", name, got, want)
		}
	}
	for _, m := range LanguageModels() {
		if m.Context <= 0 {
			t.Errorf("%s says nothing about what it can read", m.Name)
		}
	}
}

// machine makes the tests a machine with this much memory, and puts the
// real one back after.
func machine(t *testing.T, total int64) {
	was := machineMemory
	machineMemory = func() int64 { return total }
	t.Cleanup(func() { machineMemory = was })
}

func roomOf(opts PlanOptions) Room {
	if opts.Count == 0 {
		opts.Count, opts.MinLen, opts.MaxLen = 12, 20, 30
	}
	if opts.MaxTokens == 0 {
		opts.MaxTokens = 48000
	}
	return planRoom(opts)
}

func TestTheRoomOfALocalModelIsItsContextLessTheAnswer(t *testing.T) {
	machine(t, 128<<30)
	around := runeLen(SystemPrompt) + runeLen(buildPrompt(nil, PlanOptions{Count: 12, MinLen: 20, MaxLen: 30}))
	for model, tokens := range map[string]int{
		"gemma-4-26B_q4_0-it.gguf": 262144 - 16384 - 2048,
		"Qwen3-14B-Q4_K_M.gguf":    40960 - 16384 - 2048,
	} {
		room := roomOf(PlanOptions{Local: &LocalModel{Model: model}})
		if room.Tokens != tokens || room.By != "context" {
			t.Errorf("%s: %+v, want %d tokens by context", model, room, tokens)
		}
		if want := int(float64(tokens)*localCharsPerToken) - around; room.Chars != want {
			t.Errorf("%s: %d characters, want %d", model, room.Chars, want)
		}
	}
	// A room the engine sizes the context by has to fit the context it
	// sizes: the most a window may send never needs more than the model
	// holds.
	room := roomOf(PlanOptions{Local: &LocalModel{Model: "gemma-4-26B_q4_0-it.gguf"}})
	if size := contextFor(room.Chars, 48000); size > 262144 {
		t.Errorf("a full window asks for a context of %d", size)
	}
	// A server somebody started themselves holds what they started it with,
	// which the engine cannot know, so it does not stand in the way.
	if room := roomOf(PlanOptions{Local: &LocalModel{URL: "http://127.0.0.1:8080"}}); room.Tokens != 262144-18432 {
		t.Errorf("a running server: %+v", room)
	}
}

func TestTheRoomOfTheAPIIsTheContextOrTheBudget(t *testing.T) {
	cases := []struct {
		model  string
		budget float64
		tokens int
		by     string
	}{
		// A million tokens, less twice the answer for the retry.
		{"claude-sonnet-5", 2, 1_000_000 - 96_000, "context"},
		// The answer can be no longer than the model writes.
		{"claude-haiku-4-5-20251001", 2, 200_000 - 64_000, "context"},
		// Five dollars a million in: two dollars less the usual answer at
		// twenty five buys 330,000.
		{"claude-opus-5", 2, 330_000, "budget"},
		{"claude-fable-5-1", 2, 130_000, "budget"},
		// A budget the answer alone eats has no room left to read.
		{"claude-fable-5-1", 0.5, 0, "budget"},
	}
	for _, c := range cases {
		room := roomOf(PlanOptions{Model: c.model, Budget: c.budget})
		if room.Tokens != c.tokens || room.By != c.by {
			t.Errorf("%s at $%v: %+v, want %d by %s", c.model, c.budget, room, c.tokens, c.by)
		}
	}
}

// A transcript of n lines, each a sentence said over three seconds with a
// pause after it, so every line is a line of its own.
func talk(n int) *Transcript {
	var words []Cue
	for i := range n {
		at := float64(i) * 4
		words = append(words, Cue{at, at + 1.4, "Ich"}, Cue{at + 1.5, at + 3, fmt.Sprintf("erzähle %d.", i)})
	}
	return &Transcript{Words: words}
}

func TestAWindowTheModelCannotReadIsRefusedBeforeAnythingIsSent(t *testing.T) {
	lines := BuildLines(talk(4000).Words, nil, nil)
	opts := PlanOptions{Count: 12, MinLen: 20, MaxLen: 30, MaxTokens: 48000,
		Local: &LocalModel{Model: "Qwen3-14B-Q4_K_M.gguf"}}
	err := windowFits(lines, opts)
	if err == nil || !strings.Contains(err.Error(), "Qwen3-14B-Q4_K_M.gguf reads at most") {
		t.Fatalf("a window four times too long: %v", err)
	}
	if err := windowFits(lines[:100], opts); err != nil {
		t.Errorf("a short window: %v", err)
	}
	opts.Local, opts.Model, opts.Budget = nil, "claude-opus-5", 0.01
	if err := windowFits(lines[:100], opts); err == nil || !strings.Contains(err.Error(), "--budget") {
		t.Errorf("a budget that buys nothing: %v", err)
	}
}

func TestAWindowHoldsItsClipsAtTheirShortest(t *testing.T) {
	half := Window{0, 1800}
	if !Holds(half, 12, 20) || !Holds(half, 90, 20) || Holds(half, 91, 20) {
		t.Error("half an hour holds 90 clips of 20 seconds and no more")
	}
	if !Holds(Window{10, 30}, 2, 10) {
		t.Error("a window exactly as long as its clips holds them")
	}
	if Holds(Window{10, 30}, 2, 10.5) {
		t.Error("a window a second short holds them")
	}
}

// What the app adds up over a window has to be at least what the window
// really sends, wherever the window starts, or a window the range picker
// lets through is one the engine refuses.
func TestTheWeightOfAWindowIsNeverLessThanWhatItSends(t *testing.T) {
	tr := talk(1200)
	weights := WeighLines(tr)
	lines := BuildLines(tr.Words, nil, nil)
	if len(weights) != len(lines) {
		t.Fatalf("%d weights for %d lines", len(weights), len(lines))
	}
	for _, span := range [][2]int{{0, 1200}, {0, 9}, {3, 12}, {95, 105}, {990, 1200}, {500, 501}} {
		part := BuildLines(tr.Words[span[0]*2:span[1]*2], nil, nil)
		sent := runeLen(AnnotateLines(part))
		counted := 0
		for _, w := range weights[span[0]:span[1]] {
			counted += w.Chars
		}
		if counted < sent {
			t.Errorf("lines %d to %d: counted %d, sends %d", span[0], span[1], counted, sent)
		}
		// A line is counted with the widest number it could have, and the
		// first line of a window sends no pause before it, so the count is
		// a little over. A little.
		if counted > sent+4*(span[1]-span[0])+20 {
			t.Errorf("lines %d to %d: counted %d, far over the %d it sends", span[0], span[1], counted, sent)
		}
	}
	if len(WeighLines(&Transcript{})) != 0 {
		t.Error("no words, and still something to weigh")
	}
}

// What has not been heard yet is weighed at the episode's own rate once
// there is enough of it, and never lighter than the starting point.
func TestTheRateOfWhatIsNotHeardYet(t *testing.T) {
	if RateOf(nil) != SpokenChars {
		t.Error("no lines, and a rate of their own")
	}
	short := []LineWeight{{0, 3, 900}, {4, 300, 900}}
	if RateOf(short) != SpokenChars {
		t.Error("five minutes decide the rate")
	}
	dense := []LineWeight{{0, 3, 20000}, {4, 1000, 20000}}
	if got := RateOf(dense); got < 45.99 || got > 46.01 {
		t.Errorf("a dense episode weighs %v a second, want 40 and 15 percent", got)
	}
	light := []LineWeight{{0, 3, 100}, {4, 1000, 100}}
	if RateOf(light) != SpokenChars {
		t.Error("a light episode weighs less than the starting point")
	}
}

func TestAWindowNeverAsksForMoreMemoryThanTheMachineHas(t *testing.T) {
	for _, m := range LanguageModels() {
		for _, total := range []int64{8 << 30, 16 << 30, 24 << 30, 32 << 30, 64 << 30} {
			got := MemoryContext(m, total)
			fine := func(ctx int) bool {
				needs := m.NeedsAt(ctx)
				return needs+memoryHeadroom <= total || (ctx <= judgedContext && needs <= total)
			}
			if got > 0 && !fine(got) {
				t.Errorf("%s on %d GiB: %d tokens take %d bytes", m.Name, total>>30, got, m.NeedsAt(got))
			}
			if got < contextCeiling && fine(got+1) {
				t.Errorf("%s on %d GiB: %d tokens, and one more would fit", m.Name, total>>30, got)
			}
		}
	}
	gemma, _ := LanguageModelByName("gemma-4-26B_q4_0-it.gguf")
	// Tim's machine: all of it, with the headroom left.
	if got := MemoryContext(gemma, 32<<30); got != contextCeiling {
		t.Errorf("Gemma 4 26B on 32 GiB holds %d", got)
	}
	if got := MemoryContext(gemma, 8<<30); got != 0 {
		t.Errorf("Gemma 4 26B on 8 GiB holds %d", got)
	}
	if got := MemoryContext(gemma, 0); got != judgedContext {
		t.Errorf("a machine that does not say: %d", got)
	}
	// A model offered as tight gets what it was offered for.
	ministral, _ := LanguageModelByName("Ministral-3-8B-Instruct-2512-Q4_K_M.gguf")
	if got := MemoryContext(ministral, 16<<30); got != judgedContext {
		t.Errorf("Ministral 3 8B on 16 GiB holds %d", got)
	}
}

func TestTheRoomOnASmallMachineIsItsMemory(t *testing.T) {
	machine(t, 16<<30)
	room := roomOf(PlanOptions{Local: &LocalModel{Model: "/m/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf"}})
	if room.By != "memory" || room.Tokens != judgedContext-localAnswerRoom(48000) {
		t.Errorf("Ministral 3 8B on 16 GiB: %+v", room)
	}
	// And the context it is started with is never more than was checked,
	// however contextFor rounds.
	if size := localContextFor("Ministral-3-8B-Instruct-2512-Q4_K_M.gguf", room.Chars, 48000); size > judgedContext {
		t.Errorf("started with %d", size)
	}
	if size := localContextFor("Ministral-3-8B-Instruct-2512-Q4_K_M.gguf", 1_000_000, 48000); size > judgedContext {
		t.Errorf("a prompt too long for the room starts it with %d", size)
	}
	machine(t, 4<<30)
	if size := localContextFor("gemma-4-26B_q4_0-it.gguf", 1000, 48000); size != 4096 {
		t.Errorf("a model with no room at all is started with %d", size)
	}
}
