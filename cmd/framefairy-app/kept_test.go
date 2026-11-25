package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// keptPlan is a clip set with one clip, which is what the user wants back
// with a video added again.
const keptPlan = `{"source":"ep.mp4","clips":[{"id":"01","slug":"a","segments":[{"start":1,"end":2}]}]}`

// aVideoThatKeeps is a video in a library in memory with one clip set,
// on a machine that keeps its copies in /kept.
func aVideoThatKeeps(t *testing.T) (*FrameFairy, string) {
	t.Helper()
	s, source, _ := anEpisodeWithWork(t)
	if err := afero.WriteFile(s.store.files, filepath.Join(engine.LogsDir(source), "clips-0-1800.json"),
		[]byte(keptPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	s.store.machine.KeepIn("/kept")
	return s, source
}

// The user removed a video with its files and added it again later: its
// clips were gone. Now what Remove deletes was kept first, and the add
// puts it back, under another name in another folder.
func TestAVideoRemovedWithItsFilesGetsItsClipsBackWhenAddedAgain(t *testing.T) {
	s, source := aVideoThatKeeps(t)
	if err := s.RemoveEpisode(source, true); err != nil {
		t.Fatal(err)
	}
	again := "/home/tim/downloads/episode.mp4"
	if err := s.store.files.MkdirAll(filepath.Dir(again), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.store.files.Rename(source, again); err != nil {
		t.Fatal(err)
	}
	s.putBack([]string{again})
	back, err := afero.ReadFile(s.store.files, filepath.Join(engine.LogsDir(again), "clips-0-1800.json"))
	if err != nil || !bytes.Equal(back, []byte(keptPlan)) {
		t.Errorf("the clip set came back as %q, %v", back, err)
	}
}

// A folder deleted in Finder is never seen going in time, so what is in
// it is kept as it is written: the watch hears a clip set written, and
// the copy follows it.
func TestAClipSetWrittenIsKeptBeforeTheFolderGoesInFinder(t *testing.T) {
	s, source := aVideoThatKeeps(t)
	plan := filepath.Join(engine.LogsDir(source), "clips-0-1800.json")
	s.reconcile([]string{plan})
	if err := s.store.files.RemoveAll(engine.WorkDir(source)); err != nil {
		t.Fatal(err)
	}
	s.reconcile([]string{engine.WorkDir(source)})
	if s.store.Known(source) {
		t.Fatal("the video whose folder was deleted is still in the library")
	}
	// What was written back into the folder of a video let go is cleared
	// out of the way, and the video is added again after that.
	s.clearing.Wait()
	s.putBack([]string{source})
	if back, err := afero.ReadFile(s.store.files, plan); err != nil || !bytes.Equal(back, []byte(keptPlan)) {
		t.Errorf("the clip set came back as %q, %v", back, err)
	}
}

// A video added again with its folder kept has its own clips, which stay
// as they are.
func TestAVideoAddedWithItsFolderKeepsItsOwnClips(t *testing.T) {
	s, source := aVideoThatKeeps(t)
	if err := s.keep(source); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(engine.LogsDir(source), "clips-0-1800.json")
	own := []byte(`{"source":"ep.mp4","clips":[]}`)
	if err := afero.WriteFile(s.store.files, plan, own, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveEpisode(source, false); err != nil {
		t.Fatal(err)
	}
	s.putBack([]string{source})
	if got, _ := afero.ReadFile(s.store.files, plan); !bytes.Equal(got, own) {
		t.Errorf("the video's own clip set was written over: %s", got)
	}
}

func TestOnlyTheTranscriptsFilesAreTheTranscript(t *testing.T) {
	video := "/v/ep.mp4"
	logs := engine.LogsDir(video)
	for path, want := range map[string]bool{
		filepath.Join(logs, "words.json"):          true,
		filepath.Join(logs, "words.frames"):        true,
		filepath.Join(logs, "levels.json"):         false,
		filepath.Join(logs, "clips.json"):          false,
		filepath.Join("/v", "other", "words.json"): false,
	} {
		if got := writesTranscript(video, []string{path}); got != want {
			t.Errorf("writesTranscript(%s) = %v, want %v", path, got, want)
		}
	}
}
