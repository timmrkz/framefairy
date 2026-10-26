package engine

import (
	"strings"
)

// CorrectedTranscript is what the episode says, with every word corrected
// in the caption box of a clip read the way it was corrected: the
// transcript of Export transcript in a video's menu. A correction is a
// clip's, see Transcript.Of, so the clips are laid over each other, the
// clips that are not removed, in the order given. Where two clips correct
// the same word differently, the later one is read. A word removed from a
// clip's captions is still said, so it stays, see RemovedWords.
func CorrectedTranscript(t *Transcript, clips []Clip) *Transcript {
	merged := map[string]string{}
	for _, c := range clips {
		if c.Rejected {
			continue
		}
		for key, text := range c.Corrections {
			merged[key] = text
		}
	}
	return t.Of(Clip{Corrections: merged})
}

// AllClips is every clip of the summary's plan, as it was read for the
// summary, for CorrectedTranscript.
func (s PlanSummary) AllClips() []Clip { return s.clips }

// TranscriptText is the transcript as plain text for a person to read:
// sentences in paragraphs, the way the model reads them, see
// writeSentences, each paragraph opening on a line of its own with the
// time it starts at, and a blank line between two paragraphs. A paragraph
// ends at a pause of paragraphPause or once it has run paragraphSeconds.
// No words gives no text.
func TranscriptText(t *Transcript) string {
	lines := BuildLines(t.Words, nil, nil)
	var out strings.Builder
	paragraphStart := -1.0
	for n, unit := range sentenceUnits(lines) {
		first := lines[unit[0]]
		if n == 0 || first.GapBefore >= paragraphPause || first.Start()-paragraphStart >= paragraphSeconds {
			if n > 0 {
				out.WriteString("\n\n")
			}
			paragraphStart = first.Start()
			out.WriteString(clockTime(first.Start()) + "\n")
		} else {
			out.WriteString(" ")
		}
		for k, line := range lines[unit[0] : unit[1]+1] {
			if k > 0 {
				out.WriteString(" ")
			}
			out.WriteString(line.Text())
		}
	}
	if out.Len() > 0 {
		out.WriteString("\n")
	}
	return out.String()
}

// A subtitle of the transcript exported as subtitles is at most two lines
// of subtitleChars and runs at most subtitleSeconds, the size a subtitle
// tool expects and a viewer reads in its time.
const (
	subtitleChars   = 42
	subtitleSeconds = 7.0
)

// TranscriptSRT is the transcript as subtitles, the text of an srt file:
// the lines the model reads, see BuildLines, each cut at a word into
// subtitles that fit in two lines of subtitleChars and run at most
// subtitleSeconds. No words gives no text.
func TranscriptSRT(t *Transcript) string {
	var cues []Cue
	for _, line := range BuildLines(t.Words, nil, nil) {
		var words []string
		start, end := 0.0, 0.0
		flush := func() {
			if len(words) > 0 {
				text, _ := twoLines(Scrub(strings.Join(words, " "), maxLineChars))
				cues = append(cues, Cue{Start: start, End: end, Text: text})
			}
			words = words[:0]
		}
		for _, w := range line.Cues {
			if len(words) > 0 {
				_, fits := twoLines(strings.Join(append(words, w.Text), " "))
				if !fits || w.End-start > subtitleSeconds {
					flush()
				}
			}
			if len(words) == 0 {
				start = w.Start
			}
			words = append(words, w.Text)
			end = w.End
		}
		flush()
	}
	return SRT(cues)
}

// twoLines lays a subtitle out in one line of at most subtitleChars, or
// in two, broken at the space that makes them most nearly as long, and
// says whether it fits. A single word too long for a line is a line of
// its own all the same.
func twoLines(text string) (string, bool) {
	runes := []rune(text)
	if len(runes) <= subtitleChars {
		return text, true
	}
	best, off := -1, 0
	for i, r := range runes {
		if r != ' ' || i > subtitleChars || len(runes)-i-1 > subtitleChars {
			continue
		}
		d := i - (len(runes) - i - 1)
		if d < 0 {
			d = -d
		}
		if best < 0 || d < off {
			best, off = i, d
		}
	}
	if best < 0 {
		return text, !strings.Contains(text, " ")
	}
	return string(runes[:best]) + "\n" + string(runes[best+1:]), true
}
