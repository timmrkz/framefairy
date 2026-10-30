package engine

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"framefairy/engine/pynum"

	"framefairy/engine/timed"
)

// --------------------------------------------------------------------------
// time formats
// --------------------------------------------------------------------------

// SecondsToSRT formats a time for an srt file.
func SecondsToSRT(t float64) string {
	if t < 0 {
		t = 0
	}
	total := pynum.Round(t * 1000)
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
	total := pynum.Round(t * 100)
	hours, rem := total/360_000, total%360_000
	minutes, rem := rem/6_000, rem%6_000
	seconds, cs := rem/100, rem%100
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, cs)
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

// WriteSRT writes cues as an srt file.
func WriteSRT(cues []timed.Cue, path string) error {
	return os.WriteFile(path, []byte(SRT(cues)), 0o644)
}

// SRT is cues as the text of an srt file.
func SRT(cues []timed.Cue) string {
	var lines []string
	for i, cue := range cues {
		lines = append(lines, strconv.Itoa(i+1),
			SecondsToSRT(cue.Start)+" --> "+SecondsToSRT(cue.End), cue.Text, "")
	}
	return strings.Join(lines, "\n")
}
