package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
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

// The library, the cards of what was said and the captions' look, which
// a drag asks for on every step, read a plan once between them, and again
// only once it changed. A video removed from the library lets go of its
// plans, and one added again reads them afresh. Issue #608.
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
		for range 3 {
			_ = svc.captionLook(video, plan)
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
