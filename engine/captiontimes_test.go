package engine

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two captions in two pieces of a clip: "Als Kind stand ich dort." from 10
// to 12 in the episode, 0 to 2 in the clip, and "Und dann" from 20.2, 3.2
// in the clip.
func timedClip() Clip {
	return Clip{
		Segments: []Segment{{Start: 10, End: 13}, {Start: 20, End: 22}},
		Words: []Cue{
			{10.1, 10.4, "Als"}, {10.4, 10.7, "Kind"}, {10.7, 11.2, "stand"},
			{11.2, 11.5, "ich"}, {11.5, 12.0, "dort."},
			{20.2, 20.6, "Und"}, {20.6, 21.4, "dann"},
		},
	}
}

func moved(start, end *float64) CaptionTime { return CaptionTime{Start: start, End: end} }

func at(v float64) *float64 { return &v }

func TestTheClocksOfAClipAndItsEpisodeMeet(t *testing.T) {
	clip := timedClip()
	for _, c := range []struct{ clip, episode float64 }{{0, 10}, {2.5, 12.5}, {3, 20}, {4.2, 21.2}} {
		if got := EpisodeTime(clip, c.clip); !near(got, c.episode) {
			t.Errorf("%.1f in the clip is %.2f in the episode, want %.1f", c.clip, got, c.episode)
		}
		if got := ClipTime(clip, c.episode); !near(got, c.clip) {
			t.Errorf("%.1f in the episode is %.2f in the clip, want %.1f", c.episode, got, c.clip)
		}
	}
	// A moment the clip cuts out is the moment the clip comes back.
	if got := ClipTime(clip, 16); !near(got, 3) {
		t.Errorf("a moment cut out is %.2f in the clip", got)
	}
}

func TestACaptionIsShownEarlier(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, 38)
	// "Und dann" shown 0.15 s before "Und" is heard: 3.05 in the clip. It
	// lies in the pause after the first caption has gone, so nothing else
	// moves.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(20.05), nil)}
	got := Captions(clip, 38)
	if !near(got[1].Start, 3.05) || !near(got[0].End, natural[0].End) {
		t.Errorf("got %+v, was %+v", got, natural)
	}
	// Earlier than the piece it is in begins is a moment the clip cuts out,
	// so it is shown the moment the clip comes back.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(19.8), nil)}
	if got := Captions(clip, 38); !near(got[1].Start, 3.0) {
		t.Errorf("a start in what the clip cuts out: %+v", got[1])
	}
	// Earlier than the caption before it may run: that one goes when this
	// one comes, and never later than it.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(11.9), nil)}
	got = Captions(clip, 38)
	if !near(got[1].Start, 1.9) || got[0].End > got[1].Start+1e-9 {
		t.Errorf("an early start over the caption before: %+v", got)
	}
	// And never before the caption before it has been shown at all.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(20.2): moved(at(0), nil)}
	got = Captions(clip, 38)
	if got[1].Start < got[0].Start+shortestMoved-1e-9 {
		t.Errorf("a caption was put before the one before it: %+v", got)
	}
}

func TestACaptionGoesEarlierAndStaysLonger(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, 38)
	// Gone half a second after "dort." starts: the next is where it was.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(12.0))}
	got := Captions(clip, 38)
	if !near(got[0].End, 2.0) || !near(got[1].Start, natural[1].Start) {
		t.Errorf("gone earlier: %+v", got)
	}
	// Longer than the next one lets it: it stops where the next begins.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(21))}
	got = Captions(clip, 38)
	if !near(got[0].End, got[1].Start) {
		t.Errorf("stayed over the next caption: %+v", got)
	}
	// And a caption never ends before it began.
	clip.CaptionTimes = map[string]CaptionTime{wordKey(11.5): moved(nil, at(0))}
	got = Captions(clip, 38)
	if got[0].End < got[0].Start+shortestMoved-1e-9 {
		t.Errorf("ended before it began: %+v", got)
	}
}

// A timing kept against a word that no longer begins a caption, because
// the captions break elsewhere at another size, is simply not used.
func TestATimingOnAWordInsideACaptionIsNotUsed(t *testing.T) {
	clip := timedClip()
	natural := Captions(clip, 38)
	clip.CaptionTimes = map[string]CaptionTime{wordKey(10.7): moved(at(10.9), at(11.0))}
	got := Captions(clip, 38)
	for i := range got {
		if !near(got[i].Start, natural[i].Start) || !near(got[i].End, natural[i].End) {
			t.Errorf("caption %d moved by a word in its middle: %+v", i, got[i])
		}
	}
}

func TestSetCaptionTimeKeepsItAgainstTheWord(t *testing.T) {
	path := editablePlanPath(t)
	captions := filepath.Join(filepath.Dir(filepath.Dir(path)), "captions")
	if err := os.MkdirAll(captions, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(captions, "01_eins.srt")
	if err := os.WriteFile(file, []byte("1\n00:00:00,000 --> 00:00:01,000\neins\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetCaptionTime(path, "01", 10.0, "start", 9.8); err != nil {
		t.Fatal(err)
	}
	if err := SetCaptionTime(path, "01", 12.0, "end", 12.6); err != nil {
		t.Fatal(err)
	}
	c := clipsOf(t, path)["01"]
	if got := c.CaptionTimes[wordKey(10.0)].Start; got == nil || !near(*got, 9.8) {
		t.Errorf("start %v", got)
	}
	if got := c.CaptionTimes[wordKey(12.0)].End; got == nil || !near(*got, 12.6) {
		t.Errorf("end %v", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the caption file was kept, so the next render would not move the caption")
	}
	// Put back where the words put it, and nothing is left behind.
	if err := SetCaptionTime(path, "01", 10.0, "start", math.NaN()); err != nil {
		t.Fatal(err)
	}
	if err := SetCaptionTime(path, "01", 12.0, "end", math.NaN()); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "caption_times") {
		t.Errorf("an empty record of moved captions was left:\n%s", body)
	}
	// Only a word of the clip, and only a moment of an episode.
	if err := SetCaptionTime(path, "01", 20.1, "start", 20); err == nil {
		t.Error("a word of another clip was taken")
	}
	if err := SetCaptionTime(path, "01", 10.0, "middle", 10); err == nil {
		t.Error("an edge that is not an edge was taken")
	}
	if err := SetCaptionTime(path, "01", 10.0, "start", math.Inf(1)); err == nil {
		t.Error("a moment that is not one was taken")
	}
}

func TestCaptionTimesFromAPlanAreChecked(t *testing.T) {
	got := readCaptionTimes(map[string]any{
		"10100":  map[string]any{"start": 9.9},
		"x":      map[string]any{"start": 1.0},
		"010100": map[string]any{"start": 1.0},
		"-5":     map[string]any{"start": 1.0},
		"11500":  map[string]any{"end": -1.0, "start": "soon"},
		"12000":  "not an object",
	})
	if len(got) != 1 || got["10100"].Start == nil || *got["10100"].Start != 9.9 {
		t.Errorf("got %+v", got)
	}
}
