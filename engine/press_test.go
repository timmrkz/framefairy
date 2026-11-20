package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"framefairy/internal/framewire"
)

// Presses of the arrow keys, simulated the way the drags are, see
// drag_test.go: the decoder of Tim's Mac on the fake clock of a
// testing/synctest bubble, and a window that keeps frames, tells and
// asks the way app.ts does. A hand at rest steps from word to word with
// Shift and an arrow, and from frame to frame with an arrow alone, with a
// pause between two presses that a person makes to look.

// pressKept is how many frames the window keeps, KEPT_FRAMES in app.ts.
const pressKept = 64

// pressWindow is the window of a press walk.
type pressWindow struct {
	s     *Schedule
	grid  framewire.Grid
	w, h  int
	ctx   context.Context
	words []int
	all   sync.WaitGroup

	mu sync.Mutex
	// kept are the frames the window holds, the one used last at the end.
	kept []int
	// ever are the frames the window was ever sent, to count the ones
	// sent again.
	ever  map[int]bool
	sent  uint64
	hand  int
	shown int
	// waits is how long each press waited for its frame, and missed the
	// presses whose frame the window did not hold.
	waits  []time.Duration
	missed []int
	frames int
	again  int
	asked  int
	nearAt int
}

func (w *pressWindow) keep(f int) {
	w.frames++
	if w.ever[f] {
		w.again++
	}
	w.ever[f] = true
	w.use(f)
}

// use makes a kept frame the one used last, or keeps a new one, letting
// go of the one used longest ago once there are more than pressKept.
func (w *pressWindow) use(f int) {
	if i := slices.Index(w.kept, f); i >= 0 {
		w.kept = slices.Delete(w.kept, i, i+1)
	}
	w.kept = append(w.kept, f)
	if len(w.kept) > pressKept {
		w.kept = w.kept[1:]
	}
}

func (w *pressWindow) has(f int) bool { return slices.Contains(w.kept, f) }

func (w *pressWindow) next() Sent {
	w.sent++
	return Sent{Window: "sim", N: w.sent}
}

// press moves the playhead to a frame, the way a key does in the app:
// the words beside the new place handed over to be read ahead, see
// ClipTimeline's onahead, and then the kept frame shown at once and told,
// or the frame asked for, see FrameQueue. The app's log shows them come in
// that order, and the walk keeps to it.
func (w *pressWindow) press(f int) {
	w.mu.Lock()
	w.hand = f
	w.ahead()
	w.mu.Unlock()
	synctest.Wait()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.has(f) {
		w.use(f)
		w.shown = f
		w.waits = append(w.waits, 0)
		l := Hand{Grid: w.grid, Frame: f, W: w.w, H: w.h, Shown: f, Has: true, Sent: w.next()}
		w.all.Go(func() { _, _ = w.s.Look(w.ctx, l) })
		return
	}
	w.missed = append(w.missed, f)
	l := Hand{Grid: w.grid, Frame: f, W: w.w, H: w.h, Shown: w.shown, Sent: w.next()}
	began := time.Now()
	w.all.Go(func() { w.look(l, began) })
}

func (w *pressWindow) look(l Hand, began time.Time) {
	w.mu.Lock()
	w.asked++
	w.mu.Unlock()
	seen, err := w.s.Look(w.ctx, l)
	if err != nil {
		return
	}
	w.mu.Lock()
	w.keep(seen.Frame)
	if w.hand == l.Frame {
		w.shown = seen.Frame
		w.waits = append(w.waits, time.Since(began)) //nolint:forbidigo // synctest bubble
	}
	w.mu.Unlock()
	if seen.Exact && w.s.HasNear(l) {
		w.mu.Lock()
		w.nearAt++
		w.mu.Unlock()
		near := w.s.Near(w.ctx, l, func(f int) bool {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.has(f) && f >= l.Frame-16 && f <= l.Frame+8
		})
		w.mu.Lock()
		for _, n := range near {
			w.keep(n.Frame)
		}
		w.mu.Unlock()
	}
}

// ahead hands over the first frames of the words either side of the
// playhead, see ClipTimeline's onahead. Under the lock.
func (w *pressWindow) ahead() {
	var frames []int
	if i := slices.IndexFunc(w.words, func(f int) bool { return f > w.hand }); i >= 0 {
		frames = append(frames, w.words[i])
	}
	for i := len(w.words) - 1; i >= 0; i-- {
		if w.words[i] < w.hand {
			frames = append(frames, w.words[i])
			break
		}
	}
	a := Ahead{Grid: w.grid, W: w.w, H: w.h, Frames: frames, Has: slices.Clone(w.kept), Sent: w.next()}
	w.all.Go(func() {
		seen, err := w.s.Ahead(w.ctx, a)
		if err != nil {
			return
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		// The words' frames it holds already are made the ones used last
		// before the frames that came are kept, see app.ts.
		for _, f := range frames {
			for g := f - 1; g <= f+lookAhead; g++ {
				if w.has(g) {
					w.use(g)
				}
			}
		}
		for _, n := range seen {
			w.keep(n.Frame)
		}
	})
}

// pressWalk is what a press walk did.
type pressWalk struct {
	presses, asked         int
	decoded, frames, again int
	// missed are the frames pressed to that the window did not hold, the
	// first press left out.
	missed []int
	// missedWords are the ones of them a word key went to.
	missedWords []int
	waits       []time.Duration
	told        []simEvent
	began       time.Time
}

// log writes what the decoder did, press by press.
func (p pressWalk) log(t *testing.T) {
	t.Helper()
	var b strings.Builder
	for _, e := range p.told {
		if !strings.Contains(e.what, "made") {
			fmt.Fprintf(&b, "%6d ms %s\n", e.at.Sub(p.began).Milliseconds(), e.what)
		}
	}
	t.Log("\n" + b.String())
}

func (p pressWalk) String() string {
	slices.Sort(p.waits)
	most := time.Duration(0)
	if len(p.waits) > 0 {
		most = p.waits[len(p.waits)-1]
	}
	return fmt.Sprintf("%d presses, %d found their frame missing %v, %d frames decoded, %d sent to the window, %d of them again, the longest wait %v",
		p.presses, len(p.missed), p.missed, p.decoded, p.frames, p.again, most)
}

// walkPresses presses the keys of a walk over words on a new schedule,
// from the first frame, resting rest between two presses.
func walkPresses(t *testing.T, v simVideo, words []int, keys string, rest time.Duration) pressWalk {
	t.Helper()
	var p pressWalk
	grid := framewire.Grid{Num: 25, Den: 1, Count: 30000}
	synctest.Test(t, func(t *testing.T) {
		dec := newSimDecoder(v)
		s := newSchedule(dec)
		ctx, cancel := context.WithCancel(context.Background())
		w := &pressWindow{s: s, grid: grid, w: v.w, h: v.h, ctx: ctx, words: words, ever: map[int]bool{}, shown: -1, hand: -1}
		began := time.Now()
		at := words[0]
		w.press(at)
		dec.tell("press %d", at)
		time.Sleep(rest) //nolint:forbidigo // synctest bubble
		for _, k := range keys {
			switch k {
			case 'W':
				i := slices.IndexFunc(words, func(f int) bool { return f > at })
				at = words[i]
			case 'w':
				for i := len(words) - 1; i >= 0; i-- {
					if words[i] < at {
						at = words[i]
						break
					}
				}
			case 'R':
				at++
			case 'L':
				at--
			default:
				t.Fatalf("no key %q", k)
			}
			w.mu.Lock()
			missed := len(w.missed)
			w.mu.Unlock()
			w.press(at)
			dec.tell("press %c to %d", k, at)
			w.mu.Lock()
			if len(w.missed) > missed && (k == 'W' || k == 'w') {
				p.missedWords = append(p.missedWords, at)
			}
			w.mu.Unlock()
			time.Sleep(rest) //nolint:forbidigo // synctest bubble
		}
		cancel()
		s.Close()
		w.all.Wait()
		dec.streams.Wait()
		synctest.Wait()
		w.mu.Lock()
		p = pressWalk{missedWords: p.missedWords, presses: len(keys), missed: w.missed[1:], asked: w.asked, frames: w.frames, again: w.again, waits: w.waits[1:], began: began}
		w.mu.Unlock()
		dec.mu.Lock()
		p.decoded = dec.decoded
		p.told = dec.told
		dec.mu.Unlock()
	})
	return p
}

// The keys of the speed walk, speed.mjs: Shift and the right arrow 8
// times, the left arrow 3, the right arrow 3, Shift and the left arrow 7,
// and Shift and the right arrow then the left arrow 4 times.
const pressSpeedWalk = "WWWWWWWWLLLRRRwwwwwwwWLWLWLWL"

// The words of the cloud's walk video, by their first frame.
var pressWords = []int{0, 18, 25, 35, 75, 84, 93, 104, 115, 130, 142, 155, 170}

// A word key pressed finds the frame of its word in the window, at any
// pace a person reads at: the words beside are read first while the hand
// rests and handed over as soon as they are read, and the window keeps
// them. Before, a word key pressed every 150 ms found its frame missing
// at 9 words in 21: the words came only once the frames behind the hand
// were read too, a read of them could wait on a stream still decoding
// from a key frame far back, and the window let go of them for the
// frames that came after.
func TestAWordKeyFindsItsFrameInTheWindow(t *testing.T) {
	for _, rest := range []time.Duration{150 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond} {
		t.Run(rest.String(), func(t *testing.T) {
			t.Parallel()
			p := walkPresses(t, everyKey("a key frame every 2 s", 50), pressWords, pressSpeedWalk, rest)
			if len(p.missedWords) > 0 {
				p.log(t)
				t.Errorf("the word keys to frames %v found them missing: %v", p.missedWords, p)
			}
		})
	}
}

// The decoder reads each frame a press walk wants about once, and the
// window is sent each about once. For the 29 presses of the speed walk
// it decoded 587 frames and sent 126, 41 of them again, and now 239 and
// 74, 5 of them again: a read of the words ahead was stopped and started
// again with each press of a key, a new stream that seeks, the frames
// behind the hand were decoded again from the key frame before them
// although most of them were kept, here or in the window, and the window
// let go of frames it was sent for the next ones.
func TestAPressWalkDecodesEachFrameAboutOnce(t *testing.T) {
	p := walkPresses(t, everyKey("a key frame every 2 s", 50), pressWords, pressSpeedWalk, 300*time.Millisecond)
	if p.decoded > 260 || p.again > 10 {
		p.log(t)
		t.Errorf("the decoder decoded %d frames and sent %d again, want 260 and 10 at most: %v", p.decoded, p.again, p)
	}
}
