package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SystemPrompt tells the model how to choose and condense clips.
const SystemPrompt = `You choose short-form clips from a long-form interview podcast transcript. The ` +
	`format is a founder portrait, so the payoff is usually a quiet, concrete ` +
	`moment rather than a loud claim.

A clip must contain its own payoff. The opening buys attention and the payoff ` +
	`is what that attention was for: the thing that happened, the line that lands, ` +
	`what it turned out to mean. Stopping before it arrives is the worst outcome ` +
	`here, worse than running long and worse than no clip at all. If the payoff ` +
	`comes forty seconds after the setup, take the forty seconds and cut the ` +
	`middle out.

Use the length you are given. The range in the request is there to be used, ` +
	`and a clip that reaches the payoff at the top of it beats one that fits ` +
	`comfortably in the middle without getting there. Do not pad, but do not stop ` +
	`short to be brief either.

Each clip should also stand alone without the surrounding conversation, open ` +
	`on something that makes a stranger keep watching, and rest on something ` +
	`specific the guest saw, did or felt.

Condense by dropping the material between the runs you keep. Cut restarts, ` +
	`hedging and filler. Never reorder anything, never join two runs so the result ` +
	`implies something neither of them said, and keep whole clauses. When in doubt, ` +
	`keep the material.

Pauses are yours to decide. A pause between two lines in one run stays, at ` +
	`full length. To cut a pause, end a run on the line before it and start the ` +
	`next run on the line after it. The two runs may follow each other directly, ` +
	`so [[12, 14], [15, 18]] keeps lines 12 to 18 and cuts only the pause between ` +
	`14 and 15. To drop material, leave its lines out. To hold a beat before a line ` +
	`lands, keep both lines in one run.

The transcript shows how long each pause was. A long one before a short line ` +
	`is often the speaker landing something, and cutting it throws the landing ` +
	`away. A long one in the middle of someone losing their thread is dead weight. ` +
	`Only you can tell those apart, so this is not done for you.

OUTPUT CONTRACT

Your reply is parsed by a program. It is rejected outright unless every rule ` +
	`below holds.

Return exactly one JSON object and nothing else. No prose, no markdown fences.

{"clips": [{"slug": "...", "title": "...", "reason": "...", "keep": ` +
	`[[12, 18], [24, 27]]}]}

- "clips": exactly the number asked for. Fewer good ones beats padding.
- "slug": lowercase ASCII letters, digits and hyphens, at most 64 characters, ` +
	`different for every clip.
- "title": a hook line in the language of the transcript, one line, at most ` +
	`200 characters.
- "reason": one sentence, at most 300 characters.
- "keep": a non-empty array of [first, last] line numbers from the transcript. ` +
	`Each pair is a run of consecutive lines to keep.

Every pair must satisfy all of the following:

- both numbers name lines that exist in the transcript
- first is less than or equal to last
- pairs are in ascending order and do not overlap. A pair may start on the ` +
	`line right after the previous pair ends, which cuts the pause between them
- the clip begins and ends on a line that carries meaning, never on one that ` +
	`is only a hesitation
`

type jsonCandidate struct {
	start, end int
	complete   bool
	truncated  bool
	closers    string
}

// jsonCandidates finds every brace-balanced object in the text, plus the
// truncated tail.
//
// A scan that respects string literals and escapes is the only way to do this
// correctly. Searching for the first and last brace breaks the moment a reply
// contains a brace inside a string, or prose after the JSON, and both happen
// in practice.
func jsonCandidates(text string) []jsonCandidate {
	var out []jsonCandidate
	var stack []byte
	start := -1
	inString, escaped := false, false
	type safePoint struct {
		position  int
		remaining []byte
	}
	var safePoints []safePoint

	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			if len(stack) == 0 && ch == '{' {
				start = i
			}
			stack = append(stack, ch)
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			opener := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !(opener == '{' && ch == '}') && !(opener == '[' && ch == ']') {
				stack = stack[:0]
				start = -1
				continue
			}
			if len(stack) > 0 {
				// A complete value ended while still nested. This is where a
				// truncated reply can be cut back to without losing structure.
				safePoints = append(safePoints, safePoint{i + 1, append([]byte(nil), stack...)})
			} else if start >= 0 {
				out = append(out, jsonCandidate{start: start, end: i + 1, complete: true})
				start = -1
			}
		}
	}

	if len(stack) > 0 && start >= 0 {
		out = append(out, jsonCandidate{start: start, end: len(text)})
		for k := len(safePoints) - 1; k >= 0; k-- {
			point := safePoints[k]
			if point.position > start {
				var closers strings.Builder
				for j := len(point.remaining) - 1; j >= 0; j-- {
					if point.remaining[j] == '{' {
						closers.WriteByte('}')
					} else {
						closers.WriteByte(']')
					}
				}
				out = append(out, jsonCandidate{start: start, end: point.position,
					truncated: true, closers: closers.String()})
				break
			}
		}
	}
	return out
}

func stripFences(text string) string {
	cleaned := strip(text)
	if !strings.Contains(cleaned, "```") {
		return cleaned
	}
	parts := strings.Split(cleaned, "```")
	var blocks []string
	for i := 1; i < len(parts); i += 2 {
		blocks = append(blocks, parts[i])
	}
	if len(blocks) == 0 {
		return cleaned
	}
	// Keep the longest fenced block, which is the payload.
	best := blocks[0]
	for _, b := range blocks[1:] {
		if runeLen(b) > runeLen(best) {
			best = b
		}
	}
	trimmed := strings.TrimLeftFunc(best, isPySpace)
	if strings.Contains(best, "\n") && !strings.HasPrefix(trimmed, "{") &&
		!strings.HasPrefix(trimmed, "[") {
		best = strings.SplitN(best, "\n", 2)[1] // drop the language tag
	}
	return strip(best)
}

func parseObject(text string) (map[string]any, bool) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if decoder.More() {
		return nil, false
	}
	object, ok := value.(map[string]any)
	return object, ok
}

// ExtractJSONObject finds the object with the given key in a reply that may
// also contain prose. The note says how it was recovered, so the caller can
// say out loud when a reply needed salvaging.
func ExtractJSONObject(text, key string) (map[string]any, string, error) {
	cleaned := stripFences(text)
	var truncated []jsonCandidate
	for _, c := range jsonCandidates(cleaned) {
		switch {
		case c.complete:
			data, ok := parseObject(cleaned[c.start:c.end])
			if !ok {
				continue
			}
			if _, has := data[key]; has {
				note := ""
				if c.start > 0 || c.end < len(cleaned) {
					note = "extracted from surrounding prose"
				}
				return data, note, nil
			}
		case c.truncated:
			truncated = append(truncated, c)
		}
	}
	// Nothing complete parsed. Try to rescue a reply that was cut off, which
	// saves paying for the whole transcript again.
	for _, c := range truncated {
		data, ok := parseObject(cleaned[c.start:c.end] + c.closers)
		if !ok {
			continue
		}
		if value, has := data[key]; has && value != nil {
			return data, "reply was cut off, recovered what was complete", nil
		}
	}
	return nil, "", renderErr("no JSON object with a %s key could be found in the reply. "+
		"It began: %s", pyRepr(key), pyRepr(Scrub(cleaned, 200)))
}

// PlanEntry is one clip as the model proposed it, after validation.
type PlanEntry struct {
	Slug   string
	Title  string
	Reason string
	Keep   [][2]int
}

// validateEntry checks one clip of an answer, the index-th. It gives the
// clip, what was wrong with it, and whether anything of it can be used.
func validateEntry(clipAny any, index, lineCount int) (PlanEntry, []string, bool) {
	var problems []string
	clip, ok := clipAny.(map[string]any)
	if !ok {
		return PlanEntry{}, []string{fmt.Sprintf("clip %d is not an object", index)}, false
	}
	rangesIn, ok := clip["keep"].([]any)
	if !ok || len(rangesIn) == 0 {
		return PlanEntry{}, []string{fmt.Sprintf("clip %d has no keep ranges", index)}, false
	}
	var ranges [][2]int
	previous := 0
	for _, itemAny := range rangesIn {
		item, ok := itemAny.([]any)
		if !ok || len(item) != 2 {
			problems = append(problems, fmt.Sprintf("clip %d: a keep range is not a pair", index))
			continue
		}
		first, ok1 := toInt(item[0])
		last, ok2 := toInt(item[1])
		if !ok1 || !ok2 {
			problems = append(problems, fmt.Sprintf("clip %d: a keep range is not numeric", index))
			continue
		}
		if !(1 <= first && first <= lineCount && 1 <= last && last <= lineCount) {
			problems = append(problems, fmt.Sprintf("clip %d: lines %d-%d are outside 1-%d",
				index, first, last, lineCount))
			continue
		}
		if last < first {
			problems = append(problems, fmt.Sprintf("clip %d: range %d-%d is backwards",
				index, first, last))
			continue
		}
		if first <= previous {
			problems = append(problems, fmt.Sprintf("clip %d: range %d-%d overlaps the one before it",
				index, first, last))
			continue
		}
		previous = last
		ranges = append(ranges, [2]int{first, last})
	}
	if len(ranges) == 0 {
		return PlanEntry{}, problems, false
	}
	get := func(key string) string {
		value, present := clip[key]
		if !present {
			return ""
		}
		return pyStr(value)
	}
	return PlanEntry{
		Slug:   Scrub(get("slug"), 64),
		Title:  Scrub(get("title"), 200),
		Reason: Scrub(get("reason"), 300),
		Keep:   ranges,
	}, problems, true
}

// uniqueSlug gives a clip a slug no clip before it has, the position-th
// usable one. It says what it changed, if anything.
func uniqueSlug(seen map[string]bool, entry *PlanEntry, position int) string {
	problem := ""
	if entry.Slug != "" && seen[entry.Slug] {
		problem = fmt.Sprintf("clip %d: slug %s was already used", position, pyRepr(entry.Slug))
		entry.Slug = fmt.Sprintf("%s-%d", entry.Slug, position)
	}
	seen[entry.Slug] = true
	return problem
}

// ValidatePlan checks the plan is shaped the way we asked, and says precisely
// what is not. With line numbers there is nothing to interpret, a number
// either names a line or it does not.
func ValidatePlan(data map[string]any, lineCount int) ([]PlanEntry, []string, error) {
	// The checks are made one clip at a time, so an answer read as it is
	// written goes through exactly the same ones as an answer read whole.
	var problems []string
	clipsAny := data["clips"]
	clips, ok := clipsAny.([]any)
	if !ok {
		return nil, nil, renderErr(`"clips" is not a list, so the reply is not a plan`)
	}
	if len(clips) == 0 {
		return nil, nil, renderErr("the reply contained an empty clips list")
	}

	var good []PlanEntry
	for i, clipAny := range clips {
		entry, found, ok := validateEntry(clipAny, i+1, lineCount)
		problems = append(problems, found...)
		if ok {
			good = append(good, entry)
		}
	}

	seen := map[string]bool{}
	for i := range good {
		if problem := uniqueSlug(seen, &good[i], i+1); problem != "" {
			problems = append(problems, problem)
		}
	}

	if len(good) == 0 {
		shown := problems
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return nil, nil, renderErr("the reply parsed but contained no usable clip. %s",
			strings.Join(shown, ". "))
	}
	return good, problems, nil
}
