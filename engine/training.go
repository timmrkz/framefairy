package engine

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Training data is recorded in one folder for every episode, so the clip
// selection model can be improved later. It never leaves the machine, and
// it does not live with an episode: removing a video takes everything the
// app made of it, and the records outlive that. The files are append only,
// one JSON record per line, and the latest decision for a clip wins. Every
// plan record carries the key of the episode it came from, so one folder
// holds them all without mixing them up.

// PromptVersion changes whenever the prompt or the line rules change, so
// training data made with an older prompt can be told apart.
//
// Version 2 lets two runs follow each other directly, which cuts only the
// pause between them. Version 1 could only cut a pause by dropping a line.
const PromptVersion = 2

// TrainingSchema is the version of the record format.
const TrainingSchema = 1

// Decision events.
const (
	DecisionKept        = "kept"
	DecisionRejected    = "rejected"
	DecisionEdited      = "edited"
	DecisionPublished   = "published"
	DecisionUnpublished = "unpublished"
	// DecisionRendered is recorded when a clip is rendered, with its state
	// at that moment.
	DecisionRendered = "rendered"
	// DecisionViewed is recorded when a clip is played in the app. A clip
	// that was watched but never rendered, in a set where others were, is a
	// weak sign that it was not good enough.
	DecisionViewed = "viewed"
)

// CheckReasons sorts the reasons of a rejection and refuses any the format
// does not know. A reason that is not recorded is a signal lost, so callers
// check before they change anything.
func CheckReasons(reasons []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, r := range RejectReasons {
		allowed[r] = true
	}
	clean := []string{}
	for _, r := range reasons {
		if !allowed[r] {
			return nil, renderErr("unknown reason %s, use one of %s",
				r, strings.Join(RejectReasons, ", "))
		}
		clean = append(clean, r)
	}
	sort.Strings(clean)
	return clean, nil
}

// RejectReasons are the reasons a rejection may carry.
var RejectReasons = []string{"no-payoff", "weak-opening", "needs-context", "too-long",
	"too-short", "bad-cut", "boring", "other"}

// DefaultTrainingDir is where the records go unless something says
// otherwise: one folder beside the models, in the user's home.
func DefaultTrainingDir() string {
	if dir := os.Getenv("FRAMEFAIRY_TRAINING"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "training"
	}
	return filepath.Join(home, ".framefairy", "training")
}

// The one folder, set when a program starts and whenever the app's
// settings are saved: everything that records reads it, so there is one
// place and no path travels through the engine. Jobs record from their own
// goroutines while the window saves settings on another, so it is read and
// written behind a lock.
var (
	dirMu       sync.RWMutex
	trainingDir = DefaultTrainingDir()
)

// SetTrainingDir points the records at another folder. An empty path means
// the default one.
func SetTrainingDir(dir string) {
	if dir == "" {
		dir = DefaultTrainingDir()
	}
	dirMu.Lock()
	defer dirMu.Unlock()
	trainingDir = dir
}

// TrainingDir is the folder the records are written to and read from.
func TrainingDir() string {
	dirMu.RLock()
	defer dirMu.RUnlock()
	return trainingDir
}

// PlanRecord is one line of plans.jsonl.
type PlanRecord struct {
	Schema        int            `json:"schema"`
	PlanID        string         `json:"plan_id"`
	Created       string         `json:"created"`
	Engine        string         `json:"engine"`
	PromptVersion int            `json:"prompt_version"`
	Episode       RecordEpisode  `json:"episode"`
	Planner       RecordPlanner  `json:"planner"`
	Settings      RecordSettings `json:"settings"`
	ReplyKey      string         `json:"reply_key"`
	// ExampleKey identifies what the model was asked, whatever answered.
	// Plans with the same key are one training example.
	ExampleKey string            `json:"example_key"`
	System     string            `json:"system"`
	Prompt     string            `json:"prompt"`
	Lines      [][][2]float64    `json:"lines"`
	Candidates []RecordCandidate `json:"candidates"`
}

// RecordEpisode identifies what the model read.
type RecordEpisode struct {
	File string `json:"file"`
	// Key identifies the video by its content, so a renamed or re-added
	// file is still the same episode.
	Key        string     `json:"key"`
	Transcript string     `json:"transcript"`
	Window     [2]float64 `json:"window"`
}

// RecordPlanner says which model answered.
type RecordPlanner struct {
	Kind    string  `json:"kind"`
	Model   string  `json:"model"`
	Adapter *string `json:"adapter"`
}

// RecordSettings are the choices the prompt was built with.
type RecordSettings struct {
	Count int     `json:"count"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// RecordCandidate is one clip as the model proposed it.
type RecordCandidate struct {
	CID    string   `json:"cid"`
	Slug   string   `json:"slug"`
	Title  string   `json:"title"`
	Reason string   `json:"reason"`
	Keep   [][2]int `json:"keep"`
	// Segments are the clip as it was first made from the answer, before
	// anyone changed it.
	Segments [][2]float64 `json:"segments,omitempty"`
}

// DecisionRecord is one line of decisions.jsonl.
type DecisionRecord struct {
	Schema  int          `json:"schema"`
	Time    string       `json:"time"`
	PlanID  string       `json:"plan_id"`
	CID     string       `json:"cid"`
	Event   string       `json:"event"`
	Reasons []string     `json:"reasons"`
	Final   *FinalRecord `json:"final,omitempty"`
	// Changes compares the final clip with the proposal.
	Changes *Changes `json:"changes,omitempty"`
}

// Changes says how a person changed a proposed clip. Together they are the
// granular signal: an unchanged render is a full hit, moved edges and
// changed pauses say what the model got wrong.
type Changes struct {
	Unchanged      bool         `json:"unchanged"`
	StartShift     float64      `json:"start_shift"`
	EndShift       float64      `json:"end_shift"`
	LinesAdded     []int        `json:"lines_added"`
	LinesRemoved   []int        `json:"lines_removed"`
	PausesCut      [][2]float64 `json:"pauses_cut"`
	PausesRestored [][2]float64 `json:"pauses_restored"`
}

// FinalRecord is a clip as the person left it.
type FinalRecord struct {
	Lines    [][2]int     `json:"lines"`
	Segments [][2]float64 `json:"segments"`
}

var trainingMu sync.Mutex

func appendRecord(path string, record any) error {
	body, err := marshalNoEscape(record)
	if err != nil {
		return err
	}
	trainingMu.Lock()
	defer trainingMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(body, '\n'))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

// readRecords calls fn with every line of a JSON Lines file. Broken lines,
// for example from a crash mid-write, are skipped.
func readRecords(path string, fn func(line []byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if json.Valid(line) {
			fn(line)
		}
	}
	return scanner.Err()
}

func newPlanID(now time.Time) string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return "p-" + now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// ExampleKeyOf identifies what the model was asked: the transcript and the
// request built from it. The system prompt is left out, because it changes
// with the answer format while the question stays the same.
func ExampleKeyOf(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])[:16]
}

// EpisodeKey fingerprints a video by its size and its first and last
// megabyte. It is fast on a file of many gigabytes and survives renaming.
func EpisodeKey(source string) string {
	f, err := os.Open(source)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%d\n", info.Size())
	const part = 1 << 20
	buf := make([]byte, part)
	n, _ := f.ReadAt(buf, 0)
	h.Write(buf[:n])
	if info.Size() > part {
		n, _ = f.ReadAt(buf, max(info.Size()-part, part))
		h.Write(buf[:n])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// sameCandidates reports whether two answers proposed the same clips.
func sameCandidates(a, b []RecordCandidate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].CID != b[i].CID || fmt.Sprint(a[i].Keep) != fmt.Sprint(b[i].Keep) {
			return false
		}
	}
	return true
}

// wordsHash identifies the transcript words the lines were built from.
func wordsHash(lines []Line) string {
	h := sha256.New()
	for _, l := range lines {
		for _, w := range l.Cues {
			fmt.Fprintf(h, "%.3f %.3f %s\n", w.Start, w.End, w.Text)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func lineSpans(lines []Line) [][][2]float64 {
	out := make([][][2]float64, len(lines))
	for i, l := range lines {
		spans := make([][2]float64, len(l.Cues))
		for k, w := range l.Cues {
			spans[k] = [2]float64{roundTo(w.Start, 3), roundTo(w.End, 3)}
		}
		out[i] = spans
	}
	return out
}

// findPlanRecord returns the newest plan record with the given reply key.
func findPlanRecord(dir, replyKey string) (PlanRecord, bool) {
	var found PlanRecord
	ok := false
	_ = readRecords(filepath.Join(dir, "plans.jsonl"), func(line []byte) {
		var probe struct {
			PlanID   string `json:"plan_id"`
			ReplyKey string `json:"reply_key"`
		}
		if json.Unmarshal(line, &probe) == nil && probe.ReplyKey == replyKey {
			var full PlanRecord
			if json.Unmarshal(line, &full) == nil {
				found, ok = full, true
			}
		}
	})
	return found, ok
}

// findSamePlan returns an earlier record of the same answer to the same
// prompt, so doing the same thing twice is recorded once.
func findSamePlan(dir string, rec PlanRecord) (PlanRecord, bool) {
	var found PlanRecord
	ok := false
	_ = readRecords(filepath.Join(dir, "plans.jsonl"), func(line []byte) {
		if !strings.Contains(string(line), rec.ExampleKey) {
			return
		}
		var full PlanRecord
		if json.Unmarshal(line, &full) == nil && full.ExampleKey == rec.ExampleKey &&
			full.Planner.Model == rec.Planner.Model && sameCandidates(full.Candidates, rec.Candidates) {
			found, ok = full, true
		}
	})
	return found, ok
}

// loadPlanRecord returns the plan record with a plan id.
func loadPlanRecord(dir, planID string) (PlanRecord, bool) {
	var found PlanRecord
	ok := false
	_ = readRecords(filepath.Join(dir, "plans.jsonl"), func(line []byte) {
		if !strings.Contains(string(line), planID) {
			return
		}
		var full PlanRecord
		if json.Unmarshal(line, &full) == nil && full.PlanID == planID {
			found, ok = full, true
		}
	})
	return found, ok
}

// LinesForSegments maps a clip's final segments back to the numbered lines
// the model saw. A line counts as kept when at least half of its speaking
// time lies inside the segments. Consecutive kept lines form one range,
// unless a cut lies between them. Then they form two ranges that follow each
// other directly, which is how the answer format cuts a pause.
func LinesForSegments(lines [][][2]float64, segments [][2]float64) [][2]int {
	var kept []bool
	for _, words := range lines {
		total, inside := 0.0, 0.0
		for _, w := range words {
			total += w[1] - w[0]
			for _, s := range segments {
				lo, hi := max(w[0], s[0]), min(w[1], s[1])
				if hi > lo {
					inside += hi - lo
				}
			}
		}
		kept = append(kept, total > 0 && inside >= total/2)
	}
	// covered reports whether one segment spans a whole gap.
	covered := func(from, to float64) bool {
		for _, s := range segments {
			if s[0] <= from+0.001 && s[1] >= to-0.001 {
				return true
			}
		}
		return false
	}
	var out [][2]int
	for i, k := range kept {
		if !k {
			continue
		}
		number := i + 1
		if n := len(out); n > 0 && out[n-1][1] == number-1 {
			prev := lines[i-1]
			cur := lines[i]
			joined := true
			if len(prev) > 0 && len(cur) > 0 {
				gapFrom, gapTo := prev[len(prev)-1][1], cur[0][0]
				joined = gapTo <= gapFrom || covered(gapFrom, gapTo)
			}
			if joined {
				out[n-1][1] = number
				continue
			}
		}
		out = append(out, [2]int{number, number})
	}
	return out
}

// cutsOf lists the gaps between consecutive segments.
func cutsOf(segments [][2]float64) [][2]float64 {
	var out [][2]float64
	for i := 1; i < len(segments); i++ {
		if segments[i][0]-segments[i-1][1] > 0.02 {
			out = append(out, [2]float64{segments[i-1][1], segments[i][0]})
		}
	}
	return out
}

// cutsMissing are the cuts of a that no cut of b overlaps, limited to the
// stretch both clips cover.
func cutsMissing(a, b [][2]float64, from, to float64) [][2]float64 {
	out := [][2]float64{}
	for _, cut := range a {
		if cut[1] <= from || cut[0] >= to {
			continue
		}
		matched := false
		for _, other := range b {
			if min(cut[1], other[1])-max(cut[0], other[0]) > 0 {
				matched = true
			}
		}
		if !matched {
			out = append(out, cut)
		}
	}
	return out
}

func expandLines(ranges [][2]int) map[int]bool {
	out := map[int]bool{}
	for _, r := range ranges {
		for n := r[0]; n <= r[1]; n++ {
			out[n] = true
		}
	}
	return out
}

// DiffClip compares a final clip with its proposal.
func DiffClip(proposal, final [][2]float64, proposedLines, finalLines [][2]int) *Changes {
	c := &Changes{LinesAdded: []int{}, LinesRemoved: []int{}}
	if len(proposal) == 0 || len(final) == 0 {
		return c
	}
	c.StartShift = roundTo(final[0][0]-proposal[0][0], 3)
	c.EndShift = roundTo(final[len(final)-1][1]-proposal[len(proposal)-1][1], 3)
	from := max(final[0][0], proposal[0][0])
	to := min(final[len(final)-1][1], proposal[len(proposal)-1][1])
	before, after := cutsOf(proposal), cutsOf(final)
	c.PausesCut = cutsMissing(after, before, from, to)
	c.PausesRestored = cutsMissing(before, after, from, to)
	was, now := expandLines(proposedLines), expandLines(finalLines)
	for n := range now {
		if !was[n] {
			c.LinesAdded = append(c.LinesAdded, n)
		}
	}
	for n := range was {
		if !now[n] {
			c.LinesRemoved = append(c.LinesRemoved, n)
		}
	}
	sort.Ints(c.LinesAdded)
	sort.Ints(c.LinesRemoved)
	const tolerance = 0.05
	c.Unchanged = math.Abs(c.StartShift) < tolerance && math.Abs(c.EndShift) < tolerance &&
		len(c.PausesCut) == 0 && len(c.PausesRestored) == 0 && len(before) == len(after)
	return c
}

// RecordDecision appends a decision about a clip of a plan. Plans made
// before recording existed have no plan id, and nothing is recorded for
// them.
func RecordDecision(planPath, clipID, event string, reasons []string) error {
	plan, clips, err := LoadClips(planPath)
	if err != nil {
		return err
	}
	planID, _ := plan.Raw["plan_id"].(string)
	if planID == "" {
		return nil
	}
	var clip *Clip
	for i := range clips {
		if clips[i].ID == clipID {
			clip = &clips[i]
		}
	}
	if clip == nil {
		return renderErr("no clip %s in the plan", clipID)
	}
	clean, err := CheckReasons(reasons)
	if err != nil {
		return err
	}

	dir := TrainingDir()
	record := DecisionRecord{Schema: TrainingSchema, Time: time.Now().UTC().Format(time.RFC3339),
		PlanID: planID, CID: clipID, Event: event, Reasons: clean}
	switch event {
	case DecisionKept, DecisionEdited, DecisionPublished, DecisionRendered:
		final := &FinalRecord{}
		for _, s := range clip.Segments {
			final.Segments = append(final.Segments, [2]float64{roundTo(s.Start, 3), roundTo(s.End, 3)})
		}
		if rec, ok := loadPlanRecord(dir, planID); ok {
			final.Lines = LinesForSegments(rec.Lines, final.Segments)
			for _, cand := range rec.Candidates {
				if cand.CID == clipID && len(cand.Segments) > 0 {
					record.Changes = DiffClip(cand.Segments, final.Segments, cand.Keep, final.Lines)
				}
			}
		}
		record.Final = final
	case DecisionRejected, DecisionUnpublished, DecisionViewed:
	default:
		return renderErr("unknown decision %s", event)
	}
	return appendRecord(filepath.Join(dir, "decisions.jsonl"), record)
}

// recordPlan appends a plan record for a new model answer and returns its
// id. A reused answer, or the same answer to the same prompt given again,
// keeps the id it was first recorded with.
func (e *Engine) recordPlan(opts PlanOptions, sourcePath string, window Window, lines []Line,
	prompt, replyKey string, fresh bool, entries []PlanEntry, ids []string,
	segments map[string][][2]float64) string {
	if opts.LogDir == "" || !opts.Record {
		return ""
	}
	dir := TrainingDir()
	if !fresh {
		if rec, ok := findPlanRecord(dir, replyKey); ok {
			return rec.PlanID
		}
	}
	now := time.Now()
	kind, model := "api", opts.Model
	if opts.Local != nil {
		kind, model = "local", strings.TrimPrefix(opts.Model, "local:")
	}
	record := PlanRecord{
		Schema: TrainingSchema, PlanID: newPlanID(now), Created: now.UTC().Format(time.RFC3339),
		Engine: Version, PromptVersion: PromptVersion,
		Episode: RecordEpisode{File: filepath.Base(sourcePath), Key: EpisodeKey(sourcePath),
			Transcript: wordsHash(lines),
			Window:     [2]float64{roundTo(window.Start, 3), roundTo(window.End, 3)}},
		Planner:  RecordPlanner{Kind: kind, Model: model},
		Settings: RecordSettings{Count: opts.Count, Min: opts.MinLen, Max: opts.MaxLen},
		ReplyKey: replyKey, ExampleKey: ExampleKeyOf(prompt),
		System: SystemPrompt, Prompt: prompt, Lines: lineSpans(lines),
	}
	for i, entry := range entries {
		record.Candidates = append(record.Candidates, RecordCandidate{CID: ids[i], Slug: entry.Slug,
			Title: entry.Title, Reason: entry.Reason, Keep: entry.Keep, Segments: segments[ids[i]]})
	}
	if same, ok := findSamePlan(dir, record); ok {
		return same.PlanID
	}
	if err := appendRecord(filepath.Join(dir, "plans.jsonl"), record); err != nil {
		e.Log.Warn("could not record the plan for training: %s", err)
		return ""
	}
	return record.PlanID
}
