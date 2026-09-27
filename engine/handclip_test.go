package engine

import (
	"bytes"
	"context"
	"math"
	"os"
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
