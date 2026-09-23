package engine

import (
	"math"
	"sort"
)

// Where the model has already looked. A search covers the window it was
// made over, and a window is never put to the model twice by accident, so
// the app says what it is about to look at again and asks first.
// Without that, two passes over the same material would come back with the
// same moments, and the list would hold each of them twice.
//
// A window can be given back, in whole or in part: the clips inside it go
// and the plan notes the window as one the model may read again. That is
// why a plan is a window with holes in it rather than a plain window.

// Searched is a part an episode has been searched over, with the plans
// that cover it. Plans that meet or overlap end up in one part.
type Searched struct {
	Window
	Plans []string
	Clips int
}

// readWindows takes the windows out of a plan's planned_with, which is
// untrusted like everything else a plan file says. Anything that is not a
// pair of numbers making a window is left out.
func readWindows(raw any) []Window {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []Window
	for _, item := range list {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		start, okS := toFloat(fields["from"])
		end, okE := toFloat(fields["to"])
		if !okS || !okE || end <= start {
			continue
		}
		out = append(out, Window{start, end})
	}
	return MergeWindows(out)
}

// MergeWindows puts windows in order and joins the ones that touch.
func MergeWindows(in []Window) []Window {
	if len(in) == 0 {
		return nil
	}
	sorted := append([]Window(nil), in...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Start < sorted[b].Start })
	out := []Window{sorted[0]}
	for _, w := range sorted[1:] {
		last := len(out) - 1
		if w.Start <= out[last].End {
			out[last].End = math.Max(out[last].End, w.End)
			continue
		}
		out = append(out, w)
	}
	return out
}

// Without takes the parts out of a window, which leaves the pieces of
// it that are still there.
func Without(w Window, holes []Window) []Window {
	left := []Window{w}
	for _, hole := range MergeWindows(holes) {
		var next []Window
		for _, piece := range left {
			if hole.End <= piece.Start || hole.Start >= piece.End {
				next = append(next, piece)
				continue
			}
			if hole.Start > piece.Start {
				next = append(next, Window{piece.Start, hole.Start})
			}
			if hole.End < piece.End {
				next = append(next, Window{hole.End, piece.End})
			}
		}
		left = next
	}
	return left
}

// SearchedPlans gives the parts an episode has been searched over, in
// order and merged where they meet, each with the plans behind it. A plan
// made without a window covers the whole episode.
func SearchedPlans(plans []PlanSummary, duration float64) []Searched {
	var found []Searched
	for _, p := range plans {
		w := Window{p.From, p.To}
		if w.End <= w.Start {
			w = Window{0, duration}
		}
		w.Start = math.Max(0, w.Start)
		if duration > 0 {
			w.End = math.Min(w.End, duration)
		}
		if w.End <= w.Start {
			continue
		}
		// What was given back is not searched any more, so a plan can leave
		// more than one part behind.
		for _, piece := range Without(w, p.Removed) {
			found = append(found, Searched{Window: piece, Plans: []string{p.Path}, Clips: p.Clips})
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Start < found[b].Start })
	var merged []Searched
	for _, w := range found {
		last := len(merged) - 1
		if last >= 0 && w.Start <= merged[last].End {
			merged[last].End = math.Max(merged[last].End, w.End)
			merged[last].Plans = append(merged[last].Plans, w.Plans...)
			merged[last].Clips += w.Clips
			continue
		}
		merged = append(merged, w)
	}
	return merged
}

// SearchedWindows is the same, as plain parts.
func SearchedWindows(plans []PlanSummary, duration float64) []Window {
	var out []Window
	for _, s := range SearchedPlans(plans, duration) {
		out = append(out, s.Window)
	}
	return out
}

// FreeWindows gives what is left of an episode, the parts nobody has
// searched yet. Anything shorter than least is left out, because a part
// too short to hold a clip is not worth offering.
func FreeWindows(searched []Window, duration, least float64) []Window {
	var free []Window
	at := 0.0
	for _, w := range searched {
		if w.Start > at {
			free = append(free, Window{at, w.Start})
		}
		at = math.Max(at, w.End)
	}
	if duration > at {
		free = append(free, Window{at, duration})
	}
	out := free[:0]
	for _, w := range free {
		if w.End-w.Start >= least {
			out = append(out, w)
		}
	}
	return out
}
