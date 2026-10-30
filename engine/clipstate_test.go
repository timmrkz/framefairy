package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// What is done to a clip belongs to the clip, and goes with it. Tim
// removed a caption block of a clip, removed the clip, and made it again
// with I where it had been: the clip came back, and the caption block he
// had removed was still missing. A removed word was kept for the whole
// video, so every clip over it lost it, the one made again included.

// oneClipPlan is a plan of one clip, 01, from start to end, in a work
// folder of its own, and the folder of the episode's logs it is in.
func oneClipPlan(t *testing.T, fsys afero.Fs, start, end float64) (logs, path string) {
	t.Helper()
	logs = filepath.Join(memDir(t, fsys), "ep.framefairy", "logs")
	path = filepath.Join(logs, "clips.json")
	plan := fmt.Sprintf(`{"source": "ep.mp4", "clips": [{"id": "01", "slug": "a", "segments": [{"start": %g, "end": %g}]}]}`, start, end)
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	return logs, path
}

// shownWords is what clip 01 of a plan shows, its captions' words.
func shownWords(t *testing.T, fsys afero.Fs, path string, tr *Transcript) string {
	t.Helper()
	view, err := ClipCaptionsView(fsys, path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range view.Captions {
		out = append(out, cueWords(c))
	}
	return strings.Join(out, " ")
}

// captionsOf is every caption of a clip, its words and when it is up.
func captionsOf(v *CaptionsView) []string {
	var out []string
	for _, c := range v.Captions {
		out = append(out, fmt.Sprintf("%.3f-%.3f %s", c.Start, c.End, cueWords(c)))
	}
	return out
}

// readAgain is the transcript as the app reads it after an edit, from
// what was heard. Nothing beside it is read: what an edit does to words is
// kept in the clip.
func readAgain(_ afero.Fs, _ string, heard []timed.Cue) *Transcript {
	return fromStored(heard, nil, 0, 0, nil)
}

// A clip made again where a removed one was is a clip of its own, with
// every caption its words have. Removing the clip took nothing of what had
// been done to it away.
func TestAClipMadeAgainHasTheCaptionsItsRemovedOneLost(t *testing.T) {
	fsys, path := writePlanAt(t, captionPlan)
	logs := filepath.Dir(path)
	tr := readAgain(fsys, logs, captionHeard())
	before, err := ClipCaptionsView(fsys, path, "01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Captions) < 2 {
		t.Fatalf("the test needs two captions, the clip has %d", len(before.Captions))
	}
	if err := RemoveCaption(fsys, path, "01", before.Captions[0].First, tr, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(fsys, nil, path, "01", true); err != nil {
		t.Fatal(err)
	}
	// Made again by hand, over the same part of the video.
	hand := filepath.Join(logs, HandPlanName)
	body := strings.Replace(strings.Replace(captionPlan, `"01"`, `"h01"`, 1), `"p1"`, `"hand"`, 1)
	if err := afero.WriteFile(fsys, hand, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := ClipCaptionsView(fsys, hand, "h01", readAgain(fsys, logs, captionHeard()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(captionsOf(again), "\n"), strings.Join(captionsOf(before), "\n"); got != want {
		t.Errorf("the clip made again has\n%s\nthe clip it was made in place of had\n%s", got, want)
	}
}

// A caption removed in one clip is removed from that clip alone. Another
// clip over the same words keeps it, the words said keep it, so a search
// and the model read what was said, and the clip that removed it keeps it
// removed, put back after it was removed too.
func TestACaptionRemovedInOneClipIsRemovedThereAlone(t *testing.T) {
	plan := `{
  "source": "ep.mp4",
  "plan_id": "p1",
  "clips": [
    {"id": "01", "slug": "schulhof", "segments": [{"start": 59.9, "end": 66.0}]},
    {"id": "02", "slug": "schulhof-lang", "segments": [{"start": 59.9, "end": 66.0}]}
  ]
}`
	fsys, path := writePlanAt(t, plan)
	logs := filepath.Dir(path)
	tr := readAgain(fsys, logs, captionHeard())
	before, err := ClipCaptionsView(fsys, path, "02", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveCaption(fsys, path, "01", before.Captions[0].First, tr, nil); err != nil {
		t.Fatal(err)
	}
	fresh := readAgain(fsys, logs, captionHeard())
	other, err := ClipCaptionsView(fsys, path, "02", fresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(captionsOf(other), "\n"), strings.Join(captionsOf(before), "\n"); got != want {
		t.Errorf("the other clip over the same words has\n%s\nit had\n%s", got, want)
	}
	if len(fresh.Words) != len(captionHeard()) {
		t.Errorf("the video says %d words, it said %d", len(fresh.Words), len(captionHeard()))
	}
	for _, removed := range []bool{true, false} {
		if err := SetRejected(fsys, nil, path, "01", removed); err != nil {
			t.Fatal(err)
		}
		own, err := ClipCaptionsView(fsys, path, "01", readAgain(fsys, logs, captionHeard()), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(own.Captions) != len(before.Captions)-1 {
			t.Errorf("removed %v, the clip has %d captions, not %d", removed, len(own.Captions), len(before.Captions)-1)
		}
	}
}

// Tim's whole story, with the clip made by hand both times: a caption
// removed, the clip removed, and I pressed where it was. The clip made
// again is the clip that was made the first time, its pieces and every
// caption.
//
// On the real disk: ffmpeg reads the episode the clips are heard and framed from.
func TestAClipMadeByHandAgainIsTheClipMadeTheFirstTime(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	log, _ := underwayLog()
	p := handProject(t, source, log)
	ctx := context.Background()
	if _, err := p.MakeClip(ctx, "clip-1", ClipRequest{At: 20}, nil); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	tr, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	first, err := ClipCaptionsView(disk, p.HandPlanPath(), "h01", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Captions) < 2 {
		t.Fatalf("the test needs two captions, the clip has %d", len(first.Captions))
	}
	if err := RemoveCaption(disk, p.HandPlanPath(), "h01", first.Captions[1].First, tr, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(disk, nil, p.HandPlanPath(), "h01", true); err != nil {
		t.Fatal(err)
	}
	key, err := p.MakeClip(ctx, "clip-2", ClipRequest{At: 20}, nil)
	if err != nil || key != HandPlanName+"/h02" {
		t.Fatalf("%q %v %s", key, err, p.LastError())
	}
	tr, err = p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	again, err := ClipCaptionsView(disk, p.HandPlanPath(), "h02", tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	pieces := func(v *CaptionsView) string { return fmt.Sprint(v.Pieces) }
	if pieces(again) != pieces(first) {
		t.Errorf("made again it keeps %s, the first time %s", pieces(again), pieces(first))
	}
	if got, want := strings.Join(captionsOf(again), "\n"), strings.Join(captionsOf(first), "\n"); got != want {
		t.Errorf("made again it has\n%s\nthe first time\n%s", got, want)
	}
}

// A word corrected in one clip is corrected there alone, and goes with the
// clip: a clip made again where it was reads what was heard. Tim chose
// this over a correction kept for the whole video, so a clip made again
// starts from nothing done to it.
func TestACorrectionGoesWithItsClip(t *testing.T) {
	plan := `{
  "source": "ep.mp4",
  "clips": [
    {"id": "01", "slug": "schulhof", "segments": [{"start": 59.9, "end": 66.0}]},
    {"id": "02", "slug": "schulhof-lang", "segments": [{"start": 59.9, "end": 66.0}]}
  ]
}`
	fsys, path := writePlanAt(t, plan)
	tr := readAgain(fsys, filepath.Dir(path), captionHeard())
	if err := SetWordText(fsys, path, "01", 61.2, "zwei", tr); err != nil {
		t.Fatal(err)
	}
	if got := shownWordsOf(t, fsys, path, "01", tr); !strings.Contains(got, "stand zwei Typ") {
		t.Errorf("the clip corrected shows %q", got)
	}
	if got := shownWordsOf(t, fsys, path, "02", tr); !strings.Contains(got, "stand ein Typ") {
		t.Errorf("the other clip over the same words shows %q", got)
	}
	if tr.Words[2].Text != "ein" {
		t.Errorf("the video says %q", tr.Words[2].Text)
	}
	if err := SetRejected(fsys, nil, path, "01", true); err != nil {
		t.Fatal(err)
	}
	hand := filepath.Join(filepath.Dir(path), HandPlanName)
	body := `{"source": "ep.mp4", "clips": [{"id": "h01", "slug": "schulhof", "segments": [{"start": 59.9, "end": 66.0}]}]}`
	if err := afero.WriteFile(fsys, hand, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := shownWordsOf(t, fsys, hand, "h01", tr); !strings.Contains(got, "stand ein Typ") {
		t.Errorf("the clip made again shows %q", got)
	}
}

// shownWordsOf is what one clip of a plan shows, its captions' words.
func shownWordsOf(t *testing.T, fsys afero.Fs, path, id string, tr *Transcript) string {
	t.Helper()
	view, err := ClipCaptionsView(fsys, path, id, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range view.Captions {
		out = append(out, cueWords(c))
	}
	return strings.Join(out, " ")
}

// longHeard is n words heard one after the other, a third of a second
// each, about four hours for 40,000.
func longHeard(n int) []timed.Cue {
	heard := make([]timed.Cue, n)
	for i := range heard {
		at := float64(i) / 3
		heard[i] = timed.Cue{Start: at, End: at + 0.3, Text: fmt.Sprintf("wort%d", i)}
	}
	return heard
}

// The words of a clip are its own corrections over what was heard,
// whichever way they are made: only the corrected words made again, or
// every word.
func TestAClipsWordsAreTheSameMadeInPlaceOrWhole(t *testing.T) {
	t.Parallel()
	heard := longHeard(400)
	heard[7].Text = "zwei worte"
	heard[9].Text = " "
	tr := fromStored(heard, nil, 0, 0, nil)
	for seed := range int64(40) {
		rng := rand.New(rand.NewPCG(uint64(seed), 1))
		corrections := map[string]string{}
		for range rng.IntN(12) {
			w := tr.HeardWords[rng.IntN(len(tr.HeardWords))]
			corrections[wordKey(w.Start)] = []string{"ja", "na ja", "so ist das", "Typ"}[rng.IntN(4)]
		}
		if rng.IntN(4) == 0 {
			corrections[wordKey(tr.HeardWords[9].Start)] = "da"
		}
		fast := tr.Of(Clip{Corrections: corrections})
		whole := *tr
		whole.Correct(corrections)
		if !slices.Equal(fast.HeardWords, whole.HeardWords) || !slices.Equal(fast.Words, whole.Words) {
			t.Fatalf("seed %d: made in place, the words differ from the words made whole", seed)
		}
		if fast.Language != tr.Language {
			t.Errorf("seed %d: the clip's words are in %q, the video's in %q", seed, fast.Language, tr.Language)
		}
		if len(tr.fixes) != 0 || tr.HeardWords[7].Text != "zwei worte" {
			t.Fatalf("seed %d: the video's words changed", seed)
		}
	}
}

// A clip with a correction copies the words of the video and makes again
// only the one it corrects. It made every word again, on every pointer
// move on its edges.
//
// It runs alone, because testing.AllocsPerRun counts the allocations of
// the whole process.
func TestAClipWithACorrectionMakesOnlyThatWordAgain(t *testing.T) {
	tr := fromStored(longHeard(40000), nil, 0, 0, nil)
	clip := Clip{Corrections: map[string]string{wordKey(tr.HeardWords[20000].Start): "zwei Worte"}}
	if got := tr.Of(clip).Words[20001].Text; got != "Worte" {
		t.Fatalf("the corrected word reads %q", got)
	}
	if allocs := testing.AllocsPerRun(20, func() { tr.Of(clip) }); allocs > 32 {
		t.Errorf("a clip with one correction took %.0f allocations, want a few", allocs)
	}
}

func BenchmarkAClipWithACorrection(b *testing.B) {
	tr := fromStored(longHeard(40000), nil, 0, 0, nil)
	clip := Clip{Corrections: map[string]string{wordKey(tr.HeardWords[20000].Start): "zwei Worte"}}
	for b.Loop() {
		tr.Of(clip)
	}
}
