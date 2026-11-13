package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// dragPlan is a plan the size of a long episode's, thirty clips with
// their framing, found edges and thumbnails, the first with two cuts.
func dragPlan() string {
	var clips []string
	for i := range 30 {
		at := 10 + float64(i)*60
		segments := fmt.Sprintf(`[{"start": %.2f, "end": %.2f, "crop_x": 420, "crop_x_auto": 420}]`, at, at+25)
		if i == 0 {
			segments = `[{"start": 10.0, "end": 12.0, "crop_x": 420, "crop_x_auto": 420},
			  {"start": 13.0, "end": 13.2, "crop_x": 420, "crop_x_auto": 420},
			  {"start": 13.52, "end": 40.0, "crop_x": 420, "crop_x_auto": 420}]`
		}
		clips = append(clips, fmt.Sprintf(`{"id": "%02d", "slug": "clip-%d", "title": "Clip number %d",
		  "reason": "a story told from start to end with a turn in the middle", "keep": true,
		  "found": [%.2f, %.2f], "thumbnails": [%.2f, %.2f],
		  "segments": %s}`, i+1, i, i, at, at+25, at+1, at+12, segments))
	}
	return `{"source": "ep.mp4", "plan_id": "p1", "revision": 7, "clips": [` + strings.Join(clips, ",\n") + `]}`
}

// benchDrag times one step of a drag of clip 01's start to from, the
// call the app makes for every step of the hand.
func benchDrag(b *testing.B, from float64) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	if err := afero.WriteFile(fsys, path, []byte(dragPlan()), 0o644); err != nil {
		b.Fatal(err)
	}
	s := planAt(b, fsys, path)
	tr := fromStored(longHeard(9000), nil, 0, 0, nil)
	g := Gesture{Kind: "trim", Edge: "start", From: from, To: from, Frames: at25}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ShapeClipView(s, "01", g, tr, 0.1, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkADragStepShortOfACut(b *testing.B) { benchDrag(b, 11) }

func BenchmarkADragStepIntoACut(b *testing.B) { benchDrag(b, 12.5) }

func BenchmarkADragStepPastTheCuts(b *testing.B) { benchDrag(b, 30) }
