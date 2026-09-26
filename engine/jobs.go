package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Jobs and their records
//
// A job is one piece of work a person started, or the app started for
// them: a search, which is hearing and then finding, or a render. Each keeps
// one record in the episode's work folder, in jobs/, written every time it
// goes from one step to the next. The record is the only place the job's
// state is kept, so an app that starts reads what every job was doing when
// it last ran: a record in a running step is a job that was cut off by the
// app closing or the machine going off. See docs/JOBS.md.
//
// A job that is done or called off has no record. What it made is in the
// transcript, the plans and the shorts. Being called off is the app's to
// clear, where the hand is: the engine only sees its context end, which the
// app closing does too, and that one leaves the record as it was.
//
// Each step leaves something that stays however the job ends: the
// transcript as far as it was heard, the clips as they land, the shorts
// that were finished. Carrying on starts from there.
// ---------------------------------------------------------------------------

// The steps a job goes through.
const (
	StepWaiting   = "waiting"
	StepHearing   = "hearing"
	StepFinding   = "finding"
	StepRendering = "rendering"
	StepFailed    = "failed"
)

// The kinds of job.
const (
	JobSearch = "search"
	JobRender = "render"
)

// SearchID is the id of an episode's search. An episode has one search at
// a time, and a new one takes the place of the last.
const SearchID = "search"

// JobRecord is what the work folder keeps about a job.
type JobRecord struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	// A search: the window, To 0 for the end of the episode, and the
	// numbers it asks for.
	From   float64 `json:"from,omitempty"`
	To     float64 `json:"to,omitempty"`
	Count  int     `json:"count,omitempty"`
	Min    float64 `json:"min,omitempty"`
	Max    float64 `json:"max,omitempty"`
	Replan bool    `json:"replan,omitempty"`

	// A render: the plan, its clips, empty for all of them, and the clips
	// finished so far.
	Plan    string   `json:"plan,omitempty"`
	Clips   []string `json:"clips,omitempty"`
	Preview bool     `json:"preview,omitempty"`
	Done    []string `json:"done,omitempty"`

	Step  string    `json:"step"`
	Error string    `json:"error,omitempty"`
	Asked time.Time `json:"asked"`
	// Steps is when each step began and ended, in the order they came.
	// The last one has no end while it runs.
	Steps []StepTime `json:"steps"`
}

// StepTime is one step of a job and how long it took.
type StepTime struct {
	Step string     `json:"step"`
	From time.Time  `json:"from"`
	To   *time.Time `json:"to,omitempty"`
}

// Request is what a search asks for.
func (r JobRecord) Request() PlanRequest {
	return PlanRequest{From: r.From, To: r.To, Count: r.Count, Min: r.Min, Max: r.Max, Replan: r.Replan}
}

// Render is what a render asks for, less the clips it has finished.
func (r JobRecord) Render() RenderRequest {
	return RenderRequest{Plan: r.Plan, Clips: r.Clips, Preview: r.Preview}
}

// Interrupted says whether the job was cut off: it was in a step that runs
// when whatever ran it went away. A failed job says why instead.
func (r JobRecord) Interrupted() bool {
	return r.Step != StepFailed
}

// JobsDir is where an episode's jobs keep their records.
func JobsDir(source string) string {
	return filepath.Join(WorkDir(source), "jobs")
}

// jobID is what an id may be. It names a file, and it is read back from
// disk, so it is held to something that cannot be a path.
var jobID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// reasonLimit is the most of a reason a record keeps. A reason is one line
// said to a person, and a record is read every time the app starts.
const reasonLimit = 500

// recordLimit is the most a record may weigh. A record holds a few numbers
// and a reason, and it is read every time the app starts.
const recordLimit = 64 << 10

// NewRenderID is a new id for a render.
func NewRenderID() string {
	return fmt.Sprintf("render-%d", time.Now().UnixNano())
}

// WriteJob writes a job's record, all at once.
func WriteJob(source string, rec JobRecord) error {
	if !jobID.MatchString(rec.ID) {
		return fmt.Errorf("%q is no job id", rec.ID)
	}
	if err := os.MkdirAll(JobsDir(source), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(JobsDir(source), rec.ID+".json"), body)
}

// RemoveJob takes a job's record away. A job that has none is fine.
func RemoveJob(source, id string) error {
	if !jobID.MatchString(id) {
		return nil
	}
	err := os.Remove(filepath.Join(JobsDir(source), id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// ReadJobs gives the records of an episode's jobs, the search first and
// the renders in the order they were asked for. Every file is read as
// untrusted: one that is not a record of this episode is no record.
func ReadJobs(source string) []JobRecord {
	entries, err := os.ReadDir(JobsDir(source))
	if err != nil {
		return nil
	}
	var out []JobRecord
	for _, e := range entries {
		name := e.Name()
		id := strings.TrimSuffix(name, ".json")
		if !e.Type().IsRegular() || id == name || !jobID.MatchString(id) {
			continue
		}
		if rec, ok := readJob(source, filepath.Join(JobsDir(source), name)); ok && rec.ID == id {
			out = append(out, rec)
		}
	}
	slices.SortStableFunc(out, func(a, b JobRecord) int {
		if (a.Kind == JobSearch) != (b.Kind == JobSearch) {
			if a.Kind == JobSearch {
				return -1
			}
			return 1
		}
		return a.Asked.Compare(b.Asked)
	})
	return out
}

// ReadSearch gives the record of the episode's search, or nil when it has
// none.
func ReadSearch(source string) *JobRecord {
	rec, ok := readJob(source, filepath.Join(JobsDir(source), SearchID+".json"))
	if !ok || rec.ID != SearchID {
		return nil
	}
	return &rec
}

func readJob(source, path string) (JobRecord, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > recordLimit {
		return JobRecord{}, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return JobRecord{}, false
	}
	var rec JobRecord
	if json.Unmarshal(body, &rec) != nil || !saneJob(source, &rec) {
		return JobRecord{}, false
	}
	return rec, true
}

// saneJob checks a record read from disk and trims what it may say.
func saneJob(source string, r *JobRecord) bool {
	if !jobID.MatchString(r.ID) {
		return false
	}
	switch r.Step {
	case StepWaiting, StepHearing, StepFinding, StepRendering, StepFailed:
	default:
		return false
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	switch r.Kind {
	case JobSearch:
		if r.ID != SearchID || !finite(r.From) || !finite(r.To) || (r.To > 0 && r.To <= r.From) ||
			r.Count < 0 || r.Count > 1000 || !finite(r.Min) || !finite(r.Max) || r.Step == StepRendering {
			return false
		}
	case JobRender:
		// The plan is one of this episode's, by name and by where it is.
		if r.ID == SearchID || filepath.Dir(r.Plan) != filepath.Join(WorkDir(source), "logs") ||
			!planFile.MatchString(filepath.Base(r.Plan)) || len(r.Clips) > 1000 || len(r.Done) > 1000 ||
			r.Step == StepHearing || r.Step == StepFinding {
			return false
		}
	default:
		return false
	}
	r.Error = strings.TrimSpace(r.Error)
	if runes := []rune(r.Error); len(runes) > reasonLimit {
		r.Error = string(runes[:reasonLimit])
	}
	if len(r.Steps) > 100 {
		r.Steps = r.Steps[len(r.Steps)-100:]
	}
	return true
}

// planFile is what a plan is called: clips.json, or clips-<from>-<to>.json
// for a window.
var planFile = regexp.MustCompile(`^clips(-[0-9]+-[0-9]+)?\.json$`)

// Turn waits until the job may start a step: the step's lane is free.
// Hearing, finding and rendering each have a lane, see docs/JOBS.md. The
// engine asks before every step. It is given the context the step runs in,
// which the app may end to take the lane back before the job is over, and
// a function that lets the lane go. nil runs every step at once, which is
// what the command line and the tests do.
type Turn func(ctx context.Context, step string) (context.Context, func(), error)

// job keeps a running job's record.
type job struct {
	p   *Project
	rec JobRecord
}

func (p *Project) startJob(rec JobRecord) *job {
	now := time.Now()
	rec.Asked = now
	rec.Step = StepWaiting
	rec.Steps = []StepTime{{Step: StepWaiting, From: now}}
	j := &job{p: p, rec: rec}
	j.write()
	return j
}

func (j *job) write() {
	if err := WriteJob(j.p.Source, j.rec); err != nil {
		j.p.engine.Log.Warn("could not note the %s: %s", j.rec.Kind, err)
	}
}

// step ends the step the job is in and starts the next.
func (j *job) step(name string) {
	now := time.Now()
	if n := len(j.rec.Steps); n > 0 && j.rec.Steps[n-1].To == nil {
		j.rec.Steps[n-1].To = &now
	}
	j.rec.Step = name
	j.rec.Steps = append(j.rec.Steps, StepTime{Step: name, From: now})
	j.write()
}

// turn waits for the lane of a step and then starts it. It gives the
// context the step runs in and what lets the lane go again.
func (j *job) turn(ctx context.Context, turn Turn, step string) (context.Context, func(), error) {
	// Stopped as the last step ended, it stays in that step.
	if ctx.Err() != nil {
		return nil, nil, ErrCancelled
	}
	stepCtx, release := ctx, func() {}
	if turn != nil {
		if j.rec.Step != StepWaiting {
			j.step(StepWaiting)
		}
		var err error
		stepCtx, release, err = turn(ctx, step)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ErrCancelled
			}
			return nil, nil, err
		}
	}
	j.step(step)
	return stepCtx, release, nil
}

// end writes down how the job ended. Done, it adds its timings to the
// episode's and takes its record away. Failed, the record says why.
// Stopped from outside, the record stays as it was, see above. A panic is
// written down as a failure and goes on to whoever catches it.
func (j *job) end(err *error) {
	if r := recover(); r != nil {
		j.fail(fmt.Sprint(r))
		panic(r)
	}
	switch {
	case *err == nil:
		now := time.Now()
		if n := len(j.rec.Steps); n > 0 && j.rec.Steps[n-1].To == nil {
			j.rec.Steps[n-1].To = &now
		}
		j.p.addTimings(j.rec)
		if rerr := RemoveJob(j.p.Source, j.rec.ID); rerr != nil {
			j.p.engine.Log.Warn("could not clear the %s: %s", j.rec.Kind, rerr)
		}
	case errors.Is(*err, ErrCancelled):
	default:
		reason := j.p.LastError()
		if reason == "" {
			reason = (*err).Error()
		}
		j.fail(reason)
	}
}

func (j *job) fail(reason string) {
	j.rec.Error = reason
	j.step(StepFailed)
	now := time.Now()
	j.rec.Steps[len(j.rec.Steps)-1].To = &now
	j.write()
}

// Search finds clips in a window of the episode: it hears the episode as
// far as the window reaches, and no further, and then the model reads the
// window and answers. It is the command line's run, transcribing and then
// planning, with a record kept around both. It returns the plan.
//
// What was heard stays however the search ends, and the next search
// carries on from there.
func (p *Project) Search(ctx context.Context, req PlanRequest, turn Turn) (plan string, err error) {
	j := p.startJob(JobRecord{ID: SearchID, Kind: JobSearch, From: req.From, To: req.To,
		Count: req.Count, Min: req.Min, Max: req.Max, Replan: req.Replan})
	defer j.end(&err)

	end, whole, err := p.windowEnd(ctx, req)
	if err != nil {
		return "", err
	}
	// Only as far as the window. The whole episode is heard to its end,
	// which finishes the transcript rather than holding it.
	if !whole {
		p.StopAt(func() float64 { return end })
		defer p.StopAt(nil)
	}
	for {
		covered, done := Coverage(p.Source, p.Base.ASRModel)
		if done || covered >= end-0.05 {
			break
		}
		stepCtx, release, err := j.turn(ctx, turn, StepHearing)
		if err != nil {
			return "", err
		}
		err = p.Transcribe(stepCtx)
		release()
		if errors.Is(err, ErrCancelled) && ctx.Err() == nil {
			// The lane was taken back, by a search that finds while this
			// one hears. What was heard is saved, and the search waits
			// for its turn to carry on.
			continue
		}
		if err != nil {
			return "", err
		}
		if covered, done := Coverage(p.Source, p.Base.ASRModel); !done && covered < end-0.05 {
			return "", fmt.Errorf("the transcription stopped at %s, before the end of the window at %s",
				HMS(covered), HMS(end))
		}
	}
	stepCtx, release, err := j.turn(ctx, turn, StepFinding)
	if err != nil {
		return "", err
	}
	defer release()
	return p.Plan(stepCtx, req)
}

// windowEnd is where a search's window ends, and whether that is the end
// of the episode.
func (p *Project) windowEnd(ctx context.Context, req PlanRequest) (float64, bool, error) {
	info, err := p.engine.Probe(ctx, p.Source)
	if err != nil {
		if ctx.Err() != nil {
			return 0, false, ErrCancelled
		}
		return 0, false, err
	}
	if req.To <= 0 || req.To >= info.Duration-0.05 {
		return info.Duration, true, nil
	}
	return req.To, false, nil
}

// RenderJob renders clips of a plan, one at a time, and keeps a record of
// the ones it has finished. Given the record of a render that was cut off
// or failed, it renders the clips that are not finished yet.
func (p *Project) RenderJob(ctx context.Context, id string, req RenderRequest, carry *JobRecord, turn Turn) (err error) {
	clips := req.Clips
	if len(clips) == 0 {
		view, err := ReadPlan(req.Plan)
		if err != nil {
			return err
		}
		for _, c := range view.Clips {
			if !c.Rejected {
				clips = append(clips, c.ID)
			}
		}
	}
	rec := JobRecord{ID: id, Kind: JobRender, Plan: req.Plan, Clips: req.Clips, Preview: req.Preview}
	if carry != nil && carry.ID == id && carry.Plan == req.Plan {
		rec.Done = carry.Done
	}
	j := p.startJob(rec)
	defer j.end(&err)
	stepCtx, release, err := j.turn(ctx, turn, StepRendering)
	if err != nil {
		return err
	}
	defer release()
	for _, clip := range clips {
		if slices.Contains(j.rec.Done, clip) {
			continue
		}
		if err := p.Render(stepCtx, RenderRequest{Plan: req.Plan, Clips: []string{clip}, Preview: req.Preview}); err != nil {
			return err
		}
		j.rec.Done = append(j.rec.Done, clip)
		j.write()
	}
	return nil
}

// timingsName is the file an episode's finished jobs add their timings to,
// one line each, in jobs/.
const timingsName = "timings.jsonl"

// Timing is one finished job in timings.jsonl: what it was, and how many
// seconds each of its steps took.
type Timing struct {
	Kind  string    `json:"kind"`
	Asked time.Time `json:"asked"`
	// Window is how long the window of a search was, 0 for the whole
	// episode, and Count how many clips it asked for.
	Window float64 `json:"window,omitempty"`
	Count  int     `json:"count,omitempty"`
	// Clips is how many clips a render rendered.
	Clips int `json:"clips,omitempty"`
	// Model is what found the clips: the local model's file, a server's
	// address, or the API's model.
	Model string        `json:"model,omitempty"`
	Steps []StepSeconds `json:"steps"`
	Total float64       `json:"total"`
}

// StepSeconds is how long one step took.
type StepSeconds struct {
	Step    string  `json:"step"`
	Seconds float64 `json:"seconds"`
}

// addTimings adds a finished job's timings to the episode's. They stay on
// the machine, and framefairy-train adds them up over the library.
func (p *Project) addTimings(rec JobRecord) {
	t := Timing{Kind: rec.Kind, Asked: rec.Asked}
	switch rec.Kind {
	case JobSearch:
		if rec.To > rec.From {
			t.Window = rec.To - rec.From
		}
		t.Count = rec.Count
		t.Model = p.modelName()
	case JobRender:
		t.Clips = len(rec.Done)
	}
	for _, s := range rec.Steps {
		if s.To == nil {
			continue
		}
		seconds := math.Round(s.To.Sub(s.From).Seconds()*1000) / 1000
		t.Steps = append(t.Steps, StepSeconds{Step: s.Step, Seconds: seconds})
		t.Total += seconds
	}
	t.Total = math.Round(t.Total*1000) / 1000
	line, err := json.Marshal(t)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(JobsDir(p.Source), timingsName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		p.engine.Log.Warn("could not keep the timings: %s", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// modelName is the model a search asks, as the timings name it.
func (p *Project) modelName() string {
	o := p.Base
	switch {
	case o.Planner == "api":
		return o.Model
	case o.LLMURL != "":
		return "local:" + o.LLMURL
	case o.LLMModel != "":
		return filepath.Base(o.LLMModel)
	}
	return "local"
}

// ReadTimings gives the timings of an episode's finished jobs, oldest
// first. A line that is not one is left out.
func ReadTimings(source string) []Timing {
	body, err := os.ReadFile(filepath.Join(JobsDir(source), timingsName))
	if err != nil {
		return nil
	}
	var out []Timing
	for _, line := range strings.Split(string(body), "\n") {
		var t Timing
		if line != "" && json.Unmarshal([]byte(line), &t) == nil && t.Kind != "" {
			out = append(out, t)
		}
	}
	return out
}
