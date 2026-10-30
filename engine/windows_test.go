package engine

import (
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/timed"
)

// Clips made by hand searched nothing. Their set has no window, and a set
// with no window is otherwise a search of the whole episode, which laid the
// window over the whole range picker and hid every mark under it. It says
// who made it, and that is what counts, not what the file is called.
func TestAClipSetMadeByHandSearchedNothing(t *testing.T) {
	plans := []PlanSummary{
		{Path: "/ep.framefairy/logs/clips-0-1800.json", From: 0, To: 1800, Clips: 12},
		{Path: "/ep.framefairy/logs/clips-by-any-name.json", By: ByHand, Clips: 1},
	}
	got := fmt.Sprint(SearchPasses(plans, 14400))
	if want := "[{{0 1800} 1} {{1800 14400} 0}]"; got != want {
		t.Errorf("passes %s, want only the first half hour searched, %s", got, want)
	}
}

// The episode in parts by how many searches read them, the parts nobody
// searched too, and a search of the whole episode counted over all of it.
func TestSearchPasses(t *testing.T) {
	plans := []PlanSummary{
		{From: 0, To: 600, Clips: 3},
		{From: 600, To: 1200, Clips: 3},
		{From: 0, To: 600, Clips: 2},
		{Name: HandPlanName, By: ByHand, Clips: 1},
		{From: 1200, To: 1800},
	}
	got := fmt.Sprint(SearchPasses(plans, 2400))
	want := "[{{0 600} 2} {{600 1800} 1} {{1800 2400} 0}]"
	if got != want {
		t.Errorf("passes:\n got %s\nwant %s", got, want)
	}
	if got := fmt.Sprint(SearchPasses(nil, 360)); got != "[{{0 360} 0}]" {
		t.Errorf("an episode nobody searched: %s", got)
	}
	whole := []PlanSummary{{}, {From: 0, To: 360}}
	if got := fmt.Sprint(SearchPasses(whole, 360)); got != "[{{0 360} 2}]" {
		t.Errorf("an episode searched whole twice: %s", got)
	}
}

// The window moves on when the app chooses it, from where the fewest
// searches have been. Moved here from the workspace with the window's
// one owner, #458.
func TestNextWindow(t *testing.T) {
	const half, hours, six = 30 * 60.0, 4 * 3600.0, 6 * 60.0
	pass := func(from, to float64, times int) Pass { return Pass{timed.Window{Start: from, End: to}, times} }
	for _, c := range []struct {
		name           string
		passes         []Pass
		duration, size float64
		least          float64
		want           timed.Window
	}{
		{"a fresh episode opens on the first half hour", []Pass{pass(0, hours, 0)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"after the first search it moves past what was searched",
			[]Pass{pass(0, half, 1), pass(half, hours, 0)}, hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a gap in the middle is taken before the end",
			[]Pass{pass(0, half, 1), pass(half, 2*half, 0), pass(2*half, 3*half, 1), pass(3*half, hours, 0)},
			hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a part a little longer than the half hour is taken whole",
			[]Pass{pass(0, 40*60, 0), pass(40*60, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: 40 * 60}},
		{"a part well over the half hour gives up only that much",
			[]Pass{pass(0, 3600, 0), pass(3600, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"an episode searched end to end starts over at the start", []Pass{pass(0, hours, 1)}, hours, half, 0, timed.Window{Start: 0, End: half}},
		{"and the second round walks on the way the first did",
			[]Pass{pass(0, half, 2), pass(half, hours, 1)}, hours, half, 0, timed.Window{Start: half, End: 2 * half}},
		{"a short episode is searched whole", []Pass{pass(0, six, 0)}, six, six, 0, timed.Window{Start: 0, End: six}},
		{"again and again", []Pass{pass(0, six, 3)}, six, six, 0, timed.Window{Start: 0, End: six}},
		{"a scrap too short for a clip is passed over",
			[]Pass{pass(0, 10, 0), pass(10, half, 1), pass(half, hours, 0)}, hours, half, 20, timed.Window{Start: half, End: 2 * half}},
		{"nothing to go by is the whole episode", nil, hours, half, 0, timed.Window{Start: 0, End: hours}},
	} {
		if got := NextWindow(c.passes, c.duration, c.size, c.least); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// After a search the window walks on as long as it was left.
func TestFollowingWindow(t *testing.T) {
	const six, short = 6 * 60.0, 5*60 + 25.0
	for _, c := range []struct {
		name                    string
		ended, length, duration float64
		want                    timed.Window
	}{
		{"a minute at four goes on to a minute at five", 240, 60, six, timed.Window{Start: 240, End: 300}},
		{"the last one is cut at the end of the episode", 300, 60, short, timed.Window{Start: 300, End: short}},
		{"and the one after it is as long as it was set again, from the start", short, 60, short, timed.Window{Start: 0, End: 60}},
		{"a scrap too short for a clip is passed over", 310, 60, short, timed.Window{Start: 0, End: 60}},
		{"a short episode searched whole is searched whole again", six, six, six, timed.Window{Start: 0, End: six}},
	} {
		if got := FollowingWindow(c.ended, c.length, c.duration, 20); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A window the app places lands on a round step that follows the episode
// alone, so the first search and the workspace place the same window.
func TestAWindowTheAppPlacesLandsOnTheStep(t *testing.T) {
	const hours = 14423.0
	for _, c := range []struct {
		duration, want float64
	}{{hours, 120}, {3600, 30}, {360, 5}, {60, 1}, {30 * 3600, 600}, {100 * 3600, 1800}} {
		if got := PlaceStep(c.duration); got != c.want {
			t.Errorf("the step of %s is %v, want %v", HMS(c.duration), got, c.want)
		}
	}
	for _, c := range []struct {
		name         string
		w            timed.Window
		length, step float64
		duration     float64
		least        float64
		want         timed.Window
	}{
		{"the app's own window of 30:03 is 30:00 on the step", timed.Window{Start: 0, End: 1803}, 1803, 120, hours, 20, timed.Window{Start: 0, End: 1800}},
		{"a window after one that ended off the step starts on it", timed.Window{Start: 4202, End: 6004}, 1802, 120, hours, 20, timed.Window{Start: 4200, End: 6000}},
		{"the last one takes in the scrap after it", timed.Window{Start: 12600, End: 14400}, 1800, 120, hours, 20, timed.Window{Start: 12600, End: hours}},
		{"never shorter than a step", timed.Window{Start: 60, End: 62}, 2, 5, 360, 0, timed.Window{Start: 60, End: 65}},
	} {
		if got := OnStep(c.w, c.length, c.duration, c.step, c.least); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A new video's window is the first of the equal windows it is cut into,
// on a round step, the whole of a short one, and no longer than one
// search can read. The first search and the workspace both take it from
// here, so the window searched is the window drawn, #458.
func TestTheFirstWindowFollowsTheVideo(t *testing.T) {
	for _, c := range []struct {
		name           string
		duration, most float64
		want           PlacedWindow
	}{
		{"half an hour", 1800, 0, PlacedWindow{timed.Window{Start: 0, End: 600}, 600}},
		{"an hour", 3600, 0, PlacedWindow{timed.Window{Start: 0, End: 900}, 900}},
		{"four hours", 4 * 3600, 0, PlacedWindow{timed.Window{Start: 0, End: 1800}, 1800}},
		{"a little over four hours, 30:02 a window, on the step", 4*3600 + 15, 0, PlacedWindow{timed.Window{Start: 0, End: 1800}, 1800}},
		{"ten minutes is searched whole", 600, 0, PlacedWindow{timed.Window{Start: 0, End: 600}, 600}},
		{"a quarter of an hour too", 900, 0, PlacedWindow{timed.Window{Start: 0, End: 900}, 900}},
		{"no longer than a search reads", 3600, 700, PlacedWindow{timed.Window{Start: 0, End: 690}, 690}},
	} {
		if got := placeWindow(SearchPasses(nil, c.duration), c.duration, 20, c.most); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// Where speech is uneven, the window the app places fits the reach the
// range picker fits every window to, and stays on the step, so the
// workspace has nothing to cut and write back. The room at the video's
// average rate let it be longer, the fourth review of #458.
func TestAPlacedWindowFitsTheRangePickersReach(t *testing.T) {
	const duration = 3600.0
	// Slow speech for the first half hour, fast speech after it.
	var lines []LineWeight
	for at := 0.0; at+3 <= duration; at += 4 {
		chars := 40
		if at >= 1800 {
			chars = 160
		}
		lines = append(lines, LineWeight{Start: at, End: at + 3, Chars: chars})
	}
	const chars = 30000
	average := chars / RateOf(lines)
	reach := RoomAndLines{Room: Room{Chars: chars}, Lines: lines, Heard: [][2]float64{{0, duration}}, Rate: RateOf(lines)}.Longest(duration)
	if reach >= average {
		t.Fatalf("the reach %v is not shorter than the room at the average rate %v, so this proves nothing", reach, average)
	}
	got := placeWindow(SearchPasses(nil, duration), duration, 20, reach)
	step := PlaceStep(duration)
	if got.End-got.Start > reach || got.Length > reach {
		t.Errorf("the window %+v is longer than the reach %v", got, reach)
	}
	if got.Start != 0 || got.Length != float64(int(got.Length/step))*step {
		t.Errorf("the window %+v is not on the step of %v", got, step)
	}
}

// After a search the window walks on as long as it was made, and a
// window made longer than one search can read now is cut to it on the
// step, so a search the first one measured from real speech is not
// asked for more than fits.
func TestTheWindowMovesOnAsLongAsItWasMade(t *testing.T) {
	for _, c := range []struct {
		name                    string
		ended, length, duration float64
		most                    float64
		want                    PlacedWindow
	}{
		{"on from where it ended", 900, 900, 3600, 0, PlacedWindow{timed.Window{Start: 900, End: 1800}, 900}},
		{"the last one is cut, its length kept", 3000, 900, 3600, 0, PlacedWindow{timed.Window{Start: 3000, End: 3600}, 900}},
		{"cut to what a search reads, on the step", 900, 900, 3600, 700, PlacedWindow{timed.Window{Start: 900, End: 1590}, 690}},
	} {
		if got := moveWindowOn(c.ended, c.length, c.duration, 20, c.most); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// A project places its video's window from the plans in its work folder,
// past what was searched, and moves it on after a search the same way,
// no longer than the room it is given, the one the range picker reads,
// so both front ends ask one place where a search reads, #610.
func TestAProjectPlacesItsWindowPastWhatWasSearched(t *testing.T) {
	fsys := afero.NewMemMapFs()
	p := NewProject(NewEngine(NewLog(io.Discard, false, false), fsys), "/home/tim/ep.mp4", DefaultOptions())
	if err := afero.WriteFile(fsys, filepath.Join(p.LogsDir(), "clips-0-1800.json"), []byte(aRealisticPlan(3)), 0o644); err != nil {
		t.Fatal(err)
	}
	const duration = 3600.0
	roomy := RoomAndLines{Room: Room{Chars: 1e6}, Rate: SpokenChars}
	want := PlacedWindow{timed.Window{Start: 1800, End: 2700}, 900}
	if got := p.PlaceWindow(roomy, duration); got != want {
		t.Errorf("placed %+v, want the quarter hour after the half hour searched, %+v", got, want)
	}
	after := p.WindowAfter(roomy, PlanRequest{From: 1800, To: 2700}, 900, duration)
	if want := (PlacedWindow{timed.Window{Start: 2700, End: 3600}, 900}); after != want {
		t.Errorf("after the search %+v, want %+v", after, want)
	}
	toEnd := p.WindowAfter(roomy, PlanRequest{From: 2700}, 0, duration)
	if want := (PlacedWindow{timed.Window{Start: 0, End: 900}, 900}); toEnd != want {
		t.Errorf("after a search to the end %+v, want the start again, %+v", toEnd, want)
	}

	// Room for ten minutes of speech, and nothing heard yet.
	tight := RoomAndLines{Room: Room{Chars: 600 * SpokenChars}, Rate: SpokenChars}
	if got, want := p.PlaceWindow(tight, duration), (PlacedWindow{timed.Window{Start: 1800, End: 2400}, 600}); got != want {
		t.Errorf("placed %+v in room for ten minutes, want %+v", got, want)
	}
	if got, want := p.WindowAfter(tight, PlanRequest{From: 1800, To: 2700}, 900, duration), (PlacedWindow{timed.Window{Start: 2700, End: 3300}, 600}); got != want {
		t.Errorf("moved on to %+v in room for ten minutes, want %+v", got, want)
	}
}
