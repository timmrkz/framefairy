package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// ---------------------------------------------------------------------------
// Clips made by hand
//
// The model sometimes misses the moment a person wants. I and O make a clip
// at the playhead, the way In and Out mark one in every video editor. Only
// what differs from a search is here: what a clip made by hand asks for,
// how the playhead proposes a clip, and the clip set it goes into. Hearing
// is the one transcript's, see hear, and shaping, framing, numbering and
// writing are the one plan builder's, see propose. See docs/JOBS.md.
// ---------------------------------------------------------------------------

// HandPlanName is the clip set the clips made by hand go into. It grows
// with every clip, and it was made over no part of the episode, see
// madeOver.
const HandPlanName = "clips-hand.json"

// ClipRequest is a clip made by hand: the moment I or O was pressed at, and
// whether it was O, which ends the clip there rather than starting it.
type ClipRequest struct {
	At       float64
	Backward bool
}

// NewClipID is a new id for a clip made by hand.
func NewClipID() string {
	return fmt.Sprintf("clip-%d", time.Now().UnixNano())
}

// reach is what of the episode has to be heard for the clip: Longest from
// the playhead the way the clip grows, and on both sides the line the
// playhead stands in and a sentence's reach from it, where the clip's
// edges are put.
func (r ClipRequest) reach(longest, duration float64) Window {
	before, after := 2*sentenceReach, 2*sentenceReach
	if r.Backward {
		before += longest
	} else {
		after += longest
	}
	return Window{max(0, r.At-before), min(duration, r.At+after)}
}

// MakeClip makes a clip at a moment as a job, with a record, so it is cut
// off and carried on the way every job is. It has the transcript heard as
// far as the clip can reach, has the playhead propose the clip, and hands
// it to the plan builder, which shapes, frames and writes it the way it
// does every clip. It answers with the clip's key, the clip set's name and
// the clip's id.
//
// It says which clip it has on the way from the moment it starts, at the
// playhead, and the builder says it from the moment it has its sentences,
// so the app shows it arriving the way it shows a search's clips.
func (p *Project) MakeClip(ctx context.Context, id string, req ClipRequest, turn Turn) (key string, err error) {
	j := p.startJob(JobRecord{ID: id, Kind: JobClip, At: req.At, Backward: req.Backward})
	defer j.end(&err)
	log := p.engine.Log
	// Made, it is on the way no more. Called off or failed, it stays where
	// it would have appeared, and says so there, until it is carried on
	// or put away.
	defer func() {
		switch {
		case err == nil:
			log.Underway(nil, 1, true)
		case errors.Is(err, ErrCancelled):
			log.Underway([]Underway{{N: 1, Start: req.At, End: req.At, Step: StepStopped}}, 0, true)
		default:
			log.Underway([]Underway{{N: 1, Start: req.At, End: req.At, Step: StepFailed}}, 0, true)
		}
	}()
	// Whatever a job called off runs into on its way out, ffmpeg ended
	// or a step not begun, it was called off. This runs before the two
	// above, which go by it.
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ErrCancelled
		}
	}()
	// The clip at the playhead, in whatever step the job is in, until the
	// builder has it.
	stepOf := func(ctx context.Context, step string) (context.Context, func(), error) {
		log.Underway([]Underway{{N: 1, Start: req.At, End: req.At, Step: StepWaiting}}, 0, true)
		stepCtx, release, err := j.turn(ctx, turn, step)
		if err == nil {
			log.Underway([]Underway{{N: 1, Start: req.At, End: req.At, Step: step}}, 0, true)
		}
		return stepCtx, release, err
	}
	log.Underway([]Underway{{N: 1, Start: req.At, End: req.At, Step: StepWaiting}}, 0, true)

	source, err := p.engine.Probe(ctx, p.Source)
	if err != nil {
		return "", err
	}
	o := p.Base
	if err := p.hear(ctx, stepOf, req.reach(o.Max, source.Duration)); err != nil {
		return "", err
	}
	stepCtx, release, err := stepOf(ctx, StepFraming)
	if err != nil {
		return "", err
	}
	defer release()

	t, err := p.Transcript()
	if err != nil {
		return "", err
	}
	lines := BuildLines(t.Words, t.Levels(), o.MaxPause)
	build := p.engine.newPlanBuilder(stepCtx, p.Source, source, lines, nil, PlanOptions{
		Count: 1, MinLen: o.Min, MaxLen: o.Max, OutW: o.Width, OutH: o.Height,
		MaxPause: o.MaxPause, KeepPause: o.KeepPause, Recipe: o.Recipe, LogDir: p.LogsDir(),
		PlanPath: p.HandPlanPath(), By: ByHand, Grows: true, IDPrefix: "h"}, "")
	defer build.stop()
	entry, err := handEntry(lines, req, o.Min, o.Max, build.seconds)
	if err != nil {
		return "", err
	}
	// Placing the crop says how far it has come, the way hearing does, so
	// the card fills while the crop is placed rather than only wearing the
	// beam. It holds the progress line until the clip is written.
	build.placing = newCropWork()
	stopWork := make(chan struct{})
	reported := make(chan struct{})
	go func() {
		defer close(reported)
		build.placing.report(log, stopWork)
	}()
	defer func() {
		close(stopWork)
		<-reported
	}()
	build.propose(entry)
	clips, _, _, err := build.finish()
	if err != nil {
		return "", err
	}
	if len(clips) == 0 {
		if ctx.Err() != nil {
			return "", ErrCancelled
		}
		return "", renderErr("there is nothing to make a clip of at %s", HMS(req.At))
	}
	return HandPlanName + "/" + clips[0].ID, nil
}

// HandPlanPath is where this episode's clips made by hand are kept.
func (p *Project) HandPlanPath() string { return filepath.Join(p.LogsDir(), HandPlanName) }

// handEntry is the clip the playhead proposes: forward, it starts with the
// sentence the playhead stands in, from that sentence's beginning, and
// takes the lines after it. Backward, it ends with that sentence, at its
// end, and takes the lines before it, for a moment noticed only once it
// has passed. Either way it grows a line at a time until it is as long as
// shortest, never past longest for the sake of a line more. From there the
// builder puts its other edge on a sentence, the way it does every clip's.
func handEntry(lines []Line, req ClipRequest, shortest, longest float64,
	seconds func([][2]int) float64) (PlanEntry, error) {
	if len(lines) == 0 {
		return PlanEntry{}, renderErr("nothing is said here yet")
	}
	// The line the playhead stands in, counted from 1. In a pause, that is
	// the line after it for a clip that starts here and the line before it
	// for one that ends here.
	here := 0
	for n := 1; n <= len(lines); n++ {
		if lines[n-1].End() > req.At {
			here = n
			break
		}
	}
	switch {
	case here == 0 && !req.Backward:
		return PlanEntry{}, renderErr("nothing is said after %s", HMS(req.At))
	case here == 0:
		here = len(lines)
	case req.Backward && lines[here-1].Start() > req.At:
		if here == 1 {
			return PlanEntry{}, renderErr("nothing is said before %s", HMS(req.At))
		}
		here--
	}
	startOf := func(n int) int { return sentenceStart(lines, n) }
	endOf := func(n int) int { return sentenceEnd(lines, n) }
	length := func(first, last int) float64 { return seconds([][2]int{{first, last}}) }
	var first, last int
	if req.Backward {
		last = endOf(here)
		first = startOf(last)
		for first > 1 && length(first, last) < shortest && length(first-1, last) <= longest {
			first--
		}
	} else {
		first, last = startOf(here), here
		for last < len(lines) && length(first, last) < shortest && length(first, last+1) <= longest {
			last++
		}
	}
	title := handTitle(lines[first-1 : last])
	return PlanEntry{Slug: strings.ToLower(SanitiseName(title, "clip")), Title: title,
		Keep: [][2]int{{first, last}}}, nil
}

// handTitle is the first words said in a clip, which is what a person
// would call it until they call it something else.
func handTitle(lines []Line) string {
	var words []string
	more := false
	for _, l := range lines {
		for _, w := range l.Cues {
			text := strings.TrimFunc(w.Text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
			if text == "" {
				continue
			}
			if len(words) == 6 {
				more = true
				break
			}
			words = append(words, text)
		}
	}
	title := strings.Join(words, " ")
	if title == "" {
		return "Clip"
	}
	if more {
		title += " …"
	}
	return Scrub(title, 200)
}
