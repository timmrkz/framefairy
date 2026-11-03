package main

import (
	"context"
	"errors"
	"math"
	"path/filepath"

	"github.com/rs/zerolog"

	"framefairy/engine"
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

// videoWindow is the window of a video, placed first where there is none
// or what is kept no longer fits the video. One that is kept is given
// back as it is while the video cannot be read, and ok is false only
// when there is none and nothing can be placed. The window is placed
// before the kept one is changed, and kept only if nothing put one there
// that fits meanwhile, so the first search of a video just added and its
// workspace opening never place it twice.
func (s *FrameFairy) videoWindow(ctx context.Context, path string) (KeptWindow, bool) {
	info, err := s.probe(ctx, path)
	picked := s.chosenIn(path)
	kept := picked.get(s.store.files, path).Window
	if err != nil || info.Duration <= 0 {
		if kept == nil {
			return KeptWindow{}, false
		}
		return *kept, true
	}
	fits := func(w *KeptWindow) bool { return w != nil && w.To <= info.Duration+0.5 }
	if fits(kept) {
		w := *kept
		w.To = math.Min(w.To, info.Duration)
		return w, w.To > w.From
	}
	w := s.placedWindow(path, info.Duration)
	if _, err := picked.change(s.store.files, path, func(c *chosen) bool {
		if fits(c.Window) {
			w = *c.Window
			return false
		}
		c.Window = &w
		return true
	}); err != nil {
		theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
			Msg("the window the app placed could not be kept")
	}
	w.To = math.Min(w.To, info.Duration)
	return w, w.To > w.From
}

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
}

// windowFor is the window of the video whose state st is, on a video of
// duration seconds, fitted to what a search can do with it whenever that
// changed, the room or the clips asked for. Not while a search runs on
// it, because then it is the window being searched, nor while a search
// stands stopped on it, because it is the window Continue carries on. A
// window fitted is kept, so the window drawn is the video's window. nil
// while the video cannot be read.
func (s *FrameFairy) windowFor(ctx context.Context, path string, st VideoState, duration float64) *WindowView {
	if duration <= 0 {
		return nil
	}
	w, ok := s.videoWindow(ctx, path)
	if !ok {
		return nil
	}
	p := s.project(path)
	r := s.roomAndLines(p)
	limits := func(w KeptWindow) engine.WindowLimits {
		return p.Limits(r, duration, w.To-w.From, s.store.Settings().targetFor(w.To-w.From))
	}
	view := func(w KeptWindow, l engine.WindowLimits) *WindowView {
		return &WindowView{KeptWindow: w, WindowLimits: l, Shortest: p.Base.Min, Longest: p.Base.Max}
	}
	l := limits(w)
	if searchHolds(st.Jobs) {
		return view(w, l)
	}
	from, to := engine.FitWindow(w.From, w.To, l.Most, l.Least, duration)
	fitted := KeptWindow{From: from, To: to, Length: math.Max(w.Length, to-from)}
	if sameWindow(fitted, w) {
		return view(w, l)
	}
	// Only the window it was fitted from is changed. One written
	// meanwhile, by a hand or by another read that fitted it first, is
	// the window kept, so that is the one the state carries, never the
	// one read before it.
	kept, now := false, w
	if _, err := s.chosenIn(path).change(s.store.files, path, func(c *chosen) bool {
		if c.Window == nil {
			return false
		}
		if !sameWindow(*c.Window, w) {
			now = *c.Window
			return false
		}
		c.Window, kept = &fitted, true
		return true
	}); err != nil {
		theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
			Msg("the window fitted to what a search can read could not be kept")
	}
	if !kept {
		now.To = math.Min(now.To, duration)
		return view(now, limits(now))
	}
	// A target typed for the window is the target of the window fitted,
	// most often to the clips it asked for, when it was typed on this
	// video. A window of another video as long only asks for it.
	if l.Typed > 0 {
		if err := s.store.UpdateSettings(func(set *Settings) {
			if set.targetFor(w.To-w.From) > 0 && set.TargetVideo == path {
				set.TargetWindow = to - from
			}
		}); err != nil {
			theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
				Msg("the target typed could not follow the window fitted")
		}
	}
	theLog.line(zerolog.InfoLevel, "window").Str("video", filepath.Base(path)).
		Float64("from", from).Float64("to", to).Float64("most", l.Most).Float64("least", l.Least).
		Msg("the window was fitted to what a search can read")
	return view(fitted, limits(fitted))
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

// placedWindow is where the app puts a video's window, see
// engine.Project.PlaceWindow.
func (s *FrameFairy) placedWindow(path string, duration float64) KeptWindow {
	p := s.project(path)
	return keptOf(p.PlaceWindow(s.roomAndLines(p), duration))
}

// roomAndLines is what one search of the project's video can read, from
// the transcript as far as it is heard. A transcript that cannot be read
// is nothing heard yet. Every state carries the window and what it may
// be, so the lines are weighed once for each transcript read and kept
// with it, see engine.TranscriptRead: weighed for every state, four hours
// of them took most of 55 ms.
func (s *FrameFairy) roomAndLines(p *engine.Project) engine.RoomAndLines {
	h, err := s.heardIn(p)
	if err != nil {
		return p.RoomWith(engine.Weigh(nil))
	}
	return p.RoomWith(h.Weighed())
}

// keptOf is a window the engine placed as the app keeps it.
func keptOf(w engine.PlacedWindow) KeptWindow {
	return KeptWindow{From: w.Start, To: w.End, Length: w.Length}
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
// engine.Project.WindowAfter. It is done before the search says it is
// done, so the workspace reads the window moved on when it hears that.
// It is worked out before the kept window is changed, and moves it on
// only if it is still the one it was worked out from: a window written
// meanwhile has the last word, a hand's drag or the interface's own fit
// of it to the room, which step 4 of #606 moves into Go.
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
	p := s.project(path)
	w := keptOf(p.WindowAfter(s.roomAndLines(p), req, length, info.Duration))
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
	w := s.placedWindow(path, info.Duration)
	return s.ChooseWindow(ctx, path, w.From, w.To, w.Length, true)
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
