package engine

import (
	"fmt"
	"strings"
	"testing"
)

// said makes lines from text, each spoken over the seconds given after the
// pause before it, one word after another.
func said(parts ...any) []Line {
	var lines []Line
	at := 0.0
	for i := 0; i < len(parts); i += 3 {
		gap, seconds, text := parts[i].(float64), parts[i+1].(float64), parts[i+2].(string)
		at += gap
		words := strings.Fields(text)
		step := seconds / float64(len(words))
		var cues []Cue
		for k, w := range words {
			cues = append(cues, Cue{Start: at + float64(k)*step, End: at + float64(k+1)*step, Text: w})
		}
		at += seconds
		lines = append(lines, Line{Index: len(lines) + 1, Cues: cues, GapBefore: gap})
	}
	return lines
}

// The umbrella story from Tim's episode, as the transcript has it. Five
// searches cut it five ways. Each edge lands on a sentence.
func TestAClipStartsAndEndsOnASentence(t *testing.T) {
	lines := said(
		0.0, 3.5, "Und ich war dann noch ein relativ kleiner Dütz, zweite, dritte Klasse.", // 1
		1.4, 0.1, "Und", // 2
		1.1, 0.7, "irgendein Typ", // 3
		0.5, 1.8, "auf dem Schulhof gemobbt.", // 4
		0.5, 0.3, "Und", // 5
		1.3, 2.3, "zum Typen mit so einem Regenschirm. Und", // 6
		3.7, 4.9, "Er gesagt auf und dann hat er mich dort geschlagen und", // 7
		1.0, 0.1, "dieser", // 8
		0.5, 2.4, "Regenschirm ist zersprungen.", // 9
		0.8, 7.2, "Also hat sich einfach zerlegt, ne? Das war eine der ersten Erinnerungen,", // 10
		1.3, 1.6, "die ich habe, aber davor", // 11
		2.4, 0.3, "Echt,", // 12
		0.5, 0.8, "echt wenig.", // 13
		1.5, 1.4, "Äh, am Start.", // 14
	)
	for _, c := range []struct{ keep, want string }{
		// It started on "irgendein Typ" and ended on a comma: the sentence
		// before it is a second back, and the one it stopped in ends seven
		// seconds on.
		{"[[3 10]]", "[[2 13]]"},
		// It stopped on "aber davor": two seconds to the end of the sentence.
		{"[[2 11]]", "[[2 13]]"},
		// Started mid-sentence on "zum Typen": the sentence began at 5.
		{"[[6 9]]", "[[5 9]]"},
		// Whole sentences stay as they are.
		{"[[2 9]]", "[[2 9]]"},
		// "dieser" is inside a sentence that began 13 seconds back, past
		// the reach, so that edge stands.
		{"[[2 4] [8 9]]", "[[2 4] [8 9]]"},
		// A cut inside a clip lands between sentences too.
		{"[[2 3] [4 9]]", "[[2 4] [5 9]]"},
		// Two runs that overlap once they are whole sentences are one.
		{"[[2 3] [3 9]]", "[[2 9]]"},
	} {
		keep := parseRuns(t, c.keep)
		if got := fmt.Sprint(wholeSentences(lines, keep)); got != c.want {
			t.Errorf("%s became %s, not %s", c.keep, got, c.want)
		}
	}
}

// Past the reach, the model's edge stands: a transcript can go a long way
// without a full stop, and a clip is not doubled to find one.
func TestAnEdgeFarFromASentenceStands(t *testing.T) {
	lines := said(
		0.0, 2.0, "Das war so", 0.5, 9.0, "und dann kam noch einer und noch einer und dann",
		0.5, 9.0, "ging es immer weiter so ohne Ende und wieder", 0.5, 2.0, "vorbei.")
	if got := fmt.Sprint(wholeSentences(lines, [][2]int{{2, 2}})); got != "[[1 2]]" {
		t.Errorf("got %s", got)
	}
}

func parseRuns(t *testing.T, s string) [][2]int {
	t.Helper()
	var runs [][2]int
	for _, part := range strings.Split(strings.Trim(s, "[]"), "] [") {
		var a, b int
		if _, err := fmt.Sscanf(part, "%d %d", &a, &b); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		runs = append(runs, [2]int{a, b})
	}
	return runs
}

// A clip still far too long after the model was asked again loses whole
// sentences from its start until it fits. The end, the payoff, stays.
func TestAClipTooLongLosesSentencesFromItsStart(t *testing.T) {
	lines := said(
		0.0, 10.0, "Das war damals in der Schule so.", // 1
		0.5, 10.0, "Wir hatten eine Bücherwand zu Hause.", // 2
		0.5, 12.0, "Und dann habe ich die ganze Nacht gelesen", // 3
		0.5, 12.0, "bis es hell war.", // 4
	)
	seconds := func(keep [][2]int) float64 {
		total := 0.0
		for _, r := range keep {
			total += lines[r[1]-1].End() - lines[r[0]-1].Start()
		}
		return total
	}
	// 45 seconds, and 36 at most: the first sentence is enough to go.
	if got := fmt.Sprint(fromTheStart(lines, [][2]int{{1, 4}}, 36, seconds)); got != "[[2 4]]" {
		t.Errorf("got %s", got)
	}
	// At 30 at most, the second goes too.
	if got := fmt.Sprint(fromTheStart(lines, [][2]int{{1, 4}}, 30, seconds)); got != "[[3 4]]" {
		t.Errorf("got %s", got)
	}
	// A clip that fits is left alone, and one sentence too long for any
	// cut stays as it is.
	if got := fmt.Sprint(fromTheStart(lines, [][2]int{{2, 4}}, 36, seconds)); got != "[[2 4]]" {
		t.Errorf("got %s", got)
	}
	if got := fmt.Sprint(fromTheStart(lines, [][2]int{{3, 4}}, 20, seconds)); got != "[[3 4]]" {
		t.Errorf("got %s", got)
	}
	// With two runs, the first goes before the second is touched.
	if got := fmt.Sprint(fromTheStart(lines, [][2]int{{1, 1}, {3, 4}}, 30, seconds)); got != "[[3 4]]" {
		t.Errorf("got %s", got)
	}
}
