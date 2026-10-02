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
