package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
)

// VideoState is what the workspace shows of one video, read at one
// moment and sent whole, issue #606: its status, its clips, how often
// each part of it was searched and its jobs, with the clips each job has
// on the way. The window keeps the one with the largest Rev, see
// EpisodeSession in frontend/src/lib/session.svelte.ts, so it shows what
// the Go side says instead of working it out from pings.
type VideoState struct {
	Video string `json:"video"`
	// Rev is taken from the clock of the jobs, Job.Seq, as the jobs are
	// read, and the rest is read after them. So a state whose Rev is
	// larger than a job's Seq was read after that job's news was sent,
	// and holds every clip the job had said it wrote by then. An edit
	// takes a number from the same clock once it is written, see
	// ClipEntry.Rev, so a state read before the edit is never taken
	// over its answer.
	Rev      uint64               `json:"rev"`
	Status   engine.EpisodeStatus `json:"status"`
	Clips    []ClipEntry          `json:"clips"`
	Coverage CoverageView         `json:"coverage"`
	Jobs     []Job                `json:"jobs"`
}

// State reads the state of a video as it is now, see VideoState. It is
// what the workspace opens with and what it reads again after its own
// changes, and what is sent to it as the video's work goes on, see
// videoNews.
func (s *FrameFairy) State(ctx context.Context, path string) (VideoState, error) {
	if !s.store.Known(path) {
		return VideoState{}, errNotInLibrary
	}
	jobs, rev := s.jobs.ofVideo(path)
	st := VideoState{Video: path, Rev: rev, Jobs: jobs, Status: s.Episode(path), Coverage: CoverageView{Passes: []PassView{}}}
	plans := st.Status.Plans
	if st.Status.Missing {
		// A video whose file is gone keeps its clips, see Clips.
		plans = plansOf(s.store.files, path)
	}
	info := sources.lastOf(path)
	if !st.Status.Missing {
		// A video that cannot be read now still has its status, its jobs
		// and its clips, as a missing one does, so the workspace says what
		// it can. Only how often each part was searched needs its length.
		if probed, err := s.probe(ctx, path); err != nil {
			theLog.line(zerolog.WarnLevel, "app").Str("video", filepath.Base(path)).Err(err).Msg("the video could not be read for its state")
		} else {
			info = probed
			st.Coverage = coverageOf(plans, info.Duration)
		}
	}
	st.Clips = s.clipsOf(plans, info)
	return st, nil
}

// stamp takes the next number of the clock the jobs and the states of
// the videos share, see VideoState.Rev. It does nothing without a queue,
// which is what a test that builds the service by hand may have.
func (s *FrameFairy) stamp() uint64 {
	if s.jobs == nil {
		return 0
	}
	return s.jobs.stamp()
}

// told says a video's state changed, so it is sent to the window.
func (s *FrameFairy) told(path string) {
	s.news.changed(path)
}

// jobMoved says a job changed its video's state. The state that says a
// job is over goes out at once: the window shows the end with it, and
// what follows from the end, a clip chosen, the window moved on, a toast,
// see JobStore.apply in frontend/src/lib/state.svelte.ts. Held to the end
// of the wait, all of it came up to a quarter second after the work was
// over.
func (s *FrameFairy) jobMoved(path string, over bool) {
	if over {
		s.news.now(path)
		return
	}
	s.news.changed(path)
}

// How often a video's state is sent at most while its work goes on.
const newsEvery = 250 * time.Millisecond

// videoNews sends the state of a video to the window each time it
// changed: at most four times a second for each video, and always once
// more after the last change, so what the window has in the end is what
// is there. A change while a send is due is in that send, because the
// state is read when it is sent, not when it changed. Like every method
// here it does nothing on a nil videoNews, which is what a test that
// builds the service by hand has.
type videoNews struct {
	read func(path string) (VideoState, error)
	send func(VideoState)

	mu   sync.Mutex
	due  map[string]*telling
	shut bool
}

// telling is when a video's state was last sent, and the send due.
type telling struct {
	last  time.Time
	timer *time.Timer
	// sends is the sends asked for that have not ended, the one due last,
	// each closed once it has ended, so what waits on the news can tell
	// that it is all out, see sendDue in bridge_test.go.
	sends []chan struct{}
	// sending is held while the state is read and sent, so the states of
	// a video go out one at a time, in the order they were read. A read
	// that takes longer than the wait, on a slow machine, had the next one
	// begin beside it, and the newer state could go out first. A channel
	// of one, so a send that waits on it is one a synctest bubble sees
	// waiting.
	sending chan struct{}
}

func newVideoNews(read func(string) (VideoState, error), send func(VideoState)) *videoNews {
	return &videoNews{read: read, send: quietly(send), due: map[string]*telling{}}
}

// changed says the state of the video at path changed.
func (n *videoNews) changed(path string) { n.ask(path, false) }

// now says the state of the video at path changed, and sends it at once,
// the wait or not.
func (n *videoNews) now(path string) { n.ask(path, true) }

func (n *videoNews) ask(path string, now bool) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.shut {
		return
	}
	t := n.due[path]
	if t == nil {
		t = &telling{sending: make(chan struct{}, 1)}
		n.due[path] = t
	}
	if t.timer != nil {
		// The send due goes out at once instead. One that has begun reads
		// the state after this change, see tell.
		if now && t.timer.Stop() {
			t.timer.Reset(0)
		}
		return
	}
	wait := max(0, time.Until(t.last.Add(newsEvery)))
	if now {
		wait = 0
	}
	sent := make(chan struct{})
	t.sends = append(t.sends, sent)
	t.timer = time.AfterFunc(wait, func() {
		n.tell(path)
		n.ended(t, sent)
	})
}

// ended says a send has ended.
func (n *videoNews) ended(t *telling, sent chan struct{}) {
	n.mu.Lock()
	t.sends = slices.DeleteFunc(t.sends, func(c chan struct{}) bool { return c == sent })
	n.mu.Unlock()
	close(sent)
}

// tell reads the state of the video and sends it. A change from the
// moment it begins is due again, after the wait.
func (n *videoNews) tell(path string) {
	n.mu.Lock()
	t := n.due[path]
	if n.shut || t == nil {
		n.mu.Unlock()
		return
	}
	t.timer = nil
	t.last = time.Now()
	n.mu.Unlock()
	t.sending <- struct{}{}
	defer func() { <-t.sending }()
	// The app may have quit while the send before this one went out.
	n.mu.Lock()
	shut := n.shut
	n.mu.Unlock()
	if shut {
		return
	}
	st, err := n.read(path)
	if errors.Is(err, errNotInLibrary) {
		// The video left the library, and there is nobody to tell.
		n.mu.Lock()
		if t.timer == nil {
			delete(n.due, path)
		}
		n.mu.Unlock()
		return
	}
	if err != nil {
		theLog.line(zerolog.WarnLevel, "app").Str("video", filepath.Base(path)).Err(err).Msg("the state of the video could not be read")
		return
	}
	n.send(st)
}

// stop sends nothing more, as the app quits, and returns once a send
// under way has ended. A state read before the stop is never sent after
// it: in a walk, the app opened again numbers its states from the start,
// and a late state of the app that closed would be newer than all of
// them, see VideoState.Rev.
func (n *videoNews) stop() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.shut = true
	var going []chan struct{}
	for _, t := range n.due {
		if t.timer != nil && t.timer.Stop() {
			// The send not begun yet is the one asked for last.
			t.timer = nil
			close(t.sends[len(t.sends)-1])
			t.sends = t.sends[:len(t.sends)-1]
		}
		going = append(going, t.sends...)
	}
	n.mu.Unlock()
	for _, sent := range going {
		<-sent
	}
}
