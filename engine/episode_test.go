package engine

import (
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

	// A search that was asked for keeps its record, and its timings once
	// it is done, and either is enough.
	if err := WriteJob(source, JobRecord{ID: SearchID, Kind: JobSearch, Step: StepWaiting}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveJob(source, SearchID); err != nil {
		t.Fatal(err)
	}
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
