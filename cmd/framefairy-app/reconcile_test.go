package main

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// The files on the disk are the truth, and others than the app write them:
// Tim in Finder, the command line, a sync service. Whatever one of them
// changed is looked at by reconcile, and what it reads decides what the
// app does. Issue #673.

// toldList is the videos the sidebar was told about, in order.
type toldList struct {
	mu     sync.Mutex
	videos []string
}

func (l *toldList) add(ep string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.videos = append(l.videos, ep)
}

func (l *toldList) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.videos)
}

// listingTo gives the app a queue whose news of the library goes to list.
func listingTo(svc *FrameFairy, list func(string)) {
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, list)
}

func TestAChangeHeardInTheFolderOfAVideoDeletedOutsideLetsItGoAtOnce(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "youtube.mp4", "start.mp4")
	var told toldList
	listingTo(svc, told.add)
	gone, stays := eps[0], eps[1]
	if err := svc.store.files.RemoveAll(engine.WorkDir(gone)); err != nil {
		t.Fatal(err)
	}
	// The hint the watch hands on as Finder deletes the folder: the
	// folder itself, in the folder the video is in.
	svc.reconcile([]string{engine.WorkDir(gone)})
	if svc.store.Known(gone) {
		t.Errorf("%s is still in the library after its folder was deleted", gone)
	}
	if !svc.store.Known(stays) {
		t.Errorf("%s left the library, though nothing of it changed", stays)
	}
	if got := told.all(); !slices.Equal(got, []string{gone}) {
		t.Errorf("the sidebar was told about %v, want only %s", got, gone)
	}
	svc.clearing.Wait()
}

func TestAVideoMovedAwayAndBackIsToldEachTimeAndOnlyThen(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "youtube.mp4", "start.mp4")
	var told toldList
	listingTo(svc, told.add)
	sent := make(chan string, 4)
	svc.news = newVideoNews(func(path string) (VideoState, error) {
		return VideoState{Video: path}, nil
	}, func(v VideoState) { sent <- v.Video })
	t.Cleanup(svc.news.stop)
	files := svc.store.files
	video, other := eps[0], eps[1]

	steps := []struct {
		name    string
		change  func() error
		missing bool
		told    []string
	}{
		{"moved away", func() error { return files.Rename(video, video+".away") }, true, []string{video}},
		{"heard again with nothing changed", func() error { return nil }, true, []string{video}},
		{"put back", func() error { return files.Rename(video+".away", video) }, false, []string{video, video}},
	}
	for _, step := range steps {
		if err := step.change(); err != nil {
			t.Fatal(err)
		}
		svc.reconcile([]string{video})
		if got := svc.Episode(video).Missing; got != step.missing {
			t.Fatalf("%s: the library says missing %v", step.name, got)
		}
		if got := told.all(); !slices.Equal(got, step.told) {
			t.Fatalf("%s: the sidebar was told about %v, want %v", step.name, got, step.told)
		}
		// Its state is sent to the interface each time it is heard of.
		if got := <-sent; got != video {
			t.Fatalf("%s: the state of %s was sent", step.name, got)
		}
	}
	if !svc.store.Known(video) {
		t.Errorf("%s left the library, though only its file moved", video)
	}
	svc.reconcile([]string{filepath.Join(filepath.Dir(other), "elsewhere.mp4")})
	if got := told.all(); len(got) != 2 {
		t.Errorf("a change to a file of no video told the sidebar about %v", got[2:])
	}
}

func TestAChangeIsHintedForTheVideoItCanChange(t *testing.T) {
	t.Parallel()
	video := filepath.Join("/home/tim/videos", "youtube.mp4")
	work := engine.WorkDir(video)
	for _, c := range []struct {
		path string
		want bool
	}{
		{video, true},
		{work, true},
		{filepath.Join(work, "logs", "clips.json"), true},
		{"/home/tim/videos", true},
		{"/home/tim", true},
		{filepath.Join("/home/tim/videos", "start.mp4"), false},
		{work + "-old", false},
		{"/home/tim/videos/you", false},
	} {
		if got := touches(video, []string{c.path}); got != c.want {
			t.Errorf("a change at %s touches %s: %v, want %v", c.path, video, got, c.want)
		}
	}
}

// On the real disk: diskLibrary is the app over a library on the real
// disk, watched, with one video in it, and where the sidebar's news of the
// library goes. The system's watch hears only the real disk.
func diskLibrary(t *testing.T) (*FrameFairy, string, chan string) {
	t.Helper()
	dir := t.TempDir()
	files := afero.NewOsFs()
	st := openStoreIn(files, filepath.Join(dir, "config"))
	video := filepath.Join(dir, "videos", "youtube.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if added, err := st.AddEpisodes([]string{video}); err != nil || len(added) != 1 {
		t.Fatalf("adding %s: %v %v", video, added, err)
	}
	svc := &FrameFairy{store: st}
	told := make(chan string, 64)
	listingTo(svc, func(ep string) { told <- ep })
	svc.watch = newWatching(svc.reconcile)
	if svc.watch == nil {
		t.Fatal("the disk cannot be watched here")
	}
	t.Cleanup(svc.clearing.Wait)
	t.Cleanup(svc.watch.stop)
	// Reading the library is what starts the watch, as it does in the app.
	svc.Library()
	return svc, video, told
}

// toldOf reads the sidebar's news until it names video.
func toldOf(told chan string, video string) {
	for ep := range told {
		if ep == video {
			return
		}
	}
}

// On the real disk: the system's own watch on the folders is what is
// tested, the way Finder's changes reach it.
func TestAFolderDeletedInFinderLeavesTheLibraryWithoutTheAppComingToTheFront(t *testing.T) {
	t.Parallel()
	svc, video, told := diskLibrary(t)
	if err := os.RemoveAll(engine.WorkDir(video)); err != nil {
		t.Fatal(err)
	}
	toldOf(told, video)
	if svc.store.Known(video) {
		t.Errorf("the sidebar was told, and %s is still in the library", video)
	}
	if _, err := os.Stat(video); err != nil {
		t.Errorf("the video itself is gone: %v", err)
	}
}

// On the real disk: the system's own watch on the folders is what is
// tested, the way Finder's changes reach it.
func TestAVideoMovedInFinderSaysFileNotFoundAndBackWithoutTheAppComingToTheFront(t *testing.T) {
	t.Parallel()
	svc, video, told := diskLibrary(t)
	if err := os.Rename(video, video+".away"); err != nil {
		t.Fatal(err)
	}
	toldOf(told, video)
	if !svc.Episode(video).Missing {
		t.Fatalf("the sidebar was told, and %s is not missing", video)
	}
	if err := os.Rename(video+".away", video); err != nil {
		t.Fatal(err)
	}
	toldOf(told, video)
	if svc.Episode(video).Missing {
		t.Errorf("the sidebar was told, and %s is still missing", video)
	}
	if !svc.store.Known(video) {
		t.Errorf("%s left the library, though only its file moved", video)
	}
}
