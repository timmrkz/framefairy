package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
)

// What ffmpeg says it can do, and what ffprobe says a file holds, stays
// true for as long as the binary and the file stay as they were. So it is
// asked once a process and kept by the file's size and time, the way
// checkTool keeps the tools it has checked, and asked again the moment
// either changes. Every engine shares it, and there are many: the app
// makes one for every job, every caption it draws and every video it
// reads, and a render once asked ffprobe the same questions about the
// same file twelve times over, issues #346 and #351.

// fileStamp is a file as it is now, by where it really is, its size and
// the time it was last written. It reads the real disk and not an
// engine's Files, because that is where ffmpeg and ffprobe read it.
type fileStamp struct {
	path string
	toolStamp
}

// diskStampOf is the stamp of the file at path, a bare program name looked up
// the way it would be run, and false for a file that is not there.
func diskStampOf(path string) (fileStamp, bool) {
	if path == "" {
		return fileStamp{}, false
	}
	if filepath.Base(path) == path {
		found, err := exec.LookPath(path)
		if err != nil {
			return fileStamp{}, false
		}
		path = found
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fileStamp{}, false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return fileStamp{}, false
	}
	return fileStamp{abs, toolStamp{info.Size(), info.ModTime()}}, true
}

// probeKey is one question about one file: what kind of answer, the
// ffprobe or ffmpeg that gave it, and the file it is about.
type probeKey struct {
	kind string
	tool fileStamp
	file fileStamp
}

// keyFor is the key of a question kind asks tool about file, and false
// when either is not a file whose answer can be kept.
func keyFor(kind, tool, file string) (probeKey, bool) {
	t, ok := diskStampOf(tool)
	if !ok {
		return probeKey{}, false
	}
	if file == "" {
		return probeKey{kind: kind, tool: t}, true
	}
	f, ok := diskStampOf(file)
	if !ok {
		return probeKey{}, false
	}
	return probeKey{kind, t, f}, true
}

// probedMost is how many answers are kept. A file written again leaves its
// old answer behind, so the answers are all let go when there are this
// many, which no number of episodes and shorts a person works on reaches.
const probedMost = 4096

var probed = struct {
	sync.Mutex
	answers map[probeKey]any
}{answers: map[probeKey]any{}}

// recall is the answer kept for key.
func recall[T any](key probeKey) (T, bool) {
	probed.Lock()
	defer probed.Unlock()
	v, ok := probed.answers[key].(T)
	return v, ok
}

// remember keeps an answer for key.
func remember(key probeKey, v any) {
	probed.Lock()
	defer probed.Unlock()
	if len(probed.answers) >= probedMost {
		clear(probed.answers)
	}
	probed.answers[key] = v
}

// clone is a SourceInfo that shares no slice with the one kept, so a
// caller that changes what it was given changes nothing for the next.
func (s SourceInfo) clone() SourceInfo {
	s.Colour = slices.Clone(s.Colour)
	s.unevenAt = slices.Clone(s.unevenAt)
	return s
}
