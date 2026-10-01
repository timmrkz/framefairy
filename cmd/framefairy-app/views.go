package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
	o := s.store.Settings().options()
	cw, _ := engine.CropWindow(info, o.Width, o.Height)
	out := []ClipEntry{}
	for _, summary := range engine.Status(path, s.store.Settings().ASRModel).Plans {
		view, err := engine.ReadPlan(summary.Path)
		if err != nil {
			continue
		}
		for _, c := range view.Clips {
			entry := ClipEntry{ClipView: c, Key: summary.Name + "/" + c.ID, Plan: summary.Path}
			for _, seg := range c.Segments {
				entry.CropLefts = append(entry.CropLefts, engine.ClampCropX(seg.CropX, cw, info.Width))
			}
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out, nil
}

// WindowView is a part of an episode, in seconds. A searched part
// also says which plans cover it and how many clips they hold, so it can be
// let go of again.
type WindowView struct {
	From  float64  `json:"from"`
	To    float64  `json:"to"`
	Plans []string `json:"plans,omitempty"`
	Clips int      `json:"clips,omitempty"`
}

// CoverageView says where the model has already looked and what is left. A
// new window may only be drawn in what is left, so the same material is
// never put to the model twice.
type CoverageView struct {
	Searched []WindowView `json:"searched"`
	Free     []WindowView `json:"free"`
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

// Coverage gives the parts of an episode that have been searched for
// clips and the parts that are still free, leaving out free parts
// too short to hold a clip of least seconds.
func (s *FrameFairy) Coverage(ctx context.Context, path string, least float64) (CoverageView, error) {
	if !s.store.Known(path) {
		return CoverageView{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return CoverageView{}, err
	}
	plans := engine.Status(path, s.store.Settings().ASRModel).Plans
	looked := engine.SearchedPlans(plans, info.Duration)
	searched := make([]engine.Window, 0, len(looked))
	out := CoverageView{Searched: []WindowView{}, Free: []WindowView{}, Passes: []PassView{}}
	for _, p := range engine.SearchPasses(plans, info.Duration) {
		out.Passes = append(out.Passes, PassView{From: p.Start, To: p.End, Times: p.Times})
	}
	for _, w := range looked {
		searched = append(searched, w.Window)
		out.Searched = append(out.Searched,
			WindowView{From: w.Start, To: w.End, Plans: w.Plans, Clips: w.Clips})
	}
	for _, w := range engine.FreeWindows(searched, info.Duration, least) {
		out.Free = append(out.Free, WindowView{From: w.Start, To: w.End})
	}
	return out, nil
}

// RemoveSearch gives a part of an episode back: the clips inside it
// leave the list and the part is free to be searched again. It is a
// part, not a whole search, so a part of what was searched can go while
// the rest of it stays. A plan with nothing left of its window goes
// altogether. Caption files are moved aside rather than deleted, and
// rendered files stay where they are.
//
// It answers with how many clips went.
func (s *FrameFairy) RemoveSearch(ctx context.Context, path string, from, to float64) (int, error) {
	if !s.store.Known(path) {
		return 0, errNotInLibrary
	}
	if to <= from {
		return 0, nil
	}
	// The length is only needed to know when a plan made over the whole
	// episode has nothing left. A file that cannot be read still lets its
	// clips go.
	duration := 0.0
	if info, err := s.probe(ctx, path); err == nil {
		duration = info.Duration
	}
	gone := 0
	err := s.edit(path, func() error {
		for _, plan := range engine.Status(path, s.store.Settings().ASRModel).Plans {
			// A plan of this episode, named the way plans are named.
			// Nothing else is touched, whatever the interface asks for.
			if !s.store.PlanOf(path, plan.Path) {
				continue
			}
			n, err := engine.RemoveRange(plan.Path, from, to, duration)
			if err != nil {
				return err
			}
			gone += n
		}
		return nil
	})
	return gone, err
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
	return engine.ClipCaptionsView(planPath, clipID, t, s.captionOverrides(planPath))
}

// captionOverrides are the settings the render puts on top of a plan's
// caption style, so the picture shows what the file will hold.
func (s *FrameFairy) captionOverrides(planPath string) map[string]any {
	set := s.store.Settings()
	overrides := map[string]any{"margin_v": engine.SnapCaptionY(set.CaptionY)}
	// The highlight colour of the settings is for a plan that was not given
	// one of its own in the captions column, the same as the render has it.
	plan, _, err := engine.LoadClips(planPath)
	if err != nil {
		overrides["highlight_colour"] = set.HighlightColour
	} else if _, own := plan.CaptionStyle()["highlight_colour"]; !own {
		overrides["highlight_colour"] = set.HighlightColour
	}
	return overrides
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
			if r := engine.ReadSearch(j.Episode); r != nil {
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
				return engine.ArrivingCaptionsView(plan, u, t, s.captionOverrides(plan)), nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// Fonts are the faces the captions can be written in. They travel with the
// program, so every one of them renders on any machine.
func (s *FrameFairy) Fonts() []engine.CaptionFont { return engine.CaptionFonts() }

func (s *FrameFairy) clipEntry(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	clips, err := s.Clips(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	for _, c := range clips {
		if c.Plan == plan && c.ID == clipID {
			return c, nil
		}
	}
	return ClipEntry{}, fmt.Errorf("the clip %s is not in this episode's clips any more: %w", clipID, os.ErrNotExist)
}
