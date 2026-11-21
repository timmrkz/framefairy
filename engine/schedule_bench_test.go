package engine

import (
	"context"
	"testing"

	"framefairy/internal/framewire"
)

// What the schedule itself costs a press, with every frame it asks for
// kept: the planner, the kept frames and the answers, and no decoder. A
// stream it starts waits until it is stopped, so a benchmark that
// started one counts the start and the stop.

// idleSource is a decoder whose streams make no frame.
type idleSource struct{}

func (idleSource) Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
	skip func() float64, got func(at float64, frame []byte) error) error {
	<-ctx.Done()
	return ctx.Err()
}

func (idleSource) Give([]byte) {}

// benchSchedule is a schedule that keeps frames 0 to 61 of a video
// preview of 1012 by 570, all of them one buffer.
func benchSchedule(b *testing.B) (*Schedule, framewire.Grid) {
	b.Helper()
	grid := framewire.Grid{Num: 25, Den: 1, Count: 30000}
	s := newSchedule(idleSource{})
	picture := make([]byte, 1012*570*4)
	s.mu.Lock()
	for f := range 62 {
		s.kept.put(keptKey{grid: grid, w: 1012, h: 570, frame: f}, grid.At(f), picture)
	}
	s.mu.Unlock()
	b.Cleanup(s.Close)
	return s, grid
}

// A press of an arrow key onto a frame the window holds: the hand told,
// and the words beside handed over to be read ahead, every frame kept.
func BenchmarkAPressOntoAFrameTheWindowHolds(b *testing.B) {
	s, grid := benchSchedule(b)
	ctx := context.Background()
	var n uint64
	for i := 0; b.Loop(); i++ {
		f := 30 + i%2
		n++
		_, _ = s.Look(ctx, Hand{Grid: grid, Frame: f, W: 1012, H: 570, Shown: f, Has: true, Sent: Sent{Window: "w", N: n}})
		n++
		_, _ = s.Ahead(ctx, Ahead{Grid: grid, W: 1012, H: 570, Frames: []int{40, 20}, Sent: Sent{Window: "w", N: n}})
	}
}

// A look at a frame the schedule keeps and the window does not, and the
// frames around it handed over after.
func BenchmarkALookAtAKeptFrameAndItsNearFrames(b *testing.B) {
	s, grid := benchSchedule(b)
	ctx := context.Background()
	var n uint64
	for i := 0; b.Loop(); i++ {
		f := 30 + i%2
		n++
		l := Hand{Grid: grid, Frame: f, W: 1012, H: 570, Shown: f - 1, Sent: Sent{Window: "w", N: n}}
		if _, err := s.Look(ctx, l); err != nil {
			b.Fatal(err)
		}
		_ = s.HasNear(l)
		_ = s.Near(ctx, l, func(int) bool { return false })
	}
}
