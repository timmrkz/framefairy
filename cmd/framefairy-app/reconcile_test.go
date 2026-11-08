package main

import (
	"context"
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
	"framefairy/engine/runlog"
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
		{"/home/tim/Shorts/youtube/eins.part.mp4", false},
		{"/home/tim/Shorts/youtube/zwei.mp4", false},
		{"/home/tim/Shorts/youtube", false},
		{"/home/tim/Shorts/start/eins.mp4", false},
	} {
		shorts := []string{"/home/tim/Shorts/youtube/eins.mp4"}
		if got := touches(video, []string{c.path}) || touchesShort(shorts, []string{c.path}); got != c.want {
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
	svc.watch = newWatching(svc.reconcile, "")
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

// On the real disk: the system's own watch on a short is what is tested,
// the way Finder's changes reach it.
//
// A short deleted in Finder is gone from its clip at once: the workspace
// is sent the video's state, and the clip has no short, so Show in folder
// goes. The plan still says the clip was rendered.
func TestAShortDeletedInFinderIsGoneFromItsClipAtOnce(t *testing.T) {
	t.Parallel()
	var plan, short string
	svc, video, _, sent := diskLibrary(t, func(video string) {
		plan, short = renderedShort(t, video)
	})
	shortGoes(t, svc, video, sent, plan, short)
}

// On the real disk: the system's own watch on a short is what is tested,
// the way Finder's changes reach it.
//
// A short rendered while the app is open is watched from the moment its
// plan says so, so deleting it in Finder is heard at once as well.
func TestAShortRenderedWhileTheAppIsOpenIsWatched(t *testing.T) {
	t.Parallel()
	svc, video, _, sent := diskLibrary(t)
	plan, short := renderedShort(t, video)
	// The plan written is heard, and the watch follows it before the
	// reconcile that sent the news lets go. The news is read until a
	// reconcile has followed it, since what came before the plan, its
	// temporary file, can be heard on its own.
	watched := func() bool {
		svc.reconciling.Lock()
		defer svc.reconciling.Unlock()
		return slices.Contains(svc.watch.watchList(), fileio.ResolvePath(short))
	}
	newsUntil(sent, video, watched)
	shortGoes(t, svc, video, sent, plan, short)
}

// On the real disk: the system's own watch on a short is what is tested,
// the way a render's file put in place reaches it.
//
// A short rendered again is put in place over the one watched, and the
// watcher lets go of the file that was there. The watch watches the new
// one from the reconcile that hears the old one go, so deleting it in
// Finder is heard at once too.
func TestAShortRenderedAgainIsWatchedAgain(t *testing.T) {
	t.Parallel()
	var plan, short string
	svc, video, _, sent := diskLibrary(t, func(video string) {
		plan, short = renderedShort(t, video)
	})
	// A watch of its own, which says when a reconcile heard the short.
	heard := make(chan struct{}, 64)
	svc.watch.stop()
	svc.watch = newWatching(func(changed []string) {
		svc.reconcile(changed)
		if slices.Contains(changed, fileio.ResolvePath(short)) {
			heard <- struct{}{}
		}
	}, "")
	t.Cleanup(svc.watch.stop)
	svc.Library()

	// Rendered again: written beside it and put in place whole, as old as
	// the one the plan says, so the plan still names it.
	info, err := os.Stat(short)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(short+".new", []byte("a short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(short+".new", info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(short+".new", short); err != nil {
		t.Fatal(err)
	}
	<-heard
	svc.reconciling.Lock()
	watched := slices.Contains(svc.watch.watchList(), fileio.ResolvePath(short))
	svc.reconciling.Unlock()
	if !watched {
		t.Fatal("the short rendered again is not watched")
	}
	shortGoes(t, svc, video, sent, plan, short)
}

// On the real disk: the short is what the system's watch hears, see the
// tests above.
//
// renderedShort writes a short of the first clip of video's plan and the
// plan that says it was rendered, the way a render does, and gives both.
func renderedShort(t *testing.T, video string) (plan, short string) {
	t.Helper()
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
	// Put in place whole, the way editPlan writes a plan.
	plan = filepath.Join(engine.LogsDir(video), "clips.json")
	if err := fileio.WriteAtomic(afero.NewOsFs(), plan, data); err != nil {
		t.Fatal(err)
	}
	return plan, short
}

// On the real disk: the short is what the system's watch hears, see the
// tests above.
//
// shortGoes deletes short and hears the workspace told that the first
// clip of plan has no short any more, though it was rendered.
func shortGoes(t *testing.T, svc *FrameFairy, video string, sent chan string, plan, short string) {
	t.Helper()
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

// On the real disk: the system's own watch on the models folder is what is
// tested, the way Finder's changes reach it. It sets HOME, which the whole
// process shares, so it runs on its own.
//
// A model deleted in Finder is gone from the setup and the settings at
// once, and so is the whole models folder. Made again and a model put back
// in it, it is there again at once.
func TestAModelDeletedOrAddedOutsideTheAppIsSentToTheWindowAtOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	models := engine.ModelsDir()
	model := engine.LanguageModels()[0]
	put := func() {
		t.Helper()
		// Moved into place whole, the way a download lands.
		part := filepath.Join(models, model.Name+".part")
		if err := os.WriteFile(part, []byte("GGUF a model"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(part, filepath.Join(models, model.Name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	put()

	svc := withJobs(&FrameFairy{store: openStoreIn(afero.NewOsFs(), filepath.Join(home, "config"))})
	sent := make(chan SetupState, 64)
	svc.setupNews = func(state SetupState) { sent <- state }
	svc.watchTheDisk()
	t.Cleanup(svc.watch.stop)
	installed := func(state SetupState) bool {
		i := slices.IndexFunc(state.Language, func(m LanguageModelView) bool { return m.Name == model.Name })
		return i >= 0 && state.Language[i].Installed
	}
	until := func(holds func(SetupState) bool) {
		t.Helper()
		for state := range sent {
			if holds(state) {
				return
			}
		}
	}
	if !installed(svc.Setup(t.Context())) {
		t.Fatalf("%s is not installed before", model.Name)
	}

	if err := os.Remove(filepath.Join(models, model.Name)); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool { return !installed(s) })

	put()
	until(installed)

	// The folder deleted and put back whole, the way a backup puts it
	// back, is watched again, before the reconcile that sent its news
	// lets go.
	if err := os.RemoveAll(models); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool { return !installed(s) })
	back := filepath.Join(home, "back")
	if err := os.MkdirAll(back, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(back, model.Name), []byte("GGUF a model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(back, models); err != nil {
		t.Fatal(err)
	}
	until(installed)
	if err := os.Remove(filepath.Join(models, model.Name)); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool { return !installed(s) })

	// A model copied in by hand is made empty and written after, and it
	// is a model once it is written. Another model put in place whole
	// after it is made says the empty one was looked at, so what follows
	// is only the write.
	if err := os.WriteFile(filepath.Join(models, model.Name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	other := engine.LanguageModels()[1]
	if err := os.WriteFile(filepath.Join(home, other.Name), []byte("GGUF another model"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(home, other.Name), filepath.Join(models, other.Name)); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool {
		i := slices.IndexFunc(s.Language, func(m LanguageModelView) bool { return m.Name == other.Name })
		return i >= 0 && s.Language[i].Installed
	})
	copied, err := os.OpenFile(filepath.Join(models, model.Name), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := copied.WriteString("GGUF a model"); err != nil {
		t.Fatal(err)
	}
	if err := copied.Close(); err != nil {
		t.Fatal(err)
	}
	until(installed)

	// A speech model is a folder, and one copied in by hand is made
	// before its tokens.txt is written into it. The other model deleted
	// after says the folder was looked at, and is watched.
	speech := engine.SpeechModels()[0]
	if err := os.Mkdir(filepath.Join(models, speech.Name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(models, other.Name)); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool {
		return !slices.ContainsFunc(s.Language, func(m LanguageModelView) bool { return m.Name == other.Name && m.Installed })
	})
	if err := os.WriteFile(filepath.Join(models, speech.Name, "tokens.txt"), []byte("the model hears\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	until(func(s SetupState) bool {
		return slices.ContainsFunc(s.Speech, func(m SpeechModelView) bool { return m.Name == speech.Name && m.Installed })
	})
}

// On a machine the app never ran on there is no models folder and no
// folder over it to watch, and the first run's download makes both. A
// model it downloaded and Finder deleted later is gone from the settings
// at once too.
//
// On the real disk: the watch hears the models folder the app looks in, in
// a home of the test's own. It sets HOME for the whole process, so it is
// not parallel.
func TestAModelTheFirstRunDownloadedIsWatched(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	models := engine.ModelsDir()
	model := engine.LanguageModels()[0]

	svc := &FrameFairy{store: openStoreIn(afero.NewOsFs(), filepath.Join(home, "config"))}
	svc.installLanguage = func(_ *engine.Machine, _ context.Context, _ *runlog.Log, m engine.LanguageModel, dir string) error {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, m.Name), []byte("GGUF a model"), 0o644)
	}
	ended := make(chan Job, 8)
	svc.jobs = newQueue(svc.store, func(u JobUpdate) {
		if u.Job.Kind == "llm" && u.Job.State != JobQueued && u.Job.State != JobRunning {
			ended <- u.Job
		}
	}, func(string) {})
	sent := make(chan SetupState, 64)
	svc.setupNews = func(state SetupState) { sent <- state }
	svc.watchTheDisk()
	t.Cleanup(svc.watch.stop)
	if _, err := os.Stat(filepath.Dir(models)); !os.IsNotExist(err) {
		t.Fatalf("the folder over the models is there before the first run: %v", err)
	}

	svc.InstallLanguageModel(model.Name)
	if job := <-ended; job.State != JobDone {
		t.Fatalf("the download ended %s: %s", job.State, job.Error)
	}
	if err := os.Remove(filepath.Join(models, model.Name)); err != nil {
		t.Fatal(err)
	}
	// Hangs, until go test ends it, if the deletion is never heard.
	for state := range sent {
		i := slices.IndexFunc(state.Language, func(m LanguageModelView) bool { return m.Name == model.Name })
		if i >= 0 && !state.Language[i].Installed {
			return
		}
	}
}
