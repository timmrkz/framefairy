package engine

import (
	"context"
	"errors"
	"fmt"
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

// ErrStepFailed is returned when a step ended with an error that was already
// reported as an event. LastError gives its text.
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
func (p *Project) LogsDir() string { return filepath.Join(p.WorkDir(), "logs") }

// CaptionsDir holds the editable captions.
func (p *Project) CaptionsDir() string { return filepath.Join(p.WorkDir(), "captions") }

// LastError is the text of the last error the most recent step reported.
func (p *Project) LastError() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

func (p *Project) run(ctx context.Context, opts Options) error {
	p.mu.Lock()
	p.last = ""
	p.mu.Unlock()
	log := p.engine.Log
	log.SetErrorHook(func(text string) {
		p.mu.Lock()
		p.last = text
		p.mu.Unlock()
	})
	defer log.SetErrorHook(nil)
	code := p.engine.Run(ctx, opts)
	switch {
	case code == 0:
		return nil
	case code == 130 || ctx.Err() != nil:
		return ErrCancelled
	}
	return ErrStepFailed
}

// StopAt gives the transcription a place to stop for now, asked on every
// chunk: the end of the window the first search is waiting for, or 0.
func (p *Project) StopAt(at func() float64) {
	p.engine.StopAt = at
}

// Transcribe makes sure the whole episode has a transcript. It is cached, so
// calling it again costs nothing.
func (p *Project) Transcribe(ctx context.Context) error {
	opts := p.Base
	opts.TranscribeOnly = true
	opts.From, opts.To = "", ""
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
}

// Plan makes candidate clips and returns the plan file's path. A window
// that is not the whole episode gets its own plan file, as with --from and
// --to on the command line. An existing plan for the same window is reused
// unless Replan is set.
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
	name := PlanName(nil)
	if req.From > 0 || req.To > 0 {
		info, err := p.engine.Probe(ctx, p.Source)
		if err != nil {
			if ctx.Err() != nil {
				return "", ErrCancelled
			}
			return "", err
		}
		from, to := req.From, req.To
		if to <= 0 || to > info.Duration {
			to = info.Duration
		}
		if from > 0.0005 || to < info.Duration-0.0005 {
			opts.From, opts.To = fixed(from, 3), fixed(to, 3)
			name = PlanName(&Window{from, to})
		}
	}
	// The window as it was asked for, so the note says the same window
	// the range picker showed.
	if err := p.noted(req.From, req.To, func() error { return p.run(ctx, opts) }); err != nil {
		return "", err
	}
	return filepath.Join(p.LogsDir(), name), nil
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
