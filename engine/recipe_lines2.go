package engine

import (
	"fmt"
	"strings"
)

// lines2Recipe is lines with its brief put right. The transcript is written
// out exactly as lines writes it, every line with its talking time, the
// pause before it and its level, so only what the model is told differs.
//
// The brief lines asks with was written for one podcast and pulls in two
// directions at once. It calls the format a founder portrait and speaks of
// the guest. It asks for exactly N clips and says fewer beat padding. It
// gives 20 to 30 seconds and says the payoff matters more than being brief.
// It says to condense and, when in doubt, to keep. And nothing in it says
// what must never be cut, which is what went wrong in stories: the part
// where the speaker actually tells the story was left out. This brief
// gives one order of what matters, for any video, and says the task again
// after the transcript, where a model reading a long document attends to
// it best.
var lines2Recipe = Recipe{
	Name:    "lines2",
	About:   "lines, with a brief for any video that says what must never be cut, and the task said again after the transcript",
	Unit:    "line",
	Version: 1,
	System:  lines2System,
	Request: lines2Request,
	Schema:  planSchema,
}

// stories2Recipe is the same brief with the transcript written as stories
// writes it, numbered sentences in paragraphs, about half the tokens of
// lines. Beside lines2 it tells whether what went wrong in stories was its
// brief, which told the model that leaving sentences out is usual, or the
// way it writes the transcript.
var stories2Recipe = func() Recipe {
	r := storiesRecipe
	r.Name = "stories2"
	r.About = "the lines2 brief with the transcript as stories writes it, numbered sentences in paragraphs"
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

func init() {
	recipes[lines2Recipe.Name] = lines2Recipe
	recipes[stories2Recipe.Name] = stories2Recipe
}

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

const lines2System = storyBrief + `
Pauses are yours to decide. A pause between two lines in one run stays, at full ` +
	`length. To cut a pause, end a run on the line before it and start the next run on ` +
	`the line after it. The two runs may follow each other directly, so [[12, 14], [15, 18]] ` +
	`keeps lines 12 to 18 and cuts only the pause between 14 and 15. To leave material out, ` +
	`leave its lines out. A long pause before a short line is often the speaker landing ` +
	`something, and cutting it throws the landing away. A long pause in the middle of ` +
	`someone losing their thread is dead weight.

OUTPUT CONTRACT

Your reply is parsed by a program. Return exactly one JSON object and nothing else. ` +
	`No prose, no markdown fences.

{"clips": [{"slug": "...", "title": "...", "reason": "...", "keep": [[12, 18], [24, 27]]}]}

- "clips": at most the number asked for.
- "slug": lowercase ASCII letters, digits and hyphens, at most 64 characters, ` +
	`different for every clip.
- "title": a hook line in the language of the transcript, one line, at most 200 characters.
- "reason": one sentence, at most 300 characters.
- "keep": runs of lines to keep, as [first, last] line numbers from the transcript, ` +
	`in ascending order and not overlapping. A run may start on the line right after ` +
	`the previous one ends, which cuts the pause between them.
`

// storyBrief is what lines2 and stories2 both tell the model about a good
// clip, for any video.
const storyBrief = `You find the moments in a long video that work as short vertical videos on ` +
	`their own, for YouTube Shorts, Instagram Reels and TikTok. You know the video only ` +
	`from its transcript.

A moment works when it is a small, complete story. It has three parts, in this order ` +
	`of importance:

1. The heart: the part where the speaker actually tells what happened, what they ` +
	`saw, did or felt. Without it there is no story. It is never cut.
2. The payoff: the line that lands it, what happened in the end or what it meant. ` +
	`The clip ends there, on that line, not after it.
3. The opening: the least a stranger needs to follow, a question or a setup when ` +
	`there is one. The clip starts there, not in the middle of a sentence and not ` +
	`earlier than needed.

Each clip should run the length asked for. When the whole story is longer, make it ` +
	`shorter by leaving out what the story holds without: asides, restarts, repetitions, ` +
	`a second example. Never shorten it by cutting the heart or the payoff. A story that ` +
	`cannot be told within the length even so is not a clip.

Prefer moments that are specific, and a mix of kinds: something moving, a surprise, ` +
	`something funny, a vivid scene, an insight put plainly. Return at most the number ` +
	`asked for, the strongest first. Fewer strong clips beat padding.

Never reorder anything, never join two runs so that they say something the speaker ` +
	`did not, and keep whole clauses.
`

func lines2Request(lines []Line, _ [][2]int, opts PlanOptions) string {
	task := fmt.Sprintf("Find up to %d clips, the strongest first. Each runs %s to %s seconds "+
		"once what you leave out is gone. Keep the heart and the payoff of every story whole.",
		opts.Count, fixed(opts.MinLen, 0), fixed(opts.MaxLen, 0))
	ask := []string{
		task,
		fmt.Sprintf("The transcript below is numbered from 1 to %d. Those numbers are what "+
			"you return. Each line shows its talking time in seconds, and any pause before "+
			"it, so a clip's length is the lines you keep plus the pauses inside the runs "+
			"you keep.", len(lines)),
	}
	if opts.Context != "" {
		ask = append(ask, "About the video: "+opts.Context)
	}
	ask = append(ask, "", "Transcript:", "", AnnotateLines(lines), "",
		"That is the whole transcript. "+task,
		"Reply with the JSON object and nothing else.")
	return strings.Join(ask, "\n")
}
