package engine

import (
	"testing"
	"time"
)

// Placing the crop of a clip fills from start to end: the share only
// grows, moves on between two pieces of the work by the clock, never runs
// past the piece in hand, and is never full while there is work left.
func TestPlacingTheCropSaysHowFarItIs(t *testing.T) {
	var none *cropWork
	if f, left := none.share(time.Now()); f != Unknown || left != Unknown {
		t.Errorf("no work, share %v left %v", f, left)
	}
	w := newCropWork()
	if f, _ := w.share(time.Now()); f != Unknown {
		t.Errorf("before the work is known, share %v", f)
	}
	// Two pieces of 10 seconds: 40 seconds of work, scanned and sampled.
	w.start([]Span{{0, 10}, {20, 30}})
	began := w.began
	last := -1.0
	check := func(what string, at time.Duration, most float64) {
		t.Helper()
		f, left := w.share(began.Add(at))
		if f < last-1e-9 {
			t.Errorf("%s: share went back from %.3f to %.3f", what, last, f)
		}
		if f < 0 || f > most || left < 0 {
			t.Errorf("%s: share %.3f left %.1f, want at most %.3f", what, f, left, most)
		}
		last = f
	}
	w.part(10)
	w.partAt = began
	check("the first scan begun", 0, 0)
	// Far longer than the first speed says: it waits inside the piece.
	check("the first scan running long", 10*time.Second, 0.9*10/40)
	w.partDone()
	w.partAt = began.Add(2 * time.Second)
	// 10 of 40 in 2 seconds: 5 a second from here on.
	check("the first scan done", 2*time.Second, 10.0/40)
	w.part(10)
	w.partAt = began.Add(2 * time.Second)
	check("halfway through the second", 3*time.Second, 15.0/40+1e-9)
	w.partDone()
	for i := range 2 {
		w.part(10)
		w.partAt = began.Add(time.Duration(4+2*i) * time.Second)
		check("sampling", time.Duration(5+2*i)*time.Second, 0.99)
		w.partDone()
	}
	if f, left := w.share(began.Add(time.Minute)); f != 0.99 || left != 0 {
		t.Errorf("all done, share %.3f left %.1f", f, left)
	}
}
