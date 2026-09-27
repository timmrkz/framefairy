package engine

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// A clip made by hand starts where the playhead stands, is about as long as
// a search's clips are asked to be, and lands in a clip set of its own that
// searched nothing, so the range picker does not call the episode searched
// and giving a searched part back leaves it alone.
func TestAClipMadeByHand(t *testing.T) {
	source := testEpisode(t, "80")
	SetTrainingDir(t.TempDir())
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Min, base.Max = 20, 30
	p := NewProject(e, source, base)
	ctx := context.Background()
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("transcribe: %v %s", err, p.LastError())
	}

	// The sketch the app shows while the clip is framed is the clip that
	// is then made: the same parts, the same title, and captions on it.
	sketch, err := p.SketchClip(30, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sketch.Spans) == 0 || len(sketch.Captions) == 0 || sketch.Title == "" {
		t.Fatalf("the sketch has %d parts, %d captions and the title %q",
			len(sketch.Spans), len(sketch.Captions), sketch.Title)
	}

	// The way the app wraps an edit, so Undo can take the clip away. The
	// first clip made by hand also makes its clip set.
	before := TakeSnapshot(p.LogsDir())
	path, id, err := p.MakeClip(ctx, 30, false)
	if err != nil {
		t.Fatal(err)
	}
	change := Compare(before, TakeSnapshot(p.LogsDir())).LeaveOutNewClips()
	if change == nil {
		t.Fatal("making a clip by hand is not something Undo can take back")
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, clips, _ := LoadClips(path); len(clips) != 0 {
		t.Fatalf("undo left %d clips made by hand", len(clips))
	}
	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if !IsHandPlan(path) || id != "h01" {
		t.Fatalf("made %s in %s", id, path)
	}
	_, clips, err := LoadClips(path)
	if err != nil || len(clips) != 1 {
		t.Fatalf("the clip set holds %d clips: %v", len(clips), err)
	}
	c := clips[0]
	if c.Title != sketch.Title || len(c.Segments) != len(sketch.Spans) {
		t.Fatalf("the clip made is not the one sketched: %q in %d parts, sketched %q in %d",
			c.Title, len(c.Segments), sketch.Title, len(sketch.Spans))
	}
	for k, seg := range c.Segments {
		if math.Abs(seg.Start-sketch.Spans[k].Start) > 0.001 || math.Abs(seg.End-sketch.Spans[k].End) > 0.001 {
			t.Errorf("part %d is %v to %v, sketched %v to %v", k, seg.Start, seg.End,
				sketch.Spans[k].Start, sketch.Spans[k].End)
		}
	}
	if start := c.Segments[0].Start; start > 30 || start < 25 {
		t.Errorf("the clip starts at %v, not at the line the playhead stands in", start)
	}
	if d := c.Duration(); d < 15 || d > 30.5 {
		t.Errorf("the clip is %vs long, not about as long as a search's clips", d)
	}
	if len(c.Words) == 0 || c.Title == "" {
		t.Errorf("the clip has %d words and the title %q", len(c.Words), c.Title)
	}

	// Backward, the clip ends at the line the playhead stands in and
	// reaches back from it, for a moment noticed once it has passed.
	_, back, err := p.MakeClip(ctx, 60, true)
	if err != nil {
		t.Fatal(err)
	}
	_, clips, _ = LoadClips(path)
	for _, b := range clips {
		if b.ID != back {
			continue
		}
		last := b.Segments[len(b.Segments)-1]
		// The line holding second 60, which the fake recogniser, a word
		// every 0.6 seconds without a pause, makes several seconds long.
		lines := BuildLines(mustWords(t, p), nil, base.MaxPause)
		want := 0.0
		for _, l := range lines {
			if l.End() > 60 {
				want = l.End()
				break
			}
		}
		if math.Abs(last.End-want) > 0.5 {
			t.Errorf("the clip made backward ends at %v, not at the line the playhead stands in", last.End)
		}
		if d := b.Duration(); d < 15 || d > 30.5 {
			t.Errorf("the clip made backward is %vs long", d)
		}
	}

	// No search: nothing is marked searched. But a clip is a clip, so
	// giving back a part takes the clips made by hand in it, and leaves
	// the ones outside it.
	st := Status(source, base.ASRModel)
	if got := SearchedWindows(st.Plans, 80); len(got) != 0 {
		t.Errorf("a clip made by hand counts as a search of %v", got)
	}
	if n, err := RemoveRange(path, p.CaptionsDir(), 28, 32, 80); err != nil || n != 1 {
		t.Errorf("giving back the part round the first clip took %d clips: %v", n, err)
	}
	if _, left, _ := LoadClips(path); len(left) != 1 || left[0].ID != back {
		t.Errorf("after giving back a part, %d clips made by hand are left", len(left))
	}

	// Past the transcript there is nothing to make a clip of.
	if _, _, err := p.MakeClip(ctx, 500, false); err == nil {
		t.Error("a clip was made where nothing is said")
	}

	// Several at once, the way a key held down asks: each gets an id of
	// its own and none is lost.
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := p.MakeClip(ctx, 10, i%2 == 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, clips, err = LoadClips(path)
	if err != nil || len(clips) != 5 {
		t.Fatalf("the clip set holds %d clips, not the one left and four more: %v", len(clips), err)
	}
	ids := map[string]bool{}
	for _, c := range clips {
		if ids[c.ID] {
			t.Errorf("two clips are called %s", c.ID)
		}
		ids[c.ID] = true
	}
	if _, err := os.Stat(p.HandPlanPath()); err != nil {
		t.Fatal(err)
	}
}

func mustWords(t *testing.T, p *Project) []Cue {
	t.Helper()
	tr, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	return tr.Words
}

// Where the transcription from the start has not reached, a clip is made
// by hand by hearing the part around the playhead first. The island's
// words count wherever that transcription has not come, and once it has,
// its own words stand and none is counted twice.
func TestAClipMadeByHandWhereNothingIsTranscribed(t *testing.T) {
	source := testEpisode(t, "240")
	SetTrainingDir(t.TempDir())
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Min, base.Max = 20, 30
	p := NewProject(e, source, base)
	ctx := context.Background()

	if p.Heard(200) {
		t.Fatal("a part nobody transcribed counts as heard")
	}
	if _, err := p.SketchClip(200, false); err == nil {
		t.Fatal("a clip was sketched where nothing is transcribed")
	}
	if err := p.HearAround(ctx, 200, false, 240); err != nil {
		t.Fatal(err)
	}
	if !p.Heard(200) || p.Heard(20) {
		t.Fatal("the island is not where the playhead was, or reaches too far")
	}
	calls := heard
	if err := p.HearAround(ctx, 200, false, 240); err != nil || heard != calls {
		t.Fatalf("the same part was heard again: %v", err)
	}
	_, id, err := p.MakeClip(ctx, 200, false)
	if err != nil {
		t.Fatal(err)
	}
	_, clips, _ := LoadClips(p.HandPlanPath())
	if len(clips) != 1 || clips[0].ID != id || clips[0].Segments[0].Start < 170 || clips[0].Segments[0].Start > 200 {
		t.Fatalf("the clip made on the island is %+v", clips)
	}

	// Now the transcription from the start runs over it. The island is
	// left unread and no word is there twice.
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("transcribe: %v %s", err, p.LastError())
	}
	tr, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	for k := 1; k < len(tr.Words); k++ {
		if tr.Words[k].Start < tr.Words[k-1].End-0.001 {
			t.Fatalf("words %d and %d overlap: %+v %+v", k-1, k, tr.Words[k-1], tr.Words[k])
		}
	}
	if len(tr.Extra) != 0 {
		t.Errorf("the island still adds %d readings after the transcription passed it", len(tr.Extra))
	}
}

// A clip made by hand next to an island hears only what is missing, and the
// two islands read as one transcript: in time order, whatever their names
// sort as, with the seam where both heard the audio and no word twice.
func TestAClipMadeByHandNextToAnIsland(t *testing.T) {
	source := testEpisode(t, "300")
	SetTrainingDir(t.TempDir())
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Min, base.Max = 20, 30
	p := NewProject(e, source, base)
	ctx := context.Background()

	if err := p.HearAround(ctx, 200, false, 300); err != nil {
		t.Fatal(err)
	}
	if p.Unheard(200, false, 300) {
		t.Fatal("the part just heard still counts as unheard")
	}
	if !p.Unheard(130, false, 300) {
		t.Fatal("the part before the island counts as heard")
	}
	// The island for 200 is 165 to 265: from 30 seconds before it to 30
	// past the longest clip after it, and five seconds of seam. I at 130
	// needs 100 to 190, and 165 onwards is there, so only the part before
	// it is heard, reaching a little into the island.
	if err := p.HearAround(ctx, 130, false, 300); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range islandFiles(p.LogsDir()) {
		names = append(names, filepath.Base(f))
	}
	if strings.Join(names, " ") != "words-95-170.json words-165-265.json" {
		t.Fatalf("the islands are %v", names)
	}
	if p.Unheard(130, false, 300) {
		t.Fatal("the part before the island is still unheard")
	}
	words := mustWords(t, p)
	for k := 1; k < len(words); k++ {
		if words[k].Start < words[k-1].End-0.001 {
			t.Fatalf("words %d and %d overlap: %+v %+v", k-1, k, words[k-1], words[k])
		}
		if words[k].Start > 96 && words[k].End < 264 && words[k].Start-words[k-1].End > 2 {
			t.Fatalf("a hole from %.1f to %.1f", words[k-1].End, words[k].Start)
		}
	}
	if len(words) == 0 || words[0].Start > 96 || words[len(words)-1].End < 260 {
		t.Fatalf("the words reach from %.1f to %.1f", words[0].Start, words[len(words)-1].End)
	}
	// The waveform is there on both islands, and silent where nothing is
	// transcribed.
	tr, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	peaks := tr.Peaks(0, 300, 300)
	for s, dB := range peaks {
		if heard := s >= 95 && s < 265; heard != (dB > -89.5) {
			t.Fatalf("the waveform at %d s is %.1f dB", s, dB)
		}
	}
	if _, _, err := p.MakeClip(ctx, 130, false); err != nil {
		t.Fatal(err)
	}
}
