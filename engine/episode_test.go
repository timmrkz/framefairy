package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// The app searches by itself only for an episode nobody has ever searched.
// The note that says so lives with the episode, so it survives a restart,
// it is not brought back by removing the clips again, and deleting the work
// folder makes the episode new.
func TestAnEpisodeRemembersItHasBeenSearched(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if Looked(source) {
		t.Fatal("an episode nobody has searched should not say it has been")
	}
	if st := Status(source, ""); st.Looked || st.Work {
		t.Errorf("status of a fresh episode %+v", st)
	}

	if err := MarkLooked(source); err != nil {
		t.Fatal(err)
	}
	if !Looked(source) {
		t.Fatal("the note should be there after a search was asked for")
	}
	if st := Status(source, ""); !st.Looked || !st.Work {
		t.Errorf("status after a search %+v", st)
	}

	// Twice is the same as once, and nothing of the episode is disturbed.
	if err := MarkLooked(source); err != nil {
		t.Fatal(err)
	}
	if !Looked(source) {
		t.Fatal("the note should still be there")
	}

	if err := DeleteWork(source); err != nil {
		t.Fatal(err)
	}
	if Looked(source) {
		t.Error("an episode whose work folder is gone is new again")
	}
}
