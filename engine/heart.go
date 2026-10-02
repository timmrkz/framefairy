package engine

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// The heart of a clip, and fitting a clip to the length around it
//
// A model reads lines and their lengths but cannot add fifteen of them up,
// so the same prompt gives clips of 10 seconds and of 60. The lines recipe
// asks it again about the clips well off the length, which costs a second
// ask and holds those clips back until it is answered. Cutting a clip to
// the length without asking was tried once and taken out: the engine could
// not tell where the heart of a story is, and cut the setup off one story
// and the payoff out of another.
//
// The heart recipe asks once. The model names the heart of every clip, the
// lines that carry what happened and its payoff, and keeps the story around
// it as far as it reaches. The engine knows the length to the millisecond,
// and fits the clip to it on whole sentences: what runs on past the heart
// goes first, then setup from the start, and never the heart. A clip too
// short takes in the sentences before it, the way I grows a clip made by
// hand, and after it only where there is nothing before it to take. A
// clip whose heart alone runs past the length stays whole. A complete
// story of 38 seconds is worth more than one of 19 without its core.
// ---------------------------------------------------------------------------

// heartRecipe is lines asked for the heart of every clip, with the length
// left to the engine.
var heartRecipe = Recipe{
	Name: "heart",
	About: "the lines brief and transcript, the heart of every clip named, and the length fitted " +
		"by the engine on whole sentences around the heart, asked once",
	Unit:    "line",
	Hearts:  true,
	Version: 1,
	System:  heartSystem,
	Request: heartRequest,
	Schema:  heartSchema,
}

func init() { recipes[heartRecipe.Name] = heartRecipe }

// lengthParagraph is what the brief says about the length, which in the
// heart recipe is the engine's to keep.
const lengthParagraph = `Each clip should run the length asked for. When the whole story is longer, make it ` +
	`shorter by leaving out what the story holds without: asides, restarts, repetitions, ` +
	`a second example. Never shorten it by cutting the heart or the payoff. A story that ` +
	`cannot be told within the length even so is not a clip.`

const heartLength = `A program fits every clip to the length asked for, on whole lines, so do not ` +
	`count seconds. It needs two things from you. The heart: the lines that carry what ` +
	`happened and the payoff, which it never cuts. And the story around it, as far as it ` +
	`reaches, from the least a stranger needs to the payoff. When the story is too long, the ` +
	`program takes away what runs on after the heart first, then setup from the start. ` +
	`When it is too short, it takes in the lines before it. Inside the story, leave out ` +
	`what it holds without: asides, restarts, repetitions, a second example.`

// heartSystem is the brief lines asks with, the length left to the program
// and the heart asked for in the answer.
var heartSystem = strings.Replace(storyBrief, lengthParagraph, heartLength, 1) + `
Pauses are yours to decide. A pause between two lines in one run stays, at full ` +
	`length. To cut a pause, end a run on the line before it and start the next run on ` +
	`the line after it. The two runs may follow each other directly, so [[12, 14], [15, 18]] ` +
	`keeps lines 12 to 18 and cuts only the pause between 14 and 15.

OUTPUT CONTRACT

Your reply is parsed by a program. Return exactly one JSON object and nothing else. ` +
	`No prose, no markdown fences.

{"clips": [{"slug": "...", "title": "...", "reason": "...", "heart": [20, 24], "keep": [[12, 18], [20, 27]]}]}

- "clips": at most the number asked for.
- "slug": lowercase ASCII letters, digits and hyphens, at most 64 characters, ` +
	`different for every clip.
- "title": a hook line in the language of the transcript, one line, at most 200 characters.
- "reason": one sentence, at most 300 characters.
- "heart": the first and the last line of the heart and the payoff, [first, last]. ` +
	`Every line from first to last is kept.
- "keep": runs of lines the story reaches, as [first, last] line numbers from the ` +
	`transcript, in ascending order and not overlapping, the heart inside them. A run may ` +
	`start on the line right after the previous one ends, which cuts the pause between them.
`

func heartRequest(lines []Line, _ [][2]int, opts PlanOptions) string {
	task := fmt.Sprintf("Find up to %d clips, the strongest first. Name the heart of each, and "+
		"keep the story around it. The program fits each to %s to %s seconds.",
		opts.Count, fixed(opts.MinLen, 0), fixed(opts.MaxLen, 0))
	if taken := takenLines(lines, opts.Taken); len(taken) > 0 {
		task += " " + takenSentence(taken)
	}
	ask := []string{
		task,
		fmt.Sprintf("The transcript below is numbered from 1 to %d. Those numbers are what "+
			"you return. Each line shows its talking time in seconds, and any pause before it.",
			len(lines)),
	}
	if opts.Context != "" {
		ask = append(ask, "About the video: "+opts.Context)
	}
	ask = append(ask, "", "Transcript:", "", AnnotateLines(lines), "",
		"That is the whole transcript. "+task,
		"Reply with the JSON object and nothing else.")
	return strings.Join(ask, "\n")
}

// heartSchema is planSchema with the heart, which every clip has.
func heartSchema(lineCount, count int) string {
	return fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "clips": {
      "type": "array",
      "minItems": 1,
      "maxItems": %d,
      "items": {
        "type": "object",
        "properties": {
          "slug": {"type": "string", "pattern": "^[a-z0-9-]{1,64}$"},
          "title": {"type": "string", "minLength": 1, "maxLength": 200},
          "reason": {"type": "string", "minLength": 1, "maxLength": 300},
          "heart": {
            "type": "array",
            "minItems": 2,
            "maxItems": 2,
            "items": {"type": "integer", "minimum": 1, "maximum": %d}
          },
          "keep": {
            "type": "array",
            "minItems": 1,
            "maxItems": 12,
            "items": {
              "type": "array",
              "minItems": 2,
              "maxItems": 2,
              "items": {"type": "integer", "minimum": 1, "maximum": %d}
            }
          }
        },
        "required": ["slug", "title", "reason", "heart", "keep"],
        "additionalProperties": false
      }
    }
  },
  "required": ["clips"],
  "additionalProperties": false
}`, max(count, 1), max(lineCount, 1), max(lineCount, 1))
}

// fitToHeart fits a clip, runs of lines numbered from 1, to shortest and
// longest seconds around its heart, on whole sentences, and says what it
// did: "shortened", "lengthened", or nothing. seconds measures a clip as it
// is cut, and taken says whether a line is in another clip already, which
// a clip never grows into. A clip without a heart stays as it is.
//
// Too long, it lets go of what runs on past the heart first, a sentence at
// a time, and then of setup from the start, and stops before a step that
// would leave it further off the length than it was. Too short, it takes
// in the sentence before it, and the one after it only where there is
// nothing before it to take, and never past longest.
func fitToHeart(lines []Line, keep [][2]int, heart [2]int, shortest, longest float64,
	taken func(n int) bool, seconds func([][2]int) float64) ([][2]int, string) {
	if heart[0] < 1 || heart[1] < heart[0] || heart[1] > len(lines) || len(keep) == 0 {
		return keep, ""
	}
	keep = withRun(keep, heart)
	now := seconds(keep)
	off := func(s float64) float64 { return max(0, shortest-s, s-longest) }
	did := ""
	for steps := 0; now > longest && steps < len(lines); steps++ {
		next := lessAround(lines, keep, heart)
		if next == nil {
			break
		}
		s := seconds(next)
		if off(s) > off(now) {
			break
		}
		keep, now, did = next, s, "shortened"
	}
	for steps := 0; now < shortest && steps < len(lines); steps++ {
		next := moreAround(lines, keep, taken)
		if next == nil {
			break
		}
		s := seconds(next)
		if s > longest {
			// The sentence before is too long to take. One after may not be.
			next = moreAfter(lines, keep, taken)
			if next == nil {
				break
			}
			if s = seconds(next); s > longest {
				break
			}
		}
		keep, now, did = next, s, "lengthened"
	}
	return keep, did
}

// withRun is keep with every line of run kept too, runs that then overlap
// made one.
func withRun(keep [][2]int, run [2]int) [][2]int {
	var out [][2]int
	placed := false
	add := func(r [2]int) {
		if n := len(out); n > 0 && r[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], r[1])
			return
		}
		out = append(out, r)
	}
	for _, r := range keep {
		if !placed && run[0] < r[0] {
			add(run)
			placed = true
		}
		add(r)
	}
	if !placed {
		add(run)
	}
	return out
}

// lessAround is keep a sentence shorter: at its end, while it runs on past
// the heart, and then at its start, up to the heart. Nil when only the
// heart is left.
func lessAround(lines []Line, keep [][2]int, heart [2]int) [][2]int {
	out := append([][2]int(nil), keep...)
	last := &out[len(out)-1]
	if last[1] > heart[1] {
		if last[0] > heart[1] {
			// A run wholly after the heart goes at once.
			return out[:len(out)-1]
		}
		end := heart[1]
		for n := last[1] - 1; n > heart[1]; n-- {
			if finishesSentence(lines, n) {
				end = n
				break
			}
		}
		last[1] = end
		return out
	}
	first := &out[0]
	if first[0] < heart[0] {
		if first[1] < heart[0] {
			return out[1:]
		}
		start := heart[0]
		for n := first[0] + 1; n < heart[0]; n++ {
			if beginsSentence(lines, n) {
				start = n
				break
			}
		}
		first[0] = start
		return out
	}
	return nil
}

// moreAround is keep with the sentence before it taken in, or nil when
// there is none to take.
func moreAround(lines []Line, keep [][2]int, taken func(int) bool) [][2]int {
	first := keep[0][0]
	if first <= 1 || taken(first-1) {
		return moreAfter(lines, keep, taken)
	}
	start := first - 1
	for n := first - 1; n >= 1 && lines[first-2].Start()-lines[n-1].Start() <= sentenceReach; n-- {
		if taken(n) {
			break
		}
		if beginsSentence(lines, n) {
			start = n
			break
		}
	}
	out := append([][2]int(nil), keep...)
	out[0][0] = start
	return out
}

// moreAfter is keep with the sentence after it taken in, or nil when there
// is none to take.
func moreAfter(lines []Line, keep [][2]int, taken func(int) bool) [][2]int {
	last := keep[len(keep)-1][1]
	if last >= len(lines) || taken(last+1) {
		return nil
	}
	end := last + 1
	for n := last + 1; n <= len(lines) && lines[n-1].End()-lines[last].End() <= sentenceReach; n++ {
		if taken(n) {
			break
		}
		if finishesSentence(lines, n) {
			end = n
			break
		}
	}
	out := append([][2]int(nil), keep...)
	out[len(out)-1][1] = end
	return out
}
