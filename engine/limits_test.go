package engine

import "testing"

// A room of chars characters over a video heard to heardTo, the one
// reachOf builds.
func roomOfLines(chars int, heardTo, rate float64) RoomAndLines {
	r := reachOf(chars, heardTo, rate, heardTo)
	return RoomAndLines{Room: Room{Chars: chars, By: "context"}, Lines: r.lines, Heard: r.heard, Rate: rate}
}

func limitsWith(shortest, longest float64) *Project {
	return &Project{Base: Options{Min: shortest, Max: longest}}
}

// What a window and its search may be: room for the clips typed, no more
// clips than fit in the longest window, and no shortest clip too long for
// the clips asked for, each against the numbers the range picker held to
// before the limits were worked out here.
func TestAWindowsLimitsHoldItsClips(t *testing.T) {
	cases := []struct {
		what                         string
		shortest, longest            float64
		duration, length             float64
		typed                        int
		most, least                  float64
		clipsAtMost, count, suggests int
		shortestAtMost               float64
	}{
		{"nothing limits it", 20, 30, 3600, 1800, 0, 3600, 20, 30, 6, 6, 180},
		{"typed clips need room", 20, 30, 3600, 1800, 12, 3600, 240, 30, 12, 6, 180},
		{"one clip at its shortest is the floor", 5, 30, 3600, 600, 0, 3600, 10, 30, 5, 5, 180},
		{"never longer than the video", 20, 30, 100, 100, 12, 100, 100, 5, 5, 1, 20},
		{"typed past what fits is held to it", 20, 30, 200, 200, 30, 200, 200, 10, 10, 2, 20},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			l := limitsWith(c.shortest, c.longest).Limits(RoomAndLines{}, c.duration, c.length, c.typed)
			got := []float64{l.Most, l.Least, float64(l.ClipsAtMost), float64(l.Count), float64(l.Suggested), l.ShortestAtMost}
			want := []float64{c.most, c.least, float64(c.clipsAtMost), float64(c.count), float64(c.suggests), c.shortestAtMost}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("most, least, clips at most, count, suggested, shortest at most are %v, not %v", got, want)
				}
			}
		})
	}
}

// The longest a window may be is what fits anywhere, and Shortest is held
// so the clips asked for fit in it: 1000 characters a request at 100 a
// line every 4 s is a window of about 40 s.
func TestAWindowsLimitsFollowTheRoom(t *testing.T) {
	l := limitsWith(5, 10).Limits(roomOfLines(1000, 400, 25), 400, 400, 0)
	if l.Most > 44 || l.Most < 36 {
		t.Errorf("the longest window is %v, not about 40", l.Most)
	}
	if l.By != "context" {
		t.Errorf("it is limited by %q", l.By)
	}
	if float64(l.Count)*l.ShortestAtMost > l.Most+0.01 {
		t.Errorf("%d clips of %v s do not fit in %v", l.Count, l.ShortestAtMost, l.Most)
	}
}

// A window put back inside what it may be keeps its start and gives up
// its end, and grows backwards at the end of the video.
func TestAWindowIsFittedFromItsStart(t *testing.T) {
	cases := []struct {
		from, to, most, least, duration float64
		wantFrom, wantTo                float64
	}{
		{100, 400, 40, 20, 400, 100, 140},
		{100, 120, 400, 60, 400, 100, 160},
		{380, 400, 400, 60, 400, 340, 400},
		{10, 300, 400, 60, 400, 10, 300},
	}
	for _, c := range cases {
		from, to := FitWindow(c.from, c.to, c.most, c.least, c.duration)
		if from != c.wantFrom || to != c.wantTo {
			t.Errorf("%v to %v fitted to %v and %v is %v to %v, not %v to %v", c.from, c.to, c.most, c.least, from, to, c.wantFrom, c.wantTo)
		}
	}
}
