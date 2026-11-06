package main

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// watching keeps a watch on the folders the files of the videos in the
// library are in, and on the models folder and the one over it, and hands
// every path that changed in them to heard, which is reconcile, whoever
// changed it: the person in Finder, the command line, a sync service, or
// the app itself. The system says what
// changed through fsnotify: kqueue on macOS, inotify on Linux,
// ReadDirectoryChangesW on Windows.
//
// Like every method here it does nothing on a nil watching, which is what
// a test that builds the service by hand has, and what a program has
// whose system would not watch.
//
// A folder is watched by one of two watchers, by how deep it is, so no
// watcher watches a folder and the folder it is in. On macOS a watcher
// that watches a folder watches what is in it too, and when a folder is
// made in it, it sets its own watch on the new one in the same moment the
// app adds it, and the one set last wins. Its own hears nothing written
// in the folder, so a speech model's tokens.txt, or a model in a models
// folder put back from a backup, went unheard, issue #722. Two watchers
// keep the app's watch on a folder the only one in its watcher. The races
// are fsnotify's own issue 783, still open, and once it is fixed one
// watcher is enough again.
type watching struct {
	byDepth [2]*fsnotify.Watcher
	heard   func(changed []string)
	// models is the folder the models are kept in, watched with the
	// folder over it, so a model deleted or added outside the app shows
	// at once. Empty for none.
	models string
	// mu keeps follow to one at a time, and watched is what the last one
	// counted, so the count is said only when it changed.
	mu      sync.Mutex
	watched int
	done    chan struct{}
}

// newWatching starts a watch that hands what changed to heard, with the
// models folder models, or none when it is empty. It watches nothing
// until follow names the videos.
func newWatching(heard func(changed []string), models string) *watching {
	ws := &watching{heard: heard, models: models, done: make(chan struct{})}
	for i := range ws.byDepth {
		w, err := fsnotify.NewWatcher()
		if err != nil {
			theLog.line(zerolog.WarnLevel, "files").Err(err).
				Msg("the disk cannot be watched, so a change made outside the app shows once the app comes back to the front")
			for _, made := range ws.byDepth[:i] {
				_ = made.Close()
			}
			return nil
		}
		ws.byDepth[i] = w
	}
	go ws.run()
	return ws
}

// watcherOf is the watcher that watches path: the one of its depth, so a
// folder and the folder it is in are never watched by the same one.
func (ws *watching) watcherOf(path string) *fsnotify.Watcher {
	return ws.byDepth[strings.Count(filepath.Clean(path), string(filepath.Separator))%2]
}

// watchList is everything watched, by both watchers.
func (ws *watching) watchList() []string {
	return append(ws.byDepth[0].WatchList(), ws.byDepth[1].WatchList()...)
}

// watchedOf is what is watched for a video: the folder it is in, which
// holds the video and its work folder, the work folder, its logs, which
// hold the transcript, the plans and the loudness, and its shorts, which
// the plans say, see engine.Episode.Shorts. A folder that is not there
// yet is watched once it is, since its making is a change in the folder
// above it. A short that is not there is not watched, and a short
// rendered again is watched again once its plan is written.
func watchedOf(video string, shorts []string) []string {
	work := engine.WorkDir(video)
	return append([]string{filepath.Dir(video), work, filepath.Join(work, "logs")}, shorts...)
}

// follow watches the folders of these videos that are there, and no
// other, with the shorts shortsOf says each video has. It is called
// after every look at the library, and after a plan is written, which
// may name a short just rendered, so the
// watch follows what was added, removed, deleted, made again and
// rendered, the way the library itself does.
func (ws *watching) follow(files afero.Fs, videos []string, shortsOf func(video string) []string) {
	if ws == nil {
		return
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	want := map[string]bool{}
	for _, dir := range ws.modelFolders() {
		want[dir] = true
	}
	for _, v := range videos {
		for _, dir := range watchedOf(v, shortsOf(v)) {
			want[dir] = true
		}
	}
	for _, dir := range ws.watchList() {
		if !want[dir] {
			_ = ws.watcherOf(dir).Remove(dir)
		}
	}
	// Each folder is added again even when it is watched already. On
	// Linux a folder deleted stays on the list until the system says so
	// of the folder itself, which can come after the folder put back is
	// heard, and only adding it again watches the folder that is there
	// now, issue #722. Adding a folder that is watched changes nothing.
	for dir := range want {
		if _, err := files.Stat(dir); err != nil {
			continue
		}
		if err := ws.watcherOf(dir).Add(dir); err != nil {
			theLog.line(zerolog.WarnLevel, "files").Str("folder", dir).Err(err).
				Msg("a folder of a video cannot be watched, so a change in it shows once the app comes back to the front")
		}
	}
	// How much is watched, and how many files are in the folders,
	// which on macOS each take a file descriptor of their own, and each
	// folder two, one in each watcher: what the decision record asked
	// to be measured on a real library.
	now := ws.watchList()
	if len(now) == ws.watched {
		return
	}
	ws.watched = len(now)
	folders, entries, models := 0, 0, 0
	for _, dir := range now {
		if slices.Contains(ws.modelFolders(), dir) {
			models++
			continue
		}
		if list, err := afero.ReadDir(files, dir); err == nil {
			folders++
			entries += len(list)
		}
	}
	theLog.line(zerolog.InfoLevel, "files").Int("videos", len(videos)).Int("folders", folders).
		Int("shorts", len(now)-folders-models).Int("files", entries).Int("models", models).
		Msg("the folders of the videos are watched for changes")
}

// run hands what changed to heard until the watch is stopped. A change
// heard while the last was handed on waits, and all that waits is
// handed on together, so the three steps of one file written in place,
// or a folder of many files deleted, are looked at once.
func (ws *watching) run() {
	defer close(ws.done)
	a, b := ws.byDepth[0], ws.byDepth[1]
	for {
		var changed []string
		var ev fsnotify.Event
		var err error
		ok := true
		select {
		case ev, ok = <-a.Events:
		case ev, ok = <-b.Events:
		case err, ok = <-a.Errors:
		case err, ok = <-b.Errors:
		}
		if !ok {
			return
		}
		if err != nil {
			// The system's queue of changes ran over, most often, so
			// which changed is not known: every video is looked at.
			theLog.line(zerolog.WarnLevel, "files").Err(err).Msg("the watch on the disk missed a change, so every video is looked at")
			ws.heard(nil)
			continue
		}
		changed = ws.heardOf(changed, ev)
		for more := true; more; {
			select {
			case ev, ok := <-a.Events:
				more = ok
				if ok {
					changed = ws.heardOf(changed, ev)
				}
			case ev, ok := <-b.Events:
				more = ok
				if ok {
					changed = ws.heardOf(changed, ev)
				}
			default:
				more = false
			}
		}
		if len(changed) > 0 {
			ws.heard(changed)
		}
	}
}

// heardOf adds the path of an event to changed, once. A change of a
// file's permissions alone is left out: Spotlight and backups touch them
// all the time, and nothing the app has depends on them. So is what the
// app's own download writes, see downloading.
func (ws *watching) heardOf(changed []string, ev fsnotify.Event) []string {
	if ev.Op == fsnotify.Chmod || slices.Contains(changed, ev.Name) || ws.downloading(ev.Name) {
		return changed
	}
	return append(changed, ev.Name)
}

// downloading says whether path is where a download of the app writes a
// model before it is one, its part file or the folder it unpacks into,
// which it writes for minutes. Only the rename into place makes a model,
// and that is heard under the model's own name. Any other write in the
// models folder is heard, since a model copied there by hand, with Finder
// or cp, is made empty and becomes a model as it is written, see
// Installed in the engine.
func (ws *watching) downloading(path string) bool {
	if ws == nil || ws.models == "" || filepath.Dir(path) != ws.models {
		return false
	}
	return strings.HasSuffix(path, ".part") || strings.HasSuffix(path, ".unpacking")
}

// modelFolders is what is watched for the models: the models folder, the
// folder over it, which holds it, and the folder of each speech model,
// since a speech model is a folder whose tokens.txt says it is there, and
// one copied in by hand is written into after the folder is made.
func (ws *watching) modelFolders() []string {
	if ws == nil || ws.models == "" {
		return nil
	}
	dirs := []string{filepath.Dir(ws.models), ws.models}
	for _, m := range engine.SpeechModels() {
		dirs = append(dirs, filepath.Join(ws.models, m.Name))
	}
	return dirs
}

// modelsIn says whether a change at one of the paths can change which
// models are here, and whether one of the folders modelFolders names was
// made or deleted, which the watch then follows.
func (ws *watching) modelsIn(changed []string) (models, folder bool) {
	if ws == nil || ws.models == "" {
		return false, false
	}
	folders := ws.modelFolders()[1:]
	for _, p := range changed {
		if slices.Contains(folders, p) {
			models, folder = true, true
		} else if strings.HasPrefix(p, ws.models+string(filepath.Separator)) {
			models = true
		}
	}
	return models, folder
}

// stop ends the watch and returns once nothing more is handed on.
func (ws *watching) stop() {
	if ws == nil {
		return
	}
	for _, w := range ws.byDepth {
		_ = w.Close()
	}
	<-ws.done
}
