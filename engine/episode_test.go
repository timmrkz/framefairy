package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The app searches by itself only for an episode nobody has ever searched.
// What says so lives with the episode, so it survives a restart, it is not
// undone by removing the clips again, and deleting the work folder makes
// the episode new.
func TestAnEpisodeRemembersItHasBeenSearched(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if EverSearched(source) {
		t.Fatal("an episode nobody has searched should not say it has been")
	}
	if st := Status(source, ""); st.EverSearched || st.Work {
		t.Errorf("status of a fresh episode %+v", st)
	}

	// Clips made by hand search nothing: neither their set nor their
	// jobs' timings say the episode was searched.
	p := NewProject(NewEngine(NewLog(io.Discard, false, false)), source, DefaultOptions())
	hand := PlanFile{Source: "ep.mp4", PlannedWith: PlannedWith{By: ByHand}, Clips: []PlanClip{{ID: "h01",
		Slug: "hand", Keep: [][2]int{{1, 1}},
		Segments: []PlanSegment{{Start: 1, End: 20, CropX: "center"}}}}}
	if _, err := addClip(p.HandPlanPath(), hand, hand.Clips[0], "h"); err != nil {
		t.Fatal(err)
	}
	p.addTimings(JobRecord{ID: "clip-1", Kind: JobClip})
	if EverSearched(source) {
		t.Fatal("clips made by hand made the episode searched")
	}

	// A search that was asked for keeps its record, and its timings once
	// it is done, and either is enough.
	if err := WriteJob(source, JobRecord{ID: SearchID, Kind: JobSearch, Step: StepWaiting}); err != nil {
		t.Fatal(err)
	}
	if !EverSearched(source) {
		t.Fatal("an episode being searched says it was not searched")
	}
	if err := RemoveJob(source, SearchID); err != nil {
		t.Fatal(err)
	}
	p.addTimings(JobRecord{ID: SearchID, Kind: JobSearch})
	if !EverSearched(source) {
		t.Fatal("an episode that was searched says it was not")
	}
	if st := Status(source, ""); !st.EverSearched || !st.Work {
		t.Errorf("status after a search %+v", st)
	}

	if err := DeleteWork(source); err != nil {
		t.Fatal(err)
	}
	if EverSearched(source) {
		t.Error("an episode whose work folder is gone is new again")
	}
}

// A short still being written is not a short, and neither is a file in
// the out folder no clip is named for.
func TestAShortBeingWrittenIsNotCounted(t *testing.T) {
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(WorkDir(source), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(LogsDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := `{"clips": [
  {"id": "01", "slug": "done", "segments": [{"start": 1, "end": 21}]},
  {"id": "02", "slug": "half", "segments": [{"start": 30, "end": 50}]},
  {"id": "03", "slug": "upper", "segments": [{"start": 60, "end": 80}]}]}`
	if err := os.WriteFile(filepath.Join(LogsDir(source), "clips.json"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01_done.mp4", "02_half.part.mp4", "03_upper.PART.MP4", "04_gone.mp4"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := Status(source, "").Rendered; got != 1 {
		t.Errorf("%d rendered, want 1", got)
	}
}

// A short rendered into the folder the settings name for shorts is the
// clip's short: the clip says it is rendered and where, the episode counts
// it, and the app may show it, see IsShort. It stays the clip's short when
// the folder in the settings changes afterwards, and it is gone when the
// file is. A file of the same name that another episode put in that
// folder is not this clip's short, and neither is anything beside it.
func TestAShortInTheOutputFolderIsTheClipsShort(t *testing.T) {
	t.Parallel()
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	clip := func() ClipView {
		t.Helper()
		view, err := ReadPlan(plan)
		if err != nil || len(view.Clips) == 0 {
			t.Fatalf("the plan: %v", err)
		}
		return view.Clips[0]
	}
	rendered := func() int { return Status(p.Source, p.Base.ASRModel).Rendered }
	shorts := t.TempDir()
	short := filepath.Join(ResolvePath(shorts), clip().Basename+".mp4")

	if err := os.WriteFile(short, []byte("another episode's short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := clip().Rendered; got != "" {
		t.Errorf("a clip never rendered says it is, at %s", got)
	}
	if IsShort(p.Source, short) {
		t.Error("another episode's short is taken for this clip's")
	}

	p.Base.Out = shorts
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if got := clip().Rendered; got != short {
		t.Errorf("rendered into the folder for shorts, the clip says %q, not %q", got, short)
	}
	if !IsShort(p.Source, short) {
		t.Error("the short in the folder for shorts is not the clip's")
	}
	if n := rendered(); n != 1 {
		t.Errorf("the episode counts %d shorts, not 1", n)
	}

	p.Base.Out = t.TempDir()
	if got := clip().Rendered; got != short {
		t.Errorf("after the folder in the settings changed the clip says %q, not %q", got, short)
	}
	beside := filepath.Join(shorts, "notes.mp4")
	if err := os.WriteFile(beside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsShort(p.Source, beside) || IsShort(p.Source, shorts) {
		t.Error("what is beside the short is taken for one")
	}

	if err := os.Remove(short); err != nil {
		t.Fatal(err)
	}
	if got := clip().Rendered; got != "" {
		t.Errorf("a short removed by hand is still the clip's, at %s", got)
	}
	if IsShort(p.Source, short) || rendered() != 0 {
		t.Error("a short removed by hand is still counted")
	}
}

// Two episodes that render into one folder for shorts keep a short each,
// even when a clip of one has the same id and slug as a clip of the
// other. The second render leaves the first episode's short as it was,
// and neither clip takes the other's short for its own.
func TestTwoEpisodesKeepTheirOwnShorts(t *testing.T) {
	t.Parallel()
	shorts := t.TempDir()
	type episode struct {
		p    *Project
		plan string
	}
	var both [2]episode
	for i := range both {
		p, _ := searchProject(t, nil)
		plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
		if err != nil {
			t.Fatalf("%v %s", err, p.LastError())
		}
		p.Base.Out = shorts
		both[i] = episode{p, plan}
	}
	clip := func(ep episode) ClipView {
		t.Helper()
		view, err := ReadPlan(ep.plan)
		if err != nil || len(view.Clips) == 0 {
			t.Fatalf("the plan: %v", err)
		}
		return view.Clips[0]
	}
	render := func(ep episode) string {
		t.Helper()
		if err := ep.p.Render(context.Background(), RenderRequest{Plan: ep.plan}); err != nil {
			t.Fatalf("%v %s", err, ep.p.LastError())
		}
		short := clip(ep).Rendered
		if short == "" {
			t.Fatal("a clip just rendered says it has no short")
		}
		return short
	}
	first, second := both[0], both[1]
	if a, b := clip(first).Basename, clip(second).Basename; a != b {
		t.Fatalf("the clips are %s and %s, which never meet in one folder", a, b)
	}

	firstShort := render(first)
	before, err := os.Stat(firstShort)
	if err != nil {
		t.Fatal(err)
	}
	secondShort := render(second)

	if secondShort == firstShort {
		t.Errorf("both episodes' shorts are %s", firstShort)
	}
	after, err := os.Stat(firstShort)
	if err != nil || !os.SameFile(before, after) {
		t.Errorf("the second render wrote over the first episode's short at %s", firstShort)
	}
	if got := clip(first).Rendered; got != firstShort {
		t.Errorf("the first episode's clip says its short is %q, not %q", got, firstShort)
	}
	if IsShort(first.p.Source, secondShort) {
		t.Error("the first episode takes the second's short for its own")
	}
	if IsShort(second.p.Source, firstShort) {
		t.Error("the second episode takes the first's short for its own")
	}
}

// What a render makes is no edit, so an edit made before a render is
// undone after it as it would have been without it, and the clip is still
// rendered afterwards.
func TestAnEditIsUndoneAfterARender(t *testing.T) {
	t.Parallel()
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	view, err := ReadPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	id := view.Clips[0].ID
	change := edited(t, filepath.Dir(plan), func() error { return SetCaptionY(plan, id, 400) })
	p.Base.Out = t.TempDir()
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if _, err := change.Undo(); err != nil {
		t.Fatalf("the edit before the render cannot be undone: %v", err)
	}
	view, err = ReadPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if c := view.Clips[0]; c.CaptionYMoved {
		t.Error("the undo left the caption where the edit put it")
	} else if c.Rendered == "" {
		t.Error("the undo took the short away from the clip")
	}
}
