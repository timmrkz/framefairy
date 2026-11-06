package engine

import "math"

// What a search asks for when nobody has said: how long its window is, how
// many clips it looks for in it, and how long the local model may think.
// All three follow the episode and the window, so a new episode shows its
// first clips as soon as it can, and a window drawn by hand gets as many
// clips and as much thought as its length is worth. The workspace shows
// what they come to for its window, which the app sends it, see Limits.

// The window grows with the episode, but more slowly: as the square root
// of its length. An episode twice as long gets windows about 1.4 times as
// long, and about 1.4 times as many of them, so a longer episode waits a
// little longer for its first clip, in step with the whole of its work.
// windowFactor sets where it stands: four hours make windows of half an
// hour, one hour of 15 minutes, and half an hour of about 10. Tim chose
// those three.
const windowFactor = 3.75

// leastWindow is the shortest window an episode is cut into, so a short
// video is not cut into scraps, and one this short or shorter is searched
// whole.
const leastWindow = 10 * 60

// windowLengths are the lengths the square root rule is rounded to,
// lengths a person reads at a glance: four hours still get half an hour,
// one hour 15 minutes and half an hour 10, and nothing between them gets
// an odd length like 23 minutes. None is shorter than leastWindow, and
// half an hour lies on the range picker's lines of a long video.
var windowLengths = []float64{10 * 60, 15 * 60, 20 * 60, 30 * 60}

// SuggestedWindow is how long each window of an episode is, in seconds:
// the square root rule rounded to the nearest of windowLengths, nearest
// as a share of it, so 24 minutes is nearer 20 than 30 and 25 nearer 30.
// The episode is walked in windows this long from its start, so each
// starts on a multiple of it, and the last one takes what is left, see
// NextWindow and OnStep.
func SuggestedWindow(duration float64) float64 {
	if duration <= leastWindow {
		return math.Max(duration, 0)
	}
	ideal := math.Sqrt(windowFactor*duration/60) * 60
	best := windowLengths[0]
	for _, length := range windowLengths[1:] {
		if math.Abs(math.Log(ideal/length)) < math.Abs(math.Log(ideal/best)) {
			best = length
		}
	}
	return best
}

// clipsApart is how many clip lengths of a half hour window there are for
// each clip asked for. The model gives fewer clips when fewer moments are
// strong enough, and with 8 asked of a half hour it gave 6, so a half hour
// is asked for 6: one clip for every twelve clip lengths. Longer clips need
// longer moments, so fewer of them are in the same window.
//
// Other windows are asked for in proportion to the square root of their
// length, as the windows themselves grow. In proportion to the length
// itself, six minutes were asked for one clip, which Tim found far too
// few: a short window is still worth a choice, and it holds a dozen clips
// of 25 seconds. With the square root it is asked for 3, ten minutes for
// 3 and an hour for 8.
const (
	clipsApart = 12
	clipsFor   = 30 * 60
)

// SuggestedCount is how many clips a search looks for in a window of the
// given length, in seconds, with clips from least to most seconds long.
// Never fewer than one.
func SuggestedCount(window, least, most float64) int {
	clip := (least + most) / 2
	if clip <= 0 || window <= 0 {
		return 1
	}
	return max(int(math.Round(math.Sqrt(window*clipsFor)/(clipsApart*clip))), 1)
}

// SuggestedSteps is the length of window, in seconds, at which the count
// SuggestedCount suggests rises past each number, with clips from least
// to most seconds long: the first is the shortest window that suggests 2,
// and so on, up to upTo clips. It is how a hand dragging a window sees
// the count follow it, without a rule of its own, see WindowLimits.Steps.
// Each length is found from SuggestedCount itself, to the millisecond,
// so the two never disagree.
func SuggestedSteps(least, most float64, upTo int) []float64 {
	var steps []float64
	low := 0.0
	for n := 2; n <= upTo; n++ {
		high := math.Max(low, 1)
		for SuggestedCount(high, least, most) < n {
			if high > MaxEpisodeSeconds {
				return steps
			}
			high *= 2
		}
		for high-low > 0.001 {
			mid := (low + high) / 2
			if SuggestedCount(mid, least, most) >= n {
				high = mid
			} else {
				low = mid
			}
		}
		steps = append(steps, high)
		low = high
	}
	return steps
}

// ThinkForWindow asks for the thinking budget that fits the window, see
// SuggestedThink. It is the default, and what a run is given unless a
// number is.
const ThinkForWindow = -2

// The local model thinks in proportion to the window it reads: weighing a
// transcript is work that grows with how much of it there is, and not with
// how many clips are picked from it. DefaultThink was measured for half an
// hour, so a window gets that share of it, and never less than leastThink,
// so a short window is still thought about, nor more than mostThink, so a
// window drawn over hours does not think for minutes before its first
// clip.
const (
	leastThink = 512
	mostThink  = 2 * DefaultThink
)

// SuggestedThink is how many tokens the local model may think about a
// window of the given length, in seconds.
func SuggestedThink(window float64) int {
	n := int(math.Round(DefaultThink * window / (30 * 60)))
	return min(max(n, leastThink), mostThink)
}
