package engine

import (
	"context"
	"errors"
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
	_ func() float64, got func(at float64, frame []byte) error) error {
	s := &heldStream{from: grid.Holding(from), key: key, ctx: ctx, frames: make(chan heldFrame), ended: make(chan error, 1)}
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
	_ = now.give(t, 150)
	if got := <-again; got.seen.Frame != 150 || got.seen.Exact {
		t.Fatalf("the place got %+v, %v, want its key frame", got.seen, got.err)
	}
	again = lookFor(s, move(170, 150))
	waits(s, move(170, 150))
	if err := now.give(t, 170); !errors.Is(err, errLookDone) {
		t.Fatalf("a stream for a hand that moves read on past its place: %v", err)
	}
	if got := <-again; got.seen.Frame != 170 || !got.seen.Exact {
		t.Fatalf("the place got %+v, %v, want its own frame", got.seen, got.err)
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
	back = lookFor(s, move(97, 99))
	run := dec.next(t)
	if run.from != 97-lookBack || run.key {
		t.Fatalf("a slow step back opened a stream from %d, key first %v", run.from, run.key)
	}
	for f := run.from; f <= 97; f++ {
		_ = run.give(t, f)
	}
	if got := <-back; got.seen.Frame != 97 || !got.seen.Exact {
		t.Fatalf("the step back got %+v, %v", got.seen, got.err)
	}
	for f := 96; f > 97-lookBack; f-- {
		if got := <-lookFor(s, move(f, f+1)); got.seen.Frame != f || !got.seen.Exact {
			t.Fatalf("the step back to %d got %+v, %v", f, got.seen, got.err)
		}
	}
	dec.none(t)
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
