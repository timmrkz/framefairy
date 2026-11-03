package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"framefairy/engine"
)

// A failure is told once, at the moment the queue sees it, in the words
// the toast shows: a search that fails as it runs, with Report, and a
// render refused as it is asked for, without. A download is told in its
// card alone, so it sends no notice.
func TestAFailureIsToldOnceAsItHappens(t *testing.T) {
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
	notices := make(chan Notice, 8)
	q.notice = func(n Notice) { notices <- n }
	const video = "/home/tim/ep.mp4"

	job := q.add(video, engine.JobSearch, "Find clips", func(context.Context, *engine.Project) (string, error) {
		return "", errors.New("über the memory the model may have")
	})
	n := <-notices
	want := Notice{Video: video, Job: job.ID, Kind: engine.JobSearch, Text: "Über the memory the model may have", Report: true}
	if n != want {
		t.Errorf("a search that failed told %+v, not %+v", n, want)
	}
	q.waitJob(job.ID)

	refused := q.refuse(video, engine.JobRender, "Render", "the app is closing")
	if n := <-notices; n.Job != refused.ID || n.Text != "The app is closing" || n.Report {
		t.Errorf("a render refused told %+v", n)
	}
	q.refuse("", "model", "Speech model", "there is no speech model called x")
	if len(notices) != 0 {
		t.Errorf("more was told: %+v", <-notices)
	}
}

// A job that was failed already, as the app opened, said why the time it
// failed, so it is told nothing again as the app reads it back, and
// nothing when it is read again as failed or as ending any other way.
func TestAFailureSeenOnlyAsTheAppOpensIsNotToldAgain(t *testing.T) {
	s, eps := memoryLibrary(t, "talk.mp4")
	s.jobs = newQueue(s.store, func(JobUpdate) {}, func(string) {})
	var told []Notice
	s.jobs.notice = func(n Notice) { told = append(told, n) }
	for _, rec := range []engine.JobRecord{
		{ID: engine.SearchID, Kind: engine.JobSearch, Step: engine.StepFailed, Error: "the model went away"},
		{ID: "clip-9", Kind: engine.JobClip, At: 95.5, Step: engine.StepFailed, Error: "nobody speaks there"},
	} {
		if err := engine.WriteJob(s.store.files, eps[0], rec); err != nil {
			t.Fatal(err)
		}
	}
	s.restoreJobs(eps)
	jobs := s.jobs.list()
	if len(jobs) != 2 || len(told) != 0 {
		t.Fatalf("the app opened on %d jobs and told %+v", len(jobs), told)
	}
	for _, j := range jobs {
		if j.State != JobFailed {
			t.Errorf("the job %s came back %s, not failed", j.Kind, j.State)
		}
		for _, state := range []string{JobFailed, JobDone, JobStopped, JobCancelled} {
			was := j
			s.jobs.update(&was, func(j *Job) { j.State = state })
		}
	}
	if len(told) != 0 {
		t.Errorf("jobs failed already and read again told %+v", told)
	}
}

// Jobs fail from several goroutines at once, as many asked and refused,
// while the jobs are listed: each failure is told once, and only once.
func TestFailuresAtOnceAreEachToldOnce(t *testing.T) {
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
	var mu sync.Mutex
	told := map[string]int{}
	q.notice = func(n Notice) {
		mu.Lock()
		told[n.Job]++
		mu.Unlock()
	}
	fail := func(context.Context, *engine.Project) (string, error) { return "", errors.New("it broke") }
	const hands = 8
	var all sync.WaitGroup
	ids := make(chan string, 2*hands)
	for i := range hands {
		video := fmt.Sprintf("/home/tim/ep%d.mp4", i%3)
		all.Go(func() { ids <- q.add(video, engine.JobClip, "Make a clip", fail).ID })
		all.Go(func() {
			ids <- q.refuse(video, engine.JobTranscribe, "Transcribe whole video", "the video is being removed").ID
		})
		all.Go(func() { q.list() })
	}
	all.Wait()
	close(ids)
	var asked []string
	for id := range ids {
		q.waitJob(id)
		asked = append(asked, id)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, id := range asked {
		if told[id] != 1 {
			t.Errorf("job %s was told %d times", id, told[id])
		}
	}
	if len(told) != len(asked) {
		t.Errorf("%d jobs were told, %d failed", len(told), len(asked))
	}
}
