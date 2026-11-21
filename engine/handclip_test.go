package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
)

// underwayLog is a log that keeps every list of clips on the way it is
// told, in order.
func underwayLog() (*runlog.Log, func() [][]runlog.Underway) {
	var mu sync.Mutex
	var told [][]runlog.Underway
	log := runlog.New(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev runlog.Event) {
		if ev.Kind != runlog.EventUnderway {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		told = append(told, append([]runlog.Underway(nil), ev.Underway...))
	})
	return log, func() [][]runlog.Underway {
		mu.Lock()
		defer mu.Unlock()
		return append([][]runlog.Underway(nil), told...)
	}
}

// On the real disk: ffmpeg reads the episode, and the project works beside it.
func handProject(t *testing.T, source string, log *runlog.Log) *Project {
	t.Helper()
	var heard int32
	e := NewEngine(log, NewMachine(disk))
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
// it is written. Its card is known by the name of the clip it becomes
// from the first, so it is the same clip from the card to the clip
// written.
//
// On the real disk: ffmpeg reads the episode the clip is heard and framed from.
func TestAClipMadeByHand(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	log, told := underwayLog()
	p := handProject(t, source, log)
	ctx := context.Background()

	key, err := p.MakeClip(ctx, "clip-1", ClipRequest{At: 20}, Host{})
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if key != HandPlanName+"/h01" {
		t.Fatalf("made %q", key)
	}
	// Heard where the clip can reach, and not from the start.
	reach := ClipRequest{At: 20}.reach(30, 70)
	if gaps := Unheard(disk, source, p.Base.ASRModel, reach); len(gaps) != 0 {
		t.Errorf("%v not heard, of the %v the clip can reach", gaps, reach)
	}
	if covered, _ := Coverage(disk, source, p.Base.ASRModel); covered != 0 {
		t.Errorf("heard from the start to %.1f s", covered)
	}
	view, err := ReadPlan(disk, p.HandPlanPath())
	if err != nil || len(view.Clips) != 1 {
		t.Fatalf("%v %+v", err, view)
	}
	c := view.Clips[0]
	// How long it grows is TestWhatThePlayheadProposes. Here it holds the
	// playhead and is no longer than Longest.
	if c.Start > 20 || c.Start+c.Duration < 20 || c.Duration > 30.5 {
		t.Errorf("the clip runs %.1f s from %.1f s, want at most 30 s from the line at 20 s", c.Duration, c.Start)
	}
	if !strings.HasPrefix(c.Title, "wort") {
		t.Errorf("titled %q, want its first words", c.Title)
	}
	// What an opened video says is the edges of the lines a clip made by
	// hand is made of, so the app knows where I and O can make one.
	opened := NewMachine(disk).Episode(source)
	if _, err := opened.Transcript(p.Base.ASRModel, p.Base.SilenceDB); err != nil {
		t.Fatal(err)
	}
	st := opened.Status(p.Base.ASRModel, p.Base.SilenceDB)
	lined, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	if want := saidOf(BuildLines(lined.Words, lined.Levels(), p.Base.MaxPause)); st.Said == nil || want == nil || *st.Said != *want {
		t.Errorf("said %v, not %v, the edges of the lines MakeClip makes", st.Said, want)
	}
	summaries := st.Plans
	if len(summaries) != 1 || summaries[0].By != ByHand {
		t.Fatalf("clip sets %+v", summaries)
	}
	if passes := fmt.Sprint(SearchPasses(summaries, 70)); passes != "[{{0 70} 0}]" {
		t.Errorf("clips made by hand searched %s", passes)
	}
	if left := ReadJobs(disk, source); len(left) != 0 {
		t.Errorf("records left behind: %+v", left)
	}

	lists := told()
	if len(lists) < 3 {
		t.Fatalf("on the way %v", lists)
	}
	if first := lists[0]; len(first) != 1 || first[0].N != 1 || first[0].Start != 20 || first[0].End != 20 {
		t.Errorf("first on the way %+v, want the playhead", first)
	}
	for _, list := range lists {
		for _, u := range list {
			if u.Clip != key {
				t.Errorf("on the way as %q, and written as %q", u.Clip, key)
			}
		}
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
	// Before its crop is placed it says what it keeps, and its captions are
	// laid out from that the way the written clip's are.
	var cut *runlog.Underway
	for _, list := range lists {
		if len(list) == 1 && len(list[0].Pieces) > 0 {
			cut = &list[0]
			break
		}
	}
	if cut == nil {
		t.Fatalf("never said what it keeps: %v", lists)
	}
	if first, last := cut.Pieces[0][0], cut.Pieces[len(cut.Pieces)-1][1]; math.Abs(first-c.Start) > 0.01 ||
		math.Abs(last-(c.Start+c.Duration)) > 0.5 {
		t.Errorf("on the way it keeps %v, it was written from %.2f for %.2f s", cut.Pieces, c.Start, c.Duration)
	}
	heard, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	arriving := ArrivingCaptionsView(planAt(t, disk, p.HandPlanPath()), *cut, heard, nil)
	written, err := ClipCaptionsView(planAt(t, disk, p.HandPlanPath()), c.ID, heard, nil)
	if arriving == nil || err != nil || len(arriving.Captions) == 0 || len(arriving.Captions) != len(written.Captions) {
		t.Errorf("captions on the way %v, written %v %v", arriving, written, err)
	}

	// O at 60 s ends a clip there and grows it back, into the same set,
	// under the id it was given when it was asked for.
	key, err = p.MakeClip(ctx, "clip-2", ClipRequest{At: 60, Backward: true, ID: "h03"}, Host{})
	if err != nil || key != HandPlanName+"/h03" {
		t.Fatalf("%q %v", key, err)
	}
	view, _ = ReadPlan(disk, p.HandPlanPath())
	if len(view.Clips) != 2 {
		t.Fatalf("%d clips", len(view.Clips))
	}
	// It ends with the sentence the playhead stands in. The test's words
	// have no punctuation, so that sentence is the line, which runs until
	// it is maxLineSeconds long, and where lines fall moves with how long
	// each word is. So the clip ends at 60 s or after, by no more than a
	// line, and is as long as it may be.
	o := view.Clips[1]
	if o.End < 60-0.01 || o.End > 60+maxLineSeconds || o.Duration < 20 || o.Duration > 30.5 {
		t.Errorf("O at 60 s made %.1f to %.1f s", o.Start, o.Start+o.Duration)
	}
}

// transcriptOpens is a disk that counts how often the files of a
// transcript are opened, the one way a transcript is read.
type transcriptOpens struct {
	afero.Fs
	n atomic.Int64
}

func (f *transcriptOpens) Open(name string) (afero.File, error) {
	f.count(name)
	return f.Fs.Open(name)
}

func (f *transcriptOpens) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f.count(name)
	return f.Fs.OpenFile(name, flag, perm)
}

func (f *transcriptOpens) count(name string) {
	if slices.Contains(TranscriptFiles(filepath.Dir(name)), name) {
		f.n.Add(1)
	}
}

// A press of I or O where the video is heard already reads no transcript
// from the disk: the one the video holds is the one the clip is made of.
// A long video's transcript takes a fifth of a second to read, every time
// it is read, before the clip's crop can be placed.
//
// On the real disk: ffmpeg reads the episode the clip is heard and framed from.
func TestAClipMadeByHandReadsTheTranscriptTheVideoHolds(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	log, _ := underwayLog()
	opens := &transcriptOpens{Fs: disk}
	var heard int32
	e := NewEngine(log, NewMachine(opens))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Min, base.Max = 20, 30
	p := NewProject(e, source, base)
	ctx := context.Background()

	if _, err := p.MakeClip(ctx, "clip-1", ClipRequest{At: 20}, Host{}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	before := opens.n.Load()
	if _, err := p.MakeClip(ctx, "clip-2", ClipRequest{At: 20}, Host{}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if n := opens.n.Load() - before; n != 0 {
		t.Errorf("the second clip opened the transcript's files %d times, want none", n)
	}
	if view, err := ReadPlan(disk, p.HandPlanPath()); err != nil || len(view.Clips) != 2 {
		t.Fatalf("%v %+v", err, view)
	}
}

// Clips made by hand at the same moment each go in, each with a number of
// its own, taken where the set is written.
//
// On the real disk: ffmpeg reads the episode the clips are heard and framed from.
func TestClipsMadeByHandAtOnce(t *testing.T) {
	t.Parallel()
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
			q := NewProject(NewEngine(log, NewMachine(disk)), source, p.Base)
			keys[i], errs[i] = q.MakeClip(context.Background(), fmt.Sprintf("clip-%d", i),
				ClipRequest{At: float64(5 + i*12)}, Host{})
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
	view, err := ReadPlan(disk, p.HandPlanPath())
	if err != nil || len(view.Clips) != 4 {
		t.Fatalf("%v, %d clips", err, len(view.Clips))
	}
}

// A clip made by hand called off leaves no clip, keeps its card where the
// clip would have appeared, saying it stopped, and keeps its record, to be
// carried on like any job.
//
// On the real disk: the project works beside an episode ffmpeg made.
func TestAClipMadeByHandCalledOff(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	log, told := underwayLog()
	p := handProject(t, source, log)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.MakeClip(ctx, "clip-9", ClipRequest{At: 30}, Host{}); err == nil {
		t.Fatal("a clip called off was made")
	}
	if _, clips, err := LoadClips(disk, p.HandPlanPath()); err == nil && len(clips) > 0 {
		t.Errorf("%d clips made", len(clips))
	}
	lists := told()
	if len(lists) == 0 {
		t.Fatal("never on the way")
	}
	if last := lists[len(lists)-1]; len(last) != 1 || last[0].Step != StepStopped || last[0].Start != 30 {
		t.Errorf("last on the way %+v, want it stopped at the playhead", last)
	}
	recs := ReadJobs(disk, source)
	if len(recs) != 1 || recs[0].Kind != JobClip || recs[0].Clip() != (ClipRequest{At: 30, ID: "h01"}) || recs[0].Step == StepFailed {
		t.Errorf("records %+v", recs)
	}
	// Carried on from its record, its card keeps the name it had.
	if on := recs[0].Underway(); len(on) != 1 || on[0].Clip != HandPlanName+"/h01" || lists[len(lists)-1][0].Clip != on[0].Clip {
		t.Errorf("stopped as %+v, read back as %+v, want both %s/h01", lists[len(lists)-1], on, HandPlanName)
	}
}

// The id a clip made by hand is given when it is asked for comes after
// those of the clips made by hand there are and of those still on their
// way, so two asked for at once never share one.
func TestNextHandClipID(t *testing.T) {
	t.Parallel()
	fs := afero.NewMemMapFs()
	// A work folder is only made beside a video whose folder is there.
	if err := fs.MkdirAll("/v", 0o755); err != nil {
		t.Fatal(err)
	}
	p := NewProject(NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(fs)), "/v/episode.mp4", DefaultOptions())
	if got := NextHandClipID(fs, p.Source, nil); got != "h01" {
		t.Errorf("with no clip made by hand, %q", got)
	}
	if got := NextHandClipID(fs, p.Source, []string{"h01", "h04", "t30-02", "hx"}); got != "h05" {
		t.Errorf("with h01 and h04 on their way, %q", got)
	}
	set := PlanFile{Source: "episode.mp4"}
	for _, id := range []string{"h01", "h02"} {
		if _, err := addClip(fs, p.HandPlanPath(), set, PlanClip{ID: id, Slug: id,
			Segments: []PlanSegment{{Start: 1, End: 25, CropX: "center"}}}, handPrefix); err != nil {
			t.Fatal(err)
		}
	}
	if got := NextHandClipID(fs, p.Source, []string{"h01"}); got != "h03" {
		t.Errorf("with h01 and h02 written, %q", got)
	}
	// An id taken by the time the clip is written is the next one there.
	added, err := addClip(fs, p.HandPlanPath(), set, PlanClip{ID: "h02", Slug: "again",
		Segments: []PlanSegment{{Start: 30, End: 55, CropX: "center"}}}, handPrefix)
	if err != nil || added.ID != "h03" {
		t.Errorf("a clip given h02, already there, went in as %q, %v", added.ID, err)
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
		var cues []timed.Cue
		for k, w := range words {
			at := start + float64(k)*(4.5/float64(len(words)))
			cues = append(cues, timed.Cue{Start: at, End: at + 4.5/float64(len(words)), Text: w})
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

// Where I and O make a clip, the rule the app's I and O keys go by:
// MarkingOf says it with the video's state, and the workspace offers I
// before InBefore and O from OutFrom on.
func TestWhereIAndOMakeAClip(t *testing.T) {
	all := [][2]float64{{0, 120}}
	said := func(a, b float64) *[2]float64 { return &[2]float64{a, b} }
	cases := []struct {
		name     string
		at       float64
		out      bool
		heard    [][2]float64
		said     *[2]float64
		duration float64
		makes    bool
	}{
		{"I at the very end, nothing heard", 120, false, nil, nil, 120, false},
		{"I a moment before the end, nothing heard", 119.97, false, nil, nil, 120, false},
		{"I on the last frame, where play stops short of the end", 120 - 1.0/15, false, nil, nil, 120, false},
		{"I on the last frame, half heard", 120 - 1.0/15, false, [][2]float64{{0, 60}}, said(3, 58), 120, false},
		{"I in the last half second, half heard", 119.6, false, [][2]float64{{0, 60}}, said(3, 58), 120, false},
		{"O at the very start, nothing heard", 0, true, nil, nil, 120, false},
		{"I after the last word, all heard", 115, false, all, said(3, 110), 120, false},
		{"I on the end of the last word, all heard", 110, false, all, said(3, 110), 120, false},
		{"O before the first word, all heard", 2, true, all, said(3, 110), 120, false},
		{"I before the last word", 109.9, false, all, said(3, 110), 120, true},
		{"O on the first word", 3, true, all, said(3, 110), 120, true},
		{"O after the first word", 115, true, all, said(3, 110), 120, true},
		{"I before the first word", 2, false, all, said(3, 110), 120, true},
		{"I with no word in a video heard to its end", 60, false, all, nil, 120, false},
		{"O with no word in a video heard to its end", 60, true, all, nil, 120, false},
		{"I where the part after is not heard yet", 100, false, [][2]float64{{0, 60}}, said(3, 58), 120, true},
		{"O where the part before is not heard yet", 100, true, [][2]float64{{60, 120}}, said(70, 110), 120, true},
		{"O with nothing heard", 60, true, nil, nil, 120, true},
		{"I with parts out of order and a gap under half a second", 115, false,
			[][2]float64{{60.3, 120}, {0, 60}}, said(3, 110), 120, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := MarkingOf(c.heard, c.said, c.duration)
			if m == nil {
				t.Fatal("no marking for a video with a length")
			}
			makes := c.at < m.InBefore
			if c.out {
				makes = c.at >= m.OutFrom
			}
			if makes != c.makes {
				t.Errorf("at %v it makes a clip %v, want %v, with %+v", c.at, makes, c.makes, *m)
			}
		})
	}
	if m := MarkingOf(nil, nil, 0); m != nil {
		t.Errorf("a video of no length has %+v, and nothing to decide", *m)
	}
}
