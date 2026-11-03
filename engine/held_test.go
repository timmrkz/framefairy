package engine

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

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
	if st := Status(m, source, "/models/modell", nil); st.Covered != 3 {
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
			return m.transcriptHeld(m.Episode(video), stamp, func() (*TranscriptRead, error) {
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
			if _, err := m.transcriptHeld(m.Episode("/v/f.mp4"), "1", broken); err == nil {
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
			r, err := m.transcriptHeld(m.Episode(video), stamp, func() (*TranscriptRead, error) {
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
			_, _ = m.transcriptHeld(ep, "1", func() (*TranscriptRead, error) {
				close(started)
				<-held
				panic("read")
			})
		}()
		<-started
		waited := make(chan error)
		go func() {
			_, err := m.transcriptHeld(ep, "1", func() (*TranscriptRead, error) { return nil, nil })
			waited <- err
		}()
		synctest.Wait()
		close(held)
		if err := <-waited; err == nil {
			t.Fatal("a waiter on a read that panicked got no error")
		}
		want := newTranscriptRead(&Transcript{}, true)
		if r, err := m.transcriptHeld(ep, "1", func() (*TranscriptRead, error) { return want, nil }); err != nil || r != want {
			t.Fatalf("after a panic the video gave %v, %v, not a new read", r, err)
		}
	})
}
