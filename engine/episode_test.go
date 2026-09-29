package engine

import (
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

// A short still being written is not a short.
func TestAShortBeingWrittenIsNotCounted(t *testing.T) {
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(WorkDir(source), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"01_done.mp4", "02_half.part.mp4", "03_upper.PART.MP4"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := Status(source, "").Rendered; got != 1 {
		t.Errorf("%d rendered, want 1", got)
	}
}
