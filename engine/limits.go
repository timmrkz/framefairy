package engine

import "math"

// The limits of a window and of the search asked of it, worked out once
// here, for the app and the command line alike. The app sends them to
// the workspace with the video's state, so the range picker holds a drag
// to them in the frame the hand moves, and the clip settings to theirs,
// without a rule of its own, see docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.

// WindowLimits is what a window of a video may be, and what a search of
// it may ask for.
type WindowLimits struct {
	// Most is the longest a window may be, the length that fits wherever
	// it is drawn, see Reach.Anywhere. The whole video when nothing limits
	// it.
	Most float64 `json:"most"`
	// Least is the shortest: room for the clips typed for it, one after
	// another at their shortest, or for one clip when nothing is typed.
	Least float64 `json:"least"`
	// By is what sets Most, see Room.By.
	By string `json:"by"`
	// Typed is the number of clips typed for this window, 0 when the
	// count follows the window. Suggested is what the window suggests,
	// and Count what a search of it asks for, the one or the other.
	Typed     int `json:"typed"`
	Suggested int `json:"suggested"`
	Count     int `json:"count"`
	// ClipsAtMost is the most clips a search may ask for: as many as fit
	// at their shortest in the longest window. ShortestAtMost is the
	// longest Shortest may be, so Count clips of it still fit.
	ClipsAtMost    int     `json:"clipsAtMost"`
	ShortestAtMost float64 `json:"shortestAtMost"`
	// Steps are the lengths of window at which Suggested rises, see
	// SuggestedSteps, up to ClipsAtMost, so the count a drag suggests
	// is read off them as the hand moves.
	Steps []float64 `json:"steps"`
}

// The bounds of what a hand may type: clips asked for, and how long the
// shortest may be.
const (
	mostClipsAsked   = 30
	leastShortest    = 5
	mostShortest     = 180
	leastWindowFloor = 10
)

// Limits is what a window of length seconds on a video of duration seconds
// may be with the room r, the clip lengths in o and typed clips asked for
// it, 0 for none.
func (o Options) Limits(r RoomAndLines, duration, length float64, typed int) WindowLimits {
	most := duration
	if longest := r.Longest(duration); longest > 0 {
		most = longest
	}
	shortest, longest := o.Min, o.Max
	l := WindowLimits{Most: most, By: r.By, Typed: typed}
	l.ClipsAtMost = max(1, min(mostClipsAsked, int(math.Floor((most+0.01)/math.Max(shortest, 1)))))
	l.Typed = min(typed, l.ClipsAtMost)
	l.Suggested = min(SuggestedCount(length, shortest, longest), l.ClipsAtMost)
	l.Count = l.Suggested
	if l.Typed > 0 {
		l.Count = l.Typed
	}
	l.Least = math.Min(math.Max(float64(max(l.Typed, 1))*shortest, leastWindowFloor), duration)
	l.Steps = SuggestedSteps(shortest, longest, l.ClipsAtMost)
	l.ShortestAtMost = math.Max(leastShortest, math.Min(mostShortest, math.Floor((most+0.01)/float64(max(l.Count, 1)))))
	return l
}

// FitWindow is the window from..to put back inside what it may be: no
// longer than most and no shorter than least, on a video of duration
// seconds. It keeps its start when it can, and gives up the end first,
// because the start is where the window was put.
func FitWindow(from, to, most, least, duration float64) (float64, float64) {
	from = math.Min(math.Max(from, 0), duration)
	to = math.Min(math.Max(to, from), duration)
	to = math.Min(to, from+most)
	if to-from < least {
		to = math.Min(duration, from+least)
		from = math.Max(0, to-least)
	}
	return from, to
}
