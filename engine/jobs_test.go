package engine

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// searchProject makes a project of a forty second episode with the stand-in
// speech model and the stand-in language model.
func searchProject(t *testing.T, rec func(string) (Recognizer, error)) (*Project, *int32) {
	t.Helper()
	source := testEpisode(t, "40")
	SetTrainingDir(t.TempDir())
	var asked int32
	server := fakeModel(t, &asked)
	t.Cleanup(server.Close)
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	var heard int32
	if rec == nil {
		rec = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	}
	e.OpenRecognizer = rec
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Preset = "ultrafast"
	return NewProject(e, source, base), &asked
}

func TestASearchHearsItsWindowAndFindsItsClips(t *testing.T) {
	p, asked := searchProject(t, nil)
	var turns []string
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		turns = append(turns, step)
		// While it waits for its turn, the record says so.
		if rec := ReadSearch(p.Source); rec == nil || rec.Step != StepWaiting {
			t.Errorf("waiting for %s, the record says %+v", step, rec)
		}
		return ctx, func() {}, nil
	}
	plan, err := p.Search(context.Background(), PlanRequest{From: 5, To: 25, Count: 1, Min: 5}, turn)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if filepath.Base(plan) != "clips-5-25.json" || *asked != 1 {
		t.Errorf("plan %s, the model asked %d times", plan, *asked)
	}
	if strings.Join(turns, " ") != "hearing finding" {
		t.Errorf("turns asked for: %v", turns)
	}
	// Heard the window and nothing else.
	if parts := HeardParts(p.Source, p.Base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{5, 25} {
		t.Errorf("heard %v", parts)
	}
	if rec := ReadSearch(p.Source); rec != nil {
		t.Errorf("a search that found its clips keeps %+v", rec)
	}
	timings := ReadTimings(p.Source)
	if len(timings) != 1 {
		t.Fatalf("%d timings", len(timings))
	}
	var steps []string
	for _, s := range timings[0].Steps {
		steps = append(steps, s.Step)
	}
	if got := strings.Join(steps, " "); got != "waiting hearing waiting finding" {
		t.Errorf("timed %s", got)
	}
	if timings[0].Window != 20 || timings[0].Count != 1 || timings[0].Total <= 0 {
		t.Errorf("timings %+v", timings[0])
	}

	// The next window goes on from what was heard, and only needs the
	// model when the transcript is there already. The model gives the
	// moment the first search found, so the search keeps nothing, and
	// ends with a plan that says so rather than failing.
	turns = nil
	again, err := p.Search(context.Background(), PlanRequest{From: 5, To: 15, Count: 1, Min: 5}, turn)
	if err != nil {
		t.Fatal(err, p.LastError())
	}
	if _, clips, err := LoadClips(again); err != nil || len(clips) != 0 {
		t.Errorf("a window with nothing new: %v, %d clip(s)", err, len(clips))
	}
	if strings.Join(turns, " ") != "finding" {
		t.Errorf("a window already heard asked for %v", turns)
	}
}

// The whole episode is heard to its end, and the transcript is finished.
func TestASearchOfTheWholeEpisodeFinishesTheTranscript(t *testing.T) {
	p, _ := searchProject(t, nil)
	if _, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if _, done := Coverage(p.Source, p.Base.ASRModel); !done {
		t.Error("the transcript was not finished")
	}
}

// stops is a speech model that stops the search the moment it is given
// audio, the way the app closing does.
type stops struct {
	cancel context.CancelFunc
	fakeRecognizer
}

func (s stops) Recognize(samples []float32, rate int) []Token {
	s.cancel()
	return s.fakeRecognizer.Recognize(samples, rate)
}

// Stopped from outside while it hears, the record says it was hearing, so
// an app that starts reads it as cut off. What was heard stays.
func TestASearchStoppedWhileItHearsKeepsItsRecord(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var heard int32
	p, _ := searchProject(t, func(string) (Recognizer, error) { return stops{cancel, fakeRecognizer{&heard}}, nil })
	_, err := p.Search(ctx, PlanRequest{From: 0, To: 30, Count: 1, Min: 5}, nil)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("stopped: %v", err)
	}
	rec := ReadSearch(p.Source)
	if rec == nil || rec.Step != StepHearing || !rec.Interrupted() || rec.To != 30 {
		t.Fatalf("the record: %+v", rec)
	}
	if req := rec.Request(); req.To != 30 || req.Count != 1 || req.Min != 5 {
		t.Errorf("carried on as %+v", req)
	}
	if covered, _ := Coverage(p.Source, p.Base.ASRModel); covered <= 0 {
		t.Error("nothing heard was kept")
	}
	if len(ReadTimings(p.Source)) != 0 {
		t.Error("a search that did not finish was timed")
	}
}

// Stopped while it waits for the model's lane, the record says waiting.
func TestASearchStoppedWhileItWaitsKeepsItsRecord(t *testing.T) {
	p, asked := searchProject(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		if step == StepFinding {
			cancel()
			return nil, nil, ctx.Err()
		}
		return ctx, func() {}, nil
	}
	if _, err := p.Search(ctx, PlanRequest{To: 20, Count: 1, Min: 5}, turn); !errors.Is(err, ErrCancelled) {
		t.Fatalf("stopped: %v", err)
	}
	if rec := ReadSearch(p.Source); rec == nil || rec.Step != StepWaiting || !rec.Interrupted() {
		t.Errorf("the record: %+v", rec)
	}
	if *asked != 0 {
		t.Error("the model was asked")
	}
}

func TestASearchThatFailsSaysWhy(t *testing.T) {
	t.Run("while it hears", func(t *testing.T) {
		p, _ := searchProject(t, func(string) (Recognizer, error) {
			return nil, errors.New("the speech model is not there")
		})
		if _, err := p.Search(context.Background(), PlanRequest{To: 20, Count: 1, Min: 5}, nil); err == nil {
			t.Fatal("no error")
		}
		rec := ReadSearch(p.Source)
		if rec == nil || rec.Step != StepFailed || rec.Interrupted() ||
			!strings.Contains(rec.Error, "the speech model is not there") {
			t.Errorf("the record: %+v", rec)
		}
		if last := rec.Steps[len(rec.Steps)-1]; last.Step != StepFailed || last.To == nil {
			t.Errorf("the last step: %+v", last)
		}
	})
	t.Run("while it finds", func(t *testing.T) {
		p, _ := searchProject(t, nil)
		broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error": {"message": "out of memory"}}`, http.StatusInternalServerError)
		}))
		defer broken.Close()
		p.Base.LLMURL = broken.URL
		if _, err := p.Search(context.Background(), PlanRequest{To: 20, Count: 1, Min: 5}, nil); err == nil {
			t.Fatal("no error")
		}
		rec := ReadSearch(p.Source)
		if rec == nil || rec.Step != StepFailed || !strings.Contains(rec.Error, "out of memory") {
			t.Errorf("the record: %+v", rec)
		}
		// What was heard stays.
		if covered, _ := Coverage(p.Source, p.Base.ASRModel); covered < 19.9 {
			t.Errorf("heard to %.1f", covered)
		}
	})
}

// A render renders its clips one at a time and keeps a record of the ones
// it finished, so one that was cut off carries on with the rest.
func TestARenderCarriesOnWithTheClipsItHasNotFinished(t *testing.T) {
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := ReadPlan(plan)
	if err != nil || len(view.Clips) == 0 {
		t.Fatalf("plan: %v", err)
	}
	id := NewRenderID()
	// A render that was cut off after the first clip.
	cut := &JobRecord{ID: id, Kind: JobRender, Plan: plan, Done: []string{view.Clips[0].ID}, Step: StepRendering}
	var turns []string
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		turns = append(turns, step)
		return ctx, func() {}, nil
	}
	p.Base.Out = t.TempDir()
	shorts := func() int {
		found, _ := filepath.Glob(filepath.Join(p.Base.Out, "*.mp4"))
		return len(found)
	}
	if err := p.RenderJob(context.Background(), id, RenderRequest{Plan: plan}, cut, turn); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if got := shorts(); got != len(view.Clips)-1 {
		t.Errorf("%d shorts rendered, the %d clips less the one finished before", got, len(view.Clips))
	}
	if strings.Join(turns, " ") != StepRendering {
		t.Errorf("turns %v", turns)
	}
	if len(ReadJobs(p.Source)) != 0 {
		t.Error("a finished render keeps its record")
	}
	timings := ReadTimings(p.Source)
	if len(timings) != 2 || timings[1].Kind != JobRender {
		t.Errorf("timings %+v", timings)
	}

	// And one from the start renders every clip.
	if err := p.RenderJob(context.Background(), NewRenderID(), RenderRequest{Plan: plan}, nil, nil); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if n := ReadTimings(p.Source)[2].Clips; n != len(view.Clips) || shorts() != len(view.Clips) {
		t.Errorf("rendered %d of %d clips, %d shorts", n, len(view.Clips), shorts())
	}
}

func TestARecordThatIsNotOneIsNoRecord(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ep.mp4")
	logs := filepath.Join(WorkDir(source), "logs")
	good := []JobRecord{
		{ID: SearchID, Kind: JobSearch, To: 60, Count: 12, Min: 20, Max: 30, Step: StepHearing},
		{ID: "render-1", Kind: JobRender, Plan: filepath.Join(logs, "clips-0-60.json"), Step: StepRendering},
	}
	for _, r := range good {
		if err := WriteJob(source, r); err != nil {
			t.Fatal(err)
		}
	}
	bad := map[string]string{
		"render-2.json": `{"id": "render-2", "kind": "render", "plan": "/etc/clips.json", "step": "rendering"}`,
		"render-3.json": `{"id": "render-3", "kind": "render", "plan": "` + filepath.Join(logs, "notes.txt") + `", "step": "rendering"}`,
		"render-4.json": `{"id": "render-5", "kind": "render", "step": "rendering"}`,
		"render-6.json": `{"id": "render-6", "kind": "render", "step": "hearing"}`,
		// An id in capitals. Not Search.json: macOS does not tell that from
		// search.json, and it overwrote the good record on the build runner.
		"Other.json":     `{"id": "Other", "kind": "search", "step": "hearing"}`,
		"other.json":     `{"id": "other", "kind": "search", "step": "hearing"}`,
		"render-7.json":  `not json`,
		"render-8.json":  `{"id": "render-8", "kind": "upload", "step": "waiting"}`,
		"render-9.json":  `{"id": "render-9", "kind": "render", "step": "done"}`,
		"timings.jsonl":  `{"kind": "search"}`,
		"render-10.json": `{"id": "render-10", "kind": "render", "step": "rendering", "plan": "` + filepath.Join(logs, "clips.json") + `", "clips": ["` + strings.Repeat(`a", "`, 1001) + `"]}`,
	}
	for name, body := range bad {
		if err := os.WriteFile(filepath.Join(JobsDir(source), name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := ReadJobs(source)
	if len(got) != 2 || got[0].ID != SearchID || got[1].ID != "render-1" {
		t.Errorf("read %+v", got)
	}
	if err := WriteJob(source, JobRecord{ID: "../out", Kind: JobRender}); err == nil {
		t.Error("a record written outside the jobs folder")
	}
	if err := RemoveJob(source, "render-1"); err != nil || len(ReadJobs(source)) != 1 {
		t.Errorf("removed: %v, %d left", err, len(ReadJobs(source)))
	}
}

// A record is read from disk, where anything can be written. Whatever is
// there, reading it never fails the app, and what is read is sane.
func FuzzReadJob(f *testing.F) {
	f.Add([]byte(`{"id": "search", "kind": "search", "to": 60, "count": 12, "step": "hearing"}`))
	f.Add([]byte(`{"id": "render-1", "kind": "render", "plan": "x", "step": "failed", "error": "` + strings.Repeat("é", 600) + `"}`))
	f.Add([]byte(`{"id": "search", "kind": "search", "from": 1e400, "step": "finding"}`))
	dir := f.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	f.Fuzz(func(t *testing.T, body []byte) {
		path := filepath.Join(dir, "record.json")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		rec, ok := readJob(source, path)
		if !ok {
			return
		}
		if !jobID.MatchString(rec.ID) || len([]rune(rec.Error)) > reasonLimit || len(rec.Steps) > 100 {
			t.Errorf("read %+v", rec)
		}
		if rec.Kind == JobRender && filepath.Dir(rec.Plan) != filepath.Join(WorkDir(source), "logs") {
			t.Errorf("a render of %s", rec.Plan)
		}
	})
}

// takenBack is a speech model that ends the step it hears in the first time
// it is given audio, the way the app takes the hearing lane back for a
// search that finds.
type takenBack struct {
	take *context.CancelFunc
	fakeRecognizer
}

func (s takenBack) Recognize(samples []float32, rate int) []Token {
	if *s.take != nil {
		(*s.take)()
		*s.take = nil
	}
	return s.fakeRecognizer.Recognize(samples, rate)
}

// A search whose hearing lane is taken back waits for its turn again and
// carries on from what it heard.
func TestASearchCarriesOnHearingWhenItGetsTheLaneBack(t *testing.T) {
	var take context.CancelFunc
	var heard int32
	p, asked := searchProject(t, func(string) (Recognizer, error) {
		return takenBack{&take, fakeRecognizer{&heard}}, nil
	})
	var turns []string
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		turns = append(turns, step)
		stepCtx, cancel := context.WithCancel(ctx)
		if step == StepHearing && len(turns) == 1 {
			take = cancel
		}
		return stepCtx, cancel, nil
	}
	// The whole episode, which is heard to its end without a hold to stop
	// at first.
	if _, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, turn); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if got := strings.Join(turns, " "); got != "hearing hearing finding" {
		t.Errorf("turns %s", got)
	}
	if *asked != 1 {
		t.Errorf("the model asked %d times", *asked)
	}
	if _, done := Coverage(p.Source, p.Base.ASRModel); !done {
		t.Error("the transcript was not finished")
	}
}

// Called off by hand, a job's record says so, and keeps the window and the
// numbers it was asked for, so it can be carried on.
func TestAJobCalledOffByHandSaysSo(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ep.mp4")
	if err := WriteJob(source, JobRecord{ID: SearchID, Kind: JobSearch, To: 1800, Count: 12, Min: 20,
		Step: StepHearing, Steps: []StepTime{{Step: StepHearing}}}); err != nil {
		t.Fatal(err)
	}
	if err := StopJob(source, SearchID); err != nil {
		t.Fatal(err)
	}
	rec := ReadSearch(source)
	if rec == nil || rec.Step != StepStopped || !rec.Interrupted() || rec.To != 1800 || rec.Count != 12 {
		t.Fatalf("the record: %+v", rec)
	}
	if rec.Steps[0].To == nil {
		t.Error("the step it was in did not end")
	}
	if err := StopJob(source, "render-9"); err != nil {
		t.Errorf("a job with no record: %v", err)
	}
}

// counting is a speech model that counts the seconds of audio it is given.
type counting struct {
	seconds *float64
	fakeRecognizer
}

func (c counting) Recognize(samples []float32, rate int) []Token {
	*c.seconds += float64(len(samples)) / float64(rate)
	return c.fakeRecognizer.Recognize(samples, rate)
}

// A search of a later window hears its window, not the episode from the
// start or from where the transcript ends, and says how long is left until
// the end of what it hears, not until the end of the episode. Tim saw a
// search of the half hour after a half hour already heard say almost six
// minutes left.
func TestASearchOfALaterWindowHearsOnlyWhatIsNew(t *testing.T) {
	var heard int32
	seconds := 0.0
	p, _ := searchProject(t, func(string) (Recognizer, error) {
		return counting{&seconds, fakeRecognizer{&heard}}, nil
	})
	if _, err := p.Search(context.Background(), PlanRequest{To: 15, Count: 1, Min: 5}, nil); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	first := seconds
	if first > 15+hearingPad+0.1 {
		t.Errorf("the first window of 15 s heard %.1f s", first)
	}
	var lefts []float64
	p.engine.Log.SetSink(func(ev Event) {
		if ev.Kind == EventProgress && ev.Stage == "" && ev.Covered > 0 {
			lefts = append(lefts, ev.Remaining)
		}
	})
	seconds = 0
	if _, err := p.Search(context.Background(), PlanRequest{From: 20, To: 30, Count: 1, Min: 5}, nil); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if seconds > 10+2*hearingPad+0.1 {
		t.Errorf("a window from 20 to 30 heard %.1f s", seconds)
	}
	if gaps := Unheard(p.Source, p.Base.ASRModel, Window{0, 30}); len(gaps) != 1 || gaps[0] != (Window{15, 20}) {
		t.Errorf("unheard %v, want only what no search asked for", gaps)
	}
	t.Logf("heard %.1f s, then %.1f s, time left said %v", first, seconds, lefts)
}

// Captions switched off in the captions column render a short without any:
// no caption file is made for it, and the short is there.
func TestAShortRendersWithoutCaptionsWhenTheyAreOff(t *testing.T) {
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetCaptionStyle(plan, map[string]any{"text": false}); err != nil {
		t.Fatal(err)
	}
	p.Base.Out = t.TempDir()
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if shorts, _ := filepath.Glob(filepath.Join(p.Base.Out, "*.mp4")); len(shorts) == 0 {
		t.Fatal("no short")
	}
	if made, _ := filepath.Glob(filepath.Join(p.CaptionsDir(), "*.ass")); len(made) != 0 {
		t.Errorf("captions were made for a short without them: %v", made)
	}
}

// A panic while a job hears gives the lane of hearing back. The queue
// recovers the panic and fails the job, but a lane held for good would
// leave every later search waiting for a turn that never comes.
func TestAPanicWhileHearingGivesTheLaneBack(t *testing.T) {
	p, _ := searchProject(t, func(string) (Recognizer, error) {
		panic("the speech model broke")
	})
	held := 0
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		held++
		stepCtx, cancel := context.WithCancel(ctx)
		return stepCtx, func() { held--; cancel() }, nil
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, turn)
	}()
	if held != 0 {
		t.Errorf("%d lanes still held after the panic", held)
	}
}

// A render that fails leaves the short there was before it, and no half of
// the new one. ffmpeg here writes a few bytes where it was told to and
// fails, the way a render cut off halfway leaves a file behind.
func TestARenderThatFailsKeepsTheShortBefore(t *testing.T) {
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Base.Out = t.TempDir()
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	shorts, _ := filepath.Glob(filepath.Join(p.Base.Out, "*.mp4"))
	if len(shorts) != 1 {
		t.Fatalf("shorts %v", shorts)
	}
	before, err := os.ReadFile(shorts[0])
	if err != nil {
		t.Fatal(err)
	}

	// Every other call goes to the real ffmpeg. The render is the one call
	// that asks for +faststart.
	broken := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do last=$a; done\n" +
		"case \" $* \" in *\" +faststart \"*) printf half > \"$last\"; exit 1 ;; esac\n" +
		"exec '" + p.Engine().FFmpeg + "' \"$@\"\n"
	if err := os.WriteFile(broken, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p.Engine().FFmpeg = broken
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err == nil {
		t.Fatal("the render did not fail")
	}
	after, err := os.ReadFile(shorts[0])
	if err != nil {
		t.Fatalf("the short before is gone: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("the short before was overwritten")
	}
	if left, _ := filepath.Glob(filepath.Join(p.Base.Out, "*")); len(left) != 1 {
		t.Errorf("left in the folder: %v", left)
	}
}

// The plan a search says it wrote is the plan it wrote, whatever the
// window's edges. Run is handed the window as text to the millisecond and
// names the plan from that, so an edge a hair under a whole second was
// named one second apart on either side, and the search answered with a
// plan that was not there.
func TestAPlanIsWhereTheSearchSaysItIs(t *testing.T) {
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{From: 9.9996, To: 39.9996, Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if !isFile(plan) {
		made, _ := filepath.Glob(filepath.Join(p.LogsDir(), "clips-*.json"))
		t.Fatalf("the search answered %s, and wrote %v", filepath.Base(plan), made)
	}
}
