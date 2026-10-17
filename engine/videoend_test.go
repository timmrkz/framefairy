package engine

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// A clip near the end of a video, 21.52 s long at 25 frames a second, and
// the last words said before it ends.
const nearTheEndPlan = `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "x", "segments": [` +
	`{"start": 10.0, "end": 14.0, "crop_x": 120}, {"start": 19.0, "end": 21.0, "crop_x": 120}]}]}`

var (
	nearTheEnd      = SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 21.52}
	nearTheEndWords = fromStored([]Cue{
		{10, 10.5, "eins"}, {12, 13.8, "zwei"}, {19.2, 20.8, "drei"}, {21.1, 21.45, "vier"},
	}, nil, 0, 0, nil)
)

// The end of a clip dragged past the end of the video stops where the
// video ends, on the clip timeline while the hand moves and in the plan
// once it lets go. It was saved where the hand was, and the short then
// came out shorter than its clip and failed its check. Found by Claude
// writing the walk for 2.166, plan row 2.170.
func TestAClipEndsWhereTheVideoDoes(t *testing.T) {
	cases := []struct {
		name string
		g    Gesture
	}{
		{"the end on frames", Gesture{Kind: "trim", Edge: "end", From: 24.3}},
		{"the end on words", Gesture{Kind: "trim", Edge: "end", From: 24.3, ToWords: true}},
		{"both edges on frames", Gesture{Kind: "trim", Edge: "both", From: 10, To: 30}},
		{"both edges on words", Gesture{Kind: "trim", Edge: "both", From: 10, To: 30, ToWords: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			path := filepath.Join(memDir(t, fsys), "clips.json")
			if err := afero.WriteFile(fsys, path, []byte(nearTheEndPlan), 0o644); err != nil {
				t.Fatal(err)
			}
			c.g.Frames = nearTheEnd
			shaped, err := ShapeClip(fsys, path, "01", c.g, nearTheEndWords, 0.1)
			if err != nil {
				t.Fatal(err)
			}
			if end := shaped.Pieces[len(shaped.Pieces)-1].End; end != 21.52 {
				t.Errorf("while the hand moves the clip ends at %v, not where the video does, 21.52", end)
			}
			if shaped.Playhead > 21.52 {
				t.Errorf("the playhead goes to %v, after the video", shaped.Playhead)
			}
			if err := Reshape(fsys, nil, path, "01", c.g, nearTheEndWords, 0.1); err != nil {
				t.Fatal(err)
			}
			_, clips, err := LoadClips(fsys, path)
			if err != nil {
				t.Fatal(err)
			}
			segs := clips[0].Segments
			if end := segs[len(segs)-1].End; end != 21.52 {
				t.Errorf("the clip was saved ending at %v, not where the video does, 21.52", end)
			}
		})
	}
}

// Every piece ends where the video does at the latest, whatever made it,
// and a single edge lands inside the video too. The end is the frame after
// the last one, so a video whose length falls inside a frame keeps that
// frame whole.
func TestAPieceEndsWhereTheVideoDoes(t *testing.T) {
	cases := []struct {
		name       string
		source     SourceInfo
		start, end float64
		want       [2]float64
	}{
		{"a piece over the end", nearTheEnd, 20, 30, [2]float64{20, 21.52}},
		{"a piece after the end", nearTheEnd, 25, 30, [2]float64{21.48, 21.52}},
		{"a piece inside", nearTheEnd, 20, 21, [2]float64{20, 21}},
		{"a length inside a frame", SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 21.53}, 20, 30, [2]float64{20, 21.56}},
		{"no frame rate", SourceInfo{Duration: 21.5}, 20, 30, [2]float64{20, 21.5}},
		{"no length", SourceInfo{FPSNum: 25, FPSDen: 1}, 20, 30, [2]float64{20, 30}},
	}
	for _, c := range cases {
		start, end := c.source.PieceOnFrames(c.start, c.end)
		if got := [2]float64{start, end}; got != c.want {
			t.Errorf("%s: %v to %v became %v, not %v", c.name, c.start, c.end, got, c.want)
		}
	}
	if at := nearTheEnd.FrameAt(40); at != 21.52 {
		t.Errorf("an edge put after the video lands at %v, not at its end, 21.52", at)
	}
	if at := nearTheEnd.FrameAt(-2); at != 0 {
		t.Errorf("an edge put before the video lands at %v, not at its start", at)
	}
	if end := nearTheEnd.End(); end != 21.52 {
		t.Errorf("the video ends at %v, not 21.52", end)
	}
	// The video preview's frames end where every piece does.
	grid := nearTheEnd.Grid()
	if got := grid.Start + float64(grid.Count)/25; got != nearTheEnd.End() {
		t.Errorf("the video preview's frames end at %v and the pieces at %v", got, nearTheEnd.End())
	}
}

// A short of a clip saved past the end of its video, before the end was
// held, is rendered to the end of the video and checked against that, so
// it is made rather than failing its check.
func TestAClipSavedPastTheEndIsRenderedToTheEnd(t *testing.T) {
	clip := Clip{ID: "01", Segments: []Segment{{Start: 10, End: 14}, {Start: 19, End: 24}}}
	on := nearTheEnd.OnFrames(clip)
	if d := on.Duration(); d != 6.52 {
		t.Errorf("the clip on the video's frames is %v s long, not 6.52", d)
	}
}
