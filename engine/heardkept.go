package engine

import (
	"encoding/json"
	"math"
	"path/filepath"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
)

// What a video's transcript has heard is kept in its work folder,
// logs/heard.json, beside the transcript, so a start of the app reads a
// few hundred bytes of each video instead of its whole transcript. The
// library asks Status of every video as the app starts, and every start
// read and snapped every word of every video only to learn whether it
// was done, the parts heard and from the first line to the last, issue
// #953. See docs/decisions/2026-11-21-what-is-heard-kept-in-the-work-folder.md.

// heardVersion is the version of logs/heard.json. A file of another
// version is not read, and the next read of the transcript writes it
// again.
const heardVersion = 1

// heardName is the file what a video's transcript has heard is kept in,
// in its logs folder.
const heardName = "heard.json"

// heardFile is what logs/heard.json holds: the stamp it was read at and
// what was read.
type heardFile struct {
	Version int `json:"version"`
	// Stamp is the transcript's stamp, the video, the speech model, the
	// floor and the files the transcript is read from, see
	// transcriptStamp, and the build that read it, see buildStamp. The
	// words are snapped to the sound on every read, and where the lines
	// start and end follows from the snapping, so a new build reads the
	// transcript again.
	Stamp string       `json:"stamp"`
	Done  bool         `json:"done"`
	Heard [][2]float64 `json:"heard"`
	Said  *[2]float64  `json:"said,omitempty"`
}

// mostHeardParts is the most parts a kept file may say were heard. The
// hearing leaves a few parts at most, one for each place a person
// searched before the whole video was heard.
const mostHeardParts = 1 << 14

// heardStamp is the stamp what is heard is kept by: the transcript's,
// and the build that read it.
func heardStamp(at transcriptAt) string { return at.stamp + buildStamp() }

// readKeptHeard is what is kept as heard for the video at source, when it
// was kept at stamp and says what a transcript can say. The file is
// outside bytes: anyone can write a work folder.
func readKeptHeard(fsys afero.Fs, source, stamp string) (heardSoFar, bool) {
	data, err := afero.ReadFile(fsys, filepath.Join(LogsDir(source), heardName))
	if err != nil {
		return heardSoFar{}, false
	}
	return parseKeptHeard(data, stamp)
}

// parseKeptHeard reads the bytes of a kept heard.json for the transcript
// at stamp. The parts are in order and apart, the way a transcript says
// them, and the times are inside the longest video there can be.
func parseKeptHeard(data []byte, stamp string) (heardSoFar, bool) {
	var file heardFile
	if decodeJSON(data, &file) != nil || file.Version != heardVersion || file.Stamp != stamp ||
		file.Heard == nil || len(file.Heard) > mostHeardParts {
		return heardSoFar{}, false
	}
	within := func(v float64) bool { return v >= 0 && v <= MaxEpisodeSeconds && !math.IsNaN(v) }
	last := 0.0
	for _, p := range file.Heard {
		if !within(p[0]) || !within(p[1]) || p[0] < last || p[1] <= p[0] {
			return heardSoFar{}, false
		}
		last = p[1]
	}
	if s := file.Said; s != nil && (!within(s[0]) || !within(s[1]) || s[1] < s[0]) {
		return heardSoFar{}, false
	}
	return heardSoFar{done: file.Done, heard: file.Heard, said: file.Said}, true
}

// keepHeard writes what the transcript at stamp has heard into the logs
// folder of the video at source, which is there, since the transcript was
// just read from it. What is not written is read from the transcript
// again at the next start, which is all it costs, so it is not flushed to
// the drive either.
func keepHeard(fsys afero.Fs, source, stamp string, h heardSoFar) {
	logs := LogsDir(source)
	if !fileio.Exists(fsys, logs) {
		return
	}
	heard := h.heard
	if heard == nil {
		heard = [][2]float64{}
	}
	body, err := json.Marshal(heardFile{Version: heardVersion, Stamp: stamp, Done: h.done, Heard: heard, Said: h.said})
	if err != nil {
		return
	}
	_ = fileio.WriteUnflushed(fsys, filepath.Join(logs, heardName), body)
}
