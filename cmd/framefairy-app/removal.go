package main

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/rs/zerolog"
)

// Work that stopped, a search or a clip made by hand, can be removed, so
// it is not carried on, issue #257 and #431. Its row says Removed and
// offers it back for as long as a removed clip's row does, and only then
// is the work forgotten, its record and all. That time is kept here, by
// the job, so it runs on whichever video is open, and the window draws
// how much of it is left from the job and decides nothing. The window
// once kept the time and called the work off when it ran out, with a
// timer of its own for each video, see
// docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.

// removalTime is how long removed work can be put back, the
// secondThoughts the window draws a removed row for, in
// frontend/src/lib/removed.ts.
const removalTime = 10 * time.Second

// timeKeeper is the clock removed work is timed by: the time of day, and
// a call once a while has gone by, which can be stopped. The app's is the
// system's, and the bridge hands in the one a walk runs on, so a walk
// that runs the page's ten seconds on runs these on with them, see
// walkTime in walktime_test.go.
type timeKeeper interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

// systemTime is the system's clock.
type systemTime struct{}

func (systemTime) Now() time.Time { return time.Now() }

func (systemTime) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// JobAway is how the removal of a job stands: when it is forgotten, or,
// while a hand holds its row, how long is left.
type JobAway struct {
	// Until is when the work is forgotten, in milliseconds since the
	// epoch, and 0 while it is held.
	Until int64 `json:"until"`
	// Left is how many seconds are left while it is held.
	Left float64 `json:"left"`
	// Whole is how many seconds it had in all.
	Whole float64 `json:"whole"`
}

// RemoveWork removes work that stopped: it is forgotten once its time is
// up, unless it is put back first. Anything else is left as it is. It
// answers with the state of the job's video read after it, so the row
// says Removed in the frame the answer comes.
func (s *FrameFairy) RemoveWork(ctx context.Context, id string) (VideoState, error) {
	return s.stateAfter(ctx, s.jobs.away(id, func(j *Job, now time.Time) bool {
		if !j.unfinished() && !(j.State == JobFailed && makesClips(j.Kind)) || j.Away != nil {
			return false
		}
		j.Away = &JobAway{Until: now.Add(removalTime).UnixMilli(), Whole: removalTime.Seconds()}
		return true
	}, s.forgetRemoved))
}

// HoldRemoval holds the time of removed work while a hand is on its row,
// and lets it run on when held is false.
func (s *FrameFairy) HoldRemoval(id string, held bool) {
	s.jobs.away(id, func(j *Job, now time.Time) bool {
		// A new JobAway each time, never one changed in place: the
		// snapshots sent before hold the one they were sent with.
		was := j.Away
		if was == nil || held == (was.Until == 0) {
			return false
		}
		if held {
			j.Away = &JobAway{Left: max(0, time.UnixMilli(was.Until).Sub(now).Seconds()), Whole: was.Whole}
			return true
		}
		j.Away = &JobAway{Until: now.Add(time.Duration(was.Left * float64(time.Second))).UnixMilli(), Whole: was.Whole}
		return true
	}, s.forgetRemoved)
}

// PutWorkBack puts removed work back before its time is up, as it was,
// and answers with the state of its video read after it.
func (s *FrameFairy) PutWorkBack(ctx context.Context, id string) (VideoState, error) {
	return s.stateAfter(ctx, s.jobs.away(id, func(j *Job, _ time.Time) bool {
		if j.Away == nil {
			return false
		}
		j.Away = nil
		return true
	}, s.forgetRemoved))
}

// stateAfter is the state of the video of a job, read now.
func (s *FrameFairy) stateAfter(ctx context.Context, video string) (VideoState, error) {
	if video == "" {
		return VideoState{}, errors.New("there is no such work")
	}
	return s.State(ctx, video)
}

// forgetRemoved forgets removed work whose time is up, its record and all.
func (s *FrameFairy) forgetRemoved(id string) {
	theLog.line(zerolog.InfoLevel, "jobs").Str("job", id).Msg("the work removed is forgotten, its time is up")
	s.CancelJob(id)
}

// away changes how the removal of job id stands, and sets its timer by
// what change left: one that runs when Until is set, and none when it is
// held or put back. The timer calls forget with the job's id. It gives
// the video of the job, and nothing when there is no such job.
func (q *queue) away(id string, change func(*Job, time.Time) bool, forget func(string)) string {
	q.mu.Lock()
	var job *Job
	for _, j := range q.jobs {
		if j.ID == id {
			job = j
		}
	}
	if job == nil {
		q.mu.Unlock()
		return ""
	}
	if !change(job, q.time.Now()) {
		q.mu.Unlock()
		return job.Episode
	}
	if stop := q.removals[id]; stop != nil {
		stop()
		delete(q.removals, id)
	}
	if a := job.Away; a != nil && a.Until != 0 {
		q.removals[id] = q.time.AfterFunc(time.UnixMilli(a.Until).Sub(q.time.Now()), func() {
			q.mu.Lock()
			delete(q.removals, id)
			// Only the removal the timer was set for is forgotten. Work
			// settled since, carried on by a new job or forgotten with its
			// video, holds no removal, and its record may be the new
			// job's by now.
			due := slices.ContainsFunc(q.jobs, func(j *Job) bool { return j.ID == id && j.Away == a })
			q.mu.Unlock()
			if due {
				forget(id)
			}
		})
	}
	q.stampLocked(job)
	snapshot := *job
	q.mu.Unlock()
	theLog.jobLine(zerolog.InfoLevel, &snapshot).Bool("removed", snapshot.Away != nil).
		Bool("held", snapshot.Away != nil && snapshot.Away.Until == 0).Msg("the removal of the work that stopped changed")
	q.emit(JobUpdate{Job: snapshot})
	return snapshot.Episode
}
