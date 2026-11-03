package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"

	"framefairy/engine"
)

// The window the engine places on a video fits the reach the state says
// the range picker may draw, where speech is uneven too, so nothing is
// cut and kept again. The two were once worked out apart and disagreed,
// the fourth review of #458, and both now come from engine.RoomAndLines.
func TestThePlacedWindowFitsTheRoomTheRangePickerReads(t *testing.T) {
	t.Parallel()
	svc, source, duration := anUnevenVideo(t)

	reach := svc.roomAndLines(svc.project(source)).Longest(duration)
	if reach >= engine.SuggestedWindow(duration) {
		t.Fatalf("one search reads %v s, no less than a window, so this proves nothing", reach)
	}
	got := svc.placedWindow(source, duration)
	if got.To-got.From > reach || got.Length > reach {
		t.Errorf("the window %+v is longer than the reach of %v s", got, reach)
	}
	if got.From != 0 || got.Length <= 0 {
		t.Errorf("the window %+v is not the first of the video", got)
	}
}

// anUnevenVideo is a video of an hour, heard all through, slow speech for
// its first half hour and fast and long words after it, so one search
// reads less than a window of it. It probes as an hour long.
func anUnevenVideo(t *testing.T) (*FrameFairy, string, float64) {
	t.Helper()
	svc, eps := memoryLibrary(t, "ep.mp4")
	source := eps[0]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	const duration = 3600.0
	var words [][3]any
	for at := 0.0; at+1 <= duration; at += 0.5 {
		word := "ja"
		if at >= 1800 {
			word = "Donaudampfschifffahrtsgesellschaft"
		}
		words = append(words, [3]any{at, at + 0.4, fmt.Sprintf("%s%d", word, len(words))})
	}
	heardEpisode(t, svc, source, duration, words)
	// A file in memory has no stamp on the real disk, which is what the
	// probes are kept by, so the one there is stands for it.
	svc.probes = &probed{info: map[string]engine.SourceInfo{"": {Duration: duration}}}
	return svc, source, duration
}

// The state carries the window with what a search can do with it, and a
// window kept longer than one search reads, as a window kept before the
// room shrank is, comes in the state fitted and is kept so: the
// workspace draws what Go sends and fits nothing itself.
func TestTheStateCarriesTheWindowFittedToWhatASearchReads(t *testing.T) {
	t.Parallel()
	svc, source, duration := anUnevenVideo(t)
	ctx := context.Background()
	if err := svc.chooseWindow(source, 0, duration, duration, true); err != nil {
		t.Fatal(err)
	}
	st, err := svc.State(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	w := st.Window
	if w == nil {
		t.Fatal("the state carries no window")
	}
	reach := svc.roomAndLines(svc.project(source)).Longest(duration)
	if math.Abs(w.Most-reach) > 0.01 || w.Most >= duration {
		t.Errorf("the window may be %v s long, not the %v s one search reads", w.Most, reach)
	}
	if w.From != 0 || w.To-w.From > w.Most+0.01 || w.To-w.From < w.Least {
		t.Errorf("the window %v–%v s is not fitted to %v–%v s", w.From, w.To, w.Least, w.Most)
	}
	if w.Count < 1 || w.Count > w.ClipsAtMost || w.Shortest <= 0 || w.Longest < w.Shortest {
		t.Errorf("the window's search asks %+v", w.WindowLimits)
	}
	if kept := keptWindow(svc, source); kept == nil || !sameWindow(*kept, w.KeptWindow) {
		t.Errorf("the window kept is %+v, not the one fitted, %+v", kept, w.KeptWindow)
	}
}

// A search running on the window holds it as it is, even where it is
// longer than a search reads now, because it is the window being
// searched, and a stopped one holds it for Continue.
func TestASearchHoldsItsWindowAsItIs(t *testing.T) {
	t.Parallel()
	for _, state := range []string{JobRunning, JobStopped} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			svc, source, duration := anUnevenVideo(t)
			if err := svc.chooseWindow(source, 0, duration, duration, true); err != nil {
				t.Fatal(err)
			}
			job := Job{ID: "s1", Episode: source, State: state, JobRecord: engine.JobRecord{Kind: engine.JobSearch}}
			svc.jobs.mu.Lock()
			svc.jobs.jobs = append(svc.jobs.jobs, &job)
			svc.jobs.mu.Unlock()
			st, err := svc.State(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			if st.Window == nil || st.Window.To != duration {
				t.Errorf("a search %s on the window and the window came as %+v", state, st.Window)
			}
		})
	}
}

// What is typed for a search is held to what the window can give: no
// more clips than fit in it, clips no longer than share it, and the
// shortest never longer than the longest, by moving the other one. A
// number typed in one field keeps the target typed before.
func TestWhatIsTypedForASearchIsHeldToTheWindow(t *testing.T) {
	t.Parallel()
	svc, source, _ := anUnevenVideo(t)
	ctx := context.Background()
	st, err := svc.SetSearch(ctx, source, 99, 20, 30, "target")
	if err != nil {
		t.Fatal(err)
	}
	most := st.Window.ClipsAtMost
	if set := svc.store.Settings(); set.Target != most || st.Window.Typed != most || most >= 99 {
		t.Errorf("99 clips typed kept %d and the window says %d, not the %d that fit", set.Target, st.Window.Typed, most)
	}
	if _, err := svc.SetSearch(ctx, source, 3, 20, 30, "target"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSearch(ctx, source, 0, 40, 30, "min"); err != nil {
		t.Fatal(err)
	}
	if set := svc.store.Settings(); set.Min != 40 || set.Max != 40 || set.Target != 3 {
		t.Errorf("a shortest of 40 s over a longest of 30 s kept %v–%v s and a target of %d", set.Min, set.Max, set.Target)
	}
	if _, err := svc.SetSearch(ctx, source, 0, 40, 25, "max"); err != nil {
		t.Fatal(err)
	}
	if set := svc.store.Settings(); set.Min != 25 || set.Max != 25 {
		t.Errorf("a longest of 25 s under a shortest of 40 s kept %v–%v s", set.Min, set.Max)
	}
	before, err := svc.State(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	st, err = svc.SetSearch(ctx, source, 0, 1000, 1000, "min")
	if err != nil {
		t.Fatal(err)
	}
	if set := svc.store.Settings(); set.Min != before.Window.ShortestAtMost || set.Min >= 180 {
		t.Errorf("a shortest of 1000 s kept %v s, not the %v s each of 3 clips may have", set.Min, before.Window.ShortestAtMost)
	}
	// The window grew to hold three clips that long, and the three typed
	// stay typed for it.
	if w := st.Window; w.Typed != 3 || w.To-w.From < 3*before.Window.ShortestAtMost {
		t.Errorf("after clips of %v s the window is %v–%v s and asks for %d typed", before.Window.ShortestAtMost, w.From, w.To, w.Typed)
	}
}

// A target typed on one video stays when another video's window of the
// same length is fitted shorter: the fit carries the target along only
// on the video it was typed on. A state read of the other video, as it
// is heard, once took the target typed on this one away.
func TestATargetTypedOnOneVideoStaysWhenAnotherIsFitted(t *testing.T) {
	t.Parallel()
	svc, eps := memoryLibrary(t, "a.mp4", "b.mp4")
	a, b := eps[0], eps[1]
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	const duration = 3600.0
	for _, ep := range eps {
		var words [][3]any
		for at := 0.0; at+1 <= duration; at += 0.5 {
			word := "ja"
			if ep == b && at >= 1800 {
				word = "Donaudampfschifffahrtsgesellschaft"
			}
			words = append(words, [3]any{at, at + 0.4, fmt.Sprintf("%s%d", word, len(words))})
		}
		heardEpisode(t, svc, ep, duration, words)
	}
	svc.probes = &probed{info: map[string]engine.SourceInfo{"": {Duration: duration}}}
	ctx := context.Background()
	most := func(ep string) float64 {
		st, err := svc.State(ctx, ep)
		if err != nil || st.Window == nil {
			t.Fatalf("the state of %s: %v", ep, err)
		}
		return st.Window.Most
	}
	mostA, mostB := most(a), most(b)
	if mostA <= mostB+20 {
		t.Fatalf("a reads %v s and b %v s, so b is fitted to no shorter a window", mostA, mostB)
	}
	length := math.Round((mostA + mostB) / 2)
	if err := svc.chooseWindow(a, 0, length, length, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSearch(ctx, a, 5, 20, 30, "target"); err != nil {
		t.Fatal(err)
	}
	if err := svc.chooseWindow(b, 0, length, length, true); err != nil {
		t.Fatal(err)
	}
	stB, err := svc.State(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if w := stB.Window; w.To-w.From > mostB+0.01 || w.Typed != 0 {
		t.Errorf("b's window of %v s came as %v–%v s asking for %d typed", length, w.From, w.To, w.Typed)
	}
	stA, err := svc.State(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if stA.Window.Typed != 5 {
		t.Errorf("a's window of %v s asks for %d typed after b's was fitted, not the 5 typed on it", length, stA.Window.Typed)
	}
}

// Everything the workspace does to a window at once, from several
// goroutines, while states are read: the window moved by hand, placed
// again, and the numbers of a search typed. Once all is still, the state
// says the window that is kept, fitted to what a search reads.
func TestAWindowTakesEverythingAtOnce(t *testing.T) {
	t.Parallel()
	svc, source, duration := anUnevenVideo(t)
	ctx := context.Background()
	var all sync.WaitGroup
	answers := make(chan VideoState, 5*4)
	answer := func(st VideoState, err error) {
		if err != nil {
			t.Error(err)
			return
		}
		answers <- st
	}
	for i := range 4 {
		all.Go(func() {
			from := float64(i) * 300
			answer(svc.ChooseWindow(ctx, source, from, duration, duration-from, true))
		})
		all.Go(func() { answer(svc.PlaceWindow(ctx, source)) })
		all.Go(func() { answer(svc.SetSearch(ctx, source, 1+i, 20, 30, "target")) })
		all.Go(func() { answer(svc.SetSearch(ctx, source, 0, float64(15+i*10), 30, "min")) })
		all.Go(func() { answer(svc.State(ctx, source)) })
	}
	all.Wait()
	close(answers)
	// Every answer carries a window fitted to what a search reads, never
	// the one read before another call fitted it.
	for st := range answers {
		if w := st.Window; w == nil || w.To-w.From > w.Most+0.01 || w.To > duration {
			t.Errorf("an answer of rev %d came with the window %+v", st.Rev, w)
		}
	}
	st, err := svc.State(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	w := st.Window
	if w == nil || w.To-w.From > w.Most+0.01 || w.To-w.From < w.Least-0.01 || w.To > duration {
		t.Fatalf("the window came as %+v", w)
	}
	if kept := keptWindow(svc, source); kept == nil || !sameWindow(*kept, w.KeptWindow) {
		t.Errorf("the window kept is %+v, the one in the state %+v", kept, w.KeptWindow)
	}
}
