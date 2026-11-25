package main

import (
	"context"
	"math"
	"path/filepath"

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
	req = s.asKept(path, req)
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobSearch, jobLabel(engine.JobRecord{Kind: engine.JobSearch}), notInLibrary, func(j *Job) {
			j.From, j.To, j.Count, j.Min = req.From, req.To, req.Count, req.Min
		})
	}
	s.jobs.makeWayForSearch(path)
	first := s.firstLane(path, req)
	job := s.jobs.addSteps(path, engine.JobSearch, jobLabel(engine.JobRecord{Kind: engine.JobSearch}), true, func(j *Job) {
		j.JobRecord.ID, j.From, j.To, j.Count, j.Min, j.click = engine.SearchID, req.From, req.To, req.Count, req.Min, click
		j.Carried, j.Lane = carried, first
	}, func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
		// Its window is the video's window, written here, in the search's
		// own work and so before windowOn below, never after it. Written
		// once the job was queued, a search that found its clips at once
		// could move the window on first and then have it written back,
		// the fourth review of #458. Not where the job is prepared, which
		// holds the queue.
		s.searchesWindow(path, req)
		s.searchBegins(path)
		result, err := p.Search(ctx, req, host)
		if err == nil && ctx.Err() == nil {
			s.windowOn(path, req)
		}
		return result, err
	})
	return job
}

// asKept is req with what it leaves out, at 0, taken from what is kept:
// how many clips, the number typed for a window this long or what the
// window suggests, the count the workspace shows beside it, and how long
// they may be. The workspace sends only the window, so a number typed and
// Search pressed at once never searches with the number from before it:
// on a busy Mac the press read the numbers from the state before the one
// the typing answered with, issue #877. A number in req is kept as it is.
func (s *FrameFairy) asKept(path string, req engine.PlanRequest) engine.PlanRequest {
	set := s.store.Settings()
	o := set.options()
	if req.Min <= 0 {
		req.Min = o.Min
	}
	if req.Max <= 0 {
		req.Max = o.Max
	}
	o.Min, o.Max = req.Min, req.Max
	if req.Count > 0 || !s.store.Known(path) {
		return req
	}
	info, err := s.probe(context.Background(), path)
	if err != nil || info.Duration <= 0 {
		return req
	}
	end := req.To
	if end <= 0 || end > info.Duration {
		end = info.Duration
	}
	length := end - req.From
	r := s.store.machine.Episode(path).Room(o)
	req.Count = o.Limits(r, info.Duration, length, set.targetFor(length)).Count
	return req
}

// firstLane is the lane a search waits in before its first turn: the
// lane of its first step. A search of a window not heard yet hears it
// first, so it waits to transcribe from the press on. It said it waited
// to find clips while the engine got ready, and the row of a search
// carried on said Window 0:00 to 2:00 under Transcribing for that moment,
// seen once the app heard its own writes, see reconcile.
func (s *FrameFairy) firstLane(path string, req engine.PlanRequest) string {
	// A video that cannot be read now is as long as the window says,
	// which is all the search needs to know of it.
	duration := req.To
	if info, err := s.probe(context.Background(), path); err == nil {
		duration = info.Duration
	}
	if duration <= 0 {
		return LaneFinding
	}
	opts := s.store.Settings().options()
	if s.store.machine.Episode(path).FirstStep(opts.ASRModel, opts.SilenceDB, req, duration) == engine.StepHearing {
		return LaneHearing
	}
	return LaneFinding
}

// Continue carries on a job that was cut off, stopped by hand or failed,
// from what it left. click names the press, the way it does for Search.
func (s *FrameFairy) Continue(id, click string) Job {
	var stopped *Job
	for _, j := range s.jobs.list() {
		// A search that failed before it had a record, refused as it was
		// asked for, is asked for again.
		if j.ID == id && j.unfinished() &&
			(j.JobRecord.ID != "" || j.Kind == engine.JobSearch) {
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
		if rec := engine.ReadSearch(s.store.files, path); rec != nil && stopped.JobRecord.ID != "" {
			req = rec.Request()
			req.CarryOn = true
			carried = s.planned(path, *rec)
		}
		// A search refused before it had a record asks for what is kept,
		// see asKept.
		return s.search(path, req, click, carried)
	}
	var carry *engine.JobRecord
	for _, rec := range engine.ReadJobs(s.store.files, path) {
		if rec.ID == stopped.JobRecord.ID {
			carry = &rec
		}
	}
	if carry == nil {
		return s.jobs.refuse(path, stopped.Kind, stopped.Label, "there is nothing left to carry on")
	}
	if carry.Kind == engine.JobClip {
		return s.makeClip(path, carry.Clip(), carry.ID, click, false)
	}
	switch carry.Hears() {
	case engine.HearsWhole:
		return s.Transcribe(path)
	case engine.HearsPart, engine.HearsAround:
		return s.transcribe(path, carry.Part(), carry.Hears() == engine.HearsAround, false)
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
	view, err := s.store.machine.Episode(path).ReadPlan(rec.PlanPath(path, duration))
	if err != nil {
		return 0
	}
	return len(view.Clips)
}

// render queues a render, carrying on the one the record is of, if any.
func (s *FrameFairy) render(path string, req engine.RenderRequest, carry *engine.JobRecord) Job {
	label := jobLabel(engine.JobRecord{Kind: engine.JobRender, Preview: req.Preview})
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
		j.JobRecord.ID, j.Plan, j.Clips = id, req.Plan, append([]string(nil), req.Clips...)
	}, func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
		if err := p.RenderJob(ctx, id, req, carry, host); err != nil {
			return req.Plan, err
		}
		if !req.Preview {
			// A finished render is the strongest sign a clip was right, and
			// it is recorded with the clip as it was rendered.
			ids := req.Clips
			if len(ids) == 0 {
				if view, err := s.store.machine.Episode(path).ReadPlan(req.Plan); err == nil {
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
// moment it is asked for, at the playhead, and it is the clip chosen.
func (s *FrameFairy) MakeClip(path string, at float64, backward bool) Job {
	return s.makeClip(path, engine.ClipRequest{At: at, Backward: backward}, "", "", true)
}

// makeClip queues a clip made by hand, carrying on the one the record id
// is of, if any. click names the press of Continue, if it was one. A new
// one is given the id it is written under before it is queued, see
// engine.NextHandClipID, so its card has the key of the clip it becomes
// and keeps it when it is written. With choose its card is the clip
// chosen from the moment the job is queued, before it can run, so every
// state of the video from then on says it is chosen. The workspace picked
// the card once the press had its answer, and a busy machine wrote the
// clip before that pick reached the Go side, so a state came with the
// card gone and its clip not chosen, issue #697.
func (s *FrameFairy) makeClip(path string, req engine.ClipRequest, carry, click string, choose bool) Job {
	label := jobLabel(engine.JobRecord{Kind: engine.JobClip})
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobClip, label, notInLibrary)
	}
	if math.IsNaN(req.At) || math.IsInf(req.At, 0) || req.At < 0 || req.At > engine.MaxEpisodeSeconds {
		return s.jobs.refuse(path, engine.JobClip, label, "there is no such moment in the video")
	}
	s.jobs.stopTranscribing(path, "clip made by hand")
	id := engine.NewClipID()
	if carry != "" {
		id = carry
		s.jobs.settle(path, engine.JobClip, id)
	}
	s.naming.Lock()
	defer s.naming.Unlock()
	if req.ID == "" {
		var taken []string
		jobs, _ := s.jobs.ofVideo(path)
		for _, j := range jobs {
			if j.Kind == engine.JobClip && j.ClipID != "" {
				taken = append(taken, j.ClipID)
			}
		}
		req.ID = engine.NextHandClipID(s.store.files, path, taken)
	}
	key := req.Key()
	var picks *picked
	if choose {
		picks = s.chosenIn(path)
		picks.load(s.store.files, path)
	}
	job := s.jobs.addSteps(path, engine.JobClip, label, false, func(j *Job) {
		j.JobRecord.ID, j.At, j.Backward, j.ClipID, j.click = id, req.At, req.Backward, req.ID, click
		j.Underway = engine.JobRecord{Kind: engine.JobClip, At: req.At, ClipID: req.ID, Step: engine.StepWaiting}.Underway()
		// With the queue's lock held, so the job cannot run before it.
		// A job the queue refused chooses nothing.
		if picks != nil && j.State == JobRunning {
			picks.keep(key, s.jobs.nextStampLocked())
		}
	}, func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
		return p.MakeClip(ctx, id, req, host)
	})
	if picks != nil && job.State == JobRunning {
		if err := picks.save(s.store.files, path); err != nil {
			theLog.line(zerolog.WarnLevel, "chosen").Str("video", filepath.Base(path)).Err(err).Msg("the clip the app chose could not be kept")
		}
		theLog.line(zerolog.InfoLevel, "chosen").Str("video", filepath.Base(path)).Str("clip", key).Str("why", "the clip made by hand was asked for").Msg("the app chose a clip")
	}
	return job
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
		if j.JobRecord.ID == "" {
			if j.State == JobFailed && makesClips(j.Kind) {
				s.jobs.settleRefused(id)
				return true
			}
			continue
		}
		if j.unfinished() {
			theLog.jobLine(zerolog.InfoLevel, &j).Str("state", j.State).Msg("the work that stopped is removed, its record forgotten")
			s.forgetRecord(j.Episode, j.JobRecord.ID)
			s.jobs.settle(j.Episode, j.Kind, j.JobRecord.ID)
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
