package main

import (
	"context"
	"log"
	"math"
	"path/filepath"

	"framefairy/engine"
)

// Searches and renders, each one job from the moment it is asked for to
// the moment it ends, see docs/JOBS.md. The engine runs the steps and keeps
// the record, the queue gives each step its turn, and the interface only
// says what was clicked.

// Search finds clips in a window of an episode: New, and Continue on a
// search that stopped. It hears the episode as far as the window reaches,
// if it has not been heard that far, and the model reads the window. An
// episode has one search at a time, so asking again while one runs gives
// the one that runs.
func (s *FrameFairy) Search(path string, req engine.PlanRequest) Job {
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobSearch, "Find clips", notInLibrary)
	}
	// A search called off a moment ago may still be on its way out, and it
	// writes the same record as it goes, so this one waits for it.
	for _, j := range s.jobs.list() {
		if j.Episode == path && j.Kind == engine.JobSearch && j.State == JobRunning && j.ctx.Err() != nil {
			s.jobs.waitJob(j.ID)
		}
	}
	// A search that stopped is taken up by this one, which writes a record
	// of its own in its place.
	s.jobs.settle(path, engine.JobSearch, "")
	return s.jobs.addSteps(path, engine.JobSearch, "Find clips", true, func(j *Job) {
		j.Record, j.From, j.To = engine.SearchID, req.From, req.To
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		// The model loads while the episode is still being heard, so the
		// search has nothing to wait for once it comes to finding.
		// A window to the end of the episode, To 0, is never heard before
		// the transcript is finished.
		if covered, done := engine.Coverage(p.Source, s.store.Settings().ASRModel); !done &&
			(req.To <= 0 || covered < req.To-0.05) {
			go func() {
				defer func() { _ = recover() }()
				if err := p.WarmModel(ctx, windowLength(req)); err != nil && ctx.Err() == nil {
					log.Printf("could not load the model ahead of the search: %v", err)
				}
			}()
		}
		return p.Search(ctx, req, turn)
	})
}

// MakeClip makes a clip by hand at a moment, In or Out, as a job like a
// search: it hears the part the clip needs where nothing has, frames the
// clip and writes it into the clip set made by hand, and Cancel, the app
// closing and Continue treat it the way they treat a search. An episode
// makes one at a time, so asking again while one runs gives the one that
// runs. Its result is the key of the clip it made. Its steps take no turn
// in a lane, see engine.MakeClipJob. Only writing the clip goes through
// the history, so Undo takes it back and no other edit waits for it.
func (s *FrameFairy) MakeClip(path string, at float64, backward bool) Job {
	const label = "Make a clip"
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobHand, label, notInLibrary)
	}
	for _, j := range s.jobs.list() {
		if j.Episode == path && j.Kind == engine.JobHand && j.State == JobRunning && j.ctx.Err() != nil {
			s.jobs.waitJob(j.ID)
		}
	}
	s.jobs.settle(path, engine.JobHand, "")
	return s.jobs.addSteps(path, engine.JobHand, label, true, func(j *Job) {
		j.Record, j.At, j.Backward, j.free = engine.HandID, at, backward, true
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		info, err := s.probe(ctx, path)
		if err != nil {
			return "", err
		}
		p.Edit = func(write func() error) error { return s.edit(path, write) }
		plan, id, err := p.MakeClipJob(ctx, at, backward, info.Duration, turn)
		if err != nil {
			return "", err
		}
		return filepath.Base(plan) + "/" + id, nil
	})
}

// Continue carries on a search, a clip made by hand or a render that was
// cut off or failed, from what it left.
func (s *FrameFairy) Continue(id string) Job {
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
	if stopped.Kind == engine.JobHand {
		// Made again at the same moment: what it heard is kept, so only
		// what it had not heard yet is heard.
		return s.MakeClip(path, stopped.At, stopped.Backward)
	}
	if stopped.Kind == engine.JobSearch {
		req := engine.PlanRequest{From: stopped.From, To: stopped.To}
		if rec := engine.ReadSearch(path); rec != nil {
			req = rec.Request()
		}
		if req.Count == 0 {
			set := s.store.Settings()
			req.Count, req.Min, req.Max = set.Count, set.Min, set.Max
		}
		return s.Search(path, req)
	}
	var carry *engine.JobRecord
	for _, rec := range engine.ReadJobs(path) {
		if rec.ID == stopped.Record {
			carry = &rec
		}
	}
	if carry == nil {
		return s.jobs.refuse(path, engine.JobRender, stopped.Label, "the render left nothing to carry on")
	}
	return s.render(path, carry.Render(), carry)
}

// render queues a render, carrying on the one the record is of, if any.
func (s *FrameFairy) render(path string, req engine.RenderRequest, carry *engine.JobRecord) Job {
	label := "Render"
	if req.Preview {
		label = "Preview"
	}
	// The plan is a path of its own, read and written to, so it is checked
	// the same way the episode is.
	if !s.store.Known(path) || (req.Plan != "" && !s.store.Known(req.Plan)) {
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
				if view, err := engine.ReadPlan(req.Plan); err == nil {
					for _, c := range view.Clips {
						if !c.Rejected {
							ids = append(ids, c.ID)
						}
					}
				}
			}
			for _, id := range ids {
				_ = engine.RecordDecision(req.Plan, id, engine.DecisionRendered, nil)
			}
		}
		return req.Plan, nil
	})
}

// firstSearch asks for the first search of an episode that has never been
// searched, the moment it is added: the first half hour, or all of a
// shorter episode, and no more than the model can read at once.
func (s *FrameFairy) firstSearch(ctx context.Context, path string) {
	if engine.EverSearched(path) {
		return
	}
	info, err := s.probe(ctx, path)
	if err != nil || info.Duration <= 0 {
		return
	}
	set := s.store.Settings()
	opts := set.options()
	room := engine.SearchRoom(opts, filepath.Join(engine.WorkDir(path), "logs"))
	req := engine.PlanRequest{Count: opts.Count, Min: opts.Min, Max: opts.Max}
	req.To = firstWindowEnd(info.Duration, room, req)
	s.Search(path, req)
}

// firstWindowEnd is where an episode's first window ends. It is what the
// workspace would draw for it: the first half hour, or the whole of a
// shorter episode, no longer than the model can read of an episode not
// heard yet, and no shorter than the clips asked for need. 0 is the end
// of the episode.
func firstWindowEnd(duration float64, room engine.Room, req engine.PlanRequest) float64 {
	end := math.Min(duration, firstLook)
	if room.Chars > 0 {
		end = math.Min(end, math.Floor(float64(room.Chars)/engine.RateOf(nil)))
	}
	end = math.Max(end, math.Min(duration, float64(req.Count)*req.Min))
	if end >= duration-0.05 {
		return 0
	}
	return end
}

// cancelSteps calls off a search or a render, or takes away the note of
// one that stopped. A search called off says so where its work was, with
// Continue, the same as one cut off by the app closing: what it heard stays
// and it can be carried on. A render called off keeps the shorts it
// finished and goes, because its Render button is where it is carried on.
// It says whether the job was one of those.
func (s *FrameFairy) cancelSteps(id string) bool {
	for _, j := range s.jobs.list() {
		if j.ID != id || j.Record == "" {
			continue
		}
		if j.State == JobInterrupted || j.State == JobFailed {
			s.forgetRecord(j.Episode, j.Record)
			s.jobs.settle(j.Episode, j.Kind, j.Record)
			return true
		}
		if j.Kind == engine.JobSearch || j.Kind == engine.JobHand {
			// Its record says stopped as it ends, see runJob.
			s.jobs.stopByHand(id)
			return true
		}
		s.jobs.cancel(id)
		// The record goes once the render has stopped, so nothing it
		// writes on its way out brings it back.
		go func() {
			defer func() { _ = recover() }()
			s.jobs.waitJob(id)
			s.forgetRecord(j.Episode, j.Record)
		}()
		return true
	}
	return false
}

func (s *FrameFairy) forgetRecord(path, record string) {
	if err := engine.RemoveJob(path, record); err != nil {
		log.Printf("could not clear the record of %s: %v", path, err)
	}
}
