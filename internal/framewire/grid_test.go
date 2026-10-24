package framewire

import (
	"strings"
	"testing"
)

// A frame of the file is shown from the first moment of the grid at or
// after it begins, give or take Hair, and a moment is held by the last
// frame that began by then.
func TestAGridShowsEachMomentTheFrameThatHoldsIt(t *testing.T) {
	ntsc := Grid{Start: 0.5, Num: 30000, Den: 1001, Count: 100}
	for r := range 100 {
		at := ntsc.At(r)
		if got := ntsc.FirstShown(at); got != r {
			t.Fatalf("a frame beginning on moment %d is first shown for %d", r, got)
		}
		// A frame of a file a little off its place is still on it.
		if got := ntsc.FirstShown(at + 0.0009); got != r {
			t.Fatalf("a frame beginning just after moment %d is first shown for %d", r, got)
		}
		if got := ntsc.FirstShown(at - 0.0009); got != r {
			t.Fatalf("a frame beginning just before moment %d is first shown for %d", r, got)
		}
		if got := ntsc.Holding(at); got != r {
			t.Fatalf("moment %d is held by %d", r, got)
		}
		if got := ntsc.Holding(at - 0.002); got != max(r-1, 0) {
			t.Fatalf("just before moment %d is held by %d", r, got)
		}
	}
	// One that begins between two moments is shown from the second.
	if got := ntsc.FirstShown(ntsc.At(7) + 0.01); got != 8 {
		t.Errorf("a frame between moments 7 and 8 is first shown for %d", got)
	}
	// Before the grid and after it, the first and the last.
	if got := ntsc.Holding(0); got != 0 {
		t.Errorf("the start of the file is held by %d", got)
	}
	if got := ntsc.Holding(100); got != 99 {
		t.Errorf("the end of the file is held by %d", got)
	}
}

// An open carries the grid it counts on, and an open the decoder cannot
// take is no open.
func TestAnOpenCarriesItsGrid(t *testing.T) {
	g := Grid{Origin: 1.4, Start: 0.033367, Num: 30000, Den: 1001, Count: 1200}
	for _, want := range []bool{false, true} {
		id, frame, w, h, got, key, ok := ReadOpen(strings.Fields(OpenRequest(7, 42, 640, 360, g, want)))
		if !ok || id != 7 || frame != 42 || w != 640 || h != 360 || got != g || key != want {
			t.Fatalf("read back as %d %d %dx%d %+v %v %v", id, frame, w, h, got, key, ok)
		}
	}
	if _, _, _, _, got, _, ok := ReadOpen(strings.Fields("open 1 0 640 360 -0.021 0 30 1 1200")); !ok || got.Origin != -0.021 {
		t.Errorf("a clock that starts before nought was read as %+v %v", got, ok)
	}
	for _, bad := range []string{
		"open 1 0 640 360",
		"open 1 0 640 360 0 30 1 1200",
		"open 1 1200 640 360 0 0 30 1 1200",
		"open 1 -1 640 360 0 0 30 1 1200",
		"open 1 0 641 360 0 0 30 1 1200",
		"open 1 0 640 360 0 0 0 1 1200",
		"open 1 0 640 360 0 NaN 30 1 1200",
		"open 1 0 640 360 NaN 0 30 1 1200",
		"open 1 0 640 360 Inf 0 30 1 1200",
		"open 1 0 640 360 2e6 0 30 1 1200",
		"open 1 0 640 360 0 -1 30 1 1200",
		"open 1 0 640 360 0 0 30 1 0",
		"open 1 0 640 360 0 0 30 1 999999999",
		"open 1 0 640 360 0 0 30 1 1200 keys",
		"open 1 0 640 360 0 0 30 1 1200 key key",
	} {
		if _, _, _, _, _, _, ok := ReadOpen(strings.Fields(bad)); ok {
			t.Errorf("%q was taken", bad)
		}
	}
}

// The video preview hands back the grid it counts on, and one that cannot
// be counted on is no grid.
func TestTheVideoPreviewHandsBackItsGrid(t *testing.T) {
	g := Grid{Origin: -0.021, Start: 0.033367, Num: 30000, Den: 1001, Count: 1200}
	if got, ok := ReadGrid(GridText(g)); !ok || got != g {
		t.Fatalf("%q read back as %+v %v", GridText(g), got, ok)
	}
	for _, bad := range []string{"", "0,0,30,1", "0,0,30,1,1200,1", "0,0,0,1,1200", "0,-1,30,1,1200",
		"NaN,0,30,1,1200", "0,0,30,1,0", "0,0,30,1,999999999", "0,0,x,1,1200"} {
		if _, ok := ReadGrid(bad); ok {
			t.Errorf("%q was taken", bad)
		}
	}
}
