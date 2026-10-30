package engine

import (
	"math"
	"sort"

	"framefairy/engine/timed"
)

// Where the model has already looked. A search covers the window it was
// made over, and a window searched again is another pass over it, told
// which lines are clips already so it brings other moments, see PassName
// and PlanOptions.Taken. No part is ever taken or given back: any window
// can be searched, as often as anyone likes.

// MergeWindows puts windows in order and joins the ones that touch.
func MergeWindows(in []timed.Window) []timed.Window {
	if len(in) == 0 {
		return nil
	}
	sorted := append([]timed.Window(nil), in...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Start < sorted[b].Start })
	out := []timed.Window{sorted[0]}
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
	timed.Window
	Times int
}

// SearchPasses cuts an episode into the parts that have been read by the
// same number of searches, from its start to its end, the parts nobody
// has searched too, with 0. Search looks where the fewest searches have
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
				out = append(out, Pass{timed.Window{Start: at, End: e.at}, times})
			}
			at = e.at
		}
		times += e.delta
	}
	return out
}

// NextWindow is the window the app chooses by itself: where the fewest
// searches have been, earliest first, and at most one window of it, size
// long, see SuggestedWindow. The first round walks the episode from its
// start, and when every part has been searched once the second starts
// over at the start, and so on, so Search always does the same thing and
// never runs out. A part shorter than least, too short to hold a clip, is
// passed over.
func NextWindow(passes []Pass, duration, size, least float64) timed.Window {
	var room *Pass
	for i, p := range passes {
		if p.End-p.Start < math.Max(least, 0.5) {
			continue
		}
		if room == nil || p.Times < room.Times {
			room = &passes[i]
		}
	}
	if room == nil {
		return timed.Window{Start: 0, End: duration}
	}
	// A part only a little longer than a window is taken whole, rather
	// than leaving a scrap behind that is too short to search.
	span := room.End - room.Start
	if span > size*1.5 {
		span = size
	}
	return timed.Window{Start: room.Start, End: room.Start + span}
}

// FollowingWindow is where the window goes after a search found its
// clips: on from where the search ended, length long, the length the
// window was made, so a window made a minute long stays a minute long and
// the episode is walked in the steps chosen. The last one is cut at the
// end of the episode, and is only cut: the window after it is length long
// again. At the end of the episode it starts over at the start, and a
// scrap at the end shorter than least, too short for a clip, is passed
// over the same way.
func FollowingWindow(ended, length, duration, least float64) timed.Window {
	from := ended
	if duration-from < math.Max(least, 0.5) {
		from = 0
	}
	return timed.Window{Start: from, End: math.Min(from+math.Max(length, 0), duration)}
}

// placeSteps are the round steps a window the app places lands on, the
// same as the range picker's, see gridStep in frontend/src/lib/flow.ts.
var placeSteps = []float64{1, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800}

// PlaceStep is the round step a window the app places lands on: the
// smallest still about a 180th of the episode, two minutes for four hours,
// 30 seconds for one, five for six minutes. The range picker draws its
// lines on one of the same steps, finer the wider it is, so the edges of
// such a window lie on its lines from about 1440 pixels wide up, and on a
// narrower one on most of them. It depends on the episode alone, never on
// how wide the app is, so the window is the same whoever places it. It
// was once the range picker's own step, and the first search, placed
// here, could never agree with it.
func PlaceStep(duration float64) float64 {
	for _, step := range placeSteps {
		if step >= duration/180 {
			return step
		}
	}
	return placeSteps[len(placeSteps)-1]
}

// OnStep puts a window the app placed on the step: its start, and its
// length, which is never less than one step. The ends of the episode win
// over the step, and an end that would leave a scrap too short for a
// clip, least, or shorter than a step, takes it in rather than leaving it
// behind. The episode's own windows are an even share of it, 30:02 of a
// little over four hours, and a window that long lay a few seconds off
// the range picker's lines, which shows as a pixel.
func OnStep(w timed.Window, length, duration, step, least float64) timed.Window {
	from := math.Min(math.Max(math.Round(w.Start/step)*step, 0), duration)
	size := math.Max(math.Round(length/step)*step, step)
	to := math.Min(from+size, duration)
	if duration-to < math.Max(least, step) {
		to = duration
	}
	return timed.Window{Start: from, End: to}
}
