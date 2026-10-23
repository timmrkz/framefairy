package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
)

// A job that will not stop when it is told to, which is what a render in
// the middle of an ffmpeg run looks like: the process has to finish
// writing before the job can end.
func stubbornJob(release <-chan struct{}) func(context.Context, *engine.Project) (string, error) {
	return func(ctx context.Context, p *engine.Project) (string, error) {
		<-ctx.Done()
		<-release
		return "", engine.ErrCancelled
	}
}

func anEpisodeWithWork(t *testing.T) (*FrameFairy, string, string) {
	t.Helper()
	s, eps := memoryLibrary(t, "ep.mp4")
	source := eps[0]
	work := engine.WorkDir(source)
	if err := afero.WriteFile(s.store.files, filepath.Join(work, "logs", "clips.json"),
		[]byte(`{"source":"ep.mp4","clips":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.jobs = newQueue(s.store, func(JobUpdate) {}, func(string) {})
	return s, source, work
}

// Removing an episode and its work while something is still running on it
// must not delete the folder out from under that job.
//
// The job carries on writing after the folder is gone, so the folder comes
// back, and an episode the user removed leaves a folder behind with a
// partial transcript in it. Worse, the app keeps working on an episode
// that is no longer in the library.
//
// The removal gives the job stopWait, fifteen seconds, which pass on the
// fake clock of a synctest bubble in no time.
func TestAnEpisodeIsNotDeletedUnderARunningJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, source, work := anEpisodeWithWork(t)
		release := make(chan struct{})
		defer close(release)

		s.jobs.add(source, "render", "Render", stubbornJob(release))
		waitForRunning(t, s, source)

		err := s.RemoveEpisode(source, true)
		if err == nil {
			t.Error("the episode was removed while a job was still running on it")
		}
		if _, statErr := s.store.files.Stat(work); os.IsNotExist(statErr) {
			t.Error("the work folder was deleted while a job was still running on it")
		}
		if !s.store.Known(source) {
			t.Error("the episode was taken out of the library while a job was still running on it")
		}
	})
}

// The same call, once nothing is running, does what it says.
func TestAnEpisodeWithNothingRunningIsRemovedWithItsWork(t *testing.T) {
	s, source, work := anEpisodeWithWork(t)
	if err := s.RemoveEpisode(source, true); err != nil {
		t.Fatalf("removing a quiet episode: %v", err)
	}
	if _, err := s.store.files.Stat(work); !os.IsNotExist(err) {
		t.Error("the work folder is still there")
	}
	if s.store.Known(source) {
		t.Error("the episode is still in the library")
	}
	if _, err := s.store.files.Stat(source); err != nil {
		t.Error("the episode file itself was deleted, which never happens")
	}
}

// A job that does stop when it is told to is waited for, and then the work
// goes. Cancelling is the normal case and must not turn into a refusal.
func TestARunningJobThatStopsIsWaitedFor(t *testing.T) {
	s, source, work := anEpisodeWithWork(t)
	s.jobs.add(source, "render", "Render", func(ctx context.Context, p *engine.Project) (string, error) {
		<-ctx.Done()
		return "", engine.ErrCancelled
	})
	waitForRunning(t, s, source)

	if err := s.RemoveEpisode(source, true); err != nil {
		t.Fatalf("removing an episode whose job stops: %v", err)
	}
	if _, err := s.store.files.Stat(work); !os.IsNotExist(err) {
		t.Error("the work folder is still there")
	}
}

// Removing an episode without its work never waits and never refuses: the
// files stay, so a job still running on them harms nothing.
func TestAnEpisodeIsAlwaysRemovedWhenItsWorkIsKept(t *testing.T) {
	s, source, work := anEpisodeWithWork(t)
	release := make(chan struct{})
	defer close(release)
	s.jobs.add(source, "render", "Render", stubbornJob(release))
	waitForRunning(t, s, source)

	if err := s.RemoveEpisode(source, false); err != nil {
		t.Fatalf("removing an episode and keeping its work: %v", err)
	}
	if _, err := s.store.files.Stat(work); err != nil {
		t.Error("the work folder was deleted, and it was meant to be kept")
	}
	if s.store.Known(source) {
		t.Error("the episode is still in the library")
	}
}

// waitForRunning waits until a job of an episode runs, told by the queue
// as it starts.
func waitForRunning(t *testing.T, s *FrameFairy, source string) {
	t.Helper()
	s.jobs.until(func(jobs []*Job) bool {
		return slices.ContainsFunc(jobs, func(j *Job) bool { return j.Episode == source && j.State == JobRunning })
	}, nil)
}

// waitForSettled waits until no job of an episode is queued or running,
// told by the queue the moment the last one ends. A job that never ends
// hangs the test until go test's own -timeout, which shows every goroutine.
func waitForSettled(t *testing.T, s *FrameFairy, source string) {
	t.Helper()
	s.jobs.until(func(jobs []*Job) bool {
		return !slices.ContainsFunc(jobs, func(j *Job) bool { return j.Episode == source && going(j) })
	}, nil)
}

// hearing puts a search in the queue that hears the episode until it is
// told to stop, and then takes a moment to save what it heard, the way a
// real one does. What it gives says when that save is done. It runs in a
// synctest bubble, whose clock is fake, so that moment costs no time and a
// removal that did not wait for it always finds the save not done.
func hearing(t *testing.T, s *FrameFairy, path string) *atomic.Bool {
	t.Helper()
	saved := new(atomic.Bool)
	// Whatever carries on is stopped before the test's folder goes.
	t.Cleanup(func() { s.jobs.cancelEpisode(path) })
	started := make(chan struct{})
	s.jobs.addSteps(path, engine.JobSearch, "Find clips", true, nil,
		func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
			stepCtx, release, err := turn(ctx, engine.StepHearing)
			if err != nil {
				return "", engine.ErrCancelled
			}
			defer release()
			close(started)
			<-stepCtx.Done()
			// Long enough that a removal which did not wait would be seen.
			time.Sleep(20 * time.Millisecond) //nolint:forbidigo // synctest bubble
			saved.Store(true)
			return "", engine.ErrCancelled
		})
	<-started
	return saved
}

// Removing an episode while its search hears it. The search is stopped and
// waited for, and nothing of it is left running on an episode that is gone.
func TestRemovingAnEpisodeWhileItsSearchHears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, source, work := anEpisodeWithWork(t)
		saved := hearing(t, s, source)
		if err := s.RemoveEpisode(source, true); err != nil {
			t.Fatalf("removing the episode: %v", err)
		}
		if !saved.Load() {
			t.Error("the work folder was removed before the search had saved what it heard")
		}
		if _, err := s.store.files.Stat(work); !os.IsNotExist(err) {
			t.Error("the work folder is still there")
		}
		// The removal waited for the search to stop, so nothing of the
		// episode is left without anything more to wait for.
		for _, j := range s.jobs.list() {
			if j.Episode == source && going(&j) {
				t.Errorf("%s is %s on an episode that was removed", j.Kind, j.State)
			}
		}
	})
}

// While an episode is being removed, and after it is gone, nothing new is
// queued for it, from however many places it is asked for, and the
// removal is not held up by any of it. Anything asked for comes back as a
// job that failed with the reason, so the interface hears why.
func TestNothingStartsOnAnEpisodeBeingRemoved(t *testing.T) {
	s, source, _ := anEpisodeWithWork(t)
	stop := make(chan struct{})
	var asked atomic.Int64
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// The way every call of the app queues work: it checks
				// that the episode is in the library, then queues.
				if s.store.Known(source) {
					s.jobs.add(source, "render", "Render", func(ctx context.Context, p *engine.Project) (string, error) {
						<-ctx.Done()
						return "", engine.ErrCancelled
					})
				}
				s.Search(source, engine.PlanRequest{To: 10, Count: 1, Min: 5}, "")
				s.jobs.list()
				asked.Add(1)
			}
		}()
	}
	// The removal starts once every goroutine is asking, not after a
	// guess at how long they take to get going.
	for asked.Load() < 16 {
		runtime.Gosched()
	}
	err := s.RemoveEpisode(source, true)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("removing an episode that work kept being asked for: %v", err)
	}
	waitForSettled(t, s, source)
	refused := 0
	for _, j := range s.jobs.list() {
		if j.Episode != source {
			continue
		}
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("%s %s is %s after the episode was removed", j.ID, j.Kind, j.State)
		}
		if j.State == JobFailed && j.Error == "the video is being removed" {
			refused++
		}
	}
	t.Logf("%d asks refused while the episode was being removed", refused)
}

// Removing an episode and keeping its work stops its jobs as well, without
// waiting for them, so a removed episode does not go on holding the lane of
// hearing.
func TestARemovedEpisodeStopsHearing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, source, _ := anEpisodeWithWork(t)
		hearing(t, s, source)
		if err := s.RemoveEpisode(source, false); err != nil {
			t.Fatal(err)
		}
		// The search saves what it heard and ends, told by the queue. One
		// that never stops leaves everything in the bubble blocked, and
		// synctest fails the test at once.
		s.jobs.until(func(jobs []*Job) bool {
			return !slices.ContainsFunc(jobs, func(j *Job) bool {
				return j.Episode == source && j.Kind == engine.JobSearch && j.State == JobRunning
			})
		}, nil)
	})
}

// An episode added again after it was removed takes work again.
func TestAnEpisodeAddedAgainTakesWork(t *testing.T) {
	s, source, _ := anEpisodeWithWork(t)
	if err := s.RemoveEpisode(source, false); err != nil {
		t.Fatal(err)
	}
	if j := s.jobs.add(source, "render", "Render", nil); j.State != JobFailed {
		t.Errorf("work was queued for a removed episode: %s", j.State)
	}
	if _, err := s.store.AddEpisodes([]string{source}); err != nil {
		t.Fatal(err)
	}
	s.jobs.openEpisode(source)
	j := s.jobs.add(source, "render", "Render", func(ctx context.Context, p *engine.Project) (string, error) {
		return "", nil
	})
	if j.State == JobFailed {
		t.Errorf("an episode added again was refused: %s", j.Error)
	}
	// The job opens its log in the work folder as it starts. Left running,
	// it did so on a Mac in CI while the test's folder was being removed,
	// and the removal failed on a folder that was no longer empty.
	waitForSettled(t, s, source)
}
