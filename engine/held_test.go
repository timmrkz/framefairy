package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// A video has one Episode, and what it holds is one read of the
// transcript for everyone who asks, the status and the app alike, until a
// file of it is written again. Two caches read it twice before, each with
// its own idea of when to read it again.
func TestAVideosTranscriptIsReadOnceForEveryoneUntilItIsWritten(t *testing.T) {
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}}
	fsys, source, path := storedTranscript(t, words, make([]float32, 300), 3, true)
	m := NewMachine(fsys)
	if m.Episode(source) != m.Episode("/tmp/.."+source) {
		t.Fatal("one video was given two episodes")
	}
	first, err := m.Episode(source).Transcript("/models/modell", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st := m.Episode(source).Status("/models/modell", nil); st.Covered != 3 {
		t.Fatalf("the status says heard to %v, want 3", st.Covered)
	}
	if again, _ := m.Episode(source).Transcript("/models/modell", nil); again != first {
		t.Error("the transcript was read again though no file of it changed")
	}
	floor := -40.0
	if other, _ := m.Episode(source).Transcript("/models/modell", &floor); other == first {
		t.Error("another floor was given the transcript placed on the first")
	}

	stamp, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: "modell",
		Mean: num.Float(-30), Parts: [][2]num.Float{{0, 13}}, End: num.Float(13), Words: storedWords(words)}
	if err := writeTranscript(fsys, path, file, make([]float32, 1300)); err != nil {
		t.Fatal(err)
	}
	written, err := m.Episode(source).Transcript("/models/modell", nil)
	if err != nil || written == first || !written.Done || written.Heard[0][1] != 13 {
		t.Errorf("once written again it read %v heard %v, done %v", err, written.Heard, written.Done)
	}
}

// transcriptOf is the episode's transcript as read gives it, held by
// stamp the way Episode.Transcript holds it.
func transcriptOf(ep *Episode, stamp string, read func() (*TranscriptRead, error)) (*TranscriptRead, error) {
	return held(ep, &ep.transcript, stamp, ep.usedTranscript, read)
}

// The library asks the status of every video each time it is listed.
// What a transcript has heard is held for every video, so a library of
// more videos than transcripts are held lets go of no transcript a
// workspace holds, and lists again without reading any. Listing twelve
// once took the transcript of the video on screen out of the ten held.
func TestListingTheLibraryLetsGoOfNoTranscript(t *testing.T) {
	fsys := afero.NewMemMapFs()
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}}
	var videos []string
	for i := range keptTranscripts + 2 {
		video := fmt.Sprintf("/v/folge%d.mp4", i)
		storedTranscriptOf(t, fsys, video, words, make([]float32, 300), 3, false)
		videos = append(videos, video)
	}
	m := NewMachine(fsys)
	open, err := m.Episode(videos[0]).Transcript("/models/modell", nil)
	if err != nil {
		t.Fatal(err)
	}
	list := func() []*keptRead[heardSoFar] {
		var reads []*keptRead[heardSoFar]
		for _, video := range videos {
			if st := m.Episode(video).Status("/models/modell", nil); !st.Transcribed || st.Covered != 3 || st.Said == nil {
				t.Fatalf("%s listed as heard to %v, done %v, said %v", video, st.Covered, st.Transcribed, st.Said)
			}
			ep := m.Episode(video)
			m.held.Lock()
			reads = append(reads, ep.heard)
			m.held.Unlock()
		}
		return reads
	}
	first := list()
	if again, _ := m.Episode(videos[0]).Transcript("/models/modell", nil); again != open {
		t.Error("listing the library let go of the transcript of the video on screen")
	}
	for i, r := range list() {
		if r != first[i] {
			t.Errorf("%s was read again for the second listing, though nothing changed", videos[i])
		}
	}
}

// A transcript that is not this video's, heard by another speech model or
// of the file before it changed, is no transcript yet. That answer is kept
// by its stamp like a transcript, so the file is not read whole again for
// every question while it stays as it is.
func TestNoTranscriptYetIsKeptByItsStamp(t *testing.T) {
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}}
	fsys, source, _ := storedTranscript(t, words, make([]float32, 300), 3, false)
	m := NewMachine(fsys)
	ep := m.Episode(source)
	if _, err := ep.Transcript("/models/anderes", nil); !errors.Is(err, ErrNoTranscript) {
		t.Fatalf("a transcript of another model read as %v, not as none yet", err)
	}
	m.held.Lock()
	first := ep.transcript
	m.held.Unlock()
	if _, err := ep.Transcript("/models/anderes", nil); !errors.Is(err, ErrNoTranscript) {
		t.Fatalf("asked again it read as %v", err)
	}
	m.held.Lock()
	again := ep.transcript
	m.held.Unlock()
	if first == nil || again != first {
		t.Error("no transcript yet was read again though nothing changed")
	}
	if r, err := ep.Transcript("/models/modell", nil); err != nil || r == nil {
		t.Errorf("the model the transcript is of read %v", err)
	}
}

// An episode opening asks for its transcript from several calls at once,
// and it is read once. Going back to one of the last few videos reads
// nothing, one more lets go of the one asked for longest ago, a file that
// changed is read again, and a failure is not kept.
//
// The asks at once run in a testing/synctest bubble, so the test knows
// when all of them wait: synctest.Wait returns once every goroutine in the
// bubble is blocked, the first read on held and the others on that read.
func TestTranscriptsAreReadOnceAndKeptForTheLastVideos(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &Machine{}
		// a and the others fill what is kept, and one more comes after.
		others := []string{}
		for i := 1; i < keptTranscripts; i++ {
			others = append(others, fmt.Sprintf("/v/k%d.mp4", i))
		}
		a, more := "/v/a.mp4", "/v/more.mp4"
		reads := map[string]*atomic.Int32{}
		for _, video := range append(others, a, more) {
			reads[video] = &atomic.Int32{}
		}
		// The first read holds until every ask has come.
		held := make(chan struct{})
		get := func(video, stamp string) (*TranscriptRead, error) {
			return transcriptOf(m.Episode(video), stamp, func() (*TranscriptRead, error) {
				if reads[video].Add(1) == 1 && video == a {
					<-held
				}
				return newTranscriptRead(&Transcript{}, false), nil
			})
		}

		var wg sync.WaitGroup
		got := make([]*TranscriptRead, 8)
		for i := range got {
			wg.Go(func() { got[i], _ = get(a, "1") })
		}
		synctest.Wait()
		close(held)
		wg.Wait()
		if n := reads[a].Load(); n != 1 {
			t.Errorf("eight asks at once read %d times, wanted once", n)
		}
		for _, r := range got {
			if r != got[0] {
				t.Fatal("the asks at once were given different reads")
			}
		}

		for _, video := range others {
			_, _ = get(video, "1")
		}
		// All are kept: asking any again reads nothing. a is asked last, so
		// the first of the others is the one asked for longest ago when one
		// more comes.
		for _, video := range append(others, a) {
			_, _ = get(video, "1")
		}
		_, _ = get(more, "1")
		for video, n := range reads {
			if n.Load() != 1 {
				t.Errorf("%s was read %d times, wanted once", video, n.Load())
			}
		}
		first := others[0]
		_, _ = get(first, "1")
		_, _ = get(a, "1")
		if reads[first].Load() != 2 || reads[a].Load() != 1 {
			t.Errorf("after one more, %s read %d times and a %d, wanted %s again and a kept", first,
				reads[first].Load(), reads[a].Load(), first)
		}
		_, _ = get(a, "2")
		if reads[a].Load() != 2 {
			t.Error("a changed file was not read again")
		}

		fails := 0
		broken := func() (*TranscriptRead, error) { fails++; return nil, errors.New("no transcript yet") }
		for range 2 {
			if _, err := transcriptOf(m.Episode("/v/f.mp4"), "1", broken); err == nil {
				t.Error("a failure came back as an answer")
			}
		}
		if fails != 2 {
			t.Errorf("a failure was read %d times in two asks, wanted it read each time", fails)
		}
	})
}

// Everything at once on many videos, under the race detector.
func TestTranscriptsHeldFromEverywhereAtOnce(t *testing.T) {
	m := &Machine{}
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Go(func() {
			video := fmt.Sprintf("/v/%d.mp4", i%13)
			stamp := fmt.Sprint(i % 3)
			r, err := transcriptOf(m.Episode(video), stamp, func() (*TranscriptRead, error) {
				return newTranscriptRead(&Transcript{}, i%2 == 0), nil
			})
			if err != nil || r == nil {
				t.Errorf("%s gave %v, %v", video, r, err)
				return
			}
			_ = r.Weighed()
			_ = r.Said()
		})
	}
	wg.Wait()
}

// A read that panics lets go of everyone waiting on it, and is not kept.
// It runs in a testing/synctest bubble, so the read panics only once the
// waiter waits on it: synctest.Wait returns when both are blocked.
func TestATranscriptReadThatPanicsLetsGoOfItsWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := &Machine{}
		ep := m.Episode("/v/a.mp4")
		started, held := make(chan struct{}), make(chan struct{})
		go func() {
			defer func() { _ = recover() }()
			_, _ = transcriptOf(ep, "1", func() (*TranscriptRead, error) {
				close(started)
				<-held
				panic("read")
			})
		}()
		<-started
		waited := make(chan error)
		go func() {
			_, err := transcriptOf(ep, "1", func() (*TranscriptRead, error) { return nil, nil })
			waited <- err
		}()
		synctest.Wait()
		close(held)
		if err := <-waited; err == nil {
			t.Fatal("a waiter on a read that panicked got no error")
		}
		want := newTranscriptRead(&Transcript{}, true)
		if r, err := transcriptOf(ep, "1", func() (*TranscriptRead, error) { return want, nil }); err != nil || r != want {
			t.Fatalf("after a panic the video gave %v, %v, not a new read", r, err)
		}
	})
}

// planRead is the read the episode keeps of the plan at path, nil for none.
func planRead(ep *Episode, path string) *keptRead[PlanSummary] {
	ep.m.held.Lock()
	defer ep.m.held.Unlock()
	if slot := ep.plans[path]; slot != nil {
		return slot.read
	}
	return nil
}

// A video's plans are read once for the clip list, the status and the
// window alike, each again only when its file changed, whoever wrote it.
// A plan removed is let go of, and a plan that does not parse is left
// out and read again on the next question.
func TestAPlanIsReadOnceUntilItsFileChanges(t *testing.T) {
	fsys := afero.NewMemMapFs()
	m := NewMachine(fsys)
	source := "/v/folge.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	logs := LogsDir(source)
	first, second := filepath.Join(logs, "clips.json"), filepath.Join(logs, "clips-0-1800.json")
	write := func(path string, clips int) {
		t.Helper()
		if err := afero.WriteFile(fsys, path, []byte(aRealisticPlan(clips)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(first, 2)
	write(second, 3)
	ep := m.Episode(source)

	plans := ep.Plans()
	if len(plans) != 2 {
		t.Fatalf("listed %d plans, want 2", len(plans))
	}
	read := planRead(ep, first)
	if st := ep.Status("", nil); len(st.Plans) != 2 || !st.EverSearched {
		t.Errorf("the status lists %d plans, searched %v", len(st.Plans), st.EverSearched)
	}
	if view, err := ep.ReadPlan(first); err != nil || len(view.Clips) != 2 {
		t.Fatalf("the plan read for display gave %v", err)
	}
	if planRead(ep, first) != read {
		t.Error("the plan was read again though its file did not change")
	}

	write(first, 4)
	if view, err := ep.ReadPlan(first); err != nil || len(view.Clips) != 4 {
		t.Fatalf("the plan written again read %v, want its 4 clips", err)
	}
	if planRead(ep, first) == read {
		t.Error("the plan written again was not read again")
	}

	if err := fsys.Remove(second); err != nil {
		t.Fatal(err)
	}
	if plans := ep.Plans(); len(plans) != 1 || plans[0].Path != first {
		t.Errorf("after a plan was removed the episode lists %d", len(plans))
	}
	if planRead(ep, second) != nil {
		t.Error("the plan removed is still held")
	}

	if err := afero.WriteFile(fsys, second, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if plans := ep.Plans(); len(plans) != 1 {
		t.Errorf("a plan that does not parse was listed, %d plans", len(plans))
	}
	if planRead(ep, second) != nil {
		t.Error("a plan that does not parse was kept as an answer")
	}
}

// The plans asked for from everywhere at once while they are written,
// under the race detector.
func TestPlansHeldFromEverywhereAtOnce(t *testing.T) {
	fsys := afero.NewMemMapFs()
	m := NewMachine(fsys)
	source := "/v/folge.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(LogsDir(source), "clips.json")
	if err := afero.WriteFile(fsys, plan, []byte(aRealisticPlan(2)), 0o644); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			ep := m.Episode(source)
			switch i % 4 {
			case 0:
				_ = afero.WriteFile(fsys, plan, []byte(aRealisticPlan(1+i%5)), 0o644)
			case 1:
				for _, p := range ep.Plans() {
					_ = p.View()
				}
			case 2:
				_ = ep.Status("", nil)
			default:
				_, _ = ep.ReadPlan(plan)
			}
		})
	}
	wg.Wait()
}
