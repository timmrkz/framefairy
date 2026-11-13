package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/engine"
)

// newsOf is a videoNews whose reads are counted, each read giving the
// number of changes made before it, so a test can tell what a state sent
// knew.
type newsOf struct {
	news    *videoNews
	changes atomic.Int64
	mu      sync.Mutex
	sent    []VideoState
	sentAt  []time.Time
}

func countedNews() *newsOf {
	n := &newsOf{}
	n.news = newVideoNews(func(path string) (VideoState, error) {
		return VideoState{Video: path, Rev: uint64(n.changes.Load())}, nil
	}, func(v VideoState) {
		n.mu.Lock()
		defer n.mu.Unlock()
		n.sent = append(n.sent, v)
		n.sentAt = append(n.sentAt, time.Now())
	})
	return n
}

// change makes a change and says so.
func (n *newsOf) change(path string) {
	n.changes.Add(1)
	n.news.changed(path)
}

func (n *newsOf) states() ([]VideoState, []time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.sent), slices.Clone(n.sentAt)
}

// A video whose work goes on changes many times a second. Its state is
// sent at once the first time, then at most four times a second, and once
// more after the last change, so the window ends on what is there.
func TestAVideosStateIsSentAtMostFourTimesASecondAndAfterTheLastChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		start := time.Now()
		// A change every 10 ms for a second.
		for range 100 {
			n.change("a.mp4")
			time.Sleep(10 * time.Millisecond) //nolint:forbidigo // synctest bubble
		}
		time.Sleep(time.Second) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		sent, at := n.states()
		if len(sent) == 0 {
			t.Fatal("nothing was sent")
		}
		if !at[0].Equal(start) {
			t.Errorf("the first state was sent %v after the first change, not at once", at[0].Sub(start))
		}
		for i := 1; i < len(at); i++ {
			if gap := at[i].Sub(at[i-1]); gap < newsEvery {
				t.Errorf("state %d was sent %v after the one before, less than %v", i, gap, newsEvery)
			}
		}
		// A second of changes is five sends at most: at 0, 250, 500, 750
		// and 1000 ms.
		if len(sent) > 5 {
			t.Errorf("%d states sent for a second of changes", len(sent))
		}
		if last := sent[len(sent)-1]; last.Rev != 100 {
			t.Errorf("the last state sent knew %d of the 100 changes", last.Rev)
		}
	})
}

// Each video is sent on its own clock: a busy video does not hold back
// the state of another.
func TestEachVideosStateIsSentOnItsOwn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		n.change("a.mp4")
		synctest.Wait()
		n.change("a.mp4")
		n.change("b.mp4")
		synctest.Wait()
		sent, _ := n.states()
		videos := []string{}
		for _, v := range sent {
			videos = append(videos, v.Video)
		}
		// a at once, b at once, and a again only after its wait.
		if !slices.Equal(videos, []string{"a.mp4", "b.mp4"}) {
			t.Errorf("sent %v at once", videos)
		}
		time.Sleep(newsEvery) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if sent, _ = n.states(); len(sent) != 3 || sent[2].Video != "a.mp4" {
			t.Errorf("after the wait %d states were sent, the last of %s", len(sent), sent[len(sent)-1].Video)
		}
	})
}

// A send of a video's state that begins while the one before it still
// reads waits for it, so the states go out in the order they were read.
// Side by side, the newer could go first and the older after it, and what
// kept the last one sent kept the older. That is how
// TestPathWhatChangesAVideoSendsItsState failed on the Mac's CI: the
// removal tells the video twice, the first send read at once, and
// sendDue in bridge_test.go ran the second beside it, so the first read
// only had to end a few milliseconds after the second. In the app the
// second waits for its quarter second, so a read slower than that is what
// it takes there.
func TestAVideosStatesGoOutInTheOrderTheyWereRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var changes atomic.Int64
		slow := make(chan struct{})
		var reads atomic.Int64
		var mu sync.Mutex
		var sent []uint64
		news := newVideoNews(func(path string) (VideoState, error) {
			rev := uint64(changes.Load())
			if reads.Add(1) == 1 {
				<-slow
			}
			return VideoState{Video: path, Rev: rev}, nil
		}, func(v VideoState) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, v.Rev)
		})
		changes.Add(1)
		news.changed("a.mp4")
		synctest.Wait()
		// A change while the first state is still being read, and the wait
		// after it long over.
		changes.Add(1)
		news.changed("a.mp4")
		time.Sleep(time.Second) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		close(slow)
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(sent, []uint64{1, 2}) {
			t.Errorf("the states went out as %v, not in the order they were read", sent)
		}
	})
}

// A search that writes a clip and then logs a line every 10 ms, on a
// machine so busy that a state takes 400 ms to read, longer than the wait
// between two sends. Each line asks for a state. The states still go out
// one at a time with at most one waiting, so the state that says the
// search is over, five seconds on, reaches the window within two reads,
// not behind a read for every line.
func TestASlowReadKeepsNoBacklogOfStates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const read = 400 * time.Millisecond
		const lines = 500
		var changes, reads atomic.Int64
		var mu sync.Mutex
		var endAt time.Time
		news := newVideoNews(func(path string) (VideoState, error) {
			reads.Add(1)
			rev := uint64(changes.Load())
			time.Sleep(read) //nolint:forbidigo // synctest bubble
			return VideoState{Video: path, Rev: rev}, nil
		}, func(v VideoState) {
			mu.Lock()
			defer mu.Unlock()
			if v.Rev == lines+2 && endAt.IsZero() {
				endAt = time.Now()
			}
		})
		changes.Add(1)
		news.changed("a.mp4")
		for range lines {
			time.Sleep(10 * time.Millisecond) //nolint:forbidigo // synctest bubble
			changes.Add(1)
			news.changed("a.mp4")
		}
		over := time.Now()
		changes.Add(1)
		news.changed("a.mp4")
		// Long enough for a backlog of a read for every line to run out,
		// so it fails here and not as goroutines left behind.
		time.Sleep(10 * time.Minute) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if endAt.IsZero() {
			t.Fatal("the state that says the search is over was never sent")
		}
		// The read still under way, then its own.
		if late := endAt.Sub(over); late > 2*read {
			t.Errorf("the end went out %v after it came, more than two reads of %v", late, read)
		}
		// One read after another for the five seconds, and the last two.
		if n := reads.Load(); n > 5*time.Second.Milliseconds()/read.Milliseconds()+3 {
			t.Errorf("%d states were read for five seconds of changes, more than one read after another", n)
		}
	})
}

// The jobs, the measuring and the edits all say a video changed, each on
// a goroutine of its own. The state is still sent once per wait, and the
// last one sent knows of every change.
func TestAVideosStateIsSentRightWhenItChangesFromEverywhereAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 50 {
					n.change("a.mp4")
					time.Sleep(time.Millisecond) //nolint:forbidigo // synctest bubble
				}
			})
		}
		wg.Wait()
		time.Sleep(newsEvery) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		sent, _ := n.states()
		if last := sent[len(sent)-1]; last.Rev != 400 {
			t.Errorf("the last state sent knew %d of the 400 changes", last.Rev)
		}
		if len(sent) > 2+int(50*time.Millisecond/newsEvery) {
			t.Errorf("%d states sent for 50 ms of changes", len(sent))
		}
	})
}

// Once the app quits nothing more is sent, not even what was due.
func TestNoStateIsSentOnceTheAppQuits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		n.change("a.mp4")
		synctest.Wait()
		n.change("a.mp4")
		n.news.stop()
		n.change("a.mp4")
		time.Sleep(time.Second) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if sent, _ := n.states(); len(sent) != 1 {
			t.Errorf("%d states sent, the app quit after the first", len(sent))
		}
	})
}

// A video that left the library is not sent, and not kept either.
func TestAVideoThatLeftTheLibraryIsNotSent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sent := 0
		news := newVideoNews(func(string) (VideoState, error) { return VideoState{}, errNotInLibrary },
			func(VideoState) { sent++ })
		news.changed("gone.mp4")
		synctest.Wait()
		news.mu.Lock()
		kept := len(news.due)
		news.mu.Unlock()
		if sent != 0 || kept != 0 {
			t.Errorf("%d states sent and %d videos kept for a video not in the library", sent, kept)
		}
	})
}

// The workspace takes a video's state only when it is newer than what it
// has, issue #606. The state is read after the jobs it holds, so it is
// newer than every one of them, and an edit's answer is newer than a state
// read before the edit and older than one read after it, which holds it.
//
// On the real disk: the video is a copy on the disk, and its clips are
// fitted to the picture ffprobe reads.
func TestPathAStateIsNewerThanItsJobsAndAnEditBeforeIt(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	ctx := context.Background()
	before, err := d.svc.State(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Jobs) == 0 {
		t.Fatal("the state holds none of the video's jobs")
	}
	for _, j := range before.Jobs {
		if j.Seq >= before.Rev {
			t.Errorf("the job %s is at %d, the state that holds it at %d", j.ID, j.Seq, before.Rev)
		}
	}
	// What the state holds is what the clip list and the coverage say.
	clips := d.clips(ep)
	if len(before.Clips) == 0 || len(before.Clips) != len(clips) {
		t.Fatalf("the state holds %d clips, the list %d", len(before.Clips), len(clips))
	}
	coverage, err := d.svc.Coverage(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before.Coverage.Passes, coverage.Passes) {
		t.Errorf("the state says %v was searched, the coverage %v", before.Coverage.Passes, coverage.Passes)
	}
	if before.Status.Source != ep || before.Status.Missing {
		t.Errorf("the state's status is of %q, missing %v", before.Status.Source, before.Status.Missing)
	}
	first := before.Clips[0]
	edited, err := d.svc.RemoveClip(ctx, ep, first.Plan, first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Rev <= before.Rev {
		t.Errorf("the edit answered at %d, the state read before it is at %d", edited.Rev, before.Rev)
	}
	after, err := d.svc.State(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	if after.Rev <= edited.Rev {
		t.Errorf("the state read after the edit is at %d, the edit at %d", after.Rev, edited.Rev)
	}
	i := slices.IndexFunc(after.Clips, func(c ClipEntry) bool { return c.Key == first.Key })
	if i < 0 || !after.Clips[i].Rejected {
		t.Errorf("the state read after the clip was removed does not hold it removed")
	}
	if _, err := d.svc.State(ctx, ep+".other"); !errors.Is(err, errNotInLibrary) {
		t.Errorf("a video not in the library has a state: %v", err)
	}
}

// A walk's mark comes behind every state due to be sent, so a walk that
// waits for the mark sees what the Go side had to say, see caughtUp in
// bridge.mjs. Nothing waits out the four times a second for it.
func TestAStateDueIsSentBeforeAMark(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		n.change("a.mp4")
		synctest.Wait()
		n.change("a.mp4")
		n.change("b.mp4")
		sendDue(n.news)
		sent, at := n.states()
		if len(sent) != 3 || sent[len(sent)-1].Rev != 3 {
			t.Fatalf("%d states sent by the mark, the last knowing %d of 3 changes", len(sent), sent[len(sent)-1].Rev)
		}
		if !at[2].Equal(at[0]) {
			t.Errorf("the state due was sent %v after the first, not at the mark", at[2].Sub(at[0]))
		}
		// What was due is sent, and not again.
		time.Sleep(time.Second) //nolint:forbidigo // synctest bubble
		synctest.Wait()
		if sent, _ = n.states(); len(sent) != 3 {
			t.Errorf("%d states sent in the end, 3 were due", len(sent))
		}
	})
}

// A job's news goes out once the state of its video is due, so a walk
// that hears the news and then asks for a mark sees that state before the
// mark, see sendDue. Sent the other way round, the mark could come
// between the two and the walk read a state from before the job's news.
func TestAJobsNewsComesOnceItsVideosStateIsDue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := countedNews()
		n.change("a.mp4")
		synctest.Wait()
		sentAtNews := -1
		q := newQueue(nil, func(JobUpdate) {
			sendDue(n.news)
			sent, _ := n.states()
			sentAtNews = len(sent)
		}, nil)
		q.news = n.change
		q.emit(JobUpdate{Job: Job{Episode: "a.mp4"}})
		if sentAtNews != 2 {
			t.Errorf("%d states were sent by a mark the job's news asked for, the first and the one the job asked for were due", sentAtNews)
		}
	})
}

// heardStates has the desk send each video's state, as the app does to
// its window, and keeps what it sends. What it returns gives the state
// sent last once the news of every stamp of the video's jobs is out and
// the sends it made due have gone, with the seq of the newest of that
// news. It waits for news of every stamp, so it is not for a test that
// stamps a job with no news, as shutDown in jobs.go does: that would
// wait until go test's -timeout.
// whole is a state sent made whole with the clips of the one sent before
// it, the way the window makes it, see frontend/src/lib/sent.ts.
func whole(was, sent VideoState) VideoState {
	had := map[string]ClipEntry{}
	for _, c := range append(was.Clips, sent.Clips...) {
		had[c.Key] = c
	}
	sent.Clips = []ClipEntry{}
	for _, m := range sent.Order {
		sent.Clips = append(sent.Clips, had[m.Key])
	}
	sent.Order = nil
	return sent
}

func heardStates(d *desk) func(path string) (VideoState, uint64) {
	var mu sync.Mutex
	out := sync.NewCond(&mu)
	last := map[string]VideoState{}
	told := map[string]uint64{}
	svc := d.svc
	svc.news = newVideoNews(func(path string) (VideoState, error) {
		return svc.State(context.Background(), path)
	}, func(v VideoState) {
		mu.Lock()
		defer mu.Unlock()
		last[v.Video] = whole(last[v.Video], v)
	})
	svc.jobs.news = svc.told
	emit := svc.jobs.emit
	svc.jobs.emit = func(u JobUpdate) {
		emit(u)
		mu.Lock()
		told[u.Job.Episode] = max(told[u.Job.Episode], u.Job.Seq)
		mu.Unlock()
		out.Broadcast()
	}
	return func(path string) (VideoState, uint64) {
		// The queue wakes what waits on its jobs as it stamps them, before
		// their news is handed on, so the news is waited for here.
		var stamped uint64
		svc.jobs.mu.Lock()
		for _, j := range svc.jobs.jobs {
			if j.Episode == path {
				stamped = max(stamped, j.Seq)
			}
		}
		svc.jobs.mu.Unlock()
		mu.Lock()
		for told[path] < stamped {
			out.Wait()
		}
		mu.Unlock()
		sendDue(svc.news)
		mu.Lock()
		defer mu.Unlock()
		return last[path], told[path]
	}
}

// What changes a video sends its state, and the state sent last holds the
// change: the clips a search wrote, a clip removed, and the undo of it.
// The state sent after an edit is newer than the edit's answer, so the
// window, which takes no state older than an answer, takes it.
//
// On the real disk: the video is a copy on the disk, and its clips are
// fitted to the picture ffprobe reads.
func TestPathWhatChangesAVideoSendsItsState(t *testing.T) {
	d := open(t)
	sent := heardStates(d)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	found, news := sent(ep)
	clips := d.clips(ep)
	if len(clips) == 0 || len(found.Clips) != len(clips) {
		t.Fatalf("the state sent last holds %d clips, the search wrote %d", len(found.Clips), len(clips))
	}
	if found.Rev <= news {
		t.Errorf("the state sent last is at %d, read before the search's last news at %d", found.Rev, news)
	}
	// The removal comes long after the last state was sent, as it does
	// for a person, so the state it tells is read at once, beside the
	// answer.
	d.svc.news.mu.Lock()
	d.svc.news.due[ep].last = time.Time{}
	d.svc.news.mu.Unlock()
	first := clips[0]
	removed, err := d.svc.RemoveClip(context.Background(), ep, first.Plan, first.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := sent(ep)
	if after.Rev <= removed.Rev {
		t.Errorf("the state sent after the removal is at %d, its answer at %d", after.Rev, removed.Rev)
	}
	if i := slices.IndexFunc(after.Clips, func(c ClipEntry) bool { return c.Key == first.Key }); i < 0 || !after.Clips[i].Rejected {
		t.Errorf("the state sent after the removal does not hold the clip removed")
	}
	if done, err := d.svc.Undo(ep); err != nil || !done.Done {
		t.Fatalf("the removal was not undone: %v", err)
	}
	undone, _ := sent(ep)
	if undone.Rev <= after.Rev {
		t.Errorf("no state was sent after the undo")
	}
	if i := slices.IndexFunc(undone.Clips, func(c ClipEntry) bool { return c.Key == first.Key }); i < 0 || undone.Clips[i].Rejected {
		t.Errorf("the state sent after the undo does not hold the clip back")
	}
}

// A video whose file cannot be read now still has its state, its status
// and its clips, so the workspace still says what it can.
//
// On the real disk: ffprobe reads the video, and then the bytes that took
// its place.
func TestPathAVideoThatCannotBeReadStillHasItsState(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	clips := d.clips(ep)
	if err := os.WriteFile(ep, []byte("not a video any more"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := d.svc.State(context.Background(), ep)
	if err != nil {
		t.Fatalf("a video that cannot be read has no state: %v", err)
	}
	if st.Status.Source != ep || len(st.Clips) != len(clips) || len(st.Jobs) == 0 {
		t.Errorf("the state of a video that cannot be read is of %q with %d clips of %d and %d jobs", st.Status.Source, len(st.Clips), len(clips), len(st.Jobs))
	}
}

// A state holds the jobs queued or running, those that stopped before
// they were done and still say so, and of the rest the newest of each
// kind, not every job the video had since the app opened. The state is
// the one way a video's jobs reach the window, so a clip made by hand
// that stopped keeps its card while a later one is made.
func TestAStateHoldsTheJobsTheWorkspaceShows(t *testing.T) {
	q := newIdleQueue(nil, nil, nil)
	add := func(id, episode, kind, state string) {
		q.mu.Lock()
		defer q.mu.Unlock()
		j := &Job{ID: id, Episode: episode, JobRecord: engine.JobRecord{Kind: kind}, State: state}
		q.stampLocked(j)
		q.jobs = append(q.jobs, j)
	}
	for i := range 50 {
		add(fmt.Sprintf("render-%d", i), "a.mp4", engine.JobRender, JobDone)
	}
	add("search-old", "a.mp4", engine.JobSearch, JobSettled)
	add("clip-stopped", "a.mp4", engine.JobClip, JobStopped)
	add("search-new", "a.mp4", engine.JobSearch, JobDone)
	add("clip-new", "a.mp4", engine.JobClip, JobDone)
	add("render-running", "a.mp4", engine.JobRender, JobRunning)
	add("render-queued", "a.mp4", engine.JobRender, JobQueued)
	add("other", "b.mp4", engine.JobSearch, JobRunning)
	jobs, _ := q.ofVideo("a.mp4")
	ids := []string{}
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	if want := []string{"render-49", "clip-stopped", "search-new", "clip-new", "render-running", "render-queued"}; !slices.Equal(ids, want) {
		t.Errorf("the state holds %v, not %v", ids, want)
	}
}

// A video's state says how each search that is over or stopped ended,
// from the video's status, so the interface works nothing out: a search
// that found nothing, one that stopped before its window was heard, and
// one stopped once it was. A running search and a render say nothing.
func TestAStateSaysHowEachSearchEnded(t *testing.T) {
	search := func(id, state string, written int) Job {
		return Job{ID: id, State: state, Written: written, JobRecord: engine.JobRecord{Kind: engine.JobSearch, From: 600, To: 1200, Min: 20}}
	}
	jobs := []Job{
		search("found", JobDone, 2),
		search("nothing", JobDone, 0),
		search("stopped", JobStopped, 0),
		search("running", JobRunning, 0),
		{ID: "render", State: JobDone, JobRecord: engine.JobRecord{Kind: engine.JobRender}},
	}
	searchesEnded(jobs, engine.EpisodeStatus{Heard: [][2]float64{{0, 900}}}, 3600)
	got := map[string]string{}
	for _, j := range jobs {
		got[j.ID] = j.Ended
	}
	want := map[string]string{"found": engine.EndedFound, "nothing": engine.EndedNothing, "stopped": engine.EndedHearing, "running": "", "render": ""}
	if !maps.Equal(got, want) {
		t.Errorf("the searches ended %v, want %v", got, want)
	}
	transcribed := []Job{search("stopped", JobStopped, 0)}
	searchesEnded(transcribed, engine.EpisodeStatus{Transcribed: true}, 3600)
	if transcribed[0].Ended != engine.EndedFinding {
		t.Errorf("a search stopped on a video heard to its end ended %q", transcribed[0].Ended)
	}
}

// A state is sent with the clips the window was not sent as they are
// now, and every clip by its key and its hash in order, so a video whose
// work goes on without changing its clips sends a small state. A window
// that lost track asks for every clip again.
func TestAStateIsSentWithOnlyTheClipsThatChanged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clips := []ClipEntry{
			{ClipView: engine.ClipView{ID: "a", Start: 10}, Key: "p.json/a"},
			{ClipView: engine.ClipView{ID: "b", Start: 40}, Key: "p.json/b"},
		}
		var mu sync.Mutex
		var sent []VideoState
		news := newVideoNews(func(path string) (VideoState, error) {
			mu.Lock()
			defer mu.Unlock()
			return VideoState{Video: path, Clips: slices.Clone(clips)}, nil
		}, func(v VideoState) {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, v)
		})
		t.Cleanup(news.stop)
		// Each send, by the keys of the clips it holds and of its order.
		send := func(change func()) (holds, order []string) {
			mu.Lock()
			if change != nil {
				change()
			}
			mu.Unlock()
			news.changed("v.mp4")
			time.Sleep(time.Second) //nolint:forbidigo // synctest bubble
			synctest.Wait()
			mu.Lock()
			defer mu.Unlock()
			last := sent[len(sent)-1]
			for _, c := range last.Clips {
				if c.Hash == "" {
					t.Errorf("the clip %s was sent with no hash", c.Key)
				}
				holds = append(holds, c.Key)
			}
			for _, m := range last.Order {
				order = append(order, m.Key)
			}
			return holds, order
		}
		steps := []struct {
			name   string
			change func()
			holds  []string
		}{
			{"the first state", nil, []string{"p.json/a", "p.json/b"}},
			{"nothing about the clips changed", nil, nil},
			{"one clip trimmed", func() { clips[1].End = 70 }, []string{"p.json/b"}},
			{"a clip written before them", func() {
				clips = append([]ClipEntry{{ClipView: engine.ClipView{ID: "c", Start: 1}, Key: "p.json/c"}}, clips...)
			}, []string{"p.json/c"}},
			{"a clip removed", func() { clips = clips[:2] }, nil},
			{"the window asks for every clip", func() { news.sendWhole("v.mp4") }, []string{"p.json/c", "p.json/a"}},
		}
		for _, step := range steps {
			holds, order := send(step.change)
			mu.Lock()
			want := []string{}
			for _, c := range clips {
				want = append(want, c.Key)
			}
			mu.Unlock()
			if !slices.Equal(holds, step.holds) || !slices.Equal(order, want) {
				t.Errorf("%s: the state holds %v in the order %v, want %v in the order %v", step.name, holds, order, step.holds, want)
			}
		}
	})
}
