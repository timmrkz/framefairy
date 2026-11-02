package main

import (
	"encoding/json"
	"path/filepath"
	"strings"

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
// whenever it comes back to the front. A path is only a hint of where to
// look: what reconcile does is decided by what it reads on the disk, so a
// hint missed, or heard twice, or heard for a change of the app's own,
// does no harm.
//
// A video whose folder was deleted leaves the library, see
// letGoIfRemoved. A video a hint named that stays has its state sent to
// the interface, and the sidebar is told when what it lists of the video
// changed. Then the watch follows the library as it is now.
func (s *FrameFairy) reconcile(changed []string) {
	for _, ep := range s.store.Episodes() {
		if changed != nil && !touches(ep, changed) {
			continue
		}
		if s.letGoIfRemoved(ep) {
			// Read for the library, the library is being read already.
			if changed != nil {
				s.listChanged(ep)
			}
			continue
		}
		if changed != nil {
			s.told(ep)
			s.listIfChanged(ep)
		}
	}
	s.watch.follow(s.store.files, s.store.Episodes())
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
	s.levels.stop(ep)
	s.forget(ep)
	s.mu.Lock()
	delete(s.listed, ep)
	s.mu.Unlock()
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

// listIfChanged tells the sidebar about a video when what the library
// says of it is not what it said the last time a hint named it: File not
// found as its file went, or back again, how far it is heard, its clips.
func (s *FrameFairy) listIfChanged(ep string) {
	body, err := json.Marshal(s.Episode(ep))
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.listed == nil {
		s.listed = map[string]string{}
	}
	same := s.listed[ep] == string(body)
	s.listed[ep] = string(body)
	s.mu.Unlock()
	if !same {
		s.listChanged(ep)
	}
}

// listChanged tells the interface that what the library says of a video
// changed, so it reads the library again, the way the queue does when a
// job moves on. It does nothing without a queue, which is what a test
// that builds the service by hand may have.
func (s *FrameFairy) listChanged(ep string) {
	if s.jobs != nil && s.jobs.notify != nil {
		s.jobs.notify(ep)
	}
}
