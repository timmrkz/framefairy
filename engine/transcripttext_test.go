package engine

import (
	"fmt"
	"strings"
	"testing"

	"framefairy/engine/timed"
)

// The transcript exported is what the video says with the corrections of
// every clip that is not removed laid over it, the later clip read where
// two correct the same word. A word removed from a clip's captions is
// still said, so it stays.
func TestTheExportedTranscriptReadsEveryClipsCorrections(t *testing.T) {
	t.Parallel()
	heard := []timed.Cue{
		{Start: 0.0, End: 0.3, Text: "Ich"},
		{Start: 0.4, End: 0.7, Text: "war"},
		{Start: 0.8, End: 1.1, Text: "drei"},
		{Start: 1.2, End: 1.5, Text: "Jahre"},
		{Start: 1.6, End: 1.9, Text: "alt."},
		{Start: 4.0, End: 4.3, Text: "Meine"},
		{Start: 4.4, End: 4.7, Text: "Mutta"},
		{Start: 4.8, End: 5.1, Text: "lachte."},
	}
	tr := fromStored(heard, nil, 0, 0, nil)
	key := func(i int) string { return tr.keyAt(i) }
	clips := []Clip{
		{Segments: []Segment{{Start: 0, End: 2}}, Corrections: map[string]string{key(2): "vier"},
			RemovedWords: RemovedWords{key(1): true}},
		// Removed, so its corrections went with it.
		{Rejected: true, Segments: []Segment{{Start: 3.9, End: 5.2}}, Corrections: map[string]string{key(5): "Deine"}},
		{Segments: []Segment{{Start: 3.9, End: 5.2}}, Corrections: map[string]string{key(6): "Muter"}},
		// The later of two clips that correct the same word.
		{Segments: []Segment{{Start: 4.3, End: 5.2}}, Corrections: map[string]string{key(6): "Mutter"}},
	}
	got := TranscriptText(CorrectedTranscript(tr, clips))
	want := "0:00\nIch war vier Jahre alt.\n\n0:04\nMeine Mutter lachte.\n"
	if got != want {
		t.Errorf("the transcript reads\n%q\nwant\n%q", got, want)
	}
	// The transcript itself still reads what was heard.
	if got := TranscriptText(tr); !strings.Contains(got, "drei") || !strings.Contains(got, "Mutta") {
		t.Errorf("the corrections reached the transcript itself: %q", got)
	}
}

// Sentences close together stay in one paragraph, and a paragraph that
// has run long enough ends at the next sentence. No words is no text.
func TestTheExportedTranscriptIsInParagraphs(t *testing.T) {
	t.Parallel()
	var words []timed.Cue
	at := 0.0
	for i := range 40 {
		text := "eins"
		if i%4 == 3 {
			text = "Satz."
		}
		words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: text})
		at += 1.4
	}
	got := TranscriptText(&Transcript{Words: words})
	paragraphs := strings.Split(strings.TrimSuffix(got, "\n"), "\n\n")
	if len(paragraphs) != 2 || !strings.HasPrefix(paragraphs[0], "0:00\neins eins eins Satz. eins") ||
		!strings.HasPrefix(paragraphs[1], "0:50\n") {
		t.Errorf("the transcript reads\n%s", got)
	}
	if got := TranscriptText(&Transcript{}); got != "" {
		t.Errorf("no words read %q", got)
	}
}

// The transcript as subtitles: a subtitle never runs longer than two lines
// or subtitleSeconds, a long one is two lines about as long, and every
// word is in one subtitle, in order.
func TestTheExportedSubtitlesAreShort(t *testing.T) {
	t.Parallel()
	var words []timed.Cue
	at := 0.0
	for i := range 60 {
		text := "Erinnerung"
		if i%15 == 14 {
			text = "Ende."
		}
		words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: text})
		at += 0.35
	}
	srt := TranscriptSRT(&Transcript{Words: words})
	blocks := strings.Split(strings.TrimSpace(srt), "\n\n")
	if len(blocks) < 3 || !strings.HasPrefix(srt, "1\n00:00:00,000 --> ") {
		t.Fatalf("subtitles:\n%s", srt)
	}
	said := 0
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		text := lines[2:]
		if len(text) > 2 {
			t.Errorf("a subtitle of %d lines:\n%s", len(text), block)
		}
		for _, line := range text {
			if len([]rune(line)) > subtitleChars {
				t.Errorf("a line of %d characters: %q", len([]rune(line)), line)
			}
			said += len(strings.Fields(line))
		}
	}
	if said != len(words) {
		t.Errorf("%d words in the subtitles, %d said", said, len(words))
	}
	if got := TranscriptSRT(&Transcript{}); got != "" {
		t.Errorf("no words read %q", got)
	}

	// Short words said slowly would fit two lines for half a minute, so
	// the time is what cuts them, and no subtitle runs into the next.
	// Found by the review of #458, nothing here looked at the times.
	words = words[:0]
	for i := range 40 {
		words = append(words, timed.Cue{Start: float64(i) * 1.2, End: float64(i)*1.2 + 1, Text: "ja"})
	}
	srt = TranscriptSRT(&Transcript{Words: words})
	blocks = strings.Split(strings.TrimSpace(srt), "\n\n")
	ended := -1.0
	for _, block := range blocks {
		times := strings.Split(strings.Split(block, "\n")[1], " --> ")
		from, to := srtSeconds(t, times[0]), srtSeconds(t, times[1])
		if to-from > subtitleSeconds {
			t.Errorf("a subtitle of %.3f s:\n%s", to-from, block)
		}
		if from < ended {
			t.Errorf("a subtitle from %.3f s begins before the one before it ends at %.3f s", from, ended)
		}
		ended = to
	}
	if len(blocks) < 5 {
		t.Errorf("40 words over 48 s in %d subtitles", len(blocks))
	}
}

// srtSeconds reads a time of an srt file, 00:01:02,345.
func srtSeconds(t *testing.T, at string) float64 {
	t.Helper()
	var h, m, sec, ms int
	if _, err := fmt.Sscanf(at, "%d:%d:%d,%d", &h, &m, &sec, &ms); err != nil {
		t.Fatalf("the time %q: %v", at, err)
	}
	return float64(h*3600+m*60+sec) + float64(ms)/1000
}
