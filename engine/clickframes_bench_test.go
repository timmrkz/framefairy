package engine

import (
	"context"
	"sync"
	"testing"

	"framefairy/internal/framewire"
)

// makingSource is a decoder that reads every frame into a buffer given
// back to it, or into new memory when none is, writing every byte the way
// a decoder writes a frame.
type makingSource struct {
	mu   sync.Mutex
	free [][]byte
}

func (d *makingSource) Stream(ctx context.Context, grid framewire.Grid, from float64, width, height int, key bool,
	skip func() float64, got func(at float64, frame []byte) error) error {
	for f := grid.Holding(from); f < grid.Count; f++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		picture := d.buffer(width * height * 4)
		for i := range picture {
			picture[i] = byte(f)
		}
		if err := got(grid.At(f), picture); err != nil {
			return err
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (d *makingSource) buffer(size int) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n := len(d.free); n > 0 && len(d.free[n-1]) == size {
		b := d.free[n-1]
		d.free = d.free[:n-1]
		return b
	}
	return make([]byte, size)
}

func (d *makingSource) Give(frame []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.free = append(d.free, frame)
}

// BenchmarkClicksAlongAVideoPreviewOf1600By900 is a hand clicking from
// place to place along a long video, each place two frames decoded, its
// frame and the one before, and the window writing the frame out and
// giving it back. The frames the schedule lets go of go back to the
// decoder, which reads the next ones into them, see keptFrames.
func BenchmarkClicksAlongAVideoPreviewOf1600By900(b *testing.B) {
	grid := framewire.Grid{Num: 25, Den: 1, Count: 1 << 20}
	s := newSchedule(&makingSource{})
	b.Cleanup(s.Close)
	ctx := context.Background()
	b.ReportAllocs()
	frame := 0
	for b.Loop() {
		frame += 100
		seen, err := s.Look(ctx, Hand{Grid: grid, Frame: frame, W: 1600, H: 900, Shown: -1})
		if err != nil {
			b.Fatal(err)
		}
		// Given back the way the app does, once written.
		if g, ok := any(s).(interface{ Give([]byte) }); ok {
			g.Give(seen.Picture)
		}
	}
}
