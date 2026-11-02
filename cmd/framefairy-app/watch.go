package main

import (
	"path/filepath"
	"slices"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// watching keeps a watch on the folders the files of the videos in the
// library are in, and hands every path that changed in them to heard,
// which is reconcile, whoever changed it: the person in Finder, the
// command line, a sync service, or the app itself. The system says what
// changed through fsnotify: kqueue on macOS, inotify on Linux,
// ReadDirectoryChangesW on Windows.
//
// Like every method here it does nothing on a nil watching, which is what
// a test that builds the service by hand has, and what a program has
// whose system would not watch.
type watching struct {
	w     *fsnotify.Watcher
	heard func(changed []string)
	// mu keeps follow to one at a time, and folders is what the last one
	// counted, so the count is said only when it changed.
	mu      sync.Mutex
	folders int
	done    chan struct{}
}

// newWatching starts a watch that hands what changed to heard. It
// watches nothing until follow names the videos.
func newWatching(heard func(changed []string)) *watching {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		theLog.line(zerolog.WarnLevel, "files").Err(err).
			Msg("the disk cannot be watched, so a change made outside the app shows once the app comes back to the front")
		return nil
	}
	ws := &watching{w: w, heard: heard, done: make(chan struct{})}
	go ws.run()
	return ws
}

// watchedOf is the folders watched for a video: the one it is in, which
// holds the video and its work folder, the work folder, and its logs,
// which hold the transcript, the plans and the loudness. A folder that is
// not there yet is watched once it is, since its making is a change in
// the folder above it.
func watchedOf(video string) []string {
	work := engine.WorkDir(video)
	return []string{filepath.Dir(video), work, filepath.Join(work, "logs")}
}

// follow watches the folders of these videos that are there, and no
// other. It is called after every look at the library, so the watch
// follows what was added, removed, deleted and made again, the way the
// library itself does.
func (ws *watching) follow(files afero.Fs, videos []string) {
	if ws == nil {
		return
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	want := map[string]bool{}
	for _, v := range videos {
		for _, dir := range watchedOf(v) {
			want[dir] = true
		}
	}
	have := ws.w.WatchList()
	for _, dir := range have {
		if !want[dir] {
			_ = ws.w.Remove(dir)
		}
	}
	for dir := range want {
		if slices.Contains(have, dir) {
			continue
		}
		if info, err := files.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		if err := ws.w.Add(dir); err != nil {
			theLog.line(zerolog.WarnLevel, "files").Str("folder", dir).Err(err).
				Msg("a folder of a video cannot be watched, so a change in it shows once the app comes back to the front")
		}
	}
	// How many folders are watched, and how many files are in them,
	// which on macOS each take a file descriptor of their own: what
	// the decision record asked to be measured on a real library.
	now := ws.w.WatchList()
	if len(now) == ws.folders {
		return
	}
	ws.folders = len(now)
	entries := 0
	for _, dir := range now {
		if list, err := afero.ReadDir(files, dir); err == nil {
			entries += len(list)
		}
	}
	theLog.line(zerolog.InfoLevel, "files").Int("videos", len(videos)).Int("folders", len(now)).Int("files", entries).
		Msg("the folders of the videos are watched for changes")
}

// run hands what changed to heard until the watch is stopped. A change
// heard while the last was handed on waits, and all that waits is
// handed on together, so the three steps of one file written in place,
// or a folder of many files deleted, are looked at once.
func (ws *watching) run() {
	defer close(ws.done)
	for {
		var changed []string
		select {
		case ev, ok := <-ws.w.Events:
			if !ok {
				return
			}
			changed = heardOf(changed, ev)
		case err, ok := <-ws.w.Errors:
			if !ok {
				return
			}
			theLog.line(zerolog.WarnLevel, "files").Err(err).Msg("the watch on the disk missed a change")
			continue
		}
		for more := true; more; {
			select {
			case ev, ok := <-ws.w.Events:
				if !ok {
					more = false
					break
				}
				changed = heardOf(changed, ev)
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
// all the time, and nothing the app has depends on them.
func heardOf(changed []string, ev fsnotify.Event) []string {
	if ev.Op == fsnotify.Chmod || slices.Contains(changed, ev.Name) {
		return changed
	}
	return append(changed, ev.Name)
}

// stop ends the watch and returns once nothing more is handed on.
func (ws *watching) stop() {
	if ws == nil {
		return
	}
	_ = ws.w.Close()
	<-ws.done
}
