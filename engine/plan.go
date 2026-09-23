package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// Window is the stretch of the episode a run works on, in seconds.
type Window struct {
	Start float64
	End   float64
}

// SummariseLines reports what the transcript looks like before any token is
// spent.
func (e *Engine) SummariseLines(t *Transcript, lines []Line, window Window) {
	pauses, marked := 0, 0
	for _, line := range lines {
		if line.GapBefore >= splitPause {
			pauses++
		}
		if line.GapBefore >= 0.7 || math.Abs(line.Level) >= 3.0 {
			marked++
		}
	}
	talking := make([]float64, len(t.Words))
	for i, w := range t.Words {
		talking[i] = w.End - w.Start
	}
	covered := math.Max(window.End-window.Start, 1.0)
	e.Log.Info("%s words in %s speech lines, %s pauses below %s dB, %s lines carrying a delivery note",
		commas(len(t.Words)), commas(len(lines)), commas(pauses), fixed(t.Floor, 0), commas(marked))
	e.Log.Detail("%s min of words in %s min of audio",
		fixed(pysum(talking)/60, 1), fixed(covered/60, 1))
}

// PlanOptions are the choices that shape a plan.
type PlanOptions struct {
	Count      int
	MinLen     float64
	MaxLen     float64
	Context    string
	Model      string
	OutW, OutH int
	MaxTokens  int
	Budget     float64
	LogDir     string
	Window     *Window
	MaxPause   *float64
	KeepPause  float64
	Fresh      bool
	// Record appends new model answers to the episode's training records.
	Record bool
	// Local plans on this machine instead of through the API.
	Local *LocalModel
	// PlanPath is where each clip is written the moment it is framed. The
	// first one replaces whatever plan was there.
	PlanPath string
	// CaptionDir holds the caption files a new plan makes stale. They are
	// set aside when the first clip lands.
	CaptionDir string
}

// The plan as written to disk. Field order matches the Python version, so a
// plan reads the same whichever version wrote it.

// PlanFile is a complete clips.json.
type PlanFile struct {
	Source      string      `json:"source"`
	PlanID      string      `json:"plan_id,omitempty"`
	PlannedWith PlannedWith `json:"planned_with"`
	Clips       []PlanClip  `json:"clips"`
}

// PlannedWith stamps the settings a plan was made under, so a later run can
// notice that a reused plan was made differently.
type PlannedWith struct {
	Count int      `json:"count"`
	Min   PyFloat  `json:"min"`
	Max   PyFloat  `json:"max"`
	Model string   `json:"model"`
	From  *PyFloat `json:"from"`
	To    *PyFloat `json:"to"`
}

// PlanClip is one clip in clips.json.
type PlanClip struct {
	ID       string        `json:"id"`
	Slug     string        `json:"slug"`
	Title    string        `json:"title"`
	Reason   string        `json:"reason"`
	Keep     [][2]int      `json:"keep"`
	Words    [][3]any      `json:"words"`
	Segments []PlanSegment `json:"segments"`
}

// PlanSegment is one segment in clips.json. CropX holds a pixel offset or
// the word "center".
type PlanSegment struct {
	Start PyFloat `json:"start"`
	End   PyFloat `json:"end"`
	CropX any     `json:"crop_x"`
}

// MarshalPlan writes a plan the way Python's json.dumps(indent=2,
// ensure_ascii=False) does.
func MarshalPlan(plan any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(plan); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (e *Engine) buildPrompt(lines []Line, opts PlanOptions) string {
	ask := []string{
		fmt.Sprintf("Choose exactly %d clip(s).", opts.Count),
		fmt.Sprintf("Each clip must total between %s and %s seconds once condensed.",
			fixed(opts.MinLen, 0), fixed(opts.MaxLen, 0)),
		fmt.Sprintf("Reaching the payoff matters more than being brief. If a clip needs "+
			"%s seconds to get there, take them.", fixed(opts.MaxLen, 0)),
		fmt.Sprintf("The transcript below is numbered from 1 to %d. Those numbers are what "+
			"you return. Each line shows its talking time in seconds, and any pause before "+
			"it, so a clip's length is the lines you keep plus the pauses inside the runs "+
			"you keep.", len(lines)),
	}
	if opts.Context != "" {
		ask = append(ask, "Episode context: "+opts.Context)
	}
	ask = append(ask, "", "Transcript:", "", AnnotateLines(lines), "",
		"Reply with the JSON object and nothing else. No preamble, no explanation, "+
			"no markdown fences.")
	return strings.Join(ask, "\n")
}

type savedReply struct {
	Parsed json.RawMessage `json:"parsed"`
	Text   *string         `json:"text"`
}

// BuildPlan turns the transcript into a complete plan in memory.
func (e *Engine) BuildPlan(ctx context.Context, sourcePath string, source SourceInfo,
	lines []Line, opts PlanOptions) (*PlanFile, error) {
	prompt := e.buildPrompt(lines, opts)

	// A reply that has been paid for is reused rather than bought again. If a
	// later stage crashes, or you simply re-run with the same transcript, the
	// API is not called a second time.
	sum := sha256.Sum256([]byte(opts.Model + "\x00" + prompt))
	fingerprint := hex.EncodeToString(sum[:])[:16]
	cachePath := ""
	if opts.LogDir != "" {
		if err := os.MkdirAll(opts.LogDir, 0o755); err != nil {
			return nil, err
		}
		cachePath = filepath.Join(opts.LogDir, "reply-"+fingerprint+".json")
	}

	reply, haveReply, fresh := "", false, false
	var how *localAnswer
	if cachePath != "" {
		if _, err := os.Stat(cachePath); err == nil {
			if opts.Fresh {
				e.Log.Detail("--replan: ignoring the saved reply and asking again")
			} else if data, err := os.ReadFile(cachePath); err == nil {
				var saved savedReply
				var probe map[string]json.RawMessage
				if json.Unmarshal(data, &saved) == nil && json.Unmarshal(data, &probe) == nil {
					if _, ok := probe["parsed"]; ok {
						reply, haveReply = string(saved.Parsed), true
					} else if saved.Text != nil {
						reply, haveReply = *saved.Text, true
					}
				}
				if haveReply {
					e.Log.OK("reusing the saved reply for this transcript, no API call")
				}
			}
		}
	}

	// Clips are taken from the answer as it is written. Each is checked,
	// framed and written to the plan while the model writes the next, so
	// the first clip is there long before the last.
	build := e.newPlanBuilder(ctx, sourcePath, source, lines, opts,
		planIDFor(opts, fingerprint, !haveReply))
	defer build.stop()
	var scanner clipScanner
	listen := &Listener{Text: func(piece string) {
		for _, raw := range scanner.feed(piece) {
			build.take(raw)
		}
	}}
	if haveReply {
		listen.text(reply)
	} else {
		// A model is going to be asked, so the search has parts that take
		// time, and the clock says how far it has come through them.
		budget := -1
		if opts.Local != nil {
			budget = opts.Local.Think
		}
		clock := newSearchClock(e.Log, opts.Model, opts.Local != nil, runeLen(prompt), opts.Count, budget)
		go clock.run()
		defer clock.stop()
		build.clock = clock
		listen = clock.listen(listen)
	}

	if !haveReply && opts.Local != nil {
		err := e.Log.Step("choosing and condensing on this machine", func() error {
			answer, err := e.CallLocal(ctx, *opts.Local, prompt, len(lines), opts.Count,
				opts.MaxTokens, opts.LogDir, listen)
			if err == nil {
				reply, how = answer.Content, answer
			}
			return err
		})
		if err != nil {
			return nil, build.failed(err)
		}
		build.answered()
		how.Content = ""
		saveReply(cachePath, reply, how)
		haveReply, fresh = true, true
	}

	if !haveReply {
		// These checks only matter when a request is actually going to be
		// sent. A cached run costs nothing.
		estimate, outEstimate, measured := estimateUsage(opts.LogDir, runeLen(prompt), "plan")
		facts := FactsFor(opts.Model)
		sourceNote := "estimated"
		if measured {
			sourceNote = "from past runs"
		}
		likely := 0.0
		costNote := ""
		if facts.Priced {
			likely = float64(float64(estimate)/1e6*facts.PriceIn) +
				float64(float64(outEstimate)/1e6*facts.PriceOut)
			costNote = ", roughly $" + fixed(likely, 2)
		}
		e.Log.Info("transcript: %s lines, %s characters", commas(len(lines)), commas(runeLen(prompt)))
		e.Log.Info("expecting about %s tokens in and %s out (%s)%s",
			commas(estimate), commas(outEstimate), sourceNote, costNote)
		if !measured {
			e.Log.Detail("most of the output is the model thinking before it answers, " +
				"which is billed as output")
		}

		// Three limits, for three different reasons. The model's context
		// window, its maximum output, and your wallet.
		if estimate+opts.MaxTokens > facts.Context {
			return nil, renderErr("the prompt is roughly %s tokens and the reply may be up to "+
				"%s, which together do not fit in %s's %s token context window.",
				commas(estimate), commas(opts.MaxTokens), opts.Model, commas(facts.Context))
		}
		if opts.MaxTokens > facts.MaxOutput {
			return nil, renderErr("--max-tokens %s is above %s's limit of %s output tokens.",
				commas(opts.MaxTokens), opts.Model, commas(facts.MaxOutput))
		}
		if facts.Priced && likely > opts.Budget {
			return nil, renderErr("this call would cost about $%s, over your $%s limit. "+
				"Raise it with --budget if that is fine.", fixed(likely, 2), fixed(opts.Budget, 2))
		}

		err := e.Log.Step("choosing and condensing", func() error {
			var err error
			reply, err = e.CallClaudeWithHeadroom(ctx, prompt, opts.Model, opts.MaxTokens,
				opts.LogDir, "plan", listen)
			return err
		})
		if err != nil {
			return nil, build.failed(err)
		}
		build.answered()
		saveReply(cachePath, reply, nil)
		fresh = true
	}

	// The whole answer is read once more when it is done. Every clip in it
	// has normally been taken by now. This catches what the scanner could
	// not see, an answer that only parses once it is salvaged, and says
	// what was wrong with the clips that were left out.
	data, note, err := ExtractJSONObject(reply, "clips")
	switch {
	case err != nil && build.count() > 0:
		// The clips taken as the answer arrived are whole and checked. What
		// is left of it cannot be read, and a repair would only be asked to
		// guess at it.
		e.Log.Warn("the end of the answer could not be read. The %d clip(s) before it are kept.",
			build.count())
		data = nil
	case err != nil && opts.Local != nil:
		return nil, build.failed(renderErr("%s The prompt and the answer are in %s.", err, opts.LogDir))
	case err != nil:
		e.Log.Warn("%s", err)
		if opts.LogDir != "" {
			e.Log.Warn("the raw reply is in %s", opts.LogDir)
		}
		var repaired string
		stepErr := e.Log.Step("repairing the reply", func() error {
			var err error
			repaired, err = e.RepairJSON(ctx, reply, opts.Model, opts.LogDir)
			return err
		})
		if stepErr != nil {
			return nil, build.failed(stepErr)
		}
		data, note, err = ExtractJSONObject(repaired, "clips")
		if err != nil {
			return nil, build.failed(renderErr("%s The prompt and every reply are in %s, so nothing is "+
				"lost. Inspect them, or write the plan by hand.", err, opts.LogDir))
		}
		note = strings.TrimLeft(note+", after a repair call", ", ")
	}
	if note != "" {
		e.Log.Warn("the reply needed salvaging: %s", note)
	}
	if data != nil {
		raw, problems, err := ValidatePlan(data, len(lines))
		if err != nil && build.count() == 0 {
			return nil, build.failed(err)
		}
		for i, problem := range problems {
			if i >= 6 {
				break
			}
			e.Log.Warn("in the model's answer: %s", problem)
		}
		build.rest(raw)
	}

	clips, entries, ids, err := build.finish()
	if err != nil {
		return nil, err
	}
	if build.clock != nil {
		build.clock.finish(len(clips) > 0)
	}
	e.Log.OK("%d candidate clip(s) proposed, %d kept", len(entries), len(clips))
	if len(clips) == 0 {
		return nil, renderErr("no usable clips came back from the model")
	}
	proposed := map[string][][2]float64{}
	for _, c := range clips {
		for _, seg := range c.Segments {
			proposed[c.ID] = append(proposed[c.ID], [2]float64{float64(seg.Start), float64(seg.End)})
		}
	}
	span := Window{0, source.Duration}
	if opts.Window != nil {
		span = *opts.Window
	}
	planID := e.recordPlan(opts, sourcePath, span, lines, prompt, fingerprint, fresh, entries, ids,
		proposed, build.planID)
	if err := build.settle(planID); err != nil {
		return nil, err
	}
	if opts.Local == nil {
		e.Log.Info("planning used %d API request(s) over %d HTTP attempt(s)",
			e.Calls.Requests, e.Calls.Attempts)
	}
	return &PlanFile{Source: filepath.Base(sourcePath), PlanID: planID, PlannedWith: build.stamp,
		Clips: clips}, nil
}

// saveReply keeps an answer so the same transcript is never planned twice.
// It is stored parsed when it parses, so the file is readable rather than one
// long escaped line.
func saveReply(cachePath, reply string, how *localAnswer) {
	if cachePath == "" {
		return
	}
	// How the model got to its answer goes beside it, where there is
	// anything to say: the tokens it read, thought and wrote, and how fast.
	extra := ""
	if how != nil {
		if told, err := json.MarshalIndent(how, "  ", "  "); err == nil {
			extra = ",\n  \"how\": " + string(told)
		}
	}
	var body []byte
	if json.Valid([]byte(reply)) {
		var indented bytes.Buffer
		if json.Indent(&indented, []byte(reply), "  ", "  ") == nil {
			body = []byte("{\n  \"parsed\": " + indented.String() + extra + "\n}")
		}
	}
	if body == nil {
		body, _ = MarshalPlan(map[string]string{"text": reply})
	}
	_ = os.WriteFile(cachePath, body, 0o644)
}
