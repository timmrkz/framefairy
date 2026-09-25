package main

import (
	"context"
	"io"
	"path/filepath"
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
	var savedAt time.Time
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
				savedAt = time.Now()
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
	returned := time.Now()
	mu.Lock()
	gotSaved, gotHeard, landed := saved, heard, savedAt
	mu.Unlock()
	// The search starts the moment the paused transcription has written
	// down what it heard. The wait used to sleep a full second after the
	// pause before it looked at the transcript again.
	if lag := returned.Sub(landed); lag > 150*time.Millisecond {
		t.Errorf("the search started %s after the transcript was written down", lag)
	}
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
	stood, _ := s.jobs.find(path, "transcribe")
	s.carryOn(paused)
	// A new transcription, in whatever state it is by now: the real one
	// here has no speech model and can have failed already.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := s.jobs.find(path, "transcribe"); ok && j.ID != stood.ID {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the transcription did not carry on")
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
	// Carried on is a new transcription, in whatever state it is by the
	// time this looks: the one here has no speech model and fails at once,
	// and on a fast machine it has already failed.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if j, ok := s.jobs.find(path, "transcribe"); ok && j.ID != first.ID {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the transcription was never carried on")
}

// The hold of an episode's transcription: set from the workspace, read by
// the transcription on every chunk, let go of by the search or by Cancel.
func TestTheTranscriptionHold(t *testing.T) {
	s, path, home := library(t)
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	if err := s.HoldTranscription(filepath.Join(home, "not-mine.mp4"), 1800); err == nil {
		t.Error("a hold was set on an episode that is not in the library")
	}
	if err := s.HoldTranscription(path, 1800); err != nil {
		t.Fatal(err)
	}
	if got := s.holdOf(path); got != 1800 {
		t.Errorf("the hold reads %v", got)
	}
	// Cancel before the search: the hold goes, and the transcription, which
	// stopped there, stays stopped. It runs for a search and for nothing
	// else.
	before := transcriptions(s, path)
	if err := s.HoldTranscription(path, 0); err != nil {
		t.Fatal(err)
	}
	if s.holdOf(path) != 0 {
		t.Error("the hold is still there after it was let go of")
	}
	if transcriptions(s, path) != before {
		t.Error("letting go of the hold started a transcription")
	}
	// Removing the episode forgets its hold.
	_ = s.HoldTranscription(path, 900)
	s.forget(path)
	if s.holdOf(path) != 0 {
		t.Error("a removed episode kept its hold")
	}
}

// Holds are set and let go of from the window while transcriptions read
// them on their own goroutines.
func TestHoldsFromEverywhereAtOnce(t *testing.T) {
	s, path, _ := library(t)
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				switch (g + i) % 4 {
				case 0:
					_ = s.HoldTranscription(path, float64(600+i))
				case 1:
					s.holdOf(path)
				case 2:
					s.releaseHold(path)
				default:
					_ = s.HoldTranscription(path, 0)
				}
			}
		}()
	}
	wg.Wait()
}

// live says whether an episode has a transcription running or waiting.
func live(s *FrameFairy, path string) bool {
	j, ok := s.jobs.find(path, "transcribe")
	return ok && (j.State == JobRunning || j.State == JobQueued)
}

// until waits for a job to end.
func until(t *testing.T, s *FrameFairy, id string) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range s.jobs.list() {
			if j.ID == id && j.State != JobRunning && j.State != JobQueued {
				return j
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the job never ended")
	return Job{}
}

// The transcription of an episode runs for its searches. Once the search
// has the words for its window, the episode's own transcription stays
// stopped. It used to carry on through the whole episode after every
// search, hours of audio nobody had asked for.
func TestASearchLeavesItsOwnTranscriptionStopped(t *testing.T) {
	s, path, _ := library(t)
	was := coverage
	coverage = func(string, string) (float64, bool) { return 300, false }
	t.Cleanup(func() { coverage = was })
	transcribing(t, s, path)

	job := s.Plan(path, engine.PlanRequest{To: 300, Count: 1, Min: 10, Max: 30})
	until(t, s, job.ID)
	// Give a transcription that carries on the time to be queued.
	time.Sleep(300 * time.Millisecond)
	if live(s, path) || transcriptions(s, path) != 1 {
		t.Errorf("after the search %d transcriptions, one still running: %v", transcriptions(s, path), live(s, path))
	}
	if s.holdOf(path) != 0 {
		t.Error("the search left its hold behind")
	}
}

// A search whose window is not transcribed starts the transcription
// itself, even one that was stopped before, and holds it at the window.
// A stopped transcription used to fail the search with "the transcription
// is paused", and New stayed off until someone found the button at the edge
// of the range picker that carried it on.
func TestASearchStartsAStoppedTranscriptionHeldAtItsWindow(t *testing.T) {
	s, path, _ := library(t)
	was := coverage
	coverage = func(string, string) (float64, bool) { return 100, false }
	t.Cleanup(func() { coverage = was })
	s.CancelJob(transcribing(t, s, path))
	for live(s, path) {
		time.Sleep(10 * time.Millisecond)
	}

	job := s.Plan(path, engine.PlanRequest{To: 300, Count: 1, Min: 10, Max: 30})
	held := 0.0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && transcriptions(s, path) < 2 {
		if h := s.holdOf(path); h > 0 {
			held = h
		}
		time.Sleep(5 * time.Millisecond)
	}
	if transcriptions(s, path) != 2 {
		t.Fatalf("%d transcriptions, the search did not start one", transcriptions(s, path))
	}
	if h := s.holdOf(path); h > 0 {
		held = h
	}
	if held != 300 {
		t.Errorf("the transcription was held at %v, the window ends at 300", held)
	}
	s.CancelJob(job.ID)
	until(t, s, job.ID)
}

// Cancel on a search that waits for the words stops the transcription it
// waited for: it ran for that search alone.
func TestCancellingAWaitingSearchStopsTheTranscription(t *testing.T) {
	s, path, _ := library(t)
	was := coverage
	coverage = func(string, string) (float64, bool) { return 100, false }
	t.Cleanup(func() { coverage = was })
	transcribing(t, s, path)

	job := s.Plan(path, engine.PlanRequest{To: 300, Count: 1, Min: 10, Max: 30})
	time.Sleep(200 * time.Millisecond)
	s.CancelJob(job.ID)
	until(t, s, job.ID)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && live(s, path) {
		time.Sleep(20 * time.Millisecond)
	}
	if live(s, path) {
		t.Error("the transcription went on after its search was called off")
	}
	if s.holdOf(path) != 0 {
		t.Error("the search left its hold behind")
	}
}

// A new episode transcribes as far as the start of its first window and no
// further. One that has been searched before waits for a search to ask.
func TestANewEpisodeIsTranscribedForItsFirstSearch(t *testing.T) {
	s, path, _ := library(t)
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	s.transcribeForFirstSearch(path)
	if transcriptions(s, path) != 1 || s.holdOf(path) != firstLook {
		t.Errorf("%d transcriptions, held at %v", transcriptions(s, path), s.holdOf(path))
	}

	s2, other, _ := library(t)
	if err := engine.MarkLooked(other); err != nil {
		t.Fatal(err)
	}
	s2.transcribeForFirstSearch(other)
	if transcriptions(s2, other) != 0 || s2.holdOf(other) != 0 {
		t.Errorf("an episode searched before: %d transcriptions, held at %v", transcriptions(s2, other), s2.holdOf(other))
	}
}
