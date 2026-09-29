package engine

import (
	"math"
	"sort"
)

// Where the model has already looked. A search covers the window it was
// made over, and a window searched again is another pass over it, told
// which lines are clips already so it brings other moments, see PassName
// and PlanOptions.Taken.
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
// order and merged where they meet, each with the plans behind it, each
// plan over the part it was made over, see madeOver.
func SearchedPlans(plans []PlanSummary, duration float64) []Searched {
	var found []Searched
	for _, p := range plans {
		w := p.Over(duration)
		w.Start = math.Max(0, w.Start)
		if duration > 0 {
			w.End = math.Min(w.End, duration)
		}
		// Nothing, for the clips made by hand, and nothing to go by for a
		// search of the whole of an episode whose length is not known.
		if w.End <= w.Start || math.IsInf(w.End, 1) {
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

// Pass is a part of an episode and how many searches have read it.
type Pass struct {
	Window
	Times int
}

// SearchPasses cuts an episode into the parts that have been read by the
// same number of searches, from its start to its end, the parts nobody
// has searched too, with 0. New searches where the fewest searches have
// been, earliest first, so the first round walks the episode from its
// start and the second starts over when every part has had one.
func SearchPasses(plans []PlanSummary, duration float64) []Pass {
	if duration <= 0 {
		return nil
	}
	type edge struct {
		at    float64
		delta int
	}
	var edges []edge
	for _, p := range plans {
		w := p.Over(duration)
		if math.IsInf(w.End, 1) || w.End <= w.Start {
			continue
		}
		for _, piece := range Without(w, p.Removed) {
			a, b := math.Max(0, piece.Start), math.Min(duration, piece.End)
			if b > a {
				edges = append(edges, edge{a, 1}, edge{b, -1})
			}
		}
	}
	edges = append(edges, edge{duration, 0})
	sort.SliceStable(edges, func(a, b int) bool { return edges[a].at < edges[b].at })
	var out []Pass
	at, times := 0.0, 0
	for _, e := range edges {
		if e.at > at {
			if n := len(out); n > 0 && out[n-1].Times == times {
				out[n-1].End = e.at
			} else {
				out = append(out, Pass{Window{at, e.at}, times})
			}
			at = e.at
		}
		times += e.delta
	}
	return out
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
