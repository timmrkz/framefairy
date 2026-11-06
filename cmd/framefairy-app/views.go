package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// ClipEntry is one clip of any clip set of an episode, with the left edge of
// its crop resolved for every segment, as the render will place it.
type ClipEntry struct {
	engine.ClipView
	Key       string `json:"key"`
	Plan      string `json:"plan"`
	CropLefts []int  `json:"cropLefts"`
	// Rev is, on the answer to an edit, the number the edit took from the
	// clock of the jobs once it was written, see VideoState.Rev. The
	// window takes no state of the video read before it. A clip in a
	// VideoState has none.
	Rev uint64 `json:"rev,omitempty"`
}

// Clips lists the clips of every clip set of an episode, in time order.
// The workspace reads it in the state of the video, see State, and calls
// it no more. The walks and the tests ask it.
func (s *FrameFairy) Clips(ctx context.Context, path string) ([]ClipEntry, error) {
	if !s.store.Known(path) {
		return nil, errNotInLibrary
	}
	// A video whose file is gone keeps its clips. Its picture says the
	// file is gone, and the list stays as it was, so nothing is said twice
	// and nothing seems lost. Its crops are fitted to the picture it had,
	// or left as the plan keeps them when the app never saw it.
	info := s.probes.lastOf(path)
	if _, err := s.store.files.Stat(path); err == nil {
		if info, err = s.probe(ctx, path); err != nil {
			return nil, err
		}
	}
	return s.clipsOf(s.plansOf(path), info), nil
}

// clipsOf is the clips of these plans, in time order, their crops fitted
// to the picture info says, see entryOf.
func (s *FrameFairy) clipsOf(plans []engine.PlanSummary, info engine.SourceInfo) []ClipEntry {
	out := []ClipEntry{}
	for _, summary := range plans {
		for _, c := range summary.View().Clips {
			out = append(out, s.entryOf(summary.Path, c, info))
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out
}

// plansOf lists the plans of an episode, as its engine Episode holds
// them, each read again only when its file changed. It is what the clip
// list, the coverage and the edits over every plan need, and all they
// need: they used to ask engine.Status, which also parses the whole
// transcript, 71 ms on an episode of four hours, for every click.
func (s *FrameFairy) plansOf(path string) []engine.PlanSummary {
	return s.store.machine.Episode(path).Plans()
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
		if info.Width == 0 {
			entry.CropLefts = append(entry.CropLefts, keptCrop(seg.CropX))
			continue
		}
		upright := info.Rotated(seg.Rotate)
		cw := s.cropWidth(upright)
		entry.CropLefts = append(entry.CropLefts, engine.ClampCropX(seg.CropX, cw, upright.Width))
	}
	return entry
}

// keptCrop is a crop as the plan keeps it, for a clip with no picture to
// fit it to.
func keptCrop(x *int) int {
	if x == nil {
		return 0
	}
	return *x
}

// CoverageView says how many searches have read each part of an episode.
// It decides nothing about where a search may go: any window can be
// searched, as often as anyone likes.
type CoverageView struct {
	// Passes is the whole episode in parts, each with how many searches
	// have read it, see engine.SearchPasses. Search goes by it.
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
// The workspace reads it in the state of the video, see State, and calls
// it no more. The walks and the tests ask it.
func (s *FrameFairy) Coverage(ctx context.Context, path string) (CoverageView, error) {
	if !s.store.Known(path) {
		return CoverageView{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return CoverageView{}, err
	}
	return coverageOf(s.plansOf(path), info.Duration), nil
}

// coverageOf is how often each part of an episode this long was searched
// by the searches that made these plans.
func coverageOf(plans []engine.PlanSummary, duration float64) CoverageView {
	out := CoverageView{Passes: []PassView{}}
	for _, p := range engine.SearchPasses(plans, duration) {
		out.Passes = append(out.Passes, PassView{From: p.Start, To: p.End, Times: p.Times})
	}
	return out
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
	summary, err := s.heldPlan(path, planPath)
	if err != nil {
		return nil, err
	}
	view, err := engine.ClipCaptionsView(summary, clipID, t, s.lookOf(path, summary))
	s.previews.tracks.keep(path, view)
	return view, err
}

// ShareOfClip gives the moment of the episode a share of a clip shows, the
// share of the clip as it plays, see engine.ShareTime. The number keys put
// the playhead there.
func (s *FrameFairy) ShareOfClip(path, planPath, clipID string, share float64) (float64, error) {
	if !s.store.PlanOf(path, planPath) {
		return 0, errNotInLibrary
	}
	summary, err := s.heldPlan(path, planPath)
	if err != nil {
		return 0, err
	}
	return engine.ClipShareTime(summary, clipID, share)
}

// captionLook is how the captions of the clips of planPath of the episode
// at path are made, the way a render job makes them: in the style the
// job's options put on the plan, on the episode's frames, see
// engine.ClipDrawing. The video preview once took the style apart from
// the render and the clip off the frames, and lit some words a frame from
// where the short lights them.
//
// The plan is read through the video's Episode, which reads it again only
// when its file changed, since a drag asks for the look on every step.
func (s *FrameFairy) captionLook(path, planPath string) *engine.Look {
	summary, _ := s.heldPlan(path, planPath)
	return s.lookOf(path, summary)
}

// heldPlan is the plan at planPath of the video at path, as the video's
// Episode holds it, read again only when its file changed. What it gives
// is shared and only read.
func (s *FrameFairy) heldPlan(path, planPath string) (engine.PlanSummary, error) {
	return s.store.machine.Episode(path).Plan(planPath)
}

// lookOf is captionLook of a plan already in hand.
func (s *FrameFairy) lookOf(path string, summary engine.PlanSummary) *engine.Look {
	plan := summary.Plan()
	style, err := s.store.Settings().options().CaptionLook(plan)
	if err != nil {
		theLog.line(zerolog.WarnLevel, "captions").Err(err).Msg("the settings' caption look is not one the render takes")
		style = plan.CaptionStyle()
	}
	return &engine.Look{Style: style, Source: s.unprobed.source(path, s.probe), Fonts: &s.store.machine.Fonts}
}

// failedProbes keeps the videos whose frames could not be read, a video
// on a drive that is not plugged in. A drag on the clip timeline asks for
// the captions' look on every step, and such a video was asked of ffprobe
// and warned about on every one of them. It is asked again after a while.
// FrameFairy keeps it. Its zero value is ready.
type failedProbes struct {
	mu    sync.Mutex
	since map[string]time.Time
}

// probeAgain is how long a video whose frames could not be read is left
// before it is asked again.
const probeAgain = 5 * time.Second

// source is what probe says the video at path is, or nothing while it
// cannot be read. The first failure of a stretch is said, with the
// reason.
func (f *failedProbes) source(path string, probe func(context.Context, string) (engine.SourceInfo, error)) engine.SourceInfo {
	f.mu.Lock()
	failed, known := f.since[path]
	f.mu.Unlock()
	if known && time.Since(failed) < probeAgain {
		return engine.SourceInfo{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source, err := probe(ctx, path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		if !known {
			theLog.line(zerolog.WarnLevel, "captions").Str("video", filepath.Base(path)).Err(err).
				Msg("the video's frames are not known, so its captions are timed on the plan's clock")
		}
		if f.since == nil {
			f.since = map[string]time.Time{}
		}
		f.since[path] = time.Now()
		return engine.SourceInfo{}
	}
	delete(f.since, path)
	return source
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
		logs := engine.LogsDir(j.Episode)
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
			plan = rec.PlanPath(j.Episode, duration)
		}
		for _, u := range j.Underway {
			if u.N == n {
				t, err := s.words(j.Episode)
				if err != nil {
					return nil, err
				}
				// The plan the clip goes into may not be written yet, and
				// its captions are then in the plan's own style.
				summary, _ := s.heldPlan(j.Episode, plan)
				view := engine.ArrivingCaptionsView(summary, u, t, s.lookOf(j.Episode, summary))
				s.previews.tracks.keep(j.Episode, view)
				return view, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}

// Fonts are the faces the captions can be written in: the ones that travel
// with the program, then the ones installed on this machine.
func (s *FrameFairy) Fonts() []engine.CaptionFont {
	// Read again, so a font installed while the app runs is in the list.
	s.store.machine.Fonts.Refresh()
	return s.store.machine.Fonts.Caption()
}

// clipEntry is one clip as it is now, read from its own plan alone. An
// edit answers with it, and it used to be found in the whole clip list,
// which read every plan of the episode and its transcript for one clip.
// An edit is written by the time it answers, so the answer takes its
// number from the clock here, see ClipEntry.Rev.
// answer is a clip as an edit left it, the answer to the edit. Its rev is
// taken first and the video's state is told after it, so a state is sent
// that was read after the answer and is newer than it. Told only before
// the answer, in editMoving, the state could take its rev first, and the
// window, which takes no state older than an edit's answer, would drop it,
// see EpisodeSession.put in frontend/src/lib/session.svelte.ts.
func (s *FrameFairy) answer(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	entry, err := s.clipEntry(ctx, path, plan, clipID)
	s.told(path)
	return entry, err
}

func (s *FrameFairy) clipEntry(ctx context.Context, path, plan, clipID string) (ClipEntry, error) {
	if !s.store.PlanOf(path, plan) {
		return ClipEntry{}, errNotInLibrary
	}
	rev := s.stamp()
	info, err := s.probe(ctx, path)
	if err != nil {
		return ClipEntry{}, err
	}
	view, err := s.store.machine.Episode(path).ReadPlan(plan)
	if err != nil {
		return ClipEntry{}, err
	}
	for _, c := range view.Clips {
		if c.ID == clipID {
			entry := s.entryOf(plan, c, info)
			entry.Rev = rev
			return entry, nil
		}
	}
	return ClipEntry{}, fmt.Errorf("the clip %s is not in this video's clips any more: %w", clipID, os.ErrNotExist)
}
