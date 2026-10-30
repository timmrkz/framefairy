package main

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"runtime/debug"

	"framefairy/engine/timed"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// Searches and renders, each one job from the moment it is asked for to
// the moment it ends, see docs/JOBS.md. The engine runs the steps and keeps
// the record, the queue gives each step its turn, and the interface only
// says what was clicked.

// Search finds clips in a window of an episode: the Search button, and Continue on a
// search that stopped. It hears the episode as far as the window reaches,
// if it has not been heard that far, and the model reads the window. An
// episode has one search at a time, so asking again while one runs gives
// the one that runs. click names the press that asked for it, so a Cancel
// pressed after it stops it even if the Cancel reaches the Go side first,
// see StopClipWork. The Go side's own searches have none.
func (s *FrameFairy) Search(path string, req engine.PlanRequest, click string) Job {
	return s.search(path, req, click, 0)
}

// search is Search, carrying on a search that had written carried clips.
func (s *FrameFairy) search(path string, req engine.PlanRequest, click string, carried int) Job {
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobSearch, jobLabel(engine.JobSearch, false), notInLibrary, func(j *Job) {
			j.From, j.To, j.Count, j.Min = req.From, req.To, req.Count, req.Min
		})
	}
	// A search called off a moment ago may still be on its way out, and it
	// writes the same record as it goes, so this one waits for it.
	for _, j := range s.jobs.list() {
		if j.Episode == path && j.Kind == engine.JobSearch && j.State == JobRunning && j.ctx.Err() != nil {
			s.jobs.waitJob(j.ID)
		}
	}
	// The video's transcription and the work on its clips go one at a
	// time, see Transcribe.
	s.stopTranscribing(path, "search")
	// A search that stopped is taken up by this one, which writes a record
	// of its own in its place.
	s.jobs.settle(path, engine.JobSearch, "")
	job := s.jobs.addSteps(path, engine.JobSearch, jobLabel(engine.JobSearch, false), true, func(j *Job) {
		j.Record, j.From, j.To, j.Count, j.Min, j.click = engine.SearchID, req.From, req.To, req.Count, req.Min, click
		j.Carried = carried
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		// Its window is the video's window, written here, in the search's
		// own work and so before windowOn below, never after it. Written
		// once the job was queued, a search that found its clips at once
		// could move the window on first and then have it written back,
		// the fourth review of #458. Not where the job is prepared, which
		// holds the queue, taken after windowMu everywhere else.
		s.searchesWindow(path, req)
		// The model loads while the episode is still being heard, so the
		// search has nothing to wait for once it comes to finding.
		// A window to the end of the episode, To 0, is never heard before
		// the transcript is finished.
		model := s.store.Settings().ASRModel
		unheard := false
		switch {
		case ctx.Err() != nil:
			// Called off before it started, it loads nothing.
		case req.To <= 0:
			_, done := engine.Coverage(s.store.files, p.Source, model)
			unheard = !done
		default:
			unheard = len(engine.Unheard(s.store.files, p.Source, model, timed.Window{Start: req.From, End: req.To})) > 0
		}
		// It also reads the transcript of the window as it is heard, so
		// the search finds most of its prompt read.
		if unheard {
			go func() {
				video := filepath.Base(path)
				defer func() {
					if v := recover(); v != nil {
						theLog.line(zerolog.ErrorLevel, "jobs").Str("video", video).Str("panic", fmt.Sprint(v)).
							Str("stack", string(debug.Stack())).Msg("reading ahead of the search stopped unexpectedly")
					}
				}()
				ahead, err := p.WarmModel(ctx, req)
				switch {
				case err != nil && ctx.Err() == nil:
					theLog.line(zerolog.WarnLevel, "jobs").Str("video", video).Err(err).
						Msg("the model could not be loaded ahead of the search")
				case ahead.Reads > 0:
					theLog.line(zerolog.InfoLevel, "jobs").Str("video", video).Int("reads", ahead.Reads).
						Int("tokens", ahead.Tokens).Int("read", ahead.Read).Int("rewritten", ahead.Rewritten).
						Int("ms", int(ahead.Seconds*1000)).
						Msg("the model read the transcript ahead of the search")
				}
			}()
		}
		result, err := p.Search(ctx, req, turn)
		if err == nil && ctx.Err() == nil {
			s.windowOn(path, req)
		}
		return result, err
	})
	return job
}

// Continue carries on a search or a render that was cut off or failed,
// from what it left. click names the press, the way it does for Search.
func (s *FrameFairy) Continue(id, click string) Job {
	var stopped *Job
	for _, j := range s.jobs.list() {
		// A search that failed before it had a record, refused as it was
		// asked for, is asked for again.
		if j.ID == id && (j.State == JobInterrupted || j.State == JobFailed) &&
			(j.Record != "" || j.Kind == engine.JobSearch) {
			stopped = &j
		}
	}
	if stopped == nil {
		return s.jobs.refuse("", "continue", "Continue", "there is nothing to carry on")
	}
	path := stopped.Episode
	if stopped.Kind == engine.JobSearch {
		req := engine.PlanRequest{From: stopped.From, To: stopped.To}
		carried := 0
		// A search that was refused wrote no record, and the one on disk
		// is an older search's, about another window.
		if rec := engine.ReadSearch(s.store.files, path); rec != nil && stopped.Record != "" {
			req = rec.Request()
			carried = s.planned(path, *rec)
		}
		if req.Count == 0 {
			set := s.store.Settings()
			req.Min, req.Max = set.Min, set.Max
			window := req.To - req.From
			if req.To <= 0 {
				if info, err := s.probe(context.Background(), path); err == nil {
					window = info.Duration - req.From
				}
			}
			req.Count = set.targetFor(window)
		}
		return s.search(path, req, click, carried)
	}
	var carry *engine.JobRecord
	for _, rec := range engine.ReadJobs(s.store.files, path) {
		if rec.ID == stopped.Record {
			carry = &rec
		}
	}
	if carry == nil {
		return s.jobs.refuse(path, stopped.Kind, stopped.Label, "there is nothing left to carry on")
	}
	if carry.Kind == engine.JobClip {
		return s.makeClip(path, carry.Clip(), carry.ID, click)
	}
	if carry.Kind == engine.JobTranscribe {
		return s.Transcribe(path)
	}
	return s.render(path, carry.Render(), carry)
}

// planned is how many clips the search a record is of has written to its
// plan, read from the plan, so it is true after the app was closed too.
func (s *FrameFairy) planned(path string, rec engine.JobRecord) int {
	duration := rec.To
	if info, err := s.probe(context.Background(), path); err == nil {
		duration = info.Duration
	}
	view, err := engine.ReadPlan(s.store.files, filepath.Join(engine.WorkDir(path), "logs", rec.PlanName(duration)))
	if err != nil {
		return 0
	}
	return len(view.Clips)
}

// render queues a render, carrying on the one the record is of, if any.
func (s *FrameFairy) render(path string, req engine.RenderRequest, carry *engine.JobRecord) Job {
	label := jobLabel(engine.JobRender, req.Preview)
	// The plan is a path of its own, read and written to, so it is checked
	// the same way the episode is, and has to be one of this episode's.
	if !s.store.Known(path) || (req.Plan != "" && !s.store.PlanOf(path, req.Plan)) {
		return s.jobs.refuse(path, engine.JobRender, label, notInLibrary)
	}
	id := engine.NewRenderID()
	if carry != nil {
		id = carry.ID
		s.jobs.settle(path, engine.JobRender, id)
	}
	return s.jobs.addSteps(path, engine.JobRender, label, false, func(j *Job) {
		j.Record, j.Plan, j.Clips = id, req.Plan, append([]string(nil), req.Clips...)
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		if err := p.RenderJob(ctx, id, req, carry, turn); err != nil {
			return req.Plan, err
		}
		if !req.Preview {
			// A finished render is the strongest sign a clip was right, and
			// it is recorded with the clip as it was rendered.
			ids := req.Clips
			if len(ids) == 0 {
				if view, err := engine.ReadPlan(s.store.files, req.Plan); err == nil {
					for _, c := range view.Clips {
						if !c.Rejected {
							ids = append(ids, c.ID)
						}
					}
				}
			}
			for _, id := range ids {
				_ = engine.RecordDecision(s.store.files, s.store.records, req.Plan, id, engine.DecisionRendered, nil)
			}
		}
		return req.Plan, nil
	})
}

// MakeClip makes a clip by hand at a moment of the episode, I or O, as a job
// like any: it has the transcript heard as far as the clip can reach, and
// the one plan builder frames and writes the clip, see engine.MakeClip.
// Any number can be made at a time, beside a search and beside each other,
// so I and O never wait for the work before them to be asked for. Its
// result is the key of the clip it made. Its card is in the list from the
// moment it is asked for, at the playhead.
func (s *FrameFairy) MakeClip(path string, at float64, backward bool) Job {
	return s.makeClip(path, engine.ClipRequest{At: at, Backward: backward}, "", "")
}

// makeClip queues a clip made by hand, carrying on the one the record id
// is of, if any. click names the press of Continue, if it was one.
func (s *FrameFairy) makeClip(path string, req engine.ClipRequest, carry, click string) Job {
	label := jobLabel(engine.JobClip, false)
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobClip, label, notInLibrary)
	}
	if math.IsNaN(req.At) || math.IsInf(req.At, 0) || req.At < 0 || req.At > engine.MaxEpisodeSeconds {
		return s.jobs.refuse(path, engine.JobClip, label, "there is no such moment in the video")
	}
	s.stopTranscribing(path, "clip made by hand")
	id := engine.NewClipID()
	if carry != "" {
		id = carry
		s.jobs.settle(path, engine.JobClip, id)
	}
	return s.jobs.addSteps(path, engine.JobClip, label, false, func(j *Job) {
		j.Record, j.At, j.Backward, j.click = id, req.At, req.Backward, click
		j.Underway = engine.JobRecord{Kind: engine.JobClip, At: req.At, Step: engine.StepWaiting}.Underway()
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		return p.MakeClip(ctx, id, req, turn)
	})
}

// firstSearch asks for the first search of an episode that has never been
// searched, the moment it is added. It searches the video's window, which
// it places, see videoWindow, the same one the workspace draws, and looks
// for the clips that window suggests, or for the target typed.
func (s *FrameFairy) firstSearch(ctx context.Context, path string) {
	if engine.EverSearched(s.store.files, path) {
		return
	}
	info, err := s.probe(ctx, path)
	if err != nil || info.Duration <= 0 {
		return
	}
	w, ok := s.videoWindow(ctx, path)
	if !ok {
		return
	}
	set := s.store.Settings()
	opts := set.options()
	req := engine.PlanRequest{From: w.From, To: w.To, Min: opts.Min, Max: opts.Max}
	// A window to the end of the episode is To 0, which is heard to its
	// end before it is searched.
	if w.To >= info.Duration-0.05 {
		req.To = 0
	}
	// A target typed for a window this long, or what this window suggests.
	req.Count = set.targetFor(w.To - w.From)
	if req.Count == 0 {
		req.Count = engine.SuggestedCount(w.To-w.From, req.Min, req.Max)
	}
	s.Search(path, req, "")
}

// cancelSteps calls off a search or a render, or takes away the note of
// one that stopped. A search called off says so where its work was, with
// Continue, the same as one cut off by the app closing: what it heard stays
// and it can be carried on. The whole video being transcribed does the
// same, its menu says Continue transcribing. A render called off keeps the shorts it
// finished and goes, because its Render button is where it is carried on.
// It says whether the job was one of those.
//
// The note of a search or a clip made by hand that stopped is taken away
// when its row in the clip list is removed, issue #431. A search the queue
// refused wrote no record, so there is nothing to forget and it is only
// settled.
func (s *FrameFairy) cancelSteps(id string) bool {
	for _, j := range s.jobs.list() {
		if j.ID != id {
			continue
		}
		if j.Record == "" {
			if j.State == JobFailed && makesClips(j.Kind) {
				s.jobs.settleRefused(id)
				return true
			}
			continue
		}
		if j.State == JobInterrupted || j.State == JobFailed {
			theLog.jobLine(zerolog.InfoLevel, &j).Str("state", j.State).Msg("the work that stopped is removed, its record forgotten")
			s.forgetRecord(j.Episode, j.Record)
			s.jobs.settle(j.Episode, j.Kind, j.Record)
			return true
		}
		// A search, a clip or the whole video's transcription says Stopped
		// in its record as it ends, and a render forgets its record then,
		// so nothing it writes on its way out brings it back, see runJob.
		s.jobs.stopByHand(id)
		return true
	}
	return false
}

func (s *FrameFairy) forgetRecord(path, record string) {
	if err := engine.RemoveJob(s.store.files, path, record); err != nil {
		theLog.line(zerolog.WarnLevel, "jobs").Str("video", filepath.Base(path)).Str("record", record).Err(err).Msg("the record could not be cleared")
	}
}
