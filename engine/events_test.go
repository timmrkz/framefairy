package engine

import (
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
	"framefairy/engine/runlog"
	"framefairy/engine/timed"
)

type recorder struct {
	mu     sync.Mutex
	events []runlog.Event
}

func (r *recorder) sink(ev runlog.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
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
	fsys := afero.NewMemMapFs()
	path := filepath.Join(memDir(t, fsys), "clips.json")
	original := `{
  "source": "episode.mp4",
  "custom": {"z": 1, "a": 2.50},
  "clips": [
    {"id": "01", "slug": "erste", "note": "mine", "segments": [{"start": 1.0, "end": 2.5, "crop_x": "center"}]},
    {"id": "02", "segments": [{"start": 3, "end": 4}]}
  ]
}`
	if err := afero.WriteFile(fsys, path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(fsys, nil, path, "02", true); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if clips[0].Rejected || !clips[1].Rejected {
		t.Errorf("rejected flags %v %v", clips[0].Rejected, clips[1].Rejected)
	}
	body, _ := afero.ReadFile(fsys, path)
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
	if err := SetRejected(fsys, nil, path, "02", false); err != nil {
		t.Fatal(err)
	}
	body, _ = afero.ReadFile(fsys, path)
	if strings.Contains(string(body), "rejected") || !strings.Contains(string(body), `"revision": 2`) {
		t.Errorf("after keeping again\n%s", body)
	}
	if err := SetRejected(fsys, nil, path, "07", true); err == nil {
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
	fsys, source := afero.NewMemMapFs(), "/home/tim/ep.mp4"
	dir := NewRecords(fsys, "", "").TrainingDir()
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := WorkDir(source)
	for _, name := range []string{"logs/words.json", "out/01.mp4", "cache/stills/1-000000.jpg"} {
		_ = afero.WriteFile(fsys, filepath.Join(work, name), []byte("x"), 0o644)
	}
	records := filepath.Join(dir, "plans.jsonl")
	if err := afero.WriteFile(fsys, records, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteWork(fsys, source); err != nil {
		t.Fatal(err)
	}
	if fileio.Exists(fsys, work) {
		t.Errorf("the work folder is still there")
	}
	if !fileio.Exists(fsys, source) {
		t.Errorf("the episode itself was deleted")
	}
	if !fileio.Exists(fsys, records) {
		t.Errorf("the training records were deleted with the episode")
	}
	if err := DeleteWork(fsys, "/home/tim/missing.mp4"); err != nil {
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
	fsys := afero.NewMemMapFs()
	words := []timed.Cue{{Start: 10, End: 10.5, Text: "eins"}, {Start: 10.6, End: 11, Text: "zwei"}, {Start: 12, End: 12.4, Text: "drei"}, {Start: 13, End: 13.5, Text: "vier"}, {Start: 14, End: 14.5, Text: "fünf"}}
	tr := &Transcript{Words: words}
	path := filepath.Join(memDir(t, fsys), "clips.json")
	plan := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x", "segments": [` +
		`{"start": 10.5, "end": 11.1, "crop_x": 120}, {"start": 11.9, "end": 12.5, "crop_x": 120}]}]}`
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	// Start moved earlier onto "eins", end moved later onto "vier".
	if err := TrimClip(fsys, path, "01", 9.9, 13.4, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	segs := clips[0].Segments
	if len(segs) != 2 || segs[0].Start != 9.9 || segs[1].End != 13.6 || *segs[0].CropX != 120 || *segs[1].CropX != 120 {
		t.Errorf("segments %+v", segs)
	}
	if said := Said(clips[0], words); len(said) != 4 {
		t.Errorf("words %v", said)
	}
	// Trimming the end back past the second piece drops it. "zwei" ends at
	// 11 and the next word starts at 12, so the edge is 11.1.
	if err := TrimClip(fsys, path, "01", 10, 11.2, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(fsys, path)
	if segs := clips[0].Segments; len(segs) != 1 || segs[0].Start != 9.9 || segs[0].End != 11.1 {
		t.Errorf("after trimming in %+v", segs)
	}
	if err := TrimClip(fsys, path, "01", 12, 12.2, tr, 0.1, ToWords); err == nil {
		t.Errorf("a clip under a second was accepted")
	}
}

// A clip edge dragged by itself stays where it was put, a frame at a time,
// the same as the edge of a cut. It may stop inside a word, which is what
// taking a breath off the end of a clip needs. The checks still hold.
func TestTrimClipToFramesStaysWhereItWasPut(t *testing.T) {
	fsys := afero.NewMemMapFs()
	words := []timed.Cue{{Start: 10, End: 10.5, Text: "eins"}, {Start: 10.6, End: 11, Text: "zwei"}, {Start: 12, End: 12.4, Text: "drei"}, {Start: 13, End: 13.5, Text: "vier"}}
	tr := &Transcript{Words: words}
	path := filepath.Join(memDir(t, fsys), "clips.json")
	plan := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x", "segments": [` +
		`{"start": 10.5, "end": 11.1, "crop_x": 120}, {"start": 11.9, "end": 12.5, "crop_x": 120}]}]}`
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	// 10.24 is inside "eins" and 12.84 is in the pause before "vier", so
	// snapping would move both.
	if err := TrimClip(fsys, path, "01", 10.24, 12.84, tr, 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	segs := clips[0].Segments
	if len(segs) != 2 || segs[0].Start != 10.24 || segs[1].End != 12.84 {
		t.Errorf("segments %+v", segs)
	}
	if err := TrimClip(fsys, path, "01", 12, 12.5, tr, 0.1, ToFrames); err == nil {
		t.Errorf("a clip under a second was accepted")
	}
	if err := TrimClip(fsys, path, "01", -3, 12.5, tr, 0.1, ToFrames); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(fsys, path)
	if clips[0].Segments[0].Start != 0 {
		t.Errorf("a clip starts before the episode: %+v", clips[0].Segments)
	}
}

func TestTouchingRunsCutOnlyThePause(t *testing.T) {
	lines := []Line{
		{Cues: []timed.Cue{{Start: 10, End: 11, Text: "a"}}},
		{Cues: []timed.Cue{{Start: 12, End: 13, Text: "b"}}},
		{Cues: []timed.Cue{{Start: 13.1, End: 14, Text: "c"}}},
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
	fsys := afero.NewMemMapFs()
	dir := memDir(t, fsys)
	logs := filepath.Join(dir, "ep.framefairy", "logs")
	_ = fsys.MkdirAll(logs, 0o755)
	words := []timed.Cue{{Start: 10, End: 10.5, Text: "Tipfeler"}, {Start: 10.6, End: 11, Text: "zwei"}, {Start: 12, End: 12.4, Text: "drei"}}
	tr := fromStored(append([]timed.Cue(nil), words...), nil, 0, 0, nil)
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.1}]},` +
		` {"id": "02", "slug": "b", "segments": [{"start": 11.9, "end": 12.5}]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = afero.WriteFile(fsys, path, []byte(plan), 0o644)

	if err := SetWordText(fsys, path, "01", 10.0004, "  Tippfehler ", tr); err != nil {
		t.Fatal(err)
	}
	_, clips, _ := LoadClips(fsys, path)
	one, two := Said(clips[0], tr.Of(clips[0]).Words), Said(clips[1], tr.Of(clips[1]).Words)
	if len(one) != 2 || one[0].Text != "Tippfehler" || len(two) != 1 || two[0].Text != "drei" {
		t.Errorf("clip words %v %v", one, two)
	}
	// The video says what was heard, and a clip over the same word that
	// was not corrected in it reads it as heard.
	if tr.Words[0].Text != "Tipfeler" {
		t.Errorf("the video was corrected: %v", tr.Words)
	}
	// A trim keeps the fix, because the clip reads its words from the
	// transcript with its corrections.
	if err := TrimClip(fsys, path, "01", 10, 12.4, tr, 0.1, ToWords); err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(fsys, path)
	if said := Said(clips[0], tr.Of(clips[0]).Words); len(said) == 0 || said[0].Text != "Tippfehler" {
		t.Errorf("trim lost the correction: %v", said)
	}
	if err := SetWordText(fsys, path, "01", 99, "x", tr); err == nil {
		t.Errorf("a word that does not exist was accepted")
	}
}

// A word corrected to nothing is removed from its clip. The clip's
// captions no longer show it, and the video still says it, so a search
// and every other clip still have it. Correcting it again brings it back.
func TestAWordCorrectedToNothingIsRemovedFromItsClip(t *testing.T) {
	fsys := afero.NewMemMapFs()
	_, path := oneClipPlan(t, fsys, 9.9, 12.5)
	words := []timed.Cue{{Start: 10, End: 10.5, Text: "äh"}, {Start: 10.6, End: 11, Text: "zwei"}, {Start: 12, End: 12.4, Text: "drei"}}
	tr := fromStored(append([]timed.Cue(nil), words...), nil, 0, 0, nil)

	if err := SetWordText(fsys, path, "01", 10.2, "  ", tr); err != nil {
		t.Fatal(err)
	}
	if got := shownWords(t, fsys, path, tr); got != "zwei drei" {
		t.Errorf("after removing, the clip shows %q", got)
	}
	if len(tr.Words) != 3 {
		t.Errorf("the video says %v", tr.Words)
	}
	if w, ok := tr.HeardAt(10.2); !ok || w.Text != "äh" {
		t.Errorf("the word heard there is %+v, %v", w, ok)
	}
	if c := clipByID(t, fsys, path, "01"); len(c.Corrections) != 0 {
		t.Errorf("removing a word corrected it: %v", c.Corrections)
	}
	if err := SetWordText(fsys, path, "01", 10.2, "ja", tr); err != nil {
		t.Fatal(err)
	}
	if got := shownWords(t, fsys, path, tr); got != "ja zwei drei" {
		t.Errorf("after correcting it again, the clip shows %q", got)
	}
	if c := clipByID(t, fsys, path, "01"); len(c.RemovedWords) != 0 {
		t.Errorf("the clip still removes %v", c.RemovedWords)
	}
}

// Removing a word takes the word out of its caption and nothing else. The
// time it was said in is no pause, so the caption goes on over it and is
// laid out the way it was. It ended at the word before, and the words
// after went on to the next caption, with nothing on screen between.
func TestRemovingAWordLeavesItsCaptionWhole(t *testing.T) {
	fsys := afero.NewMemMapFs()
	heard := []timed.Cue{{Start: 60, End: 60.4, Text: "wurde"}, {Start: 60.45, End: 61, Text: "irgendein"}, {Start: 61.05, End: 61.4, Text: "Typ"},
		{Start: 61.45, End: 62.1, Text: "auf"}, {Start: 62.15, End: 62.4, Text: "dem"}, {Start: 62.45, End: 63, Text: "Schulhof."},
		{Start: 64, End: 64.4, Text: "Und"}, {Start: 64.45, End: 64.9, Text: "dann"}}
	_, path := oneClipPlan(t, fsys, 59.9, 65)
	style := ResolveStyle(nil)
	laid := func(tr *Transcript) string {
		var out []string
		for _, c := range ClipCaptions(clipByID(t, fsys, path, "01"), tr, style) {
			var words []string
			for _, w := range c.Words {
				words = append(words, w.Text)
			}
			out = append(out, fmt.Sprintf("[%.2f-%.2f %s]", c.Start, c.End, strings.Join(words, " ")))
		}
		return strings.Join(out, " ")
	}
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	before := laid(tr)
	if err := SetWordText(fsys, path, "01", 61.7, "", tr); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(before, "Typ auf dem", "Typ dem", 1)
	if got := laid(tr); got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
}

// laidOf is a clip's captions as Captions lays them out, at 33 characters a
// line, each with when it is up and what it reads.
func laidOf(t *testing.T, fsys afero.Fs, path string, tr *Transcript) string {
	t.Helper()
	clip := clipByID(t, fsys, path, "01")
	var out []string
	for _, c := range Captions(clip, tr.clipCaptionWords(clip), 33, nil) {
		out = append(out, fmt.Sprintf("[%.2f-%.2f %s]", c.Start, c.End, c.Text))
	}
	return strings.Join(out, " ")
}

// The room a removed word took on its line stays taken, so the first word
// of the next caption is not pulled up into it. Found by a sequence in
// frontend/preview/walks: removing "auf" brought "irgendein" up from the
// caption after.
func TestRemovingAWordPullsNoWordUp(t *testing.T) {
	fsys := afero.NewMemMapFs()
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}}
	_, path := oneClipPlan(t, fsys, 59.9, 63)
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	before := laidOf(t, fsys, path, tr)
	if !strings.Contains(before, "Schulhof]") {
		t.Fatalf("the test needs a break after Schulhof: %s", before)
	}
	if err := SetWordText(fsys, path, "01", 60.6, "", tr); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(before, "mich auf dem", "mich dem", 1)
	if got := laidOf(t, fsys, path, tr); got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
	// A caption whose first word is removed appears with the word after,
	// the way it does when its left edge is dragged in over the word. Tim
	// asked for the two to end the same. The speech runs on, so the
	// caption before stays up until it appears, see
	// TestARemovedWordLeavesNoGapWhileTheSpeechRunsOn.
	if err := SetWordText(fsys, path, "01", 61.5, "", tr); err != nil {
		t.Fatal(err)
	}
	want = strings.Replace(want, "1.55 als mich dem Schulhof] [1.55-2.70 irgendein Typ]", "2.05 als mich dem Schulhof] [2.05-2.70 Typ]", 1)
	if got := laidOf(t, fsys, path, tr); got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
}

// A word removed at the edge of a caption leaves nothing blank while the
// speech runs on: the caption before stays up until the next one appears,
// whichever of the two the word was in. Where a pause comes after the word,
// the caption draws in to the words it shows and the pause stays. Found by
// Tim, who removed a word and saw the caption blocks on the clip timeline
// come apart with the sound running on under the gap, #597.
func TestARemovedWordLeavesNoGapWhileTheSpeechRunsOn(t *testing.T) {
	// The clip starts at 59.9, so 61.45 in the episode is 1.55 on its
	// clock. "irgendein" runs straight on from "Schulhof", or after a
	// pause of 0.8 s.
	runsOn := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}}
	paused := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 62.2, End: 62.65, Text: "irgendein"},
		{Start: 62.7, End: 62.95, Text: "Typ"}}
	for _, c := range []struct {
		name   string
		heard  []timed.Cue
		remove float64
		want   string
	}{
		{"nothing removed", runsOn, 0,
			"[0.00-1.55 als mich auf dem Schulhof] [1.55-2.70 irgendein Typ]"},
		{"the first word of the caption after", runsOn, 61.6,
			"[0.00-2.05 als mich auf dem Schulhof] [2.05-2.70 Typ]"},
		{"the last word of the caption before, longer than its hold", runsOn, 61.2,
			"[0.00-1.55 als mich auf dem] [1.55-2.70 irgendein Typ]"},
		{"nothing removed, a pause between", paused, 0,
			"[0.00-1.90 als mich auf dem Schulhof] [2.30-3.45 irgendein Typ]"},
		{"the first word of the caption after a pause", paused, 62.4,
			"[0.00-1.90 als mich auf dem Schulhof] [2.80-3.45 Typ]"},
		{"the last word of the caption before a pause", paused, 61.2,
			"[0.00-1.40 als mich auf dem] [2.30-3.45 irgendein Typ]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			_, path := oneClipPlan(t, fsys, 59.9, 64)
			tr := fromStored(append([]timed.Cue(nil), c.heard...), nil, 0, 0, nil)
			if c.remove > 0 {
				if err := SetWordText(fsys, path, "01", c.remove, "", tr); err != nil {
					t.Fatal(err)
				}
			}
			if got := laidOf(t, fsys, path, tr); got != c.want {
				t.Errorf("captions\n got %s\nwant %s", got, c.want)
			}
		})
	}
}

// A start dragged in over the first word of its caption moves that
// caption only, in speech that runs on too: the caption before keeps its
// end and the room the hand cleared stays empty, as Tim asked in #189.
// The same word removed in the caption box leaves no gap, #597.
func TestADraggedStartLeavesTheGapARemovedWordDoesNot(t *testing.T) {
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}, {Start: 62.25, End: 62.7, Text: "geschubst"}, {Start: 62.75, End: 63, Text: "hat."}}
	for _, c := range []struct {
		name string
		edit func(fsys afero.Fs, path string, tr *Transcript) error
		want string
	}{
		{"dragged", func(fsys afero.Fs, path string, tr *Transcript) error {
			_, err := SetCaptionTime(fsys, path, "01", 61.95, "start", 62.15, false, tr, nil)
			return err
		}, "0.00-2.05 als mich auf dem Schulhof irgendein 2.25-3.50 geschubst hat."},
		{"removed", func(fsys afero.Fs, path string, tr *Transcript) error {
			return SetWordText(fsys, path, "01", 62.1, "", tr)
		}, "0.00-2.35 als mich auf dem Schulhof irgendein 2.35-3.50 geschubst hat."},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			_, path := oneClipPlan(t, fsys, 59.9, 64)
			tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
			if got := strings.Join(shownCaptions(t, fsys, path, tr), " "); got != "0.00-2.05 als mich auf dem Schulhof irgendein 2.05-3.50 Typ geschubst hat." {
				t.Fatalf("the test needs Typ first in the second caption: %s", got)
			}
			if err := c.edit(fsys, path, tr); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(shownCaptions(t, fsys, path, tr), " "); got != c.want {
				t.Errorf("captions\n got %s\nwant %s", got, c.want)
			}
		})
	}
}

// A word removed in the caption box stays removed when the start of its
// caption is dragged out over it: Tim removed a word and dragged the
// caption after it to where he heard its next word begin, and the word
// came back, #598. The start stays where it was let go.
func TestAStartDraggedOverARemovedWordLeavesItRemoved(t *testing.T) {
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}, {Start: 62.25, End: 62.7, Text: "geschubst"}, {Start: 62.75, End: 63, Text: "hat."}}
	fsys := afero.NewMemMapFs()
	_, path := oneClipPlan(t, fsys, 59.9, 64)
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	if err := SetWordText(fsys, path, "01", 62.1, "", tr); err != nil {
		t.Fatal(err)
	}
	// Out past the middle of "Typ", 62.075.
	if _, err := SetCaptionTime(fsys, path, "01", 61.95, "start", 62.0, false, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(shownCaptions(t, fsys, path, tr), " "), "0.00-2.10 als mich auf dem Schulhof irgendein 2.10-3.50 geschubst hat."; got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
}

// The start of a caption can be moved back over the time the caption
// before holds for a word removed between them, as far as the middle of
// the last word that caption shows, and the caption before then goes
// where it appears.
// Tim removed a word and heard the next one start earlier than its block,
// and the block stopped at the caption before, #597. At the app's size of
// the captions "irgendein" is the last word of the first caption.
func TestAStartMovesBackOverARemovedWord(t *testing.T) {
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}}
	fsys := afero.NewMemMapFs()
	_, path := oneClipPlan(t, fsys, 59.9, 64)
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	if got := strings.Join(shownCaptions(t, fsys, path, tr), " "); got != "0.00-2.05 als mich auf dem Schulhof irgendein 2.05-2.70 Typ" {
		t.Fatalf("the test needs irgendein at the end of the first caption: %s", got)
	}
	if err := SetWordText(fsys, path, "01", 61.6, "", tr); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(shownCaptions(t, fsys, path, tr), " "), "0.00-2.05 als mich auf dem Schulhof 2.05-2.70 Typ"; got != want {
		t.Errorf("irgendein removed\n got %s\nwant %s", got, want)
	}
	if _, err := SetCaptionTime(fsys, path, "01", 61.95, "start", 61.7, false, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(shownCaptions(t, fsys, path, tr), " "), "0.00-1.80 als mich auf dem Schulhof 1.80-2.70 Typ"; got != want {
		t.Errorf("moved back to 61.7\n got %s\nwant %s", got, want)
	}
	// Over the caption before it stops in the middle of "Schulhof",
	// 61.175, which the caption before still shows.
	if _, err := SetCaptionTime(fsys, path, "01", 61.95, "start", 61.0, false, tr, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(shownCaptions(t, fsys, path, tr), " "), "0.00-1.27 als mich auf dem Schulhof 1.27-2.70 Typ"; got != want {
		t.Errorf("moved back to 61.0\n got %s\nwant %s", got, want)
	}
}

// In running speech, with no word removed, the start of a caption can be
// moved back into the last word of the caption before, as far as its
// middle, and the caption before goes where it appears. It stopped at the
// end of the caption before, which there is the start itself, so a word
// heard earlier than its block could not be shown earlier at all. Found
// by the review of #599.
func TestAStartMovesBackIntoTheLastWordBefore(t *testing.T) {
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 60.7, Text: "auf"},
		{Start: 60.75, End: 60.9, Text: "dem"}, {Start: 60.95, End: 61.4, Text: "Schulhof"}, {Start: 61.45, End: 61.9, Text: "irgendein"},
		{Start: 61.95, End: 62.2, Text: "Typ"}}
	fsys := afero.NewMemMapFs()
	_, path := oneClipPlan(t, fsys, 59.9, 64)
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	for _, step := range []struct {
		at   float64
		want string
	}{
		{61.7, "0.00-1.80 als mich auf dem Schulhof irgendein 1.80-2.70 Typ"},
		// No further than the middle of "irgendein", 61.675.
		{61.0, "0.00-1.77 als mich auf dem Schulhof irgendein 1.77-2.70 Typ"},
	} {
		if _, err := SetCaptionTime(fsys, path, "01", 61.95, "start", step.at, false, tr, nil); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(shownCaptions(t, fsys, path, tr), " "); got != step.want {
			t.Errorf("moved back to %.2f\n got %s\nwant %s", step.at, got, step.want)
		}
	}
}

// A word corrected and then removed keeps the room of what it reads, not
// of what the recogniser heard. Found by a walk: "vielleicht" corrected to
// "auf" and removed took ten letters' room again, and the caption
// overflowed.
func TestARemovedWordKeepsTheRoomItHad(t *testing.T) {
	fsys := afero.NewMemMapFs()
	heard := []timed.Cue{{Start: 60, End: 60.2, Text: "als"}, {Start: 60.25, End: 60.5, Text: "mich"}, {Start: 60.55, End: 61.2, Text: "vielleicht"},
		{Start: 61.25, End: 61.4, Text: "dem"}, {Start: 61.45, End: 61.9, Text: "Schulhof"}, {Start: 61.95, End: 62.4, Text: "irgendein"},
		{Start: 62.45, End: 62.7, Text: "Typ"}}
	_, path := oneClipPlan(t, fsys, 59.9, 63)
	tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	if err := SetWordText(fsys, path, "01", 60.8, "auf", tr); err != nil {
		t.Fatal(err)
	}
	before := laidOf(t, fsys, path, tr)
	if err := SetWordText(fsys, path, "01", 60.8, "", tr); err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(before, "mich auf dem", "mich dem", 1)
	if got := laidOf(t, fsys, path, tr); got != want {
		t.Errorf("captions\n got %s\nwant %s", got, want)
	}
	// And read again from what was heard, the same.
	again := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
	if got := laidOf(t, fsys, path, again); got != want {
		t.Errorf("read again\n got %s\nwant %s", got, want)
	}
	// Removed a second time, it is the same.
	if err := SetWordText(fsys, path, "01", 60.8, "", tr); err != nil {
		t.Fatal(err)
	}
	if got := laidOf(t, fsys, path, tr); got != want {
		t.Errorf("removed again\n got %s\nwant %s", got, want)
	}
}

// A removed word typed back in beside its neighbour is the word put back
// where it was heard, the same as an undo would put it: its own time, so
// it is lit while it is said, and no correction left on either word.
func TestAWordTypedBackGoesWhereItWasHeard(t *testing.T) {
	heard := []timed.Cue{{Start: 10, End: 10.4, Text: "weil"}, {Start: 10.5, End: 10.8, Text: "das"}, {Start: 11, End: 11.4, Text: "ein"}, {Start: 11.5, End: 12, Text: "echtes"}}
	for _, c := range []struct {
		name    string
		remove  []float64
		at      float64
		text    string
		want    string
		fixes   map[string]string
		removed string
	}{
		{"after", []float64{11.2}, 10.6, "das ein", "weil@10 das@10.5 ein@11 echtes@11.5", map[string]string{}, "[]"},
		{"before", []float64{11.2}, 11.6, "ein echtes", "weil@10 das@10.5 ein@11 echtes@11.5", map[string]string{}, "[]"},
		{"typed otherwise", []float64{11.2}, 10.6, "das eine", "weil@10 das@10.5 eine@11 echtes@11.5", map[string]string{"h11000": "eine"}, "[]"},
		{"more than were removed", []float64{10.6, 11.2}, 10.2, "weil das ein so", "weil@10 das@10.5 ein@11 so@11.2 echtes@11.5", map[string]string{"h11000": "ein so"}, "[]"},
		{"fewer than were removed", []float64{10.6, 11.2}, 11.6, "ein echtes", "weil@10 ein@11 echtes@11.5", map[string]string{}, "[h10500]"},
		{"nothing removed beside it", []float64{11.2}, 10.2, "weil es", "weil@10 es@10.3 das@10.5 echtes@11.5", map[string]string{"h10000": "weil es"}, "[h11000]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			_, path := oneClipPlan(t, fsys, 9.9, 12.1)
			tr := fromStored(append([]timed.Cue(nil), heard...), nil, 0, 0, nil)
			for _, at := range c.remove {
				if err := SetWordText(fsys, path, "01", at, "", tr); err != nil {
					t.Fatal(err)
				}
			}
			if err := SetWordText(fsys, path, "01", c.at, c.text, tr); err != nil {
				t.Fatal(err)
			}
			clip := clipByID(t, fsys, path, "01")
			var got []string
			for _, w := range tr.clipCaptionWords(clip) {
				if _, gone := removedText(w.Text); !gone {
					got = append(got, fmt.Sprintf("%s@%g", w.Text, math.Round(w.Start*10)/10))
				}
			}
			if strings.Join(got, " ") != c.want {
				t.Errorf("words\n got %s\nwant %s", strings.Join(got, " "), c.want)
			}
			if fixes := clip.Corrections; !maps.Equal(fixes, c.fixes) && len(fixes)+len(c.fixes) > 0 {
				t.Errorf("corrections %v, want %v", fixes, c.fixes)
			}
			if removed := fmt.Sprint(clip.RemovedWords.list()); removed != c.removed {
				t.Errorf("the clip removes %s, want %s", removed, c.removed)
			}
		})
	}
}

// A word typed into the one before it, which is how a removed word is put
// back, makes a heard word of two words. Each says which of the two it is,
// so the caption box can correct or remove one without the other.
func TestTheWordsOfOneCorrectionSayWhichTheyAre(t *testing.T) {
	tr := fromStored([]timed.Cue{{Start: 10, End: 10.4, Text: "weil"}, {Start: 10.5, End: 10.8, Text: "das"}, {Start: 11, End: 11.4, Text: "ein"}}, nil, 0, 0, nil)
	clip := Clip{Segments: []Segment{{Start: 9.9, End: 11.5}}, RemovedWords: RemovedWords{wordKey(11): true},
		Corrections: map[string]string{wordKey(10): "weil es"}}
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
	tr := fromStored([]timed.Cue{{Start: 10, End: 10.8, Text: "Und"}, {Start: 11, End: 11.4, Text: "ja"}}, nil, 0, 0, nil)
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
	if same := splitWord(timed.Cue{Start: 1, End: 2, Text: "Und"}); len(same) != 1 || same[0].End != 2 {
		t.Errorf("a single word changed: %v", same)
	}
}

// Several words typed into a short word are all shown, however little of
// its time each one gets. Tim typed four words into a word two frames
// long, and the captions lost the words given less than a frame's share
// of it, wordTouch, as if a cut had only grazed them. The frames they
// stood in then showed no caption at all, and the words he had typed were
// gone when he came back to them.
func TestEveryWordTypedIntoAShortWordIsShown(t *testing.T) {
	clip := Clip{Segments: []Segment{{Start: 10, End: 12}}}
	tr := fromStored([]timed.Cue{{Start: 10, End: 10.4, Text: "hat"}, {Start: 10.4, End: 10.48, Text: "er"}, {Start: 10.48, End: 10.8, Text: "mich"}}, nil, 0, 0, nil)
	tr.Correct(map[string]string{wordKey(10.4): "er hat es dann"})
	captions := Captions(clip, tr.Words, 40, nil)
	var said []string
	for _, c := range captions {
		for _, w := range c.Words {
			said = append(said, w.Text)
		}
	}
	if got := strings.Join(said, " "); got != "hat er hat es dann mich" {
		t.Errorf("the captions say %q", got)
	}
	// Both frames of the word show a caption, at 25 frames a second.
	for _, frame := range []float64{0.4, 0.44} {
		shown := false
		for _, c := range captions {
			shown = shown || (frame >= c.Start && frame < c.End)
		}
		if !shown {
			t.Errorf("the frame at %.2f s shows no caption: %+v", frame, captions)
		}
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
	fsys, logs := afero.NewMemMapFs(), "/home/tim/ep.framefairy/logs"

	words := []timed.Cue{{Start: 10, End: 10.4, Text: "mach"}, {Start: 10.5, End: 11.1, Text: "was"}, {Start: 11.2, End: 11.6, Text: "nicht"}}
	tr := fromStored(append([]timed.Cue(nil), words...), nil, 0, 0, nil)
	plan := `{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 9.9, "end": 11.8}]}]}`
	path := filepath.Join(logs, "clips.json")
	_ = afero.WriteFile(fsys, path, []byte(plan), 0o644)

	// The captions of the clip, built the way a render builds them.
	captions := func() []Caption {
		_, clips, err := LoadClips(fsys, path)
		if err != nil {
			t.Fatal(err)
		}
		return Captions(clips[0], tr.Of(clips[0]).Words, 40, nil)
	}
	said := func() []timed.Cue {
		var out []timed.Cue
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
	if err := SetWordText(fsys, path, "01", 10.5, "was wo", tr); err != nil {
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
	if err := SetWordText(fsys, path, "01", 10.5, "was", tr); err != nil {
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
	fsys := afero.NewMemMapFs()
	path := filepath.Join(memDir(t, fsys), "clips.json")
	plan := `{"clips": [{"id": "01", "segments": [` +
		`{"start": 0, "end": 5, "crop_x": 400}, {"start": 5, "end": 9, "crop_x": 400}, ` +
		`{"start": 10, "end": 14, "crop_x": 400}]}, ` +
		`{"id": "02", "segments": [{"start": 20, "end": 25}]}]}`
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	crops := func(id string) string {
		out := ""
		for _, s := range clipByID(t, fsys, path, id).Segments {
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
	// A crop placed by hand is the crop of the whole clip, cuts and all.
	if err := SetCrop(fsys, path, "01", 2, 501, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	if got := crops("01"); got != "500* 500* 500* " {
		t.Errorf("after moving %q", got)
	}
	// Moving it again keeps the automatic placement from the first time.
	if err := SetCrop(fsys, path, "01", 11, 600, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	// A centred clip can be moved and reset too.
	if err := SetCrop(fsys, path, "02", 21, 100, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	if got := crops("01") + "| " + crops("02"); got != "600* 600* 600* | 100* " {
		t.Errorf("after moving again %q", got)
	}
	if err := ResetCrop(fsys, path, "01", 3); err != nil {
		t.Fatal(err)
	}
	if err := ResetCrop(fsys, path, "02", 21); err != nil {
		t.Fatal(err)
	}
	if got := crops("01") + "| " + crops("02"); got != "400 400 400 | centre " {
		t.Errorf("after resetting %q", got)
	}
	body, _ := afero.ReadFile(fsys, path)
	if strings.Contains(string(body), "crop_x_auto") {
		t.Errorf("reset left the automatic value behind\n%s", body)
	}
	if err := SetCrop(fsys, path, "01", 2, -4, SourceInfo{}, 0); err == nil {
		t.Errorf("a negative crop was accepted")
	}
}
