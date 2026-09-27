package engine

import (
	"math"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// How much one search can read
//
// A search sends the transcript of its window to the model in one request,
// and one request can only hold so much. That is a number of tokens, set by
// the model and by what the answer needs, not a number of minutes. So the
// room is worked out here, from the model that is going to be asked, and
// the app turns it into the longest window the range picker lets you draw.
// A window that fits is sent as it is, in one request, with nothing split
// behind anybody's back. A window that does not fit is not drawn.
//
// Characters are the unit, because they can be counted before anything is
// sent and tokens cannot. Each way of planning has its own rate between
// the two, and the rate is the one the rest of the engine already goes by.
// ---------------------------------------------------------------------------

// Room is how much numbered transcript one search can send.
type Room struct {
	// Chars is the most characters of numbered transcript, as the model
	// reads it, one request can carry beside everything else it sends.
	Chars int `json:"chars"`
	// Tokens is the same in tokens, which is what the limit is.
	Tokens int `json:"tokens"`
	// By is what sets it: "context" for what the model can hold at once,
	// "memory" for what this machine can hold it with, "budget" for what
	// one search may cost.
	By string `json:"by"`
}

// localCharsPerToken is how many characters of German make a token for the
// local models. It is the rate the model's context is sized by as well, so
// what the room lets through is what the context is made for.
const localCharsPerToken = 2.5

// contextCeiling is the largest context llama-server is started with.
const contextCeiling = 262144

// unknownContext is what a local model this engine does not know is taken
// to hold. It is below the smallest any of the models it does know holds,
// so a window drawn for it is one every model can read.
const unknownContext = 32768

// localAnswerRoom is what a local search keeps free in the context for the
// answer, the thinking before it included. The thinking is written inside
// the answer's allowance, so it takes nothing on top.
func localAnswerRoom(maxTokens int) int {
	return min(maxTokens, 16384) + 2048
}

// LocalContext is the most tokens a local model can hold at once: what its
// file says it was made to hold, and never more than the context the engine
// starts it with. A model file this engine does not know is taken at less
// than the smallest of the ones it does.
func LocalContext(model string) int {
	name := filepath.Base(model)
	for _, m := range LanguageModels() {
		if strings.EqualFold(m.Name, name) && m.Context > 0 {
			return min(m.Context, contextCeiling)
		}
	}
	return unknownContext
}

// machineMemory is how much memory this machine has. It is a variable so
// a test can be any machine.
var machineMemory = MachineMemory

// MemoryContext is the largest context a search with this model can have
// on a machine with this much memory, by the rule the app offers models
// by, see FitsIn. A context that leaves the headroom free is always fine.
// A model that only fits tight was offered for a search of judgedContext,
// so that much it may have, and no more. A machine that does not say how
// much it has gets judgedContext too: nothing is promised, and nothing
// larger than what every model was judged at is asked of it.
func MemoryContext(m LanguageModel, total int64) int {
	if total <= 0 {
		return judgedContext
	}
	well := largestContext(func(ctx int) bool { return m.NeedsAt(ctx)+memoryHeadroom <= total })
	tight := min(largestContext(func(ctx int) bool { return m.NeedsAt(ctx) <= total }), judgedContext)
	return max(well, tight)
}

// largestContext is the largest context up to contextCeiling that fits,
// or zero when none does. What a context takes only grows with it.
func largestContext(fits func(int) bool) int {
	if !fits(0) {
		return 0
	}
	lo, hi := 0, contextCeiling
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// localHeld is the most tokens a search with this local model can have on
// this machine, and what sets it: "context" for what the model holds,
// "memory" for what the machine does. A model the engine does not know the
// shape of is held by its context alone, which for such a model is small.
func localHeld(model string) (int, string) {
	held := LocalContext(model)
	m, known := LanguageModelByName(filepath.Base(model))
	if !known {
		return held, "context"
	}
	if fits := MemoryContext(m, machineMemory()); fits < held {
		return fits, "memory"
	}
	return held, "context"
}

// localContextFor is the context a local model is started with for a
// prompt this long: what contextFor asks for, never more than the model
// holds or the machine has memory for. The room keeps every prompt inside
// both, so this never cuts a prompt short. It keeps a model from being
// started with a context it was not made for, and the rounding up in
// contextFor from asking the machine for memory the room never checked.
func localContextFor(model string, promptChars, maxTokens int) int {
	held, _ := localHeld(model)
	// Never nothing: llama-server takes a context of 0 to mean the whole
	// of what the model holds, which is the very memory this keeps it
	// from asking for. A model that has no room at all is refused before
	// it is started, and this only catches it being loaded ahead of that.
	return max(min(contextFor(promptChars, maxTokens), held), 4096)
}

// SearchRoom is the room a search made with these options has. It is what
// the app asks before a window is drawn, and the same sum BuildPlan checks
// a window against before anything is sent.
func SearchRoom(opts Options, logDir string) Room {
	plan := PlanOptions{Count: opts.Count, MinLen: opts.Min, MaxLen: opts.Max,
		Context: opts.Context, Model: opts.Model, MaxTokens: opts.MaxTokens,
		Budget: opts.Budget, LogDir: logDir}
	if opts.Planner != "api" {
		model := opts.LLMModel
		if model == "" && opts.LLMURL == "" {
			model, _ = DefaultLocalModel()
		}
		plan.Local = &LocalModel{Model: model, URL: opts.LLMURL}
	}
	return planRoom(plan)
}

// planRoom works the room out. Whatever the request sends besides the
// transcript, the instructions and the ask, comes off the top.
func planRoom(opts PlanOptions) Room {
	recipe := opts.recipe()
	around := runeLen(recipe.System) + runeLen(recipe.Request(nil, nil, opts))
	if opts.Local != nil {
		// A server somebody started themselves has the context and the
		// memory they gave it, which the engine cannot know.
		held, by := contextCeiling, "context"
		if opts.Local.URL == "" || opts.Local.Model != "" {
			held, by = localHeld(opts.Local.Model)
		}
		tokens := max(held-localAnswerRoom(opts.MaxTokens), 0)
		return Room{Chars: charsFor(tokens, localCharsPerToken, around), Tokens: tokens, By: by}
	}

	// The API. The answer is given room for the retry as well: a model that
	// thinks its way through the whole allowance is asked again with twice
	// as much, and that second ask has to fit in the same context.
	facts := FactsFor(opts.Model)
	answer := max(min(opts.MaxTokens*2, facts.MaxOutput), opts.MaxTokens)
	room := Room{Tokens: max(facts.Context-answer, 0), By: "context"}
	perToken, output := apiRate(opts.LogDir)
	if facts.Priced {
		// The same sum BuildPlan uses for what a search is likely to cost:
		// the answer it usually writes, and whatever is left of the budget
		// for reading.
		left := opts.Budget - output/1e6*facts.PriceOut
		affordable := max(int(math.Floor(left/facts.PriceIn*1e6+1e-6)), 0)
		if affordable < room.Tokens {
			room.Tokens, room.By = affordable, "budget"
		}
	}
	room.Chars = charsFor(room.Tokens, perToken, around)
	return room
}

// apiRate is how many characters make a token through the API and how long
// an answer usually is, measured on this episode when it has been searched
// through the API before and the engine's own starting points when not.
func apiRate(logDir string) (float64, float64) {
	const sample = 1_000_000
	tokens, output, _ := estimateUsage(logDir, sample, "plan")
	return sample / float64(max(tokens, 1)), float64(output)
}

func charsFor(tokens int, perToken float64, around int) int {
	return max(int(float64(tokens)*perToken)-around, 0)
}

// Holds says whether a window has room for count clips of at least least
// seconds each, one after another. Clips can overlap, but a search that
// needs them to in order to find as many as it was asked for is asking for
// more than the window has in it. A hundredth of a second is let go, so a
// window drawn on a round step is not refused over rounding.
func Holds(w Window, count int, least float64) bool {
	return float64(count)*least <= w.End-w.Start+0.01
}

// LineWeight is one line of the transcript as the room counts it: when it is
// said, and how many characters it takes in the numbered transcript the
// model reads, its line break included.
type LineWeight struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Chars int     `json:"chars"`
}

// WeighLines is the transcript cut into the lines a search would send, each
// with what it weighs. The app adds them up over a window to know whether
// the window fits the room, without asking the engine at every step of a
// drag. The numbers in front of a line depend on where the window starts,
// so a line is counted with the widest number any window of this episode
// would give it, which only ever errs towards a window that fits.
func WeighLines(t *Transcript) []LineWeight {
	lines := BuildLines(t.Words, t.Levels(), nil)
	out := make([]LineWeight, 0, len(lines))
	widest := len(itoa(len(lines)))
	for i, row := range strings.Split(AnnotateLines(lines), "\n") {
		if i >= len(lines) {
			break
		}
		// The number this line has here, and the widest it could have.
		pad := widest - len(itoa(lines[i].Index))
		out = append(out, LineWeight{Start: lines[i].Start(), End: lines[i].End(),
			Chars: runeLen(row) + 1 + max(pad, 0)})
	}
	return out
}

// SpokenChars is how many characters of numbered transcript a second of an
// episode makes, before its own transcript can say: a half hour of German
// came to about 40,000, and this leaves room to spare.
const SpokenChars = 25.0

// RateOf is how many characters a second of this episode makes, from the
// lines it has so far, with 15 percent on top for a stretch busier than the
// average. Until there are ten minutes of them the starting point stands,
// and it never goes below it: the rate is used for what has not been heard
// yet, and guessing that lighter than it turns out is what would let a
// window through that the engine then refuses.
func RateOf(lines []LineWeight) float64 {
	if len(lines) == 0 {
		return SpokenChars
	}
	span := lines[len(lines)-1].End - lines[0].Start
	if span < 600 {
		return SpokenChars
	}
	chars := 0
	for _, l := range lines {
		chars += l.Chars
	}
	return max(float64(chars)/span*1.15, SpokenChars)
}
