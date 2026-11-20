package engine

import (
	"bytes"
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/engine/timed"
)

// What is heard says how much of a video has been read. Reading does not
// unhappen, so no order in which a hearing reports and its saves land may
// take any of it back. The window kept these cases once, #610.
func TestWhatIsHeardOnlyEverGrows(t *testing.T) {
	upTo := func(at float64) [][2]float64 {
		if at <= 0 {
			return nil
		}
		return [][2]float64{{0, at}}
	}
	fresh := func() *Episode { return NewMachine(afero.NewMemMapFs()).Episode("/v/a.mp4") }
	is := func(t *testing.T, got [][2]float64, want ...[2]float64) {
		t.Helper()
		if want == nil {
			want = [][2]float64{}
		}
		if got == nil {
			got = [][2]float64{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("heard %v, want %v", got, want)
		}
	}

	t.Run("it follows the work while the hearing runs", func(t *testing.T) {
		ep := fresh()
		ep.reach(0, 12)
		is(t, ep.Reached(nil, false), [2]float64{0, 12})
		ep.reach(0, 30)
		is(t, ep.Reached(nil, false), [2]float64{0, 30})
		// The save lands, far behind the work, and changes nothing.
		ep.reach(0, 42)
		is(t, ep.Reached(upTo(24), false), [2]float64{0, 42})
	})

	// The bug as it was seen: the job reported 300 seconds, the transcript
	// on disk had been written at 240, and the moment the job stopped the
	// live number was gone.
	t.Run("pausing does not take any of it back", func(t *testing.T) {
		ep := fresh()
		ep.reach(0, 300)
		is(t, ep.Reached(upTo(240), false), [2]float64{0, 300})
		ep.reachEnds()
		is(t, ep.Reached(upTo(240), false), [2]float64{0, 300})
	})

	// A hearing carried on starts again from the end of the saved
	// transcript, behind where the work had got to. Those seconds were
	// heard, so the edge stands still until the work passes them.
	t.Run("carrying on from a pause does not either", func(t *testing.T) {
		ep := fresh()
		ep.reach(0, 300)
		ep.reachEnds()
		ep.reach(240, 240)
		is(t, ep.Reached(upTo(240), false), [2]float64{0, 300})
		ep.reach(240, 280)
		is(t, ep.Reached(upTo(240), false), [2]float64{0, 300})
		ep.reach(240, 310)
		is(t, ep.Reached(upTo(240), false), [2]float64{0, 310})
	})

	t.Run("an older save read after a newer one does not take any back", func(t *testing.T) {
		ep := fresh()
		is(t, ep.Reached(upTo(600), false), [2]float64{0, 600})
		is(t, ep.Reached(upTo(120), false), [2]float64{0, 600})
	})

	t.Run("a part heard where a clip made by hand needs it stands apart", func(t *testing.T) {
		ep := fresh()
		ep.reach(5000, 5030)
		is(t, ep.Reached(upTo(600), false), [2]float64{0, 600}, [2]float64{5000, 5030})
		// And when the gap between is heard, they are one.
		ep.reachEnds()
		is(t, ep.Reached(upTo(5030), false), [2]float64{0, 5030})
	})

	// A hearing reports and its saves land in any order, so every
	// interleaving of a likely run is walked: what is heard never shrinks
	// and always ends at the furthest either of them reached.
	t.Run("whatever order the two arrive in, the answer is the same", func(t *testing.T) {
		saved := []float64{0, 0, 60, 60, 120, 180, 180}
		running := []float64{10, 45, 45, 90, 150, 150, 200}
		for shift := range saved {
			ep := fresh()
			last, most := 0.0, 0.0
			for i := range saved {
				s := saved[max(0, i-shift)]
				r := running[i]
				most = math.Max(most, math.Max(s, r))
				ep.reach(0, r)
				got := HeardOf(ep.Reached(upTo(s), false), 0, 1e9)
				if got < last {
					t.Errorf("shift %d, step %d: heard %v after %v", shift, i, got, last)
				}
				last = got
			}
			if last != most {
				t.Errorf("shift %d: ended at %v, not %v", shift, last, most)
			}
		}
	})

	// The file changed, so the old mark is about a transcript that no
	// longer exists. What the hearing running now has reached stays.
	t.Run("a transcript being read again starts it over", func(t *testing.T) {
		ep := fresh()
		ep.Reached(upTo(600), false)
		ep.reach(0, 10)
		is(t, ep.Reached(nil, true), [2]float64{0, 10})
		ep.reach(0, 20)
		is(t, ep.Reached(nil, false), [2]float64{0, 20})
	})

	t.Run("a work folder that is gone starts it over", func(t *testing.T) {
		ep := fresh()
		ep.Reached(upTo(600), false)
		is(t, ep.Reached(nil, true))
	})
}

// How much of a span the parts hold, whether they hold all of it but half
// a second, and what of a video they leave, gaps under a second aside.
func TestWhatThePartsHeardHold(t *testing.T) {
	parts := [][2]float64{{0, 10}, {40, 60}, {60.5, 100}}
	if got := HeardOf(parts, 5, 50); got != 15 {
		t.Errorf("of 5 to 50, %v heard, want 15", got)
	}
	if !HeardOver(parts, 42, 59) || !HeardOver(parts, 41, 100) || HeardOver(parts, 5, 50) {
		t.Error("held over the wrong spans")
	}
	if got, want := NotHeard(parts, 120), [][2]float64{{10, 40}, {100, 120}}; !reflect.DeepEqual(got, want) {
		t.Errorf("not heard %v, want %v", got, want)
	}
}

// reachCounter is a recogniser that, each time it is asked, notes how
// much of the video its Episode says is heard, saved or not.
type reachCounter struct {
	fakeRecognizer
	ep   *Episode
	seen *[]float64
}

func (r reachCounter) Recognize(samples []float32, rate int) []Token {
	*r.seen = append(*r.seen, HeardOf(r.ep.Reached(nil, false), 0, math.Inf(1)))
	return r.fakeRecognizer.Recognize(samples, rate)
}

// A hearing tells the video's Episode what it has heard chunk by chunk,
// before anything is saved, so what is heard moves with the work, and
// says when it is over.
//
// On the real disk: ffmpeg reads the episode.
func TestAHearingSaysWhatItHeardAsItGoes(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	var calls int32
	var seen []float64
	ep := e.Episode(source)
	rec := reachCounter{fakeRecognizer{&calls}, ep, &seen}
	if _, err := e.Transcribe(context.Background(), source, timed.Window{Start: 0, End: 70}, rec, nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) < 2 || seen[0] != 0 || seen[len(seen)-1] <= 0 {
		t.Fatalf("heard before each chunk %v, want nothing at first and more after", seen)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Errorf("heard before each chunk %v, which did not grow with every chunk", seen)
		}
	}
	if got := HeardOf(ep.Reached(nil, false), 0, 70); got < 69 {
		t.Errorf("heard %v s of 70 once the hearing was over", got)
	}
	ep.m.held.Lock()
	hearing := ep.hearing
	ep.m.held.Unlock()
	if hearing != nil {
		t.Errorf("the hearing is over and still said to run, at %v", *hearing)
	}
}
