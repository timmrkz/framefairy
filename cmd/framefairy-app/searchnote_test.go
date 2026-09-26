package main

import (
	"context"
	"testing"
	"time"

	"framefairy/engine"
)

// A search is noted the moment it is asked for, so one the app is closed on
// while it waits for the transcript is not forgotten. One that fails while
// it waits says why.
func TestASearchIsNotedTheMomentItIsAskedFor(t *testing.T) {
	svc, mine, _ := library(t)
	job := svc.Plan(mine, engine.PlanRequest{To: 10, Count: 1, Min: 5})
	if note := engine.ReadSearchNote(mine); note == nil {
		t.Fatal("a search asked for has no note")
	}
	// The episode is no video, so the transcription it waits for fails.
	deadline := time.Now().Add(30 * time.Second)
	for {
		found, _ := svc.jobs.find(mine, "plan")
		if found.ID == job.ID && found.State != JobQueued && found.State != JobRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the search never ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
	note := engine.ReadSearchNote(mine)
	if note == nil || note.State != "failed" || note.Error == "" {
		t.Errorf("a search that failed while it waited: %+v", note)
	}
}

// Cancel pressed takes the note away, because whoever pressed it knows why
// the search stopped. The app closing stops it too and leaves the note.
func TestCancelTakesTheNoteAwayAndClosingDoesNot(t *testing.T) {
	svc, mine, _ := library(t)
	started := make(chan struct{})
	add := func() Job {
		if err := engine.NoteSearchAsked(mine, 0, 10); err != nil {
			t.Fatal(err)
		}
		return svc.jobs.add(mine, "plan", "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
			started <- struct{}{}
			<-ctx.Done()
			return "", engine.ErrCancelled
		})
	}
	job := add()
	<-started
	svc.CancelJob(job.ID)
	if note := engine.ReadSearchNote(mine); note != nil {
		t.Errorf("after Cancel: %+v", note)
	}
	for svc.busyWith("plan") {
		time.Sleep(time.Millisecond)
	}

	add()
	<-started
	svc.jobs.shutDown()
	if note := engine.ReadSearchNote(mine); note == nil || note.State != "running" {
		t.Errorf("after the app closed: %+v", note)
	}
}
