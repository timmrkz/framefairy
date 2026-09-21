package engine

import (
	"encoding/json"
	"io"
	"math"
	"sync"
	"testing"
)

// Every event the window is told about is encoded as JSON, and JSON has no
// way to say NaN or infinity. An event carrying one cannot be encoded at
// all, so the window stops hearing about that job: no progress, and no
// word that it finished either, which leaves the button it was started
// from saying it is working for ever.
//
// A share is worked out by dividing, and there are ways for that to come
// out as neither a number nor a share.
func TestAnEventTheWindowCannotBeToldAboutIsNeverSent(t *testing.T) {
	cases := []struct {
		what                         string
		fraction, remaining, covered float64
	}{
		{"an episode whose length could not be measured, 0 divided by 0",
			math.NaN(), 10, 0},
		{"a rate worked out from no elapsed time at all",
			0.5, math.Inf(1), 0},
		{"a share of an episode of no length",
			math.Inf(1), math.Inf(-1), 0},
		{"a second of the episode that is not a second",
			0.5, 10, math.NaN()},
		{"nothing known yet, which is the ordinary case",
			Unknown, Unknown, 0},
		{"an ordinary report",
			0.42, 91, 1200},
	}
	for _, c := range cases {
		var got []Event
		var mu sync.Mutex
		l := NewLog(io.Discard, false, false)
		l.SetSink(func(ev Event) {
			mu.Lock()
			got = append(got, ev)
			mu.Unlock()
		})
		l.ProgressTo("transcribing", c.fraction, c.remaining, c.covered)
		if len(got) != 1 {
			t.Fatalf("%s: %d events", c.what, len(got))
		}
		if _, err := json.Marshal(got[0]); err != nil {
			t.Errorf("%s: the window could not be told: %v", c.what, err)
		}
	}
}

// A share that is a number is still held between none of it and all of it,
// and one that is not a number is simply not known.
func TestAShareIsAShareOrIsNotKnown(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0}, {0.5, 0.5}, {1, 1},
		{1.4, 1}, {-3, 0},
		{math.NaN(), Unknown},
		{math.Inf(1), Unknown},
		{math.Inf(-1), Unknown},
		{Unknown, Unknown},
	}
	for _, c := range cases {
		var got Event
		l := NewLog(io.Discard, false, false)
		l.SetSink(func(ev Event) { got = ev })
		l.ProgressOf("working", c.in, Unknown)
		if got.Fraction != c.want {
			t.Errorf("a share of %v became %v, wanted %v", c.in, got.Fraction, c.want)
		}
	}
}

// Whatever a caller hands over, and whoever writes the next caller, the
// boundary answers for it. This drives the log the way a run does and
// checks every event that leaves it.
func TestNothingThatLeavesTheLogCanFailToEncode(t *testing.T) {
	var bad int
	l := NewLog(io.Discard, false, false)
	l.SetSink(func(ev Event) {
		if _, err := json.Marshal(ev); err != nil {
			bad++
			t.Errorf("an event could not be encoded: %v", err)
		}
	})
	awkward := []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, 1, -7, math.MaxFloat64}
	for _, f := range awkward {
		for _, r := range awkward {
			l.ProgressTo("working", f, r, f)
			l.ProgressOf("working", f, r)
		}
	}
	l.Info("%s", "a line")
	l.ClearProgress()
	_ = l.Step("a step", func() error { return nil })
	if bad > 0 {
		t.Errorf("%d events could not be encoded", bad)
	}
}
