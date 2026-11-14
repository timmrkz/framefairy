package main

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"

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

// isMarked says whether the folder of a video is known to carry the mark.
func isMarked(files afero.Fs, video string) bool {
	ok, err := marked(files, video)
	return ok && err == nil
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
	if !listed(svc.Library(), gone) || !isMarked(files, gone) {
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
	if err := writeChosen(files, ep, chosen{Opened: 1}); err != nil {
		t.Fatal(err)
	}
	if lib := svc.Library(); listed(lib, ep) {
		t.Fatalf("the folder written back kept %s in the library", ep)
	}
	svc.clearing.Wait()
	if there, _ := afero.DirExists(files, engine.WorkDir(ep)); there {
		t.Errorf("what was written back into %s is still there", engine.WorkDir(ep))
	}
}

// removeHeld is a disk whose RemoveAll says that it began and waits to be
// let go, so a test holds a delete in the middle.
type removeHeld struct {
	afero.Fs
	began, letGo chan struct{}
	once         sync.Once
}

func (f *removeHeld) RemoveAll(path string) error {
	f.once.Do(func() { close(f.began) })
	<-f.letGo
	return f.Fs.RemoveAll(path)
}

func TestQuittingWaitsForTheDeleteOfAVideoLetGo(t *testing.T) {
	// The app quit while it deleted what was written back into the folder
	// of a video let go, and left part of it behind, issue #507. The
	// delete is held, and quitting waits for it.
	synctest.Test(t, func(t *testing.T) {
		svc, eps := memoryLibrary(t, "youtube.mp4")
		files := svc.store.files
		ep := eps[0]
		if err := files.RemoveAll(engine.WorkDir(ep)); err != nil {
			t.Fatal(err)
		}
		if err := writeChosen(files, ep, chosen{Opened: 1}); err != nil {
			t.Fatal(err)
		}
		held := &removeHeld{Fs: files, began: make(chan struct{}), letGo: make(chan struct{})}
		svc.store.files, svc.store.machine.Files = held, held
		withJobs(svc)
		if listed(svc.Library(), ep) {
			t.Fatalf("the folder written back kept %s in the library", ep)
		}
		<-held.began
		quit := make(chan struct{})
		go func() {
			svc.stopAll()
			close(quit)
		}()
		synctest.Wait()
		select {
		case <-quit:
			t.Fatal("the app quit in the middle of deleting what was written back")
		default:
		}
		close(held.letGo)
		<-quit
		if there, _ := afero.DirExists(files, engine.WorkDir(ep)); there {
			t.Errorf("what was written back into %s is still there after the app quit", engine.WorkDir(ep))
		}
	})
}

func TestTheLibraryIsReadWhileAVideoLetGoIsCleared(t *testing.T) {
	// Reading the library waited for the whole delete of what was written
	// back into the folder of a video let go, issue #691: the delete held
	// the store's lock, and the library, its settings and its list all
	// wait for that lock. The delete is held in the middle, and the
	// library is read while it is, on the test's own goroutine, so a read
	// that waits for the delete hangs the test until go test's -timeout.
	// The same video added again meanwhile still waits for the delete, so
	// the mark it writes is not deleted with the rest.
	synctest.Test(t, func(t *testing.T) {
		svc, eps := memoryLibrary(t, "youtube.mp4", "start.mp4")
		files := svc.store.files
		gone, stays := eps[0], eps[1]
		if err := files.RemoveAll(engine.WorkDir(gone)); err != nil {
			t.Fatal(err)
		}
		if err := writeChosen(files, gone, chosen{Opened: 1}); err != nil {
			t.Fatal(err)
		}
		held := &removeHeld{Fs: files, began: make(chan struct{}), letGo: make(chan struct{})}
		svc.store.files, svc.store.machine.Files = held, held
		withJobs(svc)
		if listed(svc.Library(), gone) {
			t.Fatalf("the folder written back kept %s in the library", gone)
		}
		<-held.began
		if lib := svc.Library(); !listed(lib, stays) || listed(lib, gone) {
			t.Errorf("read while %s is cleared, the library is %v", gone, lib)
		}
		if eps := svc.store.Episodes(); !slices.Equal(eps, []string{stays}) {
			t.Errorf("read while %s is cleared, the list is %v", gone, eps)
		}
		if err := svc.store.UpdateSettings(func(s *Settings) { s.Target = 3 }); err != nil {
			t.Fatal(err)
		}
		if got := svc.store.Settings().Target; got != 3 {
			t.Errorf("settings changed while %s is cleared read back %d", gone, got)
		}
		added := make(chan error)
		go func() {
			_, err := svc.store.AddEpisodes([]string{gone})
			added <- err
		}()
		synctest.Wait()
		select {
		case err := <-added:
			t.Fatalf("%s was added again in the middle of the delete, %v", gone, err)
		default:
		}
		close(held.letGo)
		if err := <-added; err != nil {
			t.Fatal(err)
		}
		svc.clearing.Wait()
		if !listed(svc.Library(), gone) || !isMarked(files, gone) {
			t.Errorf("added again while it was cleared, %s is not in the library with its folder marked", gone)
		}
	})
}

func TestAVideoLetGoWhileTheAppQuitsIsClearedAtOnce(t *testing.T) {
	// Once quitting waits for the deletes under way, a video let go starts
	// none beside them, and is cleared before the library is read.
	svc, eps := memoryLibrary(t, "youtube.mp4")
	withJobs(svc)
	svc.stopAll()
	files := svc.store.files
	ep := eps[0]
	if err := files.RemoveAll(engine.WorkDir(ep)); err != nil {
		t.Fatal(err)
	}
	if err := writeChosen(files, ep, chosen{Opened: 1}); err != nil {
		t.Fatal(err)
	}
	if listed(svc.Library(), ep) {
		t.Fatalf("the folder written back kept %s in the library", ep)
	}
	if there, _ := afero.DirExists(files, engine.WorkDir(ep)); there {
		t.Errorf("what was written back into %s is still there once the library was read", engine.WorkDir(ep))
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
	if err := writeChosen(files, stays, chosen{Opened: 1}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{gone, engine.WorkDir(gone)} {
		if err := files.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeChosen(files, gone, chosen{Opened: 2}); err != nil {
		t.Fatal(err)
	}
	// The app starts: it looks at the disk before the window asks, the
	// look watchTheDisk takes, here without the watch, which would hear
	// the real disk under a library in memory.
	svc.reconcile(nil)
	t.Cleanup(svc.clearing.Wait)
	if opened := svc.Opened(10); slices.Contains(opened, gone) || !slices.Contains(opened, stays) {
		t.Errorf("the app opens on %v, want %s and not %s", opened, stays, gone)
	}
}

// markUnreadable is a disk on which the mark of a work folder cannot be
// looked at, while everything else reads as usual: a network share or an
// external drive waking from sleep that fails one look.
type markUnreadable struct {
	afero.Fs
	err error
}

func (f markUnreadable) Stat(name string) (os.FileInfo, error) {
	if filepath.Base(name) == addedFile {
		return nil, &os.PathError{Op: "stat", Path: name, Err: f.err}
	}
	return f.Fs.Stat(name)
}

func TestAMarkThatCannotBeLookedAtKeepsTheVideoAndItsWork(t *testing.T) {
	// One failed look at the mark of a video's folder took the video out
	// of the library and deleted its transcript, as if the folder had
	// been deleted in Finder. Only a mark that is not there means that.
	for _, c := range []struct {
		name string
		err  error
	}{
		{"an input or output error", syscall.EIO},
		{"no permission", syscall.EACCES},
		{"a drive that went away for a moment", syscall.ENXIO},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc, eps := memoryLibrary(t, "youtube.mp4")
			withJobs(svc)
			ep := eps[0]
			files := svc.store.files
			transcript := filepath.Join(engine.WorkDir(ep), "logs", "transcript.json")
			if err := afero.WriteFile(files, transcript, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			svc.store.files = markUnreadable{files, c.err}

			lib := svc.Library()
			svc.clearing.Wait()
			if !listed(lib, ep) {
				t.Errorf("with its mark not to be looked at, %s left the library", ep)
			}
			if there, _ := afero.Exists(files, transcript); !there {
				t.Errorf("with its mark not to be looked at, the transcript of %s was deleted", ep)
			}

			// Out of the library, a folder whose mark cannot be looked at
			// may be the folder of the same video added again, so it is
			// not deleted either.
			svc.store.episodes = nil
			if cleared, err := svc.store.clearLetGo(ep); cleared || err == nil {
				t.Errorf("clearing the folder of %s with its mark not to be looked at said %v, %v", ep, cleared, err)
			}
			if there, _ := afero.Exists(files, transcript); !there {
				t.Errorf("clearing the folder of %s with its mark not to be looked at deleted its transcript", ep)
			}
		})
	}
}
