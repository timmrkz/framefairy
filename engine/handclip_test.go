package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

// underwayLog is a log that keeps every list of clips on the way it is
// told, in order.
func underwayLog() (*Log, func() [][]Underway) {
	var mu sync.Mutex
	var told [][]Underway
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev Event) {
		if ev.Kind != EventUnderway {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		told = append(told, append([]Underway(nil), ev.Underway...))
	})
	return log, func() [][]Underway {
		mu.Lock()
		defer mu.Unlock()
		return append([][]Underway(nil), told...)
	}
}

func handProject(t *testing.T, source string, log *Log) *Project {
	t.Helper()
	var heard int32
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Min, base.Max = 20, 30
	return NewProject(e, source, base)
}

// A clip made by hand goes where every clip goes: the transcript is heard
// as far as it can reach, the playhead proposes it, and the plan builder
// shapes, frames and writes it, into the clip set made by hand, which grows
// with each one and searched nothing. It is on the way from the moment it
// is asked for, at the playhead, and then where its sentences are, until
// it is written.
func TestAClipMadeByHand(t *testing.T) {
	source := testEpisode(t, "70")
	log, told := underwayLog()
	p := handProject(t, source, log)
	ctx := context.Background()

	key, err := p.MakeClip(ctx, "clip-1", ClipRequest{At: 20}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if key != HandPlanName+"/h01" {
		t.Fatalf("made %q", key)
	}
	covered, _ := Coverage(source, p.Base.ASRModel)
	if covered < 20+30 {
		t.Errorf("heard to %.1f s, not as far as the clip can reach", covered)
	}
	view, err := ReadPlan(p.HandPlanPath())
	if err != nil || len(view.Clips) != 1 {
		t.Fatalf("%v %+v", err, view)
	}
	c := view.Clips[0]
	if c.Start > 20 || c.Duration < 20 || c.Duration > 30.5 {
		t.Errorf("the clip runs %.1f s from %.1f s, want 20 to 30 s from the line at 20 s", c.Duration, c.Start)
	}
	if !strings.HasPrefix(c.Title, "wort") {
		t.Errorf("titled %q, want its first words", c.Title)
	}
	summaries := Status(source, p.Base.ASRModel).Plans
	if len(summaries) != 1 || summaries[0].By != ByHand {
		t.Fatalf("clip sets %+v", summaries)
	}
	if searched := SearchedWindows(summaries, 70); len(searched) != 0 {
		t.Errorf("clips made by hand searched %v", searched)
	}
	if left := ReadJobs(source); len(left) != 0 {
		t.Errorf("records left behind: %+v", left)
	}

	lists := told()
	if len(lists) < 3 {
		t.Fatalf("on the way %v", lists)
	}
	if first := lists[0]; len(first) != 1 || first[0].N != 1 || first[0].Start != 20 || first[0].End != 20 {
		t.Errorf("first on the way %+v, want the playhead", first)
	}
	framing := false
	for _, list := range lists {
		if len(list) == 1 && list[0].Step == StepFraming && list[0].N == 1 &&
			math.Abs(list[0].Start-c.Start) < 0.5 && list[0].Title == c.Title {
			framing = true
		}
	}
	if !framing {
		t.Errorf("never on the way as its sentences: %v", lists)
	}
	if last := lists[len(lists)-1]; len(last) != 0 {
		t.Errorf("still on the way once written: %+v", last)
	}

	// O at 60 s ends a clip there and grows it back, into the same set.
	key, err = p.MakeClip(ctx, "clip-2", ClipRequest{At: 60, Backward: true}, nil)
	if err != nil || key != HandPlanName+"/h02" {
		t.Fatalf("%q %v", key, err)
	}
	view, _ = ReadPlan(p.HandPlanPath())
	if len(view.Clips) != 2 {
		t.Fatalf("%d clips", len(view.Clips))
	}
	o := view.Clips[1]
	if o.End < 60-0.01 || o.Start > 60-20 || o.Duration > 30.5 {
		t.Errorf("O at 60 s made %.1f to %.1f s", o.Start, o.Start+o.Duration)
	}
}

// Clips made by hand at the same moment each go in, each with a number of
// its own, taken where the set is written.
func TestClipsMadeByHandAtOnce(t *testing.T) {
	source := testEpisode(t, "70")
	log, _ := underwayLog()
	p := handProject(t, source, log)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	keys := make([]string, 4)
	errs := make([]error, 4)
	for i := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A project each, the way the app runs every job with its own.
			q := NewProject(NewEngine(log), source, p.Base)
			keys[i], errs[i] = q.MakeClip(context.Background(), fmt.Sprintf("clip-%d", i),
				ClipRequest{At: float64(5 + i*12)}, nil)
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, key := range keys {
		if errs[i] != nil {
			t.Fatalf("clip %d: %v", i, errs[i])
		}
		if seen[key] {
			t.Errorf("%s made twice", key)
		}
		seen[key] = true
	}
	view, err := ReadPlan(p.HandPlanPath())
	if err != nil || len(view.Clips) != 4 {
		t.Fatalf("%v, %d clips", err, len(view.Clips))
	}
}

// A clip made by hand called off leaves no clip, says nothing is on the
// way any more, and keeps its record, to be carried on like any job.
func TestAClipMadeByHandCalledOff(t *testing.T) {
	source := testEpisode(t, "70")
	log, told := underwayLog()
	p := handProject(t, source, log)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.MakeClip(ctx, "clip-9", ClipRequest{At: 30}, nil); err == nil {
		t.Fatal("a clip called off was made")
	}
	if _, clips, err := LoadClips(p.HandPlanPath()); err == nil && len(clips) > 0 {
		t.Errorf("%d clips made", len(clips))
	}
	lists := told()
	if len(lists) == 0 || len(lists[len(lists)-1]) != 0 {
		t.Errorf("still on the way: %v", lists)
	}
	recs := ReadJobs(source)
	if len(recs) != 1 || recs[0].Kind != JobClip || recs[0].Clip() != (ClipRequest{At: 30}) || !recs[0].Interrupted() {
		t.Errorf("records %+v", recs)
	}
}

// The playhead proposes a clip of whole sentences: I starts with the
// sentence the playhead stands in, from where that sentence begins, and O
// ends with it, where it ends. Each grows a line at a time to Shortest and
// never past Longest.
func TestWhatThePlayheadProposes(t *testing.T) {
	// Lines of 5 s, one after the other. A sentence ends at lines 2, 5
	// and 9.
	texts := []string{"Es war", "einmal so.", "Und dann", "kam er", "zurück.", "Wir", "gingen", "nach", "Hause."}
	var lines []Line
	for i, text := range texts {
		start := float64(i) * 5
		words := strings.Fields(text)
		var cues []Cue
		for k, w := range words {
			at := start + float64(k)*(4.5/float64(len(words)))
			cues = append(cues, Cue{at, at + 4.5/float64(len(words)), w})
		}
		lines = append(lines, Line{Index: i + 1, Cues: cues})
	}
	seconds := func(keep [][2]int) float64 {
		total := 0.0
		for _, run := range keep {
			total += lines[run[1]-1].End() - lines[run[0]-1].Start()
		}
		return total
	}
	for _, c := range []struct {
		name      string
		req       ClipRequest
		min, max  float64
		wantFirst int
		wantLast  int
	}{
		// In line 4, whose sentence begins at line 3.
		{"I in a sentence", ClipRequest{At: 16}, 12, 30, 3, 5},
		// Grown to Shortest: 3 to 6 is 19.5 s.
		{"I grows to Shortest", ClipRequest{At: 16}, 18, 30, 3, 6},
		// Never past Longest for a line more.
		{"I stops at Longest", ClipRequest{At: 16}, 30, 17, 3, 5},
		// O in line 4 ends where its sentence ends, line 5.
		{"O in a sentence", ClipRequest{At: 16, Backward: true}, 12, 30, 3, 5},
		// Grown back to Shortest.
		// Grown back to Shortest, a line at a time. Putting its start on a
		// sentence is the builder's, as it is for every clip.
		{"O grows back", ClipRequest{At: 16, Backward: true}, 18, 30, 2, 5},
		// In the pause after line 6, I takes the line after, from where
		// its sentence begins.
		{"I in a pause", ClipRequest{At: 29.8}, 3, 30, 6, 7},
	} {
		entry, err := handEntry(lines, c.req, c.min, c.max, seconds)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := entry.Keep; len(got) != 1 || got[0] != [2]int{c.wantFirst, c.wantLast} {
			t.Errorf("%s: keeps %v, want [[%d %d]]", c.name, got, c.wantFirst, c.wantLast)
		}
	}
	if _, err := handEntry(lines, ClipRequest{At: 60}, 10, 30, seconds); err == nil {
		t.Error("I past the last word proposed a clip")
	}
	if entry, _ := handEntry(lines, ClipRequest{At: 16}, 12, 30, seconds); entry.Title != "Und dann kam er zurück" {
		t.Errorf("titled %q", entry.Title)
	}
}
