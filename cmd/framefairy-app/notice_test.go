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
// failed, so it is told nothing again, and nor is one that ends any other
// way.
func TestAFailureSeenOnlyAsTheAppOpensIsNotToldAgain(t *testing.T) {
	for _, state := range []string{JobFailed, JobDone, JobStopped, JobCancelled} {
		job := Job{ID: "job-1", Episode: "/home/tim/ep.mp4", State: JobFailed, JobRecord: engine.JobRecord{Kind: engine.JobSearch}}
		q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
		var told []Notice
		q.notice = func(n Notice) { told = append(told, n) }
		was := job
		q.update(&was, func(j *Job) { j.State = state })
		if len(told) != 0 {
			t.Errorf("a job failed already and read again as %s told %+v", state, told)
		}
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
