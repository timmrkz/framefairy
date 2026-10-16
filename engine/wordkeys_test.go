package engine

import (
	"math"
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

// clipReads is the words a clip's captions show, in order, whichever
// caption each is in.
func clipReads(t *testing.T, fsys afero.Fs, path string, tr *Transcript) string {
	t.Helper()
	return strings.ReplaceAll(clipSays(t, fsys, path, tr), " | ", " ")
}

// framesLoud is a loudness from 59 s that is sound where loud says and
// silence everywhere else.
func framesLoud(loud func(at float64) bool) (frames []float32, start float64) {
	start = 59.0
	for i := range 500 {
		level := float32(-70)
		if loud(start + float64(i)*FrameSeconds) {
			level = -20
		}
		frames = append(frames, level)
	}
	return frames, start
}

// clipPlan is a plan of one clip from 59.9 to 63 s, with what its entry
// says beside its segments.
func clipPlan(t *testing.T, entry string) (afero.Fs, string) {
	t.Helper()
	return writePlanAt(t, `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "clips": [
    {"id": "01", "slug": "kamen",
     "segments": [{"start": 59.9, "end": 63.0}]`+entry+`}
  ]
}`)
}

// Two words the recogniser heard start in the same step, the first no
// longer than nothing, are two words to a clip. Kept by the millisecond
// they were heard at, they had one key, so a correction of the first
// changed the second and removing the second removed the first.
// Found by the review of #227.
func TestTwoWordsHeardAtOnceKeepTheirOwnFixes(t *testing.T) {
	t.Parallel()
	floor := -40.0
	frames, start := framesLoud(func(float64) bool { return true })
	tr := fromStored([]Cue{{60.0, 60.4, "Und"}, {61.0, 61.0, "dass"}, {61.0, 61.3, "5000"},
		{61.4, 61.8, "Leute"}, {62.0, 62.4, "kamen"}}, frames, start, -20, &floor)
	at := func(text string) float64 { return tr.HeardWords[slicesIndex(tr.HeardWords, text)].Start }
	fsys, path := clipPlan(t, "")
	if err := SetWordText(fsys, path, "01", at("dass"), "das", tr); err != nil {
		t.Fatal(err)
	}
	if got, want := clipReads(t, fsys, path, tr), "Und das 5000 Leute kamen"; got != want {
		t.Errorf("after dass was made das the clip reads %q, want %q", got, want)
	}
	for _, text := range []string{"Leute", "5000"} {
		if err := RemoveWord(fsys, path, "01", at(text), -1, tr); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := clipReads(t, fsys, path, tr), "Und das kamen"; got != want {
		t.Errorf("after Leute and 5000 were removed the clip reads %q, want %q", got, want)
	}
}

// A key from before 2.254 is where a word starts on the sound, and a short
// word heard in a pause is moved forward to where the sound begins, which
// is where the recogniser heard the next word start. Read as a key of
// now, the fix of the short word landed on the next one, and the first
// edit of the clip kept it there. Found by the review of #227.
func TestAFixFromBeforeStaysOnItsOwnWord(t *testing.T) {
	t.Parallel()
	floor := -40.0
	frames, start := framesLoud(func(at float64) bool {
		return (at >= 60.0 && at < 60.4) || (at >= 61.2 && at < 61.5) || (at >= 62.0 && at < 62.4)
	})
	tr := fromStored([]Cue{{60.0, 60.4, "Und"}, {61.0, 61.08, "ja"}, {61.2, 61.5, "Typ"},
		{62.0, 62.4, "kamen"}}, frames, start, -20, &floor)
	ja := tr.HeardWords[slicesIndex(tr.HeardWords, "ja")]
	if wordKey(ja.Start) != "61200" {
		t.Fatalf("the test needs ja moved to where Typ was heard, it starts at %.3f", ja.Start)
	}
	for _, c := range []struct{ entry, want string }{
		{`, "corrections": {"61200": "jo"}`, "Und jo Typ kamen"},
		{`, "removed_words": ["61200"]`, "Und Typ kamen"},
	} {
		fsys, path := clipPlan(t, c.entry)
		if got := clipReads(t, fsys, path, tr); got != c.want {
			t.Errorf("a plan from before with %s reads %q, want %q", c.entry, got, c.want)
		}
		// An edit of another word keeps the clip's words anew, on the
		// same words.
		if err := SetWordText(fsys, path, "01", tr.HeardWords[0].Start, "Und so", tr); err != nil {
			t.Fatal(err)
		}
		if got, want := clipReads(t, fsys, path, tr), strings.Replace(c.want, "Und", "Und so", 1); got != want {
			t.Errorf("after an edit a plan from before with %s reads %q, want %q", c.entry, got, want)
		}
	}
}

// edgeTranscript is captionHeard with a soft stretch before "Und", which
// opens the second caption, that is sound with the silence level at
// -50 dB and silence at -40 dB, so the level moves where "Und" starts.
func edgeTranscript(silenceDB float64) *Transcript {
	start := 59.0
	var frames []float32
	for i := range 800 {
		at := start + float64(i)*FrameSeconds
		level := float32(-70)
		for _, w := range captionHeard() {
			if at >= w.Start && at < w.End {
				level = -20
			}
		}
		if at >= 63.9 && at < 64.3 {
			level = -45
		}
		frames = append(frames, level)
	}
	return fromStored(captionHeard(), frames, start, -30, &silenceDB)
}

// A caption edge moved by hand stays where it was put when the silence
// level moves the word it is kept on. It was kept by where that word
// starts on the sound, so a moved level left it meeting no word, and the
// caption went back to where its words put it. Plan row 2.266.
func TestAMovedCaptionEdgeHoldsWhenTheSilenceLevelMoves(t *testing.T) {
	loud, quiet := edgeTranscript(-50), edgeTranscript(-40)
	und := func(tr *Transcript) float64 { return tr.HeardWords[slicesIndex(tr.HeardWords, "Und")].Start }
	if und(loud) == und(quiet) {
		t.Fatalf("the test needs the level to move a word, Und starts at %.3f with both", und(loud))
	}
	fsys, path := captionPlanPath(t)
	opening := func(tr *Transcript) CaptionView {
		t.Helper()
		view, err := ClipCaptionsView(fsys, path, "01", tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range view.Captions {
			if c.First == und(tr) {
				return c
			}
		}
		t.Fatalf("no caption opens on Und at %.3f", und(tr))
		return CaptionView{}
	}
	before := opening(loud)
	if _, err := SetCaptionTime(fsys, path, "01", before.First, "start", und(loud)-0.5, false, loud, nil); err != nil {
		t.Fatal(err)
	}
	moved := opening(loud)
	if !moved.StartMoved || moved.Start >= before.Start {
		t.Fatalf("the caption starts at %.3f, moved %v, after its start was put 0.5 s earlier than %.3f",
			moved.Start, moved.StartMoved, before.Start)
	}
	if got := opening(quiet); !got.StartMoved || math.Abs(got.Start-moved.Start) > 0.001 {
		t.Errorf("with the level moved the caption starts at %.3f, moved %v, want %.3f where it was put",
			got.Start, got.StartMoved, moved.Start)
	}
}

// A caption moved by hand in a plan from before 2.266 is kept by where its
// word starts on the sound. It still reads that way, and the next move of
// either of its edges keeps both by the word's own key.
func TestAMovedCaptionFromBeforeIsReadAndKeptAnew(t *testing.T) {
	loud, quiet := edgeTranscript(-50), edgeTranscript(-40)
	und := quiet.HeardWords[slicesIndex(quiet.HeardWords, "Und")].Start
	fsys, path := writePlanAt(t, `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "clips": [
    {"id": "01", "slug": "schulhof",
     "segments": [{"start": 59.9, "end": 66.0}],
     "caption_times": {"`+wordKey(und)+`": {"start": `+fixed(und-0.5, 3)+`}}}
  ]
}`)
	second := func(tr *Transcript) CaptionView {
		t.Helper()
		view, err := ClipCaptionsView(fsys, path, "01", tr, nil)
		if err != nil {
			t.Fatal(err)
		}
		return view.Captions[len(view.Captions)-1]
	}
	from := second(quiet)
	if !from.StartMoved {
		t.Fatalf("a caption moved in a plan from before starts at %.3f, not moved", from.Start)
	}
	if _, err := SetCaptionTime(fsys, path, "01", from.Last, "end", from.End+0.3+59.9, false, quiet, nil); err != nil {
		t.Fatal(err)
	}
	body, _ := afero.ReadFile(fsys, path)
	if strings.Contains(string(body), `"`+wordKey(und)+`"`) {
		t.Errorf("the key from before is still kept:\n%s", body)
	}
	if got := second(loud); !got.StartMoved || math.Abs(got.Start-from.Start) > 0.001 {
		t.Errorf("after an edit and a change of the level the caption starts at %.3f, moved %v, want %.3f",
			got.Start, got.StartMoved, from.Start)
	}
}
