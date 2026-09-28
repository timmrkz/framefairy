package engine

import (
	"fmt"
	"math"
	"strings"
)

// ---------------------------------------------------------------------------
// Fitting clips to the length
//
// A model does not know how long a clip runs. It sees numbers, of lines or
// of sentences, and the same prompt gives clips of ten seconds on one run
// and of sixty on the next. The engine knows to the millisecond. So a clip
// well off the length asked for is held back as the answer comes in, and
// once the answer is in, the model is told how long each of those runs and
// how long each line or sentence around it lasts, and asked for them again.
// Asked in the same conversation, with the model still loaded, it reads
// only its own answer and the new question, not the transcript again.
//
// Taking something out of a clip that is too long is the cut inside a
// moment a good short needs, and the model seldom makes it unasked.
//
// Whatever comes back, a clip is never lost: the one nearer the length is
// kept, and one that still does not fit is flagged as it always was.
// ---------------------------------------------------------------------------

// fitAround is how many lines or sentences either side of a clip the model
// is told the length of, which is what it may take in.
const fitAround = 6

// holding says whether clips are waiting to be fitted.
func (b *planBuilder) holding() []PlanEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]PlanEntry(nil), b.held...)
}

// fitRequest is what the model is asked about the clips held back.
func (b *planBuilder) fitRequest(held []PlanEntry) string {
	recipe := b.opts.recipe()
	unit := recipe.Unit
	if unit == "" {
		unit = "line"
	}
	lo, hi := fixed(b.opts.MinLen, 0), fixed(b.opts.MaxLen, 0)
	var out strings.Builder
	if recipe.Edit {
		fmt.Fprintf(&out, "Now the edit. These are your clips as they are cut, measured once what "+
			"you left out is gone:\n\n")
	} else {
		fmt.Fprintf(&out, "These clips are well off %s to %s seconds. Measured, once what you left "+
			"out is gone:\n\n", lo, hi)
	}
	for _, entry := range held {
		seconds := b.seconds(entry.Keep)
		off := ""
		switch {
		case seconds > b.opts.MaxLen:
			off = fmt.Sprintf(", %s over the %s second maximum", fixed(seconds-b.opts.MaxLen, 0), hi)
		case seconds < b.opts.MinLen:
			off = fmt.Sprintf(", %s under the %s second minimum", fixed(b.opts.MinLen-seconds, 0), lo)
		}
		keep := toUnits(entry.Keep, b.units)
		fmt.Fprintf(&out, "- %q runs %s seconds%s. It keeps %s.\n", entry.Slug,
			fixed(seconds, 0), off, jsonRuns(keep))
		first, last := keep[0][0], keep[len(keep)-1][1]
		var around []string
		for n := max(first-fitAround, 1); n <= min(last+fitAround, len(b.units)); n++ {
			u := b.units[n-1]
			around = append(around, fmt.Sprintf("[%d] %s", n,
				fixed(b.seconds([][2]int{{u[0] + 1, u[1] + 1}}), 1)))
		}
		fmt.Fprintf(&out, "  Seconds of each %s around it: %s\n", unit, strings.Join(around, " "))
	}
	if recipe.Edit {
		fmt.Fprintf(&out, "\nFor each clip, read where it starts and where it ends. It must open on the "+
			"%s that lets a stranger follow what is coming, a question or a setup when there is one, "+
			"not in the middle of the story. It must end on the %s that lands it, the payoff, and "+
			"nothing after that does not add to it. Inside, leave out what the story holds without: "+
			"asides, restarts, a second example. Each must end up %s to %s seconds. Move what is "+
			"not right and keep what is. Give every clip again, in this order and with the same "+
			"slugs. Reply with the JSON object and nothing else.", unit, unit, lo, hi)
		return out.String()
	}
	fmt.Fprintf(&out, "\nGive these clips again, only these, in this order and with the same slugs. "+
		"Shorten one that is too long by leaving out %ss between its opening and its payoff: "+
		"asides, restarts, a second example. Keep the payoff. Lengthen one that is too short "+
		"with the %ss just before or after it that belong to the same moment. "+
		"Reply with the JSON object and nothing else.", unit, unit)
	return out.String()
}

// fit asks for the clips held back once more, with ask, and hands to the
// framers, for each, whichever of the two is nearer the length. An answer
// that leaves a clip out, or gives another clip in its place, is asked
// once more about the clips it left out. Without an ask, or when it fails,
// the clips go as they came.
func (b *planBuilder) fit(ask func(request string, count int) (string, error)) {
	held := b.holding()
	if len(held) == 0 {
		return
	}
	b.mu.Lock()
	slugs := map[string]bool{}
	for _, entry := range b.order {
		slugs[entry.Slug] = true
	}
	b.mu.Unlock()
	answers := make([]*PlanEntry, len(held))
	for round := 0; ask != nil && round < 2; round++ {
		var asked []PlanEntry
		var places []int
		for i, entry := range held {
			if answers[i] == nil {
				asked = append(asked, entry)
				places = append(places, i)
			}
		}
		if len(asked) == 0 {
			break
		}
		request := b.fitRequest(asked)
		if round > 0 {
			request += " Give only " + quotedSlugs(asked) + ", no other clip."
		}
		reply, err := ask(request, len(asked))
		var again []PlanEntry
		if err == nil {
			again, err = b.readFit(reply)
		}
		for n, i := range places {
			if found, ok := fitFor(asked[n], n, again, slugs); ok {
				answers[i] = &found
			}
		}
		if err != nil {
			b.e.Log.Warn("the clips that do not fit could not be asked for again, so they "+
				"stay as they are: %s", err)
			break
		}
		for n, i := range places {
			if answers[i] != nil {
				continue
			}
			var gave []string
			for _, a := range again {
				gave = append(gave, fmt.Sprintf("%q", a.Slug))
			}
			said := "nothing"
			if len(gave) > 0 {
				said = strings.Join(gave, ", ")
			}
			next := "so it is asked for once more"
			if round > 0 {
				next = "so it stays as it was"
			}
			b.e.Log.Info("%s was not in the answer, which gave %s, %s", asked[n].Slug, said, next)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held = nil
	for i, entry := range held {
		chosen := entry
		if found := answers[i]; found != nil {
			was, now := b.seconds(entry.Keep), b.seconds(found.Keep)
			// Taking in its neighbours may run a clip into another one.
			_, repeats := sameMomentAs(b.entries, *found)
			// An edit is taken unless it runs further off the length. A fit
			// is taken only when it comes nearer.
			better, done := b.distance(now) < b.distance(was), "fitted"
			if b.opts.recipe().Edit {
				better, done = b.distance(now) <= b.distance(was), "edited"
			}
			switch {
			case repeats || !better:
				b.e.Log.Detail("%s kept as it was: asked again, it came back at %ss",
					entry.Slug, fixed(now, 1))
			case fmt.Sprint(found.Keep) == fmt.Sprint(entry.Keep):
				b.e.Log.Detail("%s kept as it was", entry.Slug)
			default:
				chosen.Keep = found.Keep
				b.e.Log.Info("%s %s, %s rather than %s, %ss rather than %ss", entry.Slug, done,
					jsonRuns(toUnits(found.Keep, b.units)), jsonRuns(toUnits(entry.Keep, b.units)),
					fixed(now, 1), fixed(was, 1))
			}
		}
		if b.closed {
			return
		}
		b.queueLocked(chosen)
	}
}

// quotedSlugs names clips by their slugs, the way the model wrote them.
func quotedSlugs(entries []PlanEntry) string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = fmt.Sprintf("%q", entry.Slug)
	}
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// readFit reads the model's answer about the clips held back.
func (b *planBuilder) readFit(reply string) ([]PlanEntry, error) {
	data, _, err := ExtractJSONObject(reply, "clips")
	if err != nil {
		return nil, err
	}
	again, _, err := ValidatePlan(data, b.units)
	for i := range again {
		again[i] = b.shaped(again[i])
	}
	return again, err
}

// fitFor finds the clip given again for entry: the one with its slug, or
// failing that the one in its place, unless that one carries the slug of
// another clip of the answer. Asked about one clip, a model sometimes gives
// another from its first answer, and that says nothing about this one.
func fitFor(entry PlanEntry, place int, again []PlanEntry, slugs map[string]bool) (PlanEntry, bool) {
	for _, a := range again {
		if a.Slug == entry.Slug {
			return a, true
		}
	}
	if place < len(again) && !slugs[again[place].Slug] {
		return again[place], true
	}
	return PlanEntry{}, false
}

// distance is how far a length is from what counts as fitting.
func (b *planBuilder) distance(seconds float64) float64 {
	return math.Max(0, math.Max(b.opts.MinLen*0.9-seconds, seconds-b.opts.MaxLen*1.2))
}

// toUnits turns runs of lines, numbered from 1, back into runs of the
// units that hold them, numbered from 1. It is toLines the other way.
func toUnits(keep [][2]int, units [][2]int) [][2]int {
	find := func(line int) int {
		for n, u := range units {
			if line-1 >= u[0] && line-1 <= u[1] {
				return n + 1
			}
		}
		return 1
	}
	out := make([][2]int, len(keep))
	for i, run := range keep {
		out[i] = [2]int{find(run[0]), find(run[1])}
	}
	return out
}

func jsonRuns(runs [][2]int) string {
	parts := make([]string, len(runs))
	for i, r := range runs {
		parts[i] = fmt.Sprintf("[%d, %d]", r[0], r[1])
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
