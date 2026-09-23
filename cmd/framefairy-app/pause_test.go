package main

import (
	"context"
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
