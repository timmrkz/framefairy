package engine

import (
	"container/list"
	"context"
	"errors"
	"slices"
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
	run  *lookRun
	kept keptFrames
	// What to read while the hand rests, see Ahead.
	ahead *aheadWork
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
	return &Schedule{frames: frames, kept: newKeptFrames(lookKeptBytes)}
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
	// only where it is nearer the place than that one, and a hand that
	// moves a frame or two back from it goes back slowly, see startFor.
	Shown int
}

// Seen is a frame sent for a look: which frame of the grid, where it
// starts, its picture, see framewire.Picture, which nobody may change,
// whether it is the frame of the place, or one on the way to it, and
// whether it was kept, so nothing was decoded for it.
type Seen struct {
	Frame   int
	At      float64
	Picture []byte
	Exact   bool
	Kept    bool
}

// ErrOvertaken answers a look that a newer look took the place of. Its
// window has moved on and draws nothing for it.
var ErrOvertaken = errors.New("a newer look took this one's place")

// How a schedule reads around the place the hand rests on, in frames of
// the grid: the frames after it, so a step to the right finds its frame
// kept, and on a slow step back, the frames before it, so the next steps
// back find theirs. A hand that went back no further than lookSlow frames
// from the frame on screen goes slowly. Further, it has left those frames
// behind before they would be shown, and a far step is quicker with its
// key frame first. Issue #596.
const (
	lookAhead = 4
	lookBack  = 15
	lookSlow  = 2
)

// lookKeptBytes is how much of the frames a schedule decoded it keeps,
// the ones used last first: about 40 frames at 1512 by 850, a big
// window's video preview. lookCardBytes is how much of the first frames
// of the clips on the cards are read ahead for the window to keep: about
// 78 clips at that size, the likeliest cards first, see Ahead.
const (
	lookKeptBytes = 192 << 20
	lookCardBytes = 400 << 20
)

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
	// ahead says the run reads a frame ahead of the hand, see Ahead, until
	// a look of the hand finds its frame on the run's way and takes it.
	ahead bool
	// done is closed once the run's stream has ended, see Near.
	done chan struct{}
	// passing says the newest look the run answers passes over the frames
	// around its place, see passes: the run holds at its target, see hold.
	passing bool
	// ctx is the run's, and wake, while it holds, lets it read on.
	ctx  context.Context
	wake chan struct{}
	// behind says the run reads the frames before a hand that steps back
	// slowly, see readBehind. It reads ahead of the hand, the way ahead
	// does, but for the hand, not for an Ahead.
	behind bool
}

// passes tells whether a hand moves more than lookSlow frames at a time,
// and so passes over the frames around each place: a fast drag. The frames
// after its place would only keep the decoder from the next place, and the
// frames around it would only be sent to the window and let go of again.
func passes(l Hand) bool {
	return l.Moving && (l.Shown < 0 || absInt(l.Shown-l.Frame) > lookSlow)
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
	if v := s.waiting; v != nil && !l.Follow && v.look.Grid == l.Grid && v.look.W == l.W && v.look.H == l.H && v.look.Frame == l.Frame {
		// The same place asked twice, the frame under the hand and the
		// first of the play cued there: both get it, the way a frame after
		// the place does.
		l.Follow = true
		w.look = l
	}
	if !l.Follow {
		s.overtake(l)
	}
	w.back = l.Moving && l.Shown > l.Frame && l.Shown-l.Frame <= lookSlow
	if k, ok := s.kept.get(key); ok {
		if r := s.run; !l.Follow && r != nil && (!r.ahead || r.behind && !l.Moving) && !r.covers(l) && !r.after(l) {
			s.stopRun()
		}
		if r := s.run; r != nil && !r.ahead && r.after(l) {
			r.target = max(r.target, l.Frame)
			r.readOn(l)
		}
		if w.back {
			s.readBehind(l)
		}
		// A hand that let go may let the reading ahead start.
		s.restart()
		s.mu.Unlock()
		return Seen{Frame: l.Frame, At: k.at, Picture: k.picture, Exact: true, Kept: true}, nil
	}
	r := s.run
	switch {
	case l.Follow:
		// The run for the place reads on to the frames after it, see got.
		s.follows = append(s.follows, w)
		reaches := r != nil && !r.ahead && r.grid == l.Grid && r.w == l.W && r.h == l.H && l.Frame >= r.next
		if reaches {
			r.readOn(l)
		}
		if !reaches && (r == nil || s.waiting == nil) {
			s.stopRun()
			s.startFor(w)
		}
	case r != nil && r.covers(l) && (!r.ahead || l.Frame <= r.target):
		s.waiting = w
		r.target = max(r.target, l.Frame)
		r.ahead, r.behind = false, false
		r.readOn(l)
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
	if a := s.ahead; a != nil {
		s.ahead = nil
		a.done <- aheadDone{err: ErrOvertaken}
	}
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
	r := s.run
	if r == nil {
		return
	}
	r.stop()
	s.run = nil
	if a := s.ahead; r.ahead && !r.behind && a != nil && r.next <= r.target && a.grid == r.grid && a.w == r.w && a.h == r.h {
		// The hand came first: the frames are read once it rests again.
		a.todo = append([]aheadSpan{{from: r.next, to: r.target}}, a.todo...)
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
	r := &lookRun{grid: l.Grid, w: l.W, h: l.H, from: from, target: l.Frame, moving: l.Moving, next: from, stop: stop, done: make(chan struct{}), passing: passes(l), ctx: ctx}
	s.run = r
	var skip func() float64
	if key {
		// Past its key frame a run with the key frame first wants the frames
		// from its target on, so the ones between are decoded and not made
		// into pictures: a click far along the clip timeline, a hand
		// gripping the playhead, made a picture of every frame from the key
		// frame on.
		skip = func() float64 {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !r.gave {
				return 0
			}
			return r.grid.At(r.target)
		}
	}
	go func() {
		err := s.frames.Stream(ctx, r.grid, r.grid.At(from), r.w, r.h, key, skip, func(at float64, frame []byte) error {
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
	if a := s.ahead; a != nil {
		a.take(r.grid, r.w, r.h, frame, at, picture)
	}
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
	if r.ahead {
		if frame >= r.target {
			s.run = nil
			return errLookDone
		}
		return nil
	}
	last := r.target
	for _, w := range s.follows {
		last = max(last, w.look.Frame)
	}
	if frame >= last && r.passing && frame < last+lookAhead {
		return s.hold(r)
	}
	if frame >= last+lookAhead {
		s.run = nil
		return errLookDone
	}
	return nil
}

// hold keeps a run of a hand that passes over the frames around its
// place where it is, at the frame after the place, with nothing decoded
// and nothing made into a picture: the hand rests there once it lets go,
// and a play cued there wants the frames after it, which the run then
// reads on to with no second seek. A look elsewhere stops it, one the
// run covers or the hand resting on its place lets it read on, see
// readOn. Under the lock, which it lets go of while it holds.
func (s *Schedule) hold(r *lookRun) error {
	wake := make(chan struct{})
	r.wake = wake
	s.mu.Unlock()
	select {
	case <-wake:
	case <-r.ctx.Done():
	}
	s.mu.Lock()
	if s.run != r || r.ctx.Err() != nil {
		return errLookDone
	}
	return nil
}

// readOn lets a run that holds read on, for a look that wants the frames
// after its place. Under the lock.
func (r *lookRun) readOn(l Hand) {
	r.passing = passes(l)
	if r.wake != nil && !r.passing {
		close(r.wake)
		r.wake = nil
	}
}

// after tells whether the run stands right after the place of a look, the
// way a run that holds does: the frames it reads next are the ones after
// the place.
func (r *lookRun) after(l Hand) bool {
	return r.grid == l.Grid && r.w == l.W && r.h == l.H && r.next == l.Frame+1
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
	close(r.done)
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
		return
	}
	s.readAhead()
}

// Ahead is what the hand is likely to ask for next while it rests, read
// whenever the decoder has nothing to do for the hand and handed to the
// window to keep, so a step there costs no request: the first frames of
// the words either side of the playhead, which Shift and an arrow step
// to, with the frame before each and lookAhead after, which the arrows
// step to from there, and then the first frames of the clips on the
// cards, the likeliest to be clicked first, as many as lookCardBytes
// holds. Each in the order given. Tim asked for every clip on the cards
// to be ready to show, issue #383.
type Ahead struct {
	Grid   framewire.Grid
	W, H   int
	Frames []int
	Cards  []int
	// Has are the frames the window holds at this size already, which are
	// neither read nor handed over again.
	Has []int
	// Held says the hand holds the playhead: nothing is read until a newer
	// Ahead says it let go. A card's seek stood between the hand and the
	// frame under it, issue #596.
	Held bool
}

type aheadWork struct {
	grid framewire.Grid
	w, h int
	// todo is where each read starts and ends, in the order given.
	todo []aheadSpan
	// wants are the frames to hand over, and out the ones read or kept
	// so far, in the order they came.
	wants map[int]bool
	out   []Seen
	held  bool
	done  chan aheadDone
}

type aheadSpan struct{ from, to int }

type aheadDone struct {
	out []Seen
	err error
}

// take hands a frame over to the window, once, where it is one it wants.
// Under the lock.
func (a *aheadWork) take(grid framewire.Grid, w, h, frame int, at float64, picture []byte) {
	if grid != a.grid || w != a.w || h != a.h || !a.wants[frame] {
		return
	}
	delete(a.wants, frame)
	a.out = append(a.out, Seen{Frame: frame, At: at, Picture: picture, Exact: true, Kept: true})
}

// Ahead reads the frames of a while the hand rests and returns them once
// they are all read, the ones kept already with them. A newer Ahead takes
// this one's place and answers it with ErrOvertaken, an empty one too,
// which stops the reading: a play's frames come from the same decoder. A
// held one returns at once with nothing.
func (s *Schedule) Ahead(ctx context.Context, a Ahead) ([]Seen, error) {
	work := &aheadWork{grid: a.Grid, w: a.W, h: a.H, held: a.Held, wants: map[int]bool{}, done: make(chan aheadDone, 1)}
	has := make(map[int]bool, len(a.Has))
	for _, f := range a.Has {
		has[f] = true
	}
	inGrid := func(f int) bool { return f >= 0 && f < a.Grid.Count }
	for _, f := range a.Frames {
		read := aheadSpan{from: -1}
		for g := f - 1; g <= f+lookAhead; g++ {
			if !inGrid(g) || has[g] || work.wants[g] {
				continue
			}
			work.wants[g] = true
			if read.from < 0 {
				read.from = g
			}
			read.to = g
		}
		if read.from >= 0 {
			work.todo = append(work.todo, read)
		}
	}
	room := lookCardBytes
	for _, f := range a.Cards {
		if size := a.W * a.H * 4; size > room {
			break
		} else {
			room -= size
		}
		if !inGrid(f) || has[f] || work.wants[f] {
			continue
		}
		work.wants[f] = true
		work.todo = append(work.todo, aheadSpan{from: f, to: f})
	}
	s.mu.Lock()
	if old := s.ahead; old != nil {
		s.ahead = nil
		old.done <- aheadDone{err: ErrOvertaken}
	}
	if r := s.run; r != nil && r.ahead && !r.behind {
		s.stopRun()
	}
	for f := range work.wants {
		if k, ok := s.kept.peek(keptKey{grid: a.Grid, w: a.W, h: a.H, frame: f}); ok {
			work.take(a.Grid, a.W, a.H, f, k.at, k.picture)
		}
	}
	slices.SortFunc(work.out, func(x, y Seen) int { return x.Frame - y.Frame })
	s.ahead = work
	s.restart()
	s.looks.WakeLocked()
	s.mu.Unlock()
	if a.Held {
		// Kept for when the hand lets go, which a newer Ahead says. No
		// request waits on it meanwhile.
		return nil, nil
	}
	select {
	case d := <-work.done:
		return d.out, d.err
	case <-ctx.Done():
		s.mu.Lock()
		if s.ahead == work {
			s.ahead = nil
			if r := s.run; r != nil && r.ahead && !r.behind {
				s.stopRun()
			}
		}
		s.mu.Unlock()
		return nil, ctx.Err()
	}
}

// readBehind reads the frames before a hand that steps back slowly
// while it is still among the frames read before it, so it does not wait
// at the end of them. A step back is read from lookBack frames before it,
// with no key frame first, see startFor. Once fewer than lookBack frames
// before the hand are kept, the lookBack frames before those are read,
// with nothing waiting for them. The decoder has nothing to do while the
// hand finds its frames kept. A look of the hand that covers the run takes
// it over, and one that lets go stops it. Under the lock.
func (s *Schedule) readBehind(l Hand) {
	if s.run != nil {
		return
	}
	kept := func(f int) bool { return s.kept.has(keptKey{grid: l.Grid, w: l.W, h: l.H, frame: f}) }
	b := l.Frame
	for b > 0 && l.Frame-b < lookBack && kept(b-1) {
		b--
	}
	if b == 0 || l.Frame-b >= lookBack {
		return
	}
	from := max(0, b-lookBack)
	ctx, stop := context.WithCancel(context.Background())
	r := &lookRun{grid: l.Grid, w: l.W, h: l.H, from: from, target: b - 1, next: from, stop: stop, ahead: true, behind: true, done: make(chan struct{})}
	s.run = r
	go func() {
		err := s.frames.Stream(ctx, r.grid, r.grid.At(from), r.w, r.h, false, nil, func(at float64, picture []byte) error {
			return s.got(r, at, picture)
		})
		s.ended(r, ctx, err)
	}()
}

// readAhead starts the decoder on the next frame to read ahead, once it
// has nothing to do for the hand and the hand does not hold the playhead,
// or says the reading is done. Under the lock.
func (s *Schedule) readAhead() {
	a := s.ahead
	if a == nil || a.held || s.run != nil || s.waiting != nil || len(s.follows) > 0 {
		return
	}
	for len(a.todo) > 0 {
		read := a.todo[0]
		a.todo = a.todo[1:]
		// The frames of the read the window still wants, from the first.
		for read.from <= read.to && !a.wants[read.from] {
			read.from++
		}
		if read.from > read.to {
			continue
		}
		ctx, stop := context.WithCancel(context.Background())
		r := &lookRun{grid: a.grid, w: a.w, h: a.h, from: read.from, target: read.to, next: read.from, stop: stop, ahead: true, done: make(chan struct{})}
		s.run = r
		go func() {
			err := s.frames.Stream(ctx, r.grid, r.grid.At(r.from), r.w, r.h, false, nil, func(at float64, picture []byte) error {
				return s.got(r, at, picture)
			})
			s.ended(r, ctx, err)
		}()
		return
	}
	s.ahead = nil
	a.done <- aheadDone{out: a.out}
}

// Near hands over the frames kept around a place the hand rests on, the
// ones the run for it read on the way and past it, once that run has
// ended: lookBack frames before it and lookAhead after, and of those only
// the ones has says the window does not hold. The window keeps them, so
// the arrow keys and a slow drag find their frames with no request, see
// app.ts. A newer look of the hand ends the run, and Near with it.
func (s *Schedule) Near(ctx context.Context, l Hand, has func(frame int) bool) []Seen {
	from, to := max(0, l.Frame-lookBack), min(l.Grid.Count-1, l.Frame+lookAhead)
	s.mu.Lock()
	if r := s.run; r != nil && !r.ahead && r.grid == l.Grid && r.w == l.W && r.h == l.H && r.target >= from && r.target <= to {
		s.mu.Unlock()
		select {
		case <-r.done:
		case <-ctx.Done():
			return nil
		}
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	var near []Seen
	for f := from; f <= to; f++ {
		if f == l.Frame || has(f) {
			continue
		}
		if k, ok := s.kept.peek(keptKey{grid: l.Grid, w: l.W, h: l.H, frame: f}); ok {
			near = append(near, Seen{Frame: f, At: k.at, Picture: k.picture, Exact: true, Kept: true})
		}
	}
	return near
}

// HasNear tells whether Near would hand over frames around the place of
// a look the window had answered: the run for the place still reads, or
// frames around it are kept. A hand that passes over them has none, see
// passes.
func (s *Schedule) HasNear(l Hand) bool {
	if passes(l) {
		return false
	}
	from, to := max(0, l.Frame-lookBack), min(l.Grid.Count-1, l.Frame+lookAhead)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.run; r != nil && !r.ahead && r.grid == l.Grid && r.w == l.W && r.h == l.H && r.target >= from && r.target <= to {
		return true
	}
	for f := from; f <= to; f++ {
		if f != l.Frame && s.kept.has(keptKey{grid: l.Grid, w: l.W, h: l.H, frame: f}) {
			return true
		}
	}
	return false
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
	k.remove(key)
	k.at[key] = k.order.PushFront(keptFrame{key: key, at: at, picture: picture})
	k.bytes += len(picture)
	k.fit()
}

func (k *keptFrames) remove(key keptKey) {
	if e, ok := k.at[key]; ok {
		k.bytes -= len(e.Value.(keptFrame).picture)
		k.order.Remove(e)
		delete(k.at, key)
	}
}

// fit lets the frames used longest ago go until the rest fit.
func (k *keptFrames) fit() {
	for k.bytes > k.most && k.order.Len() > 1 {
		e := k.order.Back()
		f := e.Value.(keptFrame)
		k.order.Remove(e)
		delete(k.at, f.key)
		k.bytes -= len(f.picture)
	}
}

// peek is get without making the frame the one used last.
func (k *keptFrames) peek(key keptKey) (keptFrame, bool) {
	if e, ok := k.at[key]; ok {
		return e.Value.(keptFrame), true
	}
	return keptFrame{}, false
}

func (k *keptFrames) has(key keptKey) bool {
	_, ok := k.at[key]
	return ok
}
