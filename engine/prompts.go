package engine

import (
	"embed"
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/template"
)

// ---------------------------------------------------------------------------
// Prompts kept as text
//
// A recipe whose prompt is a file in prompts/ asks with that file, filled
// in: the transcript, how many clips, how long. Kept as text rather than as
// Go strings, a prompt reads the way the model reads it, and a change to it
// is a change to a few lines anyone can comment on in a pull request.
//
// A file is what the model is sent. Most are one message, the whole file.
// The three that came from the Go code, lines, heart and heart-opening,
// also have a system part: it comes first, after "=== system ===", and the
// message after "=== user ===". The program fills in what is between {{
// and }}: the transcript, how many clips, how long.
// ---------------------------------------------------------------------------

//go:embed prompts/*.txt
var promptFiles embed.FS

// promptData is what a prompt file is filled in with.
type promptData struct {
	Count int
	// Lines is how many lines the transcript numbers.
	Lines      int
	Min, Max   string
	Transcript string
	// Taken says which lines are in clips already, empty when none are,
	// and TakenLines only names them, as in lines 3-7 and 12.
	Taken, TakenLines string
	Context           string
	// Pause is the shortest pause the transcript marks, in words, "a
	// second" say, empty when it marks none. Times is whether each line has
	// the time it starts at. MaxWords is the longest a clip may run in
	// words, as the prompts say it, 0 when not said.
	Pause    string
	Times    bool
	MaxWords int
}

const (
	systemMark = "=== system ===\n"
	userMark   = "=== user ===\n"
)

// promptParts is the system part and the message of the prompt file of
// that name, the system part empty for a file that is one message. A file
// that is not there, or has a system part and no message, is a fault of
// the program, found by its tests.
func promptParts(name string) (system, request string) {
	text, err := promptFiles.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic(fmt.Sprintf("there is no prompt %s: %v", name, err))
	}
	body := strings.TrimSuffix(string(text), "\n")
	rest, ok := strings.CutPrefix(body, systemMark)
	if !ok {
		return "", body
	}
	system, request, ok = strings.Cut(rest, "\n"+userMark)
	if !ok {
		panic(fmt.Sprintf("the prompt %s has a system part and no message", name))
	}
	return system, request
}

// promptSystem is the system part of the prompt file of that name, empty
// for a prompt that is one message.
func promptSystem(name string) string {
	system, _ := promptParts(name)
	return system
}

// prompt fills in the request of the prompt file of that name.
func prompt(name string, lines []Line, transcript string, opts PlanOptions) string {
	_, request := promptParts(name)
	t := template.Must(template.New(name).Option("missingkey=error").Parse(request))
	data := promptData{Count: opts.Count, Lines: len(lines), Min: fixed(opts.MinLen, 0),
		Max: fixed(opts.MaxLen, 0), Transcript: transcript, Context: opts.Context}
	if taken := takenLines(lines, opts.Taken); len(taken) > 0 {
		data.Taken, data.TakenLines = takenSentence(taken), "lines "+takenNames(taken)
		if len(taken) == 1 && taken[0][0] == taken[0][1] {
			data.TakenLines = "line " + takenNames(taken)
		}
	}
	sw := opts.Switches
	data.Times = sw.Times
	switch {
	case sw.Pause == 1:
		data.Pause = "a second"
	case sw.Pause > 0:
		data.Pause = trimFloat(sw.Pause) + " seconds"
	}
	// The length is said in words, unless the lines have their times and
	// it is said in seconds, and only the longest, see wordsShare. A
	// window too short to tell how fast its speaker talks says no words.
	if rate := wordsPerSecond(lines); !sw.Times && rate > 0 {
		data.MaxWords = int(wordsShare*rate*opts.MaxLen + 0.5)
	}
	var out strings.Builder
	if err := t.Execute(&out, data); err != nil {
		panic(fmt.Sprintf("the prompt %s cannot be filled in: %v", name, err))
	}
	return out.String()
}

// wordsShare is the share of the longest length a prompt says in words,
// the same for every prompt, since it is the same model that reads them.
// Told nothing of the length, middle named stories of up to 351 words.
// Told 44 to 66 words, it named 8 of 18 within it in Tim's runs of
// 3 October, the typical one about a third over the top and a few two to
// four times over, and none short of the bottom when it thought in full.
// So only the longest is said, at three quarters: the typical story then
// ends near the top of the length the clip may have, a longer one is
// ended earlier by the engine, and a shorter one is grown with the
// sentences before it. Both ends were said once, 42 to 47 words, and a
// band that narrow set the model counting the words of every story until
// it ran out of tokens.
const wordsShare = 0.75

// PromptSwitches are what a recipe asked from a prompt file shows the model
// beyond its words, each switched on by name after a + in a comparison,
// as in points+pause2+times. With none, the model gets the words of each
// line and the length of a clip in words.
type PromptSwitches struct {
	// Pause marks a line that comes after a pause of at least that many
	// seconds with "…" in front of it. 0 marks none. +pause is a second,
	// +pause2 two.
	Pause float64
	// Times gives each line the minute and second it starts at, from the
	// start of the transcript, so the length of a stretch is one
	// subtraction, +times.
	Times bool
}

// ParseSwitches reads switches written as in pause2+times.
func ParseSwitches(text string) (PromptSwitches, error) {
	var sw PromptSwitches
	for _, name := range strings.Split(text, "+") {
		switch {
		case name == "times":
			sw.Times = true
		case name == "pause":
			sw.Pause = 1
		case strings.HasPrefix(name, "pause"):
			n, err := strconv.ParseFloat(strings.TrimPrefix(name, "pause"), 64)
			if err != nil || n <= 0 || n > 10 || math.IsNaN(n) {
				return sw, fmt.Errorf("+%s: a pause is +pause for a second or +pause2 for two, at most ten", name)
			}
			sw.Pause = n
		default:
			return sw, fmt.Errorf("+%s is no switch. There are +pause, +pause2 and +times. The length is said in words without +times", name)
		}
	}
	return sw, nil
}

// String is the switches as they are written, +pause2+times say.
func (sw PromptSwitches) String() string {
	var out string
	switch {
	case sw.Pause == 1:
		out += "+pause"
	case sw.Pause > 0:
		out += "+pause" + trimFloat(sw.Pause)
	}
	if sw.Times {
		out += "+times"
	}
	return out
}

// leanTranscript is every line numbered with its words, and what the
// switches add: the minute and second it starts at, and "…" in front of a
// line after a long pause. A recipe that leaves the length to the engine
// has no use for how long a line runs, and how loud a line was said says
// nothing about the story in it.
func leanTranscript(lines []Line, sw PromptSwitches) string {
	if len(lines) == 0 {
		return ""
	}
	first := lines[0].Start()
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "[%d", i+1)
		if sw.Times {
			at := int(line.Start() - first)
			fmt.Fprintf(&b, " %d:%02d", at/60, at%60)
		}
		b.WriteString("] ")
		if sw.Pause > 0 && line.GapBefore >= sw.Pause {
			b.WriteString("… ")
		}
		b.WriteString(line.Text())
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
