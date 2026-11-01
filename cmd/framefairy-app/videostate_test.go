package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
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
// On the real disk: ffmpeg makes the video, and its clips are fitted to
// the picture ffprobe reads.
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
