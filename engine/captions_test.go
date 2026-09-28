package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Caption files are the one part of a clip a person edits by hand, in
// whatever editor they like, so they come back with any line ending, any
// encoding accident and any markup their editor felt like adding.

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
		if c.seconds < 0 {
			continue
		}
		back, err := clockToSeconds(c.srt)
		if err != nil || math.Abs(back-c.seconds) > 0.001 {
			t.Errorf("reading %s back gave %v, %v", c.srt, back, err)
		}
	}
	for _, bad := range []string{"", "12", "00:00", "eins", "00:00:00", "1:2:3:4,000"} {
		if _, err := clockToSeconds(bad); err == nil {
			t.Errorf("clockToSeconds(%q) was accepted", bad)
		}
	}
}

func TestLoadSRTTakesWhatItCanAndCleansIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01_a.srt")
	body := "\ufeff1\r\n00:00:01,000 --> 00:00:02,000\r\n<i>Als</i> Kind\r\n\r\n" +
		"2\n00:00:03,000 --> 00:00:04,000 X1:0 X2:100\nzweite &amp; dritte\nzeile\n\n" +
		"3\nkeine zeit\nverloren\n\n" +
		"00:00:05,000 --> 00:00:06,000\nohne nummer\n\n" +
		"5\n00:00:07,000 --> 00:00:08,000\n\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cues, err := LoadSRT(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Cue{
		{1, 2, "Als Kind"},
		// Entities are left as they are: what was written is what comes
		// back, around every edit.
		{3, 4, "zweite &amp; dritte zeile"},
		{5, 6, "ohne nummer"},
	}
	if len(cues) != len(want) {
		t.Fatalf("got %v", cues)
	}
	for i, w := range want {
		if cues[i] != w {
			t.Errorf("cue %d = %+v, want %+v", i, cues[i], w)
		}
	}
}

func TestWriteSRTAndReadItBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "01_a.srt")
	cues := []Cue{{0, 1.234, "Erste Zeile"}, {1.5, 3, "Zweite, mit Komma"}}
	if err := WriteSRT(cues, path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSRT(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	for i, c := range cues {
		if got[i].Text != c.Text || math.Abs(got[i].Start-c.Start) > 0.001 ||
			math.Abs(got[i].End-c.End) > 0.001 {
			t.Errorf("cue %d came back as %+v, want %+v", i, got[i], c)
		}
	}
}

// FuzzLoadSRT reads whatever the editor left behind. Whatever that is, the
// cues have to be sorted, timed and free of anything that could repaint a
// terminal, because this text goes into proof.txt and into the log.
func FuzzLoadSRT(f *testing.F) {
	f.Add("1\n00:00:01,000 --> 00:00:02,000\nAls Kind\n")
	f.Add("\ufeff1\r\n00:00:01,000 --> 00:00:02,000\r\n<b>fett</b>\r\n")
	f.Add("00:00:09,000 --> 00:00:08,000\nrückwärts\n\n--> \n\n")
	f.Add("1\n0:0:0.0 --> 99:99:99,999\n\x1b[2Jverwischt\n")
	f.Fuzz(func(t *testing.T, body string) {
		path := filepath.Join(t.TempDir(), "01_a.srt")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Skip()
		}
		cues, err := LoadSRT(path)
		if err != nil {
			return
		}
		for i, cue := range cues {
			if !isFinite(cue.Start) || !isFinite(cue.End) || cue.Start < 0 {
				t.Fatalf("cue %d is timed %v-%v", i, cue.Start, cue.End)
			}
			if i > 0 && cue.Start < cues[i-1].Start {
				t.Fatalf("cue %d starts before the one before it", i)
			}
			if cue.Text == "" || strings.ContainsFunc(cue.Text, isControl) ||
				strings.ContainsAny(cue.Text, "\n\r") {
				t.Fatalf("cue %d carries %q", i, cue.Text)
			}
		}
	})
}
