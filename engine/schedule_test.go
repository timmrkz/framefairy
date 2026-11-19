package engine

import (
	"context"
	"errors"
	"slices"
	"testing"

	"framefairy/internal/framewire"
)

// The schedule is a function of the places the hand gives it and of what
// the decoder answers, so these tests give it places and make the
// decoder's frames themselves, one at a time, and count what was decoded
// and what was sent. A stream of the decoder stands still until the test
// gives it a frame, so no test waits on the clock.

// heldDecoder is a decoder whose streams hand over a frame when the test
// gives one, and say so once the schedule has taken it.
type heldDecoder struct {
	opened chan *heldStream
}

type heldStream struct {
	from   int
	key    bool
	skip   func() float64
	ctx    context.Context
	frames chan heldFrame
	ended  chan error
}

type heldFrame struct {
	frame int
	took  chan error
	end   error
}

func newHeldDecoder() *heldDecoder {
	return &heldDecoder{opened: make(chan *heldStream, 16)}
}

func (d *heldDecoder) Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
	skip func() float64, got func(at float64, frame []byte) error) error {
	s := &heldStream{from: grid.Holding(from), key: key, skip: skip, ctx: ctx, frames: make(chan heldFrame), ended: make(chan error, 1)}
	d.opened <- s
	for {
		select {
		case <-ctx.Done():
			s.ended <- ctx.Err()
			return ctx.Err()
		case f := <-s.frames:
			if f.end != nil {
				s.ended <- f.end
				f.took <- f.end
				return f.end
			}
			err := got(grid.At(f.frame), make([]byte, width*height*4))
			f.took <- err
			if err != nil {
				s.ended <- err
				return err
			}
		}
	}
}

// give hands over a frame and says what the schedule made of it: nil to
// go on, or why the stream ends.
func (s *heldStream) give(t *testing.T, frame int) error {
	t.Helper()
	took := make(chan error, 1)
	select {
	case s.frames <- heldFrame{frame: frame, took: took}:
	case <-s.ctx.Done():
		t.Fatalf("frame %d was given to the stream from %d, which was stopped", frame, s.from)
	}
	return <-took
}

// next is the stream the schedule opens next.
func (d *heldDecoder) next(t *testing.T) *heldStream {
	t.Helper()
	return <-d.opened
}

// none says no stream was opened since the last one taken.
func (d *heldDecoder) none(t *testing.T) {
	t.Helper()
	select {
	case s := <-d.opened:
		t.Fatalf("a stream from %d was opened", s.from)
	default:
	}
}

// stopped says the stream was stopped, by the schedule or by itself.
func (s *heldStream) stopped(t *testing.T) {
	t.Helper()
	<-s.ctx.Done()
}

// until goes on once ok says yes, asked again each time a look starts to
// wait, never on the clock. ok is asked with the schedule's lock held.
func until(s *Schedule, ok func() bool) {
	for {
		s.mu.Lock()
		if ok() {
			s.mu.Unlock()
			return
		}
		changed := s.looks.WaitLocked()
		s.mu.Unlock()
		<-changed
	}
}

// waits goes on once the hand's look for h waits on the schedule.
func waits(s *Schedule, h Hand) {
	until(s, func() bool { return s.waiting != nil && s.waiting.look == h })
}

var lookGrid = framewire.Grid{Num: 25, Den: 1, Count: 500}

type looked struct {
	seen Seen
	err  error
}

// lookFor asks the schedule for a place, answered on the channel.
func lookFor(s *Schedule, h Hand) chan looked {
	out := make(chan looked, 1)
	go func() {
		seen, err := s.Look(context.Background(), h)
		out <- looked{seen, err}
	}()
	return out
}

func rest(frame int) Hand { return Hand{Grid: lookGrid, Frame: frame, W: 4, H: 2, Shown: -1} }
func move(frame, shown int) Hand {
	return Hand{Grid: lookGrid, Frame: frame, W: 4, H: 2, Moving: true, Shown: shown}
}

// The arrow keys after a click find their frames kept: the frame of the
// click, the one before it, and a few after it, from one stream.
func TestAClickBringsItsFrameAndTheFramesBesideIt(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, rest(48))
	st := dec.next(t)
	if st.from != 47 || st.key {
		t.Fatalf("a click opened a stream from %d, key first %v, want from the frame before it without the key frame", st.from, st.key)
	}
	for _, f := range []int{47, 48} {
		if err := st.give(t, f); err != nil {
			t.Fatalf("frame %d ended the stream: %v", f, err)
		}
	}
	got := <-click
	if got.err != nil || got.seen.Frame != 48 || !got.seen.Exact {
		t.Fatalf("the click got %+v, %v", got.seen, got.err)
	}
	for f := 49; f < 48+lookAhead; f++ {
		if err := st.give(t, f); err != nil {
			t.Fatalf("frame %d ended the stream: %v", f, err)
		}
	}
	if err := st.give(t, 48+lookAhead); !errors.Is(err, errLookDone) {
		t.Fatalf("the stream went on past the frames after the click: %v", err)
	}
	for _, f := range []int{47, 49, 48 + lookAhead} {
		got := <-lookFor(s, rest(f))
		if got.err != nil || got.seen.Frame != f || !got.seen.Exact {
			t.Fatalf("a step to %d got %+v, %v", f, got.seen, got.err)
		}
	}
	dec.none(t)
}

// A click far from the frame on screen has its own frame read first, and
// the frame before it once the frames after it are read. Read from the
// frame before, the frame clicked came a frame later, and the frame
// before can lie before the place's key frame, a whole seek further back.
func TestAFarClickBringsItsOwnFrameFirst(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	far := rest(300)
	far.Shown = 10
	click := lookFor(s, far)
	st := dec.next(t)
	if st.from != 300 || st.key {
		t.Fatalf("a far click opened a stream from %d, key first %v, want from its own frame", st.from, st.key)
	}
	_ = st.give(t, 300)
	if got := <-click; got.err != nil || got.seen.Frame != 300 || !got.seen.Exact {
		t.Fatalf("the click got %+v, %v", got.seen, got.err)
	}
	for f := 301; f < 300+lookAhead; f++ {
		_ = st.give(t, f)
	}
	if err := st.give(t, 300+lookAhead); !errors.Is(err, errLookDone) {
		t.Fatalf("the stream went on past the frames after the click: %v", err)
	}
	before := dec.next(t)
	if before.from != 299 {
		t.Fatalf("the frame before the click was read from %d", before.from)
	}
	if err := before.give(t, 299); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading of the frame before went on: %v", err)
	}
	until(s, func() bool { return s.run == nil })
	if got := <-lookFor(s, rest(299)); !got.seen.Kept {
		t.Fatalf("a step back after the click got %+v, %v, want it kept", got.seen, got.err)
	}
	dec.none(t)
}

// A click on another place before the frame came stops the decoder's work
// for the first at once, and the first click is answered as overtaken.
func TestTheNewestPlaceWins(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	first := lookFor(s, rest(48))
	old := dec.next(t)
	second := lookFor(s, rest(200))
	if got := <-first; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the first click got %+v, %v, want it overtaken", got.seen, got.err)
	}
	old.stopped(t)
	st := dec.next(t)
	if st.from != 199 {
		t.Fatalf("the second click opened a stream from %d", st.from)
	}
	_ = st.give(t, 199)
	_ = st.give(t, 200)
	if got := <-second; got.err != nil || got.seen.Frame != 200 || !got.seen.Exact {
		t.Fatalf("the second click got %+v, %v", got.seen, got.err)
	}
}

// A drag faster than the frames under it can be decoded: each place the
// hand passed waits for the first frame of the place before, the key frame
// the decoder starts from, which is sent on the way, so the picture
// follows the hand. Once the hand is at a place, the key frame and then
// the frame of the place come. Issue #281.
func TestADragShowsTheKeyFramesOnTheWay(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	press := lookFor(s, rest(10))
	st := dec.next(t)
	_ = st.give(t, 9)
	_ = st.give(t, 10)
	if got := <-press; got.seen.Frame != 10 {
		t.Fatalf("the press got %+v, %v", got.seen, got.err)
	}
	first := lookFor(s, move(120, 10))
	run := dec.next(t)
	if run.from != 120 || !run.key {
		t.Fatalf("a place the hand moved to opened a stream from %d, key first %v", run.from, run.key)
	}
	st.stopped(t)
	// Two more places before the first's key frame came: no stream for
	// them yet.
	second := lookFor(s, move(160, 10))
	waits(s, move(160, 10))
	if got := <-first; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the first place got %+v, %v", got.seen, got.err)
	}
	third := lookFor(s, move(170, 10))
	waits(s, move(170, 10))
	if got := <-second; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the second place got %+v, %v", got.seen, got.err)
	}
	dec.none(t)
	// The key frame of the first place: nearer the hand than the frame on
	// screen, so sent, and the decoder goes on to the newest place.
	if err := run.give(t, 100); !errors.Is(err, errLookDone) {
		t.Fatalf("the stream for a place left went on after its first frame: %v", err)
	}
	got := <-third
	if got.err != nil || got.seen.Frame != 100 || got.seen.Exact {
		t.Fatalf("the newest place got %+v, %v, want frame 100 on the way", got.seen, got.err)
	}
	now := dec.next(t)
	if now.from != 170 || !now.key {
		t.Fatalf("the newest place opened a stream from %d, key first %v", now.from, now.key)
	}
	// The hand stays: its key frame, then its own frame.
	again := lookFor(s, move(170, 100))
	waits(s, move(170, 100))
	if now.skip == nil || now.skip() > 0 {
		t.Fatal("the stream would skip its key frame")
	}
	_ = now.give(t, 150)
	if got := <-again; got.seen.Frame != 150 || got.seen.Exact {
		t.Fatalf("the place got %+v, %v, want its key frame", got.seen, got.err)
	}
	// Past the key frame, the frames before the place are not made into
	// pictures.
	if at := now.skip(); at != lookGrid.At(170) {
		t.Fatalf("the stream skips to %v, not to the place at %v", at, lookGrid.At(170))
	}
	// The hand went 20 frames at a time, past the frames around each
	// place, so the stream holds after the place, with nothing decoded.
	again = lookFor(s, move(170, 150))
	waits(s, move(170, 150))
	held := make(chan error, 1)
	go func() { held <- now.give(t, 170) }()
	if got := <-again; got.seen.Frame != 170 || !got.seen.Exact {
		t.Fatalf("the place got %+v, %v, want its own frame", got.seen, got.err)
	}
	until(s, func() bool { return s.run != nil && s.run.wake != nil })
	// A hand that goes on slowly from there has the stream read on, so its
	// next frames are found on the way, and a step there opens no stream.
	step := lookFor(s, move(172, 170))
	waits(s, move(172, 170))
	if err := <-held; err != nil {
		t.Fatalf("the stream that held ended with %v", err)
	}
	_ = now.give(t, 171)
	_ = now.give(t, 172)
	if got := <-step; got.seen.Frame != 172 || !got.seen.Exact {
		t.Fatalf("the slow step got %+v, %v", got.seen, got.err)
	}
	step = lookFor(s, move(173, 172))
	waits(s, move(173, 172))
	_ = now.give(t, 173)
	if got := <-step; got.seen.Frame != 173 || !got.seen.Exact {
		t.Fatalf("the step on got %+v, %v", got.seen, got.err)
	}
	for f := 174; f < 173+lookAhead; f++ {
		if err := now.give(t, f); err != nil {
			t.Fatalf("the stream stopped at frame %d, before the frames after the place: %v", f, err)
		}
	}
	// While the hand holds the playhead, the stream holds once it has read
	// the frames after the place, and the hand's next frame is read on
	// from there, with no seek. A slow drag to the right once opened a
	// stream every few frames, and some waited 190 ms for their key frame.
	parked := make(chan error, 1)
	go func() { parked <- now.give(t, 173+lookAhead) }()
	until(s, func() bool { return s.run != nil && s.run.resting })
	next := 174 + lookAhead
	step = lookFor(s, move(next, 173+lookAhead))
	waits(s, move(next, 173+lookAhead))
	if err := <-parked; err != nil {
		t.Fatalf("the stream that held after the place ended with %v", err)
	}
	_ = now.give(t, next)
	if got := <-step; got.seen.Frame != next || !got.seen.Exact {
		t.Fatalf("the step past the frames read got %+v, %v", got.seen, got.err)
	}
	for f := next + 1; f < next+lookAhead; f++ {
		if err := now.give(t, f); err != nil {
			t.Fatalf("the stream stopped at frame %d, before the frames after the place: %v", f, err)
		}
	}
	go func() { parked <- now.give(t, next+lookAhead) }()
	until(s, func() bool { return s.run != nil && s.run.resting })
	// The hand lets go, which the window says with what to read ahead: the
	// stream ends.
	if got := <-aheadFor(s, aheadOf(nil, nil)); got.err != nil {
		t.Fatalf("the Ahead as the hand let go ended with %v", got.err)
	}
	if err := <-parked; !errors.Is(err, errLookDone) {
		t.Fatalf("the stream went on after the hand let go: %v", err)
	}
	dec.none(t)
}

// A click far along the clip timeline grips the playhead, and the frames
// after its place come once the hand lets go, from the stream that found
// the place, with no second seek: a play cued there wants them.
func TestALetGoReadsOnWhereTheClickHeld(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, move(300, 10))
	st := dec.next(t)
	_ = st.give(t, 280)
	if got := <-click; got.seen.Frame != 280 {
		t.Fatalf("the click got %+v, %v, want its key frame on the way", got.seen, got.err)
	}
	click = lookFor(s, move(300, 280))
	waits(s, move(300, 280))
	held := make(chan error, 1)
	go func() { held <- st.give(t, 300) }()
	if got := <-click; got.seen.Frame != 300 || !got.seen.Exact {
		t.Fatalf("the click got %+v, %v", got.seen, got.err)
	}
	until(s, func() bool { return s.run != nil && s.run.wake != nil })
	if got := <-lookFor(s, rest(300)); !got.seen.Kept {
		t.Fatalf("letting go got %+v, %v", got.seen, got.err)
	}
	if err := <-held; err != nil {
		t.Fatalf("the stream that held ended with %v", err)
	}
	for f := 301; f < 300+lookAhead; f++ {
		if err := st.give(t, f); err != nil {
			t.Fatalf("the stream stopped at %d: %v", f, err)
		}
	}
	if err := st.give(t, 300+lookAhead); !errors.Is(err, errLookDone) {
		t.Fatalf("the stream read on past the frames after the place: %v", err)
	}
	dec.none(t)
}

// A key frame further from the place than the frame on screen is not
// sent: the picture would jump away from the hand.
func TestAKeyFrameFurtherThanTheFrameOnScreenIsNotSent(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	at := lookFor(s, move(120, 118))
	st := dec.next(t)
	_ = st.give(t, 100)
	select {
	case got := <-at:
		t.Fatalf("the place got %+v, %v while its key frame was further than the frame on screen", got.seen, got.err)
	default:
	}
	_ = st.give(t, 120)
	if got := <-at; got.seen.Frame != 120 || !got.seen.Exact {
		t.Fatalf("the place got %+v, %v", got.seen, got.err)
	}
}

// Letting go is a place that rests: it stops the decoder's work for a
// place the hand moved over at once, without waiting for its key frame,
// and asks the frame under the hand without its key frame first. A place
// parked behind a stream for a place the hand had left waited up to
// 950 ms on Tim's Mac, issue #924.
func TestLettingGoStopsTheWorkForAPlacePassed(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	passed := lookFor(s, move(120, 10))
	old := dec.next(t)
	letGo := lookFor(s, rest(300))
	if got := <-passed; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the place passed got %+v, %v", got.seen, got.err)
	}
	old.stopped(t)
	st := dec.next(t)
	if st.from != 299 || st.key {
		t.Fatalf("letting go opened a stream from %d, key first %v", st.from, st.key)
	}
	_ = st.give(t, 299)
	_ = st.give(t, 300)
	if got := <-letGo; got.seen.Frame != 300 || !got.seen.Exact {
		t.Fatalf("letting go got %+v, %v", got.seen, got.err)
	}
}

// A hand going slowly to the left: the frames before its place come with
// it, kept, so the next steps back find theirs, where each was a seek of
// its own with its key frame first. Issue #596.
func TestASlowDragBackBringsTheFramesBehindIt(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	press := lookFor(s, rest(100))
	st := dec.next(t)
	_ = st.give(t, 99)
	_ = st.give(t, 100)
	<-press
	back := lookFor(s, move(99, 100))
	if got := <-back; got.seen.Frame != 99 || !got.seen.Exact {
		t.Fatalf("a step back to the frame kept got %+v, %v", got.seen, got.err)
	}
	// The step back to 99 found its frame kept, and the frames before it
	// are read, see TestASlowDragBackReadsOnBehindTheHand: the next step
	// back finds its frame on their way.
	back = lookFor(s, move(97, 99))
	run := dec.next(t)
	if run.from != 99-lookBack || run.key {
		t.Fatalf("a slow step back opened a stream from %d, key first %v", run.from, run.key)
	}
	for f := run.from; f <= 97; f++ {
		_ = run.give(t, f)
	}
	if got := <-back; got.seen.Frame != 97 || !got.seen.Exact {
		t.Fatalf("the step back got %+v, %v", got.seen, got.err)
	}
	for f := 96; f >= run.from; f-- {
		if got := <-lookFor(s, move(f, f+1)); got.seen.Frame != f || !got.seen.Exact {
			t.Fatalf("the step back to %d got %+v, %v", f, got.seen, got.err)
		}
	}
	// The hand is at the first frame read, so the frames before it are
	// read next.
	if next := dec.next(t); next.from != run.from-lookBack {
		t.Fatalf("the frames behind the hand were read from %d", next.from)
	}
}

// A hand going on slowly to the left never waits at the end of the frames
// read behind it: once fewer than lookBack of them are left before it, the
// ones before those are read while it steps through the ones there are.
// A slow drag to the left stood still every lookBack frames, and where
// the hand let go there, the frame came 150 ms later.
func TestASlowDragBackReadsOnBehindTheHand(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	back := lookFor(s, move(97, 99))
	run := dec.next(t)
	for f := run.from; f <= 97; f++ {
		_ = run.give(t, f)
	}
	<-back
	for f := 98; f < 97+lookAhead; f++ {
		_ = run.give(t, f)
	}
	if err := run.give(t, 97+lookAhead); !errors.Is(err, errLookDone) {
		t.Fatalf("the step back's stream went on: %v", err)
	}
	until(s, func() bool { return s.run == nil })
	// The first step back finds its frame kept, and the frames before the
	// ones kept are read.
	if got := <-lookFor(s, move(96, 97)); got.seen.Frame != 96 || !got.seen.Kept {
		t.Fatalf("the step back to 96 got %+v, %v", got.seen, got.err)
	}
	behind := dec.next(t)
	if behind.from != 97-2*lookBack || behind.key {
		t.Fatalf("the frames behind the hand were read from %d, key first %v", behind.from, behind.key)
	}
	for f := behind.from; f < 97-lookBack-1; f++ {
		if err := behind.give(t, f); err != nil {
			t.Fatalf("the reading behind the hand stopped at %d: %v", f, err)
		}
	}
	if err := behind.give(t, 97-lookBack-1); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading behind the hand went on past the frames kept: %v", err)
	}
	until(s, func() bool { return s.run == nil })
	// Every step down to there finds its frame kept, and the next frames
	// behind are read once fewer than lookBack are left.
	for f := 95; f > 97-lookBack-1; f-- {
		if got := <-lookFor(s, move(f, f+1)); !got.seen.Kept {
			t.Fatalf("the step back to %d got %+v, %v", f, got.seen, got.err)
		}
	}
	dec.none(t)
	f := 97 - lookBack - 1
	if got := <-lookFor(s, move(f, f+1)); !got.seen.Kept {
		t.Fatalf("the step back to %d got %+v, %v", f, got.seen, got.err)
	}
	next := dec.next(t)
	if next.from != 97-3*lookBack {
		t.Fatalf("the next frames behind the hand were read from %d", next.from)
	}
	// The hand lets go: it goes back no more, so the reading stops.
	if got := <-lookFor(s, rest(f)); !got.seen.Kept {
		t.Fatalf("letting go got %+v, %v", got.seen, got.err)
	}
	next.stopped(t)
}

// The frames a play from the place shows first follow the frame of the
// place from the same stream, and a click elsewhere overtakes them.
func TestTheFramesAfterAPlaceComeFromItsStream(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	place := lookFor(s, rest(48))
	st := dec.next(t)
	follows := []chan looked{}
	for f := 49; f <= 48+2*lookAhead; f++ {
		h := rest(f)
		h.Follow = true
		follows = append(follows, lookFor(s, h))
		until(s, func() bool { return len(s.follows) == f-48 })
	}
	for f := 47; f <= 48+2*lookAhead; f++ {
		if err := st.give(t, f); err != nil && f < 48+2*lookAhead {
			t.Fatalf("frame %d ended the stream: %v", f, err)
		}
	}
	<-place
	for i, c := range follows {
		if got := <-c; got.err != nil || got.seen.Frame != 49+i || !got.seen.Exact {
			t.Fatalf("frame %d after the place got %+v, %v", 49+i, got.seen, got.err)
		}
	}
	dec.none(t)

	h := rest(250)
	h.Follow = true
	follow := lookFor(s, h)
	follow2 := dec.next(t)
	_ = lookFor(s, rest(10))
	if got := <-follow; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("a frame after a place left got %+v, %v", got.seen, got.err)
	}
	follow2.stopped(t)
}

// A stream that fails says why to the look that waits for it.
func TestAFailedStreamSaysWhy(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	at := lookFor(s, rest(48))
	st := dec.next(t)
	broke := errors.New("the picture could not be decoded")
	took := make(chan error, 1)
	st.frames <- heldFrame{end: broke, took: took}
	<-took
	if got := <-at; !errors.Is(got.err, broke) {
		t.Fatalf("the look got %+v, %v", got.seen, got.err)
	}
	dec.none(t)
}

// The frames kept are held to their bytes, the one used longest ago let
// go of first.
func TestTheKeptFramesLetTheOldestGo(t *testing.T) {
	k := newKeptFrames(3 * 8)
	key := func(f int) keptKey { return keptKey{grid: lookGrid, w: 2, h: 1, frame: f} }
	for f := range 3 {
		k.put(key(f), 0, make([]byte, 8))
	}
	if _, ok := k.get(key(0)); !ok {
		t.Fatal("frame 0 was let go of")
	}
	k.put(key(3), 0, make([]byte, 8))
	if _, ok := k.get(key(1)); ok {
		t.Error("frame 1, used longest ago, is still kept")
	}
	for _, f := range []int{0, 2, 3} {
		if _, ok := k.get(key(f)); !ok {
			t.Errorf("frame %d was let go of", f)
		}
	}
}

// A video whose workspace went to sleep keeps its decoder a while, and
// none of the frames it kept: three videos each kept up to lookKeptBytes.
func TestASleepingVideoKeepsNoFrames(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, rest(48))
	st := dec.next(t)
	_ = st.give(t, 47)
	_ = st.give(t, 48)
	<-click
	s.LetFramesGo()
	s.mu.Lock()
	kept := s.kept.bytes
	s.mu.Unlock()
	if kept != 0 {
		t.Fatalf("the schedule still keeps %d bytes of frames", kept)
	}
}

type readAhead struct {
	frames []int
	err    error
}

// aheadFor asks the schedule to read a ahead, answered on the channel
// with the frames handed over.
func aheadFor(s *Schedule, a Ahead) chan readAhead {
	out := make(chan readAhead, 1)
	go func() {
		seen, err := s.Ahead(context.Background(), a)
		got := readAhead{err: err}
		for _, f := range seen {
			got.frames = append(got.frames, f.Frame)
		}
		out <- got
	}()
	return out
}

func aheadOf(frames, cards []int) Ahead {
	return Ahead{Grid: lookGrid, W: 4, H: 2, Frames: frames, Cards: cards}
}

// The frames read ahead wait until the decoder has nothing to do for the
// hand, and a step or a card clicked then finds its frame kept, nothing
// decoded for it. The word beside first, then the cards. Issue #383.
func TestTheFramesAheadAreReadOnceTheHandRestsAndFoundKept(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, rest(10))
	st := dec.next(t)
	read := aheadFor(s, aheadOf([]int{50}, []int{200}))
	until(s, func() bool { return s.ahead != nil })
	dec.none(t)
	for f := 9; f <= 10+lookAhead; f++ {
		_ = st.give(t, f)
	}
	if got := <-click; got.seen.Frame != 10 || got.seen.Kept {
		t.Fatalf("the click got %+v, %v", got.seen, got.err)
	}
	// The word's frame comes with the frame before it, where the arrow to
	// the left steps, and the frames after it, where the arrow to the
	// right does.
	// No key frame first: nobody waits to be shown one.
	word := dec.next(t)
	if word.from != 49 || word.key {
		t.Fatalf("the word beside was read from %d, key first %v", word.from, word.key)
	}
	for f := 49; f < 50+lookAhead; f++ {
		if err := word.give(t, f); err != nil {
			t.Fatalf("the reading stopped at %d: %v", f, err)
		}
	}
	if err := word.give(t, 50+lookAhead); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading went on past the word's frames: %v", err)
	}
	card := dec.next(t)
	if card.from != 200 {
		t.Fatalf("the card's frame was read from %d", card.from)
	}
	_ = card.give(t, 200)
	got := <-read
	if got.err != nil {
		t.Fatalf("the reading ahead ended with %v", got.err)
	}
	if want := []int{49, 50, 51, 52, 53, 54, 200}; !slices.Equal(got.frames, want) {
		t.Fatalf("the window was handed frames %v, want %v", got.frames, want)
	}
	for _, f := range []int{50, 200} {
		if got := <-lookFor(s, rest(f)); !got.seen.Exact || !got.seen.Kept {
			t.Errorf("a step to frame %d got %+v, %v, want it kept", f, got.seen, got.err)
		}
	}
	dec.none(t)
}

// A look of the hand stops the reading ahead between two frames, and the
// frame it was reading is read once the hand rests again.
func TestTheHandStopsTheReadingAhead(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	read := aheadFor(s, aheadOf([]int{50}, nil))
	word := dec.next(t)
	_ = word.give(t, 40)
	click := lookFor(s, rest(300))
	word.stopped(t)
	st := dec.next(t)
	if st.from != 299 {
		t.Fatalf("the click opened a stream from %d", st.from)
	}
	for f := 299; f <= 300+lookAhead; f++ {
		_ = st.give(t, f)
	}
	if got := <-click; got.seen.Frame != 300 {
		t.Fatalf("the click got %+v, %v", got.seen, got.err)
	}
	again := dec.next(t)
	if again.from != 49 {
		t.Fatalf("the reading ahead went on from %d, not the word's frames", again.from)
	}
	for f := 49; f <= 50+lookAhead; f++ {
		_ = again.give(t, f)
	}
	if got := <-read; got.err != nil {
		t.Fatalf("the reading ahead ended with %v", got.err)
	}
}

// While the hand holds the playhead nothing is read ahead: a card's seek
// stood between the hand and the frame under it, issue #596. Once it lets
// go, the reading starts.
func TestNothingIsReadAheadWhileTheHandHoldsThePlayhead(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	held := aheadOf(nil, []int{200})
	held.Held = true
	if got := <-aheadFor(s, held); got.err != nil || got.frames != nil {
		t.Fatalf("the Ahead while held got %v, %v", got.frames, got.err)
	}
	dec.none(t)
	read := aheadFor(s, aheadOf(nil, []int{200}))
	card := dec.next(t)
	if card.from != 200 {
		t.Fatalf("the card's frame was read from %d", card.from)
	}
	_ = card.give(t, 200)
	if got := <-read; got.err != nil {
		t.Fatalf("the reading ahead ended with %v", got.err)
	}
}

// The window holds the frames around the place the hand rests on, the
// ones it does not have yet, once the run for the place has read them:
// a step with the arrow keys then shows its frame with no request.
func TestTheFramesAroundAPlaceGoToTheWindow(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, rest(48))
	st := dec.next(t)
	_ = st.give(t, 47)
	_ = st.give(t, 48)
	<-click
	if !s.HasNear(rest(48)) {
		t.Fatal("the look says no frames are near the click, with its run still reading")
	}
	if s.HasNear(rest(300)) {
		t.Fatal("the look says frames are near a place nothing was read around")
	}
	if !s.HasNear(move(48, 50)) {
		t.Fatal("the look says no frames are near a slow drag's place")
	}
	if s.HasNear(move(48, 60)) {
		t.Fatal("a fast drag would be sent the frames around a place it passes over")
	}
	near := make(chan []Seen, 1)
	go func() { near <- s.Near(context.Background(), rest(48), func(f int) bool { return f == 47 }) }()
	for f := 49; f <= 48+lookAhead; f++ {
		_ = st.give(t, f)
	}
	var got []int
	for _, n := range <-near {
		got = append(got, n.Frame)
	}
	if want := []int{49, 50, 51, 52}; !slices.Equal(got, want) {
		t.Fatalf("the window was handed frames %v, want %v", got, want)
	}
}

// A newer Ahead takes the place of the one before, an empty one too, as a
// play starts: what was being read is stopped, so the play's frames do
// not wait behind it.
func TestANewerAheadStopsTheOneBefore(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	read := aheadFor(s, aheadOf([]int{50, 60}, nil))
	word := dec.next(t)
	if got := <-aheadFor(s, aheadOf(nil, nil)); got.err != nil {
		t.Fatalf("an empty Ahead ended with %v", got.err)
	}
	if got := <-read; !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the Ahead before got %v", got.err)
	}
	word.stopped(t)
	dec.none(t)
}

// An Ahead the window sent before the newest one, which reached the Go
// side after it, is answered as overtaken and stops nothing: the newest
// one reads on. The window paused a play and sent the frames of the
// playhead it stopped on a millisecond after an older ask, which came
// last and took its place, so a step to the next word waited for its
// frame.
func TestAnAheadSentBeforeTheNewestTakesNothingsPlace(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	newest := aheadOf([]int{50}, nil)
	newest.Sent = Sent{Window: "a", N: 2}
	read := aheadFor(s, newest)
	word := dec.next(t)
	older := aheadOf([]int{300}, nil)
	older.Sent = Sent{Window: "a", N: 1}
	if got := <-aheadFor(s, older); !errors.Is(got.err, ErrOvertaken) || got.frames != nil {
		t.Fatalf("the Ahead sent before the newest got %v, %v, want it overtaken", got.frames, got.err)
	}
	for f := 49; f <= 50+lookAhead; f++ {
		if err := word.give(t, f); f < 50+lookAhead && err != nil {
			t.Fatalf("the newest Ahead stopped at %d: %v", f, err)
		}
	}
	if got := <-read; got.err != nil || len(got.frames) != 6 {
		t.Fatalf("the newest Ahead got %v, %v", got.frames, got.err)
	}
	// Another window's ask is the newest when it comes.
	other := aheadOf([]int{300}, nil)
	other.Sent = Sent{Window: "b", N: 1}
	read = aheadFor(s, other)
	st := dec.next(t)
	if st.from != 299 {
		t.Fatalf("another window's Ahead was read from %d", st.from)
	}
	for f := 299; f <= 300+lookAhead; f++ {
		_ = st.give(t, f)
	}
	if got := <-read; got.err != nil {
		t.Fatalf("another window's Ahead got %v", got.err)
	}
}

// A look the window sent before the newest one, which reached the Go side
// after it, is answered as overtaken and leaves the newest place waiting:
// the decoder goes on to where the hand is, not to where it was.
func TestALookSentBeforeTheNewestTakesNothingsPlace(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	newest := rest(200)
	newest.Sent = Sent{Window: "a", N: 2}
	place := lookFor(s, newest)
	st := dec.next(t)
	older := rest(48)
	older.Sent = Sent{Window: "a", N: 1}
	if got := <-lookFor(s, older); !errors.Is(got.err, ErrOvertaken) {
		t.Fatalf("the look sent before the newest got %+v, %v, want it overtaken", got.seen, got.err)
	}
	_ = st.give(t, 199)
	_ = st.give(t, 200)
	if got := <-place; got.err != nil || got.seen.Frame != 200 {
		t.Fatalf("the newest look got %+v, %v", got.seen, got.err)
	}
}

// What the window holds is neither read nor handed over again, and what
// the schedule kept already is handed over with no decoding.
func TestTheWindowIsHandedOnlyWhatItLacks(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	click := lookFor(s, rest(50))
	st := dec.next(t)
	for f := 49; f <= 50+lookAhead; f++ {
		_ = st.give(t, f)
	}
	<-click
	a := aheadOf([]int{50}, []int{200, 300})
	a.Has = []int{51, 300}
	read := aheadFor(s, a)
	card := dec.next(t)
	if card.from != 200 {
		t.Fatalf("the reading ahead started from %d, not the card the window lacks", card.from)
	}
	_ = card.give(t, 200)
	got := <-read
	if got.err != nil {
		t.Fatalf("the reading ahead ended with %v", got.err)
	}
	if want := []int{49, 50, 52, 53, 54, 200}; !slices.Equal(got.frames, want) {
		t.Fatalf("the window was handed frames %v, want %v", got.frames, want)
	}
	dec.none(t)
}

// The frame under the hand and the first frame of the play cued there are
// asked for at once: the second does not take the first one's place, and
// both get the frame. One of them came back empty, and the picture stood
// on the frame after the playhead for good.
func TestTheSamePlaceAskedTwiceAnswersBoth(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	still := lookFor(s, rest(48))
	st := dec.next(t)
	waits(s, rest(48))
	cue := lookFor(s, rest(48))
	until(s, func() bool { return len(s.follows) == 1 })
	_ = st.give(t, 47)
	_ = st.give(t, 48)
	for _, got := range []looked{<-still, <-cue} {
		if got.err != nil || got.seen.Frame != 48 || !got.seen.Exact {
			t.Fatalf("a look at the place got %+v, %v", got.seen, got.err)
		}
	}
	dec.none(t)
}
