package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// Added is what became of files given to the app, picked with Add or
// dropped anywhere on it: the videos added, the one to open and why
// anything was left out. The window opens Open, so Add and a drop end the
// same way, see takeFiles.
type Added struct {
	Added []string `json:"added"`
	// The first video added, or when none was, the first given that is in
	// the library already, so a video given again opens rather than
	// doing nothing.
	Open  string `json:"open,omitempty"`
	Error string `json:"error,omitempty"`
}

// takeFiles adds the videos among files given to the app, through
// addEpisodes, and leaves out anything else with the reason. Add and a
// drop both come here, so they decide the same things the same way.
func (s *FrameFairy) takeFiles(how string, files []string) Added {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = filepath.Base(f)
	}
	theLog.line(zerolog.InfoLevel, "app").Str("how", how).Strs("files", names).Msg("files given to the app")

	var videos, others []string
	for _, f := range files {
		if engine.IsVideo(f) {
			videos = append(videos, f)
		} else {
			others = append(others, filepath.Base(f))
		}
	}
	added, err := s.addEpisodes(videos)
	out := Added{Added: added}
	if out.Added == nil {
		out.Added = []string{}
	}
	if len(added) > 0 {
		out.Open = added[0]
	} else {
		library := s.store.Episodes()
		for _, v := range videos {
			abs, absErr := filepath.Abs(v)
			if absErr == nil && slices.Contains(library, abs) {
				out.Open = abs
				break
			}
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
		theLog.line(zerolog.WarnLevel, "app").Str("how", how).Int("added", len(added)).Err(errors.New(out.Error)).Msg("files were left out")
	} else {
		theLog.line(zerolog.InfoLevel, "app").Str("how", how).Int("added", len(added)).Msg("the files are in the library")
	}
	return out
}

// dropFiles takes files dropped anywhere on the app, the way Add takes
// the ones picked in its box.
func (s *FrameFairy) dropFiles(files []string) Added { return s.takeFiles("drop", files) }

// notVideos says that files are not videos, by name.
func notVideos(names []string) string {
	if len(names) == 1 {
		return names[0] + " is not a video, so it was left out"
	}
	last := len(names) - 1
	return strings.Join(names[:last], ", ") + " and " + names[last] + " are not videos, so they were left out"
}
