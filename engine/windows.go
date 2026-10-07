package engine

import (
	"math"
	"sort"
)

// Where the model has already looked. A search covers the window it was
// made over, and a window searched again is another pass over it, told
// which lines are clips already so it brings other moments, see PassName
// and PlanOptions.Taken. No part is ever taken or given back: any window
// can be searched, as often as anyone likes.

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
		a, b := math.Max(0, w.Start), math.Min(duration, w.End)
		if b > a {
			edges = append(edges, edge{a, 1}, edge{b, -1})
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
