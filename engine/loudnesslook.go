package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"sync"

	"framefairy/engine/timed"
	"framefairy/internal/wake"
)

// LoudnessSchedule answers where the clip timeline looks with the loudness
// of its view, the way Schedule answers where the hand is with a frame,
// through the same looks: the window says the view and what it holds, and
// the answer goes out the moment the measuring brings readings of the view
// the window does not hold. Before, what was measured went to the disk,
// the window heard it changed up to a quarter of a second later, and read
// the whole loudness again, 3 MB for four hours, issue #968, see
// docs/decisions/2026-11-21-the-waveform-keeps-up-with-the-hand.md.
//
// It holds the loudness the way Loudness lays it out, the readings and the
// levels for zooming out, and keeps the levels right as each reading comes,
// so an answer is a copy of the bytes of the view in each level, and never
// a pass over the whole video. Its rule, where to measure, is the
// measuring's own, see MeasureLevels: the view first, then the rest.
type LoudnessSchedule struct {
	mu      sync.Mutex
	looking looks[View, []byte]
	view    View
	// held is the loudness as Loudness lays it out, count readings, and
	// levels are its levels, each a part of held.
	held   []byte
	count  int
	levels [][]byte
	// got says which readings were measured here, and version counts the
	// changes: each chunk of loudnessChunk readings keeps the version it
	// last changed at, so a look learns at once whether its view changed
	// since what its window holds.
	got     []bool
	version uint32
	chunks  []uint32
	// gotIn is how many readings of each chunk were measured here.
	gotIn []int
	// told is the version each chunk had when the window, toldTo, was
	// last sent it. A window's version alone cannot say what it holds: an
	// answer carries the view of its look, not every chunk measured
	// before it, so a view measured while the window looked elsewhere was
	// told nothing was new.
	told   []uint32
	toldTo string
	// started says Start has laid out the loudness: a look before it
	// waits. ended says the measuring has ended: a look is answered at
	// once, with what there is.
	started bool
	ended   bool
	// waits wakes whoever waits whenever a look starts to wait, so a test
	// goes on once the schedule has it. Only the tests wait on it.
	waits wake.Wake
}

// loudnessChunk is how many readings share a version, 10 s of them.
const loudnessChunk = 1000

// View is where the clip timeline looks, from and to in seconds, and what
// its window holds: the version of the loudness it was last answered
// with, 0 for none since it read the loudness whole, and how many
// readings that loudness has. Which chunks the window was sent the
// schedule keeps itself, by Sent.Window, see LoudnessSchedule.told.
type View struct {
	From, To float64
	Has      uint32
	Count    int
	Sent     Sent
}

// ErrNothingNew answers a view whose window holds all there is of it.
var ErrNothingNew = errors.New("the window holds the loudness of the view")

// NewLoudnessSchedule is the schedule of a measuring about to start. Looks
// wait until it starts, see Start.
func NewLoudnessSchedule() *LoudnessSchedule { return &LoudnessSchedule{} }

// Start holds a video's loudness as it is when the measuring starts, base
// in the bytes Loudness makes, for a video of count readings at least, as
// the probe says, and answers the looks that wait with it.
func (s *LoudnessSchedule) Start(base []byte, count int) {
	var readings []byte
	if len(base) >= loudnessHead {
		n := int(binary.LittleEndian.Uint32(base))
		if loudnessHead+n <= len(base) {
			readings = base[loudnessHead : loudnessHead+n]
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grow(max(count, len(readings)), readings)
	s.started = true
	s.version++
	for c := range s.chunks {
		s.chunks[c] = s.version
	}
	s.answer()
}

// grow lays the loudness out again for count readings, the first of them
// readings, and the levels worked out from them. Under the lock, or before
// the schedule is shared.
func (s *LoudnessSchedule) grow(count int, readings []byte) {
	size := loudnessHead
	for k := count; k > 0; k = (k + 1) / 2 {
		size += k
		if k == 1 {
			break
		}
	}
	held := make([]byte, size)
	binary.LittleEndian.PutUint32(held, uint32(count))
	binary.LittleEndian.PutUint16(held[4:], uint16(math.Round(timed.FrameSeconds*1e6)))
	floor := int8(loudnessFloor)
	held[6] = byte(floor)
	held[7] = loudnessSteps
	var levels [][]byte
	at := loudnessHead
	for k := count; k > 0; k = (k + 1) / 2 {
		levels = append(levels, held[at:at+k])
		at += k
		if k == 1 {
			break
		}
	}
	if count > 0 {
		copy(levels[0], readings)
		for k := 1; k < len(levels); k++ {
			for i := range levels[k] {
				levels[k][i] = pairMax(levels[k-1], i)
			}
		}
	}
	got := make([]bool, count)
	copy(got, s.got)
	chunks := make([]uint32, (count+loudnessChunk-1)/loudnessChunk)
	copy(chunks, s.chunks)
	gotIn := make([]int, len(chunks))
	copy(gotIn, s.gotIn)
	told := make([]uint32, len(chunks))
	copy(told, s.told)
	s.held, s.count, s.levels, s.got, s.chunks, s.gotIn, s.told = held, count, levels, got, chunks, gotIn, told
}

// pairMax is the loudest of the two readings of level under i of the level
// above it, or the one alone at the end.
func pairMax(level []byte, i int) byte {
	v := level[2*i]
	if 2*i+1 < len(level) {
		v = max(v, level[2*i+1])
	}
	return v
}

// View is where the clip timeline looks, for the measuring to go there
// first.
func (s *LoudnessSchedule) View() (from, to float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view.From, s.view.To
}

// Put takes readings measured from reading first on, in dB, and answers
// the looks whose view they reach.
func (s *LoudnessSchedule) Put(first int, frames []float32) {
	if len(frames) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	if end := first + len(frames); end > s.count {
		// The sound runs a packet past what the probe said.
		s.grow(end, s.levels0())
	}
	s.version++
	for i, f := range frames {
		n := first + i
		if !s.got[n] {
			s.got[n] = true
			s.gotIn[n/loudnessChunk]++
		}
		s.set(n, loudnessByte(f))
	}
	for c := first / loudnessChunk; c <= (first+len(frames)-1)/loudnessChunk; c++ {
		s.chunks[c] = s.version
	}
	s.answer()
}

func (s *LoudnessSchedule) levels0() []byte {
	if len(s.levels) == 0 {
		return nil
	}
	return s.levels[0]
}

// set puts a reading and the levels above it right, as far up as they
// change. Under the lock.
func (s *LoudnessSchedule) set(n int, b byte) {
	s.levels[0][n] = b
	for k := 1; k < len(s.levels); k++ {
		n >>= 1
		v := pairMax(s.levels[k-1], n)
		if s.levels[k][n] == v {
			return
		}
		s.levels[k][n] = v
	}
}

// Ended says the measuring has ended, however it ended, before it started
// too: a look that waits, and every look from now on, is answered at once
// with what there is, or with ErrNothingNew, never left to wait for
// readings no measuring will bring.
func (s *LoudnessSchedule) Ended() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
	if s.started {
		s.answer()
	}
	s.looking.fail(func(View) error { return ErrNothingNew })
}

// Look answers a view with the loudness of it the window does not hold,
// at once when it is measured, or as soon as the measuring brings some of
// it, see piece. A view whose window holds all there is of it is answered
// with ErrNothingNew, and one a newer view took the place of with
// ErrOvertaken.
func (s *LoudnessSchedule) Look(ctx context.Context, v View) ([]byte, error) {
	s.mu.Lock()
	if !s.looking.newest(v.Sent) {
		s.mu.Unlock()
		return nil, ErrOvertaken
	}
	s.looking.overtake()
	s.view = v
	s.seen(v)
	if s.ended && !s.started {
		s.mu.Unlock()
		return nil, ErrNothingNew
	}
	if !s.started {
		w := s.looking.add(v)
		s.waits.WakeLocked()
		s.mu.Unlock()
		p, _, err := s.looking.wait(ctx, &s.mu, w)
		return p, err
	}
	if s.news(v) {
		p := s.piece(v)
		s.mu.Unlock()
		return p, nil
	}
	if s.ended || s.measured(v) {
		s.mu.Unlock()
		return nil, ErrNothingNew
	}
	w := s.looking.add(v)
	s.waits.WakeLocked()
	s.mu.Unlock()
	p, _, err := s.looking.wait(ctx, &s.mu, w)
	return p, err
}

// answer answers the looks whose view changed since what their window
// holds. Under the lock.
func (s *LoudnessSchedule) answer() {
	if !s.started {
		return
	}
	s.looking.answer(func(v View) ([]byte, bool) {
		if !s.news(v) {
			return nil, false
		}
		return s.piece(v), true
	})
}

// seen takes what a look says of its window: a window of another name, or
// one that read the loudness whole since, Has 0, holds nothing it was
// sent here. Under the lock.
func (s *LoudnessSchedule) seen(v View) {
	if v.Sent.Window != s.toldTo || v.Has == 0 {
		s.toldTo = v.Sent.Window
		clear(s.told)
	}
}

// span is the readings a view shows, from and to, the second not
// included. Under the lock.
func (s *LoudnessSchedule) span(v View) (int, int) {
	first := max(int(math.Floor(v.From/timed.FrameSeconds)), 0)
	last := min(int(math.Ceil(v.To/timed.FrameSeconds)), s.count)
	return min(first, last), last
}

// news says whether the window of a view lacks readings of it: it holds a
// loudness of another length, or a chunk of the view changed since it was
// last sent it. Under the lock.
func (s *LoudnessSchedule) news(v View) bool {
	if v.Count != s.count {
		return true
	}
	first, last := s.span(v)
	if first >= last {
		return false
	}
	for c := first / loudnessChunk; c <= (last-1)/loudnessChunk; c++ {
		if s.chunks[c] > s.told[c] {
			return true
		}
	}
	return false
}

// measured says whether every reading of a view was measured here. Under
// the lock.
func (s *LoudnessSchedule) measured(v View) bool {
	first, last := s.span(v)
	for n := first; n < last; {
		c := n / loudnessChunk
		end := min((c+1)*loudnessChunk, s.count)
		if n == c*loudnessChunk && end <= last {
			// A chunk in the view whole is counted, not read.
			if s.gotIn[c] < end-n {
				return false
			}
			n = end
			continue
		}
		if !s.got[n] {
			return false
		}
		n++
	}
	return true
}

// piece is what a look is answered with: the head of the loudness, then
// the version, the first reading and the one after the last, as four
// bytes each, little end first, and a byte that says whether the view is
// measured whole, so the window asks again only when it is not, three
// bytes of nothing, and then the bytes of those readings in each level, one
// level after another. A window that holds a loudness of another length
// is sent all of it, from the first reading to the last, which is the
// loudness as Loudness lays it out after the head. Under the lock.
func (s *LoudnessSchedule) piece(v View) []byte {
	first, last := s.span(v)
	if v.Count != s.count {
		first, last = 0, s.count
	}
	size := loudnessPieceHead
	if last > first {
		for k := range s.levels {
			size += (last-1)>>k - first>>k + 1
		}
	}
	out := make([]byte, size)
	copy(out, s.held[:loudnessHead])
	binary.LittleEndian.PutUint32(out[8:], s.version)
	binary.LittleEndian.PutUint32(out[12:], uint32(first))
	binary.LittleEndian.PutUint32(out[16:], uint32(last))
	if s.measured(v) {
		out[20] = 1
	}
	at := loudnessPieceHead
	if last > first {
		for k, level := range s.levels {
			at += copy(out[at:], level[first>>k:(last-1)>>k+1])
		}
		for c := first / loudnessChunk; c <= (last-1)/loudnessChunk; c++ {
			s.told[c] = s.chunks[c]
		}
	}
	return out
}

// loudnessPieceHead is how long the head of a piece is, see piece.
const loudnessPieceHead = 24
