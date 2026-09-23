package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	if err := TrimClip(path, "01", 9.9, 13.4, tr, 0.1); err != nil {
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
	if len(clips[0].Words) != 4 {
		t.Errorf("words %v", clips[0].Words)
	}
	// Trimming the end back past the second piece drops it. "zwei" ends at
	// 11 and the next word starts at 12, so the edge is 11.1.
	if err := TrimClip(path, "01", 10, 11.2, tr, 0.1); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if segs := clips[0].Segments; len(segs) != 1 || segs[0].Start != 9.9 || segs[0].End != 11.1 {
		t.Errorf("after trimming in %+v", segs)
	}
	if err := TrimClip(path, "01", 12, 12.2, tr, 0.1); err == nil {
		t.Errorf("a clip under a second was accepted")
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
	captions := filepath.Join(dir, "ep.framefairy", "captions")
	_ = os.MkdirAll(logs, 0o755)
	_ = os.MkdirAll(captions, 0o755)
	words := []Cue{{10, 10.5, "Tipfeler"}, {10.6, 11, "zwei"}, {12, 12.4, "drei"}}
	tr := &Transcript{Words: append([]Cue(nil), words...)}
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.1}],` +
		` "words": [[10.0, 10.5, "Tipfeler"], [10.6, 11.0, "zwei"]]},` +
		` {"id": "02", "slug": "b", "segments": [{"start": 11.9, "end": 12.5}], "words": [[12.0, 12.4, "drei"]]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = os.WriteFile(path, []byte(plan), 0o644)
	_ = os.WriteFile(filepath.Join(captions, "01_a.srt"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(captions, "02_b.srt"), []byte("old"), 0o644)

	n, err := SetWordText(logs, 10.0004, "  Tippfehler ", tr)
	if err != nil || n != 1 {
		t.Fatalf("changed %d, %v", n, err)
	}
	_, clips, _ := LoadClips(path)
	if clips[0].Words[0].Text != "Tippfehler" || clips[1].Words[0].Text != "drei" {
		t.Errorf("clip words %v %v", clips[0].Words, clips[1].Words)
	}
	if exists(filepath.Join(captions, "01_a.srt")) || !exists(filepath.Join(captions, "02_b.srt")) {
		t.Errorf("only the changed clip's caption file should be gone")
	}
	fresh := append([]Cue(nil), words...)
	ApplyCorrections(fresh, LoadCorrections(logs))
	if fresh[0].Text != "Tippfehler" || fresh[1].Text != "zwei" {
		t.Errorf("corrections %v", fresh)
	}
	// A trim takes the words again from the transcript and keeps the fix.
	if err := TrimClip(path, "01", 10, 12.4, &Transcript{Words: fresh}, 0.1); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	if clips[0].Words[0].Text != "Tippfehler" {
		t.Errorf("trim lost the correction: %v", clips[0].Words)
	}
	if _, err := SetWordText(logs, 10, "   ", tr); err == nil {
		t.Errorf("an empty word was accepted")
	}
	if _, err := SetWordText(logs, 99, "x", tr); err == nil {
		t.Errorf("a word that does not exist was accepted")
	}
}

// A word the recogniser heard as one where two were said is corrected by
// typing both. The captions then break and highlight them one by one.
func TestACorrectedWordCanHoldSeveralWords(t *testing.T) {
	clip := Clip{
		Segments: []Segment{{Start: 10, End: 12}},
		Words:    []Cue{{10, 10.8, "Und da"}, {11, 11.4, "ja"}},
	}
	got := ClipWords(clip)
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
	// One word stays one word, whatever it holds.
	if same := SplitCorrected([]Cue{{1, 2, "Und"}}); len(same) != 1 || same[0].End != 2 {
		t.Errorf("a single word changed: %v", same)
	}
}

// A word added by hand reaches the caption file, and taking it away again
// reaches it too.
//
// Correcting a word to two words is how a word the recogniser never heard
// is added, and correcting it back to one is how it is taken away. Both
// are one correction on one word of the transcript: what the engine keeps
// is still the one moment, and the caption splits that moment between
// however many words now stand in it.
//
// The split was tested where it happens and nowhere after, so nothing
// said that it survived as far as a file. It has to: the srt beside a
// rendered short is the caption anyone would export, and a word that is
// burned into the picture and missing from the file is two answers to the
// same question.
func TestAnAddedWordReachesTheCaptionFile(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "ep.framefairy", "logs")
	captionDir := filepath.Join(dir, "ep.framefairy", "captions")
	_ = os.MkdirAll(logs, 0o755)
	_ = os.MkdirAll(captionDir, 0o755)

	words := []Cue{{10, 10.4, "mach"}, {10.5, 11.1, "was"}, {11.2, 11.6, "nicht"}}
	tr := &Transcript{Words: append([]Cue(nil), words...)}
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.8}],` +
		` "words": [[10.0, 10.4, "mach"], [10.5, 11.1, "was"], [11.2, 11.6, "nicht"]]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = os.WriteFile(path, []byte(plan), 0o644)

	// What the caption file says now, built the way a render builds it.
	saidInFile := func() []Cue {
		_, clips, err := LoadClips(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := resolveCues(clips[0], captionDir, false, 40); err != nil {
			t.Fatal(err)
		}
		srt := filepath.Join(captionDir, clips[0].Basename()+".srt")
		if !exists(srt) {
			t.Fatal("no caption file was written")
		}
		loaded, err := LoadCaptions(srt)
		if err != nil {
			t.Fatal(err)
		}
		var out []Cue
		for _, c := range loaded {
			out = append(out, c.Words...)
		}
		return out
	}

	before := saidInFile()
	if len(before) != 3 {
		t.Fatalf("three words were said: %v", before)
	}

	// A word is added: what was heard as one word was two.
	if n, err := SetWordText(logs, 10.5, "was wo", tr); err != nil || n != 1 {
		t.Fatalf("adding a word changed %d clips, %v", n, err)
	}
	added := saidInFile()
	if len(added) != 4 {
		t.Fatalf("the added word never reached the file: %v", added)
	}
	if added[1].Text != "was" || added[2].Text != "wo" {
		t.Errorf("the file has the wrong words: %v", added)
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
	// And the text of the caption itself, which is what the srt carries.
	if !strings.Contains(captionCues(mustCaptions(t, captionDir, path))[0].Text, "was wo") {
		t.Errorf("the caption text is missing the added word")
	}

	// Taking it away again is the same correction the other way.
	if n, err := SetWordText(logs, 10.5, "was", tr); err != nil || n != 1 {
		t.Fatalf("taking the word away changed %d clips, %v", n, err)
	}
	gone := saidInFile()
	if len(gone) != 3 {
		t.Fatalf("the word was not taken away again: %v", gone)
	}
	if gone[1].Text != "was" || gone[1].Start != before[1].Start || gone[1].End != before[1].End {
		t.Errorf("the word did not go back to what it was: %v", gone)
	}
}

// The captions of the one clip in a test plan, freshly built.
func mustCaptions(t *testing.T, captionDir, planPath string) []Caption {
	t.Helper()
	_, clips, err := LoadClips(planPath)
	if err != nil {
		t.Fatal(err)
	}
	cues, err := resolveCues(clips[0], captionDir, false, 40)
	if err != nil {
		t.Fatal(err)
	}
	return cues
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
