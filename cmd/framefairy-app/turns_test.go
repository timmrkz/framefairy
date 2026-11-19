package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// The state carries the clip chosen as it will be once each of its parts
// is rotated a quarter further, and what R shows in the frame it is
// pressed in is what RotatePart then answers with, so the answer changes
// nothing on screen. The window turned it by a copy of the rule once,
// #610.
func TestTheStateSaysHowEachPartOfTheChosenClipTurns(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "ep.mp4")
	video := eps[0]
	withJobs(svc)
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1, Duration: 60}, nil
	}
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	body := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "eins",
	  "segments": [{"start": 10.0, "end": 11.1, "crop_x": 120},
	               {"start": 11.1, "end": 12.0, "crop_x": 120},
	               {"start": 13.0, "end": 14.0, "crop_x": 120}]}]}`
	if err := afero.WriteFile(svc.store.files, plan, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, err := svc.ChooseClip(ctx, video, "clips.json/01")
	if err != nil {
		t.Fatal(err)
	}
	turns := st.Turns
	if turns == nil || turns.Clip != "clips.json/01" || len(turns.Parts) != 2 {
		t.Fatalf("the turns of the clip chosen came as %+v, want one for each of its two parts", turns)
	}
	// The part after the cut, the second, as R would show it.
	shown := turns.Parts[1]
	answer, err := svc.RotatePart(ctx, video, plan, "01", 13.5)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Part != answer.Segments[2].Part {
		t.Errorf("the turn is of part %d, the piece turned is in part %d", shown.Part, answer.Segments[2].Part)
	}
	for _, c := range []struct {
		what        string
		shown, said any
	}{
		{"rotations", shown.Rotations, answer.Rotations},
		{"pieces", shown.Segments, answer.Segments},
		{"crops", shown.CropLefts, answer.CropLefts},
		{"crop steps", shown.CropSteps, answer.CropSteps},
		{"keyframes", shown.CropMarks, answer.CropMarks},
	} {
		if !reflect.DeepEqual(c.shown, c.said) {
			t.Errorf("the %s shown before the answer were %+v, the answer says %+v", c.what, c.shown, c.said)
		}
	}
	if answer.Segments[2].Rotate != 90 || answer.Segments[0].Rotate != 0 {
		t.Errorf("R turned the pieces %d, %d and %d", answer.Segments[0].Rotate, answer.Segments[1].Rotate, answer.Segments[2].Rotate)
	}
}

// A state sent leaves the turns out when the window has them as they are,
// the way it leaves out the clips that did not change, and sends them
// again once they changed. They are a framing for each part of the clip,
// sent four times a second while work runs.
func TestAStateSentLeavesOutTheTurnsTheWindowHas(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "ep.mp4")
	video := eps[0]
	withJobs(svc)
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1, Duration: 60}, nil
	}
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	body := `{"source": "ep.mp4", "clips": [{"id": "01", "slug": "eins",
	  "segments": [{"start": 10.0, "end": 11.1}, {"start": 13.0, "end": 14.0}]}]}`
	if err := afero.WriteFile(svc.store.files, plan, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	read := func() VideoState {
		t.Helper()
		st, err := svc.ChooseClip(ctx, video, "clips.json/01")
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	news := &telling{}
	if st := news.trim(read()); st.Turns == nil || len(st.Turns.Parts) != 2 {
		t.Fatalf("the first state sent has the turns %+v", st.Turns)
	}
	if st := news.trim(read()); st.Turns != nil {
		t.Errorf("a state sent again has the turns the window has, %+v", st.Turns)
	}
	if _, err := svc.RotatePart(ctx, video, plan, "01", 13.5); err != nil {
		t.Fatal(err)
	}
	if st := news.trim(read()); st.Turns == nil || st.Turns.Parts[1].Segments[1].Rotate != 180 {
		t.Errorf("once the part turned, the state sent has the turns %+v", st.Turns)
	}
}

// How long a state of a video takes to read, and how big it is sent the
// second time, with 12 clips and the first chosen, cut into parts parts:
// the turns of each part are worked out once for each read of the plan,
// and a state sent leaves them out while they are as the window has them.
func BenchmarkTheStateOfAVideoWithAChosenClip(b *testing.B) {
	for _, parts := range []int{2, 8} {
		b.Run(fmt.Sprintf("%d parts", parts), func(b *testing.B) {
			svc, eps := memoryLibrary(b, "ep.mp4")
			video := eps[0]
			withJobs(svc)
			svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
				return engine.SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1, Duration: 3600}, nil
			}
			var clips []string
			for i := 1; i <= 12; i++ {
				var pieces []string
				for p := range parts {
					at := float64(i*200 + p*5)
					pieces = append(pieces, fmt.Sprintf(`{"start": %v, "end": %v, "crop_x": 120}`, at, at+4))
				}
				clips = append(clips, fmt.Sprintf(`{"id": "%02d", "slug": "c%d", "segments": [%s]}`, i, i, strings.Join(pieces, ",")))
			}
			plan := filepath.Join(engine.LogsDir(video), "clips.json")
			body := `{"source": "ep.mp4", "clips": [` + strings.Join(clips, ",") + `]}`
			if err := afero.WriteFile(svc.store.files, plan, []byte(body), 0o644); err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()
			st, err := svc.ChooseClip(ctx, video, "clips.json/01")
			if err != nil {
				b.Fatal(err)
			}
			news := &telling{}
			news.trim(st)
			again, err := json.Marshal(news.trim(st))
			if err != nil {
				b.Fatal(err)
			}
			for b.Loop() {
				if _, err := svc.State(ctx, video); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(again)), "B/sent")
		})
	}
}
