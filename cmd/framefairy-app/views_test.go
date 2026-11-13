package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/fileio"
)

// The number keys ask the engine where a share of the chosen clip lands,
// and the answer is a moment of that clip, read from its own plan. Clip
// 02 of historyPlan runs from 20 to 22 s. Issue #539.
func TestAShareOfAClipIsAMomentOfThatClip(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	for _, c := range []struct{ share, at float64 }{{0, 20}, {0.5, 21}, {0.9, 21.8}} {
		got, err := svc.ShareOfClip(mine, plan, "02", c.share)
		if err != nil || math.Abs(got-c.at) > 1e-6 {
			t.Errorf("%.1f of clip 02 is %v, %v, want %.1f", c.share, got, err, c.at)
		}
	}
	if _, err := svc.ShareOfClip(mine, plan, "09", 0.5); err == nil {
		t.Error("a share of a clip the plan does not have was answered")
	}
}

// planOpens is a disk that counts how often a plan file is opened, to
// read it, the one way a plan is read.
type planOpens struct {
	afero.Fs
	n atomic.Int64
}

func (f *planOpens) Open(name string) (afero.File, error) {
	f.count(name)
	return f.Fs.Open(name)
}

func (f *planOpens) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f.count(name)
	return f.Fs.OpenFile(name, flag, perm)
}

func (f *planOpens) count(name string) {
	if ok, _ := filepath.Match("clips*.json", filepath.Base(name)); ok {
		f.n.Add(1)
	}
}

// The library, the cards of what was said, a drag on the clip timeline,
// step by step, the captions of a clip and a share of it read a plan once
// between them, and again only once it changed. A video removed from the
// library lets go of its plans, and one added again reads them afresh.
// Issue #608, whose Done when is that dragging an edge reads no plan
// file that has not changed.
func TestTheAppReadsAPlanOnceUntilItChanges(t *testing.T) {
	counted := &planOpens{Fs: afero.NewMemMapFs()}
	st := openStoreIn(counted, "/home/tim/config/FrameFairy")
	video := "/home/tim/videos/ep.mp4"
	if err := afero.WriteFile(counted, video, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markAdded(counted, video); err != nil {
		t.Fatal(err)
	}
	st.episodes = []string{video}
	svc := &FrameFairy{store: st}
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := fileio.WriteAtomic(counted, plan, []byte(historyPlan)); err != nil {
		t.Fatal(err)
	}
	ask := func() {
		t.Helper()
		_ = svc.Library()
		_ = svc.Episode(video)
		if _, err := svc.Sentences(video); err != nil {
			t.Fatal(err)
		}
		for i := range 3 {
			g := engine.Gesture{Kind: "trim", Edge: "start", From: 10.2 + 0.1*float64(i)}
			if _, err := svc.Shape(video, plan, "01", g, ""); err != nil {
				t.Fatalf("a step of the drag: %v", err)
			}
		}
		if _, err := svc.Captions(video, plan, "01"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ShareOfClip(video, plan, "02", 0.5); err != nil {
			t.Fatal(err)
		}
	}

	counted.n.Store(0)
	ask()
	ask()
	if n := counted.n.Load(); n != 1 {
		t.Errorf("the plan was opened %d times, want once", n)
	}

	if err := fileio.WriteAtomic(counted, plan, []byte(strings.Replace(historyPlan, "eins", "one", 1))); err != nil {
		t.Fatal(err)
	}
	counted.n.Store(0)
	ask()
	if n := counted.n.Load(); n != 1 {
		t.Errorf("the plan written again was opened %d times, want once", n)
	}

	if err := svc.RemoveEpisode(video, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.store.AddEpisodes([]string{video}); err != nil {
		t.Fatal(err)
	}
	counted.n.Store(0)
	ask()
	if n := counted.n.Load(); n != 1 {
		t.Errorf("the plan of a video added again was opened %d times, want once, read afresh", n)
	}
}

// The library, the cards, the caption look and the status asked from
// many goroutines at once while the plan is written again and the video
// is removed from the library and added back, under the race detector.
// Once all is still, the app shows the plan as its file says.
func TestThePlansAskedThroughTheAppFromEverywhereAtOnce(t *testing.T) {
	svc, video, plan := anEpisodeWithAPlan(t)
	var wg sync.WaitGroup
	for i := range 60 {
		wg.Go(func() {
			switch i % 6 {
			case 0:
				body := strings.Replace(historyPlan, "eins", fmt.Sprintf("eins %d", i), 1)
				_ = fileio.WriteAtomic(svc.store.files, plan, []byte(body))
			case 1:
				_ = svc.Library()
			case 2:
				_ = svc.Episode(video)
			case 3:
				_, _ = svc.Sentences(video)
			case 4:
				_ = svc.captionLook(video, plan)
			default:
				if i%12 == 5 {
					_ = svc.RemoveEpisode(video, false)
				} else {
					_, _ = svc.store.AddEpisodes([]string{video})
				}
			}
		})
	}
	wg.Wait()

	if _, err := svc.store.AddEpisodes([]string{video}); err != nil {
		t.Fatal(err)
	}
	if err := fileio.WriteAtomic(svc.store.files, plan, []byte(strings.Replace(historyPlan, "eins", "last", 1))); err != nil {
		t.Fatal(err)
	}
	v, err := svc.store.machine.Episode(video).ReadPlan(plan)
	if err != nil || len(v.Clips) != 2 || v.Clips[0].Slug != "last" {
		t.Errorf("once still, the plan read %v, want the one written last", err)
	}
}

// The keyframes, the line from a circle into the next and the crop all
// follow the hand: every step of a drag on the clip timeline is answered
// with how the crop runs then, and it is how the crop runs once the same
// gesture is let go and saved. The glide line came only with the clip
// saved, found by Tim on #750, and then the keyframes, which an edge
// carries, found by him on #778.
func TestTheKeyframesAndTheCropFollowAnEdgeTheHandDrags(t *testing.T) {
	t.Parallel()
	svc, mine := memoryLibrary(t, "ep.mp4")
	video := mine[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: 60, Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}, nil
	}
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0, "crop_x": 40}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 11.0, "x": 10, "glide": true}, {"at": 13.0, "x": 300, "glide": true}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := svc.clipEntry(ctx, video, plan, "01")
	if err != nil {
		t.Fatal(err)
	}
	if want := []engine.GlideSpan{{From: 11, To: 13}}; !reflect.DeepEqual(before.Glides, want) {
		t.Fatalf("the clip glides over %v before the drag, not %v", before.Glides, want)
	}
	framing := func(e ClipEntry) Framing {
		return Framing{e.Segments, e.CropMarks, e.CropLefts, e.CropSteps, e.Glides}
	}
	last := framing(before)
	for _, g := range []engine.Gesture{
		// A part cut out of the glide, the start dragged in over the first
		// circle, which it carries, and then the end over the second, which
		// it carries on the last frame, still joined to the first.
		{Kind: "cut", From: 11.6, To: 12.2},
		{Kind: "trim", Edge: "start", From: 11.2},
		{Kind: "trim", Edge: "end", From: 12.92},
	} {
		shaped, err := svc.Shape(video, plan, "01", g, "")
		if err != nil {
			t.Fatalf("%s %s: %v", g.Kind, g.Edge, err)
		}
		if reflect.DeepEqual(shaped.Framing, last) {
			t.Errorf("%s %s: the crop stays as it was under the hand, %+v", g.Kind, g.Edge, shaped.Framing)
		}
		saved, err := svc.Reshape(ctx, video, plan, "01", g, [2]float64{10, 10})
		if err != nil {
			t.Fatalf("%s %s: %v", g.Kind, g.Edge, err)
		}
		if !reflect.DeepEqual(shaped.Framing, framing(saved)) {
			t.Errorf("%s %s: under the hand the crop runs as\n%+v\nand once saved as\n%+v", g.Kind, g.Edge, shaped.Framing, framing(saved))
		}
		last = framing(saved)
	}
	if want := []engine.CropMarkView{{At: 11.2, X: 10, Glide: true, Opens: true, FromClip: true}, {At: 12.88, X: 300, Glide: true}}; !reflect.DeepEqual(last.CropMarks, want) {
		t.Errorf("the edges carry %v, not %v", last.CropMarks, want)
	}
}

// BenchmarkAShapeAnswer is one answer to a step of a drag of the start of
// a 30 s clip at 30 fps, worked out and written as JSON, with two diamonds
// in it and with two circles gliding over 20 s of it.
func BenchmarkAShapeAnswer(b *testing.B) {
	b.Run("two diamonds", func(b *testing.B) { benchShape(b, "false") })
	b.Run("a 20 s glide", func(b *testing.B) { benchShape(b, "true") })
}

func benchShape(b *testing.B, glide string) {
	svc, mine := memoryLibrary(b, "ep.mp4")
	video := mine[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: 600, Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1}, nil
	}
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 40.0}], "words": [[10.1, 10.5, "eins"]],
	   "crop_marks": [{"at": 15.0, "x": 100, "glide": `+glide+`}, {"at": 35.0, "x": 1200, "glide": `+glide+`}]}]}`), 0o644); err != nil {
		b.Fatal(err)
	}
	if _, err := svc.clipEntry(context.Background(), video, plan, "01"); err != nil {
		b.Fatal(err)
	}
	g := engine.Gesture{Kind: "trim", Edge: "start", From: 12}
	size := 0
	for b.Loop() {
		shaped, err := svc.Shape(video, plan, "01", g, "")
		if err != nil {
			b.Fatal(err)
		}
		out, _ := json.Marshal(shaped)
		size = len(out)
	}
	b.ReportMetric(float64(size), "bytes/answer")
}
