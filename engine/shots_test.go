package engine

import (
	"fmt"
	"strings"
	"testing"

	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// A clip has one crop, #396. A search used to part a clip at every camera
// switch and give each camera a crop of its own, and a plan made then still
// holds those pieces. Such a clip plays with the crop of its first piece,
// and takes it for good at its next change.

// shotsPlan is one clip of two shots that meet at a camera switch at
// 12.80, the way ClipSegments wrote them before #396, each with a crop of
// its own.
const shotsPlan = `{"clips": [{"id": "01", "segments": [
  {"start": 9.6, "end": 12.8, "crop_x": 300},
  {"start": 12.8, "end": 16.0, "crop_x": 900}]}]}`

func shotsTranscript() *Transcript {
	return &Transcript{Words: []timed.Cue{
		{Start: 9.8, End: 10.3, Text: "eins"}, {Start: 11, End: 11.5, Text: "zwei"}, {Start: 12.2, End: 12.6, Text: "drei"},
		{Start: 13.0, End: 13.4, Text: "vier"}, {Start: 14, End: 14.5, Text: "fünf"}, {Start: 15.2, End: 15.7, Text: "sechs"},
	}}
}

// shotsAre fails unless a clip is in exactly these pieces, each with its
// crop, and the crop moved by hand where moved says so.
func shotsAre(t *testing.T, fsys afero.Fs, path, when string, want []Segment) {
	t.Helper()
	got := clipByID(t, fsys, path, "01").Segments
	show := func(s []Segment) string {
		out := ""
		for _, p := range s {
			crop := "centre"
			if p.CropX != nil {
				crop = fmt.Sprint(*p.CropX)
			}
			out += fmt.Sprintf(" [%.3f %.3f %s moved=%v]", p.Start, p.End, crop, p.Moved)
		}
		return out
	}
	if show(got) != show(want) {
		t.Errorf("%s the clip is%s, not%s", when, show(got), show(want))
	}
}

// A clip framed for two cameras plays with one crop, its first.
func TestAClipFramedForTwoCamerasPlaysWithOneCrop(t *testing.T) {
	fsys, path := writePlanAt(t, shotsPlan)
	shotsAre(t, fsys, path, "as loaded", []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(300)},
		{Start: 12.8, End: 16, CropX: intPtr(300)}})
}

// A cut made across the old switch and put back leaves one piece with one
// crop, and the pieces the clip was found with go from the plan.
func TestACutPutBackOverAnOldSwitchLeavesOnePiece(t *testing.T) {
	tr := shotsTranscript()
	for _, snap := range []Snap{ToFrames, ToWords} {
		fsys, path := writePlanAt(t, strings.Replace(shotsPlan, `"segments": [`,
			`"found_segments": [{"start": 9.6, "end": 12.8, "crop_x": 300}, {"start": 12.8, "end": 16.0, "crop_x": 900}], "segments": [`, 1))
		g := Gesture{Kind: "cut", From: 12.0, To: 13.5, ToWords: bool(snap), Frames: at25}
		if err := Reshape(fsys, nil, path, "01", g, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		cut := ClipCuts(clipByID(t, fsys, path, "01"))
		if len(cut) != 1 {
			t.Fatalf("with snap %v the clip has the cuts %+v", snap, cut)
		}
		join := Gesture{Kind: "join", From: (cut[0].From + cut[0].To) / 2}
		shaped, err := ShapeClip(fsys, path, "01", join, tr, 0.1)
		if err != nil {
			t.Fatal(err)
		}
		if err := Reshape(fsys, nil, path, "01", join, tr, 0.1); err != nil {
			t.Fatal(err)
		}
		one := []Segment{{Start: 9.6, End: 16, CropX: intPtr(300)}}
		shotsAre(t, fsys, path, "after the cut is put back", one)
		if len(shaped.Pieces) != 1 || shaped.Pieces[0].Start != 9.6 || shaped.Pieces[0].End != 16 {
			t.Errorf("with snap %v, shown %+v while putting the cut back, saved %+v", snap, shaped.Pieces, one)
		}
		data, err := afero.ReadFile(fsys, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "found_segments") {
			t.Errorf("with snap %v, the plan still keeps the pieces the clip was found with", snap)
		}
	}
}

// A crop placed by hand on an old clip is the crop of the whole clip, and
// Automatic crop brings back the one its first piece was found with.
func TestACropOnAnOldClipIsTheCropOfTheWholeClip(t *testing.T) {
	fsys, path := writePlanAt(t, shotsPlan)
	if err := SetCrop(fsys, path, "01", 15, 700, SourceInfo{}, 0); err != nil {
		t.Fatal(err)
	}
	shotsAre(t, fsys, path, "after a crop placed on the second camera", []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(700), Moved: true},
		{Start: 12.8, End: 16, CropX: intPtr(700), Moved: true}})
	if err := ResetCrop(fsys, path, "01", 15); err != nil {
		t.Fatal(err)
	}
	shotsAre(t, fsys, path, "after Automatic crop", []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(300)},
		{Start: 12.8, End: 16, CropX: intPtr(300)}})
	data, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "900") {
		t.Errorf("the plan keeps the second camera's crop: %s", data)
	}
}

// An old clip where a crop was placed by hand on one camera plays with that
// crop, not with the automatic crop of its first piece. A crop a person
// placed is a decision, and one the analysis chose a guess, found by the
// review of #398. Its next change writes it so.
func TestAnOldClipKeepsTheCropPlacedByHand(t *testing.T) {
	fsys, path := writePlanAt(t, strings.Replace(shotsPlan,
		`"crop_x": 900}`, `"crop_x": 700, "crop_x_auto": 900}`, 1))
	placed := []Segment{
		{Start: 9.6, End: 12.8, CropX: intPtr(700), Moved: true},
		{Start: 12.8, End: 16, CropX: intPtr(700), Moved: true}}
	shotsAre(t, fsys, path, "as loaded", placed)
	g := Gesture{Kind: "cut", From: 12.0, To: 13.5, ToWords: bool(ToWords), Frames: at25}
	if err := Reshape(fsys, nil, path, "01", g, shotsTranscript(), 0.1); err != nil {
		t.Fatal(err)
	}
	for _, p := range clipByID(t, fsys, path, "01").Segments {
		if p.CropX == nil || *p.CropX != 700 || !p.Moved {
			t.Errorf("after a cut, a piece from %.2f plays with %s, not the crop placed by hand",
				p.Start, fmtCrop(p.CropX))
		}
	}
	data, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"crop_x": 300`) {
		t.Errorf("the plan still keeps the first camera's automatic crop: %s", data)
	}
}
