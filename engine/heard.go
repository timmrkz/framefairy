package engine

import (
	"fmt"
	"slices"
	"sync"

	"github.com/spf13/afero"
)

// heard is what Status reads of a video's transcript: whether it is done,
// the parts heard, and from the first line to the last.
type heard struct {
	done  bool
	heard [][2]float64
	said  *[2]float64
}

// keptHeard is how many transcripts' heard are kept, one for each video
// worked on lately, the same as the app keeps transcripts of.
const keptHeard = 10

// The heard of the transcripts read last, each kept while its file and
// its video stay as they were. The app sends a video's state up to four
// times a second while its work goes on, issue #606, and reading the
// transcript of a video of four hours for it took 125 ms in a cloud
// session: 60 to read the file, 25 to place its words on the sound and 35
// to make its lines.
var heardKept = struct {
	mu   sync.Mutex
	keys []string
	of   map[string]heard
}{of: map[string]heard{}}

// heardOf is heard for the transcript at path, of the video stamp
// names, false when there is none that matches it. silenceDB is the
// floor the words are placed on the sound with, see Status.
func heardOf(fsys afero.Fs, path string, stamp sourceStamp, model string, silenceDB *float64) (heard, bool) {
	info, err := fsys.Stat(path)
	if err != nil {
		return heard{}, false
	}
	floor := "auto"
	if silenceDB != nil {
		floor = fmt.Sprint(*silenceDB)
	}
	// The files are part of the key, so two tests with a video of the
	// same name in memory never share one.
	key := fmt.Sprintf("%p|%s|%d|%d|%v|%s|%s", fsys, path, info.Size(), info.ModTime().UnixNano(), stamp, model, floor)
	heardKept.mu.Lock()
	h, ok := heardKept.of[key]
	heardKept.mu.Unlock()
	if ok {
		return h.copy(), true
	}
	file, words, frames, ok := readTranscriptFile(fsys, path, stamp, model)
	if !ok {
		return heard{}, false
	}
	// The lines a clip made by hand is made of, the way MakeClip makes
	// them, see Project.Transcript.
	t := fromStored(words, frames, float64(file.From), float64(file.Mean), silenceDB)
	h = heard{done: file.done(), heard: file.heard(), said: saidOf(BuildLines(t.Words, t.Levels(), nil))}
	heardKept.mu.Lock()
	defer heardKept.mu.Unlock()
	if _, there := heardKept.of[key]; !there {
		heardKept.keys = append(heardKept.keys, key)
	}
	heardKept.of[key] = h
	for len(heardKept.keys) > keptHeard {
		delete(heardKept.of, heardKept.keys[0])
		heardKept.keys = slices.Delete(heardKept.keys, 0, 1)
	}
	return h.copy(), true
}

// copy is heard that shares nothing with what is kept.
func (h heard) copy() heard {
	out := heard{done: h.done, heard: slices.Clone(h.heard)}
	if h.said != nil {
		said := *h.said
		out.said = &said
	}
	return out
}
