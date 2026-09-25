package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// fakeRecognizer hears a word every 0.6 seconds, so tests need no speech
// model.
type fakeRecognizer struct{ calls *int32 }

func (f fakeRecognizer) Recognize(samples []float32, rate int) []Token {
	atomic.AddInt32(f.calls, 1)
	length := float64(len(samples)) / float64(rate)
	var tokens []Token
	for at := 0.1; at+0.35 < length; at += 0.6 {
		tokens = append(tokens, Token{Text: " wort", Start: at, Duration: 0.3})
	}
	return tokens
}

func (fakeRecognizer) Close() {}

// fakeModel answers like llama-server with one clip made of the first line,
// streamed a few characters at a time the way the real one sends it.
func fakeModel(t *testing.T, asked *int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(asked, 1)
		plan := `{"clips": [{"slug": "erste", "title": "Erste", "reason": "Test", "keep": [[1, 1]]}]}`
		writeLocalStream(w, plan, 7)
	}))
}

func testEpisode(t *testing.T, seconds string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), "episode.mp4")
	// mpeg4 and aac are in every ffmpeg, the one we ship included, which
	// has no libx264. Test episodes made with libx264 could not be made on
	// the macOS runner once it tested against our ffmpeg.
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d="+seconds,
		"-f", "lavfi", "-i", "sine=f=220:d="+seconds,
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	return path
}

func TestProjectSteps(t *testing.T) {
	source := testEpisode(t, "40")
	// The records of every episode go in one folder. This test counts
	// them, so it gets a folder of its own.
	SetTrainingDir(t.TempDir())
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()

	rec := &recorder{}
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(rec.sink)
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }

	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Preset = "ultrafast"
	p := NewProject(e, source, base)
	ctx := context.Background()

	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("transcribe: %v %s", err, p.LastError())
	}
	if heard == 0 {
		t.Fatal("the recogniser was never asked")
	}
	if _, err := os.Stat(filepath.Join(p.LogsDir(), "words.json")); err != nil {
		t.Fatal(err)
	}

	// A window is cut from the whole transcript, never transcribed again.
	heard = 0
	plan, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if heard != 0 {
		t.Errorf("the window was transcribed again")
	}
	if filepath.Base(plan) != "clips-10-30.json" {
		t.Errorf("plan name %s", plan)
	}
	if asked != 1 {
		t.Errorf("model asked %d times", asked)
	}

	// The same window again reuses the plan, a new window makes its own,
	// even though another plan exists.
	if _, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 1}); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("an existing plan was not reused")
	}
	whole, err := p.Plan(ctx, PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("whole plan: %v %s", err, p.LastError())
	}
	if filepath.Base(whole) != "clips.json" || asked != 2 {
		t.Errorf("whole plan %s, model asked %d times", whole, asked)
	}
	if got := len(p.Plans()); got != 2 {
		t.Errorf("%d plans listed", got)
	}

	// Training records: one per new model answer, and a reused answer keeps
	// its id even when the plan file is made again.
	training := TrainingDir()
	countLines := func(name string) int {
		n := 0
		_ = readRecords(filepath.Join(training, name), func([]byte) { n++ })
		return n
	}
	if n := countLines("plans.jsonl"); n != 2 {
		t.Errorf("%d plan records", n)
	}
	firstPlan, _, _ := LoadClips(plan)
	firstID, _ := firstPlan.Raw["plan_id"].(string)
	if firstID == "" {
		t.Fatal("no plan_id in the plan file")
	}
	record, ok := loadPlanRecord(training, firstID)
	if !ok || record.Prompt == "" || record.System != SystemPrompt || len(record.Lines) == 0 ||
		record.Episode.Window != [2]float64{10, 30} || record.Planner.Kind != "local" ||
		len(record.Candidates) != 1 || record.Candidates[0].CID != "t10-01" {
		t.Errorf("plan record %+v", record)
	}
	if err := os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 1}); err != nil {
		t.Fatal(err)
	}
	again, _, _ := LoadClips(plan)
	if id, _ := again.Raw["plan_id"].(string); id != firstID || asked != 2 {
		t.Errorf("reused answer got id %s, model asked %d times", id, asked)
	}
	if n := countLines("plans.jsonl"); n != 2 {
		t.Errorf("reused answer recorded again, %d records", n)
	}

	if err := SetRejected(plan, "t10-01", true); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(plan, "t10-01", false); err != nil {
		t.Fatal(err)
	}
	var decisions []DecisionRecord
	_ = readRecords(filepath.Join(training, "decisions.jsonl"), func(line []byte) {
		var d DecisionRecord
		_ = json.Unmarshal(line, &d)
		decisions = append(decisions, d)
	})
	if last := decisions[len(decisions)-1]; last.Changes == nil || !last.Changes.Unchanged {
		t.Errorf("an untouched kept clip should count as unchanged: %+v", last.Changes)
	}

	if len(decisions) != 2 || decisions[0].Event != "rejected" || decisions[0].Final != nil ||
		decisions[1].Event != "kept" || decisions[1].Final == nil ||
		len(decisions[1].Final.Lines) == 0 || decisions[1].Final.Lines[0][0] != 1 {
		t.Errorf("decisions %+v", decisions)
	}

	st := Status(source, base.ASRModel)
	if !st.Transcribed || st.TranscriptStale || len(st.Plans) != 2 || st.Previews != 0 {
		t.Errorf("status %+v", st)
	}
	if st.Plans[1].From != 10 && st.Plans[0].From != 10 {
		t.Errorf("window not in summaries %+v", st.Plans)
	}
	tr, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	if d := tr.Duration(); d < 39.5 || d > 40.5 {
		t.Errorf("transcript lasts %v", d)
	}
	if peaks := tr.Peaks(0, 40, 100); len(peaks) != 100 || peaks[50] < -30 {
		t.Errorf("peaks %v", peaks)
	}
	if words := tr.WordsBetween(10, 12); len(words) < 2 || words[0].End <= 10 {
		t.Errorf("words %v", words)
	}

	if err := p.Render(ctx, RenderRequest{Plan: plan, Preview: true}); err != nil {
		t.Fatalf("render: %v %s", err, p.LastError())
	}
	matches, _ := filepath.Glob(filepath.Join(p.WorkDir(), "preview", "*.mp4"))
	if len(matches) != 1 {
		t.Errorf("preview files %v", matches)
	}

	view, err := ReadPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Clips) != 1 || view.Clips[0].Reason != "Test" || view.Clips[0].Preview == "" ||
		view.Summary.From != 10 || len(view.Clips[0].Words) == 0 {
		t.Errorf("plan view %+v", view)
	}

	frame, err := e.Still(ctx, source, 12.7, 25, 320)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(frame); err != nil || info.Size() == 0 || filepath.Base(frame) != "320-000012680.jpg" {
		t.Errorf("still %s %v", frame, err)
	}

	sawRenderProgress := false
	for _, ev := range rec.events {
		if ev.Kind == EventProgress && ev.Fraction > 0 {
			sawRenderProgress = true
		}
	}
	if !sawRenderProgress {
		t.Errorf("no progress with a fraction was reported")
	}
	// Starting the episode over asks the model again. The same answer to the
	// same prompt keeps its first record.
	if err := os.RemoveAll(p.LogsDir()); err != nil {
		t.Fatal(err)
	}
	if err := p.Transcribe(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Plan(ctx, PlanRequest{From: 10, To: 30, Count: 1}); err != nil {
		t.Fatalf("plan after reset: %v %s", err, p.LastError())
	}
	reset, _, _ := LoadClips(plan)
	if id, _ := reset.Raw["plan_id"].(string); id != firstID || asked != 3 {
		t.Errorf("after reset id %s, model asked %d times", id, asked)
	}
	if n := countLines("plans.jsonl"); n != 2 {
		t.Errorf("the same answer was recorded again, %d records", n)
	}
}

func TestProjectReportsErrors(t *testing.T) {
	source := testEpisode(t, "2")
	log := NewLog(&bytes.Buffer{}, false, false)
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return nil, errors.New("no model here")
	}
	p := NewProject(e, source, DefaultOptions())
	err := p.Transcribe(context.Background())
	if !errors.Is(err, ErrStepFailed) {
		t.Fatalf("got %v", err)
	}
	if want := "transcription failed: no model here"; len(p.LastError()) < len(want) ||
		p.LastError()[:len(want)] != want {
		t.Errorf("last error %q", p.LastError())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Transcribe(ctx); !errors.Is(err, ErrCancelled) {
		t.Errorf("cancelled run gave %v", err)
	}
}
