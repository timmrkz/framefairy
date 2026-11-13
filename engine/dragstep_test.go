package engine

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// The plan a drag reads once is shared by every step of the hand, so no
// step may change it: a cut taken out, a cut moved and an edge dragged
// each leave it as it was read, and the next step is shaped from the clip
// as it is in the file, #801.
func TestADragStepLeavesThePlanItReadAsItWas(t *testing.T) {
	s, tr := longPlan(t, false)
	frames := SourceInfo{Duration: 9000, Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1}
	clips, err := s.orderedClips()
	if err != nil {
		t.Fatal(err)
	}
	read, err := MarshalPlan(clips)
	if err != nil {
		t.Fatal(err)
	}
	look := &Look{Source: frames}
	steps := []Gesture{
		{Kind: "cut", From: 4103, To: 4105, Frames: frames},
		{Kind: "move", Edge: "from", Index: 0, From: 4107.5, Frames: frames},
		{Kind: "trim", Edge: "start", From: 4101.5, Frames: frames},
		{Kind: "join", Index: 1, From: 4108.5, Frames: frames},
	}
	for _, g := range steps {
		first, err := ShapeClipView(s, "41", g, tr, 0.1, look, nil)
		if err != nil {
			t.Fatalf("%s %s: %v", g.Kind, g.Edge, err)
		}
		again, err := ShapeClipView(s, "41", g, tr, 0.1, look, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first.Clip.Segments, again.Clip.Segments) {
			t.Errorf("%s %s: the same step made %v and then %v", g.Kind, g.Edge, first.Clip.Segments, again.Clip.Segments)
		}
		now, err := MarshalPlan(clips)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(read, now) {
			t.Fatalf("%s %s changed the plan every step reads", g.Kind, g.Edge)
		}
	}
}

// longPlan is a video of 2.3 h with 25000 words and a plan of 80 clips over
// it, each in three pieces with keyframes, clip 41 with a corrected word
// when corrected.
func longPlan(b testing.TB, corrected bool) (PlanSummary, *Transcript) {
	tr := fromStored(longHeard(25000), nil, 0, 0, nil)
	var clips []string
	for i := range 80 {
		at := 100 + float64(i)*100
		fix := ""
		if corrected && i == 40 {
			fix = fmt.Sprintf(`, "corrections": {%q: "zwei Worte"}`, tr.keyAt(int(at*3)+10))
		}
		clips = append(clips, fmt.Sprintf(`{"id": "%02d", "slug": "c%d", "title": "Ein Titel %d", "hook": "ein Haken",
		 "segments": [{"start": %.3f, "end": %.3f, "crop_x": 120}, {"start": %.3f, "end": %.3f, "crop_x": 300}, {"start": %.3f, "end": %.3f, "crop_x": 500}],
		 "crop_marks": [{"at": %.3f, "x": 100, "glide": true}, {"at": %.3f, "x": 1200, "glide": true}]%s}`,
			i+1, i, i, at, at+8, at+9, at+18, at+19, at+27, at+1, at+20, fix))
	}
	data := `{"source": "ep.mp4", "plan_id": "p1", "clips": [` + strings.Join(clips, ",\n") + `]}`
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	if err := afero.WriteFile(fsys, path, []byte(data), 0o644); err != nil {
		b.Fatal(err)
	}
	return planAt(b, fsys, path), tr
}

// One step of a drag of a clip's edge, and of a caption's edge, on a long
// video, #801.
func BenchmarkADragStep(b *testing.B) {
	frames := SourceInfo{Duration: 9000, Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1}
	for _, c := range []struct {
		name      string
		corrected bool
		g         Gesture
	}{
		{"start on frames", false, Gesture{Kind: "trim", Edge: "start", From: 4101.5, Frames: frames}},
		{"start to words", false, Gesture{Kind: "trim", Edge: "start", From: 4101.5, ToWords: true, Frames: frames}},
		{"end to words", false, Gesture{Kind: "trim", Edge: "end", From: 4125.5, ToWords: true, Frames: frames}},
		{"start to words, corrected", true, Gesture{Kind: "trim", Edge: "start", From: 4101.5, ToWords: true, Frames: frames}},
		{"end to words, corrected", true, Gesture{Kind: "trim", Edge: "end", From: 4125.5, ToWords: true, Frames: frames}},
	} {
		b.Run(c.name, func(b *testing.B) {
			s, tr := longPlan(b, c.corrected)
			look := &Look{Source: frames}
			for b.Loop() {
				if _, err := ShapeClipView(s, "41", c.g, tr, 0.1, look, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("caption edge, corrected", func(b *testing.B) {
		s, tr := longPlan(b, true)
		look := &Look{Source: frames}
		clip, err := s.clip("41")
		if err != nil {
			b.Fatal(err)
		}
		word := look.drawing(nil, s.plan, clip, tr).Laid[2].Opens
		for b.Loop() {
			if _, err := ShapeCaptionTime(s, "41", word, "start", word-0.2, true, tr, look); err != nil {
				b.Fatal(err)
			}
		}
	})
}
