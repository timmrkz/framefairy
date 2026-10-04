package engine

import (
	"fmt"
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
	System:  promptSystem("heart"),
	Request: heartRequest,
	Schema:  heartSchema,
}

// heartOpeningRecipe is heart with the opening named too: the first line
// a stranger needs, which a clip never starts after. In Tim's first
// comparison heart started the umbrella story without "I was a small kid,
// second, third grade" and the mirror story without the father's art,
// both where the model chose to start, and lines kept both.
var heartOpeningRecipe = Recipe{
	Name: "heart-opening",
	About: "heart, with the opening named too, the first line a stranger needs, which the engine " +
		"never starts a clip after",
	Unit:    "line",
	Hearts:  true,
	Version: 1,
	System:  promptSystem("heart-opening"),
	Request: heartOpeningRequest,
	Schema:  heartOpeningSchema,
}

// heartLeanRecipe is heart-opening asked in a few lines: one message, the
// transcript first with nothing but the words, what to do after it, and a
// smaller answer. Beside heart-opening it tells whether the brief and the
// annotations of lines earn the time they take to read. Its switches add
// pause marks and times, see PromptSwitches.
var heartLeanRecipe = Recipe{
	Name: "heart-lean",
	About: "heart-opening in one short message, the transcript first with only the words, the " +
		"task after it, no slug and a short reason",
	Unit:       "line",
	Hearts:     true,
	Switchable: true,
	// The prompt says nothing of pauses, so two runs that meet are one,
	// and the pauses are the engine's to cut.
	Joins:   true,
	Version: 1,
	Request: func(lines []Line, _ [][2]int, opts PlanOptions) string {
		return prompt("heart-lean", lines, leanTranscript(lines, opts.Switches), opts)
	},
	Schema: leanSchema,
}

// pointsRecipe is three points a clip: where a story starts, where it
// lands and where it ends, one stretch. It answers without JSON, one line
// a clip with the title and the reason. The pauses are the engine's to cut,
// and the engine may cut what runs on after the payoff. Its switches add
// pause marks and times, see PromptSwitches.
var pointsRecipe = Recipe{
	Name: "points",
	About: "three line numbers a clip, start, payoff and end, with a title and a reason, one " +
		"line a clip without JSON, the pauses left to the engine",
	Unit:       "line",
	Hearts:     true,
	Joins:      true,
	Switchable: true,
	Plain:      pointsPlain,
	Version:    1,
	Request: func(lines []Line, _ [][2]int, opts PlanOptions) string {
		return prompt("points", lines, leanTranscript(lines, opts.Switches), opts)
	},
}

// middleRecipe is Tim's way of asking, as little as can be said: the
// transcript, and after it a few sentences. For each story the model
// names a line somewhere inside it, and then the line it starts on and
// the line it ends on, three numbers and nothing else. Nothing tells it
// what a short is, what a payoff is or what a video is, only how many
// words a story runs. Pointing at the story first is what it writes
// first, so its start and its end are written knowing which story they
// belong to. The engine never starts a story later and never ends it
// before the line inside it: a story too long is ended earlier on a whole
// sentence, and a shorter one takes in the sentences before it. The clip
// is named after its first words. It always marks the pauses of a second
// or more, which made the stories in Tim's comparisons hold to the length
// best of all. Its switches add times with the length in seconds, or mark
// another pause, see PromptSwitches. It is the default recipe, so its
// answer is the one PromptVersion names.
var middleRecipe = Recipe{
	Name: "middle",
	About: "only the transcript with its pauses and a few sentences with the length in words, " +
		"three line numbers a story, a line inside it, its start and its end, without JSON",
	Unit:       "line",
	Hearts:     true,
	Joins:      true,
	Switchable: true,
	Plain:      middlePlain,
	Version:    PromptVersion,
	Request: func(lines []Line, _ [][2]int, opts PlanOptions) string {
		opts.Switches = middleShows.and(opts.Switches)
		return prompt("middle", lines, leanTranscript(lines, opts.Switches), opts)
	},
}

// middleShows is what middle always shows the model besides the words:
// a mark before a line after a pause of a second or more.
var middleShows = PromptSwitches{Pause: 1}

func init() {
	recipes[heartRecipe.Name] = heartRecipe
	recipes[heartOpeningRecipe.Name] = heartOpeningRecipe
	recipes[heartLeanRecipe.Name] = heartLeanRecipe
	recipes[pointsRecipe.Name] = pointsRecipe
	recipes[middleRecipe.Name] = middleRecipe
}

// leanSchema is the answer of heart-lean: a title and a short reason, and
// the opening, the heart and the runs of the clip, no slug.
func leanSchema(lineCount, count int) string {
	line := fmt.Sprintf(`{"type": "integer", "minimum": 1, "maximum": %d}`, max(lineCount, 1))
	pair := fmt.Sprintf(`{"type": "array", "minItems": 2, "maxItems": 2, "items": %s}`, line)
	return fmt.Sprintf(`{"type": "object", "properties": {"clips": {"type": "array", "minItems": 1, `+
		`"maxItems": %d, "items": {"type": "object", "properties": {`+
		`"title": {"type": "string", "minLength": 1, "maxLength": 200}, `+
		`"reason": {"type": "string", "minLength": 1, "maxLength": 300}, `+
		`"opening": %s, "heart": %s, "keep": {"type": "array", "minItems": 1, "maxItems": 12, `+
		`"items": %s}}, "required": ["title", "reason", "opening", "heart", "keep"], `+
		`"additionalProperties": false}}}, "required": ["clips"], "additionalProperties": false}`,
		max(count, 1), line, pair, pair)
}

// heartRequest and heartOpeningRequest fill in their prompt files, which
// send the annotated transcript lines sends.
func heartRequest(lines []Line, _ [][2]int, opts PlanOptions) string {
	return prompt("heart", lines, AnnotateLines(lines), opts)
}

func heartOpeningRequest(lines []Line, _ [][2]int, opts PlanOptions) string {
	return prompt("heart-opening", lines, AnnotateLines(lines), opts)
}

// heartSchema is planSchema with the heart, which every clip has.
func heartSchema(lineCount, count int) string {
	return pointsSchema(lineCount, count, false)
}

// heartOpeningSchema is heartSchema with the opening, which every clip has.
func heartOpeningSchema(lineCount, count int) string {
	return pointsSchema(lineCount, count, true)
}

func pointsSchema(lineCount, count int, opening bool) string {
	openingField, required := "", `"slug", "title", "reason", "heart", "keep"`
	if opening {
		openingField = fmt.Sprintf(`
          "opening": {"type": "integer", "minimum": 1, "maximum": %d},`, max(lineCount, 1))
		required = `"slug", "title", "reason", "opening", "heart", "keep"`
	}
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
          "reason": {"type": "string", "minLength": 1, "maxLength": 300},%s
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
        "required": [%s],
        "additionalProperties": false
      }
    }
  },
  "required": ["clips"],
  "additionalProperties": false
}`, max(count, 1), openingField, max(lineCount, 1), max(lineCount, 1), required)
}

// fitToHeart fits a clip, runs of lines numbered from 1, to shortest and
// longest seconds around its heart, on whole sentences, and says what it
// did: "shortened", "lengthened", or nothing. opening is the line the clip
// must not start after, 0 for none. seconds measures a clip as it is cut,
// and taken says whether a line is in another clip already, which a clip
// never grows into. A clip without a heart stays as it is.
//
// Too long, it lets go of what runs on past the heart first, a sentence at
// a time, and then of setup from the start, never past the opening, and
// stops before a step that would leave it further off the length than it
// was. Too short, it takes
// in the sentence before it, and the one after it only where there is
// nothing before it to take, and never past longest.
func fitToHeart(lines []Line, keep [][2]int, heart [2]int, opening int, shortest, longest float64,
	taken func(n int) bool, seconds func([][2]int) float64) ([][2]int, string) {
	if heart[0] < 1 || heart[1] < heart[0] || heart[1] > len(lines) || len(keep) == 0 {
		return keep, ""
	}
	keep = withRun(keep, heart)
	// The setup goes no further than the opening, and a clip that starts
	// after its opening starts there. An opening after the heart begins is
	// no opening.
	floor := heart[0]
	if opening >= 1 && opening < heart[0] {
		floor = opening
		if keep[0][0] > opening {
			keep = append([][2]int(nil), keep...)
			keep[0][0] = opening
		}
	}
	now := seconds(keep)
	off := func(s float64) float64 { return max(0, shortest-s, s-longest) }
	did := ""
	for steps := 0; now > longest && steps < len(lines); steps++ {
		next := lessAround(lines, keep, heart, floor)
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
// the heart, and then at its start, up to floor, the heart or the opening
// before it. Nil when nothing more can go.
func lessAround(lines []Line, keep [][2]int, heart [2]int, floor int) [][2]int {
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
	if first[0] < floor {
		if first[1] < floor {
			return out[1:]
		}
		start := floor
		for n := first[0] + 1; n < floor; n++ {
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

// wholeHeart is a heart, lines numbered from 1, grown to the sentences its
// first and last line are in. A heart that is not inside the lines is none.
func wholeHeart(lines []Line, heart [2]int) [2]int {
	if heart[0] < 1 || heart[1] < heart[0] || heart[1] > len(lines) {
		return [2]int{}
	}
	return [2]int{sentenceStart(lines, heart[0]), sentenceEnd(lines, heart[1])}
}
