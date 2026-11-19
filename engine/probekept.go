package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"slices"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
)

// A video's probe is kept in its work folder, logs/probe.json, beside its
// transcript and its loudness, so a start of the app asks ffprobe nothing
// about a video it asked before. In memory alone it died with the app, and
// every start ran ffprobe five times for every open video while the window
// waited, issue #891. See
// docs/decisions/2026-11-19-a-videos-probe-kept-in-its-work-folder.md.

// probeVersion is the version of logs/probe.json. A file of another
// version is not read, and the next probe writes it again.
const probeVersion = 1

// probeName is the file a video's probe is kept in, in its logs folder.
const probeName = "probe.json"

// probeFile is what logs/probe.json holds: the video it was made from, the
// ffprobe that answered, and the answer.
type probeFile struct {
	Version int         `json:"version"`
	Source  sourceStamp `json:"source"`
	Tool    string      `json:"tool"`
	Info    keptInfo    `json:"info"`
}

// keptInfo is a SourceInfo as it is kept, its own fields and the ones the
// engine keeps to itself. What a render learns of each piece it cuts,
// unevenAt and keyAt, is not kept: it is learned again by the piece.
type keptInfo struct {
	Width         int         `json:"width"`
	Height        int         `json:"height"`
	Turned        int         `json:"turned"`
	FPSNum        int         `json:"fpsNum"`
	FPSDen        int         `json:"fpsDen"`
	Duration      float64     `json:"duration"`
	VideoStart    float64     `json:"videoStart"`
	Variable      bool        `json:"variable"`
	Colour        [][2]string `json:"colour"`
	SoundRate     int         `json:"soundRate"`
	SoundChannels int         `json:"soundChannels"`
	Origin        float64     `json:"origin"`
	HasClock      bool        `json:"hasClock"`
	Unindexed     bool        `json:"unindexed"`
}

// toolStamp is how the kept probe names the ffprobe that answered: its
// size and the time it was written, so a new build of the app, with an
// ffprobe of its own, asks again.
func toolStamp(tool fileStamp) string {
	return fmt.Sprintf("%d %d", tool.size, tool.mod.UnixNano())
}

// readKeptProbe is the probe kept for the video at source, when it was
// made from the video as it is now by the same ffprobe, and every field
// is one a probe gives. The file is outside bytes: anyone can write a work
// folder, and its colour tags are handed to ffmpeg.
func readKeptProbe(fsys afero.Fs, source, tool string) (SourceInfo, bool) {
	if fsys == nil {
		return SourceInfo{}, false
	}
	data, err := afero.ReadFile(fsys, filepath.Join(LogsDir(source), probeName))
	if err != nil {
		return SourceInfo{}, false
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		return SourceInfo{}, false
	}
	return parseKeptProbe(data, stamp, tool)
}

// parseKeptProbe reads the bytes of a kept probe for a video of the given
// stamp, asked of the given ffprobe.
func parseKeptProbe(data []byte, stamp sourceStamp, tool string) (SourceInfo, bool) {
	var file probeFile
	if decodeJSON(data, &file) != nil || file.Version != probeVersion || file.Source != stamp || file.Tool != tool {
		return SourceInfo{}, false
	}
	k := file.Info
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if k.Width <= 0 || k.Height <= 0 || k.Width > maxSide || k.Height > maxSide ||
		k.FPSNum <= 0 || k.FPSDen <= 0 || k.FPSNum > maxRate || k.FPSDen > maxRate ||
		!slices.Contains([]int{0, 90, 180, 270}, k.Turned) ||
		!finite(k.Duration) || k.Duration < 0 || k.Duration > MaxEpisodeSeconds ||
		!finite(k.VideoStart) || !finite(k.Origin) || math.Abs(k.VideoStart) > MaxEpisodeSeconds || math.Abs(k.Origin) > MaxEpisodeSeconds ||
		k.SoundRate < 0 || k.SoundChannels < 0 || k.SoundRate > maxRate || k.SoundChannels > 64 ||
		len(k.Colour) > 3 {
		return SourceInfo{}, false
	}
	for _, c := range k.Colour {
		if !slices.Contains([]string{"color_primaries", "color_trc", "colorspace"}, c[0]) || !plainWord(c[1]) {
			return SourceInfo{}, false
		}
	}
	return SourceInfo{Width: k.Width, Height: k.Height, Turned: k.Turned, FPSNum: k.FPSNum, FPSDen: k.FPSDen,
		Duration: k.Duration, VideoStart: k.VideoStart, Variable: k.Variable, Colour: slices.Clone(k.Colour),
		SoundRate: k.SoundRate, SoundChannels: k.SoundChannels,
		origin: k.Origin, hasClock: k.HasClock, unindexed: k.Unindexed}, true
}

// The most a kept probe may say: a side of a picture, and a rate's
// numerator or denominator or a sound's samples a second.
const (
	maxSide = 1 << 15
	maxRate = 1 << 24
)

// plainWord is a colour tag's value as ffprobe names one, bt709 or
// arib-std-b67: a few letters, digits, dashes and dots, beginning with a
// letter or a digit, so it never reads as a flag of ffmpeg's.
func plainWord(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for i, r := range s {
		letter := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if !letter && (i == 0 || r != '-' && r != '_' && r != '.') {
			return false
		}
	}
	return true
}

// keepProbe writes the probe of the video at source into the logs folder
// of its work folder, when there is one. A short or a file the command
// line was handed has none, and no folder is made for it: one made here
// could bring back a work folder a removal had just deleted. What is not
// written is asked of ffprobe again at the next start, which is all it
// costs, so it is not flushed to the drive either.
func keepProbe(fsys afero.Fs, source, tool string, info SourceInfo) {
	logs := LogsDir(source)
	if fsys == nil || !fileio.Exists(fsys, logs) {
		return
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		return
	}
	body, err := json.Marshal(probeFile{Version: probeVersion, Source: stamp, Tool: tool, Info: keptInfo{
		Width: info.Width, Height: info.Height, Turned: info.Turned, FPSNum: info.FPSNum, FPSDen: info.FPSDen,
		Duration: info.Duration, VideoStart: info.VideoStart, Variable: info.Variable, Colour: info.Colour,
		SoundRate: info.SoundRate, SoundChannels: info.SoundChannels,
		Origin: info.origin, HasClock: info.hasClock, Unindexed: info.unindexed,
	}})
	if err != nil {
		return
	}
	_ = fileio.WriteUnflushed(fsys, filepath.Join(logs, probeName), body)
}
