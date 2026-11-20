package engine

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"framefairy/internal/framewire"
	"framefairy/internal/wake"
)

// Schedule decides which frames of a video the decoder makes for the
// video preview while it stands still: a click, a drag of the playhead, a
// step with the arrow keys. The window says where the hand is and which
// frame it shows, Hand, and draws the frame it is sent. Which frame comes
// next, what the decoder gives up on and what is kept is decided here,
// where the cursors and their cost can be seen, see
// docs/decisions/2026-11-20-the-video-preview-keeps-up-with-the-hand.md.
//
// It is one rule, asked again after every frame the decoder makes and
// every move of the hand, see plan: the decoder always works toward the
// frame under the hand while the window does not have it, and every frame
// it makes that is nearer the hand than the frame on screen goes up at
// once. Once the hand's frame is there, it reads the frames around it, the
// way the hand is going, and then what the hand is likely to ask for next,
// see Ahead.
type Schedule struct {
	frames framesSource
	// say writes a line for the log, see dragLog. Nil in a test.
	say func(line string)

	mu sync.Mutex
	// hand is the newest look of the hand, once there is one, and back
	// says its last move went to the left. early says a look of the place
	// takes a frame on the way to it: the hand drags, or let go of a drag
	// there. A click takes only its own frame.
	hand    Hand
	handSet bool
	back    bool
	early   bool
	// step is how far the hand's last move went, in frames.
	step int
	// The looks of the hand's place that wait for a frame nearer it than
	// the one they show.
	waiting []*lookWaiter
	// The decoder's work: one stream at a time.
	run  *lookRun
	kept keptFrames
	// keys are the key frames of each grid the decoder was seen to start
	// from, in order, see keyBefore.
	keys map[framewire.Grid][]int
	// What to read while the hand rests, see Ahead.
	ahead *aheadWork
	// The play that has the decoder, see Play.
	play *Play
	// Where the newest look of the hand and the newest Ahead stood among
	// what their window sent, see Sent.
	lastLook, lastAhead Sent
	// looks wakes whoever waits whenever a look starts to wait or a run
	// starts or ends, so a test goes on once the schedule has it. Only the
	// tests wait on it.
	looks wake.Wake
	// drag is the drag under way, for the log, see dragLog.
	drag *dragLog
}

// dragLog is what a drag did, written as one line once the hand lets go:
// every place the hand went to in turn, and how long the window waited
// at most for a frame. A drag that lagged on someone's machine is then
// told in their report the way it went, and can be played in the
// simulation, see drag_test.go.
type dragLog struct {
	began  time.Time
	places []int
	// longest is the longest a look waited for its answer, at place at.
	longest time.Duration
	at      int
}

// dragPlaces is how many places of a drag the log keeps, about eight
// seconds of it.
const dragPlaces = 500

// logDrag takes a look into the drag under way, and writes the drag that
// ended. Under the lock.
func (s *Schedule) logDrag(l Hand) {
	if !l.Moving {
		s.endDrag()
		return
	}
	if s.drag == nil {
		s.drag = &dragLog{began: time.Now()}
	}
	if d := s.drag; len(d.places) < dragPlaces && (len(d.places) == 0 || d.places[len(d.places)-1] != l.Frame) {
		d.places = append(d.places, l.Frame)
	}
}

// waited notes how long a look of a drag waited. Under the lock.
func (s *Schedule) waited(l Hand, took time.Duration) {
	if d := s.drag; d != nil && l.Moving && took > d.longest {
		d.longest, d.at = took, l.Frame
	}
}

// endDrag writes the drag under way, if any. Under the lock.
func (s *Schedule) endDrag() {
	d := s.drag
	if d == nil {
		return
	}
	s.drag = nil
	if s.say == nil || len(d.places) == 0 {
		return
	}
	var b strings.Builder
	for i, f := range d.places {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(f))
	}
	s.say(fmt.Sprintf("a drag of %d ms over %d places, the longest wait %d ms at frame %d, places %s",
		time.Since(d.began).Milliseconds(), len(d.places), d.longest.Milliseconds(), d.at, b.String()))
}

// framesSource is where a schedule's frames come from, the episode's
// decoder in the app, see EpisodeFrames.Stream. A test hands in its own,
// which makes frames when the test says.
type framesSource interface {
	Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
		skip func() float64, got func(at float64, frame []byte) error) error
}

// NewSchedule is the schedule of the video whose decoder this is. What it
// says for the log goes where the decoder's lines go.
func NewSchedule(f *EpisodeFrames) *Schedule {
	s := newSchedule(f)
	s.say = func(line string) { f.said.say("schedule", f.path, 0, slog.LevelDebug, line) }
	return s
}

func newSchedule(frames framesSource) *Schedule {
	return &Schedule{frames: frames, kept: newKeptFrames(lookKeptBytes), keys: map[framewire.Grid][]int{}}
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
	// Shown is the frame on screen, or -1: a look is answered with a frame
	// nearer the place than that one. A look whose window shows the frame
	// of the place tells only where the hand is, and is answered with
	// ErrShown at once.
	Shown int
	// Sent is where the look stands among what its window sent.
	Sent Sent
}

// Sent is where a look or an Ahead stands among what its window sent: the
// window's own name, and how many it had sent before. Requests reach the
// Go side in any order, since each is served on a goroutine of its own,
// so the newest is the one the window sent last, not the one that came
// last. An Ahead sent a millisecond before the newest once came after it
// and took its place, and the frames of the word beside were never read.
// A request with no window's name stands nowhere and is the newest when
// it comes.
type Sent struct {
	Window string
	N      uint64
}

// before says the window sent s before t.
func (s Sent) before(t Sent) bool {
	return s.Window != "" && s.Window == t.Window && s.N < t.N
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

// ErrShown answers a look whose window shows the frame of its place
// already: the schedule now knows where the hand is, and there is nothing
// to send.
var ErrShown = errors.New("the window shows the frame of the place")

// How many frames a schedule reads around the place the hand is on, in
// frames of the grid: after a place the hand rests on, so a step to the
// right finds its frame kept, and before it, so the steps to the left
// find theirs. A hand that drags to the right has twice as many read
// ahead of it, and one that drags to the left has the frames from the key
// frame before it read, see around. A click further than lookBack frames
// from the frame on screen has its own frame read first, see startHand.
// Issue #596.
const (
	lookAhead = 4
	lookBack  = 15
	lookFast  = 8
)

// lookSeek is how many frames ahead of a stream a key frame has to lie
// before a new stream that seeks to it is worth more than reading on:
// about what an open costs, 10 ms, at 3.8 ms a frame on an M2 Max.
const lookSeek = 2

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
	look   Hand
	answer chan lookAnswer
}

type lookAnswer struct {
	seen Seen
	err  error
}

// What a run of the decoder reads for: the frame under the hand, the
// frames around it, see around, or what the hand is likely to ask for
// next, see Ahead.
type runKind int

const (
	runHand runKind = iota
	runAfter
	runBefore
	runAhead
)

// lookRun is one stream of the decoder: from a frame on, every frame it
// makes handed over, kept, and sent to the looks that want it, until the
// schedule wants other work, see plan.
type lookRun struct {
	kind runKind
	grid framewire.Grid
	w, h int
	from int
	// key says the stream hands over the key frame it starts decoding
	// from first, and every frame from there, see EpisodeFrames.Stream.
	key bool
	// target is the last frame the run reads.
	target int
	// next is the frame after the last one it handed over.
	next int
	gave bool
	stop context.CancelFunc
	// done is closed once the run's stream has ended, see Near.
	done chan struct{}
}

func (r *lookRun) sized(l Hand) bool {
	return r.grid == l.Grid && r.w == l.W && r.h == l.H
}

// Look answers where the hand is with a frame: the frame of the place,
// or, while the hand drags or once it lets go of a drag, the frame kept or
// made next that is nearer the place than the frame on screen. A newer
// look of another place answers it with ErrOvertaken.
func (s *Schedule) Look(ctx context.Context, l Hand) (Seen, error) {
	w := &lookWaiter{look: l, answer: make(chan lookAnswer, 1)}
	s.mu.Lock()
	if l.Sent.before(s.lastLook) {
		// The window has sent a newer look, which took this one's place
		// before it came.
		s.mu.Unlock()
		return Seen{}, ErrOvertaken
	}
	s.lastLook = l.Sent
	s.logDrag(l)
	same := s.handSet && sameSize(s.hand, l)
	if !same || s.hand.Frame != l.Frame {
		s.overtake()
		s.early = l.Moving || same && s.hand.Moving
		if same && l.Frame != s.hand.Frame {
			s.back = l.Frame < s.hand.Frame
			s.step = framesApart(l.Frame, s.hand.Frame)
		}
	} else {
		// The same place asked again: the window drew a frame on the way
		// and wants the next, or two asks crossed. Both are answered.
		s.early = s.early || l.Moving
	}
	s.hand, s.handSet = l, true
	// The hand takes the playhead from a play.
	s.stopPlay()
	if l.Shown == l.Frame {
		s.plan()
		s.mu.Unlock()
		return Seen{}, ErrShown
	}
	if seen, ok := s.best(l); ok {
		s.plan()
		s.mu.Unlock()
		return seen, nil
	}
	s.waiting = append(s.waiting, w)
	s.plan()
	s.looks.WakeLocked()
	s.mu.Unlock()
	asked := time.Now()
	select {
	case a := <-w.answer:
		s.mu.Lock()
		s.waited(l, time.Since(asked))
		s.mu.Unlock()
		return a.seen, a.err
	case <-ctx.Done():
		s.mu.Lock()
		if i := slices.Index(s.waiting, w); i >= 0 {
			s.waiting = slices.Delete(s.waiting, i, i+1)
		}
		s.mu.Unlock()
		return Seen{}, ctx.Err()
	}
}

func sameSize(a, b Hand) bool {
	return a.Grid == b.Grid && a.W == b.W && a.H == b.H
}

// best is the kept frame a look is answered with: its own, or where the
// look takes one on the way, the kept frame nearest its place that is
// nearer than the frame on screen. Under the lock.
func (s *Schedule) best(l Hand) (Seen, bool) {
	key := keptKey{grid: l.Grid, w: l.W, h: l.H, frame: l.Frame}
	if k, ok := s.kept.get(key); ok {
		return Seen{Frame: l.Frame, At: k.at, Picture: k.picture, Exact: true, Kept: true}, true
	}
	if !s.early {
		return Seen{}, false
	}
	// Nearer than the frame on screen, and no further than a few spans of
	// key frames: further than that, the decoder has the frame sooner
	// than this finds it, and nothing that far is kept for long.
	most := 4 * s.span(l.Grid)
	if l.Shown >= 0 {
		most = min(most, framesApart(l.Shown, l.Frame)-1)
	}
	// The side the frame on screen is on first: a frame there is on the
	// way from it.
	toward := -1
	if l.Shown > l.Frame {
		toward = 1
	}
	for d := 1; d <= most; d++ {
		for _, f := range [2]int{l.Frame + toward*d, l.Frame - toward*d} {
			key.frame = f
			if k, ok := s.kept.get(key); ok {
				return Seen{Frame: f, At: k.at, Picture: k.picture, Kept: true}, true
			}
		}
	}
	return Seen{}, false
}

// overtake answers every look that waits with ErrOvertaken: the hand went
// elsewhere. Under the lock.
func (s *Schedule) overtake() {
	for _, w := range s.waiting {
		w.answer <- lookAnswer{err: ErrOvertaken}
	}
	s.waiting = nil
}

// LetFramesGo empties the frames the schedule kept, once the video's
// workspace is asleep: the decoder stays open a while for a video that
// comes back, and each of those kept up to lookKeptBytes.
func (s *Schedule) LetFramesGo() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kept = newKeptFrames(lookKeptBytes)
}

// Close stops the decoder's work for the hand and answers every look that
// waits with ErrOvertaken: the video preview has let go of the video.
func (s *Schedule) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endDrag()
	s.overtake()
	s.handSet = false
	if a := s.ahead; a != nil {
		s.ahead = nil
		a.done <- aheadDone{err: ErrOvertaken}
	}
	s.stopPlay()
	s.stopRun()
	s.kept = newKeptFrames(lookKeptBytes)
}

// stopRun stops the decoder's work between two frames. Under the lock.
func (s *Schedule) stopRun() {
	r := s.run
	if r == nil {
		return
	}
	r.stop()
	s.run = nil
	if a := s.ahead; r.kind == runAhead && a != nil && r.next <= r.target && a.grid == r.grid && a.w == r.w && a.h == r.h {
		// The hand came first: the frames are read once it rests again.
		a.todo = append([]aheadSpan{{from: r.next, to: r.target}}, a.todo...)
	}
	s.looks.WakeLocked()
}

// span is how many frames there are from one key frame to the next, as
// far as the schedule knows: a second's worth, which is what encoders
// make unless told otherwise, and what Tim's videos have.
func (s *Schedule) span(g framewire.Grid) int {
	return max(1, (g.Num+g.Den/2)/g.Den)
}

// keyBefore is the key frame the decoder starts from for a frame, as far
// as the schedule knows: the one the decoder was seen to start from, where
// the frame lies within a span after it, and otherwise one on the spans
// counted from it, or from the start. Under the lock.
func (s *Schedule) keyBefore(g framewire.Grid, frame int) int {
	span := s.span(g)
	keys := s.keys[g]
	i, _ := slices.BinarySearch(keys, frame+1)
	if i == 0 {
		if len(keys) > 0 && keys[0]-frame <= span {
			// Counted back from the first key frame known.
			return frame - ((frame-keys[0])%span+span)%span
		}
		return frame - frame%span
	}
	k := keys[i-1]
	return k + (frame-k)/span*span
}

// learnKey keeps a key frame the decoder was seen to start from. Under
// the lock.
func (s *Schedule) learnKey(g framewire.Grid, frame int) {
	keys := s.keys[g]
	i, found := slices.BinarySearch(keys, frame)
	if !found {
		s.keys[g] = slices.Insert(keys, i, frame)
	}
}

// work is what the decoder should do next: a run of a kind, from a frame
// to a frame, with the key frame first or not.
type work struct {
	kind     runKind
	from, to int
	key      bool
}

// plan is the schedule's one rule, asked again after every frame the
// decoder makes, every look of the hand, and every change of what has the
// decoder: what the decoder should do now, see want, and whether the run
// under way does it, see goesOn. A run that does not is stopped between
// two frames and the one that does is started. Under the lock.
func (s *Schedule) plan() {
	if s.play != nil {
		return
	}
	j, ok := s.want()
	if r := s.run; r != nil {
		if ok && s.goesOn(r, j) {
			r.kind, r.target = j.kind, max(j.to, r.next-1)
			return
		}
		s.stopRun()
	}
	if !ok {
		s.readAhead()
		return
	}
	s.start(j)
}

// want is what the decoder should do for the hand, the most wanted
// first: the frame under it, then the frames on the side it drags to,
// then a few on either side, and false when all of it is kept. The few on
// either side are read whatever the hand does, since nobody knows where
// it goes next, and a decoder with nothing to do costs the hand the same
// as one stopped between two frames for its next place. Under the lock.
func (s *Schedule) want() (work, bool) {
	if !s.handSet {
		return work{}, false
	}
	h := s.hand
	has := func(f int) bool { return s.kept.has(keptKey{grid: h.Grid, w: h.W, h: h.H, frame: f}) }
	after, before := s.around()
	if !has(h.Frame) && h.Shown != h.Frame {
		j := s.forHand(h, after)
		for j.from < h.Frame && has(j.from) {
			j.from++
		}
		return j, true
	}
	ahead := func(n int) (work, bool) {
		last := min(h.Grid.Count-1, h.Frame+n)
		for f := h.Frame + 1; f <= last; f++ {
			if !has(f) {
				return work{kind: runAfter, from: f, to: last}, true
			}
		}
		return work{}, false
	}
	behind := func(n int) (work, bool) {
		// The lowest of the frames kept one after another up to the hand.
		b := h.Frame
		for b > 0 && h.Frame-b < n && has(b-1) {
			b--
		}
		if b == 0 || h.Frame-b >= n {
			return work{}, false
		}
		// One stream from the key frame before the furthest of them up
		// to the kept ones: the frames from that key frame on are decoded
		// either way, and the hand that goes on to the left wants them
		// next.
		return work{kind: runBefore, from: s.keyBefore(h.Grid, max(0, h.Frame-n)), to: b - 1, key: true}, true
	}
	sides := []func() (work, bool){
		func() (work, bool) { return ahead(after) },
		func() (work, bool) { return behind(before) },
		func() (work, bool) { return ahead(lookAhead) },
		func() (work, bool) { return behind(lookBack) },
	}
	if s.hand.Moving && s.step > lookFast {
		sides = sides[:2]
	} else if s.hand.Moving && s.back {
		sides[0], sides[1] = sides[1], sides[0]
		sides[2], sides[3] = sides[3], sides[2]
	}
	for _, side := range sides {
		if j, ok := side(); ok {
			return j, true
		}
	}
	return work{}, false
}

// around is how many frames after the hand and before it are read first,
// once its own frame is there: a few either side of a hand that rests,
// more ahead of one that drags to the right, and a span of key frames
// behind one that drags to the left, where every frame costs a seek to
// the key frame before it. A hand that drags faster than lookFast frames
// at a move passes over them before they would be shown, and has only the
// few either side read: the decoder is kept for the frame under it. Under
// the lock.
func (s *Schedule) around() (after, before int) {
	switch {
	case !s.hand.Moving:
		return lookAhead, lookBack
	case s.step > lookFast:
		return 0, 0
	case s.back:
		return 0, s.span(s.hand.Grid)
	}
	return 2 * lookAhead, 0
}

// forHand is the run for the frame under the hand. A hand that drags has
// the key frame the decoder starts from first, and every frame from
// there, each sent where it is nearer than the frame on screen: the
// picture follows the hand while the decoder works its way to the frame.
// A click has its own frame and the frame before, which the arrow to the
// left asks for next, from the frame before: the frames between the key
// frame and it are decoded and not made into pictures. A click far from
// the frame on screen has its own frame first, and the frame before is
// read after, see want. Under the lock.
func (s *Schedule) forHand(h Hand, after int) work {
	to := min(h.Grid.Count-1, h.Frame+after)
	switch {
	case h.Moving && s.back:
		// Every frame of the span the hand drags into: it wants them
		// one after another as it goes on, and a stream that read only
		// the nearest would seek back to the same key frame for the rest.
		return work{kind: runHand, from: s.keyBefore(h.Grid, h.Frame), to: to, key: true}
	case h.Moving || s.early:
		return work{kind: runHand, from: s.nearer(h), to: to, key: true}
	case h.Shown >= 0 && framesApart(h.Frame, h.Shown) > lookBack:
		return work{kind: runHand, from: h.Frame, to: to}
	}
	return work{kind: runHand, from: max(0, h.Frame-1), to: to}
}

// nearer is the first frame of a drag's run: the furthest from the hand
// of the frames between the key frame before it and it that are nearer
// the hand than the frame on screen. Every frame from there is sent as it
// comes, so the picture walks toward the hand while the decoder reads up
// to it. Under the lock.
func (s *Schedule) nearer(h Hand) int {
	k := s.keyBefore(h.Grid, h.Frame)
	if h.Shown < 0 {
		return k
	}
	return max(k, h.Frame-framesApart(h.Shown, h.Frame)+1)
}

// goesOn tells whether the run under way does the job: it reads the
// frames of the job by reading on, on the same grid at the same size,
// with no key frame worth seeking to on the way. A job before the run's
// place, or after a key frame it would read up to first, is a new stream:
// a seek to that key frame is quicker than reading every frame up to it.
// A job that wants the key frame first is done by a run that reads on to
// its frame, since that run sends every frame on the way. Under the lock.
func (s *Schedule) goesOn(r *lookRun, j work) bool {
	if !r.sized(s.hand) || r.kind == runAhead {
		return false
	}
	if j.kind == runBefore {
		return r.kind == runBefore && r.target == j.to
	}
	if j.key && !r.key && !r.gave {
		// A click the hand now drags from: its frames on the way are
		// worth more than what the stream has decoded so far.
		return false
	}
	if j.from < r.next {
		// The run passed frames the job wants. A run for the hand that
		// has yet to reach its frame goes on all the same: the frames it
		// hands on the way are nearer than the picture too, and a new
		// stream would start over on each move of a hand that drags back
		// toward it.
		if j.kind != runHand || r.kind != runHand || r.next > s.hand.Frame {
			return false
		}
		return s.keyBefore(r.grid, s.hand.Frame) <= r.next+lookSeek
	}
	return s.keyBefore(r.grid, j.from) <= r.next+lookSeek
}

// start starts the decoder on a job for the hand. Under the lock.
func (s *Schedule) start(j work) {
	s.startRun(j, s.hand.Grid, s.hand.W, s.hand.H)
}

// startRun starts a stream of the decoder for a job on a grid at a size.
// Under the lock.
func (s *Schedule) startRun(j work, grid framewire.Grid, w, h int) {
	ctx, stop := context.WithCancel(context.Background())
	r := &lookRun{kind: j.kind, grid: grid, w: w, h: h, from: j.from, key: j.key, target: j.to, next: j.from, stop: stop, done: make(chan struct{})}
	s.run = r
	s.looks.WakeLocked()
	go func() {
		err := s.frames.Stream(ctx, r.grid, r.grid.At(r.from), r.w, r.h, r.key, nil, func(at float64, frame []byte) error {
			return s.got(r, at, frame)
		})
		s.ended(r, err)
	}()
}

// errLookDone ends a run that has no more to do: it read the frames it was
// for, or the schedule wants other work.
var errLookDone = errors.New("the look has its frames")

// got takes a frame the decoder made for a run: kept, sent to every look
// it is nearer than the frame on screen of, and then the schedule asked
// whether the run goes on, see plan.
func (s *Schedule) got(r *lookRun, at float64, picture []byte) error {
	frame := r.grid.Holding(at)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.key && !r.gave && frame < r.from {
		s.learnKey(r.grid, frame)
	}
	r.gave = true
	// The key frame handed first leaves the run where it was asked to
	// start: the frames up to there are decoded and not handed.
	r.next = max(r.next, frame+1)
	s.kept.put(keptKey{grid: r.grid, w: r.w, h: r.h, frame: frame}, at, picture)
	if a := s.ahead; a != nil {
		a.take(r.grid, r.w, r.h, frame, at, picture)
	}
	// A frame of a run the schedule has stopped is shown too, where it is
	// nearer: it was made, and the hand that left its place may be near.
	s.answer(r, frame)
	if s.run != r {
		return errLookDone
	}
	if r.kind == runAhead {
		if frame >= r.target {
			s.run = nil
			s.looks.WakeLocked()
			s.plan()
			return errLookDone
		}
		return nil
	}
	s.plan()
	if s.run != r {
		return errLookDone
	}
	return nil
}

// answer sends the looks that wait the frame a run made where it is
// nearer their place than the frame they show, or rather the kept frame
// nearest it, see best. Under the lock.
func (s *Schedule) answer(r *lookRun, frame int) {
	kept := s.waiting[:0]
	for _, w := range s.waiting {
		l := w.look
		near := frame == l.Frame || s.early && (l.Shown < 0 || framesApart(frame, l.Frame) < framesApart(l.Shown, l.Frame))
		if r.sized(l) && near {
			if seen, ok := s.best(l); ok {
				seen.Kept = false
				w.answer <- lookAnswer{seen: seen}
				continue
			}
		}
		kept = append(kept, w)
	}
	s.waiting = kept
}

// ended takes the end of a run's stream. A run that read what it was for,
// or was stopped, leaves the schedule to say what comes next. One whose
// decoder failed, or whose video ended before its last frame, tells the
// looks waiting for what it never brought why, and the hand's place is
// read no more until the hand moves: asked again, it would fail again.
func (s *Schedule) ended(r *lookRun, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(r.done)
	if s.run != r {
		return
	}
	s.run = nil
	s.looks.WakeLocked()
	failed := err != nil && !errors.Is(err, errLookDone)
	if !failed && r.next > r.target {
		s.plan()
		return
	}
	why := err
	if !failed {
		why = errors.New("the video ended before the frame asked for")
	}
	kept := s.waiting[:0]
	for _, w := range s.waiting {
		if r.sized(w.look) && w.look.Frame >= r.next {
			w.answer <- lookAnswer{err: why}
			continue
		}
		kept = append(kept, w)
	}
	s.waiting = kept
	if r.kind != runAhead {
		s.handSet = false
	}
	s.plan()
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
	// Sent is where it stands among what its window sent.
	Sent Sent
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
	if a.Sent.before(s.lastAhead) {
		// The window has sent a newer Ahead, which took this one's place
		// before it came.
		s.mu.Unlock()
		return nil, ErrOvertaken
	}
	s.lastAhead = a.Sent
	if old := s.ahead; old != nil {
		s.ahead = nil
		old.done <- aheadDone{err: ErrOvertaken}
	}
	if r := s.run; r != nil && r.kind == runAhead {
		s.stopRun()
	}
	for f := range work.wants {
		if k, ok := s.kept.peek(keptKey{grid: a.Grid, w: a.W, h: a.H, frame: f}); ok {
			work.take(a.Grid, a.W, a.H, f, k.at, k.picture)
		}
	}
	slices.SortFunc(work.out, func(x, y Seen) int { return x.Frame - y.Frame })
	s.ahead = work
	s.plan()
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
			if r := s.run; r != nil && r.kind == runAhead {
				s.stopRun()
			}
		}
		s.mu.Unlock()
		return nil, ctx.Err()
	}
}

// readAhead starts the decoder on the next frame to read ahead, once it
// has nothing to do for the hand and the hand does not hold the playhead,
// or says the reading is done. Under the lock.
func (s *Schedule) readAhead() {
	a := s.ahead
	if a == nil || a.held || s.run != nil || s.play != nil || len(s.waiting) > 0 {
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
		s.startRun(work{kind: runAhead, from: read.from, to: read.to}, a.grid, a.w, a.h)
		return
	}
	s.ahead = nil
	a.done <- aheadDone{out: a.out}
}

// Near hands over the frames kept around a place the hand rests on, the
// ones the runs for it read on the way, after it and before it, once the
// runs for its frame and the frames after it have ended: lookBack frames
// before it and lookAhead after, and of those only the ones has says the
// window does not hold. The window keeps them, so the arrow keys and a
// slow drag find their frames with no request, see app.ts. A newer look
// of the hand ends the run, and Near with it.
func (s *Schedule) Near(ctx context.Context, l Hand, has func(frame int) bool) []Seen {
	from, to := max(0, l.Frame-lookBack), min(l.Grid.Count-1, l.Frame+lookAhead)
	s.mu.Lock()
	for {
		r := s.run
		if r == nil || r.kind != runHand && r.kind != runAfter || !r.sized(l) || r.target < from || r.from > to {
			break
		}
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
// a look the window had answered: a run reads around it, or frames around
// it are kept. A hand that drags has none: the frames around each place
// would be sent and let go of again.
func (s *Schedule) HasNear(l Hand) bool {
	if l.Moving {
		return false
	}
	from, to := max(0, l.Frame-lookBack), min(l.Grid.Count-1, l.Frame+lookAhead)
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.run; r != nil && r.kind != runAhead && r.sized(l) && r.target >= from && r.from <= to {
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

func framesApart(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}
