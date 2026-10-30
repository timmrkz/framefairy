package engine

import (
	"math"
	"math/rand/v2"
	"testing"
)

// A line every four seconds, three seconds long, each weighing 100
// characters, for as long as the transcript goes, the room the range
// picker's tests in room.test.ts use.
func reachOf(chars int, heardTo, rate, duration float64) *Reach {
	var lines []LineWeight
	for at := 0.0; at+3 <= heardTo; at += 4 {
		lines = append(lines, LineWeight{Start: at, End: at + 3, Chars: 100})
	}
	var heard [][2]float64
	if heardTo > 0 {
		heard = [][2]float64{{0, heardTo}}
	}
	return NewReach(RoomAndLines{Room: Room{Chars: chars}, Lines: lines, Heard: heard, Rate: rate}, duration)
}

func TestAWindowWeighsWhatTheRangePickerWeighs(t *testing.T) {
	r := reachOf(1e9, 400, 25, 400)
	for _, c := range []struct{ from, to, want float64 }{{0, 4, 100}, {0, 4.5, 200}, {3.5, 4, 0}, {0, 400, 10000}} {
		if got := r.weight(c.from, c.to); got != c.want {
			t.Errorf("%v to %v weighs %v, want %v", c.from, c.to, got, c.want)
		}
	}
	r = reachOf(1e9, 40, 25, 400)
	for _, c := range []struct{ from, to, want float64 }{{39, 40, 0}, {100, 110, 250}, {0, 50, 1000 + 25*10}} {
		if got := r.weight(c.from, c.to); got != c.want {
			t.Errorf("%v to %v weighs %v, want %v, with what is not heard at the rate", c.from, c.to, got, c.want)
		}
	}
	if got := reachOf(1e9, 400, 25, 400).Anywhere(); got != 400 {
		t.Errorf("a room that holds the video reaches %v, not all 400", got)
	}
	if got := NewReach(RoomAndLines{Rate: 20}, 400).Anywhere(); got != 400 {
		t.Errorf("no room yet reaches %v, not all 400", got)
	}
}

// The longest window that fits anywhere is set by the densest stretch,
// fits from every start, and is a whole second, the cases of room.test.ts.
func TestAWindowThatFitsAnywhereFitsEverywhere(t *testing.T) {
	r := reachOf(2000, 200, 25, 4000)
	reach := r.Anywhere()
	if reach > 80 || reach <= 75 {
		t.Errorf("heard for 200 s, the densest stretch reaches %v, not 76 to 80", reach)
	}
	for s := 0.0; s+reach <= 4000; s += 0.37 {
		if !r.fits(s, s+reach) {
			t.Fatalf("a window of %v from %v does not fit", reach, s)
		}
	}
	r = reachOf(80000, 3600, 40, 4*3600)
	most := r.Anywhere()
	if most != math.Floor(most) {
		t.Errorf("the reach %v is not a whole second", most)
	}
	for from := 0.0; from+most <= 4*3600; from += 7.3 {
		if !r.fits(from, from+most) {
			t.Fatalf("a window of %v from %v does not fit", most, from)
		}
	}
}

// Over fixed seeds of made-up speech, the reach fits from every start,
// and two seconds more do not fit from some start, so it is the densest
// stretch's and not shorter. Two, because both the end a start reaches and
// the reach itself land on a whole second, each giving up less than one.
// The starts are where a window weighs most, at the edges of lines and of
// what is heard, and a grid between them.
func TestAnywhereFitsEverywhereAndNoMore(t *testing.T) {
	for seed := range uint64(40) {
		random := rand.New(rand.NewPCG(seed, 7))
		duration := 600 + random.Float64()*7200
		heardTo := math.Floor(random.Float64() * duration)
		var lines []LineWeight
		for at := 0.0; at < heardTo-1; {
			long := 0.5 + random.Float64()*6
			lines = append(lines, LineWeight{Start: at, End: math.Min(at+long, heardTo), Chars: 10 + random.IntN(200)})
			at += long + random.Float64()*3
		}
		var heard [][2]float64
		if heardTo > 0 {
			heard = [][2]float64{{0, heardTo}}
		}
		r := NewReach(RoomAndLines{Room: Room{Chars: 200 + random.IntN(20000)}, Lines: lines, Heard: heard, Rate: random.Float64() * 40}, duration)
		reach := r.Anywhere()
		if reach >= duration {
			continue
		}
		starts := []float64{0, heardTo}
		for _, l := range lines {
			starts = append(starts, l.Start, math.Max(l.Start, l.End-0.001))
		}
		for s := 0.0; s < duration; s += 3.1 {
			starts = append(starts, s)
		}
		tight := false
		for _, s := range starts {
			if s+reach > duration {
				continue
			}
			if !r.fits(s, s+reach) {
				t.Fatalf("seed %d: a window of %v from %v does not fit", seed, reach, s)
			}
			if s+reach+2 <= duration && !r.fits(s, s+reach+2) {
				tight = true
			}
		}
		if !tight {
			t.Errorf("seed %d: a window two seconds longer than the reach %v fits everywhere", seed, reach)
		}
	}
}
