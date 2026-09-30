package engine

import (
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// How far placing the crop of one clip has come
//
// Placing the crop is two passes over what a clip keeps: ffmpeg scans each
// piece for camera switches, and then frames are sampled from each shot to
// find where the faces are. Both go through every second the clip keeps
// once, so the work is twice the clip's length in seconds, and each piece
// scanned and each shot sampled is that much of it done.
//
// Between two of those the share moves on by the clock, at the speed the
// work has gone so far, and never past the end of the piece in hand. So a
// clip made by hand fills smoothly from start to end, rather than standing
// still and jumping, and never stands full while there is work left.
// ---------------------------------------------------------------------------

// firstSpeed is how many seconds of a clip placing the crop gets through a
// second before it has done anything to measure: a clip of 25 seconds in
// about two, which is what it takes on an M2 Max.
const firstSpeed = 25.0

// cropWork is the work of placing the crop of one clip. A nil one counts
// nothing, which is what a search's framers are given: a search says how
// far it has come as a whole, see searchClock.
type cropWork struct {
	mu      sync.Mutex
	whole   float64
	done    float64
	current float64
	began   time.Time
	partAt  time.Time
}

func newCropWork() *cropWork { return &cropWork{} }

// start is the work known: spans are what the clip keeps.
func (w *cropWork) start(spans []Span) {
	if w == nil {
		return
	}
	whole := 0.0
	for _, s := range spans {
		whole += max(s.End-s.Start, 0)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.whole, w.done, w.current = 2*whole, 0, 0
	w.began, w.partAt = time.Now(), time.Now()
}

// part begins a piece of the work of so many seconds of the clip.
func (w *cropWork) part(seconds float64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current, w.partAt = max(seconds, 0), time.Now()
}

// partDone is the piece in hand done.
func (w *cropWork) partDone() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.done = min(w.done+w.current, w.whole)
	w.current, w.partAt = 0, time.Now()
}

// share is how far the work is, from 0 to 1, and the seconds left, at now.
// Unknown for both until the work is known.
func (w *cropWork) share(now time.Time) (float64, float64) {
	if w == nil {
		return Unknown, Unknown
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.whole <= 0 {
		return Unknown, Unknown
	}
	speed := firstSpeed
	if took := w.partAt.Sub(w.began).Seconds(); w.done > 0 && took > 0 {
		speed = w.done / took
	}
	// Inside the piece in hand the clock moves the share on, never past
	// nine tenths of the piece, so a piece that runs long waits short of
	// its end rather than running into the next.
	inPart := min(now.Sub(w.partAt).Seconds()*speed, 0.9*w.current)
	done := min(w.done+inPart, w.whole)
	return min(done/w.whole, 0.99), (w.whole - done) / speed
}

// report says how far the work is, about four times a second, until stop
// is closed. It holds the progress line meanwhile, so the scans inside it
// do not show their own progress or clear it.
func (w *cropWork) report(log *Log, stop <-chan struct{}) {
	log.HoldProgress(true)
	defer log.HoldProgress(false)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if fraction, left := w.share(now); fraction != Unknown {
				log.ProgressFound("placing the crop", fraction, left, 0)
			}
		}
	}
}
