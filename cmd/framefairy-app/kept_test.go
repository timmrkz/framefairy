package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
// puts it back, under another name in another folder, and the video is
// not new, so a check of its picture that fails does not take the folder
// with it.
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
	added, fresh, err := s.store.addEpisodes([]string{again})
	if err != nil || len(added) != 1 {
		t.Fatalf("added %v, %v", added, err)
	}
	if fresh[again] {
		t.Error("the video added again is taken as new, so its folder would go with a failed check")
	}
	back, err := afero.ReadFile(s.store.files, filepath.Join(engine.LogsDir(again), "clips-0-1800.json"))
	if err != nil || !bytes.Equal(back, []byte(keptPlan)) {
		t.Errorf("the clip set came back as %q, %v", back, err)
	}
}

// A folder deleted in Finder is never seen going in time, so what is in
// it is kept as it is written: the watch sees a clip set written, and
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
	if _, _, err := s.store.addEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	// The delete of what was written back into the folder of the video let
	// go runs before the add or after it, and after it finds the video in
	// the library again and deletes nothing.
	s.clearing.Wait()
	if back, err := afero.ReadFile(s.store.files, plan); err != nil || !bytes.Equal(back, []byte(keptPlan)) {
		t.Errorf("the clip set came back as %q, %v", back, err)
	}
}

// Adding a video again, the delete of what was written back into its
// folder after a Finder deletion, and Remove with its files, all at once.
// The clearing once ran between the put back and the add, deleted what
// was put back, and the video started as new, its new work then kept over
// the old. However they fall, the copy is never lost, and a video in the
// library has its clip set.
func TestAddingAgainWhileTheFolderIsClearedAndRemovedLosesNothing(t *testing.T) {
	for range 20 {
		s, source := aVideoThatKeeps(t)
		if err := s.keep(source); err != nil {
			t.Fatal(err)
		}
		if err := s.store.files.RemoveAll(engine.WorkDir(source)); err != nil {
			t.Fatal(err)
		}
		s.reconcile([]string{engine.WorkDir(source)})
		s.clearing.Wait()
		// A job still stopping writes into the folder of the video let go.
		if err := afero.WriteFile(s.store.files, filepath.Join(engine.WorkDir(source), "levels.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		var all sync.WaitGroup
		all.Go(func() { _, _, _ = s.store.addEpisodes([]string{source}) })
		all.Go(func() { s.clearLetGo(source) })
		all.Go(func() { _ = s.RemoveEpisode(source, true) })
		all.Wait()
		plan := filepath.Join(engine.LogsDir(source), "clips-0-1800.json")
		if s.store.Known(source) {
			if back, err := afero.ReadFile(s.store.files, plan); err != nil || !bytes.Equal(back, []byte(keptPlan)) {
				t.Fatalf("the video is in the library and its clip set is %q, %v", back, err)
			}
			continue
		}
		if _, _, err := s.store.addEpisodes([]string{source}); err != nil {
			t.Fatal(err)
		}
		if back, err := afero.ReadFile(s.store.files, plan); err != nil || !bytes.Equal(back, []byte(keptPlan)) {
			t.Fatalf("added once more, the clip set is %q, %v", back, err)
		}
	}
}

// A video added with its folder kept has its own clips, which stay as
// they are, and a video refused for sharing a folder with one in the
// library writes nothing into it.
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
	if _, fresh, err := s.store.addEpisodes([]string{source}); err != nil || fresh[source] {
		t.Fatalf("added with fresh %v, %v", fresh, err)
	}
	if got, _ := afero.ReadFile(s.store.files, plan); !bytes.Equal(got, own) {
		t.Errorf("the video's own clip set was written over: %s", got)
	}
}

// ep.mp4 and ep.mov share one work folder, so the second is not added.
// The copy kept of the second was once put back before that was asked,
// into the folder of the first.
func TestAVideoRefusedForSharingAFolderPutsNothingBack(t *testing.T) {
	s, source := aVideoThatKeeps(t)
	twin := strings.TrimSuffix(source, ".mp4") + ".mov"
	if err := afero.WriteFile(s.store.files, twin, []byte("another video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.store.machine.Keep(twin); err != nil {
		t.Fatal(err)
	}
	// The twin's copy holds a clip set, and the video in the library has
	// none yet.
	logs := engine.LogsDir(source)
	if err := afero.WriteFile(s.store.files, filepath.Join(logs, "clips-0-1800.json"), []byte(keptPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.store.machine.Keep(twin); err != nil {
		t.Fatal(err)
	}
	if err := s.store.files.RemoveAll(logs); err != nil {
		t.Fatal(err)
	}
	if added, _, err := s.store.addEpisodes([]string{twin}); err == nil || len(added) != 0 {
		t.Fatalf("added %v, %v, want the twin refused", added, err)
	}
	if exists, _ := afero.Exists(s.store.files, filepath.Join(logs, "clips-0-1800.json")); exists {
		t.Error("the twin refused put its clip set into the folder of the video in the library")
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

// A video added again through Add, the way the user does it, has its
// transcript and clip set back once its picture is checked, and is not
// transcribed again: the transcription it had got to its end.
//
// On the real disk: the episode's decoder reads what is added.
func TestAVideoAddedAgainIsNotTranscribedAgain(t *testing.T) {
	// Not side by side: open sets HOME for the whole process.
	d := open(t)
	d.svc.store.machine.KeepIn(filepath.Join(d.home, "kept"))
	video := d.episode("folge", "12")
	if added, err := d.svc.addEpisodes([]string{video}); err != nil || len(added) != 1 {
		t.Fatalf("added %v, %v", added, err)
	}
	<-d.svc.checked(video)
	d.idle(video)
	if _, done := d.heard(video); !done {
		t.Fatalf("the video added was not transcribed to its end: %+v", d.svc.Jobs())
	}
	plan := filepath.Join(engine.LogsDir(video), "clips-0-1800.json")
	if err := afero.WriteFile(d.svc.store.files, plan, []byte(keptPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.RemoveEpisode(video, true); err != nil {
		t.Fatal(err)
	}
	again := filepath.Join(d.home, "downloads", "episode twelve.mp4")
	if err := d.svc.store.files.MkdirAll(filepath.Dir(again), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := d.svc.store.files.Rename(video, again); err != nil {
		t.Fatal(err)
	}
	if added, err := d.svc.addEpisodes([]string{again}); err != nil || len(added) != 1 {
		t.Fatalf("added again %v, %v", added, err)
	}
	<-d.svc.checked(again)
	d.idle(again)
	back, err := afero.ReadFile(d.svc.store.files, filepath.Join(engine.LogsDir(again), "clips-0-1800.json"))
	if err != nil || !bytes.Equal(back, []byte(keptPlan)) {
		t.Errorf("the clip set came back as %q, %v", back, err)
	}
	if _, done := d.heard(again); !done {
		t.Error("the transcript put back is not the whole video's")
	}
	if slices.ContainsFunc(d.svc.Jobs(), func(j Job) bool { return j.Episode == again && j.Kind == engine.JobTranscribe }) {
		t.Errorf("the video added again was transcribed again: %+v", d.svc.Jobs())
	}
}
