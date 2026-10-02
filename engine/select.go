package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SystemPrompt tells the model how to choose and condense clips. It is
// PromptVersion 3, a brief for any video. The one before was written for
// one podcast and pulled two ways: a founder portrait and the guest,
// exactly N clips against fewer beat padding, 20 to 30 seconds against the
// payoff mattering more than being brief, condense against keep when in
// doubt. And nothing in it said what must never be cut. This one gives one
// order of what matters, and the request says the task again after the
// transcript, where a model reading a long document attends to it best.
// Side by side on Tim's episode, as the recipe lines2, it read better
// than the brief before it.
const SystemPrompt = storyBrief + `
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

// storyBrief is what lines and stories2 both tell the model about a good
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
	// Heart is the first and last line the model named as the heart of
	// the clip, which fitting it to the length never cuts. It is zero
	// where the recipe asks for none, see heart.go.
	Heart [2]int
	// Opening is the line the model named as the one a clip must not
	// start after, 0 where the recipe asks for none, see heart.go.
	Opening int
}

// validateEntry checks one clip of an answer, the index-th. It gives the
// clip, what was wrong with it, and whether anything of it can be used.
func validateEntry(clipAny any, index, lineCount int) (PlanEntry, []string, bool) {
	var problems []string
	clip, ok := clipAny.(map[string]any)
	if !ok {
		return PlanEntry{}, []string{fmt.Sprintf("clip %d is not an object", index)}, false
	}
	// The points recipe names three lines rather than runs. They are the
	// run from start to end, the payoff its heart and the start its opening.
	if _, has := clip["keep"]; !has {
		if _, named := clip["start"]; named {
			points, problem := fromPoints(clip, index, lineCount)
			if problem != "" {
				return PlanEntry{}, []string{problem}, false
			}
			clip = points
		}
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
	// A heart that cannot be read is no heart, and the clip is taken as
	// the model kept it.
	var heart [2]int
	if heartAny, present := clip["heart"]; present {
		pair, ok := heartAny.([]any)
		var first, last int
		ok1, ok2 := false, false
		if ok && len(pair) == 2 {
			first, ok1 = toInt(pair[0])
			last, ok2 = toInt(pair[1])
		}
		switch {
		case !ok1 || !ok2:
			problems = append(problems, fmt.Sprintf("clip %d: its heart is not a pair of numbers", index))
		case first < 1 || last > lineCount || last < first:
			problems = append(problems, fmt.Sprintf("clip %d: its heart %d-%d is not inside 1-%d",
				index, first, last, lineCount))
		default:
			heart = [2]int{first, last}
		}
	}
	opening := 0
	if openingAny, present := clip["opening"]; present {
		n, ok := toInt(openingAny)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("clip %d: its opening is not a number", index))
		case n < 1 || n > lineCount:
			problems = append(problems, fmt.Sprintf("clip %d: its opening %d is not inside 1-%d",
				index, n, lineCount))
		default:
			opening = n
		}
	}
	get := func(key string) string {
		value, present := clip[key]
		if !present {
			return ""
		}
		return pyStr(value)
	}
	// An answer without slugs gets them from the titles, the way a clip
	// made by hand does.
	slug, title := Scrub(get("slug"), 64), Scrub(get("title"), 200)
	if slug == "" {
		slug = strings.ToLower(SanitiseName(title, "clip"))
	}
	return PlanEntry{
		Slug:    slug,
		Title:   title,
		Reason:  Scrub(get("reason"), 300),
		Keep:    ranges,
		Heart:   heart,
		Opening: opening,
	}, problems, true
}

// fromPoints reads a clip given as start, payoff and end into the run, the
// heart and the opening the other recipes give, or says what is wrong.
func fromPoints(clip map[string]any, index, lineCount int) (map[string]any, string) {
	var n [3]int
	for i, key := range []string{"start", "payoff", "end"} {
		v, ok := toInt(clip[key])
		if !ok || v < 1 || v > lineCount {
			return nil, fmt.Sprintf("clip %d: its %s is not a line from 1 to %d", index, key, lineCount)
		}
		n[i] = v
	}
	start, payoff, end := n[0], n[1], n[2]
	if payoff < start {
		return nil, fmt.Sprintf("clip %d: its payoff %d comes before its start %d", index, payoff, start)
	}
	end = max(end, payoff)
	out := make(map[string]any, len(clip)+3)
	for k, v := range clip {
		out[k] = v
	}
	out["keep"] = []any{[]any{float64(start), float64(end)}}
	out["heart"] = []any{float64(payoff), float64(payoff)}
	out["opening"] = float64(start)
	return out, ""
}

// readEntry checks one clip of an answer in the units the recipe numbered,
// and turns its runs into runs of lines, which is what everything after
// the answer works in.
func readEntry(clipAny any, index int, units [][2]int) (PlanEntry, []string, bool) {
	entry, problems, ok := validateEntry(clipAny, index, len(units))
	if !ok {
		return entry, problems, false
	}
	keep, err := toLines(entry.Keep, units)
	if err != nil {
		return PlanEntry{}, append(problems, fmt.Sprintf("clip %d: %s", index, err)), false
	}
	entry.Keep = keep
	if entry.Opening > 0 {
		if opening, err := toLines([][2]int{{entry.Opening, entry.Opening}}, units); err != nil {
			entry.Opening = 0
		} else {
			entry.Opening = opening[0][0]
		}
	}
	if entry.Heart[0] > 0 {
		heart, err := toLines([][2]int{entry.Heart}, units)
		if err != nil {
			entry.Heart = [2]int{}
		} else {
			entry.Heart = heart[0]
		}
	}
	return entry, problems, true
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
func ValidatePlan(data map[string]any, units [][2]int) ([]PlanEntry, []string, error) {
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
		entry, found, ok := readEntry(clipAny, i+1, units)
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
