package main

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// transcribing puts a stand-in transcription in the queue that runs until
// it is told to stop, and then takes a moment to save what it heard, the
// way a real one does.
func transcribing(t *testing.T, s *FrameFairy, path string) string {
	t.Helper()
	// Whatever carries on is stopped before the test's folder goes.
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	started := make(chan struct{})
	job := s.jobs.addOnce(path, "transcribe", "Transcription", func(ctx context.Context, p *engine.Project) (string, error) {
		close(started)
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond)
		return "", engine.ErrCancelled
	})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the transcription never started")
	}
	return job.ID
}

func transcriptions(s *FrameFairy, path string) int {
	n := 0
	for _, j := range s.jobs.list() {
		if j.Episode == path && j.Kind == "transcribe" {
			n++
		}
	}
	return n
}

// A search pauses the transcription and it carries on afterwards, even
// when the search is over before the transcription has finished saving.
func TestASearchPausesTheTranscriptionAndItCarriesOn(t *testing.T) {
	s, path, _ := library(t)
	first := transcribing(t, s, path)

	paused := s.pauseTranscriptions()
	if len(paused) != 1 || paused[0] != path {
		t.Fatalf("paused %v", paused)
	}
	s.carryOn(paused)

	if transcriptions(s, path) != 2 {
		t.Fatalf("%d transcriptions, want the paused one and the one carrying on", transcriptions(s, path))
	}
	if j, _ := s.jobs.find(path, "transcribe"); j.ID == first {
		t.Error("the transcription that carries on is the one that was paused")
	}
}

// A transcription paused by hand is not running when the search starts,
// so the search leaves it paused.
func TestATranscriptionPausedByHandStaysPaused(t *testing.T) {
	s, path, _ := library(t)
	first := transcribing(t, s, path)
	s.CancelJob(first)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if j, _ := s.jobs.find(path, "transcribe"); j.State != JobRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	paused := s.pauseTranscriptions()
	s.carryOn(paused)
	if len(paused) != 0 || transcriptions(s, path) != 1 {
		t.Errorf("paused %v, %d transcriptions", paused, transcriptions(s, path))
	}
}

// Pausing and carrying on from several goroutines at once, the way two
// searches and the interface could, never leaves the transcription stopped
// for good and never starts it twice.
func TestPausingAndCarryingOnAtOnce(t *testing.T) {
	s, path, _ := library(t)
	transcribing(t, s, path)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.carryOn(s.pauseTranscriptions())
			s.jobs.list()
		}()
	}
	wg.Wait()
	live := 0
	for _, j := range s.jobs.list() {
		if j.Kind == "transcribe" && (j.State == JobQueued || j.State == JobRunning) {
			live++
		}
	}
	if live > 1 {
		t.Errorf("%d transcriptions of one episode at once", live)
	}
	if transcriptions(s, path) < 2 {
		t.Error("the transcription never carried on")
	}
}

// A search that waits for the transcript starts when the audio has been
// heard to the end of the window, not when the transcript file next says
// so. It pauses the transcription there, the pause writes down what was
// heard, and the transcription is handed back to be carried on after the
// search. The file is saved seconds apart, and in those seconds the
// recogniser hears minutes of audio, which is how far the range picker's
// edge ran past the window while the search still waited.
func TestASearchStartsWhenTheWindowIsHeard(t *testing.T) {
	s, path, _ := library(t)
	var mu sync.Mutex
	saved, heard := 0.0, 0.0
	was := coverage
	coverage = func(string, string) (float64, bool) {
		mu.Lock()
		defer mu.Unlock()
		return saved, false
	}
	t.Cleanup(func() { coverage = was })
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })

	// A transcription that hears ten seconds of audio every 50 ms and never
	// saves while it runs. Stopped, it takes a moment and saves all it
	// heard, the way the engine does.
	s.jobs.addOnce(path, "transcribe", "Transcription", func(ctx context.Context, p *engine.Project) (string, error) {
		for {
			select {
			case <-ctx.Done():
				time.Sleep(100 * time.Millisecond)
				mu.Lock()
				saved = heard
				mu.Unlock()
				return "", engine.ErrCancelled
			case <-time.After(50 * time.Millisecond):
			}
			mu.Lock()
			heard += 10
			at := heard
			mu.Unlock()
			p.Log().ProgressTo("transcribing", at/3600, 1, at)
		}
	})

	search := engine.NewProject(engine.NewEngine(engine.NewLog(io.Discard, false, false)), path, s.store.Settings().options())
	began := time.Now()
	paused, err := s.waitForTranscript(context.Background(), search, engine.PlanRequest{To: 300})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotSaved, gotHeard := saved, heard
	mu.Unlock()
	if len(paused) != 1 || paused[0] != path {
		t.Errorf("paused %v, want the episode's transcription handed back", paused)
	}
	if gotSaved < 300 {
		t.Errorf("the search started on %.0f s written down, the window ends at 300", gotSaved)
	}
	// Heard at 10 s every 50 ms and looked at every 250 ms, it is paused
	// within about half a minute of audio past the end, where the file
	// alone would have let it run on until the next save.
	if gotHeard > 400 {
		t.Errorf("the transcription ran on to %.0f s before the search took over", gotHeard)
	}
	if time.Since(began) > 5*time.Second {
		t.Errorf("the wait took %s", time.Since(began))
	}
	s.carryOn(paused)
	if j, _ := s.jobs.find(path, "transcribe"); j.State != JobQueued && j.State != JobRunning {
		t.Errorf("the transcription did not carry on, it is %s", j.State)
	}
}

// A search stopped while it waits hands back what it paused, so Cancel
// never leaves the transcription stopped.
func TestACancelledWaitHandsBackWhatItPaused(t *testing.T) {
	s, path, _ := library(t)
	was := coverage
	coverage = func(string, string) (float64, bool) { return 0, false }
	t.Cleanup(func() { coverage = was })
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	s.jobs.addOnce(path, "transcribe", "Transcription", func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().ProgressTo("transcribing", 0.5, 1, 500)
		<-ctx.Done()
		// It saves nothing, so the file never reaches the window.
		time.Sleep(time.Second)
		return "", engine.ErrCancelled
	})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(600*time.Millisecond, cancel)
	search := engine.NewProject(engine.NewEngine(engine.NewLog(io.Discard, false, false)), path, s.store.Settings().options())
	paused, err := s.waitForTranscript(ctx, search, engine.PlanRequest{To: 300})
	if err == nil {
		t.Fatal("a cancelled wait said it was done")
	}
	if len(paused) != 1 {
		t.Errorf("a cancelled wait handed back %v", paused)
	}
}

// A transcription that takes long to stop is still carried on once it
// has, and the search's way out does not wait for it. After a fixed wait
// the old job was still running, the new ask was taken for it, and the
// transcription stayed paused for good.
func TestASlowPauseIsStillCarriedOn(t *testing.T) {
	s, path, _ := library(t)
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	started := make(chan struct{})
	s.jobs.addOnce(path, "transcribe", "Transcription", func(ctx context.Context, p *engine.Project) (string, error) {
		close(started)
		<-ctx.Done()
		// Saving what it heard takes longer than the wait on the way out.
		time.Sleep(1500 * time.Millisecond)
		return "", engine.ErrCancelled
	})
	<-started
	first, _ := s.jobs.find(path, "transcribe")

	paused := s.pauseTranscriptions()
	began := time.Now()
	s.carryOn(paused)
	if took := time.Since(began); took > 1500*time.Millisecond {
		t.Errorf("carrying on held the search's way out for %s", took)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := s.jobs.find(path, "transcribe"); ok && j.ID != first.ID &&
			(j.State == JobQueued || j.State == JobRunning) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the transcription was never carried on")
}
