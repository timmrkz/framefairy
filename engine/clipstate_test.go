package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// What is done to a clip belongs to the clip, and goes with it. Tim
// removed a caption block of a clip, removed the clip, and made it again
// with I where it had been: the clip came back, and the caption block he
// had removed was still missing. A removed word was kept for the whole
// video, so every clip over it lost it, the one made again included.

// captionsOf is every caption of a clip, its words and when it is up.
func captionsOf(v *CaptionsView) []string {
	var out []string
	for _, c := range v.Captions {
		out = append(out, fmt.Sprintf("%.3f-%.3f %s", c.Start, c.End, cueWords(c)))
	}
	return out
}

// readAgain is the transcript as the app reads it after an edit, from
// what was heard and what the episode keeps beside it.
func readAgain(fsys afero.Fs, logs string, heard []Cue) *Transcript {
	t := fromStored(heard, nil, 0, 0, nil)
	t.Correct(LoadCorrections(fsys, logs))
	return t
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
	if err := RemoveCaption(fsys, logs, path, "01", before.Captions[0].First, tr, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(fsys, path, "01", true); err != nil {
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
	if err := RemoveCaption(fsys, logs, path, "01", before.Captions[0].First, tr, nil); err != nil {
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
		if err := SetRejected(fsys, path, "01", removed); err != nil {
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
	if err := RemoveCaption(disk, p.LogsDir(), p.HandPlanPath(), "h01", first.Captions[1].First, tr, nil); err != nil {
		t.Fatal(err)
	}
	if err := SetRejected(disk, p.HandPlanPath(), "h01", true); err != nil {
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
