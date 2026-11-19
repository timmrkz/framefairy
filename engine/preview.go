package engine

import (
	"strings"

	"framefairy/engine/runes"
)

// tail keeps the last lines ffmpeg said that were not about a frame, for
// the reason when it fails.
type tail struct {
	lines []string
}

func (t *tail) add(line string) {
	t.lines = append(t.lines, line)
	if len(t.lines) > 8 {
		t.lines = t.lines[1:]
	}
}

func (t *tail) String() string {
	return runes.Scrub(strings.Join(t.lines, " "), 400)
}
