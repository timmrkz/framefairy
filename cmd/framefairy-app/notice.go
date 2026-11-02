package main

import (
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// A failure is said once, at the moment the queue sees it: a job of a
// video that goes from waiting or running to failed, or one refused as it
// is asked for. It is not a fact about the video's state, which says the
// job failed for as long as it stands, after a restart too, so it is sent
// on an event of its own, "notice", and the interface shows it in a toast.
// The interface once decided which failures to tell, from the jobs it saw
// change, and kept the ones it had told so as not to tell them twice. See
// docs/decisions/2026-11-01-every-rule-of-the-workspace-in-go.md.

// Notice is a failure to tell, in the words the toast shows.
type Notice struct {
	// Video is the video the work was of.
	Video string `json:"video"`
	// Job is the job that failed, and Kind what it was doing.
	Job  string `json:"job"`
	Kind string `json:"kind"`
	// Text is why, as a sentence.
	Text string `json:"text"`
	// Report says the toast offers Report a Problem, for a failure the
	// person cannot put right alone.
	Report bool `json:"report"`
}

// noticeOf is the notice of a job that failed, and false for one whose
// failure is said where its work was alone: a download says why in its
// card on the settings page.
func noticeOf(j Job) (Notice, bool) {
	if j.Episode == "" || j.State != JobFailed {
		return Notice{}, false
	}
	n := Notice{Video: j.Episode, Job: j.ID, Kind: j.Kind, Report: true}
	var none string
	switch j.Kind {
	case engine.JobSearch:
		none = "The search failed, and no reason was given."
	case engine.JobClip:
		none = "The clip could not be made, and no reason was given."
	case engine.JobTranscribe:
		none = "The transcription failed, and no reason was given."
	case engine.JobRender:
		none, n.Report = "The render failed.", false
	default:
		return Notice{}, false
	}
	n.Text = sentence(strings.TrimSpace(j.Error))
	if n.Text == "" {
		n.Text = none
	}
	return n, true
}

// tell sends the notice of a job that failed, if it has one.
func (q *queue) tell(j Job) {
	n, ok := noticeOf(j)
	if !ok {
		return
	}
	theLog.line(zerolog.InfoLevel, "app").Str("video", filepath.Base(j.Episode)).Str("job", j.ID).
		Str("kind", j.Kind).Bool("report", n.Report).Msg("a failure is told")
	if q.notice != nil {
		q.notice(n)
	}
}
