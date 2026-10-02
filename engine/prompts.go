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
// Such a prompt is one message. The transcript comes first and what to do
// with it after, so the instructions are the last thing the model reads
// before it answers, and are said once. The lines brief told the model
// the task, then the transcript, then the task again. There is no system
// part: everything the model needs is in the one message.
// ---------------------------------------------------------------------------

//go:embed prompts/*.txt
var promptFiles embed.FS

// promptData is what a prompt file is filled in with.
type promptData struct {
	Count      int
	Min, Max   string
	Transcript string
	// Taken says which lines are in clips already, empty when none are.
	Taken   string
	Context string
}

// prompt fills in the prompt file of that name. A file that does not parse
// is a fault of the program, found by its tests.
func prompt(name string, lines []Line, transcript string, opts PlanOptions) string {
	text, err := promptFiles.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		panic(fmt.Sprintf("there is no prompt %s: %v", name, err))
	}
	t := template.Must(template.New(name).Option("missingkey=error").Parse(string(text)))
	data := promptData{Count: opts.Count, Min: fixed(opts.MinLen, 0), Max: fixed(opts.MaxLen, 0),
		Transcript: transcript, Context: opts.Context}
	if taken := takenLines(lines, opts.Taken); len(taken) > 0 {
		data.Taken = takenSentence(taken)
	}
	var out strings.Builder
	if err := t.Execute(&out, data); err != nil {
		panic(fmt.Sprintf("the prompt %s cannot be filled in: %v", name, err))
	}
	return strings.TrimRight(out.String(), "\n")
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
