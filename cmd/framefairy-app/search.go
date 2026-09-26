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
	// A search that stopped is taken up by this one, which writes a record
	// of its own in its place.
	s.jobs.settle(path, engine.JobSearch, "")
	// From here on this episode has been looked at, so the workspace never
	// asks for a first search of its own.
	if err := engine.MarkLooked(path); err != nil {
		log.Printf("could not note the search of %s: %v", path, err)
	}
	return s.jobs.addSteps(path, engine.JobSearch, "Find clips", true, func(j *Job) {
		j.Record, j.From, j.To = engine.SearchID, req.From, req.To
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		// The model loads while the episode is still being heard, so the
		// search has nothing to wait for once it comes to finding.
		if covered, done := engine.Coverage(p.Source, s.store.Settings().ASRModel); !done && covered < req.To-0.05 {
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

// Continue carries on a search or a render that was cut off or failed,
// from what it left.
func (s *FrameFairy) Continue(id string) Job {
	var stopped *Job
	for _, j := range s.jobs.list() {
		if j.ID == id && (j.State == JobInterrupted || j.State == JobFailed) && j.Record != "" {
			stopped = &j
		}
	}
	if stopped == nil {
		return s.jobs.refuse("", "continue", "Continue", "there is nothing to carry on")
	}
	path := stopped.Episode
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
	if engine.Looked(path) || engine.ReadSearch(path) != nil ||
		len(engine.Status(path, s.store.Settings().ASRModel).Plans) > 0 {
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
// one that stopped, and with it its record. Called off by hand, it has
// nothing to report. It says whether the job was one of those.
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
		s.jobs.cancel(id)
		// The record goes once the job has stopped, so nothing it writes
		// on its way out brings it back. A search asked for in the
		// meantime has written the same record, and keeps it.
		go func() {
			defer func() { _ = recover() }()
			s.jobs.waitJob(id)
			for _, other := range s.jobs.list() {
				if other.ID != id && other.Episode == j.Episode && other.Record == j.Record &&
					(other.State == JobQueued || other.State == JobRunning) {
					return
				}
			}
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
