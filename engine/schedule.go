package engine

import (
	"container/list"
	"context"
	"errors"
	"sync"

	"framefairy/internal/framewire"
	"framefairy/internal/wake"
)

// Schedule decides which frames of a video the decoder makes for the
// video preview while it stands still: a click, a drag of the playhead, a
// step with the arrow keys. The window says where the hand is and what it
// does there, Hand, and draws the frame it is sent. Which frame comes
// next, what the decoder gives up on and what is kept is decided here,
// where the cursors and their cost can be seen, see
// docs/decisions/2026-11-19-go-schedules-the-video-previews-frames.md.
//
// The newest place wins. A look for a place the hand has left is answered
// with ErrOvertaken the moment a newer one comes, and the decoder stops
// working for it between two frames, see EpisodeFrames.Stream, unless it
// is the first frame of a hand that moves, which comes first, so the
// picture follows a drag faster than its frames can be decoded. While the
// hand moves, the key frame the decoder starts from is sent on the way to
// the frame under it, and once it rests, the frame under it.
type Schedule struct {
	frames framesSource

	mu sync.Mutex
	// The newest look of the hand that has not been answered, and the
	// looks that follow it, which a newer look of the hand does not
	// overtake while the decoder is on its way to them, see Follow.
	waiting *lookWaiter
	follows []*lookWaiter
	// The decoder's work for the hand: one stream at a time.
	run *lookRun
	// Where the hand was last, to tell a slow step back, see startFor.
	last place
	kept keptFrames
	// looks wakes whoever waits whenever a look starts to wait, so a test
	// goes on once the schedule has it. Only the tests wait on it.
	looks wake.Wake
}

// framesSource is where a schedule's frames come from, the episode's
// decoder in the app, see EpisodeFrames.Stream. A test hands in its own,
// which makes frames when the test says.
type framesSource interface {
	Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
		skip func() float64, got func(at float64, frame []byte) error) error
}

// NewSchedule is the schedule of the video whose decoder this is.
func NewSchedule(f *EpisodeFrames) *Schedule {
	return newSchedule(f)
}

func newSchedule(frames framesSource) *Schedule {
	return &Schedule{frames: frames, last: place{frame: -1}, kept: newKeptFrames(lookKeptBytes)}
}

// Hand is where the hand is: a frame of the grid, at a size, and what it
// does there.
type Hand struct {
	Grid  framewire.Grid
	Frame int
	W, H  int
	// Moving says the hand moves over the place, a drag of the playhead
	// past its first place. A look that does not move rests: a click, a
	// step, a drag that let go.
	Moving bool
	// Follow says the frame is one of those after the place the hand
	// rests on, which a play from there shows first. It overtakes no
	// look, and is answered once the decoder has it.
	Follow bool
	// Shown is the frame on screen, or -1, so a frame on the way is sent
	// only where it is nearer the place than that one.
	Shown int
}

// Seen is a frame sent for a look: which frame of the grid, where it
// starts, its picture, see framewire.Picture, which nobody may change,
// and whether it is the frame of the place, or one on the way to it.
type Seen struct {
	Frame   int
	At      float64
	Picture []byte
	Exact   bool
}

// ErrOvertaken answers a look that a newer look took the place of. Its
// window has moved on and draws nothing for it.
var ErrOvertaken = errors.New("a newer look took this one's place")

// How a schedule reads around the place the hand rests on, in frames of
// the grid: the frames after it, so a step to the right finds its frame
// kept, and on a slow step back, the frames before it, so the next steps
// back find theirs. A hand that went back no further than lookSlow frames
// since its last place goes slowly. Further, it has left those frames
// behind before they would be shown, and a far step is quicker with its
// key frame first. Issue #596.
const (
	lookAhead = 4
	lookBack  = 15
	lookSlow  = 2
)

// lookKeptBytes is how much of the frames a schedule decoded it keeps,
// the ones used last first: about 40 frames at 1512 by 850, a big
// window's video preview.
const lookKeptBytes = 192 << 20

type lookWaiter struct {
	look Hand
	// back says the hand went back slowly to this place, see startFor.
	back   bool
	answer chan lookAnswer
}

type lookAnswer struct {
	seen Seen
	err  error
}

// lookRun is one stream of the decoder for the hand: from a frame on, the
// way to its target and a few frames past it.
type lookRun struct {
	grid     framewire.Grid
	w, h     int
	from     int
	target   int
	moving   bool
	next     int
	gave     bool
	stop     context.CancelFunc
	finished bool
}

// covers tells whether the run will hand over the frame of the look: on
// its grid at its size, not passed, and no further than a few frames past
// its target, which it reads on to by itself.
func (r *lookRun) covers(l Hand) bool {
	return r.grid == l.Grid && r.w == l.W && r.h == l.H && l.Frame >= r.next && l.Frame <= r.target+lookAhead
}

// Look answers where the hand is with a frame: the frame of the place, or
// while the hand moves, one on the way to it that is nearer than the
// frame on screen. A newer look of the hand answers it with ErrOvertaken.
func (s *Schedule) Look(ctx context.Context, l Hand) (Seen, error) {
	key := keptKey{grid: l.Grid, w: l.W, h: l.H, frame: l.Frame}
	w := &lookWaiter{look: l, answer: make(chan lookAnswer, 1)}
	s.mu.Lock()
	if !l.Follow {
		s.overtake(l)
		last := s.last
		w.back = l.Moving && last.grid == l.Grid && last.w == l.W && last.h == l.H && last.frame > l.Frame && last.frame-l.Frame <= lookSlow
		s.last = place{grid: l.Grid, w: l.W, h: l.H, frame: l.Frame}
	}
	if k, ok := s.kept.get(key); ok {
		if r := s.run; !l.Follow && r != nil && !r.covers(l) {
			s.stopRun()
		}
		s.mu.Unlock()
		return Seen{Frame: l.Frame, At: k.at, Picture: k.picture, Exact: true}, nil
	}
	r := s.run
	switch {
	case l.Follow:
		// The run for the place reads on to the frames after it, see got.
		s.follows = append(s.follows, w)
		reaches := r != nil && r.grid == l.Grid && r.w == l.W && r.h == l.H && l.Frame >= r.next
		if !reaches && (r == nil || s.waiting == nil) {
			s.stopRun()
			s.startFor(w)
		}
	case r != nil && r.covers(l):
		s.waiting = w
		r.target = max(r.target, l.Frame)
	case r != nil && !r.gave && r.moving && l.Moving:
		// The decoder's first frame for a place the hand passed comes
		// first, and the run for this place starts after it, see got. A
		// drag over the clip timeline would otherwise start a seek for
		// every place it passes, and stop each before its first frame,
		// so nothing would be shown until the hand rested.
		s.waiting = w
	default:
		s.waiting = w
		s.stopRun()
		s.startFor(w)
	}
	s.looks.WakeLocked()
	s.mu.Unlock()
	select {
	case a := <-w.answer:
		return a.seen, a.err
	case <-ctx.Done():
		s.mu.Lock()
		s.drop(w)
		s.mu.Unlock()
		return Seen{}, ctx.Err()
	}
}

// Close stops the decoder's work for the hand and answers every look that
// waits with ErrOvertaken: the video preview has let go of the video.
func (s *Schedule) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w := s.waiting; w != nil {
		w.answer <- lookAnswer{err: ErrOvertaken}
	}
	for _, w := range s.follows {
		w.answer <- lookAnswer{err: ErrOvertaken}
	}
	s.waiting, s.follows = nil, nil
	s.stopRun()
	s.kept = newKeptFrames(lookKeptBytes)
}

// overtake answers the look of the hand that waits with ErrOvertaken, and
// the looks that follow it where the newer look is for another place.
// Under the lock.
func (s *Schedule) overtake(l Hand) {
	if w := s.waiting; w != nil {
		s.waiting = nil
		w.answer <- lookAnswer{err: ErrOvertaken}
	}
	kept := s.follows[:0]
	for _, w := range s.follows {
		if w.look.Grid == l.Grid && w.look.W == l.W && w.look.H == l.H && w.look.Frame > l.Frame && w.look.Frame <= l.Frame+lookAhead {
			kept = append(kept, w)
			continue
		}
		w.answer <- lookAnswer{err: ErrOvertaken}
	}
	s.follows = kept
}

// drop takes a look whose window gave up on it out of those waiting.
// Under the lock.
func (s *Schedule) drop(w *lookWaiter) {
	if s.waiting == w {
		s.waiting = nil
	}
	for i, f := range s.follows {
		if f == w {
			s.follows = append(s.follows[:i], s.follows[i+1:]...)
			break
		}
	}
}

// stopRun stops the decoder's work for the hand between two frames. Under
// the lock.
func (s *Schedule) stopRun() {
	if s.run != nil {
		s.run.stop()
		s.run = nil
	}
}

// startFor starts the decoder's work for a look. A hand that moves has
// the key frame first, but on a slow step back, where the frame on screen
// is nearer than the key frame: that run starts lookBack frames before the
// place, and every frame from there is kept. A hand that rests has the
// frame before its place too, which the arrow to the left asks for next.
// Under the lock.
func (s *Schedule) startFor(w *lookWaiter) {
	l := w.look
	from, key := l.Frame, l.Moving
	switch {
	case l.Follow:
	case w.back:
		from, key = max(0, l.Frame-lookBack), false
	case !l.Moving:
		from = max(0, l.Frame-1)
	}
	ctx, stop := context.WithCancel(context.Background())
	r := &lookRun{grid: l.Grid, w: l.W, h: l.H, from: from, target: l.Frame, moving: l.Moving, next: from, stop: stop}
	s.run = r
	go func() {
		err := s.frames.Stream(ctx, r.grid, r.grid.At(from), r.w, r.h, key, nil, func(at float64, frame []byte) error {
			return s.got(r, at, frame)
		})
		s.ended(r, ctx, err)
	}()
}

// errLookDone ends a run that has no more to do: it reached its target and
// the frames after it, or the hand went elsewhere.
var errLookDone = errors.New("the look has its frames")

// got takes a frame the decoder made for a run: kept, and sent to the
// looks it answers. The frame of a place is sent to its look. The first
// frame of a run, the key frame the decoder starts from, is sent on the
// way to a hand that moves, where it is nearer the place than the frame
// on screen.
func (s *Schedule) got(r *lookRun, at float64, picture []byte) error {
	frame := r.grid.Holding(at)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kept.put(keptKey{grid: r.grid, w: r.w, h: r.h, frame: frame}, at, picture)
	first := !r.gave
	r.gave = true
	r.next = max(r.next, frame+1)
	if s.run != r {
		return errLookDone
	}
	seen := Seen{Frame: frame, At: at, Picture: picture}
	kept := s.follows[:0]
	for _, w := range s.follows {
		if w.look.Frame == frame && w.look.Grid == r.grid && w.look.W == r.w && w.look.H == r.h {
			exact := seen
			exact.Exact = true
			w.answer <- lookAnswer{seen: exact}
			continue
		}
		kept = append(kept, w)
	}
	s.follows = kept
	w := s.waiting
	if w != nil && w.look.Grid == r.grid && w.look.W == r.w && w.look.H == r.h {
		l := w.look
		seen.Exact = frame == l.Frame
		if seen.Exact || first && l.Moving && nearerThanShown(frame, l) {
			s.waiting = nil
			w.answer <- lookAnswer{seen: seen}
		}
	}
	if w != nil && !seen.Exact && !r.covers(w.look) {
		// A place the hand moved on to while this run brought its first
		// frame: it goes next, and its look asked again finds it on its
		// way.
		s.run = nil
		s.startFor(w)
		return errLookDone
	}
	last := r.target
	for _, w := range s.follows {
		last = max(last, w.look.Frame)
	}
	if frame >= last+lookAhead || frame >= last && r.moving {
		s.run = nil
		return errLookDone
	}
	return nil
}

// nearerThanShown tells whether a frame on the way to the place of a look is nearer
// it than the frame on screen.
func nearerThanShown(frame int, l Hand) bool {
	if l.Shown < 0 {
		return true
	}
	return framesApart(frame, l.Frame) < framesApart(l.Shown, l.Frame)
}

func framesApart(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// ended takes the end of a run's stream: a look still waiting for what
// the run never brought gets a run of its own, or why the decoder failed.
func (s *Schedule) ended(r *lookRun, ctx context.Context, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.finished = true
	if s.run == r {
		s.run = nil
	}
	if s.run != nil || ctx.Err() != nil && !errors.Is(err, errLookDone) {
		return
	}
	failed := err != nil && !errors.Is(err, errLookDone)
	if !failed && r.next < r.grid.Count {
		// Stopped by itself with no more to do, or waiting for a look
		// that came after.
		s.restart()
		return
	}
	why := err
	if why == nil {
		why = errors.New("the video ended before the frame asked for")
	}
	if w := s.waiting; w != nil && r.covers(w.look) {
		s.waiting = nil
		w.answer <- lookAnswer{err: why}
	}
	kept := s.follows[:0]
	for _, w := range s.follows {
		if r.covers(w.look) {
			w.answer <- lookAnswer{err: why}
			continue
		}
		kept = append(kept, w)
	}
	s.follows = kept
	s.restart()
}

// restart starts a run for what still waits, the hand first. Under the
// lock.
func (s *Schedule) restart() {
	if s.run != nil {
		return
	}
	if w := s.waiting; w != nil {
		s.startFor(w)
		return
	}
	if len(s.follows) > 0 {
		s.startFor(s.follows[0])
	}
}

// keptKey is a frame of a grid at a size.
type keptKey struct {
	grid  framewire.Grid
	w, h  int
	frame int
}

type keptFrame struct {
	key     keptKey
	at      float64
	picture []byte
}

// keptFrames holds the frames decoded last, up to most bytes, the frame
// used longest ago let go of first. A frame kept is never changed: the
// buffer the decoder handed over is the schedule's for good, and nobody
// gives it back for a later frame.
type keptFrames struct {
	most  int
	bytes int
	order list.List
	at    map[keptKey]*list.Element
}

func newKeptFrames(most int) keptFrames {
	return keptFrames{most: most, at: map[keptKey]*list.Element{}}
}

func (k *keptFrames) get(key keptKey) (keptFrame, bool) {
	e, ok := k.at[key]
	if !ok {
		return keptFrame{}, false
	}
	k.order.MoveToFront(e)
	return e.Value.(keptFrame), true
}

func (k *keptFrames) put(key keptKey, at float64, picture []byte) {
	if e, ok := k.at[key]; ok {
		k.bytes -= len(e.Value.(keptFrame).picture)
		k.order.Remove(e)
	}
	k.at[key] = k.order.PushFront(keptFrame{key: key, at: at, picture: picture})
	k.bytes += len(picture)
	for k.bytes > k.most && k.order.Len() > 1 {
		e := k.order.Back()
		f := e.Value.(keptFrame)
		k.order.Remove(e)
		delete(k.at, f.key)
		k.bytes -= len(f.picture)
	}
}
