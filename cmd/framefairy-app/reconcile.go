package main

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// reconcile brings what the app holds and shows of the videos in the
// library in step with their files. The files on the disk are the truth,
// and the app is one of several that write them: the person in Finder or
// a terminal, the command line, a sync service. This is the one place
// where a change any of them made lands, see the decision record
// docs/decisions/2026-11-01-the-app-in-step-with-its-files.md.
//
// changed is the paths heard to have changed, from the watch on the
// disk, see watching, and nil for a look at every video in the library,
// which the app takes when it reads the library, as it starts and
// whenever it comes back to the front, and when the watch says it missed
// a change. A path is only a hint of where to look: what reconcile does
// is decided by what it reads on the disk, and nothing is kept of what
// it read the time before, so a hint missed, or heard twice, or heard for
// a change of the app's own, does no harm.
//
// A video whose folder was deleted leaves the library, see
// letGoIfRemoved. A video a hint named that stays has its state sent to
// the open workspace, which videoNews keeps to four times a second. The
// sidebar is told only of what changes its rows from outside: a video
// that left, its file gone or back, its work folder made or deleted.
// What changes inside the work folder is the app's own work most of the
// time, and the sidebar hears of that from the jobs, see listChanged.
// Then the watch follows the library, when a folder it watches or would
// watch was made or deleted. A change in the models folder, and every
// look at the whole library, sends the setup and the settings what they
// show of the models, see tellSetup.
func (s *FrameFairy) reconcile(changed []string) {
	s.reconcileTimed(changed, nil)
}

// lookSteps is how long each step of a look at the whole library took, the
// models, the videos and the watch, for the first look's line in the log.
type lookSteps struct {
	models, videos, watch time.Duration
}

// reconcileTimed is reconcile, with how long each step took kept in
// steps, when it is given.
func (s *FrameFairy) reconcileTimed(changed []string, steps *lookSteps) {
	s.reconciling.Lock()
	defer s.reconciling.Unlock()
	if steps == nil {
		steps = &lookSteps{}
	}
	if s.look(changed, steps) {
		s.followTimed(steps)
	}
}

// followTimed has the watch follow the library, see watching.follow, and
// keeps how long it took in steps.
func (s *FrameFairy) followTimed(steps *lookSteps) {
	began := time.Now()
	s.watch.follow(s.store.files, s.store.Episodes(), s.shortsOf)
	steps.watch = time.Since(began)
}

// look is reconcile but for the watch, under its lock, and says whether
// the watch is to follow the library after it.
func (s *FrameFairy) look(changed []string, steps *lookSteps) (follow bool) {
	began := time.Now()
	took := func(into *time.Duration) {
		now := time.Now()
		*into, began = now.Sub(began), now
	}
	follow = changed == nil
	// A model deleted or added outside the app shows in the setup and
	// the settings at once, and a look at every video looks at the
	// models too, for whatever the watch could not hear.
	if models, folder := s.watch.modelsIn(changed); models || changed == nil {
		// A models folder made is watched before it is read, so a model
		// written into it in between is heard, issue #722. This follow
		// waits for one under way, which would only take its ask next.
		if folder {
			s.watch.followWaiting(s.store.files, s.store.Episodes(), s.shortsOf)
		}
		s.tellSetup()
	}
	took(&steps.models)
	for _, ep := range s.store.Episodes() {
		if changed != nil && !touches(ep, changed) && !touchesShort(s.shortsOf(ep), changed) {
			continue
		}
		if s.letGoIfRemoved(ep) {
			follow = true
			// Read for the library, the library is being read already.
			if changed != nil {
				s.listChanged(ep)
			}
			continue
		}
		if changed == nil {
			continue
		}
		s.told(ep)
		if reshapes(ep, changed) {
			follow = true
			s.listChanged(ep)
		}
		// A plan written may name a short just rendered, and a short
		// rendered again replaces the file watched, whose watch its
		// watcher lets go of, so the new one is watched by the follow
		// the old one's going brings, see watching.
		if writesPlan(ep, changed) || touchesShort(s.shortsOf(ep), changed) {
			follow = true
		}
	}
	took(&steps.videos)
	return follow
}

// watchTheDisk starts the watch on the folders of the videos. What it
// watches it learns from the first look, see lookFirst.
func (s *FrameFairy) watchTheDisk() {
	s.watch = newWatching(s.reconcile, engine.ModelsDir())
}

// lookFirst takes the first look at every video in the library, as the
// app starts, and then says where the searches and renders that were cut
// off or failed the last time stopped. It runs beside the making of the
// app's window, which waited behind it: the first start of a new build
// showed no window for 1.6 s in the median on Tim's Mac, issue #888. The
// window's calls that name a video wait for it instead, see store.Known,
// and so do the job list and the library. A video deleted outside the app
// while it was closed is then let go before anything opens it or writes
// into its folder: Tim deleted two videos with their folders, an earlier
// version wrote the chosen clip back into one, and the app opened on it
// after an update and said it was not in the library.
//
// The window's calls wait for the look at the videos and the jobs, and
// not for the watch on their folders, which is set after they are
// answered: on the first start of a new build macOS took 1.7 s on the
// median to let the watch open the files in them, issue #990. A change
// made in a folder before the watch is on it is read by the next look,
// as the app comes back to the front.
//
// It closes the store's looked once the videos are looked at, whatever
// happens, so a look that goes wrong holds no call for good. Each step's
// time is in the log, to see on the Mac where a slow first start goes.
func (s *FrameFairy) lookFirst() {
	began := time.Now()
	answered := sync.OnceFunc(func() {
		if s.store.looked != nil {
			close(s.store.looked)
		}
	})
	defer func() {
		if caught := recover(); caught != nil {
			theLog.line(zerolog.ErrorLevel, "files").Str("stack", string(debug.Stack())).
				Msgf("the first look at the library stopped unexpectedly: %v", caught)
		}
		answered()
	}()
	var steps lookSteps
	follow := func() bool {
		s.reconciling.Lock()
		defer s.reconciling.Unlock()
		return s.look(nil, &steps)
	}()
	restoring := time.Now()
	s.restoreJobs(s.store.Episodes())
	jobs := time.Since(restoring)
	answered()
	window := time.Since(began)
	if follow {
		s.followTimed(&steps)
	}
	theLog.line(zerolog.InfoLevel, "files").Int("videos", len(s.store.Episodes())).
		Int64("models_ms", steps.models.Milliseconds()).Int64("videos_ms", steps.videos.Milliseconds()).
		Int64("jobs_ms", jobs.Milliseconds()).Int64("window_ms", window.Milliseconds()).
		Int64("watch_ms", steps.watch.Milliseconds()).
		Int64("ms", time.Since(began).Milliseconds()).Msg("the first look at the library is over")
}

// tellSetup sends the window what the setup and the settings show of the
// models, the way they read it with Setup, when it is not what was sent
// last: a model copied in by hand is written for minutes, and each write
// is heard. It runs inside reconcile, whose lock keeps setupSent.
func (s *FrameFairy) tellSetup() {
	if s.setupNews == nil {
		return
	}
	state := s.Setup(context.Background())
	if s.setupSent != nil && reflect.DeepEqual(*s.setupSent, state) {
		return
	}
	s.setupSent = &state
	s.setupNews(state)
}

// modelsMade is the app's word to itself that a download may have made
// the models folder, and the folder over it. The watch follows a folder
// only once it is there, and on a machine the app never ran on neither
// is, so without it a model the first run downloaded and Finder deleted
// later would go on showing until the next look at the library.
func (s *FrameFairy) modelsMade() {
	s.reconcile([]string{engine.ModelsDir()})
}

// shortsOf is the shorts of a video, as its plans say, see
// engine.Episode.Shorts.
func (s *FrameFairy) shortsOf(video string) []string {
	return s.store.machine.Episode(video).Shorts()
}

// touches says whether a change at one of the paths can change what the
// app has of video: the video itself, its work folder or anything in it,
// or a folder the video is in.
func touches(video string, changed []string) bool {
	work := engine.WorkDir(video)
	sep := string(filepath.Separator)
	for _, p := range changed {
		if p == video || p == work || strings.HasPrefix(p, work+sep) || strings.HasPrefix(video, p+sep) {
			return true
		}
	}
	return false
}

// touchesShort says whether one of the paths is one of a video's shorts,
// a short deleted or moved in Finder. It is asked only when touches says
// no, because the shorts are read from the plans.
func touchesShort(shorts, changed []string) bool {
	for _, p := range changed {
		if slices.Contains(shorts, p) {
			return true
		}
	}
	return false
}

// writesPlan says whether one of the paths is a plan of video, which a
// render writes the folder of its short into.
func writesPlan(video string, changed []string) bool {
	logs := engine.LogsDir(video)
	for _, p := range changed {
		if ok, _ := filepath.Match("clips*.json", filepath.Base(p)); ok && filepath.Dir(p) == logs {
			return true
		}
	}
	return false
}

// reshapes says whether a change at one of the paths is to the video
// itself or to a folder the watch keeps for it, see watchedOf: the file
// gone or back, a folder made or deleted. Those change the sidebar's row
// of the video and which folders are watched. A file written inside the
// work folder does neither.
func reshapes(video string, changed []string) bool {
	work := engine.WorkDir(video)
	sep := string(filepath.Separator)
	for _, p := range changed {
		if p == video || p == work || p == filepath.Join(work, "logs") || strings.HasPrefix(video, p+sep) {
			return true
		}
	}
	return false
}

// letGoIfRemoved takes a video whose folder was deleted outside the app,
// with the video or without it, see deletedOutside, out of the library,
// the way Remove with Delete does, and says whether it did: its folder
// is everything the app has of it, so with the folder gone the video is
// gone from the app too, and adding it again starts it from nothing.
//
// Its work stops and the library no longer has it before anything else,
// so nothing new starts on it. Whatever was still writing its folder is
// waited for out of the way, and what it wrote back is deleted.
func (s *FrameFairy) letGoIfRemoved(ep string) bool {
	if !deletedOutside(s.store.files, ep) {
		return false
	}
	gone, err := s.store.letGo(ep)
	if err != nil {
		theLog.line(zerolog.ErrorLevel, "files").Str("video", filepath.Base(ep)).Err(err).
			Msg("the video's folder was deleted outside the app, and the library could not be written without it")
	}
	if !gone {
		return false
	}
	_, err = s.store.files.Stat(ep)
	theLog.line(zerolog.InfoLevel, "files").Str("video", filepath.Base(ep)).
		Str("folder", filepath.Base(engine.WorkDir(ep))).Bool("videoDeleted", err != nil).
		Msg("the video's folder was deleted outside the app, so the video leaves the library")
	s.jobs.closeEpisode(ep)
	s.jobs.forgetEpisode(ep)
	s.levels.closeEpisode(ep)
	s.forget(ep)
	s.clearMu.Lock()
	if s.quitting {
		s.clearMu.Unlock()
		s.clearLetGo(ep)
		return true
	}
	s.clearing.Go(func() { s.clearLetGo(ep) })
	s.clearMu.Unlock()
	return true
}

// listChanged tells the interface that what the library says of a video
// changed, so it reads the library again, the way the queue does when a
// job moves on or ends. It does nothing without a queue, which is what a test
// that builds the service by hand may have.
func (s *FrameFairy) listChanged(ep string) {
	if s.jobs != nil && s.jobs.notify != nil {
		s.jobs.notify(ep)
	}
}
