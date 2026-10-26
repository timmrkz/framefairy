package engine

import (
	"math"
	"sort"
)

// Reach is how far a window may reach in one search, from the room and the
// weight of every line heard so far, the same sum the range picker makes
// while a window is dragged, see Reach in frontend/src/lib/room.ts. The
// app places its windows with it, so a window it places is one the
// workspace leaves as it is: the app capped its windows at the video's
// average rate and the workspace at its densest stretch, and cut every
// window the app placed again, found by the fourth review of #458.
type Reach struct {
	chars    int
	lines    []LineWeight
	sums     []int
	heard    [][2]float64
	rate     float64
	duration float64
}

// NewReach is the reach of a room of chars characters over lines, with
// heard the parts the transcript has heard, and rate what a second weighs
// outside them.
func NewReach(chars int, lines []LineWeight, heard [][2]float64, rate, duration float64) *Reach {
	sums := make([]int, 1, len(lines)+1)
	for _, l := range lines {
		sums = append(sums, sums[len(sums)-1]+l.Chars)
	}
	return &Reach{chars: chars, lines: lines, sums: sums, heard: heard, rate: rate, duration: duration}
}

// reachHalvings is how many times a bisection halves. Forty halvings of a
// four hour episode are far below a millisecond.
const reachHalvings = 40

func (r *Reach) known() bool { return r.chars > 0 }

// weight is what a window from..to sends. A line the window only touches
// is counted whole, which only ever errs towards a window that fits.
func (r *Reach) weight(from, to float64) float64 {
	if to <= from {
		return 0
	}
	first := sort.Search(len(r.lines), func(i int) bool { return r.lines[i].End > from })
	past := sort.Search(len(r.lines), func(i int) bool { return r.lines[i].Start >= to })
	chars := 0.0
	if past > first {
		chars = float64(r.sums[past] - r.sums[first])
	}
	heard := 0.0
	for _, p := range r.heard {
		heard += math.Max(math.Min(p[1], to)-math.Max(p[0], from), 0)
	}
	if unheard := to - from - heard; unheard > 0 {
		chars += unheard * r.rate
	}
	return chars
}

func (r *Reach) fits(from, to float64) bool {
	return !r.known() || r.weight(from, to) <= float64(r.chars)
}

// longestFrom is the latest end a window starting at from may have.
func (r *Reach) longestFrom(from float64) float64 {
	if r.fits(from, r.duration) {
		return r.duration
	}
	lo, hi := from, r.duration
	for range reachHalvings {
		mid := (lo + hi) / 2
		if r.fits(from, mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	// On a whole second, so the window can be said out loud.
	if whole := math.Floor(lo + 1e-6); whole > from && r.fits(from, whole) {
		return whole
	}
	return lo
}

// Anywhere is the longest window that fits wherever it is drawn, on a
// whole second: the densest stretch decides it. See anywhere in room.ts,
// which says why one length and how the starts are chosen.
func (r *Reach) Anywhere() float64 {
	if !r.known() || r.fits(0, r.duration) {
		return r.duration
	}
	shortest := r.duration
	starts := []float64{0}
	for _, p := range r.heard {
		starts = append(starts, p[0], p[1])
	}
	for _, l := range r.lines {
		starts = append(starts, math.Max(l.Start, l.End-0.001))
	}
	for _, start := range starts {
		if start >= r.duration {
			continue
		}
		if r.fits(start, math.Min(start+shortest, r.duration)) {
			continue
		}
		end := r.longestFrom(start)
		if end >= r.duration {
			continue
		}
		shortest = math.Min(shortest, end-start)
	}
	return math.Floor(shortest)
}
