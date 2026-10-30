package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/engine/timed"
)

// Times as the caption and transcript files the program writes spell them.
func TestCaptionTimesRoundTrip(t *testing.T) {
	cases := []struct {
		seconds float64
		srt     string
		ass     string
	}{
		{0, "00:00:00,000", "0:00:00.00"},
		{1.5, "00:00:01,500", "0:00:01.50"},
		{83.25, "00:01:23,250", "0:01:23.25"},
		{3723.456, "01:02:03,456", "1:02:03.46"},
		{-5, "00:00:00,000", "0:00:00.00"},
	}
	for _, c := range cases {
		if got := SecondsToSRT(c.seconds); got != c.srt {
			t.Errorf("SecondsToSRT(%v) = %s, want %s", c.seconds, got, c.srt)
		}
		if got := SecondsToASS(c.seconds); got != c.ass {
			t.Errorf("SecondsToASS(%v) = %s, want %s", c.seconds, got, c.ass)
		}
	}
}

// On the real disk: an srt file is for a person to open in another
// program.
func TestWriteSRTWritesTheCues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "words.srt")
	cues := []timed.Cue{{Start: 0, End: 1.234, Text: "Erste Zeile"}, {Start: 1.5, End: 3, Text: "Zweite, mit Komma"}}
	if err := WriteSRT(cues, path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:00,000 --> 00:00:01,234\nErste Zeile\n\n2\n00:00:01,500 --> 00:00:03,000\nZweite, mit Komma\n"
	if !strings.HasPrefix(string(body), want) {
		t.Errorf("the file is\n%q\nwant it to start\n%q", body, want)
	}
}
