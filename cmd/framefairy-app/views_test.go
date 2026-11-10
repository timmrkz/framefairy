package main

import (
	"context"
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
			if _, err := svc.Shape(video, plan, "01", g); err != nil {
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

// The line from a circle into the next follows the hand: every step of a
// drag on the clip timeline is answered with where the crop glides then,
// and it is where the crop glides once the same gesture is let go and
// saved. It came only with the clip saved, so the line stood where it was
// while an edge or a cut was dragged across it, found by Tim on #750.
func TestTheGlideLineFollowsAnEdgeTheHandDrags(t *testing.T) {
	t.Parallel()
	svc, mine := memoryLibrary(t, "ep.mp4")
	video := mine[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: 60, Width: 480, Height: 270, FPSNum: 25, FPSDen: 1}, nil
	}
	plan := filepath.Join(engine.LogsDir(video), "clips.json")
	if err := afero.WriteFile(svc.store.files, plan, []byte(`{"source": "ep.mp4", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 14.0}], "words": [[10.1, 10.5, "eins"]],
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
	last := before.Glides
	for _, g := range []engine.Gesture{
		// A part cut out of the glide, and then the end dragged in over
		// the second circle, which ends the glide.
		{Kind: "cut", From: 11.6, To: 12.2},
		{Kind: "trim", Edge: "end", From: 12.6},
	} {
		shaped, err := svc.Shape(video, plan, "01", g)
		if err != nil {
			t.Fatalf("%s: %v", g.Kind, err)
		}
		if reflect.DeepEqual(shaped.Glides, last) {
			t.Errorf("%s: the glide line stays at %v under the hand", g.Kind, shaped.Glides)
		}
		saved, err := svc.Reshape(ctx, video, plan, "01", g, [2]float64{10, 10})
		if err != nil {
			t.Fatalf("%s: %v", g.Kind, err)
		}
		if !reflect.DeepEqual(shaped.Glides, saved.Glides) {
			t.Errorf("%s: the glide line under the hand is %v, and %v once saved", g.Kind, shaped.Glides, saved.Glides)
		}
		last = saved.Glides
	}
}
