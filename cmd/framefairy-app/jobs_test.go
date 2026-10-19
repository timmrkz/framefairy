package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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

	// Nothing may be left running: every job was asked to stop.
	deadline := time.Now().Add(20 * time.Second)
	for {
		busy := 0
		for _, j := range q.list() {
			if j.State == JobQueued || j.State == JobRunning {
				busy++
			}
		}
		if busy == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d jobs never stopped", busy)
		}
		time.Sleep(20 * time.Millisecond)
	}
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
	// Wait until one of them really is running, so the stop has something
	// to wait for rather than finding everything still queued.
	select {
	case <-running:
	case <-time.After(10 * time.Second):
		t.Fatal("no job ever started")
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.cancelEpisode(episode)
		}()
	}
	wg.Wait()

	for _, j := range q.list() {
		if j.State == JobQueued || j.State == JobRunning {
			t.Errorf("job %s is still %s", j.ID, j.State)
		}
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

	var mu sync.Mutex
	finished := map[string]string{}
	q := newQueue(st, func(u JobUpdate) {
		mu.Lock()
		if u.Job.State != JobQueued && u.Job.State != JobRunning {
			finished[u.Job.ID] = u.Job.State
		}
		mu.Unlock()
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

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		_, b := finished[second.ID]
		_, c := finished[third.ID]
		mu.Unlock()
		if b && c {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, j := range []Job{first, second, third} {
		got, ok := q.find(j.Episode, j.Kind)
		if !ok || got.State != JobDone {
			t.Errorf("%s ended %q, the lane should have carried on", j.ID, got.State)
		}
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n := 0
		for _, j := range q.list() {
			if j.State == JobRunning {
				n++
			}
		}
		if n == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	time.Sleep(50 * time.Millisecond)
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
func TestTheQueueUnderEverythingTheAppDoesAtOnce(t *testing.T) {
	t.Parallel()
	var events, broken atomic.Int64
	q := newQueue(memoryStore(), func(u JobUpdate) {
		events.Add(1)
		if u.Job.ID == "" || u.Job.Episode == "" || u.Job.Lane == "" || u.Job.State == "" {
			broken.Add(1)
		}
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

	// hands does everything the window can do, from several goroutines at
	// once, until stop is closed.
	hands := func(stop chan struct{}) *sync.WaitGroup {
		var wg sync.WaitGroup
		for hand := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; ; k++ {
					select {
					case <-stop:
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
		return &wg
	}

	stop := make(chan struct{})
	wg := hands(stop)
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()

	// No lane died of the panics: work queued on each after all of it
	// still runs and ends, once what still waits there is called off.
	for _, j := range q.list() {
		q.cancel(j.ID)
	}
	after := map[string]Job{}
	for _, kind := range kinds {
		job := q.add("/eps/d.mp4", kind, "After", quick)
		after[job.ID] = job
	}
	deadline := time.Now().Add(20 * time.Second)
	for len(after) > 0 {
		for _, j := range q.list() {
			if _, waited := after[j.ID]; waited && j.State == JobDone {
				delete(after, j.ID)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d lanes never ran the work queued after the panics: %v", len(after), after)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Then the app quits while the window is still at it.
	stop = make(chan struct{})
	wg = hands(stop)
	time.Sleep(100 * time.Millisecond)
	stopped := q.shutDown()
	close(stop)
	wg.Wait()
	if !stopped {
		t.Error("the jobs did not stop when the app quit")
	}
	if j := q.add("/eps/e.mp4", "render", "Render", waiting); j.State != JobFailed {
		t.Errorf("work was queued after the app quit: %s", j.State)
	}
	// What was running when it quit has ended, and nothing slipped in
	// while it did.
	deadline = time.Now().Add(20 * time.Second)
	for {
		var left []string
		for _, j := range q.list() {
			if j.State == JobQueued || j.State == JobRunning {
				left = append(left, j.ID+" "+j.Episode+" "+j.State)
			}
		}
		if len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still going after the app quit: %v", left)
		}
		time.Sleep(20 * time.Millisecond)
	}
	failed := 0
	for _, j := range q.list() {
		if j.State == JobFailed && strings.Contains(j.Error, "stopped unexpectedly") {
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

// Whatever order the news reaches the window in, the snapshot of a job
// with the largest number is the job as it ended. News is sent after the
// queue's lock is let go, so a job's queued and running could arrive the
// other way round, and a window that kept the last one it heard showed a
// finished job as waiting, for good.
func TestTheNewestNewsOfAJobIsHowItEnded(t *testing.T) {
	var mu sync.Mutex
	newest := map[string]Job{}
	q := newQueue(memoryStore(), func(u JobUpdate) {
		mu.Lock()
		defer mu.Unlock()
		if u.Job.Seq == 0 {
			t.Errorf("%s arrived without a number", u.Job.ID)
		}
		if have, ok := newest[u.Job.ID]; !ok || u.Job.Seq > have.Seq {
			newest[u.Job.ID] = u.Job
		}
	}, func(string) {})

	quick := func(ctx context.Context, p *engine.Project) (string, error) {
		p.Log().ProgressOf("working", 0.5, 1)
		return "", nil
	}
	waiting := func(ctx context.Context, p *engine.Project) (string, error) {
		select {
		case <-ctx.Done():
			return "", engine.ErrCancelled
		case <-time.After(5 * time.Millisecond):
			return "", nil
		}
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
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		busy := false
		for _, j := range q.list() {
			if j.State == JobQueued || j.State == JobRunning {
				busy = true
			}
		}
		if !busy {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, j := range q.list() {
		heard := newest[j.ID]
		if heard.State != j.State {
			t.Errorf("%s ended %s, and the newest news of it says %s", j.ID, j.State, heard.State)
		}
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
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the job never ended")
	}
	// The file is closed after the job says it ended, so it is read once
	// it holds what was said.
	var body string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logs, _ := filepath.Glob(filepath.Join(engine.JobsDir(episode), "*-render.log"))
		if len(logs) == 1 {
			b, _ := os.ReadFile(logs[0])
			body = string(b)
			if strings.Contains(body, "ffmpeg said something odd") {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, want := range []string{"rendering the first clip", "ffmpeg said something odd"} {
		if !strings.Contains(body, want) {
			t.Errorf("the log has no %q:\n%s", want, body)
		}
	}
}

// Relaunch finds the queue idle and closes it in one step, so no search
// starts between the two and is stopped half way by the quit, issue #382.
// Work is added from several goroutines at once while Relaunch asks again
// and again: whenever it was told the queue was idle, nothing runs or
// waits until it reopens, and work added meanwhile is refused with the
// reason.
func TestRelaunchLetsNoWorkStartAfterIt(t *testing.T) {
	t.Parallel()
	q := newQueue(memoryStore(), func(JobUpdate) {}, func(string) {})
	quick := func(ctx context.Context, p *engine.Project) (string, error) { return "ok", nil }
	kinds := []string{"render", "model", "plan", engine.JobSearch}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for hand := range 4 {
		wg.Go(func() {
			for k := 0; ; k++ {
				select {
				case <-stop:
					return
				default:
				}
				job := q.add(fmt.Sprintf("/eps/%d.mp4", hand), kinds[(hand+k)%len(kinds)], "Work", quick)
				if job.State == JobFailed && job.Error != "the app is relaunching into a new build" {
					t.Errorf("work refused with %q", job.Error)
				}
				time.Sleep(time.Duration(k%3) * time.Millisecond)
			}
		})
	}

	var idle, refused int
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		reopen, ok := q.closeIfIdle()
		if !ok {
			refused++
			reopen()
			continue
		}
		idle++
		for range 20 {
			if q.busy() {
				t.Fatal("work ran or waited after Relaunch found the queue idle")
			}
			time.Sleep(50 * time.Microsecond)
		}
		reopen()
		reopen()
	}
	close(stop)
	wg.Wait()
	if idle == 0 || refused == 0 {
		t.Fatalf("Relaunch found the queue idle %d times and busy %d times, both are needed to prove anything", idle, refused)
	}
	// Reopened, it takes work again.
	if job := q.add("/eps/after.mp4", "render", "After", quick); job.State == JobFailed {
		t.Errorf("work after the queue reopened was refused: %s", job.Error)
	}
}
