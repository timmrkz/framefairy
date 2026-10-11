package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// Dropped is what became of files dropped on the app: the videos added,
// the one to open and why anything was left out. The window opens Open
// the way it opens a video picked with Add.
type Dropped struct {
	Added []string `json:"added"`
	// The first video dropped that is in the library now, added by this
	// drop or there before it, so a video dropped again opens rather than
	// doing nothing.
	Open  string `json:"open,omitempty"`
	Error string `json:"error,omitempty"`
}

// dropFiles adds the videos among files dropped anywhere on the app, the
// way Add adds the ones picked in its box, see addEpisodes. Anything else
// is left out with the reason.
func (s *FrameFairy) dropFiles(files []string) Dropped {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	theLog.line(zerolog.InfoLevel, "app").Strs("files", names).Msg("files dropped on the app")

	var videos, others []string
	for _, f := range files {
		if engine.IsVideo(f) {
			videos = append(videos, f)
		} else {
			others = append(others, filepath.Base(f))
		}
	}
	added, err := s.addEpisodes(videos)
	out := Dropped{Added: added}
	if out.Added == nil {
		out.Added = []string{}
	}
	library := s.store.Episodes()
	for _, v := range videos {
		abs, absErr := filepath.Abs(v)
		if absErr == nil && slices.Contains(library, abs) {
			out.Open = abs
			break
		}
	}
	var why []string
	if len(others) > 0 {
		why = append(why, notVideos(others))
	}
	if err != nil {
		why = append(why, err.Error())
	}
	if len(why) > 0 {
		out.Error = strings.Join(why, ". And ")
		theLog.line(zerolog.WarnLevel, "app").Int("added", len(added)).Err(errors.New(out.Error)).Msg("a drop left files out")
	} else {
		theLog.line(zerolog.InfoLevel, "app").Int("added", len(added)).Msg("a drop is in the library")
	}
	return out
}

// notVideos says that files are not videos, by name.
func notVideos(names []string) string {
	if len(names) == 1 {
		return names[0] + " is not a video, so it was left out"
	}
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + " and " + names[last] + " are not videos, so they were left out"
}
