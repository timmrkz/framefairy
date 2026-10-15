package engine

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// keyedFrames is the loudness of captionHeard, with a soft stretch before
// "Typ" that is sound with the silence setting at -50 dB and silence at
// -40 dB, so the setting moves where "Typ" starts.
func keyedFrames() (frames []float32, start float64) {
	start = 59.0
	for i := range 800 {
		at := start + float64(i)*FrameSeconds
		level := float32(-70)
		for _, w := range captionHeard() {
			if at >= w.Start && at < w.End {
				level = -20
			}
		}
		if at >= 61.2 && at < 61.6 {
			level = -45
		}
		frames = append(frames, level)
	}
	return frames, start
}

func keyedTranscript(silenceDB float64) *Transcript {
	frames, start := keyedFrames()
	return fromStored(captionHeard(), frames, start, -30, &silenceDB)
}

// clipSays is the words a clip's captions show, in order.
func clipSays(t *testing.T, fsys afero.Fs, path string, tr *Transcript) string {
	t.Helper()
	view, err := ClipCaptionsView(fsys, path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range view.Captions {
		out = append(out, cueWords(c))
	}
	return strings.Join(out, " | ")
}

// A clip's corrections and removed words stay with their words when the
// silence setting moves where the words start. They were kept by where a
// word starts once moved onto the sound, so a change of the setting left
// them meeting no word, and the clip read as it was heard again without a
// word said. Plan row 2.254.
func TestWordFixesHoldWhenTheSilenceSettingChanges(t *testing.T) {
	loud, quiet := keyedTranscript(-50), keyedTranscript(-40)
	typ := func(tr *Transcript) Cue {
		for _, w := range tr.HeardWords {
			if w.Text == "Typ" {
				return w
			}
		}
		t.Fatal("no Typ")
		return Cue{}
	}
	if typ(loud).Start == typ(quiet).Start {
		t.Fatalf("the test needs the setting to move a word, Typ starts at %.3f with both", typ(loud).Start)
	}
	fsys, path := captionPlanPath(t)
	if err := SetWordText(fsys, path, "01", typ(loud).Start, "Kerl", loud); err != nil {
		t.Fatal(err)
	}
	auf := loud.HeardWords[slicesIndex(loud.HeardWords, "auf")]
	if err := RemoveWord(fsys, path, "01", auf.Start, -1, loud); err != nil {
		t.Fatal(err)
	}
	want := clipSays(t, fsys, path, loud)
	if !strings.Contains(want, "Kerl") || strings.Contains(want, "auf") || strings.Contains(want, "Typ") {
		t.Fatalf("the clip reads %q after Typ was made Kerl and auf removed", want)
	}
	if got := clipSays(t, fsys, path, quiet); got != want {
		t.Errorf("with the silence setting changed the clip reads %q, want %q", got, want)
	}
}

// A plan from before 2.254 keeps its words by where they start on the
// sound. It still reads the way it did with the setting it was kept with,
// and the first edit of the clip's words keeps them the way keys are kept
// now, so a change of the setting after it moves nothing.
func TestWordFixesFromBeforeAreReadAndKeptAnew(t *testing.T) {
	loud, quiet := keyedTranscript(-50), keyedTranscript(-40)
	var typAt, aufAt float64
	for _, w := range quiet.HeardWords {
		switch w.Text {
		case "Typ":
			typAt = w.Start
		case "auf":
			aufAt = w.Start
		}
	}
	fsys, path := writePlanAt(t, `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "clips": [
    {"id": "01", "slug": "schulhof",
     "segments": [{"start": 59.9, "end": 66.0}],
     "corrections": {"`+wordKey(typAt)+`": "Kerl"},
     "removed_words": ["`+wordKey(aufAt)+`"]}
  ]
}`)
	said := clipSays(t, fsys, path, quiet)
	if !strings.Contains(said, "Kerl") || strings.Contains(said, "auf") {
		t.Fatalf("a plan from before reads %q", said)
	}
	// Any edit of the clip's words keeps them anew.
	und := quiet.HeardWords[slicesIndex(quiet.HeardWords, "Und")]
	if err := SetWordText(fsys, path, "01", und.Start, "Und so", quiet); err != nil {
		t.Fatal(err)
	}
	want := clipSays(t, fsys, path, quiet)
	if got := clipSays(t, fsys, path, loud); got != want {
		t.Errorf("after an edit and a change of the setting the clip reads %q, want %q", got, want)
	}
}

func slicesIndex(words []Cue, text string) int {
	for i, w := range words {
		if w.Text == text {
			return i
		}
	}
	return -1
}
