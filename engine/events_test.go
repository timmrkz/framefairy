package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) sink(ev Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) kinds() []string {
	out := make([]string, len(r.events))
	for i, ev := range r.events {
		out[i] = string(ev.Kind)
	}
	return out
}

func TestEventsFollowTheLog(t *testing.T) {
	var out bytes.Buffer
	log := NewLog(&out, true, false)
	rec := &recorder{}
	log.SetSink(rec.sink)

	log.Info("hello %d", 1)
	log.Warn("careful")
	log.Detail("hidden in the terminal")
	err := log.Step("transcribing the audio", func() error {
		log.ProgressOf("transcribing", 0.5, 12)
		log.Detail("a line in between keeps the progress open")
		log.ClearProgress()
		log.ClearProgress()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = log.Step("planning", func() error { return errors.New("no model") })

	want := "info warn detail step-start progress detail idle step-done step-start step-failed"
	if got := strings.Join(rec.kinds(), " "); got != want {
		t.Fatalf("kinds\n got %s\nwant %s", got, want)
	}
	if rec.events[0].Text != "hello 1" {
		t.Errorf("text %q", rec.events[0].Text)
	}
	if strings.Contains(rec.events[1].Text, "\033") {
		t.Errorf("warn text carries colour codes: %q", rec.events[1].Text)
	}
	if strings.Contains(out.String(), "hidden") {
		t.Errorf("detail reached the terminal without --verbose")
	}
	progress := rec.events[4]
	if progress.Stage != "transcribing the audio" || progress.Fraction != 0.5 ||
		progress.Remaining != 12 {
		t.Errorf("progress event %+v", progress)
	}
	if rec.events[0].Stage != "" || rec.events[0].Fraction != Unknown {
		t.Errorf("event outside a step %+v", rec.events[0])
	}
	if failed := rec.events[9]; failed.Text != "no model" || failed.Stage != "planning" {
		t.Errorf("failed step %+v", failed)
	}
}

func TestProgressIsClamped(t *testing.T) {
	log := NewLog(&bytes.Buffer{}, false, false)
	rec := &recorder{}
	log.SetSink(rec.sink)
	log.ProgressOf("rendering", 1.7, Unknown)
	log.ProgressOf("rendering", Unknown, Unknown)
	if rec.events[0].Fraction != 1 || rec.events[0].Remaining != Unknown {
		t.Errorf("clamped %+v", rec.events[0])
	}
	if rec.events[1].Fraction != Unknown {
		t.Errorf("unknown %+v", rec.events[1])
	}
}

func TestJSONLines(t *testing.T) {
	var buf bytes.Buffer
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(JSONLines(&buf))
	log.OK("done <3 & more")
	log.SetSink(nil)
	log.Info("not recorded")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("lines %q", lines)
	}
	var ev Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != EventOK || ev.Text != "done <3 & more" || ev.Time.IsZero() {
		t.Errorf("decoded %+v", ev)
	}
	if !strings.Contains(lines[0], "<3 & more") {
		t.Errorf("html escaped: %s", lines[0])
	}
}

func TestSilencesAndPeaks(t *testing.T) {
	frames := []float32{-20, -60, -60, -60, -20, -60, -20}
	tr := &Transcript{Frames: frames, Start: 5, Floor: -40}
	got := tr.Silences(0.02)
	if len(got) != 1 || math.Abs(got[0].Start-5.01) > 1e-9 || math.Abs(got[0].End-5.04) > 1e-9 {
		t.Errorf("silences %v", got)
	}
	peaks := tr.Peaks(5, 5.07, 2)
	if peaks[0] != -20 || peaks[1] != -20 {
		t.Errorf("peaks %v", peaks)
	}
	if outside := tr.Peaks(0, 1, 2); outside[0] != -90 {
		t.Errorf("outside %v", outside)
	}
}

// Loudness is read every hundredth of a second and no finer, so asking for
// more parts than that gives parts that share a reading. Five buckets
// inside one reading are five copies of it, and the app, drawing a line
// between its buckets, cannot tell that from five readings that agree: the
// line comes out flat where the sound was rising, which is what made the
// clip timeline look like a display with too few pixels. The answer says
// how fine the measurement was by being that long.
func TestPeaksNeverPromiseMoreThanWasMeasured(t *testing.T) {
	tr := &Transcript{Frames: make([]float32, 500), Floor: -40}
	for i := range tr.Frames {
		tr.Frames[i] = float32(-60 + i%20)
	}

	// A tenth of a second holds ten readings, however many are asked for.
	if got := tr.Peaks(0, 0.1, 4000); len(got) != 10 {
		t.Errorf("a tenth of a second came back in %d parts, want 10", len(got))
	}
	// Asking for fewer than were measured is answered exactly.
	if got := tr.Peaks(0, 5, 100); len(got) != 100 {
		t.Errorf("100 parts of five seconds came back as %d", len(got))
	}
	// One reading is one part, never none.
	if got := tr.Peaks(0, 0.004, 50); len(got) != 1 {
		t.Errorf("four milliseconds came back in %d parts, want 1", len(got))
	}
	// And every part still carries the loudest reading in it.
	fine := tr.Peaks(0, 0.1, 4000)
	for i, level := range fine {
		if level != tr.Frames[i] {
			t.Errorf("part %d reads %v, the reading is %v", i, level, tr.Frames[i])
		}
	}
}

func TestSetRejectedKeepsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clips.json")
	original := `{
  "source": "episode.mp4",
  "custom": {"z": 1, "a": 2.50},
  "clips": [
    {"id": "01", "slug": "erste", "note": "mine", "segments": [{"start": 1.0, "end": 2.5, "crop_x": "center"}]},
    {"id": "02", "segments": [{"start": 3, "end": 4}]}
  ]
}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(path, "02", true); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if clips[0].Rejected || !clips[1].Rejected {
		t.Errorf("rejected flags %v %v", clips[0].Rejected, clips[1].Rejected)
	}
	body, _ := os.ReadFile(path)
	text := string(body)
	for _, want := range []string{`"z": 1`, `"a": 2.50`, `"note": "mine"`, `"start": 1.0`, `"revision": 1`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s in\n%s", want, text)
		}
	}
	if strings.Index(text, `"source"`) > strings.Index(text, `"clips"`) ||
		strings.Index(text, `"z"`) > strings.Index(text, `"a"`) {
		t.Errorf("key order changed\n%s", text)
	}
	if err := SetRejected(path, "02", false); err != nil {
		t.Fatal(err)
	}
	body, _ = os.ReadFile(path)
	if strings.Contains(string(body), "rejected") || !strings.Contains(string(body), `"revision": 2`) {
		t.Errorf("after keeping again\n%s", body)
	}
	if err := SetRejected(path, "07", true); err == nil {
		t.Errorf("unknown clip accepted")
	}
}

func TestLinesForSegments(t *testing.T) {
	lines := [][][2]float64{
		{{0, 1}, {1.2, 2}},
		{{3, 4}},
		{{5, 6}, {6.1, 7}},
		{{8, 9}},
	}
	// Line 1 fully in, line 2 fully in, line 3 only a quarter in, line 4
	// more than half in.
	got := LinesForSegments(lines, [][2]float64{{0, 4.5}, {5, 5.5}, {8.3, 9}})
	want := [][2]int{{1, 2}, {4, 4}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v want %v", got, want)
	}
}

// Letting go of an episode leaves nothing of it behind. The training
// records are not in there: they live in one folder of their own, so they
// are not touched by this and outlive every episode.
func TestDeleteWorkLeavesNothingButTheEpisode(t *testing.T) {
	ownTrainingDir(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := WorkDir(source)
	for _, name := range []string{"logs/words.json", "out/01.mp4", "cache/stills/1-000000.jpg"} {
		path := filepath.Join(work, name)
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, []byte("x"), 0o644)
	}
	records := filepath.Join(TrainingDir(), "plans.jsonl")
	_ = os.MkdirAll(filepath.Dir(records), 0o755)
	if err := os.WriteFile(records, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteWork(source); err != nil {
		t.Fatal(err)
	}
	if exists(work) {
		t.Errorf("the work folder is still there")
	}
	if !exists(source) {
		t.Errorf("the episode itself was deleted")
	}
	if !exists(records) {
		t.Errorf("the training records were deleted with the episode")
	}
	if err := DeleteWork(filepath.Join(dir, "missing.mp4")); err != nil {
		t.Errorf("missing work folder: %v", err)
	}
}

func TestDiffClip(t *testing.T) {
	proposal := [][2]float64{{10, 20}, {25, 30}}
	same := DiffClip(proposal, [][2]float64{{10.01, 20}, {25, 29.98}}, [][2]int{{1, 4}}, [][2]int{{1, 4}})
	if !same.Unchanged {
		t.Errorf("tiny differences count as a change: %+v", same)
	}
	trimmed := DiffClip(proposal, [][2]float64{{12, 20}, {25, 33}}, [][2]int{{1, 4}}, [][2]int{{2, 5}})
	if trimmed.Unchanged || trimmed.StartShift != 2 || trimmed.EndShift != 3 ||
		fmt.Sprint(trimmed.LinesAdded) != "[5]" || fmt.Sprint(trimmed.LinesRemoved) != "[1]" {
		t.Errorf("trimmed %+v", trimmed)
	}
	restored := DiffClip(proposal, [][2]float64{{10, 30}}, [][2]int{{1, 4}}, [][2]int{{1, 4}})
	if restored.Unchanged || len(restored.PausesRestored) != 1 || len(restored.PausesCut) != 0 {
		t.Errorf("restored %+v", restored)
	}
	cut := DiffClip(proposal, [][2]float64{{10, 14}, {16, 20}, {25, 30}}, [][2]int{{1, 4}}, [][2]int{{1, 4}})
	if cut.Unchanged || len(cut.PausesCut) != 1 || cut.PausesCut[0] != [2]float64{14, 16} {
		t.Errorf("cut %+v", cut)
	}

	// A cut the model made and a person moved. The clip's own edges did not
	// change and there are still as many cuts as before, so nothing above
	// notices it, and the export reads Unchanged to decide whether a render
	// is a full hit. Recorded as unchanged, this teaches the model that the
	// cut it proposed was the one that was wanted, which is the opposite of
	// what happened.
	moved := DiffClip(proposal, [][2]float64{{10, 21.5}, {24, 30}}, [][2]int{{1, 4}}, [][2]int{{1, 4}})
	if moved.Unchanged {
		t.Errorf("a moved cut counts as an unchanged clip: %+v", moved)
	}
	if len(moved.PausesMoved) != 1 {
		t.Fatalf("moved %+v", moved)
	}
	if moved.PausesMoved[0].Was != [2]float64{20, 25} || moved.PausesMoved[0].Now != [2]float64{21.5, 24} {
		t.Errorf("the move was recorded as %+v", moved.PausesMoved[0])
	}
	// It is one cut moved, not one taken away and another made.
	if len(moved.PausesCut) != 0 || len(moved.PausesRestored) != 0 {
		t.Errorf("a moved cut was counted twice: %+v", moved)
	}

	// A cut nudged by a frame or two is the same cut, and a person who
	// leaves it alone should not look like one who corrected it.
	nudged := DiffClip(proposal, [][2]float64{{10, 20.02}, {25.01, 30}}, [][2]int{{1, 4}}, [][2]int{{1, 4}})
	if !nudged.Unchanged || len(nudged.PausesMoved) != 0 {
		t.Errorf("a cut within the tolerance counts as moved: %+v", nudged)
	}
}

func TestTrimClipSnapsToWords(t *testing.T) {
	words := []Cue{{10, 10.5, "eins"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}, {13, 13.5, "vier"}, {14, 14.5, "fünf"}}
	tr := &Transcript{Words: words}
	path := filepath.Join(t.TempDir(), "clips.json")
	plan := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x", "segments": [` +
		`{"start": 10.5, "end": 11.1, "crop_x": 120}, {"start": 11.9, "end": 12.5, "crop_x": 300}]}]}`
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	// Start moved earlier onto "eins", end moved later onto "vier".
	if err := TrimClip(path, "01", 9.9, 13.4, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	segs := clips[0].Segments
	if len(segs) != 2 || segs[0].Start != 9.9 || segs[1].End != 13.6 || *segs[0].CropX != 120 || *segs[1].CropX != 300 {
		t.Errorf("segments %+v", segs)
	}
	if said := Said(clips[0], words); len(said) != 4 {
		t.Errorf("words %v", said)
	}
	// Trimming the end back past the second piece drops it. "zwei" ends at
	// 11 and the next word starts at 12, so the edge is 11.1.
	if err := TrimClip(path, "01", 10, 11.2, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if segs := clips[0].Segments; len(segs) != 1 || segs[0].Start != 9.9 || segs[0].End != 11.1 {
		t.Errorf("after trimming in %+v", segs)
	}
	if err := TrimClip(path, "01", 12, 12.2, tr, 0.1, ToWords); err == nil {
		t.Errorf("a clip under a second was accepted")
	}
}

// A clip edge dragged by itself stays where it was put, a frame at a time,
// the same as the edge of a cut. It may stop inside a word, which is what
// taking a breath off the end of a clip needs. The checks still hold.
func TestTrimClipToFramesStaysWhereItWasPut(t *testing.T) {
	words := []Cue{{10, 10.5, "eins"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}, {13, 13.5, "vier"}}
	tr := &Transcript{Words: words}
	path := filepath.Join(t.TempDir(), "clips.json")
	plan := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x", "segments": [` +
		`{"start": 10.5, "end": 11.1, "crop_x": 120}, {"start": 11.9, "end": 12.5, "crop_x": 300}]}]}`
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	// 10.24 is inside "eins" and 12.84 is in the pause before "vier", so
	// snapping would move both.
	if err := TrimClip(path, "01", 10.24, 12.84, tr, 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	segs := clips[0].Segments
	if len(segs) != 2 || segs[0].Start != 10.24 || segs[1].End != 12.84 {
		t.Errorf("segments %+v", segs)
	}
	if err := TrimClip(path, "01", 12, 12.5, tr, 0.1, ToFrames); err == nil {
		t.Errorf("a clip under a second was accepted")
	}
	if err := TrimClip(path, "01", -3, 12.5, tr, 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if clips[0].Segments[0].Start != 0 {
		t.Errorf("a clip starts before the episode: %+v", clips[0].Segments)
	}
}

func TestTouchingRunsCutOnlyThePause(t *testing.T) {
	lines := []Line{
		{Cues: []Cue{{10, 11, "a"}}},
		{Cues: []Cue{{12, 13, "b"}}},
		{Cues: []Cue{{13.1, 14, "c"}}},
	}
	segs := SegmentsFromRanges([][2]int{{1, 1}, {2, 3}}, lines, 0.1, nil)
	if len(segs) != 2 || segs[0].End != 11.1 || segs[1].Start != 11.9 {
		t.Errorf("pause not cut %+v", segs)
	}
	// Lines 2 and 3 are 0.1 s apart, less than the room around a cut, so
	// touching runs there join into one piece.
	segs = SegmentsFromRanges([][2]int{{1, 2}, {3, 3}}, lines, 0.1, nil)
	if len(segs) != 1 || segs[0].Start != 9.9 || segs[0].End != 14.1 {
		t.Errorf("short pause %+v", segs)
	}
	spans := lineSpans(lines)
	got := LinesForSegments(spans, [][2]float64{{9.9, 11.1}, {11.9, 14.1}})
	if fmt.Sprint(got) != "[[1 1] [2 3]]" {
		t.Errorf("lines %v", got)
	}
	got = LinesForSegments(spans, [][2]float64{{9.9, 14.1}})
	if fmt.Sprint(got) != "[[1 3]]" {
		t.Errorf("joined lines %v", got)
	}
}

func TestWordCorrections(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "ep.framefairy", "logs")
	_ = os.MkdirAll(logs, 0o755)
	words := []Cue{{10, 10.5, "Tipfeler"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}}
	tr := fromStored(append([]Cue(nil), words...), nil, 0, 0, nil)
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.1}]},` +
		` {"id": "02", "slug": "b", "segments": [{"start": 11.9, "end": 12.5}]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = os.WriteFile(path, []byte(plan), 0o644)

	if err := SetWordText(logs, 10.0004, "  Tippfehler ", tr); err != nil {
		t.Fatal(err)
	}
	_, clips, _ := LoadClips(path)
	one, two := Said(clips[0], tr.Words), Said(clips[1], tr.Words)
	if len(one) != 2 || one[0].Text != "Tippfehler" || len(two) != 1 || two[0].Text != "drei" {
		t.Errorf("clip words %v %v", one, two)
	}
	// A transcript read again from what the recogniser heard takes the
	// correction from corrections.json.
	again := fromStored(append([]Cue(nil), words...), nil, 0, 0, nil)
	again.Correct(LoadCorrections(logs))
	if again.Words[0].Text != "Tippfehler" {
		t.Errorf("the correction was not kept: %v", again.Words)
	}
	// A trim keeps the fix, because the clip reads its words from the
	// transcript.
	if err := TrimClip(path, "01", 10, 12.4, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if said := Said(clips[0], tr.Words); len(said) == 0 || said[0].Text != "Tippfehler" {
		t.Errorf("trim lost the correction: %v", said)
	}
	if err := SetWordText(logs, 99, "x", tr); err == nil {
		t.Errorf("a word that does not exist was accepted")
	}
}

// A word corrected to nothing is removed. The clip no longer says it and
// the captions no longer show it, and it is still the word heard there, so
// correcting it again brings it back, and so does reading the transcript
// again from what the recogniser heard.
func TestAWordCorrectedToNothingIsRemoved(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "ep.framefairy", "logs")
	_ = os.MkdirAll(logs, 0o755)
	words := []Cue{{10, 10.5, "äh"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}}
	tr := fromStored(append([]Cue(nil), words...), nil, 0, 0, nil)
	clip := Clip{Segments: []Segment{{Start: 9.9, End: 12.5}}}
	said := func() string {
		var out []string
		for _, w := range Said(clip, tr.Words) {
			out = append(out, w.Text)
		}
		return strings.Join(out, " ")
	}

	if err := SetWordText(logs, 10.2, "  ", tr); err != nil {
		t.Fatal(err)
	}
	if got := said(); got != "zwei drei" {
		t.Errorf("after removing, the clip says %q", got)
	}
	if w, ok := tr.HeardAt(10.2); !ok || w.Text != "" {
		t.Errorf("the word heard there is %+v, %v", w, ok)
	}
	again := fromStored(append([]Cue(nil), words...), nil, 0, 0, nil)
	again.Correct(LoadCorrections(logs))
	if len(again.Words) != 2 || again.Words[0].Text != "zwei" {
		t.Errorf("the removal was not kept: %v", again.Words)
	}
	if err := SetWordText(logs, 10.2, "ja", tr); err != nil {
		t.Fatal(err)
	}
	if got := said(); got != "ja zwei drei" {
		t.Errorf("after correcting it again, the clip says %q", got)
	}
}

// Removing a word takes the word out of its caption and nothing else. The
// time it was said in is no pause, so the caption goes on over it and is
// laid out the way it was. It ended at the word before, and the words
// after went on to the next caption, with nothing on screen between.
func TestRemovingAWordLeavesItsCaptionWhole(t *testing.T) {
	heard := []Cue{{60, 60.4, "wurde"}, {60.45, 61, "irgendein"}, {61.05, 61.4, "Typ"},
		{61.45, 62.1, "auf"}, {62.15, 62.4, "dem"}, {62.45, 63, "Schulhof."},
		{64, 64.4, "Und"}, {64.45, 64.9, "dann"}}
	clip := Clip{Segments: []Segment{{Start: 59.9, End: 65}}}
	style := ResolveStyle(nil)
	laid := func(tr *Transcript) string {
		var out []string
		for _, c := range ClipCaptions(clip, tr, style) {
			var words []string
			for _, w := range c.Words {
				words = append(words, w.Text)
			}
			out = append(out, fmt.Sprintf("[%.2f-%.2f %s]", c.Start, c.End, strings.Join(words, " ")))
		}
		return strings.Join(out, " ")
	}
	tr := fromStored(append([]Cue(nil), heard...), nil, 0, 0, nil)
	before := laid(tr)
	logs := filepath.Join(t.TempDir(), "logs")
	_ = os.MkdirAll(logs, 0o755)
	if err := SetWordText(logs, 61.7, "", tr); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(before, "Typ auf dem", "Typ dem", 1)
	if got := laid(tr); got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
}

// A removed word typed back in beside its neighbour is the word put back
// where it was heard, the same as an undo would put it: its own time, so
// it is lit while it is said, and no correction left on either word.
func TestAWordTypedBackGoesWhereItWasHeard(t *testing.T) {
	heard := []Cue{{10, 10.4, "weil"}, {10.5, 10.8, "das"}, {11, 11.4, "ein"}, {11.5, 12, "echtes"}}
	for _, c := range []struct {
		name   string
		remove []float64
		at     float64
		text   string
		want   string
		fixes  map[string]string
	}{
		{"after", []float64{11.2}, 10.6, "das ein", "weil@10 das@10.5 ein@11 echtes@11.5", map[string]string{}},
		{"before", []float64{11.2}, 11.6, "ein echtes", "weil@10 das@10.5 ein@11 echtes@11.5", map[string]string{}},
		{"typed otherwise", []float64{11.2}, 10.6, "das eine", "weil@10 das@10.5 eine@11 echtes@11.5", map[string]string{"11000": "eine"}},
		{"more than were removed", []float64{10.6, 11.2}, 10.2, "weil das ein so", "weil@10 das@10.5 ein@11 so@11.2 echtes@11.5", map[string]string{"11000": "ein so"}},
		{"fewer than were removed", []float64{10.6, 11.2}, 11.6, "ein echtes", "weil@10 ein@11 echtes@11.5", map[string]string{"10500": ""}},
		{"nothing removed beside it", []float64{11.2}, 10.2, "weil es", "weil@10 es@10.3 das@10.5 echtes@11.5", map[string]string{"10000": "weil es", "11000": ""}},
	} {
		t.Run(c.name, func(t *testing.T) {
			logs := filepath.Join(t.TempDir(), "logs")
			_ = os.MkdirAll(logs, 0o755)
			tr := fromStored(append([]Cue(nil), heard...), nil, 0, 0, nil)
			for _, at := range c.remove {
				if err := SetWordText(logs, at, "", tr); err != nil {
					t.Fatal(err)
				}
			}
			if err := SetWordText(logs, c.at, c.text, tr); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, w := range tr.Words {
				got = append(got, fmt.Sprintf("%s@%g", w.Text, math.Round(w.Start*10)/10))
			}
			if strings.Join(got, " ") != c.want {
				t.Errorf("words\n got %s\nwant %s", strings.Join(got, " "), c.want)
			}
			if fixes := LoadCorrections(logs); !maps.Equal(fixes, c.fixes) {
				t.Errorf("corrections %v, want %v", fixes, c.fixes)
			}
		})
	}
}

// A word typed into the one before it, which is how a removed word is put
// back, makes a heard word of two words. Each says which of the two it is,
// so the caption box can correct or remove one without the other.
func TestTheWordsOfOneCorrectionSayWhichTheyAre(t *testing.T) {
	tr := fromStored([]Cue{{10, 10.4, "weil"}, {10.5, 10.8, "das"}, {11, 11.4, "ein"}}, nil, 0, 0, nil)
	tr.Correct(map[string]string{wordKey(10): "weil es", wordKey(11): ""})
	clip := Clip{Segments: []Segment{{Start: 9.9, End: 11.5}}}
	view := captionsView(Plan{}, clip, tr, nil)
	var got []string
	for _, c := range view.Captions {
		for _, line := range c.Lines {
			for _, w := range line.Words {
				part := "-"
				if w.Part != nil {
					part = itoa(*w.Part)
				}
				said := -1.0
				if w.Said != nil {
					said = *w.Said
				}
				got = append(got, fmt.Sprintf("%s@%g:%s/%s", w.Text, said, part, w.Whole))
			}
		}
	}
	want := "weil@10:0/weil es es@10:1/weil es das@10.5:-/das"
	if strings.Join(got, " ") != want {
		t.Errorf("words\n got %s\nwant %s", strings.Join(got, " "), want)
	}
}

// A word the recogniser heard as one where two were said is corrected by
// typing both. The words then hold them one by one, so the captions break
// and highlight them one by one.
func TestACorrectedWordCanHoldSeveralWords(t *testing.T) {
	clip := Clip{Segments: []Segment{{Start: 10, End: 12}}}
	tr := fromStored([]Cue{{10, 10.8, "Und"}, {11, 11.4, "ja"}}, nil, 0, 0, nil)
	tr.Correct(map[string]string{wordKey(10): "Und da"})
	got := ClipWords(clip, tr.Words)
	if len(got) != 3 {
		t.Fatalf("words %v", got)
	}
	if got[0].Text != "Und" || got[1].Text != "da" || got[2].Text != "ja" {
		t.Errorf("texts %v", got)
	}
	if got[0].Start != 0 || math.Abs(got[1].End-0.8) > 0.001 || got[2].Start != 1 {
		t.Errorf("the split has to stay inside the word: %v", got)
	}
	if got[0].End != got[1].Start {
		t.Errorf("no gap between the parts: %v", got)
	}
	// Longer words take longer to say, so they get more of the span.
	if got[0].End-got[0].Start <= got[1].End-got[1].Start {
		t.Errorf("the share is not by length: %v", got)
	}
	// The word heard stays one word, the one a correction is kept against.
	if heard, ok := tr.HeardAt(10.5); !ok || heard.Text != "Und da" || heard.End != 10.8 {
		t.Errorf("the word heard is %+v", heard)
	}
	// One word stays one word, whatever it holds.
	if same := splitWord(Cue{1, 2, "Und"}); len(same) != 1 || same[0].End != 2 {
		t.Errorf("a single word changed: %v", same)
	}
}

// A word added by hand reaches the captions, and taking it away again
// reaches them too.
//
// Correcting a word to two words is how a word the recogniser never heard
// is added, and correcting it back to one is how it is taken away. Both
// are one correction on one word of the transcript: what the engine keeps
// is still the one moment, and the words split that moment between
// however many words now stand in it.
func TestAnAddedWordReachesTheCaptions(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "ep.framefairy", "logs")
	_ = os.MkdirAll(logs, 0o755)

	words := []Cue{{10, 10.4, "mach"}, {10.5, 11.1, "was"}, {11.2, 11.6, "nicht"}}
	tr := fromStored(append([]Cue(nil), words...), nil, 0, 0, nil)
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.8}]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = os.WriteFile(path, []byte(plan), 0o644)

	// The captions of the clip, built the way a render builds them.
	captions := func() []Caption {
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		return Captions(clips[0], tr.Words, 40, nil)
	}
	said := func() []Cue {
		var out []Cue
		for _, c := range captions() {
			out = append(out, c.Words...)
		}
		return out
	}

	before := said()
	if len(before) != 3 {
		t.Fatalf("three words were said: %v", before)
	}

	// A word is added: what was heard as one word was two.
	if err := SetWordText(logs, 10.5, "was wo", tr); err != nil {
		t.Fatalf("adding a word: %v", err)
	}
	added := said()
	if len(added) != 4 {
		t.Fatalf("the added word never reached the captions: %v", added)
	}
	if added[1].Text != "was" || added[2].Text != "wo" {
		t.Errorf("the captions have the wrong words: %v", added)
	}
	// Both halves carry a moment, inside the one the word was heard at and
	// with no gap between them, or the highlight would stall in the middle.
	if added[1].Start != before[1].Start || added[2].End != before[1].End {
		t.Errorf("the halves left the word they came from: %v", added)
	}
	if added[1].End != added[2].Start {
		t.Errorf("a gap opened between the halves: %v", added)
	}
	if !(added[1].End > added[1].Start && added[2].End > added[2].Start) {
		t.Errorf("a half lasts no time at all: %v", added)
	}
	// And the text of the caption itself.
	if !strings.Contains(captions()[0].Text, "was wo") {
		t.Errorf("the caption text is missing the added word")
	}

	// Taking it away again is the same correction the other way.
	if err := SetWordText(logs, 10.5, "was", tr); err != nil {
		t.Fatalf("taking the word away: %v", err)
	}
	gone := said()
	if len(gone) != 3 {
		t.Fatalf("the word was not taken away again: %v", gone)
	}
	if gone[1].Text != "was" || gone[1].Start != before[1].Start || gone[1].End != before[1].End {
		t.Errorf("the word did not go back to what it was: %v", gone)
	}
}

func TestCropByHand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clips.json")
	plan := `{"clips": [{"id": "01", "segments": [` +
		`{"start": 0, "end": 5, "crop_x": 400}, {"start": 5, "end": 9, "crop_x": 1200}, ` +
		`{"start": 10, "end": 14, "crop_x": 400}, {"start": 14, "end": 18}]}]}`
	if err := os.WriteFile(path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	crops := func() string {
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		out := ""
		for _, s := range clips[0].Segments {
			x := "centre"
			if s.CropX != nil {
				x = fmt.Sprint(*s.CropX)
			}
			if s.Moved {
				x += "*"
			}
			out += x + " "
		}
		return out
	}
	// Moving the first angle moves both of its pieces, not the others.
	if err := SetCrop(path, "01", 2, 501); err != nil {
		t.Fatal(err)
	}
	if got := crops(); got != "500* 1200 500* centre " {
		t.Errorf("after moving %q", got)
	}
	// Moving it again keeps the automatic placement from the first time.
	if err := SetCrop(path, "01", 11, 600); err != nil {
		t.Fatal(err)
	}
	// A centred piece can be moved and reset too.
	if err := SetCrop(path, "01", 16, 100); err != nil {
		t.Fatal(err)
	}
	if got := crops(); got != "600* 1200 600* 100* " {
		t.Errorf("after moving again %q", got)
	}
	if err := ResetCrop(path, "01", 3); err != nil {
		t.Fatal(err)
	}
	if err := ResetCrop(path, "01", 15); err != nil {
		t.Fatal(err)
	}
	if got := crops(); got != "400 1200 400 centre " {
		t.Errorf("after resetting %q", got)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "crop_x_auto") {
		t.Errorf("reset left the automatic value behind\n%s", body)
	}
	if err := SetCrop(path, "01", 2, -4); err == nil {
		t.Errorf("a negative crop was accepted")
	}
}

// While a search holds the progress line, nothing that runs inside it ends
// the line: not a step that finishes, not ffmpeg clearing its own progress
// when it is done with a clip. Each did, the app took the search's fill
// away, and it came back with the search's next report.
func TestAHeldLineIsOnlyEverLetGoByWhoHoldsIt(t *testing.T) {
	log := NewLog(&bytes.Buffer{}, false, false)
	rec := &recorder{}
	log.SetSink(rec.sink)
	log.HoldProgress(true)
	log.ProgressFound("Finding clips", 0.4, 30, 2)
	_ = log.Step("fitting 2 clip(s) to the length", func() error {
		log.ProgressOf("finding camera switches", 0.5, 1)
		log.ClearProgress()
		return nil
	})
	log.ClearProgress()
	if got, want := strings.Join(rec.kinds(), " "), "progress step-start step-done"; got != want {
		t.Fatalf("kinds while held\n got %s\nwant %s", got, want)
	}
	log.HoldProgress(false)
	log.ClearProgress()
	if got := rec.kinds(); got[len(got)-1] != "idle" {
		t.Errorf("let go, the line was not cleared: %v", got)
	}
	// Outside a hold a step that ends still ends its progress.
	rec.events = nil
	_ = log.Step("rendering", func() error {
		log.ProgressOf("rendering", 0.5, 1)
		return nil
	})
	if got, want := strings.Join(rec.kinds(), " "), "step-start progress idle step-done"; got != want {
		t.Errorf("kinds\n got %s\nwant %s", got, want)
	}
}
