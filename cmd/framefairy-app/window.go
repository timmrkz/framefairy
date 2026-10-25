package main

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"sync"

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

// windowMu keeps the window from being placed twice at once, by the first
// search of a video just added and by its workspace opening.
var windowMu sync.Mutex

// videoWindow is the window of a video, placed first where there is none
// or what is kept no longer fits the video. One that is kept is given
// back as it is while the video cannot be read, and ok is false only
// when there is none and nothing can be placed.
func (s *FrameFairy) videoWindow(ctx context.Context, path string) (KeptWindow, bool) {
	info, err := s.probe(ctx, path)
	windowMu.Lock()
	defer windowMu.Unlock()
	kept := readChosen(s.store.files, path).Window
	if err != nil || info.Duration <= 0 {
		if kept == nil {
			return KeptWindow{}, false
		}
		return *kept, true
	}
	if kept != nil && kept.To <= info.Duration+0.5 {
		w := *kept
		w.To = math.Min(w.To, info.Duration)
		return w, w.To > w.From
	}
	w := s.placedWindow(path, info.Duration)
	if err := changeChosen(s.store.files, path, func(c *chosen) { c.Window = &w }); err != nil {
		theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
			Msg("the window the app placed could not be kept")
	}
	return w, true
}

// placedWindow is where the app puts a video's window, see placeWindow,
// from what was searched of it, its settings and what one search can read
// of it, see windowReach.
func (s *FrameFairy) placedWindow(path string, duration float64) KeptWindow {
	opts := s.store.Settings().options()
	passes := engine.SearchPasses(plansOf(s.store.files, path), duration)
	return placeWindow(passes, duration, opts.Min, s.windowReach(path, duration))
}

// windowReach is the longest window one search can read wherever it is
// drawn, the reach the range picker fits every window to, see
// engine.Reach, from the same room as Room gives the workspace. 0 is no
// limit. It was once the room at the video's average rate, longer than the
// range picker's wherever speech is uneven, so the workspace cut every
// window placed here again and wrote it back off the step, found by the
// fourth review of #458.
func (s *FrameFairy) windowReach(path string, duration float64) float64 {
	opts := s.store.Settings().options()
	p := s.projectWith(path, opts)
	room := engine.SearchRoom(p.Files, opts, p.LogsDir())
	if room.Chars <= 0 {
		return 0
	}
	var lines []engine.LineWeight
	var heard [][2]float64
	rate := engine.SpokenChars
	if t, err := s.transcript(p); err == nil {
		lines = engine.WeighLines(t)
		heard = t.Heard
		rate = engine.RateOf(lines)
	}
	return engine.NewReach(room.Chars, lines, heard, rate, duration).Anywhere()
}

// lengthWithin is a window's length cut to most, what one search can
// read, on a whole number of steps and no shorter than least, or most
// where that is shorter still. most 0 is no limit.
func lengthWithin(size, most, step, least float64) float64 {
	if most > 0 && most < size {
		return math.Max(math.Floor(most/step)*step, math.Min(least, most))
	}
	return size
}

// placeWindow is where the fewest searches have been, earliest first, as
// long as the video's windows are, see engine.NextWindow, no longer than
// most, what one search can read, and no shorter than one clip of
// shortest, on a round step, see engine.OnStep. Its length is a whole
// number of steps, and is what the window walks on with. most 0 is no
// limit.
func placeWindow(passes []engine.Pass, duration, shortest, most float64) KeptWindow {
	least := math.Min(math.Max(shortest, 10), duration)
	size := engine.SuggestedWindow(duration)
	step := engine.PlaceStep(duration)
	size = lengthWithin(size, most, step, least)
	next := engine.NextWindow(passes, duration, size, least)
	placed := engine.OnStep(next, next.End-next.Start, duration, step, least)
	length := math.Max(math.Round(size/step)*step, step)
	return KeptWindow{From: placed.Start, To: placed.End, Length: math.Max(length, placed.End-placed.Start)}
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
	windowMu.Lock()
	defer windowMu.Unlock()
	if kept := readChosen(s.store.files, path).Window; kept != nil && sameWindow(*kept, w) {
		return
	}
	_ = changeChosen(s.store.files, path, func(c *chosen) { c.Window = &w })
}

// windowOn moves the window on after a search found its clips: from
// where it ended, as long as the window was made, see
// engine.FollowingWindow, and no longer than one search can read now,
// which the first search of a video measures from its real speech. It is
// done before the search says it is done,
// so the workspace reads the window moved on when it hears that.
func (s *FrameFairy) windowOn(path string, req engine.PlanRequest) {
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return
	}
	ended := req.To
	if ended <= 0 {
		ended = info.Duration
	}
	windowMu.Lock()
	defer windowMu.Unlock()
	length := ended - req.From
	if kept := readChosen(s.store.files, path).Window; kept != nil && kept.Length > 0 {
		length = kept.Length
	}
	least := math.Min(math.Max(s.store.Settings().Min, 10), info.Duration)
	length = lengthWithin(length, s.windowReach(path, info.Duration), engine.PlaceStep(info.Duration), least)
	next := engine.FollowingWindow(ended, length, info.Duration, least)
	w := KeptWindow{From: next.Start, To: next.End, Length: math.Max(length, next.End-next.Start)}
	if !w.ok() {
		return
	}
	if err := changeChosen(s.store.files, path, func(c *chosen) { c.Window = &w }); err != nil {
		theLog.line(zerolog.WarnLevel, "window").Str("video", filepath.Base(path)).Err(err).
			Msg("the window could not be moved on after the search")
	}
}

// PlaceWindow puts the window back where the app would place it, a
// double-click on its marks. It is a hand's step, which undo takes back.
func (s *FrameFairy) PlaceWindow(path string) (*KeptWindow, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return nil, errors.New("the video cannot be read")
	}
	windowMu.Lock()
	w := s.placedWindow(path, info.Duration)
	windowMu.Unlock()
	if err := s.ChooseWindow(path, w.From, w.To, w.Length, true); err != nil {
		return nil, err
	}
	return &w, nil
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
	kept := readChosen(s.store.files, path).Window
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
		if j.Kind == engine.JobSearch && (j.State == JobInterrupted || j.State == JobFailed) {
			halted[j.Episode] = true
		}
	}
	for _, ep := range episodes {
		if halted[ep] {
			s.markAside(ep)
		}
	}
}
