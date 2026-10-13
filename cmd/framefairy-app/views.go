package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// ClipEntry is one clip of any clip set of an episode, with the left edge of
// its crop resolved for every segment, as the render will place it.
type ClipEntry struct {
	engine.ClipView
	Key       string `json:"key"`
	Plan      string `json:"plan"`
	CropLefts []int  `json:"cropLefts"`
}

// Clips lists the clips of every clip set of an episode, in time order.
func (s *FrameFairy) Clips(ctx context.Context, path string) ([]ClipEntry, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return nil, err
	}
	out := []ClipEntry{}
	for _, summary := range plansOf(s.store.files, path) {
		for _, c := range summary.View().Clips {
			out = append(out, s.entryOf(summary.Path, c, info))
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out, nil
}

// plansOf lists the plans of an episode, each read once. It is what the
// clip list, the coverage and the edits over every plan need, and all
// they need: they used to ask engine.Status, which also parses the whole
// transcript, 71 ms on an episode of four hours, for every click.
func plansOf(fsys afero.Fs, path string) []engine.PlanSummary {
	return engine.PlanSummaries(fsys, engine.LogsDir(path))
}

// cropWidth is how wide the crop of a short is in the episode's pixels,
// of a piece that is not rotated.
func (s *FrameFairy) cropWidth(info engine.SourceInfo) int {
	o := s.store.Settings().options()
	cw, _ := engine.CropWindow(info, o.Width, o.Height)
	return cw
}

// entryOf is a clip of the plan at plan as the clip list holds it. A
// rotated piece has its crop on the picture rotated, so its crop is worked
// out on that, see SourceView.TurnedCropWidth.
func (s *FrameFairy) entryOf(plan string, c engine.ClipView, info engine.SourceInfo) ClipEntry {
	entry := ClipEntry{ClipView: c, Key: filepath.Base(plan) + "/" + c.ID, Plan: plan}
	for _, seg := range c.Segments {
		upright := info.Rotated(seg.Rotate)
		cw := s.cropWidth(upright)
		entry.CropLefts = append(entry.CropLefts, engine.ClampCropX(seg.CropX, cw, upright.Width))
	}
	return entry
}

// CoverageView says how many searches have read each part of an episode.
// It decides nothing about where a search may go: any window can be
// searched, as often as anyone likes.
type CoverageView struct {
	// Passes is the whole episode in parts, each with how many searches
	// have read it, see engine.SearchPasses. New goes by it.
	Passes []PassView `json:"passes"`
}

// PassView is a part of an episode and how many searches have read it.
type PassView struct {
	From  float64 `json:"from"`
	To    float64 `json:"to"`
	Times int     `json:"times"`
}

// Coverage gives the episode in parts by how many searches have read
// each, see CoverageView.
func (s *FrameFairy) Coverage(ctx context.Context, path string) (CoverageView, error) {
	if !s.store.Known(path) {
		return CoverageView{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return CoverageView{}, err
	}
	out := CoverageView{Passes: []PassView{}}
	for _, p := range engine.SearchPasses(plansOf(s.store.files, path), info.Duration) {
		out.Passes = append(out.Passes, PassView{From: p.Start, To: p.End, Times: p.Times})
	}
	return out, nil
}

// Captions gives the captions of one clip, on the clip's own clock and in
// the look the render draws them in, so the interface can lay them over the
// picture while the clip plays.
func (s *FrameFairy) Captions(path, planPath, clipID string) (*engine.CaptionsView, error) {
	if !s.store.PlanOf(path, planPath) {
		return nil, errNotInLibrary
	}
	t, err := s.words(path)
	if err != nil {
		return nil, err
	}
	view, err := engine.ClipCaptionsView(s.store.files, planPath, clipID, t, s.captionLook(path, planPath))
	captionTracks.keep(path, view)
	return view, err
}

// captionLook is how the captions of the clips of planPath of the episode
// at path are made, the way a render job makes them: in the style the
// job's options put on the plan, on the episode's frames, see
// engine.ClipDrawing. The video preview once took the style apart from
// the render and the clip off the frames, and lit some words a frame from
// where the short lights them.
func (s *FrameFairy) captionLook(path, planPath string) *engine.Look {
	plan, _, _ := engine.LoadClips(s.store.files, planPath)
	style, err := s.store.Settings().options().CaptionLook(plan)
	if err != nil {
		theLog.line(zerolog.WarnLevel, "captions").Err(err).Msg("the settings' caption look is not one the render takes")
		style = plan.CaptionStyle()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source, err := s.probe(ctx, path)
	if err != nil {
		theLog.line(zerolog.WarnLevel, "captions").Err(err).Msg("the video's frames are not known, so its captions are timed on the plan's clock")
	}
	return &engine.Look{Style: style, Source: source}
}

// ArrivingCaptions are the captions of a clip on its way, the nth of a
// job, laid out the way they will be once it is written, in the style of
// the clip set it goes into. Nil until the job knows what the clip keeps.
// The workspace draws them on the clip timeline while the crop is placed.
func (s *FrameFairy) ArrivingCaptions(jobID string, n int) (*engine.CaptionsView, error) {
	for _, j := range s.jobs.list() {
		if j.ID != jobID {
			continue
		}
		if !s.store.Known(j.Episode) {
			return nil, errNotInLibrary
		}
		logs := filepath.Join(engine.WorkDir(j.Episode), "logs")
		plan := filepath.Join(logs, engine.HandPlanName)
		if j.Kind == engine.JobSearch {
			// The search's own record says which pass of its window it
			// is, and so which plan it writes.
			rec := engine.JobRecord{From: j.From, To: j.To}
			if r := engine.ReadSearch(s.store.files, j.Episode); r != nil {
				rec = *r
			}
			duration := rec.To
			if info, err := s.probe(context.Background(), j.Episode); err == nil {
				duration = info.Duration
			}
			plan = filepath.Join(logs, rec.PlanName(duration))
		}
		for _, u := range j.Underway {
			if u.N == n {
				t, err := s.words(j.Episode)
				if err != nil {
					return nil, err
				}
				view := engine.ArrivingCaptionsView(s.store.files, plan, u, t, s.captionLook(j.Episode, plan))
				captionTracks.keep(j.Episode, view)
				return view, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// Fonts are the faces the captions can be written in. They travel with the
// program, so every one of them renders on any machine.
func (s *FrameFairy) Fonts() []engine.CaptionFont { return engine.CaptionFonts() }

// clipEntry is one clip as it is now, read from its own plan alone. An
// edit answers with it, and it used to be found in the whole clip list,
// which read every plan of the episode and its transcript for one clip.
func (s *FrameFairy) clipEntry(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	view, err := engine.ReadPlan(s.store.files, plan)
	if err != nil {
		return ClipEntry{}, err
	}
	for _, c := range view.Clips {
		if c.ID == clipID {
			return s.entryOf(plan, c, info), nil
		}
	}
	return ClipEntry{}, fmt.Errorf("the clip %s is not in this video's clips any more: %w", clipID, os.ErrNotExist)
}
