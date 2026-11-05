package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
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

// The state of a video named by a change is sent to the workspace each
// time, and the sidebar is told when the video itself changed, not when
// something was written in its work folder, which is the app's own work
// most of the time. videoNews keeps time, so the test runs in a bubble.
func TestAVideoMovedAwayAndBackIsSentEachTimeAndTheSidebarToldOnlyThen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		plan := filepath.Join(engine.WorkDir(video), "logs", "clips.json")

		steps := []struct {
			name    string
			change  func() error
			heard   string
			missing bool
			told    int
		}{
			{"moved away", func() error { return files.Rename(video, video+".away") }, video, true, 1},
			{"heard again with nothing changed", func() error { return nil }, video, true, 2},
			{"put back", func() error { return files.Rename(video+".away", video) }, video, false, 3},
			{"a plan written by the command line", func() error {
				return afero.WriteFile(files, plan, []byte("{}"), 0o644)
			}, plan, false, 3},
		}
		for _, step := range steps {
			if err := step.change(); err != nil {
				t.Fatal(err)
			}
			svc.reconcile([]string{step.heard})
			if got := svc.Episode(video).Missing; got != step.missing {
				t.Fatalf("%s: the library says missing %v", step.name, got)
			}
			if got := told.all(); len(got) != step.told {
				t.Fatalf("%s: the sidebar was told %d times, want %d", step.name, len(got), step.told)
			}
			// Its state is sent to the workspace each time it is heard of.
			if got := <-sent; got != video {
				t.Fatalf("%s: the state of %s was sent", step.name, got)
			}
		}
		if !svc.store.Known(video) {
			t.Errorf("%s left the library, though only its file moved", video)
		}
		svc.reconcile([]string{filepath.Join(filepath.Dir(other), "elsewhere.mp4")})
		synctest.Wait()
		if got := told.all(); len(got) != 3 {
			t.Errorf("a change to a file of no video told the sidebar about %v", got[3:])
		}
		select {
		case v := <-sent:
			t.Errorf("a change to a file of no video sent the state of %s", v)
		default:
		}
	})
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
		{"/home/tim/Shorts/youtube/eins.mp4", true},
		{"/home/tim/Shorts/youtube", true},
		{"/home/tim/Shorts/youtube-old/eins.mp4", false},
		{"/home/tim/Shorts/start/eins.mp4", false},
	} {
		shorts := []string{"/home/tim/Shorts/youtube"}
		if got := touches(video, shorts, []string{c.path}); got != c.want {
			t.Errorf("a change at %s touches %s: %v, want %v", c.path, video, got, c.want)
		}
	}
}

// On the real disk: diskLibrary is the app over a library on the real
// disk, watched, with one video in it, and where the sidebar's news of the
// library goes and the states sent to the workspace. The system's watch
// hears only the real disk. Each of prepare is given the video before the
// library is read, which starts the watch.
func diskLibrary(t *testing.T, prepare ...func(video string)) (*FrameFairy, string, chan string, chan string) {
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
	if err := os.MkdirAll(filepath.Join(engine.WorkDir(video), "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &FrameFairy{store: st}
	told := make(chan string, 64)
	listingTo(svc, func(ep string) { told <- ep })
	sent := make(chan string, 64)
	svc.news = newVideoNews(func(path string) (VideoState, error) {
		return VideoState{Video: path}, nil
	}, func(v VideoState) {
		select {
		case sent <- v.Video:
		default:
		}
	})
	t.Cleanup(svc.news.stop)
	svc.watch = newWatching(svc.reconcile)
	if svc.watch == nil {
		t.Fatal("the disk cannot be watched here")
	}
	t.Cleanup(svc.clearing.Wait)
	t.Cleanup(svc.watch.stop)
	for _, p := range prepare {
		p(video)
	}
	// Reading the library is what starts the watch, as it does in the app.
	svc.Library()
	return svc, video, told, sent
}

// newsUntil reads news of videos, the sidebar's or the workspace's, until
// one names video and holds is true. A folder deleted is heard file by
// file, so the first news of it may come before the last file is gone.
func newsUntil(news chan string, video string, holds func() bool) {
	for ep := range news {
		if ep == video && holds() {
			return
		}
	}
}

// On the real disk: the system's own watch on the folders is what is
// tested, the way Finder's changes reach it.
func TestAFolderDeletedInFinderLeavesTheLibraryWithoutTheAppComingToTheFront(t *testing.T) {
	t.Parallel()
	svc, video, told, _ := diskLibrary(t)
	if err := os.RemoveAll(engine.WorkDir(video)); err != nil {
		t.Fatal(err)
	}
	newsUntil(told, video, func() bool { return !svc.store.Known(video) })
	if _, err := os.Stat(video); err != nil {
		t.Errorf("the video itself is gone: %v", err)
	}
}

// On the real disk: the system's own watch on the folders is what is
// tested, the way Finder's changes reach it.
func TestAVideoMovedInFinderSaysFileNotFoundAndBackWithoutTheAppComingToTheFront(t *testing.T) {
	t.Parallel()
	svc, video, told, _ := diskLibrary(t)
	if err := os.Rename(video, video+".away"); err != nil {
		t.Fatal(err)
	}
	newsUntil(told, video, func() bool { return svc.Episode(video).Missing })
	if err := os.Rename(video+".away", video); err != nil {
		t.Fatal(err)
	}
	newsUntil(told, video, func() bool { return !svc.Episode(video).Missing })
	if !svc.store.Known(video) {
		t.Errorf("%s left the library, though only its file moved", video)
	}
}

// On the real disk: the system's own watch on the logs folder is what is
// tested, the way the command line's writes reach it.
func TestAPlanWrittenByTheCommandLineIsSentToTheWorkspaceAtOnce(t *testing.T) {
	t.Parallel()
	_, video, _, sent := diskLibrary(t)
	plan := filepath.Join(engine.WorkDir(video), "logs", "clips.json")
	if err := os.WriteFile(plan, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	newsUntil(sent, video, func() bool { return true })
}

// On the real disk: the system's own watch on the folder a short was
// rendered into is what is tested, the way Finder's changes reach it.
//
// A short deleted in Finder is gone from its clip at once: the workspace
// is sent the video's state, and the clip has no short, so Show in folder
// goes. The plan still says the clip was rendered.
func TestAShortDeletedInFinderIsGoneFromItsClipAtOnce(t *testing.T) {
	t.Parallel()
	var plan, short string
	svc, video, _, sent := diskLibrary(t, func(video string) {
		shorts := filepath.Join(filepath.Dir(filepath.Dir(video)), "Shorts", "youtube")
		if err := os.MkdirAll(shorts, 0o755); err != nil {
			t.Fatal(err)
		}
		short = filepath.Join(shorts, "eins.mp4")
		if err := os.WriteFile(short, []byte("a short"), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(short)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(historyPlan), &body); err != nil {
			t.Fatal(err)
		}
		body["shorts"] = map[string]any{"01": map[string]any{
			"folder": shorts, "name": "eins.mp4", "size": info.Size(), "modified": info.ModTime().UnixNano()}}
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		plan = filepath.Join(engine.LogsDir(video), "clips.json")
		if err := os.WriteFile(plan, data, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	shortOf := func() string {
		v, err := svc.store.machine.Episode(video).ReadPlan(plan)
		if err != nil || len(v.Clips) == 0 {
			t.Fatalf("the plan read %v", err)
		}
		return v.Clips[0].Short
	}
	// The engine names a short by where its folder leads. On a Mac the
	// temporary folder is under /var, which leads to /private/var.
	if got, want := shortOf(), fileio.ResolvePath(short); got != want {
		t.Fatalf("the clip's short is %q before, want %s", got, want)
	}
	if err := os.Remove(short); err != nil {
		t.Fatal(err)
	}
	newsUntil(sent, video, func() bool { return shortOf() == "" })
	if v, _ := svc.store.machine.Episode(video).ReadPlan(plan); !v.Clips[0].Rendered {
		t.Error("the clip whose short was deleted no longer says it was rendered")
	}
}
