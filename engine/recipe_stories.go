package engine

import (
	"fmt"
	"math"
	"strings"
)

// storiesRecipe asks for stories rather than for lines. The model is told
// what makes a moment work as a short, for any video, and reads the
// transcript the way a person would: sentences in paragraphs, a time at the
// start of each paragraph and three dots for a long pause. It answers in
// sentence numbers. Everything measured in milliseconds, which pause is
// cut, where a word begins, is the engine's.
var storiesRecipe = Recipe{
	Name:    "stories",
	About:   "a brief for any video, the transcript as sentences in paragraphs, the strongest first",
	Unit:    "sentence",
	Joins:   true,
	Version: 4,
	System:  storiesSystem,
	Units:   sentenceUnits,
	Request: storiesRequest,
	Schema:  planSchema,
}

// storiesEditRecipe is stories asked twice: the first ask finds the
// stories and the second, with the transcript still read, looks only at
// where each one starts and ends. Half the thinking goes to each.
var storiesEditRecipe = func() Recipe {
	r := storiesRecipe
	r.Name = "stories-edit"
	r.About = "stories, then a second ask about where every clip starts and ends, the thinking split between the two"
	r.Edit = true
	return r
}()

func init() {
	recipes[storiesRecipe.Name] = storiesRecipe
	recipes[storiesEditRecipe.Name] = storiesEditRecipe
}

const storiesSystem = `You find the moments in a long video that work as short vertical videos on ` +
	`their own, for YouTube Shorts, Instagram Reels and TikTok. You know the video ` +
	`only from what is said in it, as a transcript.

A moment works when it is a small, complete story in the speaker's own words. It ` +
	`opens on something that makes a stranger stay, it turns, and it lands on a ` +
	`payoff: what happened, what it meant, the line that stays with you. Someone who ` +
	`has seen nothing else of the video understands it.

Strong moments come in different kinds: something human or moving, a surprise, ` +
	`something funny, tension, a vivid scene, a line worth quoting, an insight put ` +
	`plainly. Look for all of them and prefer a mix. Something specific the speaker ` +
	`saw, did or felt beats something general.

A clip must reach its payoff, and it must fit the length asked for. When a ` +
	`moment runs longer than that, keep the opening and the payoff and leave out ` +
	`sentences between them: asides, restarts, a second example, whatever the ` +
	`story holds without. Leaving out a sentence or two inside a moment is usual, ` +
	`not a last resort. Never reorder anything, and never join two parts so that ` +
	`they say something the speaker did not.

Fewer strong clips beat padding. Return at most the number asked for, the ` +
	`strongest first.

OUTPUT CONTRACT

Your reply is parsed by a program. Return exactly one JSON object and nothing ` +
	`else. No prose, no markdown fences.

{"clips": [{"slug": "...", "title": "...", "reason": "...", "keep": [[12, 14], [17, 18]]}]}

- "slug": lowercase ASCII letters, digits and hyphens, at most 64 characters, ` +
	`different for every clip.
- "title": a hook line in the language of the transcript, at most 200 characters.
- "reason": one sentence on why it works, at most 300 characters.
- "keep": runs of sentences to keep, as [first, last] sentence numbers from the ` +
	`transcript, in order and not overlapping. A clip is its runs played one after ` +
	`the other. A new run starts only where something is left out, so ` +
	`[[12, 14], [17, 18]] leaves out 15 and 16. Pauses are not yours to cut.
`

// A sentence ends where its last word ends one, or when it has run long
// enough that a clip needs to be able to cut inside it. Not at a pause: a
// sentence cut at a pause is a place to end a clip in mid-sentence, and
// that is where clips ended, on "aber davor".
const (
	sentenceSeconds = 30.0
	// A paragraph ends at a pause this long, or once it has run this long,
	// and the next one opens with its time.
	paragraphPause   = 1.5
	paragraphSeconds = 45.0
	// A pause this long before a sentence is written as three dots.
	dotsPause = 1.0
)

// sentenceUnits groups the lines into sentences. Lines already end at every
// sentence end once they have some length, so a sentence is one line or a
// few.
func sentenceUnits(lines []Line) [][2]int {
	var units [][2]int
	start := 0
	for i := range lines {
		last := i == len(lines)-1
		ends := endsSentence(strings.TrimSpace(lines[i].Text())) ||
			lines[i].End()-lines[start].Start() >= sentenceSeconds
		if ends || last {
			units = append(units, [2]int{start, i})
			start = i + 1
		}
	}
	return units
}

func storiesRequest(lines []Line, units [][2]int, opts PlanOptions) string {
	ask := []string{
		fmt.Sprintf("Find up to %d clips in the transcript below, the strongest first.", opts.Count),
		fmt.Sprintf("Each clip runs %s to %s seconds once the sentences you leave out are gone.",
			fixed(opts.MinLen, 0), fixed(opts.MaxLen, 0)),
		fmt.Sprintf("The transcript has %d sentences, each with its number in brackets. "+
			"Each paragraph opens with the time it starts at, so you can tell how long "+
			"a stretch runs. Three dots mark a long pause.", len(units)),
	}
	// Sentence numbers say nothing of time, and a model does not add up
	// the times of paragraphs well. Words it can count, so the length is
	// said in words, at the rate this speaker talks.
	if rate := wordsPerSecond(lines); rate > 0 {
		ask = append(ask, fmt.Sprintf("Speech here runs at about %s words a second, so %s to %s "+
			"seconds is about %d to %d words.", fixed(rate, 1), fixed(opts.MinLen, 0),
			fixed(opts.MaxLen, 0), int(math.Round(rate*opts.MinLen)), int(math.Round(rate*opts.MaxLen))))
	}
	if opts.Context != "" {
		ask = append(ask, "About the video: "+opts.Context)
	}
	ask = append(ask, "", "Transcript:", "", writeSentences(lines, units), "",
		"Reply with the JSON object and nothing else.")
	return strings.Join(ask, "\n")
}

// wordsPerSecond is how fast the lines are spoken, pauses included, since
// a clip's length has its pauses in it too. Zero for too little to tell.
func wordsPerSecond(lines []Line) float64 {
	if len(lines) == 0 {
		return 0
	}
	words := 0
	for _, line := range lines {
		words += len(line.Cues)
	}
	seconds := lines[len(lines)-1].End() - lines[0].Start()
	if seconds < 10 {
		return 0
	}
	return float64(words) / seconds
}

// writeSentences writes the transcript as paragraphs of numbered
// sentences. A paragraph opens with its time.
func writeSentences(lines []Line, units [][2]int) string {
	var out strings.Builder
	paragraphStart := -1.0
	for n, unit := range units {
		first := lines[unit[0]]
		opens := n == 0 || first.GapBefore >= paragraphPause ||
			first.Start()-paragraphStart >= paragraphSeconds
		if opens {
			if n > 0 {
				out.WriteString("\n")
			}
			paragraphStart = first.Start()
			fmt.Fprintf(&out, "(%s)", clockTime(first.Start()))
		} else if first.GapBefore >= dotsPause {
			out.WriteString(" …")
		}
		fmt.Fprintf(&out, " [%d]", n+1)
		for k, line := range lines[unit[0] : unit[1]+1] {
			if k > 0 && line.GapBefore >= dotsPause {
				out.WriteString(" …")
			}
			out.WriteString(" " + line.Text())
		}
	}
	return out.String()
}

// clockTime writes seconds as m:ss, or h:mm:ss past the hour.
func clockTime(seconds float64) string {
	s := int(seconds)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
