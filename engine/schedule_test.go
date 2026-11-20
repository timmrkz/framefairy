package engine

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	until(s, func() bool { return slices.ContainsFunc(s.waiting, func(w *lookWaiter) bool { return w.look == h }) })
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
	// Then the frames further before it, from the key frame before them.
	if behind := dec.next(t); behind.from != 25 || !behind.key {
		t.Fatalf("the frames before the click were read from %d, key first %v", behind.from, behind.key)
	}
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
	// The frames before it come last, from the key frame before the
	// furthest of them, in one stream.
	before := dec.next(t)
	if before.from != 275 || !before.key {
		t.Fatalf("the frames before the click were read from %d, key first %v", before.from, before.key)
	}
	for f := 275; f < 299; f++ {
		_ = before.give(t, f)
	}
	if err := before.give(t, 299); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading of the frames before went on: %v", err)
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
	// Let go after a drag, every frame from the key frame before it is
	// shown on the way, with nothing on screen near it.
	st := dec.next(t)
	if st.from != 300 || !st.key {
		t.Fatalf("letting go opened a stream from %d, key first %v", st.from, st.key)
	}
	_ = st.give(t, 300)
	if got := <-letGo; got.seen.Frame != 300 || !got.seen.Exact {
		t.Fatalf("letting go got %+v, %v", got.seen, got.err)
	}
}

// A hand going slowly to the left: the span of key frames it drags into
// is read whole, every frame of it kept, and once its own frame is there
// the span before is read next, so the steps back find their frames kept
// and the hand never waits at the end of the frames read. Read only to
// the hand's frame, each step back into a span was a seek of its own,
// issue #596, and a slow drag to the left stood still every few frames.
func TestASlowDragBackReadsWholeSpansBehindTheHand(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	press := lookFor(s, rest(100))
	st := dec.next(t)
	_ = st.give(t, 99)
	_ = st.give(t, 100)
	<-press
	// The frame kept nearest the hand is sent at once, and the span the
	// hand drags into is read from its key frame.
	if got := <-lookFor(s, move(97, 100)); got.seen.Frame != 99 || got.seen.Exact {
		t.Fatalf("the step back got %+v, %v, want frame 99 kept", got.seen, got.err)
	}
	st.stopped(t)
	run := dec.next(t)
	if run.from != 75 || !run.key {
		t.Fatalf("a slow step back opened a stream from %d, key first %v", run.from, run.key)
	}
	back := lookFor(s, move(97, 99))
	for f := 75; f < 96; f++ {
		_ = run.give(t, f)
	}
	// The first frame nearer the hand than the one on screen is sent as
	// it comes.
	_ = run.give(t, 96)
	if got := <-back; got.seen.Frame != 96 || got.seen.Exact {
		t.Fatalf("the step back got %+v, %v, want frame 96 on the way", got.seen, got.err)
	}
	back = lookFor(s, move(97, 96))
	if err := run.give(t, 97); !errors.Is(err, errLookDone) {
		t.Fatalf("the stream went on past the hand: %v", err)
	}
	if got := <-back; got.seen.Frame != 97 || !got.seen.Exact {
		t.Fatalf("the step back got %+v, %v", got.seen, got.err)
	}
	behind := dec.next(t)
	if behind.from != 50 || !behind.key {
		t.Fatalf("the span behind the hand was read from %d, key first %v", behind.from, behind.key)
	}
	for f := 96; f >= 75; f-- {
		if got := <-lookFor(s, move(f, f+1)); got.seen.Frame != f || !got.seen.Exact || !got.seen.Kept {
			t.Fatalf("the step back to %d got %+v, %v, want it kept", f, got.seen, got.err)
		}
	}
	dec.none(t)
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
	// The frames before the click come before anything is read ahead.
	behind := dec.next(t)
	if behind.from != 0 || !behind.key {
		t.Fatalf("the frames before the click were read from %d, key first %v", behind.from, behind.key)
	}
	for f := 0; f < 8; f++ {
		_ = behind.give(t, f)
	}
	if err := behind.give(t, 8); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading of the frames before went on: %v", err)
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
	behind := dec.next(t)
	for f := behind.from; f < 298; f++ {
		_ = behind.give(t, f)
	}
	if err := behind.give(t, 298); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading of the frames before went on: %v", err)
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
	if s.HasNear(move(48, 50)) {
		t.Fatal("a drag would be sent the frames around a place it passes over")
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
	behind := dec.next(t)
	for f := behind.from; f < 48; f++ {
		_ = behind.give(t, f)
	}
	if err := behind.give(t, 48); !errors.Is(err, errLookDone) {
		t.Fatalf("the reading of the frames before went on: %v", err)
	}
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
	until(s, func() bool { return len(s.waiting) == 2 })
	_ = st.give(t, 47)
	_ = st.give(t, 48)
	for _, got := range []looked{<-still, <-cue} {
		if got.err != nil || got.seen.Frame != 48 || !got.seen.Exact {
			t.Fatalf("a look at the place got %+v, %v", got.seen, got.err)
		}
	}
	dec.none(t)
}

// What the schedule wants the decoder to do next, for each kind of hand,
// see want: the frame under the hand first, then the frames on the side
// it drags to, then a few on either side.
func TestTheScheduleWantsTheFramesTheHandNeedsFirst(t *testing.T) {
	cases := []struct {
		name string
		hand Hand
		// back and step are how the hand moved last.
		back bool
		step int
		kept []int
		want work
		none bool
	}{
		{name: "a click reads from the frame before it, for the arrow to the left",
			hand: rest(110), want: work{kind: runHand, from: 109, to: 110 + lookAhead}},
		{name: "a far click reads its own frame first",
			hand: Hand{Grid: lookGrid, Frame: 110, W: 4, H: 2, Shown: 400}, want: work{kind: runHand, from: 110, to: 110 + lookAhead}},
		{name: "a drag to the right reads every frame nearer than the picture",
			hand: move(110, 104), step: 3, want: work{kind: runHand, from: 105, to: 110 + 2*lookAhead, key: true}},
		{name: "a drag to the right from the span before reads from the key frame",
			hand: move(110, 90), step: 3, want: work{kind: runHand, from: 100, to: 110 + 2*lookAhead, key: true}},
		{name: "a drag to the right skips what is kept",
			hand: move(110, 104), step: 3, kept: []int{105, 106}, want: work{kind: runHand, from: 107, to: 110 + 2*lookAhead, key: true}},
		{name: "a slow drag to the left reads its span whole",
			hand: move(110, 112), back: true, step: 2, want: work{kind: runHand, from: 100, to: 110, key: true}},
		{name: "a fast drag to the left reads from the key frame before the hand",
			hand: move(110, 140), back: true, step: 30, want: work{kind: runHand, from: 100, to: 110, key: true}},
		{name: "a drag to the right with its frame reads ahead of it",
			hand: move(110, 109), step: 1, kept: []int{110}, want: work{kind: runAfter, from: 111, to: 110 + 2*lookAhead}},
		{name: "a drag to the left with its frame reads the span behind it",
			hand: move(110, 111), back: true, step: 1, kept: []int{110}, want: work{kind: runBefore, from: 75, to: 109, key: true}},
		{name: "a drag to the left with its span reads a few after it",
			hand: move(110, 111), back: true, step: 1, kept: span(85, 110), want: work{kind: runAfter, from: 111, to: 110 + lookAhead}},
		{name: "a fast drag with its frame reads nothing more",
			hand: move(110, 140), back: true, step: 30, kept: []int{110}, none: true},
		{name: "a hand at rest has the frames after it, then before it",
			hand: rest(110), kept: span(110, 110+lookAhead), want: work{kind: runBefore, from: 75, to: 109, key: true}},
		{name: "a hand at rest with all of it kept wants nothing",
			hand: rest(110), kept: span(110-lookBack, 110+lookAhead), none: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSchedule(newHeldDecoder())
			s.hand, s.handSet, s.back, s.step = c.hand, true, c.back, c.step
			s.early = c.hand.Moving
			for _, f := range c.kept {
				s.kept.put(keptKey{grid: lookGrid, w: 4, h: 2, frame: f}, lookGrid.At(f), make([]byte, 32))
			}
			got, ok := s.want()
			if c.none {
				if ok {
					t.Fatalf("wants %+v, want nothing", got)
				}
				return
			}
			if !ok || got != c.want {
				t.Fatalf("wants %+v, %v, want %+v", got, ok, c.want)
			}
		})
	}
}

// span is the frames from one to another, both in.
func span(from, to int) []int {
	var out []int
	for f := from; f <= to; f++ {
		out = append(out, f)
	}
	return out
}

// Whether the run under way goes on for what the schedule wants next, or
// is stopped for a new stream, see goesOn.
func TestARunGoesOnOnlyWhereThatIsQuickest(t *testing.T) {
	cases := []struct {
		name string
		run  lookRun
		hand int
		job  work
		want bool
	}{
		{name: "a drag on within the span reads on",
			run: lookRun{kind: runHand, from: 100, next: 105, target: 108}, hand: 110,
			job: work{kind: runHand, from: 106, to: 118}, want: true},
		{name: "a drag past a key frame seeks to it",
			run: lookRun{kind: runHand, from: 100, next: 105, target: 108}, hand: 130,
			job: work{kind: runHand, from: 126, to: 138}, want: false},
		{name: "a run that has yet to reach the hand goes on, though the picture fell further behind",
			run: lookRun{kind: runHand, from: 100, next: 105, target: 110}, hand: 110,
			job: work{kind: runHand, from: 102, to: 110}, want: true},
		{name: "a hand gone back before the run starts again",
			run: lookRun{kind: runHand, from: 100, next: 105, target: 110}, hand: 103,
			job: work{kind: runHand, from: 100, to: 103}, want: false},
		{name: "the frames behind go on to the same end",
			run: lookRun{kind: runBefore, from: 75, next: 80, target: 99}, hand: 100,
			job: work{kind: runBefore, from: 75, to: 99, key: true}, want: true},
		{name: "the frames behind a hand that moved on start again",
			run: lookRun{kind: runBefore, from: 75, next: 80, target: 99}, hand: 101,
			job: work{kind: runBefore, from: 75, to: 100, key: true}, want: false},
		{name: "reading ahead for the cards gives way to the hand",
			run: lookRun{kind: runAhead, from: 100, next: 105, target: 110}, hand: 106,
			job: work{kind: runHand, from: 106, to: 106}, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSchedule(newHeldDecoder())
			s.hand, s.handSet = move(c.hand, c.hand-1), true
			r := c.run
			r.grid, r.w, r.h = lookGrid, 4, 2
			if got := s.goesOn(&r, c.job); got != c.want {
				t.Fatalf("goes on %v, want %v", got, c.want)
			}
		})
	}
}

// A window that holds the frame under the hand says where the hand is all
// the same: nothing is read for that frame, and the frames around it are.
// One that shows the frame at another size, from before the app was
// sized, waits for it at the new size.
func TestAWindowWithTheFrameStillSaysWhereTheHandIs(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	h := rest(110)
	h.Shown, h.Has = 110, true
	if got := <-lookFor(s, h); !errors.Is(got.err, ErrShown) {
		t.Fatalf("a look at the frame on screen got %+v, %v", got.seen, got.err)
	}
	st := dec.next(t)
	if st.from != 111 || st.key {
		t.Fatalf("a hand on the frame on screen had a stream from %d, key first %v, want the frames after it", st.from, st.key)
	}
	sized := Hand{Grid: lookGrid, Frame: 110, W: 8, H: 4, Shown: 110}
	resized := lookFor(s, sized)
	st.stopped(t)
	again := dec.next(t)
	_ = again.give(t, 109)
	_ = again.give(t, 110)
	if got := <-resized; got.err != nil || got.seen.Frame != 110 || !got.seen.Exact {
		t.Fatalf("the frame on screen at another size got %+v, %v", got.seen, got.err)
	}
}

// A drag is written to the log as one line once the hand lets go, with
// every place it went to, so a drag that lagged on someone's machine is
// told in their report the way it went.
func TestADragIsLoggedOnceTheHandLetsGo(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	var lines []string
	s.say = func(line string) { lines = append(lines, line) }
	for _, h := range []Hand{move(10, -1), move(11, -1), move(13, -1)} {
		_ = lookFor(s, h)
		waits(s, h)
	}
	until(s, func() bool { return len(lines) == 0 && s.drag != nil && len(s.drag.places) == 3 })
	_ = lookFor(s, rest(13))
	until(s, func() bool { return len(lines) == 1 })
	s.mu.Lock()
	line := lines[0]
	s.mu.Unlock()
	if !strings.Contains(line, "over 3 places") || !strings.HasSuffix(line, "places 10,11,13") {
		t.Fatalf("the drag was logged as %q", line)
	}
}

// A hand that jumps far is shown no frame kept from the stretch it left,
// however much nearer than the frame on screen: the picture would jump
// there and away again once the key frame before the hand comes. The
// first frame on the way is that key frame, read from the start.
func TestAFarJumpShowsNoFrameOfTheStretchItLeft(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	s.mu.Lock()
	for f := 10; f <= 40; f++ {
		s.kept.put(keptKey{grid: lookGrid, w: 4, h: 2, frame: f}, lookGrid.At(f), make([]byte, 32))
	}
	s.mu.Unlock()
	h := move(110, 10)
	at := lookFor(s, h)
	st := dec.next(t)
	if st.from != 100 || !st.key {
		t.Fatalf("the jump had a stream from %d, key first %v, want from its key frame at 100", st.from, st.key)
	}
	_ = st.give(t, 100)
	if got := <-at; got.err != nil || got.seen.Frame != 100 || got.seen.Exact {
		t.Fatalf("the jump got %+v, %v, want the key frame before the hand on the way", got.seen, got.err)
	}
}

// A video preview that lets go of the video in the middle of a drag
// leaves no hand behind: the first look of the next is the frame of its
// place, and no frame on the way, as the hand never moved there. An
// episode opened on its clip showed a frame before the clip's first,
// because a trim had dragged the playhead before its workspace slept.
func TestTheNextVideoPreviewFindsNoDragLeftBehind(t *testing.T) {
	dec := newHeldDecoder()
	s := newSchedule(dec)
	defer s.Close()
	dragged := lookFor(s, move(30, 20))
	st := dec.next(t)
	_ = st.give(t, 30)
	if got := <-dragged; got.err != nil || got.seen.Frame != 30 {
		t.Fatalf("the drag got %+v, %v", got.seen, got.err)
	}
	s.LetFramesGo()
	opened := lookFor(s, rest(19))
	again := dec.next(t)
	if again.key {
		t.Fatalf("the look of a new video preview had a stream from %d with its key frame first, as for a drag", again.from)
	}
	for f := again.from; f <= 19; f++ {
		_ = again.give(t, f)
	}
	if got := <-opened; got.err != nil || got.seen.Frame != 19 || !got.seen.Exact {
		t.Fatalf("the look of a new video preview got %+v, %v, want its own frame and none on the way", got.seen, got.err)
	}
}
