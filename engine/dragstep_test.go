package engine

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// A step of a drag reads only the clip under the hand from the plan, and
// finds the clip findClip finds in every plan, the odd ones too: ids that
// are numbers, names given twice, a list item that is no clip, two lists
// of clips, #801.
func TestADragStepReadsTheClipEditsRead(t *testing.T) {
	plans := []string{
		`{"clips": [{"id": 1, "x": [1,2]}, {"slug": "b"}, 5, {"id": "03", "id": "07"}, {"id": "x y"}], "clips2": 1}`,
		`{"clips": [{"id": "01"}], "clips": [{"id": "02", "seg": {"a": null}}, {"id": true}]}`,
		`{"source": "s", "clips": [{"id": "Ä"}, {"id": 2.50}, {"id": "02"}]}`,
		`{"clips": "none"}`,
	}
	for _, p := range plans {
		value, err := decodeOrdered([]byte(p))
		if err != nil {
			t.Fatal(err)
		}
		var clips []*object
		if list, ok := value.(*object).values[keyClips].([]any); ok {
			for _, item := range list {
				if c, ok := item.(*object); ok {
					clips = append(clips, c)
				}
			}
		}
		for _, id := range []string{"01", "02", "03", "04", "05", "07", "x-y", "x_y", "true", "2.5", "2.50", "a", "nope"} {
			want, werr := findClip(clips, id)
			got, gerr := orderedClip([]byte(p), id)
			if (werr == nil) != (gerr == nil) {
				t.Fatalf("%s, clip %q: findClip says %v and orderedClip %v", p, id, werr, gerr)
			}
			if werr != nil {
				continue
			}
			a, _ := MarshalPlan(want)
			b, _ := MarshalPlan(got)
			if !bytes.Equal(a, b) {
				t.Errorf("%s, clip %q: findClip found %s and orderedClip %s", p, id, a, b)
			}
		}
	}
}

// longPlan is a video of 2.3 h with 25000 words and a plan of 80 clips over
// it, each in three pieces with keyframes, clip 41 with a corrected word
// when corrected.
func longPlan(b *testing.B, corrected bool) (PlanSummary, *Transcript) {
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
				if _, err := ShapeClipView(s, "41", c.g, tr, 0.1, look); err != nil {
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
		word := look.drawing(s.plan, clip, tr).Laid[2].Opens
		for b.Loop() {
			if _, err := ShapeCaptionTime(s, "41", word, "start", word-0.2, true, tr, look); err != nil {
				b.Fatal(err)
			}
		}
	})
}
