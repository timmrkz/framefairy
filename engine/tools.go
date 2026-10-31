package engine

import (
	"path/filepath"
	"strings"
)

// UseTools points the engine at an ffmpeg and an ffprobe that were chosen,
// on the command line or in the app's settings. An empty one leaves what
// NewEngine found. With only ffmpeg chosen, ffprobe is looked for beside
// it, under the name the system gives it, since the two come together.
func (e *Engine) UseTools(ffmpeg, ffprobe string) {
	if ffmpeg != "" {
		e.FFmpeg = ffmpeg
	}
	if ffprobe != "" {
		e.FFprobe = ffprobe
		return
	}
	if ffmpeg == "" {
		return
	}
	name := "ffprobe"
	if strings.EqualFold(filepath.Ext(ffmpeg), ".exe") {
		name = "ffprobe.exe"
	}
	if guess := filepath.Join(filepath.Dir(ffmpeg), name); exists(disk, guess) {
		e.FFprobe = guess
	}
}
