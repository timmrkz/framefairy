package main

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"path/filepath"
	"slices"
	"strconv"
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
	// Window is the video's window, with its limits, see WindowView, or
	// nil while the video cannot be read.
	Window *WindowView `json:"window"`
	// Chosen is the key of the clip chosen, by a hand or by the app, see
	// choose.go, or nothing. It is read after the clips, and chosen from
	// them and the jobs, so a clip the app chose comes with the state that
	// holds it.
	Chosen string `json:"chosen"`
	// ChosenCard is the card the clip chosen was while it was on its way,
	// when the app chose the clip because that card was written as it,
	// rule 7, so a play of the card goes on as a play of the clip.
	ChosenCard string `json:"chosenCard,omitempty"`
	// Order is, in a state sent to the window, every clip of the video in
	// the order of Clips in a state read, by its key and its hash. Clips
	// then holds only the clips the window was not sent as they are now,
	// see videoNews.trim, so a state of a video whose clips did not change
	// stays small. A state read by a call holds every clip and no Order.
	// See docs/decisions/2026-11-13-a-state-sends-only-the-clips-that-changed.md.
	Order []ClipMark `json:"order,omitempty"`
	// Kept says, in a state sent to the window, that its clips are those
	// of the state sent before, all of them as they were and in the same
	// order, so it holds neither clips nor Order: a state that named
	// every clip went over the 8 KB Wails sends inline at about 95 clips.
	Kept bool `json:"kept,omitempty"`
}

// ClipMark is a clip in a state sent to the window, by its key and its
// hash, see VideoState.Order.
type ClipMark struct {
	Key  string `json:"key"`
	Hash string `json:"hash"`
}

// State reads the state of a video as it is now, see VideoState. It is
// what the workspace opens with and what it reads again after its own
// changes, and what is sent to it as the video's work goes on, see
// videoNews. Reading it is also where the app chooses a clip from what
// it read, so a read can change the clip chosen and write it to
// chosen.json, see chosenFor, and where it starts hearing what of that
// clip is not heard yet, so a read can queue a transcription, see
// hearChosen.
func (s *FrameFairy) State(ctx context.Context, path string) (VideoState, error) {
	if !s.store.Known(path) {
		return VideoState{}, errNotInLibrary
	}
	// The transcript first. The status says what it heard and the window
	// is fitted to its lines, and both take it from this one read, where
	// each read the files for itself before, #800.
	_, _ = s.heardIn(path)
	jobs, rev := s.jobs.ofVideo(path)
	st := VideoState{Video: path, Rev: rev, Jobs: jobs, Status: s.Episode(path), Coverage: CoverageView{Passes: []PassView{}}}
	plans := st.Status.Plans
	if st.Status.Missing {
		// A video whose file is gone keeps its clips, see Clips.
		plans = s.plansOf(path)
	}
	info := s.probes.lastOf(path)
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
	st.Clips = s.clipsOf(plans, info, st.Status.Heard)
	searchesEnded(st.Jobs, st.Status, info.Duration)
	if !st.Status.Missing {
		st.Window = s.windowFor(ctx, path, st, info.Duration)
	}
	st.Chosen, st.ChosenCard = s.chosenFor(path, st)
	s.hearChosen(path, st, info.Duration)
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
	// sent is the hash of each clip as the window was last sent it, by
	// its key, see trim. Only the send that holds sending reads or
	// changes it.
	sent map[string]string
	// order is the Order of the state sent last, nil until a state was.
	order []ClipMark
	// whole says the next state goes with every clip, because the window
	// lost track of what it was sent, see SendWhole.
	whole bool
	// sending is held while the state is read and sent, so the states of
	// a video go out one at a time, in the order they were read. Two
	// sends of one video at once, one begun while the other still read,
	// could end the other way round, and the older state went out last.
	// A send whose timer has fired waits on it before it takes anything,
	// so its timer stays set while it waits, and the changes that come
	// meanwhile join it: one send runs and one waits, however slow a read
	// is. A channel of one, so a send that waits on it is one a synctest
	// bubble sees waiting.
	sending chan struct{}
}

func newVideoNews(read func(string) (VideoState, error), send func(VideoState)) *videoNews {
	return &videoNews{read: read, send: quietly(send), due: map[string]*telling{}}
}

// changed says the state of the video at path changed.
func (n *videoNews) changed(path string) {
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
		return
	}
	wait := max(0, time.Until(t.last.Add(newsEvery)))
	sent := make(chan struct{})
	t.sends = append(t.sends, sent)
	t.timer = time.AfterFunc(wait, func() {
		n.tell(path, t)
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

// tell reads the state of the video and sends it, once the send before it
// has ended. A change from the moment it begins is due again, after the
// wait.
func (n *videoNews) tell(path string, t *telling) {
	t.sending <- struct{}{}
	defer func() { <-t.sending }()
	n.mu.Lock()
	// The app may have quit while the send before this one went out.
	if n.shut {
		n.mu.Unlock()
		return
	}
	t.timer = nil
	t.last = time.Now()
	if t.whole {
		t.whole = false
		t.sent, t.order = nil, nil
	}
	n.mu.Unlock()
	st, err := n.read(path)
	if errors.Is(err, errNotInLibrary) {
		// The video left the library, and there is nobody to tell.
		n.mu.Lock()
		if t.timer == nil && n.due[path] == t {
			delete(n.due, path)
		}
		n.mu.Unlock()
		return
	}
	if err != nil {
		theLog.line(zerolog.WarnLevel, "app").Str("video", filepath.Base(path)).Err(err).Msg("the state of the video could not be read")
		return
	}
	n.send(t.trim(st))
}

// trim leaves out of a state the clips the window was last sent as they
// are now, and says every clip in Order, see VideoState.Order, or, when
// not one clip changed, leaves out the clips and the order both, see
// VideoState.Kept. A video's
// state is sent about four times a second while its work goes on, and
// whole it was a kilobyte for every clip, though the clips seldom
// change: the window then fetched every one of them over a request of
// its own, which Wails makes for an event over 8 KB.
func (t *telling) trim(st VideoState) VideoState {
	sent := make(map[string]string, len(st.Clips))
	order := make([]ClipMark, 0, len(st.Clips))
	changed := []ClipEntry{}
	for _, c := range st.Clips {
		c.Hash = ""
		h := clipHash(c)
		sent[c.Key] = h
		order = append(order, ClipMark{Key: c.Key, Hash: h})
		if t.sent[c.Key] != h {
			c.Hash = h
			changed = append(changed, c)
		}
	}
	was := t.order
	t.sent, t.order = sent, order
	if was != nil && len(changed) == 0 && slices.Equal(was, order) {
		st.Clips, st.Kept = []ClipEntry{}, true
		return st
	}
	st.Clips, st.Order = changed, order
	return st
}

// clipHash is what a clip holds, as a short text.
func clipHash(c ClipEntry) string {
	h := fnv.New64a()
	if err := json.NewEncoder(h).Encode(c); err != nil {
		// Nothing in a clip fails to marshal. Should it, the clip is
		// sent every time.
		return ""
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// sendWhole says the next state of the video goes with every clip, and
// that it is due.
func (n *videoNews) sendWhole(path string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	if t := n.due[path]; t != nil {
		t.whole = true
	}
	n.mu.Unlock()
	n.changed(path)
}

// SendWhole sends the state of a video with every clip, for a window
// that was sent a clip by its key alone and does not have it, after it
// was opened again, see VideoState.Order.
func (s *FrameFairy) SendWhole(path string) error {
	if !s.store.Known(path) {
		return errNotInLibrary
	}
	theLog.line(zerolog.InfoLevel, "app").Str("video", filepath.Base(path)).Msg("the window asked for the state of the video with every clip")
	s.news.sendWhole(path)
	return nil
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

// searchesEnded says of each search among jobs that is done or stopped
// how it ended, from the video's status and its length, see
// engine.SearchEnded.
func searchesEnded(jobs []Job, status engine.EpisodeStatus, duration float64) {
	heard := status.Heard
	if status.Transcribed {
		heard = [][2]float64{{0, duration}}
	}
	for i := range jobs {
		j := &jobs[i]
		if j.Kind != engine.JobSearch || (j.State != JobDone && !j.unfinished()) {
			continue
		}
		j.Ended = engine.SearchEnded(j.JobRecord, j.State == JobDone, j.Written+j.Carried, heard, duration)
	}
}
