package main

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// Tim deleted a video's folder in Finder to remove the video, and it
// stayed in the sidebar with nothing in it, and opening it wrote a folder
// back. Its folder is everything the app has of a video, so a video whose
// folder is gone leaves the library. Plan row 2.247.

func listed(lib []engine.EpisodeStatus, path string) bool {
	return slices.ContainsFunc(lib, func(e engine.EpisodeStatus) bool { return e.Source == path })
}

// withJobs gives the app a queue, which letting a video go stops the work
// of.
func withJobs(svc *FrameFairy) *FrameFairy {
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	return svc
}

func TestAVideoWhoseFolderIsDeletedLeavesTheLibrary(t *testing.T) {
	svc, eps := memoryLibrary(t, "youtube.mp4", "start.mp4")
	withJobs(svc)
	files := svc.store.files
	gone, stays := eps[0], eps[1]
	if lib := svc.Library(); !listed(lib, gone) || !listed(lib, stays) {
		t.Fatalf("the library before is %v", lib)
	}
	if err := files.RemoveAll(engine.WorkDir(gone)); err != nil {
		t.Fatal(err)
	}
	lib := svc.Library()
	if listed(lib, gone) || !listed(lib, stays) {
		t.Fatalf("with the folder of %s deleted, the library is %v", gone, lib)
	}
	if svc.store.Known(gone) {
		t.Errorf("%s is still known to the store", gone)
	}
	if again := openStoreIn(files, svc.store.dir).Episodes(); slices.Contains(again, gone) {
		t.Errorf("the library read again still has %s: %v", gone, again)
	}
	if ok, _ := afero.Exists(files, gone); !ok {
		t.Errorf("the video %s itself was deleted", gone)
	}
	// Added again, it is a video like any other, with its folder marked.
	if added, err := svc.store.AddEpisodes([]string{gone}); err != nil || len(added) != 1 {
		t.Fatalf("adding it again: %v %v", added, err)
	}
	if !listed(svc.Library(), gone) || !marked(files, gone) {
		t.Errorf("added again, %s is not in the library with its folder marked", gone)
	}
}

func TestAVideoNotThereKeepsItsPlaceWhateverItsFolder(t *testing.T) {
	// A drive taken out takes the video and its folder together, and the
	// video says File not found until it is back.
	svc, eps := memoryLibrary(t, "youtube.mp4")
	files := svc.store.files
	if err := files.RemoveAll(filepath.Dir(eps[0])); err != nil {
		t.Fatal(err)
	}
	lib := svc.Library()
	if len(lib) != 1 || !lib[0].Missing {
		t.Errorf("with the video and its folder gone, the library is %+v", lib)
	}
}

func TestAFolderWrittenBackAfterItWasDeletedIsLetGoAndCleared(t *testing.T) {
	// Opening the video wrote the chosen clip and the waveform back, so the
	// folder was there again with nothing of its own in it.
	svc, eps := memoryLibrary(t, "youtube.mp4")
	withJobs(svc)
	files := svc.store.files
	ep := eps[0]
	if err := files.RemoveAll(engine.WorkDir(ep)); err != nil {
		t.Fatal(err)
	}
	if err := changeChosen(files, ep, func(c *chosen) { c.Opened = 1 }); err != nil {
		t.Fatal(err)
	}
	if lib := svc.Library(); listed(lib, ep) {
		t.Fatalf("the folder written back kept %s in the library", ep)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		there, _ := afero.DirExists(files, engine.WorkDir(ep))
		if !there {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("what was written back into %s is still there", engine.WorkDir(ep))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestVideosAddedBeforeTheMarkAreMarkedOnceByTheirWork(t *testing.T) {
	files := afero.NewMemMapFs()
	config := "/home/tim/config/FrameFairy"
	worked := "/home/tim/videos/start.mp4"
	remnant := "/home/tim/videos/youtube.mp4"
	for _, v := range []string{worked, remnant} {
		if err := afero.WriteFile(files, v, []byte("not really a video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string) {
		t.Helper()
		if err := afero.WriteFile(files, path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// One with a transcript, the way every video added before the mark
	// has, and one with only what opening it writes back.
	write(filepath.Join(engine.LogsDir(worked), engine.TranscriptName(nil)))
	write(filepath.Join(engine.WorkDir(worked), chosenFile))
	write(filepath.Join(engine.WorkDir(remnant), chosenFile))
	write(filepath.Join(engine.LogsDir(remnant), "levels.json"))
	write(filepath.Join(engine.LogsDir(remnant), "levels.frames"))
	write(filepath.Join(config, "library.json"))
	if err := afero.WriteFile(files, filepath.Join(config, "library.json"),
		[]byte(`["`+worked+`","`+remnant+`"]`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := withJobs(&FrameFairy{store: openStoreIn(files, config)})
	if !marked(files, worked) || marked(files, remnant) {
		t.Fatalf("marked: %s %v, %s %v", worked, marked(files, worked), remnant, marked(files, remnant))
	}
	lib := svc.Library()
	if !listed(lib, worked) || listed(lib, remnant) {
		t.Errorf("the library is %v, want %s and not %s", lib, worked, remnant)
	}
}

func TestAVideoDeletedWithItsFolderLeavesTheLibrary(t *testing.T) {
	// Tim deleted two videos from Documents with their folders, and they
	// stayed with a red lamp, File not found. The folder they were in is
	// still there, so they were deleted, not taken away on a drive.
	svc, eps := memoryLibrary(t, "PXL_1.mp4", "PXL_2.mp4", "start.mp4")
	withJobs(svc)
	files := svc.store.files
	both, videoOnly := eps[0], eps[1]
	for _, p := range []string{both, engine.WorkDir(both), videoOnly} {
		if err := files.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
	lib := svc.Library()
	if listed(lib, both) {
		t.Errorf("%s, deleted with its folder, is still in the library", both)
	}
	// A video moved away whose work is still beside where it was can be
	// moved back, so it stays and says File not found.
	if !listed(lib, videoOnly) || !listed(lib, eps[2]) {
		t.Errorf("the library is %v, want %s and %s still in it", lib, videoOnly, eps[2])
	}
}

func TestTheAppNeverOpensOnAVideoDeletedOutsideIt(t *testing.T) {
	// Tim deleted a video with its folder while it was open, the build
	// before #218 wrote the chosen clip back, and after the update the app
	// opened on it, asking the Go side about a video it had just let go.
	svc, eps := memoryLibrary(t, "PXL_1.mp4", "start.mp4")
	withJobs(svc)
	files := svc.store.files
	gone, stays := eps[0], eps[1]
	if err := changeChosen(files, stays, func(c *chosen) { c.Opened = 1 }); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{gone, engine.WorkDir(gone)} {
		if err := files.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := changeChosen(files, gone, func(c *chosen) { c.Opened = 2 }); err != nil {
		t.Fatal(err)
	}
	if opened := svc.Opened(10); slices.Contains(opened, gone) || !slices.Contains(opened, stays) {
		t.Errorf("the app opens on %v, want %s and not %s", opened, stays, gone)
	}
}
