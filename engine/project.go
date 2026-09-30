package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Project is one episode, driven step by step as the app does it. Every step
// goes through Run with the same options the command line would pass, so the
// app and the command line always behave the same.
type Project struct {
	// Source is the episode file.
	Source string
	// Base holds the settings every step starts from.
	Base Options

	engine *Engine
	mu     sync.Mutex
	last   string
}

// ErrStepFailed is what every step that failed is, with the reason wrapped
// in it. LastError gives the reason's text.
var ErrStepFailed = errors.New("step failed")

// ErrCancelled is returned when a step was stopped through its context.
var ErrCancelled = errors.New("cancelled")

// NewProject makes a project for an episode. The engine's log should have a
// sink, because that is where every message goes.
func NewProject(e *Engine, source string, base Options) *Project {
	base.Source = source
	return &Project{Source: source, Base: base, engine: e}
}

// WorkDir is the folder next to the episode that holds everything.
func (p *Project) WorkDir() string { return WorkDir(p.Source) }

// LogsDir holds the transcript, plans and records.
func (p *Project) LogsDir() string { return LogsDir(p.Source) }

// LogsDir is the folder that holds the transcript, plans and records of the
// episode at source.
func LogsDir(source string) string { return filepath.Join(WorkDir(source), "logs") }

// CaptionsDir holds the editable captions.
func (p *Project) CaptionsDir() string { return filepath.Join(p.WorkDir(), "captions") }

// LastError is the text of the last error the most recent step reported.
func (p *Project) LastError() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

// run is one step, and what went wrong is the error the engine returned,
// not the last line it logged. That line is still logged, for the app's
// activity and anything else that reads the log. The error keeps its kind
// under ErrStepFailed.
func (p *Project) run(ctx context.Context, opts Options) error {
	p.mu.Lock()
	p.last = ""
	p.mu.Unlock()
	err := p.engine.execute(ctx, opts)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrCancelled) || ctx.Err() != nil:
		return ErrCancelled
	}
	p.engine.Log.Error("%s", err)
	p.mu.Lock()
	p.last = err.Error()
	p.mu.Unlock()
	return fmt.Errorf("%w: %w", ErrStepFailed, err)
}

// Transcribe makes sure the whole episode has a transcript. It is cached, so
// calling it again costs nothing.
func (p *Project) Transcribe(ctx context.Context) error {
	return p.Hear(ctx, Window{})
}

// Hear makes sure the transcript has heard a window of the episode, and has
// it hear what of the window it has not. A window with no end is the whole
// episode.
func (p *Project) Hear(ctx context.Context, span Window) error {
	opts := p.Base
	opts.TranscribeOnly = true
	opts.From, opts.To = "", ""
	if span.End > 0 {
		opts.From, opts.To = fixed(span.Start, 3), fixed(span.End, 3)
	}
	return p.run(ctx, opts)
}

// PlanRequest is what the app asks the planner for.
type PlanRequest struct {
	// From and To limit planning to a window, in seconds. Zero To means the
	// end of the episode.
	From, To float64
	Count    int
	Min, Max float64
	// Replan asks the model again even if a plan for this window exists.
	Replan bool
	// Pass is which search of the window this is, each with a plan of its
	// own, see PassName. 0 is the first for Plan, and for Search the next
	// one the window has not had yet, which it then keeps in its record,
	// so a search carried on writes to the plan it began.
	Pass int
}

// Plan makes candidate clips and returns the plan file's path. A window
// that is not the whole episode gets its own plan file, as with --from and
// --to on the command line. An existing plan for the same window is reused
// unless Replan is set.
//
// It is the finding step of a search, see Search in jobs.go, which the app
// runs. On its own it keeps no record.
func (p *Project) Plan(ctx context.Context, req PlanRequest) (string, error) {
	opts := p.Base
	opts.PlanOnly = true
	opts.Replan = req.Replan
	opts.ExactPlan = true
	opts.From, opts.To, opts.ClipsPath = "", "", ""
	if req.Count > 0 {
		opts.Count = req.Count
	}
	if req.Min > 0 {
		opts.Min = req.Min
	}
	if req.Max > 0 {
		opts.Max = req.Max
	}
	window, err := p.planWindow(ctx, req)
	if err != nil {
		return "", err
	}
	name := PassName(window, req.Pass)
	if window != nil {
		opts.From, opts.To = fixed(window.Start, 3), fixed(window.End, 3)
	}
	opts.Pass = max(req.Pass, 1)
	// Every search is told what the searches before it proposed, so a
	// window searched again brings other moments rather than the same
	// ones, see Taken.
	opts.Taken = p.proposed(PlanFor(p.WorkDir(), opts.folder(), opts.Experiment, name))
	if err := p.run(ctx, opts); err != nil {
		return "", err
	}
	return PlanFor(p.WorkDir(), opts.folder(), opts.Experiment, name), nil
}

// warmChars is how many characters of prompt a second of window makes,
// with room to spare, see SpokenChars.
const warmChars = SpokenChars

// WarmModel loads the local model for a search of a window seconds long
// that has not started yet, so the search finds it loaded. It returns once
// the model is in memory, and the model waits a few minutes for the
// search. With the API, or a server that is already running, there is
// nothing to load.
func (p *Project) WarmModel(ctx context.Context, seconds float64) error {
	opts := p.Base
	if opts.Planner != "local" || opts.LLMURL != "" {
		return nil
	}
	local, err := resolveLocal(opts)
	if err != nil {
		return err
	}
	chars := int(max(seconds, 60)*warmChars) + runeLen(SystemPrompt)
	_, release, err := p.engine.warmModel(ctx, *local, localContextFor(local.Model, chars, opts.MaxTokens), p.LogsDir())
	if errors.Is(err, errModelBusy) {
		// Another model is in use, a search of another episode. The search
		// this was for loads the model itself when it gets its turn.
		return nil
	}
	if err != nil {
		return err
	}
	release(warmKeep)
	return nil
}

// PlanName is the plan file for a window, or for the whole episode when
// window is nil.
func PlanName(window *Window) string {
	if window == nil {
		return "clips.json"
	}
	return fmt.Sprintf("clips-%d-%d.json", int(window.Start), int(window.End))
}

// PassName is the plan file of a search of a window after the first,
// clips-<from>-<to>-<pass>.json. The first is PlanName's, so the plans
// made before there were passes are the first pass of their windows. A
// later pass is always of a window, the whole episode too, so its name
// says which.
func PassName(window *Window, pass int) string {
	if pass <= 1 || window == nil {
		return PlanName(window)
	}
	return fmt.Sprintf("clips-%d-%d-%d.json", int(window.Start), int(window.End), pass)
}

// planWindow is the window a request is for, or nil for the whole
// episode. A later pass over the whole episode is a window from its start
// to its end, see PassName.
func (p *Project) planWindow(ctx context.Context, req PlanRequest) (*Window, error) {
	if req.From <= 0 && req.To <= 0 && req.Pass <= 1 {
		return nil, nil
	}
	info, err := p.engine.Probe(ctx, p.Source)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		return nil, err
	}
	return searchWindow(req.From, req.To, req.Pass, info.Duration), nil
}

// searchWindow is the window a search of from to to asks for, in an
// episode duration seconds long, or nil for the whole episode. A later
// pass over the whole episode is a window from its start to its end, see
// PassName. The search, its record and Run all name the plan from it.
//
// The edges are rounded to the millisecond, the way the window is handed
// to Run, which names the plan from that. Named from an edge as asked, a
// hair under a whole second, the plan a search answered with was a second
// off the plan Run wrote.
func searchWindow(from, to float64, pass int, duration float64) *Window {
	if from <= 0 && to <= 0 && pass <= 1 {
		return nil
	}
	from, to = toMillisecond(from), toMillisecond(to)
	if to <= 0 || to > duration {
		to = duration
	}
	if pass > 1 || from > 0.0005 || to < duration-0.0005 {
		return &Window{from, to}
	}
	return nil
}

// toMillisecond rounds a time the way fixed(t, 3) writes it.
func toMillisecond(t float64) float64 { return math.Round(t*1000) / 1000 }

// nextPass is the first pass of a window that has no plan yet.
func (p *Project) nextPass(ctx context.Context, req PlanRequest) (int, error) {
	opts := p.Base
	for pass := 1; ; pass++ {
		req.Pass = pass
		window, err := p.planWindow(ctx, req)
		if err != nil {
			return 0, err
		}
		if !exists(PlanFor(p.WorkDir(), opts.folder(), opts.Experiment, PassName(window, pass))) {
			return pass, nil
		}
	}
}

// proposed is every part of the episode a clip has been proposed for, by
// a search or by hand, in every plan but the one about to be written. A
// clip that was removed counts too: it was proposed, and turned down.
func (p *Project) proposed(except string) []Window {
	var out []Window
	for _, plan := range p.Plans() {
		if plan == except || !IsPlanFile(plan) {
			continue
		}
		_, clips, err := LoadClips(plan)
		if err != nil {
			continue
		}
		for _, c := range clips {
			for _, seg := range c.Segments {
				if seg.End > seg.Start {
					out = append(out, Window{seg.Start, seg.End})
				}
			}
		}
	}
	return MergeWindows(out)
}

func (p *Project) planFiles() map[string]int64 {
	found := map[string]int64{}
	matches, _ := filepath.Glob(filepath.Join(p.LogsDir(), "clips*.json"))
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() {
			found[m] = info.ModTime().UnixNano()
		}
	}
	return found
}

// Plans lists the plan files of this episode, newest first.
func (p *Project) Plans() []string {
	found := p.planFiles()
	var names []string
	for name := range found {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b int) bool { return found[names[a]] > found[names[b]] })
	return names
}

// RenderRequest says which clips of which plan to render.
type RenderRequest struct {
	Plan string
	// Clips are clip ids or basenames. Empty means all of them.
	Clips []string
	// Preview renders fast at half size into preview/.
	Preview bool
}

// Render renders clips from a plan.
func (p *Project) Render(ctx context.Context, req RenderRequest) error {
	opts := p.Base
	opts.ClipsPath = req.Plan
	opts.Clip = append([]string(nil), req.Clips...)
	opts.Preview = req.Preview
	opts.From, opts.To = "", ""
	return p.run(ctx, opts)
}
