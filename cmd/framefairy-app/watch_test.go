package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// heldWatcher stands in for one of the system's watchers. It keeps the
// folders it was asked to watch and every call it was asked, and what was
// asked of it once it was closed, which the system's own may answer by
// closing a descriptor it already closed.
type heldWatcher struct {
	mu        sync.Mutex
	watching  []string
	asked     []string
	afterward []string
	closed    bool
	// refuses is what Add answers for a folder it will not watch.
	refuses map[string]error
	ev      chan fsnotify.Event
	errs    chan error
}

func newHeldWatcher(watching ...string) *heldWatcher {
	return &heldWatcher{watching: watching, ev: make(chan fsnotify.Event), errs: make(chan error)}
}

func (w *heldWatcher) ask(call string) {
	w.asked = append(w.asked, call)
	if w.closed {
		w.afterward = append(w.afterward, call)
	}
}

func (w *heldWatcher) Add(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ask("add " + name)
	if err := w.refuses[name]; err != nil {
		return err
	}
	if !slices.Contains(w.watching, name) {
		w.watching = append(w.watching, name)
	}
	return nil
}

func (w *heldWatcher) Remove(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ask("remove " + name)
	w.watching = slices.DeleteFunc(w.watching, func(d string) bool { return d == name })
	return nil
}

func (w *heldWatcher) WatchList() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ask("list")
	return slices.Clone(w.watching)
}

func (w *heldWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.ev)
		close(w.errs)
	}
	return nil
}

func (w *heldWatcher) events() <-chan fsnotify.Event { return w.ev }
func (w *heldWatcher) errors() <-chan error          { return w.errs }

// calls is the folders the watcher was asked to add, with verb "add",
// or to take off, with "remove".
func (w *heldWatcher) calls(verb string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var dirs []string
	for _, call := range w.asked {
		if dir, ok := strings.CutPrefix(call, verb+" "); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// A folder the watch watches already is never added again, and one that
// is gone is left to its watcher, which lets it go by itself. On macOS
// fsnotify adds a watched folder again with the descriptor it had for it,
// and when the folder was deleted in the same moment it closed that
// descriptor a second time, which closed a file the test had open, issue
// #722. Only a folder not watched and there is added, and only one
// watched, there and no longer wanted is taken off.
func TestTheWatchAddsOnlyWhatItDoesNotWatchAndTakesOffOnlyWhatIsThere(t *testing.T) {
	t.Parallel()
	files := afero.NewMemMapFs()
	video := filepath.Join("/home", "tim", "Movies", "episode.mp4")
	work := engine.WorkDir(video)
	logs := filepath.Join(work, "logs")
	gone := filepath.Join("/home", "tim", "Old", "old.framefairy")
	left := filepath.Join("/home", "tim", "Kept", "kept.framefairy")
	for _, dir := range []string{logs, left} {
		if err := files.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The video's folder and its work folder are watched, so are a work
	// folder deleted and one of a video no longer in the library.
	held := [2]*heldWatcher{newHeldWatcher(), newHeldWatcher()}
	ws := watchingWith(func([]string) {}, "", [2]watcher{held[0], held[1]})
	t.Cleanup(ws.stop)
	for _, dir := range []string{filepath.Dir(video), work, gone, left} {
		w := ws.watcherOf(dir).(*heldWatcher)
		w.watching = append(w.watching, dir)
	}

	ws.follow(files, []string{video}, func(string) []string { return nil })

	var adds, removes []string
	for _, w := range held {
		adds = append(adds, w.calls("add")...)
		removes = append(removes, w.calls("remove")...)
	}
	if !slices.Equal(adds, []string{logs}) {
		t.Errorf("added %q, want only the folder not watched yet, %s", adds, logs)
	}
	if !slices.Equal(removes, []string{left}) {
		t.Errorf("took off %q, want only the folder still there and not wanted, %s", removes, left)
	}
}

// Nothing asks a watcher anything once the watch is stopped: a follow
// after the stop does nothing, and one under way when the watch stops
// ends before the watchers close. On macOS a watcher asked to add a
// folder while it closed closed the folder's descriptor twice, which
// closed the folder a test was removing, issue #722. Run from several
// goroutines at once, so the race detector sees them together.
func TestNothingAsksAWatcherAnythingOnceTheWatchIsStopped(t *testing.T) {
	t.Parallel()
	files := afero.NewMemMapFs()
	video := filepath.Join("/home", "tim", "Movies", "episode.mp4")
	if err := files.MkdirAll(filepath.Join(engine.WorkDir(video), "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	shorts := func(string) []string { return nil }
	held := [2]*heldWatcher{newHeldWatcher(), newHeldWatcher()}
	ws := watchingWith(func([]string) {}, "", [2]watcher{held[0], held[1]})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { ws.follow(files, []string{video}, shorts) })
	}
	wg.Go(ws.stop)
	wg.Wait()
	ws.follow(files, []string{video}, shorts)

	for i, w := range held {
		if !w.closed {
			t.Errorf("watcher %d is not closed", i)
		}
		if len(w.afterward) > 0 {
			t.Errorf("watcher %d was asked %q after it closed", i, w.afterward)
		}
	}
}

// Which paths in and over the models folder are the app's own download
// at work, left unheard, and which can change the models the setup shows.
func TestWhatTheWatchHearsOfTheModels(t *testing.T) {
	t.Parallel()
	over := filepath.Join("/home", "tim", ".framefairy")
	models := filepath.Join(over, "models")
	ws := &watching{models: models}
	speech := filepath.Join(models, engine.SpeechModels()[0].Name)
	for _, c := range []struct {
		path                        string
		downloading, models, folder bool
	}{
		{models, false, true, true},
		{over, false, false, false},
		{filepath.Join(over, "speed.json"), false, false, false},
		{filepath.Join(over, "training"), false, false, false},
		{filepath.Join(models, "gemma.gguf"), false, true, false},
		{filepath.Join(models, "gemma.gguf.part"), true, true, false},
		{filepath.Join(models, "parakeet.unpacking"), true, true, false},
		{filepath.Join(models, "parakeet", "tokens.txt"), false, true, false},
		{filepath.Join(models, "parakeet", "x.part"), false, true, false},
		{filepath.Join(over, "models2", "gemma.gguf"), false, false, false},
		{filepath.Join(over, "models2"), false, false, false},
	} {
		heard, folder := ws.modelsIn([]string{c.path})
		if got := ws.downloading(c.path); got != c.downloading || heard != c.models || folder != c.folder {
			t.Errorf("%s: downloading %v, models %v, folder %v, want %v, %v, %v",
				c.path, got, heard, folder, c.downloading, c.models, c.folder)
		}
	}
	if _, folder := ws.modelsIn([]string{speech}); !folder {
		t.Errorf("the folder of a speech model is not followed: %s", speech)
	}
	var none *watching
	if m, f := none.modelsIn([]string{models}); m || f || none.downloading(models+"/x.part") {
		t.Error("no watch hears the models")
	}
}

// No watcher watches a folder and the folder it is in: on macOS the
// watcher of a folder sets a watch of its own on every folder made in it,
// in the same moment the app adds that folder, and when it came last the
// app heard nothing written in the new folder, issue #722. So every
// folder the app watches, for a video and for the models, is in the other
// watcher from the folder it is in.
func TestAFolderIsNeverWatchedByTheWatcherOfTheFolderItIsIn(t *testing.T) {
	t.Parallel()
	ws := newWatching(func([]string) {}, filepath.Join("/home", "tim", ".framefairy", "models"))
	if ws == nil {
		t.Skip("this system watches nothing")
	}
	t.Cleanup(ws.stop)
	video := filepath.Join("/home", "tim", "Movies", "episode.mp4")
	work := engine.WorkDir(video)
	folders := append(watchedOf(video, []string{filepath.Join(work, "shorts", "clip-1.mp4")}), ws.modelFolders()...)
	if ws.byDepth[0] == ws.byDepth[1] {
		t.Fatal("both depths have the same watcher")
	}
	for _, dir := range folders {
		if ws.watcherOf(dir) == ws.watcherOf(filepath.Dir(dir)) {
			t.Errorf("%s is watched by the watcher of %s, the folder it is in", dir, filepath.Dir(dir))
		}
	}
}

// A folder a watcher will not take is said in the log once, not at every
// look at the library, and said again only after it was watched once
// more. On Tim's Mac a folder macOS keeps to itself was said 139 times.
func TestAFolderThatCannotBeWatchedIsSaidOnce(t *testing.T) {
	_, read := testLog(t) // the log is the program's one, so not parallel
	files := afero.NewMemMapFs()
	video := filepath.Join("/home", "tim", "Movies", "episode.mp4")
	logs := filepath.Join(engine.WorkDir(video), "logs")
	if err := files.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	held := [2]*heldWatcher{newHeldWatcher(), newHeldWatcher()}
	ws := watchingWith(func([]string) {}, "", [2]watcher{held[0], held[1]})
	t.Cleanup(ws.stop)
	folder := filepath.Dir(video)
	refusing := ws.watcherOf(folder).(*heldWatcher)
	refusing.refuses = map[string]error{folder: errors.New("operation not permitted")}
	said := func() int {
		n := 0
		for _, line := range read() {
			if line["msg"] == "a folder of a video cannot be watched, so a change in it shows once the app comes back to the front" && line["folder"] == folder {
				n++
			}
		}
		return n
	}

	for range 3 {
		ws.follow(files, []string{video}, func(string) []string { return nil })
	}
	if n := said(); n != 1 {
		t.Fatalf("the folder that cannot be watched is said %d times after three looks, want once", n)
	}

	refusing.mu.Lock()
	refusing.refuses = nil
	refusing.mu.Unlock()
	ws.follow(files, []string{video}, func(string) []string { return nil })
	refusing.mu.Lock()
	refusing.refuses = map[string]error{folder: errors.New("operation not permitted")}
	refusing.watching = nil
	refusing.mu.Unlock()
	ws.follow(files, []string{video}, func(string) []string { return nil })
	if n := said(); n != 2 {
		t.Errorf("the folder refused again after it was watched is said %d times in all, want twice", n)
	}
}
