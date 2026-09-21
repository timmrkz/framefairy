package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Cue is a timed piece of text: one word, or one caption made of words, with
// its start and end in seconds.
type Cue struct {
	Start float64
	End   float64
	Text  string
}

// Span is a stretch of time, in seconds.
type Span struct {
	Start float64
	End   float64
}

// Reading is one loudness measurement, in dB, at a point in time.
type Reading struct {
	At    float64
	Level float64
}

// --------------------------------------------------------------------------
// time formats
// --------------------------------------------------------------------------

var (
	clockRe = regexp.MustCompile(`^(?:(\d+):)?(\d{1,2}):(\d{1,2})[.,](\d{1,3})$`)
)

func clockToSeconds(value string) (float64, error) {
	value = strip(value)
	if m := clockRe.FindStringSubmatch(value); m != nil {
		hours := 0
		if m[1] != "" {
			hours, _ = strconv.Atoi(m[1])
		}
		minutes, _ := strconv.Atoi(m[2])
		seconds, _ := strconv.Atoi(m[3])
		frac := m[4] + strings.Repeat("0", 3-len(m[4]))
		ms, _ := strconv.Atoi(frac)
		return float64(hours*3600+minutes*60+seconds) + float64(ms)/1000.0, nil
	}
	return 0, fmt.Errorf("unrecognised timestamp: %s", pyRepr(value))
}

// SecondsToSRT formats a time for an srt file.
func SecondsToSRT(t float64) string {
	if t < 0 {
		t = 0
	}
	total := pyround(t * 1000)
	hours, rem := total/3_600_000, total%3_600_000
	minutes, rem := rem/60_000, rem%60_000
	seconds, ms := rem/1000, rem%1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, ms)
}

// SecondsToASS formats a time for an ass file.
func SecondsToASS(t float64) string {
	if t < 0 {
		t = 0
	}
	total := pyround(t * 100)
	hours, rem := total/360_000, total%360_000
	minutes, rem := rem/6_000, rem%6_000
	seconds, cs := rem/100, rem%100
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, cs)
}

// splitLines splits on any line ending, for the separators that survive
// control-character removal.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// --------------------------------------------------------------------------
// parsers
// --------------------------------------------------------------------------

var (
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	blockSplitRe = regexp.MustCompile(`\n[\s\v]*\n`)
)

// cleanCaption strips control characters and markup. Caption text ends up in
// proof.txt and in per-clip srt files, both of which get opened in a
// terminal. An ESC byte surviving that far means a caption can clear the
// screen, retitle the window or repaint earlier output.
//
// It leaves the words themselves alone, including anything that looks like
// an HTML entity. Turning &amp; into & would mean that a caption written and
// read back again is not the caption that was written, and captions are
// written and read back around every edit.
func cleanCaption(text string) string {
	text = removeControl(text)
	text = tagRe.ReplaceAllString(text, "")
	var kept []string
	for _, line := range splitLines(text) {
		if line = strip(line); line != "" {
			kept = append(kept, line)
		}
	}
	return strip(strings.Join(kept, " "))
}

func captionBlocks(raw string) []string {
	return blockSplitRe.Split(strip(strings.ReplaceAll(raw, "\r\n", "\n")), -1)
}

func nonBlankLines(block string) []string {
	var lines []string
	for _, line := range strings.Split(block, "\n") {
		if strip(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseSRT(raw string) []Cue {
	var cues []Cue
	for _, block := range captionBlocks(raw) {
		lines := nonBlankLines(block)
		timing := -1
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				timing = i
				break
			}
		}
		if timing < 0 {
			continue
		}
		left, right, _ := strings.Cut(lines[timing], "-->")
		right = strings.Split(strip(right), " ")[0]
		start, err1 := clockToSeconds(left)
		end, err2 := clockToSeconds(right)
		if err1 != nil || err2 != nil {
			continue
		}
		if text := cleanCaption(strings.Join(lines[timing+1:], "\n")); text != "" {
			cues = append(cues, Cue{start, end, text})
		}
	}
	return cues
}

var breakAfter = []string{".", "!", "?", "…", ":", ";", ","}

func endsWithBreak(s string) bool {
	for _, mark := range breakAfter {
		if strings.HasSuffix(s, mark) {
			return true
		}
	}
	return false
}

func endsSentence(s string) bool {
	s = strings.TrimRight(s, "\"'“”»«)")
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") ||
		strings.HasSuffix(s, "?") || strings.HasSuffix(s, "…")
}

// MaxCaptionBytes is far beyond any real caption file.
const MaxCaptionBytes = 40 * 1024 * 1024

// LoadSRT reads a per-clip caption file, sorted by start.
func LoadSRT(path string) ([]Cue, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxCaptionBytes {
		return nil, fmt.Errorf("%s is %.0f MB, which is far too large for a "+
			"caption file", filepath.Base(path), float64(info.Size())/1e6)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimPrefix(strings.ToValidUTF8(string(data), "\uFFFD"), "\ufeff")
	cues := parseSRT(raw)
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].Start < cues[j].Start })
	return cues, nil
}

// WriteSRT writes cues as an srt file.
func WriteSRT(cues []Cue, path string) error {
	var lines []string
	for i, cue := range cues {
		lines = append(lines, strconv.Itoa(i+1),
			SecondsToSRT(cue.Start)+" --> "+SecondsToSRT(cue.End), cue.Text, "")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
