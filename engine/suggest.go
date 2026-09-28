package engine

import "math"

// What a search asks for when nobody has said: how long its window is, how
// many clips it looks for in it, and how long the local model may think.
// All three follow the episode and the window, so a new episode shows its
// first clips as soon as it can, and a window drawn by hand gets as many
// clips and as much thought as its length is worth. The workspace works
// the same out while a window is dragged, in frontend/src/lib/suggest.ts,
// and both are held to the same table of cases in their tests.

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

// SuggestedWindow is how long each window of an episode is, in seconds.
// The episode is cut into equal windows as close to the square root rule
// as divide it evenly, so the last is as long as the others and no scrap is
// left at the end. None is shorter than leastWindow.
func SuggestedWindow(duration float64) float64 {
	if duration <= leastWindow {
		return math.Max(duration, 0)
	}
	ideal := math.Max(math.Sqrt(windowFactor*duration/60)*60, leastWindow)
	n := math.Max(math.Round(duration/ideal), 1)
	if duration/n < leastWindow {
		n = math.Max(math.Floor(duration/leastWindow), 1)
	}
	return duration / n
}

// clipsApart is how many clip lengths of window there are for each clip
// asked for. The model gives fewer clips when fewer moments are strong
// enough, and with 8 asked of a half hour it gave 6, so it is asked for 6:
// one clip for every twelve clip lengths, about 8 % of the window. Longer
// clips need longer moments, so fewer of them are in the same window.
const clipsApart = 12

// SuggestedCount is how many clips a search looks for in a window of the
// given length, in seconds, with clips from least to most seconds long.
// Never fewer than one.
func SuggestedCount(window, least, most float64) int {
	clip := (least + most) / 2
	if clip <= 0 || window <= 0 {
		return 1
	}
	return max(int(math.Round(window/(clipsApart*clip))), 1)
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
