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
	fmt.Fprintf(&out, "These clips are well off %s to %s seconds. Measured, once what you left "+
		"out is gone:\n\n", lo, hi)
	for _, entry := range held {
		seconds := b.seconds(entry.Keep)
		off := ""
		if seconds > b.opts.MaxLen {
			off = fmt.Sprintf("%s over the %s second maximum", fixed(seconds-b.opts.MaxLen, 0), hi)
		} else {
			off = fmt.Sprintf("%s under the %s second minimum", fixed(b.opts.MinLen-seconds, 0), lo)
		}
		keep := toUnits(entry.Keep, b.units)
		fmt.Fprintf(&out, "- %q runs %s seconds, %s. It keeps %s.\n", entry.Slug,
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
	fmt.Fprintf(&out, "\nGive these clips again, only these, in this order and with the same slugs. "+
		"Shorten one that is too long by leaving out %ss between its opening and its payoff: "+
		"asides, restarts, a second example. Keep the payoff. Lengthen one that is too short "+
		"with the %ss just before or after it that belong to the same moment. "+
		"Reply with the JSON object and nothing else.", unit, unit)
	return out.String()
}

// fit asks for the clips held back once more, with ask, and hands to the
// framers, for each, whichever of the two is nearer the length. Without an
// ask, or when it fails, the clips go as they came.
func (b *planBuilder) fit(ask func(request string, count int) (string, error)) {
	held := b.holding()
	if len(held) == 0 {
		return
	}
	var again []PlanEntry
	if ask != nil {
		reply, err := ask(b.fitRequest(held), len(held))
		if err == nil {
			again, err = b.readFit(reply)
		}
		if err != nil {
			b.e.Log.Warn("the clips that do not fit could not be asked for again, so they "+
				"stay as they are: %s", err)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.held = nil
	for i, entry := range held {
		chosen := entry
		if found, ok := fitFor(entry, i, again); ok {
			was, now := b.seconds(entry.Keep), b.seconds(found.Keep)
			// Taking in its neighbours may run a clip into another one.
			_, repeats := sameMomentAs(b.entries, found)
			if !repeats && b.distance(now) < b.distance(was) {
				chosen.Keep = found.Keep
				b.e.Log.Info("%s fitted, %ss rather than %ss", entry.Slug, fixed(now, 1), fixed(was, 1))
			} else {
				b.e.Log.Detail("%s kept as it was: asked again, it came back at %ss",
					entry.Slug, fixed(now, 1))
			}
		}
		if b.closed {
			return
		}
		b.queueLocked(chosen)
	}
}

// readFit reads the model's answer about the clips held back.
func (b *planBuilder) readFit(reply string) ([]PlanEntry, error) {
	data, _, err := ExtractJSONObject(reply, "clips")
	if err != nil {
		return nil, err
	}
	again, _, err := ValidatePlan(data, b.units)
	for i := range again {
		again[i] = b.joined(again[i])
	}
	return again, err
}

// fitFor finds the clip given again for entry: the one with its slug, or
// failing that the one in its place.
func fitFor(entry PlanEntry, place int, again []PlanEntry) (PlanEntry, bool) {
	for _, a := range again {
		if a.Slug == entry.Slug {
			return a, true
		}
	}
	if place < len(again) {
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
