package engine

import (
	"fmt"
	"strings"
)

// stories2Recipe is the brief lines asks with, with the transcript written
// as stories writes it, numbered sentences in paragraphs, about half the
// tokens of lines. Beside lines it tells whether what went wrong in
// stories was its brief, which told the model that leaving sentences out
// is usual, or the way it writes the transcript.
var stories2Recipe = func() Recipe {
	r := storiesRecipe
	r.Name = "stories2"
	r.About = "the lines brief with the transcript as stories writes it, numbered sentences in paragraphs"
	r.Version = 1
	r.System = storyBrief + `
OUTPUT CONTRACT

Your reply is parsed by a program. Return exactly one JSON object and nothing else. ` +
		`No prose, no markdown fences.

{"clips": [{"slug": "...", "title": "...", "reason": "...", "keep": [[12, 14], [17, 18]]}]}

- "clips": at most the number asked for.
- "slug": lowercase ASCII letters, digits and hyphens, at most 64 characters, ` +
		`different for every clip.
- "title": a hook line in the language of the transcript, one line, at most 200 characters.
- "reason": one sentence, at most 300 characters.
- "keep": runs of sentences to keep, as [first, last] sentence numbers from the ` +
		`transcript, in order and not overlapping. A new run starts only where something ` +
		`is left out, so [[12, 14], [17, 18]] leaves out 15 and 16.
`
	r.Request = stories2Request
	return r
}()

func init() { recipes[stories2Recipe.Name] = stories2Recipe }

func stories2Request(lines []Line, units [][2]int, opts PlanOptions) string {
	task := fmt.Sprintf("Find up to %d clips, the strongest first. Each runs %s to %s seconds "+
		"once what you leave out is gone. Keep the heart and the payoff of every story whole.",
		opts.Count, fixed(opts.MinLen, 0), fixed(opts.MaxLen, 0))
	ask := []string{task,
		fmt.Sprintf("The transcript has %d sentences, each with its number in brackets. "+
			"Each paragraph opens with the time it starts at. Three dots mark a long pause.",
			len(units))}
	if rate := wordsPerSecond(lines); rate > 0 {
		ask = append(ask, fmt.Sprintf("Speech here runs at about %s words a second, so %s to %s "+
			"seconds is about %d to %d words.", fixed(rate, 1), fixed(opts.MinLen, 0),
			fixed(opts.MaxLen, 0), int(rate*opts.MinLen+0.5), int(rate*opts.MaxLen+0.5)))
	}
	if opts.Context != "" {
		ask = append(ask, "About the video: "+opts.Context)
	}
	ask = append(ask, "", "Transcript:", "", writeSentences(lines, units), "",
		"That is the whole transcript. "+task,
		"Reply with the JSON object and nothing else.")
	return strings.Join(ask, "\n")
}
