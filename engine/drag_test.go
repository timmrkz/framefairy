package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/internal/framewire"
)

// A drag of the playhead, simulated: a decoder that takes as long as the
// one on Tim's Mac, a hand that moves the way a hand does, and a window
// that asks the way the app does, all on the fake clock of a
// testing/synctest bubble, so a drag of seconds runs in milliseconds and
// every run is the same. What the video preview shows is checked against
// five targets, the ones in the decision record, for every drag:
//
//  1. The decoder never rests while the picture is not the frame under
//     the hand.
//  2. The picture moves toward the hand at least every 50 ms, and 95 % of
//     the time within 25 ms, counted from when the frame that moves it
//     could be there at the soonest: a hand that turns round into a
//     stretch nobody read waits for its key frame on any schedule, see
//     floor.
//  3. After letting go, the exact frame comes within the time an open and
//     the frames of a whole key frame span take, 135 ms at 25 frames.
//  4. The picture is never further behind the hand than one key frame
//     span and how far the hand moved in the last 50 ms.
//  5. While the picture is more than a key frame span from the hand, it
//     moves at least every 50 ms, with no allowance: a fast drag, the
//     usual drag on the range picker, turns by the second target's
//     allowance at every step, so a schedule that never caught up would
//     pass the second.
//
// The decoder's costs were fitted from the decoder's lines in Tim's
// reports of 19 and 20 November 2026, an M2 Max: about 10 ms to open a
// stream at a key frame, then 3.8 ms for every frame, one at a time.
const (
	simOpen  = 10 * time.Millisecond
	simFrame = 3800 * time.Microsecond
	// simScale is what making a picture of a decoded frame adds, at the
	// size of the video preview. The reports do not tell it apart from
	// the decoding, so it is a guess, on the side of too much: a frame let
	// go unscaled costs simFrame, one handed over simFrame and simScale.
	simScale = time.Millisecond
	// simTick is a frame of the screen, at which the hand moves and the
	// window draws.
	simTick = time.Second / 60
	// simKeepUp is how many frames the decoder makes in a frame of the
	// screen: a hand that moves further at a step leaves frames on its
	// way that nobody could read.
	simKeepUp = int(simTick / (simFrame + simScale))
	// simNear is how far the picture may be from the hand, in frames, and
	// count as there for the second target: two frames, 80 ms of video,
	// are where the hand is to the eye. The frame under the hand itself is
	// held to the third, once the hand stops.
	simNear = 2
)

// simDecoder decodes the way the episode's decoder does, see
// EpisodeFrames.Stream and goOn in framefairy-frames: a stream reads on
// from a cursor that stands before its first frame with no key frame
// between, and otherwise opens at the key frame before it. It decodes one
// frame at a time, whichever stream asks.
type simDecoder struct {
	video simVideo
	chip  chan struct{}
	// streams counts the streams under way, so a drag ends once the frame
	// each was making is made: a stream stopped finishes its frame, as the
	// decoder's cursor does.
	streams sync.WaitGroup
	// picture is one buffer every frame is a piece of: the schedule counts
	// the bytes it keeps, and none of them is ever written.
	picture []byte

	mu   sync.Mutex
	idle []int
	busy []simSpan
	// decoded counts every frame the decoder decoded, handed over or not.
	decoded int
	// made is every frame the decoder handed over, when.
	made []simMark
	// told is what the decoder did, for a drag that missed a target.
	told []simEvent
}

// simEvent is a line of what happened in a drag, at a moment.
type simEvent struct {
	at   time.Time
	what string
}

func (d *simDecoder) tell(format string, args ...any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.told = append(d.told, simEvent{time.Now(), fmt.Sprintf(format, args...)})
}

type simSpan struct{ from, to time.Time }

func newSimDecoder(v simVideo) *simDecoder {
	return &simDecoder{video: v, chip: make(chan struct{}, 1), picture: make([]byte, v.w*v.h*4)}
}

func (d *simDecoder) keyOf(frame int) int { return d.video.keyOf(frame) }

// simVideo is the video a drag is over and the video preview it is shown
// in: where its key frames are, the longest stretch from one to the next,
// and the size its frames are made at, which says how many the schedule
// keeps.
type simVideo struct {
	name  string
	keyOf func(frame int) int
	span  int
	w, h  int
}

// everyKey is a video with a key frame every span frames, shown at the
// size the cloud measures at.
func everyKey(name string, span int) simVideo {
	return simVideo{name: name, keyOf: func(f int) int { return f - f%span }, span: span, w: 1012, h: 570}
}

// unevenKeys is a video whose key frames come at gaps that change, the
// way an encoder puts one at each cut: 12, 25, 50 and 75 frames apart in
// turn.
func unevenKeys() simVideo {
	gaps := []int{12, 25, 50, 75}
	round := 0
	for _, g := range gaps {
		round += g
	}
	return simVideo{
		name: "key frames 12 to 75 frames apart",
		keyOf: func(f int) int {
			k := f - f%round
			for _, g := range gaps {
				if k+g > f {
					break
				}
				k += g
			}
			return k
		},
		span: slices.Max(gaps),
		w:    1012,
		h:    570,
	}
}

// work keeps the decoder busy for so long, once it is free.
func (d *simDecoder) work(ctx context.Context, cost time.Duration) error {
	select {
	case d.chip <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	began := time.Now()
	time.Sleep(cost) //nolint:forbidigo // synctest bubble
	d.mu.Lock()
	d.busy = append(d.busy, simSpan{began, time.Now()})
	d.mu.Unlock()
	<-d.chip
	return nil
}

func (d *simDecoder) Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
	skip func() float64, got func(at float64, frame []byte) error) error {
	d.streams.Add(1)
	defer d.streams.Done()
	first := grid.Holding(from)
	d.mu.Lock()
	best := -1
	for i, c := range d.idle {
		if c < first && d.keyOf(first) <= c && (best < 0 || c > d.idle[best]) {
			best = i
		}
	}
	at := -1
	if best >= 0 {
		at = d.idle[best] + 1
		d.idle = slices.Delete(d.idle, best, best+1)
	}
	d.mu.Unlock()
	seeked := at < 0
	if seeked {
		d.tell("decoder: stream from %d, key first %v, opens at key frame %d", first, key, d.keyOf(first))
	} else {
		d.tell("decoder: stream from %d, key first %v, reads on from %d", first, key, at)
	}
	last := -1
	defer func() {
		if last < 0 {
			return
		}
		d.mu.Lock()
		d.idle = append(d.idle, last)
		if len(d.idle) > cursorsKept[pictureCursor] {
			d.idle = d.idle[1:]
		}
		d.mu.Unlock()
	}()
	if seeked {
		if err := d.work(ctx, simOpen); err != nil {
			return err
		}
		at = d.keyOf(first)
	}
	for f := at; f < grid.Count; f++ {
		// With key, a stream that opened hands the key frame over first,
		// then the frames from the one asked for, as the decoder's open
		// does, issue #281.
		out := f >= first || key && seeked && f == at
		if out && skip != nil {
			if s := skip(); s > 0 && f < grid.Holding(s) {
				out = false
			}
		}
		cost := simFrame
		if out {
			cost += simScale
		}
		if err := d.work(ctx, cost); err != nil {
			return err
		}
		last = f
		d.mu.Lock()
		d.decoded++
		d.mu.Unlock()
		if !out {
			continue
		}
		d.mu.Lock()
		d.made = append(d.made, simMark{at: time.Now(), frame: f})
		d.mu.Unlock()
		err := got(grid.At(f), d.picture[:width*height*4])
		d.tell("decoder: frame %d made, %v", f, err)
		if err != nil {
			return err
		}
	}
	return nil
}

// simWindow asks the schedule for where the hand is the way the app does,
// see app.ts: a look for each place the hand moves to that the window
// does not hold, a frame on the way drawn and the place asked again, and
// the frames around a place the hand rests on asked for once it has its
// frame.
type simWindow struct {
	s    *Schedule
	grid framewire.Grid
	w, h int
	ctx  context.Context
	all  sync.WaitGroup

	mu     sync.Mutex
	hand   int
	held   bool
	places int
	shown  int
	has    map[int]bool
	sent   uint64
	shows  []simMark
	told   []simEvent
}

type simMark struct {
	at    time.Time
	frame int
	// held says the hand holds the playhead, for a mark of the hand.
	held bool
}

func newSimWindow(ctx context.Context, s *Schedule, grid framewire.Grid, v simVideo) *simWindow {
	return &simWindow{s: s, grid: grid, w: v.w, h: v.h, ctx: ctx, shown: -1, has: map[int]bool{}}
}

// show draws a frame. Under the lock.
func (w *simWindow) show(frame int) {
	if frame == w.shown {
		return
	}
	w.shown = frame
	w.shows = append(w.shows, simMark{at: time.Now(), frame: frame})
	w.told = append(w.told, simEvent{time.Now(), fmt.Sprintf("window: shows %d, the hand at %d", frame, w.hand)})
}

// to moves the hand: held while it drags, let go once it rests.
func (w *simWindow) to(frame int, held bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if held && !w.held {
		w.places = 0
	}
	w.held = held
	if frame == w.hand && w.shown == frame {
		return
	}
	moved := frame != w.hand || w.shown < 0
	w.hand = frame
	if !moved {
		return
	}
	if held {
		w.places++
	}
	moving := held && w.places > 1
	if w.has[frame] {
		// The window tells Go where the hand is all the same, so the
		// frames around it are read before the hand gets there.
		w.show(frame)
		w.sent++
		l := Hand{Grid: w.grid, Frame: frame, W: w.w, H: w.h, Moving: moving, Shown: frame, Has: true, Sent: Sent{Window: "sim", N: w.sent}}
		w.all.Go(func() { _, _ = w.s.Look(w.ctx, l) })
		return
	}
	w.all.Go(func() { w.look(frame, moving) })
}

// look asks for a place until it has its frame, or the hand left it.
func (w *simWindow) look(frame int, moving bool) {
	for {
		w.mu.Lock()
		if w.hand != frame {
			w.mu.Unlock()
			return
		}
		w.sent++
		l := Hand{Grid: w.grid, Frame: frame, W: w.w, H: w.h, Moving: moving, Shown: w.shown, Sent: Sent{Window: "sim", N: w.sent}}
		w.mu.Unlock()
		w.mu.Lock()
		w.told = append(w.told, simEvent{time.Now(), fmt.Sprintf("window: looks at %d, moving %v, shows %d", frame, moving, l.Shown)})
		w.mu.Unlock()
		seen, err := w.s.Look(w.ctx, l)
		w.mu.Lock()
		w.told = append(w.told, simEvent{time.Now(), fmt.Sprintf("window: the look at %d answered %d exact %v, %v", frame, seen.Frame, seen.Exact, err)})
		w.mu.Unlock()
		if err != nil {
			return
		}
		w.mu.Lock()
		w.has[seen.Frame] = true
		if w.shown < 0 || framesApart(seen.Frame, w.hand) < framesApart(w.shown, w.hand) {
			w.show(seen.Frame)
		}
		still := !w.held
		w.mu.Unlock()
		if seen.Exact {
			if still && w.s.AskNear(l) {
				near := w.s.Near(w.ctx, l, func(f int) bool {
					w.mu.Lock()
					defer w.mu.Unlock()
					return w.has[f]
				})
				w.mu.Lock()
				for _, n := range near {
					w.has[n.Frame] = true
				}
				w.mu.Unlock()
			}
			return
		}
	}
}

// simPath is where the hand is at each frame of the screen while it holds
// the playhead, from the first, where it pressed. It lets go at the last.
type simPath []int

// along makes a path from where the hand presses and how far it moves at
// each frame of the screen.
func along(from int, steps ...[2]int) simPath {
	p := simPath{from}
	for _, s := range steps {
		for range s[0] {
			p = append(p, p[len(p)-1]+s[1])
		}
	}
	return p
}

// wander is a hand nobody can guess: it speeds up and slows down, stops,
// turns round, and now and then lets go and presses somewhere else.
func wander(seed uint64, count int) []simPath {
	r := rand.New(rand.NewPCG(seed, 0x5eed))
	var out []simPath
	at := count / 4
	for range 3 {
		p := simPath{at}
		speed := 0
		for range 60 + r.IntN(180) {
			switch n := r.IntN(100); {
			case n < 10:
				speed = r.IntN(7) - 3
			case n < 16:
				speed = r.IntN(41) - 20
			case n < 20:
				speed = 0
			case n < 23:
				speed = -speed
			}
			at = min(max(at+speed, 0), count-1)
			p = append(p, at)
		}
		out = append(out, p)
		at = r.IntN(count)
	}
	return out
}

// simDrag is one drag simulated: what the picture did and what the
// decoder did.
type simDrag struct {
	video     simVideo
	start     time.Time
	ends      time.Time
	hands     []simMark
	shows     []simMark
	busy      []simSpan
	made      []simMark
	told      []simEvent
	standLong time.Duration
	standAt   time.Duration
	stands    []time.Duration
	idleLong  time.Duration
	stillLong time.Duration
	stillAt   time.Duration
	behind    int
	behindAt  time.Duration
	// farLong is the longest the picture stood still while it was more
	// than a span of key frames from the hand, with no allowance: a fast
	// drag turns at every step by the allowance of the second target, so
	// it is held to this one.
	farLong time.Duration
	farAt   time.Duration
}

// drag runs the paths one after the other on a new schedule, each held
// and then let go and rested on for half a second.
func drag(t *testing.T, grid framewire.Grid, v simVideo, paths ...simPath) simDrag {
	t.Helper()
	var d simDrag
	synctest.Test(t, func(t *testing.T) {
		dec := newSimDecoder(v)
		s := newSchedule(dec)
		ctx, cancel := context.WithCancel(context.Background())
		w := newSimWindow(ctx, s, grid, v)
		d.start = time.Now()
		for _, p := range paths {
			for i, f := range p {
				w.to(f, i < len(p)-1)
				d.hands = append(d.hands, simMark{time.Now(), f, i < len(p)-1})
				time.Sleep(simTick) //nolint:forbidigo // synctest bubble
			}
			w.to(p[len(p)-1], false)
			time.Sleep(time.Second / 2) //nolint:forbidigo // synctest bubble
		}
		d.ends = time.Now()
		cancel()
		s.Close()
		w.all.Wait()
		dec.streams.Wait()
		synctest.Wait()
		w.mu.Lock()
		d.shows = w.shows
		d.told = w.told
		w.mu.Unlock()
		dec.mu.Lock()
		d.busy = dec.busy
		d.made = dec.made
		d.told = append(d.told, dec.told...)
		dec.mu.Unlock()
		for _, h := range d.hands {
			d.told = append(d.told, simEvent{h.at, fmt.Sprintf("hand: at %d, held %v", h.frame, h.held)})
		}
		slices.SortStableFunc(d.told, func(a, b simEvent) int { return a.at.Compare(b.at) })
	})
	d.video = v
	d.measure()
	return d
}

// measure reads the targets off the drag, a millisecond at a time. The
// picture is held to the second and the fourth target while the hand
// drags, from its first move after it pressed until it lets go: where it
// pressed is a click, whose frame comes the way the frame after letting
// go does, by the third. A stand-still counts from when the frame that
// ends it could be there at the soonest, see floor, for a decoder that
// set out when the hand last turned round or jumped.
// floor is how long a stand-still that begins now, with the hand at h and
// the picture at sh, lasts whatever the schedule does: nothing where the
// decoder already made a frame nearer the hand than the picture, and
// otherwise as long as a decoder that starts this moment takes to the
// first frame nearer, from the key frame before the hand. A hand that
// turns round into a stretch of the video nobody read waits that long on
// any schedule, since frames are decoded only forward from a key frame.
func (d *simDrag) floor(now time.Time, h, sh int) time.Duration {
	far := framesApart(sh, h)
	if sh < 0 {
		far = h + 1
	}
	for _, m := range d.made {
		if m.at.After(now) {
			break
		}
		if framesApart(m.frame, h) < far {
			return 0
		}
	}
	key := d.video.keyOf(h)
	lo := max(key, h-far+1)
	return simOpen + time.Duration(lo-key)*simFrame + simScale
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (d *simDrag) measure() {
	hand, shown, busy := 0, 0, 0
	h, sh, pressed := -1, -1, -1
	var idle, farFor time.Duration
	var waiting, dragging bool
	// A stand-still began at waited, and what it waited for could not be
	// there before excused, see floor.
	var waited, excused time.Time
	// Where the hand arrived last, when, and whether its frame was shown
	// since.
	arrived, arrivedAt, exact := -1, d.start, true
	// When the hand last turned round, pressed, or moved too fast for
	// every frame on its way to be read, and the step after: where it is
	// then nobody could have known before.
	turned, way, fast := d.start, 0, false
	// caught says the picture changed since the hand pressed: until then
	// it is the frame of where the hand was before, as far off as that.
	caught := false
	var lastHand []int
	for at := time.Duration(0); d.start.Add(at).Before(d.ends); at += time.Millisecond {
		now := d.start.Add(at)
		handMoved := false
		for hand < len(d.hands) && !d.hands[hand].at.After(now) {
			handMoved = true
			m := d.hands[hand]
			switch {
			case !m.held:
				dragging, pressed = false, -1
			case pressed < 0:
				pressed, turned, way, caught = m.frame, now, 0, false
			case m.frame != pressed:
				dragging = true
			}
			if step := m.frame - h; h >= 0 && step != 0 {
				w := 1
				if step < 0 {
					w = -1
				}
				apart := framesApart(m.frame, h)
				if w != way || apart > simKeepUp || fast {
					turned = now
				}
				fast = apart > simKeepUp
				way = w
			}
			h = m.frame
			hand++
		}
		moved := false
		for shown < len(d.shows) && !d.shows[shown].at.After(now) {
			sh = d.shows[shown].frame
			shown++
			moved = true
		}
		for busy < len(d.busy) && !d.busy[busy].to.After(now) {
			busy++
		}
		working := busy < len(d.busy) && !d.busy[busy].from.After(now)
		lastHand = append(lastHand, h)
		if len(lastHand) > 50 {
			lastHand = lastHand[1:]
		}
		off := sh != h
		ended := func() {
			if since := now.Sub(waited.Add(max(0, excused.Sub(waited)))); waiting && since > 0 {
				d.stands = append(d.stands, since)
			}
		}
		switch {
		case !off || !dragging || framesApart(sh, h) <= simNear:
			ended()
			waiting = false
		case moved || !waiting:
			ended()
			waiting, waited, excused = true, now, turned.Add(d.floor(now, h, sh))
		case handMoved:
			// The hand went on to where nobody could know it would.
			excused = later(excused, turned.Add(d.floor(now, h, sh)))
		}
		if since := now.Sub(waited.Add(max(0, excused.Sub(waited)))); waiting && since > d.standLong {
			d.standLong, d.standAt = since, at
		}
		if far := dragging && framesApart(sh, h) > d.video.span; far && !moved {
			farFor += time.Millisecond
			if farFor > d.farLong {
				d.farLong, d.farAt = farFor, at
			}
		} else {
			farFor = 0
		}
		if off && !working {
			idle += time.Millisecond
		} else {
			idle = 0
		}
		d.idleLong = max(d.idleLong, idle)
		if moved {
			caught = true
		}
		if off && dragging && caught {
			lo, hi := slices.Min(lastHand), slices.Max(lastHand)
			if over := framesApart(sh, h) - (hi - lo); over > d.behind {
				d.behind, d.behindAt = over, at
			}
		}
		if h != arrived {
			arrived, arrivedAt, exact = h, now, false
		}
		if !exact {
			if took := now.Sub(arrivedAt); took > d.stillLong {
				d.stillLong, d.stillAt = took, at
			}
			exact = sh == h
		}
	}
}

// p95 is the stand-still that 95 % of them are no longer than.
func (d *simDrag) p95() time.Duration {
	if len(d.stands) == 0 {
		return 0
	}
	s := slices.Clone(d.stands)
	slices.Sort(s)
	return s[(len(s)*95)/100]
}

// misses says each target the drag missed: only the first, that the
// decoder never rests, unless all five are asked for.
func (d *simDrag) misses(all bool) []string {
	var out []string
	if d.idleLong > 2*time.Millisecond {
		out = append(out, fmt.Sprintf("the decoder rested %v while the picture was not the hand's", d.idleLong))
	}
	if !all {
		return out
	}
	if d.standLong > 50*time.Millisecond {
		out = append(out, fmt.Sprintf("the picture stood still %v while the hand was elsewhere", d.standLong))
	}
	if p := d.p95(); p > 25*time.Millisecond {
		out = append(out, fmt.Sprintf("95 %% of the stand-stills were up to %v", p))
	}
	if most := simOpen + time.Duration(d.video.span+1)*(simFrame+simScale); d.stillLong > most {
		out = append(out, fmt.Sprintf("the frame under the hand came %v after it stopped, more than %v", d.stillLong, most))
	}
	if d.behind > d.video.span {
		out = append(out, fmt.Sprintf("the picture was %d frames further behind than the hand moved in 50 ms, %v in", d.behind, d.behindAt))
	}
	if d.farLong > 50*time.Millisecond {
		out = append(out, fmt.Sprintf("the picture stood still %v while it was more than a span from the hand, %v in", d.farLong, d.farAt))
	}
	return out
}

// around is what happened in the drag in the 150 ms up to a moment.
func (d *simDrag) around(at time.Duration) string {
	var b strings.Builder
	for _, e := range d.told {
		if t := e.at.Sub(d.start); t >= at-300*time.Millisecond && t <= at+5*time.Millisecond {
			fmt.Fprintf(&b, "%9.3f ms  %s\n", float64(t)/float64(time.Millisecond), e.what)
		}
	}
	return b.String()
}

func (d *simDrag) String() string {
	return fmt.Sprintf("longest stand-still %v, 95 %% within %v, decoder rested %v at most, frame after the hand stopped %v at most, %d frames behind at most, %v still while a span behind",
		d.standLong, d.p95(), d.idleLong, d.stillLong, d.behind, d.farLong)
}

// Every drag, slow or fast, either way, turning round or not, keeps the
// picture near the hand, see the targets above, over a video with a key
// frame every second, which is what Tim's videos have. Over videos whose
// key frames are further apart, or come unevenly, the schedule knows only
// the key frames the decoder was seen to start from, and guesses the
// rest. There the decoder is held to never resting, and the other four
// targets are said, not held: a guess that is wrong costs the frames
// between where the decoder starts and where it was asked to.
func TestADragKeepsThePictureNearTheHand(t *testing.T) {
	grid := framewire.Grid{Num: 25, Den: 1, Count: 30000}
	drags := []struct {
		name  string
		paths []simPath
	}{
		{"slowly to the right", []simPath{along(1000, [2]int{120, 1})}},
		{"slowly to the left", []simPath{along(1000, [2]int{120, -1})}},
		{"to the right as fast as the decoder", []simPath{along(1000, [2]int{90, 4})}},
		{"to the right a little faster than the decoder", []simPath{along(1000, [2]int{90, 6})}},
		{"to the left at two frames a step", []simPath{along(1000, [2]int{90, -2})}},
		{"fast to the right", []simPath{along(1000, [2]int{60, 30})}},
		{"fast to the left", []simPath{along(5000, [2]int{60, -30})}},
		{"slowly to the right and back", []simPath{along(1000, [2]int{60, 1}, [2]int{60, -1})}},
		{"fast to the right and back", []simPath{along(1000, [2]int{30, 20}, [2]int{30, -20})}},
		{"slowly, then fast, then slowly", []simPath{along(1000, [2]int{40, 1}, [2]int{20, 25}, [2]int{40, 1}, [2]int{20, -25}, [2]int{40, -1})}},
		{"a click far off, then slowly on", []simPath{{1000}, along(9000, [2]int{60, 1})}},
	}
	for seed := range uint64(12) {
		drags = append(drags, struct {
			name  string
			paths []simPath
		}{fmt.Sprintf("wandering, seed %d", seed), wander(seed, grid.Count)})
	}
	videos := []struct {
		simVideo
		all bool
	}{
		{everyKey("a key frame a second", 25), true},
		{everyKey("a key frame every two seconds", 50), false},
		{everyKey("a key frame every ten seconds", 250), false},
		{unevenKeys(), false},
	}
	for _, v := range videos {
		for _, c := range drags {
			t.Run(v.name+", "+c.name, func(t *testing.T) {
				t.Parallel()
				d := drag(t, grid, v.simVideo, c.paths...)
				t.Log(d.String())
				for _, m := range d.misses(v.all) {
					t.Error(m)
				}
				if t.Failed() {
					t.Logf("the longest stand-still a span from the hand, %v in:\n%s", d.farAt, d.around(d.farAt))
					t.Logf("the longest stand-still, %v in:\n%s", d.standAt, d.around(d.standAt))
					t.Logf("the slowest frame under a hand that stopped, %v in:\n%s", d.stillAt, d.around(d.stillAt))
					t.Logf("the furthest behind, %v in:\n%s", d.behindAt, d.around(d.behindAt))
				}
			})
		}
	}
}

// A video preview of any size lets the decoder rest once the hand does:
// what is read around the hand fits in the frames the schedule keeps. At
// 2560 by 1440 and more, fewer frames fit than were read around the hand,
// each run let go of what the one before read, and the decoder started
// again for good, with nothing read ahead.
func TestTheDecoderRestsAtAnySizeOfTheVideoPreview(t *testing.T) {
	grid := framewire.Grid{Num: 25, Den: 1, Count: 30000}
	for _, size := range [][2]int{{1012, 570}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			v := everyKey("a key frame a second", 25)
			v.w, v.h = size[0], size[1]
			d := drag(t, grid, v, simPath{1000}, along(2000, [2]int{60, -1}), along(3000, [2]int{60, 1}))
			for _, m := range d.misses(false) {
				t.Error(m)
			}
			quiet := d.ends.Add(-time.Second / 4)
			late := 0
			for _, m := range d.made {
				if m.at.After(quiet) {
					late++
				}
			}
			if late > 0 {
				t.Errorf("the decoder made %d frames in the last 250 ms of half a second the hand rested", late)
			}
		})
	}
}
