package framewire

import (
	"fmt"
	"math"
	"strconv"
)

// Hair is how far a frame may begin from its place on the grid of a rate
// and still be on it: more than ffmpeg's rounding of a time to the ticks
// of a stream, half of Matroska's millisecond or of a phone's 1/600 s,
// and much less than a frame. The engine cuts a short by it and the
// episode's decoder shows the video preview's frames by it.
const Hair = 0.001

// Grid is the frames of the short, the ones the video preview shows: one
// every Den/Num seconds from Start, Count of them. Times are in seconds
// after the start of the file, the way ffmpeg's -ss and every time in a
// clip count, see engine.SourceInfo.Grid, which makes it.
//
// Frame r of the grid is the frame of the file that holds its moment, the
// last to begin at or before it, give or take Hair, and the file's first
// frame before the picture begins. That is the frame the render takes
// there, see byTimes in engine/render.go. For a file whose frames come at
// even times on its own rate, the grid is its frames. For one whose frames
// come at uneven times, it is the short's rate, and a frame of the file
// may stand for two moments of the grid or for none.
type Grid struct {
	Start float64
	Num   int
	Den   int
	Count int
}

// maxFrames is the most frames a grid may have: a day at 240 a second.
const maxFrames = 24 * 3600 * 240

// Valid tells whether the grid has frames that can be counted.
func (g Grid) Valid() bool {
	return g.Num > 0 && g.Den > 0 && g.Count > 0 && g.Count <= maxFrames && g.Start >= 0 &&
		!math.IsInf(g.Start, 0) && !math.IsNaN(g.Start)
}

// At is when frame r begins.
func (g Grid) At(r int) float64 {
	return g.Start + float64(r)*float64(g.Den)/float64(g.Num)
}

// Holding is the frame whose moment is the last at or before t, give or
// take Hair, and the first or the last frame outside the grid.
func (g Grid) Holding(t float64) int {
	r := g.frames(g.micros(t) + hairMicros)
	return int(min(max(r, 0), int64(g.Count-1)))
}

// FirstShown is the first frame a picture of the file that begins at
// begins is shown for: the first moment of the grid at or after it, give
// or take Hair. It is shown until the next picture's FirstShown, and for
// none of the grid's frames where that is the same. It is worked out in
// whole microseconds, the way the render's fps filter takes it, so a
// picture that begins exactly a Hair after a moment holds it here as it
// does in the short.
func (g Grid) FirstShown(begins float64) int {
	k := -g.frames(hairMicros - g.micros(begins))
	return int(min(max(k, -1), maxFrames+1))
}

// hairMicros is Hair in microseconds.
const hairMicros = 1000

// micros is how far after the grid's start t is, in whole microseconds,
// each rounded on its own the way the render's times are.
func (g Grid) micros(t float64) int64 {
	if math.IsNaN(t) {
		return 0
	}
	t = min(max(t, -1e6), 1e7)
	return int64(math.Round(t*1e6)) - int64(math.Round(g.Start*1e6))
}

// frames is how many frames of the grid fit in us microseconds, rounded
// down.
func (g Grid) frames(us int64) int64 {
	a, b := us*int64(g.Num), int64(g.Den)*1_000_000
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

// OpenRequest is the request that opens a cursor at frame of the grid,
// for frames width by height.
func OpenRequest(cursor uint32, frame, width, height int, g Grid) string {
	return fmt.Sprintf("open %d %d %d %d %s %d %d %d", cursor, frame, width, height,
		strconv.FormatFloat(g.Start, 'f', -1, 64), g.Num, g.Den, g.Count)
}

// ReadOpen reads an open request, split into its words, the other way:
// the cursor, the frame, the size and the grid, and whether they are an
// open the decoder can take.
func ReadOpen(f []string) (cursor uint32, frame, width, height int, g Grid, ok bool) {
	if len(f) != 9 || f[0] != "open" {
		return 0, 0, 0, 0, Grid{}, false
	}
	id, err := strconv.ParseUint(f[1], 10, 32)
	ints := [6]int{}
	for i, at := range []int{2, 3, 4, 6, 7, 8} {
		if err == nil {
			ints[i], err = strconv.Atoi(f[at])
		}
	}
	if err == nil {
		g.Start, err = strconv.ParseFloat(f[5], 64)
	}
	frame, width, height = ints[0], ints[1], ints[2]
	g.Num, g.Den, g.Count = ints[3], ints[4], ints[5]
	if err != nil || !g.Valid() || frame < 0 || frame >= g.Count ||
		width < 2 || height < 2 || width > 7680 || height > 4320 || width%2 != 0 || height%2 != 0 {
		return 0, 0, 0, 0, Grid{}, false
	}
	return uint32(id), frame, width, height, g, true
}
