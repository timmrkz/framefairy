package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options are everything a run can be told. The command line fills these in,
// and a GUI can fill in the same struct.
type Options struct {
	Source         string
	ClipsPath      string
	Replan         bool
	PlanOnly       bool
	TranscribeOnly bool
	// ASRModel is the speech model folder. Empty means the default location.
	ASRModel string

	Count     int
	Min       float64
	Max       float64
	From      string
	To        string
	MaxPause  *float64
	KeepPause float64
	SilenceDB *float64
	Context   string

	// Planner is "local", the default, or "api".
	Planner   string
	LLMModel  string
	LLMServer string
	LLMURL    string
	Model     string
	MaxTokens int
	// Think is the most a local model may think, in tokens, negative for
	// no limit.
	Think   int
	Budget  float64
	Prefill bool

	FFmpeg  string
	FFprobe string

	NoCaptions      bool
	Clip            []string
	Out             string
	Width           int
	Height          int
	NoUpscale       bool
	CRF             int
	Preset          string
	Encoder         string
	AudioBitrate    string
	Preview         bool
	Font            string
	FontSize        int
	MarginV         *int
	RefreshCaptions bool
	// ExactPlan never falls back to another plan file when the one for this
	// window is missing, it makes the plan instead. The app sets it, the
	// command line does not.
	ExactPlan bool
	// NoRecord stops training records from being written.
	NoRecord bool
	// TrainingDir is the one folder the records go in. Empty means the
	// default, ~/.framefairy/training.
	TrainingDir     string
	HighlightColour string
	NoHighlight     bool
	DryRun          bool
}

// DefaultOptions are the defaults the command line documents.
func DefaultOptions() Options {
	return Options{
		Count: 12, Min: 20, Max: 30, KeepPause: 0.10, Planner: "local",
		Model: DefaultModel, MaxTokens: 48000, Think: DefaultThink, Budget: 2.00,
		Width: 1080, Height: 1920, CRF: 18, Preset: "slow", AudioBitrate: "256k",
	}
}

// WorkDir is where a run keeps everything for one episode, next to the
// source.
func WorkDir(source string) string {
	base := filepath.Base(source)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return filepath.Join(filepath.Dir(source), stem+".framefairy")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Run executes one complete run and returns the process exit code.
func (e *Engine) Run(ctx context.Context, opts Options) int {
	log := e.Log
	e.Prefill = opts.Prefill
	if opts.TrainingDir != "" {
		SetTrainingDir(opts.TrainingDir)
	}
	if opts.FFmpeg != "" {
		e.FFmpeg = opts.FFmpeg
	}
	if opts.FFprobe != "" {
		e.FFprobe = opts.FFprobe
	} else if opts.FFmpeg != "" {
		name := "ffprobe"
		if strings.EqualFold(filepath.Ext(opts.FFmpeg), ".exe") {
			name = "ffprobe.exe"
		}
		if guess := filepath.Join(filepath.Dir(opts.FFmpeg), name); exists(guess) {
			e.FFprobe = guess
		}
	}

	if !exists(opts.Source) {
		log.Error("source not found: %s", opts.Source)
		return 1
	}
	if opts.Planner != "local" && opts.Planner != "api" {
		log.Error("--planner must be local or api")
		return 1
	}
	// A shortest longer than the longest leaves nothing a clip could be, and
	// the model would be asked for clips between thirty and twenty seconds.
	if opts.Max > 0 && opts.Min > opts.Max {
		log.Error("the shortest clip cannot be longer than the longest: %s over %s",
			trimFloat(opts.Min), trimFloat(opts.Max))
		return 1
	}
	if opts.Count < 1 {
		log.Error("a search has to look for at least one clip")
		return 1
	}
	for _, dim := range []struct {
		name  string
		value int
	}{{"width", opts.Width}, {"height", opts.Height}} {
		if dim.value < 16 || dim.value > 8192 || dim.value%2 != 0 {
			log.Error("--%s must be an even number between 16 and 8192", dim.name)
			return 1
		}
	}

	work := WorkDir(opts.Source)
	outDir := opts.Out
	if outDir == "" {
		outDir = filepath.Join(work, "out")
	}
	captionDir := filepath.Join(work, "captions")
	if err := os.MkdirAll(work, 0o755); err != nil {
		log.Error("cannot create %s: %s", work, err)
		return 1
	}

	e.SkipCaptions = opts.NoCaptions
	e.WantEncoder = opts.Encoder
	log.Info("%s", filepath.Base(opts.Source))

	if err := e.Preflight(ctx); err != nil {
		return e.fail(ctx, err)
	}

	source, err := e.Probe(ctx, opts.Source)
	if err != nil {
		if ctx.Err() != nil {
			return e.fail(ctx, ctx.Err())
		}
		log.Error("cannot read %s: %s", opts.Source, err)
		return 1
	}

	rs := RenderSettings{OutW: opts.Width, OutH: opts.Height, CRF: opts.CRF,
		Preset: opts.Preset, AudioBitrate: opts.AudioBitrate, ScaleUp: !opts.NoUpscale}
	if opts.Preview {
		rs.OutW, rs.OutH = opts.Width/2, opts.Height/2
		rs.CRF, rs.Preset = 30, "veryfast"
		if opts.Out == "" {
			outDir = filepath.Join(work, "preview")
		}
	}

	log.Info("source: %dx%d @ %s fps, %s min", source.Width, source.Height,
		fixed(source.FPS(), 3), fixed(source.Duration/60, 1))

	// Working a long episode in passes. Only the audio inside the window is
	// transcribed and sent, and the plan for each window is kept in its own
	// file so the passes accumulate instead of overwriting each other.
	var window *Window
	span := Window{0, source.Duration}
	if opts.From != "" || opts.To != "" {
		startAt, endAt := 0.0, source.Duration
		if opts.From != "" {
			if startAt, err = ParseTime(opts.From); err != nil {
				log.Error("%s", err)
				return 1
			}
		}
		if opts.To != "" {
			if endAt, err = ParseTime(opts.To); err != nil {
				log.Error("%s", err)
				return 1
			}
		}
		if source.Duration > 0 {
			endAt = math.Min(endAt, source.Duration)
		}
		if endAt <= startAt {
			log.Error("--to must be later than --from, and --from inside the video")
			return 1
		}
		window = &Window{startAt, endAt}
		span = *window
		log.Info("window %s to %s", HMS(startAt), HMS(endAt))
	}
	if span.End <= span.Start {
		log.Error("%s reports no duration, so there is no audio to work with", opts.Source)
		return 1
	}

	// The clip plan is something the tool produces, not something you
	// write. A saved plan is reused so hand edits survive, and --replan
	// starts over. The plan and the proof are both records of a run rather
	// than things you publish, so they live with the other records.
	logsDir := filepath.Join(work, "logs")
	planName := "clips.json"
	if window != nil {
		planName = PlanName(window)
	}
	planPath := opts.ClipsPath
	if planPath == "" {
		planPath = filepath.Join(logsDir, planName)
	}

	// Plans written by an early version sat in the work directory itself.
	// They are moved rather than ignored, so a plan you are happy with is not
	// quietly replaced by a paid-for new one.
	if opts.ClipsPath == "" {
		stale, _ := filepath.Glob(filepath.Join(work, "clips*.json"))
		sort.Strings(stale)
		for _, old := range stale {
			if !isFile(old) {
				continue
			}
			moved := filepath.Join(logsDir, filepath.Base(old))
			if exists(moved) {
				continue
			}
			if err := os.MkdirAll(logsDir, 0o755); err != nil {
				continue
			}
			if err := os.Rename(old, moved); err == nil {
				log.Info("moved %s into logs/", filepath.Base(old))
			}
		}
	}

	// A plan made with --from/--to is saved under the window's name, so a
	// later run without those flags would not find it and would quietly buy
	// another one. Planning costs money, so an existing plan is looked for
	// before that happens rather than after.
	if opts.ClipsPath == "" && !exists(planPath) && !opts.Replan && !opts.TranscribeOnly &&
		!opts.ExactPlan {
		matches, _ := filepath.Glob(filepath.Join(logsDir, "clips*.json"))
		var existing []string
		for _, m := range matches {
			if isFile(m) {
				existing = append(existing, m)
			}
		}
		sort.Strings(existing)
		if len(existing) == 1 {
			planPath = existing[0]
			log.Info("using the existing plan %s", filepath.Base(planPath))
		} else if len(existing) > 1 {
			log.Error("no %s, but several plans exist in %s:", planName, logsDir)
			for _, candidate := range existing {
				log.Error("    %s", filepath.Base(candidate))
			}
			log.Error("choose one with --clips, or repeat the --from and --to it was " +
				"made with. Add --replan to make a new one.")
			return 1
		}
	}

	outW, outH := rs.OutW, rs.OutH
	plannedNow := false
	if opts.TranscribeOnly || (opts.ClipsPath == "" && (opts.Replan || !exists(planPath))) {
		// Transcription takes minutes, so a missing key is reported before it
		// rather than after. A saved reply can make the key unnecessary, so
		// with one around this is only a warning.
		var local *LocalModel
		if !opts.TranscribeOnly && opts.Planner == "local" {
			local, err = resolveLocal(opts)
			if err != nil {
				log.Error("%s", err)
				return 1
			}
		}
		if !opts.TranscribeOnly && opts.Planner == "api" {
			if _, err := ReadAPIKey(ctx); err != nil {
				saved, _ := filepath.Glob(filepath.Join(logsDir, "reply-*.json"))
				if len(saved) == 0 || opts.Replan {
					log.Error("%s", err)
					return 1
				}
				log.Warn("no API key found. Planning only works if a saved reply matches.")
			}
		}
		// Clips at their shortest, one after another, have to fit in the
		// window, or the model is asked for more than is there. This is
		// the same sum the app holds its settings to, see Holds.
		if !opts.TranscribeOnly && !Holds(span, opts.Count, opts.Min) {
			log.Error("%d clips of at least %ss need %s, and the window is %s. Ask for fewer "+
				"clips, shorter ones, or a longer window.", opts.Count, trimFloat(opts.Min),
				HMS(float64(opts.Count)*opts.Min), HMS(span.End-span.Start))
			return 1
		}
		modelDir := opts.ASRModel
		if modelDir == "" {
			modelDir = DefaultModelDir()
		}
		transcript, err := e.LoadTranscript(ctx, opts.Source, span, logsDir, modelDir,
			opts.SilenceDB, window != nil)
		if err != nil {
			if ctx.Err() != nil {
				return e.fail(ctx, err)
			}
			log.Error("transcription failed: %s", err)
			return 1
		}
		if opts.TranscribeOnly {
			name := strings.TrimSuffix(TranscriptName(window), ".json") + ".srt"
			readable := filepath.Join(logsDir, name)
			if err := WriteTranscriptSRT(transcript, readable); err != nil {
				log.Error("cannot write %s: %s", readable, err)
				return 1
			}
			log.OK("transcript written to %s", readable)
			return 0
		}
		plannedNow = true
		lines := BuildLines(transcript.Words, transcript.Levels(), opts.MaxPause)
		if len(lines) == 0 {
			log.Error("no speech was found in the audio")
			return 1
		}
		e.SummariseLines(transcript, lines, span)
		plan, err := e.BuildPlan(ctx, opts.Source, source, lines,
			PlanOptions{
				Count: opts.Count, MinLen: opts.Min, MaxLen: opts.Max,
				Context: opts.Context, Model: plannerName(opts), OutW: outW, OutH: outH,
				MaxTokens: opts.MaxTokens, Budget: opts.Budget, LogDir: logsDir,
				Window: window, MaxPause: opts.MaxPause, KeepPause: opts.KeepPause,
				Fresh: opts.Replan, Local: local, Record: !opts.NoRecord,
				PlanPath: planPath, CaptionDir: captionDir,
			})
		if err != nil {
			return e.planFailed(ctx, err)
		}
		// Each clip was written to the plan the moment it was framed, so
		// there is nothing left to write.
		log.OK("plan written to %s with %d clip(s)", planPath, len(plan.Clips))
	}

	if !exists(planPath) {
		log.Error("clip plan not found: %s", planPath)
		return 1
	}

	plan, clips, err := LoadClips(planPath)
	if opts.ClipsPath == "" && !plannedNow && err == nil {
		made := plan.PlannedWith()
		var differs []string
		for _, check := range []struct {
			name  string
			value float64
		}{{"count", float64(opts.Count)}, {"min", opts.Min}, {"max", opts.Max}} {
			if raw, present := made[check.name]; present {
				if was, ok := toFloat(raw); !ok || was != check.value {
					shown := pyStr(raw)
					if ok {
						shown = trimFloat(was)
					}
					differs = append(differs, fmt.Sprintf("%s was %s, now %s",
						check.name, shown, trimFloat(check.value)))
				}
			}
		}
		if model, present := made["model"]; present && model != nil && pyStr(model) != plannerName(opts) {
			differs = append(differs, fmt.Sprintf("model was %s, now %s", pyStr(model), plannerName(opts)))
		}
		if len(differs) > 0 {
			log.Warn("the saved plan was made with different settings:")
			for _, d := range differs {
				log.Warn("    %s", d)
			}
			log.Warn("it is being reused as is. Add --replan to choose again with the "+
				"current ones, or delete %s", filepath.Base(planPath))
		} else if len(made) == 0 {
			log.Detail("the saved plan predates settings stamping")
		}
	}
	if err != nil {
		log.Error("clip plan is invalid: %s", err)
		return 1
	}
	if len(clips) == 0 {
		log.Error("clip plan contains no clips")
		return 1
	}

	if opts.PlanOnly {
		log.Info("plan only, nothing rendered. Edit %s and run again.", planPath)
		return 0
	}

	if len(opts.Clip) == 0 {
		var kept []Clip
		for _, c := range clips {
			if c.Rejected {
				log.Info("%s is marked rejected, skipped", c.Basename())
			} else {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			log.Error("every clip in the plan is marked rejected")
			return 1
		}
		clips = kept
	} else {
		wanted := map[string]bool{}
		for _, c := range opts.Clip {
			wanted[c] = true
		}
		var picked []Clip
		for _, c := range clips {
			if wanted[c.ID] || wanted[c.Basename()] {
				picked = append(picked, c)
			}
		}
		if len(picked) == 0 {
			names := make([]string, 0, len(wanted))
			for name := range wanted {
				names = append(names, name)
			}
			sort.Strings(names)
			log.Error("no clip matched %s", pyListRepr(names))
			return 1
		}
		clips = picked
	}

	style := plan.CaptionStyle()
	if opts.Font != "" {
		style["font"] = opts.Font
	}
	if opts.FontSize != 0 {
		style["size"] = float64(opts.FontSize)
	}
	if opts.MarginV != nil {
		style["margin_v"] = float64(*opts.MarginV)
	}
	if opts.HighlightColour != "" {
		if highlightColour(opts.HighlightColour, "") == "" {
			log.Error("--highlight-colour must look like #942192")
			return 1
		}
		// A plan given a highlight colour of its own, in the captions
		// column of the app, keeps it. This is the colour for the rest.
		if _, own := style["highlight_colour"]; !own {
			style["highlight_colour"] = opts.HighlightColour
		}
	}
	if opts.NoHighlight {
		style["highlight"] = 0.0
	}

	cropW, cropH := CropWindow(source, outW, outH)
	if opts.NoUpscale {
		log.Info("crop %dx%d, native, no resampling", cropW, cropH)
	} else {
		factor := float64(outW) / float64(cropW)
		direction := "downscale"
		if factor > 1 {
			direction = "upscale"
		}
		log.Info("crop %dx%d -> %dx%d (%sx %s)", cropW, cropH, outW, outH,
			fixed(factor, 2), direction)
	}

	failures := 0
	for _, clip := range clips {
		for _, seg := range clip.Segments {
			if seg.End <= seg.Start {
				log.Error("%s: segment end is not after start", clip.Basename())
				failures++
			}
		}
		last := clip.Segments[len(clip.Segments)-1]
		if source.Duration != 0 && last.End > source.Duration+0.5 {
			log.Error("%s: segment runs past the end of the source", clip.Basename())
			failures++
		}
	}
	if failures > 0 {
		return 1
	}

	maxChars := 38
	if raw, present := style["max_chars"]; present {
		if n, ok := toInt(raw); ok {
			maxChars = n
		}
	}

	cueMap := map[string][]Caption{}
	for _, clip := range clips {
		cues, err := resolveCues(clip, captionDir, opts.RefreshCaptions, maxChars)
		if err != nil {
			log.Error("caption problem: %s", err)
			failures++
			continue
		}
		if len(cues) == 0 && len(clip.Words) == 0 && !opts.NoCaptions {
			log.Warn("%s has no word timings, so it gets no captions. The plan was "+
				"made by an earlier version. Add --replan to make a new one.", clip.Basename())
		}
		cueMap[clip.Basename()] = cues
		started := time.Now()
		log.Info("%s: %ss, %d segment(s), %d cues", clip.Basename(),
			fixed(clip.Duration(), 1), len(clip.Segments), len(cues))
		path, err := e.RenderClip(ctx, clip, opts.Source, source, outDir, cues, rs,
			clipStyle(style, clip), captionDir, opts.DryRun)
		if err != nil {
			if ctx.Err() != nil {
				return e.fail(ctx, ctx.Err())
			}
			log.Error("%s failed: %s", clip.Basename(), err)
			failures++
			continue
		}
		if !opts.DryRun {
			size := 0.0
			if info, err := os.Stat(path); err == nil {
				size = float64(info.Size()) / 1_000_000
			}
			log.OK("%s  %s MB  in %ss", filepath.Base(path), fixed(size, 1),
				fixed(time.Since(started).Seconds(), 1))
		}
	}

	if !opts.DryRun {
		proof := filepath.Join(logsDir, "proof.txt")
		_ = os.MkdirAll(logsDir, 0o755)
		_ = os.MkdirAll(outDir, 0o755)
		if err := writeProof(proof, clips, cueMap); err != nil {
			log.Warn("could not write %s: %s", proof, err)
		}
		log.Info("proof: %s", proof)
		log.Info("captions to correct: %s", captionDir)

		made := len(clips) - failures
		cost := ""
		if e.Calls.Requests > 0 {
			cost = fmt.Sprintf(", about $%s spent across %d API call(s)",
				fixed(e.Calls.Spent, 2), e.Calls.Requests)
		}
		if failures > 0 {
			log.Warn("finished with %d failure(s), %d clip(s) in %s%s", failures, made, outDir, cost)
		} else {
			log.OK("done: %d clip(s) in %s%s", made, outDir, cost)
		}
	}
	if failures > 0 {
		return 1
	}
	return 0
}

// resolveLocal finds the local model and server before anything slow starts.
func resolveLocal(opts Options) (*LocalModel, error) {
	m := &LocalModel{Server: opts.LLMServer, Model: opts.LLMModel, URL: opts.LLMURL,
		Think: opts.Think}
	if m.URL != "" {
		return m, nil
	}
	if m.Model == "" {
		found, err := DefaultLocalModel()
		if err != nil {
			return nil, err
		}
		m.Model = found
	}
	if _, err := os.Stat(m.Model); err != nil {
		return nil, renderErr("the language model %s cannot be read: %s", m.Model, err)
	}
	server := m.Server
	if server == "" {
		server = LlamaServerPath()
	}
	if _, err := exec.LookPath(server); err != nil {
		return nil, renderErr("%s was not found. Install llama.cpp as docs/INSTALL.md describes, "+
			"or point --llm-server at the binary.", server)
	}
	return m, nil
}

// plannerName identifies what makes the plan, for the saved-reply key and
// the settings stamp.
func plannerName(opts Options) string {
	if opts.Planner == "api" {
		return opts.Model
	}
	if opts.LLMURL != "" {
		return "local:" + opts.LLMURL
	}
	model := opts.LLMModel
	if model == "" {
		model, _ = DefaultLocalModel()
	}
	return "local:" + filepath.Base(model)
}

func trimFloat(v float64) string {
	return fmt.Sprintf("%g", v)
}

// resolveCues gives a clip's captions, preferring a file you may have
// corrected by hand. Once a per-clip file exists it is used as it is, which
// is what makes a hand correction stick.
func resolveCues(clip Clip, captionDir string, force bool, maxChars int) ([]Caption, error) {
	perClip, err := SafeChild(captionDir, clip.Basename()+".srt")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(captionDir, 0o755); err != nil {
		return nil, err
	}
	if exists(perClip) && !force {
		return LoadCaptions(perClip)
	}
	captions := Captions(clip, maxChars)
	if len(captions) == 0 {
		return nil, nil
	}
	return captions, WriteCaptions(captions, perClip)
}

// clipStyle is the caption look for one clip: the plan's look, with the
// caption line moved when it was placed by hand on that clip.
func clipStyle(style map[string]any, clip Clip) map[string]any {
	if clip.CaptionY == nil {
		return style
	}
	out := make(map[string]any, len(style)+1)
	for key, value := range style {
		out[key] = value
	}
	out["margin_v"] = *clip.CaptionY
	return out
}

// ClipCaptions gives a clip's captions the way the render will draw them,
// a caption file corrected by hand first, and writes nothing itself.
func ClipCaptions(clip Clip, captionDir string, maxChars int) ([]Caption, error) {
	perClip, err := SafeChild(captionDir, clip.Basename()+".srt")
	if err != nil {
		return nil, err
	}
	if exists(perClip) {
		return LoadCaptions(perClip)
	}
	return Captions(clip, maxChars), nil
}

func writeProof(path string, clips []Clip, cueMap map[string][]Caption) error {
	lines := []string{"Render proof", strings.Repeat("=", 60), ""}
	for _, clip := range clips {
		lines = append(lines, fmt.Sprintf("[%s]  %ss  %d segment(s)", clip.Basename(),
			fixed(clip.Duration(), 1), len(clip.Segments)))
		if clip.Title != "" {
			lines = append(lines, "  title: "+clip.Title)
		}
		for i, seg := range clip.Segments {
			crop := "centred"
			if seg.CropX != nil {
				crop = fmt.Sprintf("x=%d", *seg.CropX)
			}
			lines = append(lines, fmt.Sprintf("  %d. %9s - %9s  (%5ss, %s)", i+1,
				fixed(seg.Start, 3), fixed(seg.End, 3), fixed(seg.Duration(), 2), crop))
		}
		if cues := cueMap[clip.Basename()]; len(cues) > 0 {
			texts := make([]string, len(cues))
			for i, c := range cues {
				texts[i] = c.Text
			}
			lines = append(lines, fmt.Sprintf("  captions: %d cues", len(cues)),
				"  text: "+strings.Join(texts, " "))
		} else {
			lines = append(lines, "  captions: none")
		}
		lines = append(lines, "")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// setStaleCaptionsAside moves every caption file out of the way of a new
// plan. A new plan means new cut points, so captions derived from the old one
// are timed to a timeline that no longer exists. They may still contain
// hand corrections, so they are moved aside rather than deleted. Nothing
// this tool does removes your work.
func setStaleCaptionsAside(log *Log, captionDir string) {
	stale, _ := filepath.Glob(filepath.Join(captionDir, "*.srt"))
	var files []string
	for _, f := range stale {
		if isFile(f) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return
	}
	attic := filepath.Join(captionDir, "superseded-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(attic, 0o755); err != nil {
		return
	}
	for _, old := range files {
		_ = os.Rename(old, filepath.Join(attic, filepath.Base(old)))
	}
	generated, _ := filepath.Glob(filepath.Join(captionDir, "*.ass"))
	for _, old := range generated {
		if isFile(old) {
			_ = os.Remove(old) // regenerated every render, not yours
		}
	}
	log.Info("previous captions moved to %s", attic)
}

// Interrupted is the message shown when a run is stopped with ctrl-c.
const Interrupted = "interrupted. Anything already finished is kept, whatever was in " +
	"progress is discarded, and the clips a search had already found stay in its plan."

func (e *Engine) fail(ctx context.Context, err error) int {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		e.Log.ClearProgress()
		e.Log.Warn(Interrupted)
		return 130
	}
	e.Log.Error("%s", err)
	return 1
}

func (e *Engine) planFailed(ctx context.Context, err error) int {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return e.fail(ctx, err)
	}
	var re *RenderError
	if errors.As(err, &re) {
		e.Log.Error("planning failed: %s", err)
		return 1
	}
	e.Log.Error("planning failed: %s", err)
	return 1
}
