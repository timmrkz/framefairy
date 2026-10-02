package engine

import (
	"embed"
	"fmt"
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
// A file is what the model is sent, in the order it is sent: a system part
// after "=== system ===", where the recipe has one, and the request after
// "=== user ===", the same way the logs keep a prompt. The program fills in
// what is between {{ and }}: the transcript, how many clips, how long.
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
	// Taken says which lines are in clips already, empty when none are.
	Taken   string
	Context string
}

const (
	systemMark = "=== system ===\n"
	userMark   = "=== user ===\n"
)

// promptParts is the system part and the request of the prompt file of
// that name. A file that is not there or not in two parts is a fault of
// the program, found by its tests.
func promptParts(name string) (system, request string) {
	text, err := promptFiles.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic(fmt.Sprintf("there is no prompt %s: %v", name, err))
	}
	body := strings.TrimSuffix(string(text), "\n")
	if rest, ok := strings.CutPrefix(body, systemMark); ok {
		system, request, ok = strings.Cut(rest, "\n"+userMark)
		if !ok {
			panic(fmt.Sprintf("the prompt %s has a system part and no request", name))
		}
		return system, request
	}
	request, ok := strings.CutPrefix(body, userMark)
	if !ok {
		panic(fmt.Sprintf("the prompt %s does not begin with %q or %q", name, systemMark, userMark))
	}
	return "", request
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
		data.Taken = takenSentence(taken)
	}
	var out strings.Builder
	if err := t.Execute(&out, data); err != nil {
		panic(fmt.Sprintf("the prompt %s cannot be filled in: %v", name, err))
	}
	return out.String()
}

// longPause is the pause before a line that the lean transcripts mark, with
// "…" in front of the line.
const longPause = 1.0

// bareTranscript is every line numbered, and nothing about it but whether
// a long pause comes before it. A recipe that leaves the length to the
// engine has no use for how long a line runs, and how loud a line was said
// says nothing about the story in it.
func bareTranscript(lines []Line) string {
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "[%d] %s%s\n", i+1, pauseMark(line), line.Text())
	}
	return strings.TrimRight(b.String(), "\n")
}

// timedTranscript is every line numbered with the minute and second it
// starts at, from the start of the transcript. A clip's length is then one
// subtraction, where the lines brief left the model to add up the length of
// every line in it, which it could not.
func timedTranscript(lines []Line) string {
	if len(lines) == 0 {
		return ""
	}
	first := lines[0].Start()
	var b strings.Builder
	for i, line := range lines {
		at := int(line.Start() - first)
		fmt.Fprintf(&b, "[%d %d:%02d] %s%s\n", i+1, at/60, at%60, pauseMark(line), line.Text())
	}
	return strings.TrimRight(b.String(), "\n")
}

func pauseMark(line Line) string {
	if line.GapBefore >= longPause {
		return "… "
	}
	return ""
}
