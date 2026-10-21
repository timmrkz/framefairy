package main

import (
	"math"
	"sync"

	"github.com/rs/zerolog"
)

// The screen matched to the video while it plays, issue #251.
//
// A screen shows a video evenly only when its refresh is a whole number of
// times the video's frame rate. At 120 Hz a video of 24, 30 or 60 frames a
// second is even, one of 25 or 50 is not: its frames stand 4 and 5
// refreshes by turns, 33 and 42 ms, in the app and in QuickTime alike.
// macOS never changes a screen's rate for a video by itself. Apple's own
// advice is to pick a rate in System Settings that divides evenly by the
// video's, and the screens of a MacBook Pro offer 47.95, 48, 50, 59.94 and
// 60 Hz besides. So while a video plays, the app asks for such a rate for
// itself, and gives the screen back when the play stops. macOS gives it
// back too when the app quits, however it quits, see screenSet.

// screenMode is one rate the window's screen can run at, at the size it
// runs at now, in Hz, and whether it is the one it runs at.
type screenMode struct {
	hz  float64
	now bool
}

// evenOn says whether frames of fps a second stand the same number of
// refreshes each at hz, to a thousandth of a refresh.
func evenOn(hz, fps float64) bool {
	if hz <= 0 || fps <= 0 {
		return false
	}
	n := hz / fps
	return n >= 1 && math.Abs(n-math.Round(n)) < 1e-3
}

// matchRate is the rate to ask for while a video of fps frames a second
// plays: the highest the screen offers that shows it evenly, and 0 when
// the rate it runs at already does or none does.
func matchRate(modes []screenMode, fps float64) float64 {
	best := 0.0
	for _, m := range modes {
		if m.now && evenOn(m.hz, fps) {
			return 0
		}
		if !m.now && evenOn(m.hz, fps) && m.hz > best {
			best = m.hz
		}
	}
	return best
}

// What came of matching the screen to a video, or of giving it back.
const (
	matchNoScreen = iota
	matchEven
	matchNone
	matchSwitched
	matchFailed
	matchGivenBack
	matchNothingToGive
)

// matchMu keeps one switch of the screen at a time: a play that starts as
// another stops asks twice at once.
var matchMu sync.Mutex

// MatchScreen runs the window's screen at a rate that shows a video of
// fps frames a second evenly, while it plays, and gives the screen back
// with fps 0. The log says what the screen offered and what came of it.
func (s *FrameFairy) MatchScreen(fps float64) {
	if s.window == nil {
		return
	}
	matchMu.Lock()
	defer matchMu.Unlock()
	done, from, to, modes := screenMatch(s.window.NativeWindow(), fps)
	level, msg := matchSaid(done)
	hz := make([]float64, 0, len(modes))
	for _, m := range modes {
		hz = append(hz, m.hz)
	}
	theLog.line(level, "window").Float64("fps", fps).Float64("from_hz", from).Float64("to_hz", to).Floats64("screen_hz", hz).Msg(msg)
}

// matchSaid is how what came of it reads in the log.
func matchSaid(done int) (zerolog.Level, string) {
	switch done {
	case matchEven:
		return zerolog.DebugLevel, "the screen already shows the video evenly"
	case matchNone:
		return zerolog.InfoLevel, "the screen offers no rate that shows the video evenly"
	case matchSwitched:
		return zerolog.InfoLevel, "the screen runs at a rate that shows the video evenly while it plays"
	case matchFailed:
		return zerolog.WarnLevel, "the screen would not change its rate for the video"
	case matchGivenBack:
		return zerolog.InfoLevel, "the screen runs at its own rate again"
	case matchNothingToGive:
		return zerolog.DebugLevel, "the screen was not changed, nothing to give back"
	default:
		return zerolog.DebugLevel, "no screen to match the video to"
	}
}
