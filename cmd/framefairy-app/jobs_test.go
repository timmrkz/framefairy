package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine"
)

// The queue runs jobs on goroutines of its own while the interface asks it
// things from another, so here everything it can be asked is asked at once.
// make test runs this with the race detector, which is the point of it: a
// queue that only works when nobody is looking is not a queue.
func TestTheQueueTakesEverythingAtOnce(t *testing.T) {
	home := "/home/tim"
	st := memoryStore()

	var events, broken atomic.Int64
	q := newQueue(st, func(u JobUpdate) {
		events.Add(1)
		// Every snapshot the interface receives is a whole job.
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.Lane == "" || u.Job.State == "" {
			broken.Add(1)
		}
	}, func(string) {})

	// Work that says something and then waits to be told to stop, so jobs
	// are running while everything else happens to them.
	work := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().Info("%s", "working")
		p.Log().ProgressOf("working", 0.5, 1)
		<-ctx.Done()
		return "", engine.ErrCancelled
	}

	const hands, each = 6, 8
	var wg sync.WaitGroup
	ids := make(chan string, hands*each)

	for hand := 0; hand < hands; hand++ {
		wg.Add(1)
		go func(hand int) {
			defer wg.Done()
			kinds := []string{"transcribe", "plan", "render"}
			for k := 0; k < each; k++ {
				kind := kinds[(hand+k)%len(kinds)]
				job := q.add(filepath.Join(home, "ep.mp4"), kind, "Work", work)
				ids <- job.ID
			}
		}(hand)
	}
	// Everything else the interface does while jobs are being added and run.
	for hand := 0; hand < hands; hand++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < each*4; k++ {
				q.list()
				q.find(filepath.Join(home, "ep.mp4"), "plan")
			}
		}()
	}
	// And a hand that stops jobs as soon as it hears of them.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < hands*each; i++ {
			q.cancel(<-ids)
		}
	}()
	wg.Wait()

	// Nothing may be left running: every job was asked to stop. A job
	// that never stops hangs the test until go test's own -timeout.
	q.untilNoneGoing()
	if events.Load() == 0 {
		t.Error("the interface was told nothing")
	}
	if broken.Load() != 0 {
		t.Errorf("%d job snapshots came out half written", broken.Load())
	}
}

// Stopping every job of an episode waits for the ones that are running, so
// the files can be deleted after it. It is asked from several places at
// once, because the interface does not wait for one removal before the
// next.
func TestStoppingAnEpisodeWaitsForItsJobs(t *testing.T) {
	home := "/home/tim"
	st := memoryStore()
	q := newQueue(st, func(JobUpdate) {}, func(string) {})

	episode := filepath.Join(home, "ep.mp4")
	running := make(chan struct{}, 8)
	for i := 0; i < 4; i++ {
		q.add(episode, "plan", "Find clips", func(ctx context.Context, p *engine.Project) (string, error) {
			running <- struct{}{}
			<-ctx.Done()
			return "", engine.ErrCancelled
		})
	}
	// One of them really is running, so the stop has something to wait
	// for rather than finding everything still queued.
	<-running

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !q.cancelEpisode(episode) {
				t.Error("the jobs of the episode did not stop")
			}
		}()
	}
	wg.Wait()

	for _, j := range q.list() {
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("job %s is still %s", j.ID, j.State)
		}
	}
}

// Removing a video, Search after Cancel and quitting go on the moment the
// last job they wait for has stopped. The queue's waits used to look again
// every 100, 20 and 50 ms, so each of them took that much longer than its
// work, #394. A job that will not stop is given stopWait and no more.
//
// The bubble's clock is fake, so the test runs in no time and reads the
// time each wait took exactly. Each job takes 1234 ms to stop once told,
// the way a render ends its ffmpeg, a time no look every 20, 50 or 100 ms
// lands on.
func TestTheQueueGoesOnTheMomentAJobStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const stopping = 1234 * time.Millisecond
		q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
		slow := func(ctx context.Context, p *engine.Project) (string, error) {
			<-ctx.Done()
			time.Sleep(stopping) //nolint:forbidigo // synctest bubble
			return "", engine.ErrCancelled
		}
		removed := q.add("/eps/a.mp4", "render", "Render", slow)
		searched := q.add("/eps/b.mp4", "plan", "Find clips", slow)
		q.add("/eps/c.mp4", "model", "Speech model", slow)
		synctest.Wait()
		for _, j := range q.list() {
			if j.State != JobRunning {
				t.Fatalf("%s is %s, want all three running", j.ID, j.State)
			}
		}

		took := func(what string, wait func() bool) {
			t.Helper()
			start := time.Now()
			if !wait() {
				t.Errorf("%s: the job did not stop", what)
			}
			if got := time.Since(start); got != stopping { //nolint:forbidigo // synctest bubble
				t.Errorf("%s took %v for a job that stopped in %v", what, got, stopping)
			}
		}
		took("removing a video", func() bool { return q.cancelEpisode(removed.Episode) })
		took("Search after Cancel", func() bool {
			q.cancel(searched.ID)
			q.waitJob(searched.ID)
			j, _ := q.find(searched.Episode, searched.Kind)
			return j.State == JobCancelled
		})
		took("quitting", q.shutDown)

		// A job that will not stop is waited for stopWait, then given up on.
		q = newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
		release := make(chan struct{})
		q.add("/eps/d.mp4", "render", "Render", func(ctx context.Context, p *engine.Project) (string, error) {
			<-ctx.Done()
			<-release
			return "", engine.ErrCancelled
		})
		synctest.Wait()
		start := time.Now()
		if q.shutDown() {
			t.Error("quitting said a job stopped that never did")
		}
		if got := time.Since(start); got != stopWait { //nolint:forbidigo // synctest bubble
			t.Errorf("quitting gave up after %v, want stopWait, %v", got, stopWait)
		}
		close(release)
	})
}

// A wait for jobs to stop goes on only once each job's goroutine is done,
// not at the moment its state says it ended. After that moment the job
// still closes its log in the video's work folder, tells the window and
// lets go of its lane. Removing a video deletes that folder right after
// the wait, and on Windows a folder with an open file in it cannot be
// deleted. Found in the review of #504. Here the window's notify, the last
// thing a job does, blocks, so the job's goroutine is held at its end.
func TestAWaitForJobsGoesOnOnlyOnceTheirWorkIsDone(t *testing.T) {
	for _, c := range []struct {
		name string
		wait func(q *queue, j Job) bool
	}{
		{"removing a video", func(q *queue, j Job) bool { return q.cancelEpisode(j.Episode) }},
		{"Search after Cancel", func(q *queue, j Job) bool {
			q.cancel(j.ID)
			q.waitJob(j.ID)
			return true
		}},
		{"quitting", func(q *queue, j Job) bool { return q.shutDown() }},
	} {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				told, hold := make(chan struct{}), make(chan struct{})
				q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {
					close(told)
					<-hold
				})
				j := q.add("/eps/a.mp4", "render", "Render", func(ctx context.Context, p *engine.Project) (string, error) {
					<-ctx.Done()
					return "", engine.ErrCancelled
				})
				synctest.Wait()
				returned := make(chan bool, 1)
				go func() { returned <- c.wait(q, j) }()
				<-told
				synctest.Wait()
				early := false
				select {
				case <-returned:
					early = true
					t.Error("the wait went on while the job was still telling the window it had ended")
				default:
				}
				close(hold)
				if !early && !<-returned {
					t.Error("the job did not stop")
				}
			})
		})
	}
}

// Work asked for twice at once is queued once.
//
// Looking for an existing job and then adding one is two locks with a gap
// between them. Two calls arriving together both look, both see nothing and
// both add, and the result is two transcriptions of one episode or two
// downloads writing over each other's unpacking folder. The interface can
// do that by being opened twice, and a customer can do it by pressing a
// button twice. A test that makes one call at a time proves nothing about
// it.
func TestAskingForTheSameWorkTwiceAtOnceQueuesItOnce(t *testing.T) {
	st := memoryStore()
	q := newQueue(st, func(JobUpdate) {}, func(string) {})

	held := func(ctx context.Context, p *engine.Project) (string, error) {
		<-ctx.Done()
		return "", engine.ErrCancelled
	}

	var wg sync.WaitGroup
	ids := make([]string, 24)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Two episodes, so the test also shows that once-only is per
			// piece of work rather than a queue that takes one job at all.
			episode := "/eps/one.mp4"
			if i%2 == 1 {
				episode = "/eps/two.mp4"
			}
			ids[i] = q.addOnce(episode, "transcribe", "Transcription", held).ID
		}(i)
	}
	wg.Wait()

	live := map[string]int{}
	for _, job := range q.list() {
		if job.State == JobQueued || job.State == JobRunning {
			live[job.Episode]++
		}
	}
	for episode, n := range live {
		if n != 1 {
			t.Errorf("%s has %d jobs queued or running, want 1", episode, n)
		}
	}
	if len(live) != 2 {
		t.Errorf("%d episodes have work, want 2", len(live))
	}
	// Every caller was handed a real job rather than an empty one.
	for i, id := range ids {
		if id == "" {
			t.Errorf("caller %d got no job", i)
		}
	}
	for _, job := range q.list() {
		q.cancel(job.ID)
	}
}

// A lane survives a panic around a job, not only in it. Whatever the queue
// hands its news to belongs to the window, and when that panicked, the
// goroutine that is the lane died with it: every job queued after it waited
// for ever and the app looked frozen with nothing to say why.
func TestALaneSurvivesAPanicAroundAJob(t *testing.T) {
	st := memoryStore()

	q := newQueue(st, func(u JobUpdate) {
		// The first job's news cannot be delivered at all.
		if u.Job.ID == "job-1" {
			panic("the window went away")
		}
	}, func(episode string) {
		panic("nobody is listening")
	})
	quick := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().ProgressOf("working", 0.5, 1)
		return "ok", nil
	}
	first := q.add("/eps/a.mp4", "render", "Render", quick)
	second := q.add("/eps/b.mp4", "render", "Render", quick)
	third := q.add("/eps/c.mp4", "transcribe", "Transcription", quick)

	// A lane that died leaves the job behind it queued, and the test hangs
	// until go test's own -timeout, which shows every goroutine.
	q.untilNoneGoing()
	for _, j := range []Job{first, second, third} {
		got, ok := q.find(j.Episode, j.Kind)
		if !ok || got.State != JobDone {
			t.Errorf("%s ended %q, the lane should have carried on", j.ID, got.State)
		}
	}
}

// A job whose work panics fails with the one line its toast shows, and
// the stack is in the app's log only. The job's error held the whole
// stack, and the toast showed a screen of Go, issue #604.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestAJobThatPanicsSaysWhyInOneLine(t *testing.T) {
	_, read := testLog(t)
	st := memoryStore()
	q := newQueue(st, func(JobUpdate) {}, func(string) {})
	job := q.add("/eps/a.mp4", "render", "Render", func(context.Context, *engine.Project) (string, error) {
		panic("a clip nobody framed before")
	})
	q.untilNoneGoing()
	got, _ := q.find(job.Episode, job.Kind)
	if got.State != JobFailed || got.Error != "Render stopped unexpectedly: a clip nobody framed before" {
		t.Errorf("the job ended %q with %q, want it failed with one line", got.State, got.Error)
	}
	line := find(read(), "the job stopped unexpectedly")
	has(t, line, map[string]any{"level": "error", "msg": "the job stopped unexpectedly: a clip nobody framed before"})
	if stack, _ := line["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("the app's log holds no stack of the panic, %v", line)
	}
}

// Quitting stops every job, running and queued, and nothing new starts.
func TestQuittingStopsEveryJob(t *testing.T) {
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
	waiting := func(ctx context.Context, p *engine.Project) (string, error) {
		<-ctx.Done()
		return "", engine.ErrCancelled
	}
	q.add("/eps/a.mp4", "render", "Render", waiting)
	q.add("/eps/b.mp4", "transcribe", "Transcription", waiting)
	q.add("/eps/c.mp4", "render", "Render", waiting)
	// One job in each of the two lanes runs, the third waits its turn.
	q.until(func(jobs []*Job) bool {
		n := 0
		for _, j := range jobs {
			if j.State == JobRunning {
				n++
			}
		}
		return n == 2
	}, nil)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.add("/eps/d.mp4", "render", "Render", waiting)
		}()
	}
	if !q.shutDown() {
		t.Error("the jobs did not stop")
	}
	wg.Wait()
	if j := q.add("/eps/e.mp4", "render", "Render", waiting); j.State != JobFailed {
		t.Errorf("work was queued after the app closed: %s", j.State)
	}
	// A job that slipped in after shutDown looked would be queued or
	// running now, and nothing has to pass for that to show.
	for _, j := range q.list() {
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("%s %s is %s after the app closed", j.ID, j.Episode, j.State)
		}
	}
}

// The queue under everything the app can do to it at once: work added,
// found, listed and cancelled, episodes closed for removal and stopped,
// while jobs run, report and panic, and the window's own
// news panics too. Each of these has a test of its own above, one at a
// time. Here they all happen together, and then the app quits in the
// middle of them. A lane that died of a panic leaves the work queued
// behind it waiting for good, and a job that slipped past a close or the
// quit runs on with nobody to stop it.
//
// Nothing the test does itself waits on the clock. Each step goes on when
// the queue's news says what it waits for has happened, and the hands are
// counted in rounds, not in milliseconds: 300 ms of hands was not enough
// on a busy Mac runner for any work that breaks to reach a lane, #387. A
// queue that never gets there hangs the test until go test's own
// -timeout, which shows every goroutine. The queue's own waits,
// cancelEpisode and shutDown, go on the moment the jobs they wait for
// have stopped, see TestTheQueueGoesOnTheMomentAJobStops.
func TestTheQueueUnderEverythingTheAppDoesAtOnce(t *testing.T) {
	t.Parallel()
	var events, broken atomic.Int64
	news := newJobNews()
	q := newQueue(memoryStore(), func(u JobUpdate) {
		events.Add(1)
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.Lane == "" || u.Job.State == "" {
			broken.Add(1)
		}
		news.heard(u.Job)
		// Some of the window's news cannot be delivered at all.
		if strings.HasSuffix(u.Job.ID, "3") {
			panic("the window went away")
		}
	}, func(string) { panic("nobody is listening") })

	episodes := []string{"/eps/a.mp4", "/eps/b.mp4", "/eps/c.mp4"}
	kinds := []string{"render", "model", "plan", engine.JobClip}
	waiting := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().ProgressOf("working", 0.5, 1)
		<-ctx.Done()
		return "", engine.ErrCancelled
	}
	quick := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().Info("%s", "working")
		return "ok", nil
	}
	breaks := func(ctx context.Context, p *engine.Project) (string, error) {
		panic("the work broke")
	}
	works := []func(context.Context, *engine.Project) (string, error){waiting, quick, breaks}
	fell := func(j Job) bool {
		return j.State == JobFailed && strings.Contains(j.Error, "stopped unexpectedly")
	}

	// hands does everything the window can do, from several goroutines at
	// once, until stop is closed. Each hand counts down rounds when it has
	// gone round that often.
	hands := func(stop chan struct{}, rounds int) (all, went *sync.WaitGroup) {
		all, went = &sync.WaitGroup{}, &sync.WaitGroup{}
		for hand := range 8 {
			all.Add(1)
			went.Add(1)
			go func() {
				defer all.Done()
				for k := 0; ; k++ {
					if k == rounds {
						went.Done()
					}
					select {
					case <-stop:
						if k < rounds {
							went.Done()
						}
						return
					default:
					}
					episode := episodes[(hand+k)%len(episodes)]
					kind := kinds[(hand*3+k)%len(kinds)]
					switch (hand + k) % 8 {
					case 0, 1, 2:
						job := q.add(episode, kind, "Work", works[(hand+k)%len(works)])
						if k%3 == 0 {
							q.cancel(job.ID)
						}
					case 3:
						q.list()
						q.find(episode, kind)
					case 4:
						reopen := q.closeEpisode(episode)
						q.add(episode, kind, "Work", waiting)
						reopen()
					case 5:
						q.cancelEpisode(episode)
					case 6:
						q.addOnce(episode, kind, "Work", waiting)
					case 7:
						q.busy()
						for _, j := range q.list() {
							if j.Episode == episode && (j.State == JobQueued || j.State == JobRunning) {
								q.cancel(j.ID)
								break
							}
						}
					}
				}
			}()
		}
		return all, went
	}

	// The hands go on until each has gone round 20 times and work that
	// broke has run and ended as failed, so the test is about a panic.
	stop := make(chan struct{})
	all, went := hands(stop, 20)
	went.Wait()
	news.until(func(jobs map[string]Job) bool {
		for _, j := range jobs {
			if fell(j) {
				return true
			}
		}
		return false
	})
	close(stop)
	all.Wait()

	// No lane died of the panics: work queued on each after all of it
	// still runs and ends, once what still waits there is called off.
	for _, j := range q.list() {
		q.cancel(j.ID)
	}
	after := map[string]bool{}
	for _, kind := range kinds {
		after[q.add("/eps/d.mp4", kind, "After", quick).ID] = true
	}
	news.until(func(jobs map[string]Job) bool {
		for id := range after {
			if jobs[id].State != JobDone {
				return false
			}
		}
		return true
	})

	// Then the app quits while the window is still at it.
	stop = make(chan struct{})
	all, went = hands(stop, 10)
	went.Wait()
	stopped := q.shutDown()
	close(stop)
	all.Wait()
	if !stopped {
		t.Error("the jobs did not stop when the app quit")
	}
	if j := q.add("/eps/e.mp4", "render", "Render", waiting); j.State != JobFailed {
		t.Errorf("work was queued after the app quit: %s", j.State)
	}
	// What was running when it quit has ended, which shutDown waited
	// for, and nothing slipped in while it did.
	failed := 0
	for _, j := range q.list() {
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("%s %s is still %s after the app quit", j.ID, j.Episode, j.State)
		}
		if fell(j) {
			failed++
		}
	}
	if failed == 0 {
		t.Error("no work that broke ended as failed, so none of it ran")
	}
	if events.Load() == 0 {
		t.Error("the window was told nothing")
	}
	if broken.Load() != 0 {
		t.Errorf("%d job snapshots came out half written", broken.Load())
	}
}

// logJobsOnFailure logs every job of the queue once the test has failed,
// so a failure says what was left running and how each job ended. A hang
// that go test's own -timeout ends runs no cleanup. It shows every
// goroutine instead.
func logJobsOnFailure(t testing.TB, q *queue) {
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, j := range q.list() {
			t.Logf("%s %s of %s is %s, step %q, error %q", j.ID, j.Kind, j.Episode, j.State, j.Step, j.Error)
		}
	})
}

// jobNews is what the window has heard of each job, the newest snapshot
// by its number, for a test to wait on.
type jobNews struct {
	mu     sync.Mutex
	change *sync.Cond
	jobs   map[string]Job
}

func newJobNews() *jobNews {
	n := &jobNews{jobs: map[string]Job{}}
	n.change = sync.NewCond(&n.mu)
	return n
}

// heard keeps a snapshot unless a newer one of the job came first.
func (n *jobNews) heard(j Job) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if have, ok := n.jobs[j.ID]; ok && have.Seq >= j.Seq {
		return
	}
	n.jobs[j.ID] = j
	n.change.Broadcast()
}

// until waits for the news that makes done true.
func (n *jobNews) until(done func(map[string]Job) bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for !done(n.jobs) {
		n.change.Wait()
	}
}

// untilNoneGoing waits until no job of the queue is queued or running, on
// the queue's own news of every change, see until.
func (q *queue) untilNoneGoing() {
	q.until(func(jobs []*Job) bool { return !slices.ContainsFunc(jobs, (*Job).going) }, nil)
}

// Whatever order the news reaches the window in, the snapshot of a job
// with the largest number is the job as it ended. News is sent after the
// queue's lock is let go, so a job's queued and running could arrive the
// other way round, and a window that kept the last one it heard showed a
// finished job as waiting, for good.
func TestTheNewestNewsOfAJobIsHowItEnded(t *testing.T) {
	news := newJobNews()
	q := newQueue(memoryStore(), func(u JobUpdate) {
		if u.Job.Seq == 0 {
			t.Errorf("%s arrived without a number", u.Job.ID)
		}
		news.heard(u.Job)
	}, func(string) {})

	quick := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().ProgressOf("working", 0.5, 1)
		return "", nil
	}
	// Work that ends by itself unless it was called off by then.
	waiting := func(ctx context.Context, p *engine.Project) (string, error) {
		runtime.Gosched()
		if ctx.Err() != nil {
			return "", engine.ErrCancelled
		}
		return "", nil
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25 {
				episode := fmt.Sprintf("/eps/%d.mp4", (g+i)%5)
				var j Job
				switch i % 4 {
				case 0:
					j = q.add(episode, "render", "Render", quick)
				case 1:
					j = q.addOnce(episode, "transcribe", "Transcription", waiting)
				case 2:
					j = q.add(episode, "plan", "Find clips", waiting)
				default:
					j = q.add(episode, "render", "Render", waiting)
				}
				if i%3 == 0 {
					q.cancel(j.ID)
				}
				if i%7 == 0 {
					q.cancel(j.ID)
				}
			}
		}()
	}
	wg.Wait()
	// And one job called off while it runs, on purpose. It holds the lane
	// of hearing, which none of the work above uses, until it is told to
	// stop, and it is told once its news says it runs.
	held := q.add("/eps/held.mp4", "model", "Speech model", func(ctx context.Context, p *engine.Project) (string, error) {
		<-ctx.Done()
		return "", engine.ErrCancelled
	})
	news.until(func(jobs map[string]Job) bool { return jobs[held.ID].State == JobRunning })
	q.cancel(held.ID)
	// Every job was heard of as it was added, so the news has them all.
	// It says each has ended once its last snapshot has arrived, and the
	// queue's state cannot be newer than that.
	news.until(func(jobs map[string]Job) bool {
		for _, j := range jobs {
			if j.going() {
				return false
			}
		}
		return true
	})
	news.mu.Lock()
	defer news.mu.Unlock()
	for _, j := range q.list() {
		heard := news.jobs[j.ID]
		if heard.State != j.State {
			t.Errorf("%s ended %s, and the newest news of it says %s", j.ID, j.State, heard.State)
		}
	}
	if j := news.jobs[held.ID]; j.State != JobCancelled {
		t.Errorf("the job called off while it ran ended %s, and the news says so", j.State)
	}
}

// What a job says is kept in a file in its episode's work folder, detail
// lines too, since the app has no page for it: that file is where a
// failure is looked into.
//
// On the real disk: a job log is opened inside the work folder through
// os.Root, which no link leads out of.
func TestAJobKeepsItsLogInTheWorkFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	st := openStore()
	episode := filepath.Join(home, "ep.mp4")
	if err := os.WriteFile(episode, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 1)
	q := newQueue(st, func(u JobUpdate) {
		if u.Job.State == JobFailed {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}, func(string) {})
	q.add(episode, engine.JobRender, "Work", func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().Info("%s", "rendering the first clip")
		p.Log().Detail("%s", "ffmpeg said something odd")
		return "", fmt.Errorf("%s", "the disk is full")
	})
	<-done
	// The log writes each line to the file as it is said, unbuffered, so
	// what the work said is there before the job says it ended, although
	// the file is closed only after that.
	logs, _ := filepath.Glob(filepath.Join(engine.JobsDir(episode), "*-render.log"))
	if len(logs) != 1 {
		t.Fatalf("%d job logs, want 1: %v", len(logs), logs)
	}
	b, err := os.ReadFile(logs[0])
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{"rendering the first clip", "ffmpeg said something odd"} {
		if !strings.Contains(body, want) {
			t.Errorf("the log has no %q:\n%s", want, body)
		}
	}
}

// Relaunch finds the queue idle and closes it in one step, so no search
// starts between the two and is stopped half way by the quit, issue #382.
// That it is one step is held by closeIfIdle, which looks and closes
// under one lock. No test can make the scheduler land in a gap between
// the two, so none proves it.
//
// What the test proves: in each round the queue's work ends, told by the
// jobs' news, so the queue is idle at a moment the test knows rather than
// one it hopes for, issue #455. Relaunch must find it so, and refuse the
// work added after with the reason. Before that, in each round, work is
// added from several goroutines at once while Relaunch asks again and
// again, for the race detector, and whenever Relaunch was told the queue
// was idle, nothing may run or wait until it reopens.
func TestRelaunchLetsNoWorkStartAfterIt(t *testing.T) {
	t.Parallel()
	news := newJobNews()
	q := newQueue(memoryStore(), func(u JobUpdate) { news.heard(u.Job) }, func(string) {})
	// ended waits for the news that every job named has ended. A job's
	// news is sent after the queue has said so, so from then on the queue
	// is idle unless other work came in.
	ended := func(ids []string) {
		news.until(func(jobs map[string]Job) bool {
			for _, id := range ids {
				if j, ok := jobs[id]; !ok || j.State == JobQueued || j.State == JobRunning {
					return false
				}
			}
			return true
		})
	}
	const relaunching = "the app is relaunching into a new build"

	// While work runs, Relaunch is refused and closes nothing.
	block := make(chan struct{})
	held := q.add("/eps/held.mp4", "render", "Held", func(ctx context.Context, p *engine.Project) (string, error) {
		<-block
		return "ok", nil
	})
	if reopen, ok := q.closeIfIdle(); ok {
		reopen()
		t.Fatal("Relaunch found the queue idle while work ran")
	}
	more := q.add("/eps/more.mp4", "render", "More", func(context.Context, *engine.Project) (string, error) { return "ok", nil })
	if more.State == JobFailed {
		t.Fatalf("work was refused after a Relaunch that was refused: %s", more.Error)
	}
	close(block)
	ended([]string{held.ID, more.ID})

	brief := func(ctx context.Context, p *engine.Project) (string, error) { return "ok", nil }
	kinds := []string{"render", "model", "plan", engine.JobSearch}
	// burst lets four hands add n pieces of work each, all at once. done
	// closes when they have, and took hands back the work the queue took.
	burst := func(round, n int, start <-chan struct{}) (done <-chan struct{}, took func() []string) {
		var mu sync.Mutex
		var ids []string
		var wg sync.WaitGroup
		for hand := range 4 {
			wg.Go(func() {
				<-start
				for k := range n {
					job := q.add(fmt.Sprintf("/eps/%d.mp4", hand), kinds[(round+hand+k)%len(kinds)], "Work", brief)
					switch {
					case job.State != JobFailed:
						mu.Lock()
						ids = append(ids, job.ID)
						mu.Unlock()
					case job.Error != relaunching:
						t.Errorf("work refused with %q", job.Error)
					}
				}
			})
		}
		added := make(chan struct{})
		go func() {
			wg.Wait()
			close(added)
		}()
		return added, func() []string {
			<-added
			return ids
		}
	}

	for round := range 20 {
		// Relaunch asks again and again while the hands add, so the race
		// detector sees the two at once. Whichever comes first, once it has
		// found the queue idle nothing runs or waits until it reopens.
		start := make(chan struct{})
		done, took := burst(round, 40, start)
		close(start)
		for asking := true; asking; {
			select {
			case <-done:
				asking = false
			default:
			}
			reopen, idle := q.closeIfIdle()
			for range 20 {
				if idle && q.busy() {
					t.Fatal("work ran or waited after Relaunch found the queue idle")
				}
				runtime.Gosched()
			}
			reopen()
		}
		added := took()

		// Once all of that has ended the queue is idle, and Relaunch finds
		// it so. Work asked for after that is refused, and none runs.
		ended(added)
		reopen, idle := q.closeIfIdle()
		if !idle {
			t.Fatalf("round %d: Relaunch did not find the queue idle once its work had ended", round)
		}
		start = make(chan struct{})
		_, took = burst(round, 3, start)
		close(start)
		if added := took(); len(added) > 0 {
			t.Fatalf("round %d: %d pieces of work were taken after Relaunch found the queue idle", round, len(added))
		}
		if q.busy() {
			t.Fatal("work ran or waited after Relaunch found the queue idle")
		}
		reopen()
		reopen()
	}
	// Reopened, it takes work again.
	if job := q.add("/eps/after.mp4", "render", "After", brief); job.State == JobFailed {
		t.Errorf("work after the queue reopened was refused: %s", job.Error)
	}
}

// A search that has ended asks for the model to be kept again, because it
// may have put the kept model aside for another one, see keep.go.
func TestASearchThatEndsKeepsTheModelAgain(t *testing.T) {
	t.Parallel()
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
	searched := make(chan struct{}, 1)
	q.searched = func() { searched <- struct{}{} }
	q.add("/home/tim/ep.mp4", engine.JobSearch, "Search", func(context.Context, *engine.Project) (string, error) {
		return "", nil
	})
	<-searched
}

// A job that hears a video in parts has the library read again each time
// it goes on to the next part, so the parts the library says were heard
// keep up with the work rather than catching up only when the job ends.
// Read only at the end, the share of a video heard went back each time the
// work went on to the next part and jumped from 60 to 96 % when it was
// stopped, found by Tim on PR #458.
func TestTheLibraryIsReadAgainWhenTheHearingGoesOnToTheNextPart(t *testing.T) {
	t.Parallel()
	told := make(chan string, 8)
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(episode string) { told <- episode })
	part := make(chan struct{})
	q.add("/eps/ep.mp4", engine.JobTranscribe, "Transcribe whole video", func(ctx context.Context, p *engine.Project) (string, error) {
		// The same part going on is no news to the library, the next is.
		p.Log().ProgressTo("transcribing", 0.1, 10, 0, 10)
		p.Log().ProgressTo("transcribing", 0.2, 9, 0, 20)
		p.Log().ProgressTo("transcribing", 0.3, 8, 100, 110)
		<-part
		return "", nil
	})
	if got := <-told; got != "/eps/ep.mp4" {
		t.Fatalf("the library was told of %q", got)
	}
	select {
	case got := <-told:
		t.Fatalf("the library was told of %q again before the job ended", got)
	default:
	}
	close(part)
	// And once more when it ends, as every job does.
	if got := <-told; got != "/eps/ep.mp4" {
		t.Fatalf("at the end the library was told of %q", got)
	}
}

// Stop transcribing keeps the record, so the menu says Continue
// transcribing after a restart too, and Continue clicked while the work
// still stops keeps the record of the job it started. Found by the review
// of #458: the record was removed once the stopped job ended, the new
// job's with it.
func TestStopTranscribingKeepsTheRecordOfTheJobThatCarriesOn(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "ep.mp4")
	ep := eps[0]
	news := make(chan Job, 256)
	svc.jobs = newQueue(svc.store, func(u JobUpdate) { news <- u.Job }, func(string) {})
	record := func() (engine.JobRecord, bool) {
		for _, rec := range engine.ReadJobs(svc.store.files, ep) {
			if rec.ID == engine.TranscribeID {
				return rec, true
			}
		}
		return engine.JobRecord{}, false
	}
	// A stand-in for TranscribeJob: it writes its record and hears until
	// it is called off, and then holds until it is let go.
	started := make(chan struct{})
	transcribe := func(letGo chan struct{}) Job {
		return svc.jobs.addSteps(ep, engine.JobTranscribe, "Transcribe whole video", true, func(j *Job) {
			j.JobRecord.ID = engine.TranscribeID
		}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
			rec := engine.JobRecord{ID: engine.TranscribeID, Kind: engine.JobTranscribe, Step: engine.StepHearing}
			if err := engine.WriteJob(svc.store.files, ep, rec); err != nil {
				return "", err
			}
			started <- struct{}{}
			<-ctx.Done()
			<-letGo
			return "", engine.ErrCancelled
		})
	}
	ended := func(id string) Job {
		for j := range news {
			if j.ID == id && j.State != JobQueued && j.State != JobRunning {
				return j
			}
		}
		panic("the news ended")
	}

	firstGo := make(chan struct{})
	first := transcribe(firstGo)
	<-started
	if !svc.cancelSteps(first.ID) {
		t.Fatal("Stop transcribing found no job")
	}
	// Continue transcribing, clicked while the first still stops.
	secondGo := make(chan struct{})
	second := transcribe(secondGo)
	if second.ID == first.ID {
		t.Fatal("Continue gave the job that stops")
	}
	<-started
	close(firstGo)
	if j := ended(first.ID); j.State != JobInterrupted {
		t.Errorf("Stop transcribing ended the job as %s", j.State)
	}
	if rec, ok := record(); !ok || rec.Step == engine.StepStopped {
		t.Errorf("the job that carries on lost its record: %+v, there %v", rec, ok)
	}

	svc.cancelSteps(second.ID)
	close(secondGo)
	if j := ended(second.ID); j.State != JobInterrupted || j.Step != engine.StepStopped {
		t.Errorf("Stop transcribing ended the job as %s, %s", j.State, j.Step)
	}
	if rec, ok := record(); !ok || rec.Step != engine.StepStopped {
		t.Errorf("after Stop transcribing the record is %+v, there %v", rec, ok)
	}
}

// A job holds its record instead of copying it field by field, issue
// #606, and still reaches the window with every field the window reads,
// under the names it reads them by. A search cut off as the app closed,
// read back from its record, is the case with the most of them: its id
// must be the job's and not the record's, or Continue and Cancel name a
// job the queue does not have, and its window and its numbers must come
// across, or its row says nothing of what it was searching.
func TestAJobReachesTheWindowWithEverythingItsRecordSays(t *testing.T) {
	s, eps := memoryLibrary(t, "talk.mp4")
	var sent []JobUpdate
	s.jobs = newQueue(s.store, func(u JobUpdate) { sent = append(sent, u) }, func(string) {})
	rec := engine.JobRecord{ID: engine.SearchID, Kind: engine.JobSearch, From: 60, To: 240, Count: 3,
		Min: 20, Step: engine.StepFinding, Asked: time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)}
	if err := engine.WriteJob(s.store.files, eps[0], rec); err != nil {
		t.Fatal(err)
	}
	s.jobs.restore(eps)
	if len(sent) != 1 {
		t.Fatalf("%d jobs were sent, want the one search", len(sent))
	}
	raw, err := json.Marshal(sent[0].Job)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"id": "job-1", "episode": eps[0], "kind": "search", "label": "Find clips", "state": "interrupted",
		"step": "finding", "from": 60.0, "to": 240.0, "count": 3.0, "min": 20.0, "lane": "finding",
		"queued": "2026-10-31T09:00:00Z",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s is %v, want %v, in %s", key, got[key], value, raw)
		}
	}
}
