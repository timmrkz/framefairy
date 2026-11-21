package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/runlog"
	"framefairy/engine/timed"
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
	// What the state holds is what the clip list says.
	clips := d.clips(ep)
	if len(before.Clips) == 0 || len(before.Clips) != len(clips) {
		t.Fatalf("the state holds %d clips, the list %d", len(before.Clips), len(clips))
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
	if sent.Kept {
		sent.Clips, sent.Kept = was.Clips, false
		return sent
	}
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
	// The library picks every video's jobs at once the same way, so it
	// says how a video stands with the jobs its state holds.
	all := q.ofVideos()
	for _, video := range []string{"a.mp4", "b.mp4"} {
		one, _ := q.ofVideo(video)
		if !reflect.DeepEqual(all[video], one) {
			t.Errorf("the library holds other jobs of %s than its state:\n%v\n%v", video, all[video], one)
		}
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

// A video's state says where I and O can make a clip, from what of it is
// heard and said and how long it is, engine.MarkingOf, so the workspace
// keeps no rule of its own for its keys. A video whose length is not
// known says nothing, and the keys are offered. A video transcribed to
// its end counts as heard to its end, though its transcript's last part
// ends with the last word, a second short of it here: I is offered up to
// the last word, not up to the half second before that last second.
func TestAStateSaysWhereIAndOMakeAClip(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	ctx := context.Background()
	unknown, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Marking != nil {
		t.Errorf("a video that cannot be read says I and O make a clip only in %+v", *unknown.Marking)
	}
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: 31}, nil
	}
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}, {11.0, 11.4, "drei."}})
	st, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Status.Transcribed || st.Status.Said == nil {
		t.Fatalf("the video is not transcribed with words: %+v", st.Status)
	}
	said := *st.Status.Said
	if said[0] < 10 || said[0] > 10.2 || said[1] < 11.3 || said[1] > 11.5 {
		t.Fatalf("the words are said from %v", said)
	}
	if st.Marking == nil || st.Marking.InBefore != said[1] || st.Marking.OutFrom != said[0] {
		t.Errorf("the state says I and O make a clip in %+v, not before %v and from %v", st.Marking, said[1], said[0])
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
			if last.Kept {
				order = nil
			}
			return holds, order
		}
		steps := []struct {
			name   string
			change func()
			holds  []string
			kept   bool
		}{
			{"the first state", nil, []string{"p.json/a", "p.json/b"}, false},
			{"nothing about the clips changed, so neither clips nor order", nil, nil, true},
			{"one clip trimmed", func() { clips[1].End = 70 }, []string{"p.json/b"}, false},
			{"a clip written before them", func() {
				clips = append([]ClipEntry{{ClipView: engine.ClipView{ID: "c", Start: 1}, Key: "p.json/c"}}, clips...)
			}, []string{"p.json/c"}, false},
			{"a clip removed", func() { clips = clips[:2] }, nil, false},
			{"the window asks for every clip", func() { news.sendWhole("v.mp4") }, []string{"p.json/c", "p.json/a"}, false},
			{"nothing changed again", nil, nil, true},
		}
		for _, step := range steps {
			holds, order := send(step.change)
			mu.Lock()
			var want []string
			if !step.kept {
				for _, c := range clips {
					want = append(want, c.Key)
				}
			}
			mu.Unlock()
			if !slices.Equal(holds, step.holds) || !slices.Equal(order, want) {
				t.Errorf("%s: the state holds %v in the order %v, want %v in the order %v", step.name, holds, order, step.holds, want)
			}
		}
	})
}

// A video with no clips is sent an order with nothing in it, never no
// order, because a state with no order is one the window takes as read
// whole and does not keep: the state after it said its clips were kept,
// the window had none kept, asked for every clip and was sent no order
// again, and so every state of the video was dropped while it had no
// clip, a search removed among them.
func TestAVideoWithNoClipsIsSentAnEmptyOrder(t *testing.T) {
	keys := func(st VideoState) map[string]json.RawMessage {
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	var tl telling
	first := keys(tl.trim(VideoState{Video: "v.mp4", Clips: []ClipEntry{}}))
	if string(first["order"]) != "[]" || first["kept"] != nil {
		t.Errorf("the first state of a video with no clips has the order %s and kept %s, want [] and none", first["order"], first["kept"])
	}
	if next := keys(tl.trim(VideoState{Video: "v.mp4", Clips: []ClipEntry{}})); string(next["kept"]) != "true" {
		t.Errorf("the next state of a video with no clips has kept %s, want true", next["kept"])
	}
	if read := keys(VideoState{Video: "v.mp4", Clips: []ClipEntry{}}); read["order"] != nil {
		t.Errorf("a state read has the order %s, want none", read["order"])
	}
}

// A clip's hash, which says the window already has the clip, changes
// with every step of its crop: its frame, its edge, and which piece it
// is on, and a piece with no steps is not one with none yet. The steps
// are hashed as numbers rather than written as JSON, issue #893, and a
// window kept a clip whose glide it never got when two differed and
// hashed the same.
func TestAClipsHashChangesWithEveryStepOfItsCrop(t *testing.T) {
	clip := func(steps ...[]engine.CropStep) ClipEntry {
		return ClipEntry{ClipView: engine.ClipView{ID: "a", Start: 1}, Key: "p.json/a", CropSteps: steps}
	}
	glide := []engine.CropStep{{Frame: 30, X: 100}, {Frame: 31, X: 100.5}, {Frame: 32, X: 101.25}}
	was := clipHash(clip(glide, nil))
	if again := clipHash(clip([]engine.CropStep{{Frame: 30, X: 100}, {Frame: 31, X: 100.5}, {Frame: 32, X: 101.25}}, nil)); again != was {
		t.Errorf("the same clip hashed as %s and %s", was, again)
	}
	for _, c := range []struct {
		name string
		clip ClipEntry
	}{
		{"an edge moved a hundredth", clip([]engine.CropStep{{Frame: 30, X: 100}, {Frame: 31, X: 100.51}, {Frame: 32, X: 101.25}}, nil)},
		{"a step on another frame", clip([]engine.CropStep{{Frame: 30, X: 100}, {Frame: 31, X: 100.5}, {Frame: 33, X: 101.25}}, nil)},
		{"the last step gone", clip(glide[:2], nil)},
		{"the steps on the second piece", clip(nil, glide)},
		{"a step moved to the second piece", clip(glide[:2], glide[2:])},
		{"no steps on the second piece rather than none yet", clip(glide, []engine.CropStep{})},
		{"one piece", clip(glide)},
		{"no steps at all", clip()},
		{"the clip moved", ClipEntry{ClipView: engine.ClipView{ID: "a", Start: 2}, Key: "p.json/a", CropSteps: [][]engine.CropStep{glide, nil}}},
	} {
		if h := clipHash(c.clip); h == was {
			t.Errorf("%s: the clip hashed the same as before, %s", c.name, h)
		}
	}
	// Every other field of a clip is in its hash because it is written as
	// JSON. A step's fields are hashed by name, so a field added to a step
	// is a field clipHash has to hash too.
	var fields []string
	for f := range reflect.TypeFor[engine.CropStep]().Fields() {
		fields = append(fields, f.Name)
	}
	if !slices.Equal(fields, []string{"Frame", "X"}) {
		t.Errorf("a crop step holds %v, and clipHash hashes Frame and X of it alone", fields)
	}
}

// The transcript search reads its cards again when the stamp of them in
// the video's state changes, issue #933. A word corrected in a clip set
// outside the app, a sync service bringing it in from another Mac, keeps
// every clip's pieces, which the window once read the change from, and
// still changes the cards. So it changes the stamp, and the cards read
// then say the corrected word. A state read again with nothing changed
// keeps the stamp, so the window reads nothing.
func TestAStateSaysWhenTheCardsOfWhatWasSaidChanged(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: 31}, nil
	}
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.5, "eins"}, {20.1, 20.6, "zwei"}})
	says := func(word string) bool {
		t.Helper()
		cards, err := svc.Sentences(mine)
		if err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(cards, func(c engine.SentenceView) bool { return c.Text == word })
	}
	first, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cards == "" || !says("zwei") {
		t.Fatalf("the cards are stamped %q and say zwei: %v", first.Cards, says("zwei"))
	}
	if again, _ := svc.State(ctx, mine); again.Cards != first.Cards {
		t.Errorf("a state read again with nothing changed is stamped %q, was %q", again.Cards, first.Cards)
	}

	outside := strings.Replace(historyPlan, `"slug": "zwei",`, `"slug": "zwei", "corrections": {"20100": "zwo"},`, 1)
	if outside == historyPlan {
		t.Fatal("the clip set was not changed")
	}
	if err := afero.WriteFile(svc.store.files, plan, []byte(outside), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.files.Chtimes(plan, time.Unix(2_000_000_000, 0), time.Unix(2_000_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	after, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(after.Clips, first.Clips, func(a, b ClipEntry) bool { return reflect.DeepEqual(a.Segments, b.Segments) }) {
		t.Fatal("the pieces of a clip changed, which is not the case this is about")
	}
	if after.Cards == first.Cards {
		t.Error("a word corrected outside the app left the stamp of the cards as it was")
	}
	if !says("zwo") {
		t.Error("the cards read after the stamp changed do not say the corrected word")
	}
}

// The search that marks a video in the sidebar is its newest, and only
// when it failed, stopped or was cut off. One whose window a hand moved
// off, set aside, says nothing, the way it has no row: it once still
// marked the video, #436. One that runs says nothing here, because the
// sidebar shows the video busy from its jobs.
func TestTheNewestSearchMarksTheVideoUnlessItWasSetAside(t *testing.T) {
	search := func(state string, aside bool) Job {
		j := Job{State: state, Aside: aside}
		j.Kind = engine.JobSearch
		return j
	}
	render := Job{State: JobFailed}
	render.Kind = engine.JobRender
	for _, c := range []struct {
		name string
		jobs []Job
		want searchStanding
	}{
		{"no search", nil, searchQuiet},
		{"a failed render", []Job{render}, searchQuiet},
		{"a failed search", []Job{search(JobFailed, false)}, searchFailed},
		{"a search that stopped", []Job{search(JobStopped, false)}, searchStopped},
		{"a search cut off", []Job{search(JobInterrupted, false)}, searchStopped},
		{"a search that stopped, set aside", []Job{search(JobStopped, true)}, searchQuiet},
		{"a failed search, set aside", []Job{search(JobFailed, true)}, searchQuiet},
		{"a search that runs", []Job{search(JobRunning, false)}, searchQuiet},
		{"a failed search, then one done", []Job{search(JobFailed, false), search(JobDone, false)}, searchQuiet},
		{"one done, then one that stopped", []Job{search(JobDone, false), search(JobStopped, false)}, searchStopped},
	} {
		if got := searchStands(c.jobs); got != c.want {
			t.Errorf("%s: the search stands as %d, want %d", c.name, got, c.want)
		}
	}
}

// The library and a video's state say how the video stands with its
// search, the one the queue holds: a search that stopped makes a video
// with no clips one that needs a look, in the sidebar's first read and
// in every state sent after.
func TestTheLibraryAndTheStateSayHowAVideoStandsWithItsSearch(t *testing.T) {
	s, eps, _ := stoppedSearches(t, "stopped.mp4")
	lib := s.Library()
	if len(lib) != 1 || lib[0].Stands != standsLook {
		t.Fatalf("the library says the video with a search that stopped stands as %+v", lib)
	}
	st, err := s.State(context.Background(), eps[0])
	if err != nil {
		t.Fatal(err)
	}
	if st.Status.Stands != standsLook {
		t.Errorf("the video's state says it stands as %q, want %q", st.Status.Stands, standsLook)
	}
}

// How a video stands, decided here and drawn by the sidebar: a file gone
// or a failed search first, then what needs a look, then clips made.
func TestAVideoStandsAsItsFilesAndSearchSay(t *testing.T) {
	plans := []engine.PlanSummary{{}}
	for _, c := range []struct {
		name   string
		st     engine.EpisodeStatus
		search searchStanding
		want   string
	}{
		{"a new video", engine.EpisodeStatus{}, searchQuiet, ""},
		{"a video with clips", engine.EpisodeStatus{Plans: plans}, searchQuiet, standsMade},
		{"a file gone, with clips", engine.EpisodeStatus{Missing: true, Plans: plans}, searchQuiet, standsFailed},
		{"a failed search, with clips", engine.EpisodeStatus{Plans: plans}, searchFailed, standsFailed},
		{"a search that stopped, with clips", engine.EpisodeStatus{Plans: plans}, searchStopped, standsLook},
		{"a file changed since it was transcribed", engine.EpisodeStatus{TranscriptStale: true, Plans: plans}, searchQuiet, standsLook},
		{"a file gone with a search that stopped", engine.EpisodeStatus{Missing: true}, searchStopped, standsFailed},
		{"a failed search on a changed file", engine.EpisodeStatus{TranscriptStale: true}, searchFailed, standsFailed},
	} {
		if got := standing(c.st, c.search); got != c.want {
			t.Errorf("%s stands as %q, want %q", c.name, got, c.want)
		}
	}
}

// The range picker frames the part Transcribe whole video works on, and
// the Go side says which: the part its hearing has reached while it
// hears, the first part not heard while it waits its turn, and none
// while it does not run or was told to stop.
func TestTheStateSaysWhichPartTheWholeVideoHears(t *testing.T) {
	t.Parallel()
	unheard := [][2]float64{{600, 900}, {1200, 1800}, {2400, 3600}}
	whole := func(state, step string, covered float64) Job {
		j := Job{JobRecord: engine.TranscribeRecord(timed.Window{}, false), State: state}
		j.Step = step
		if covered > 0 {
			j.Progress = &runlog.Event{Kind: runlog.EventProgress, From: covered - 30, Covered: covered}
		}
		return j
	}
	part := Job{JobRecord: engine.TranscribeRecord(timed.Window{Start: 1200, End: 1260}, false), State: JobRunning}
	part.Step = engine.StepHearing
	stopping := whole(JobRunning, engine.StepHearing, 1200.2)
	stopping.Stopping = true
	for _, c := range []struct {
		name    string
		jobs    []Job
		unheard [][2]float64
		want    *[2]float64
	}{
		{"nothing runs", nil, unheard, nil},
		{"only a part of a clip is heard", []Job{part}, unheard, nil},
		{"waits its turn", []Job{whole(JobQueued, "", 0)}, unheard, &[2]float64{600, 900}},
		{"waits behind a search", []Job{whole(JobRunning, engine.StepWaiting, 1200.2)}, unheard, &[2]float64{600, 900}},
		{"hears the second part", []Job{part, whole(JobRunning, engine.StepHearing, 1200.2)}, unheard, &[2]float64{1200, 1800}},
		{"hears where no part begins", []Job{whole(JobRunning, engine.StepHearing, 300)}, unheard, &[2]float64{600, 900}},
		{"told to stop", []Job{stopping}, unheard, nil},
		{"everything is heard", []Job{whole(JobRunning, engine.StepHearing, 3600)}, nil, nil},
	} {
		got := wholePart(c.jobs, c.unheard)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: the part framed is %v, want %v", c.name, got, c.want)
		}
	}
}
