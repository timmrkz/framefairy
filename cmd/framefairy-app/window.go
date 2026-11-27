package main

import (
	"context"
	"errors"
	"math"
	"path/filepath"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/engine/timed"
)

// The window of a video: the part of it a search reads, drawn on the
// range picker. A video has one, kept in chosen.json beside the clip
// worked on, and this side is its one owner. It places the window where
// nobody has, every search starts on it, the first one of a video just
// added too, and a search that found its clips moves it on. The workspace
// draws it, and writes it only when a hand moves it or when it no longer
// fits what a search can read. It was once placed twice, by the first
// search here and by the workspace on the range picker's step, and the
// two lay two seconds apart, so a search cancelled on one was set aside by
// the other, Tim's report of 24 October on #458.

// WindowView is the video's window as the workspace draws it, sent in
// its state, with what it and the search asked of it may be, so the
// range picker holds a drag to its limits in the frame the hand moves
// and the clip settings to theirs, and decides none of it.
type WindowView struct {
	KeptWindow
	engine.WindowLimits
	// Shortest and Longest are how long the clips a search of it asks
	// for may be, in seconds, as they are set.
	Shortest float64 `json:"shortest"`
	Longest  float64 `json:"longest"`
	// Searched is that a search has read some of the window before, see
	// engine.Searched, so Search looks there again and keeps the clips
	// there are, which its title says.
	Searched bool `json:"searched,omitempty"`
	// Heard is how many seconds of the window are heard, saved or still
	// being heard, see HeardView.Heard, and HeardAll that all of it is,
	// but the half second an edge may be off by, so a search of it finds
	// at once rather than hearing first.
	Heard    float64 `json:"heard"`
	HeardAll bool    `json:"heardAll,omitempty"`
}

// windowFor is the window of the video whose state st is, on a video of
// duration seconds, as the engine says it is, see engine.Episode.Window:
// the window kept, fitted to what a search can do with it whenever that
// changed, the room or the clips asked for, or placed where none is kept.
// Not fitted while a search runs on it, because then it is the window
// being searched, nor while a search stands stopped on it, because it is
// the window Continue carries on. A window placed or fitted is kept, so
// the window drawn is the video's window. nil while the video cannot be
// read.
func (s *FrameFairy) windowFor(ctx context.Context, path string, st VideoState, duration float64) *WindowView {
	if duration <= 0 {
		return nil
	}
	if info, err := s.probe(ctx, path); err != nil || info.Duration <= 0 {
		return nil
	}
	o := s.store.Settings().options()
	ep := s.store.machine.Episode(path)
	r := ep.Room(o)
	typed := func(length float64) int { return s.store.Settings().targetFor(length) }
	held := searchHolds(st.Jobs)
	passes := engine.SearchPasses(st.Status.Plans, duration)
	view := func(w engine.PlacedWindow, l engine.WindowLimits) *WindowView {
		heard := st.Heard.Heard
		return &WindowView{KeptWindow: keptOf(w), WindowLimits: l, Shortest: o.Min, Longest: o.Max,
			Searched: engine.Searched(passes, w.Window),
			Heard:    engine.HeardOf(heard, w.Start, w.End), HeardAll: engine.HeardOver(heard, w.Start, w.End)}
	}
	// Only the window it was worked out from is changed. One written
	// meanwhile, by a hand or by another read that kept one first, is the
	// window kept, and it is worked out in its turn, so the state carries
	// the window kept, never the one read before it. A hand that moved it
	// is the one that changes it, so this ends the moment no other call
	// writes the window between the read and the write.
	picked := s.chosenIn(path)
	kept := picked.get(s.store.files, path).Window
	for {
		w, l := ep.Window(o, r, duration, placedOf(kept), held, typed)
		var was *KeptWindow
		if kept != nil {
			// Cut at the video's end the way the engine reads it, so a
			// kept window that runs a little past the end is still the
			// one read.
			k := *kept
			k.To = math.Min(k.To, duration)
			if sameWindow(keptOf(w), k) {
				return view(w, l)
			}
			was = &k
		}
		wrote, now := false, kept
		if _, err := picked.change(s.store.files, path, func(c *chosen) bool {
			if (c.Window == nil) != (kept == nil) || (kept != nil && *c.Window != *kept) {
				now = c.Window
				return false
			}
			k := keptOf(w)
			c.Window, wrote = &k, true
			return true
		}); err != nil {
			theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
				Msg("the window the engine placed or fitted could not be kept")
			return view(w, l)
		}
		if wrote {
			if s.noteWindow(path, was, w, l) {
				// The target typed follows the window fitted, so it asks
				// for the clips typed for it.
				l = o.Limits(r, duration, w.End-w.Start, typed(w.End-w.Start))
			}
			return view(w, l)
		}
		kept = now
	}
}

// noteWindow logs a window the engine placed or fitted and kept, and
// lets a target typed for the window follow it: a target typed for the
// window is the target of the window fitted, most often to the clips it
// asked for, when it was typed on this video. A window of another video
// as long only asks for it. It says whether the target followed.
func (s *FrameFairy) noteWindow(path string, was *KeptWindow, w engine.PlacedWindow, l engine.WindowLimits) bool {
	if was == nil {
		theLog.line(zerolog.InfoLevel, "window").Str("video", filepath.Base(path)).
			Float64("from", w.Start).Float64("to", w.End).Msg("the window was placed")
		return false
	}
	followed := false
	if s.store.Settings().targetFor(was.To-was.From) > 0 {
		if err := s.store.UpdateSettings(func(set *Settings) {
			if set.targetFor(was.To-was.From) > 0 && set.TargetVideo == path {
				set.TargetWindow = w.End - w.Start
				followed = true
			}
		}); err != nil {
			theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
				Msg("the target typed could not follow the window fitted")
		}
	}
	theLog.line(zerolog.InfoLevel, "window").Str("video", filepath.Base(path)).
		Float64("from", w.Start).Float64("to", w.End).Float64("most", l.Most).Float64("least", l.Least).
		Msg("the window was fitted to what a search can read")
	return followed
}

// searchHolds says whether a search of the video holds its window: one
// that runs, or one that stopped on it and is not set aside.
func searchHolds(jobs []Job) bool {
	for _, j := range jobs {
		if j.Kind == engine.JobSearch && (j.going() || (j.unfinished() && !j.Aside)) {
			return true
		}
	}
	return false
}

// wholeJob is the video's transcription to its end among jobs, while it
// runs or waits its turn and was not told to stop, or nil.
func wholeJob(jobs []Job) *Job {
	for i := range jobs {
		// A part of a clip and the stretch around one are about that clip.
		if j := &jobs[i]; j.Hears() == engine.HearsWhole && j.going() && !j.Stopping {
			return j
		}
	}
	return nil
}

// keptOf is a window the engine placed as the app keeps it.
func keptOf(w engine.PlacedWindow) KeptWindow {
	return KeptWindow{From: w.Start, To: w.End, Length: w.Length}
}

// placedOf is the window the app keeps as the engine reads it, nil for
// none.
func placedOf(k *KeptWindow) *engine.PlacedWindow {
	if k == nil {
		return nil
	}
	return &engine.PlacedWindow{Window: timed.Window{Start: k.From, End: k.To}, Length: k.Length}
}

// searchesWindow makes the window of a search the video's window, as it
// starts. It is already, unless the window was never written: a search
// is started on the window the workspace draws, which is this one.
func (s *FrameFairy) searchesWindow(path string, req engine.PlanRequest) {
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return
	}
	to := req.To
	if to <= 0 {
		to = info.Duration
	}
	w := KeptWindow{From: req.From, To: to, Length: to - req.From}
	if !w.ok() {
		return
	}
	_, _ = s.chosenIn(path).change(s.store.files, path, func(c *chosen) bool {
		if c.Window != nil && sameWindow(*c.Window, w) {
			return false
		}
		c.Window = &w
		return true
	})
}

// windowOn moves the window on after a search found its clips, as long
// as the window was made and no longer than one search can read now,
// which the first search of a video measures from its real speech, see
// engine.Options.WindowAfter. It is done before the search says it is
// done, so the workspace reads the window moved on when it hears that.
// It is worked out before the kept window is changed, and moves it on
// only if it is still the one it was worked out from: a window written
// meanwhile has the last word, a hand's drag or the engine's fit of it
// to the room, see windowFor.
func (s *FrameFairy) windowOn(path string, req engine.PlanRequest) {
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return
	}
	picked := s.chosenIn(path)
	kept := picked.get(s.store.files, path).Window
	length := 0.0
	if kept != nil {
		length = kept.Length
	}
	o := s.store.Settings().options()
	w := keptOf(o.WindowAfter(s.store.machine.Episode(path).Room(o), req, length, info.Duration))
	if !w.ok() {
		return
	}
	if _, err := picked.change(s.store.files, path, func(c *chosen) bool {
		if (c.Window == nil) != (kept == nil) || (kept != nil && *c.Window != *kept) {
			return false
		}
		c.Window = &w
		return true
	}); err != nil {
		theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
			Msg("the window could not be moved on after the search")
	}
}

// PlaceWindow puts the window back where the app would place it, a
// double-click on its marks. It is a hand's step, which undo takes back.
// It answers with the state of the video read after it.
func (s *FrameFairy) PlaceWindow(ctx context.Context, path string) (VideoState, error) {
	if !s.store.Known(path) {
		return VideoState{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil || info.Duration <= 0 {
		return VideoState{}, errors.New("the video cannot be read")
	}
	o := s.store.Settings().options()
	ep := s.store.machine.Episode(path)
	w, _ := ep.Window(o, ep.Room(o), info.Duration, nil, false, nil)
	return s.ChooseWindow(ctx, path, w.Start, w.End, w.Length, true)
}

// sameWindow says whether two windows are the same part of the video. A
// window's length is how long it was made, not where it is, so it does
// not count.
func sameWindow(a, b KeptWindow) bool {
	return math.Abs(a.From-b.From) < 0.001 && math.Abs(a.To-b.To) < 0.001
}

// markAside says of the search of a video that stopped whether it is set
// aside: the window was moved off the window it was about, so its row in
// the clip list closes, Search starts afresh where the window is, and the
// video's dot in the sidebar says nothing of it. It is known here, from
// the one window, so the row and the dot read the same thing. It was once
// worked out in the workspace alone, and the dot stayed in the colour of
// a warning with nothing on screen to say why, issue #436.
func (s *FrameFairy) markAside(path string) {
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return
	}
	kept := s.chosenIn(path).get(s.store.files, path).Window
	s.jobs.setAside(path, func(j *Job) bool {
		if kept == nil {
			return false
		}
		to := j.To
		if to <= 0 {
			to = info.Duration
		}
		return !sameWindow(*kept, KeptWindow{From: j.From, To: math.Min(to, info.Duration)})
	})
}

// restoreJobs reads the jobs of these videos back from their records, as
// the app starts or a video is added again, and says of every search that
// stopped whether it is set aside.
func (s *FrameFairy) restoreJobs(episodes []string) {
	s.jobs.restore(episodes)
	s.markAsideIn(episodes)
}

// markAsideIn says of every search of these videos that stopped whether it
// is set aside. Only a video with such a search is read.
func (s *FrameFairy) markAsideIn(episodes []string) {
	halted := map[string]bool{}
	for _, j := range s.jobs.list() {
		if j.Kind == engine.JobSearch && j.unfinished() {
			halted[j.Episode] = true
		}
	}
	for _, ep := range episodes {
		if halted[ep] {
			s.markAside(ep)
		}
	}
}
