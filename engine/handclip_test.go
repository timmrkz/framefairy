package engine

import (
	"bytes"
	"context"
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
	path, id, err := p.MakeClip(ctx, 30)
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

	// No search: nothing is searched, and giving back a part leaves it.
	st := Status(source, base.ASRModel)
	if got := SearchedWindows(st.Plans, 80); len(got) != 0 {
		t.Errorf("a clip made by hand counts as a search of %v", got)
	}
	if n, err := RemoveRange(path, p.CaptionsDir(), 0, 80, 80); err != nil || n != 0 {
		t.Errorf("giving back the whole episode took %d clips made by hand: %v", n, err)
	}

	// Past the transcript there is nothing to make a clip of.
	if _, _, err := p.MakeClip(ctx, 500); err == nil {
		t.Error("a clip was made where nothing is said")
	}

	// Several at once, the way a key held down asks: each gets an id of
	// its own and none is lost.
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := p.MakeClip(ctx, 10); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_, clips, err = LoadClips(path)
	if err != nil || len(clips) != 5 {
		t.Fatalf("the clip set holds %d clips after five were made: %v", len(clips), err)
	}
	if _, err := os.Stat(p.HandPlanPath()); err != nil {
		t.Fatal(err)
	}
}
