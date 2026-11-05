package engine

import (
	"os"
	"path/filepath"
	"strings"

	"framefairy/engine/fileio"
)

// Whose a path is, asked of the video's Episode: the app reads, writes,
// serves and shows in a folder only the files of a video in its library,
// and it asks the video whether a path is one of them. A path is judged by
// where it really leads. A link inside a work folder can point anywhere,
// and a name is not a promise, so both the path and the folder it has to
// be inside are resolved before they are compared. See
// docs/decisions/2026-11-03-one-episode-per-video.md, Part 4.

// Linked says why the episode's video cannot be one the app keeps because
// it, or its work folder, is a link, or "" when neither is. A link inside
// a work folder is judged by where it leads, see Owns, but the folder
// itself was taken as it was found: a zip with episode.mp4 and an
// episode.framefairy that led to the home folder made every file there
// the episode's, to read, to serve and to write into.
func (ep *Episode) Linked() string {
	fsys := ep.m.Files
	if info, err := fileio.LstatIn(fsys, ep.source); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return filepath.Base(ep.source) + " is a link, which the app does not follow"
	}
	work := WorkDir(ep.source)
	if info, err := fileio.LstatIn(fsys, work); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return filepath.Base(work) + " beside " + filepath.Base(ep.source) +
			" is a link or not a folder, which the app does not follow"
	}
	return ""
}

// Owns says whether path is the episode's video itself or a file in its
// work folder, by where the path really leads. A video that is a link, or
// whose work folder is one, owns nothing, see Linked. The shorts of its
// clips, wherever they were rendered into, are IsShort's to say.
func (ep *Episode) Owns(path string) bool {
	if ep.Linked() != "" {
		return false
	}
	fsys := ep.m.Files
	clean := filepath.Clean(path)
	real := fileio.ResolveIn(fsys, clean)
	if clean == ep.source || real == fileio.ResolveIn(fsys, ep.source) {
		return true
	}
	work := fileio.ResolveIn(fsys, WorkDir(ep.source)) + string(filepath.Separator)
	return strings.HasPrefix(real, work) && len(real) > len(work)
}

// HasPlan says whether plan is a plan of the episode: a file of its own,
// see Owns, right in its logs folder, where plans are kept. A plan of one
// video given with the path of another would be edited against the wrong
// video, the wrong words and the wrong history.
func (ep *Episode) HasPlan(plan string) bool {
	if !ep.Owns(plan) {
		return false
	}
	fsys := ep.m.Files
	return filepath.Dir(fileio.ResolveIn(fsys, plan)) == fileio.ResolveIn(fsys, LogsDir(ep.source))
}
