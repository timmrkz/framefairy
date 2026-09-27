package main

// The path tests: what a person does with the app, from adding a video to
// a finished short, told through the driver in driver_test.go. They were
// written against the app as it was before jobs became one thing each, see
// docs/JOBS.md, and passed on it. They stayed as they were while the way
// work is run underneath changed. What Cancel leaves behind changed later,
// on purpose: a search called off says so, the way one cut off does.

import (
	"strings"
	"sync"
	"testing"
)

// The episodes of these tests are five minutes long, which the stand-in
// speech model hears in about a second: long enough to close the app
// while it hears.
const minutes5 = "300"

func TestPathAVideoAddedGetsItsFirstClips(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	if _, done := d.heard(ep); !done {
		t.Error("the episode was not heard to the end of its first window")
	}
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("no clips in the list")
	}
	if o := d.outcome(ep); o != (outcome{}) {
		t.Errorf("a search that found its clips says %+v", o)
	}
}

func TestPathClosedWhileHearing(t *testing.T) {
	d := open(t)
	hearing := d.speech.hearing()
	ep := d.add("ep", minutes5)
	within(t, hearing, "hearing")
	d.close()
	heardBefore, _ := d.heard(ep)
	given := d.speech.seconds()

	d.reopen()
	o := d.outcome(ep)
	if !o.interrupted {
		t.Fatalf("after the app was closed while it heard: %+v", o)
	}
	d.carryOn(ep)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("Continue found no clips")
	}
	if o := d.outcome(ep); o != (outcome{}) {
		t.Errorf("after Continue: %+v", o)
	}
	// Carried on from what was heard, not from the start. A chunk at the
	// edge may be heard again, and nothing more.
	again := d.speech.seconds() - given
	t.Logf("heard %.0f s before the app closed, %.0f s after", heardBefore, again)
	if heardBefore <= 0 {
		t.Error("nothing heard was kept when the app closed")
	}
	if again > 300-heardBefore+30 {
		t.Errorf("%.0f s were heard before the app closed, and %.0f s after", heardBefore, again)
	}
}

func TestPathClosedWhileFinding(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	ep := d.add("ep", minutes5)
	within(t, asked, "asking the model")
	d.close()

	d.reopen()
	if o := d.outcome(ep); !o.interrupted {
		t.Fatalf("after the app was closed while the model was asked: %+v", o)
	}
	given := d.speech.seconds()
	d.model.hangs(false)
	d.carryOn(ep)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("Continue found no clips")
	}
	if d.speech.seconds() != given {
		t.Error("the episode was heard again")
	}
}

func TestPathCancelWhileHearing(t *testing.T) {
	d := open(t)
	hearing := d.speech.hearing()
	ep := d.add("ep", minutes5)
	within(t, hearing, "hearing")
	d.cancel(ep)
	d.idle(ep)
	// Called off, it says so where its clips would be, with Continue, the
	// way a search cut off by the app closing does. It used to say
	// nothing, and Tim found the column bare after Cancel.
	if o := d.outcome(ep); !o.stopped {
		t.Errorf("a search called off says %+v", o)
	}
	if n := len(d.clips(ep)); n != 0 {
		t.Errorf("%d clips from a search called off while it heard", n)
	}
	// What was heard is kept, and New goes on from there.
	heardBefore, _ := d.heard(ep)
	given := d.speech.seconds()
	d.search(ep, firstWindow)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("New after Cancel found no clips")
	}
	again := d.speech.seconds() - given
	t.Logf("heard %.0f s before Cancel, %.0f s after", heardBefore, again)
	if heardBefore <= 0 {
		t.Error("nothing heard was kept on Cancel")
	}
	if again > 300-heardBefore+30 {
		t.Errorf("%.0f s were heard before Cancel, and %.0f s after", heardBefore, again)
	}
}

func TestPathCancelWhileFinding(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	ep := d.add("ep", minutes5)
	within(t, asked, "asking the model")
	d.cancel(ep)
	d.idle(ep)
	if o := d.outcome(ep); !o.stopped {
		t.Errorf("a search called off says %+v", o)
	}
	// And after the app is opened again, too.
	d.close()
	d.reopen()
	if o := d.outcome(ep); !o.stopped {
		t.Errorf("a search called off says %+v after a restart", o)
	}
	d.model.hangs(false)
	d.carryOn(ep)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Error("Continue after Cancel found no clips")
	}
	if _, done := d.heard(ep); !done {
		t.Error("the transcript was not kept")
	}
}

func TestPathFailureWhileHearing(t *testing.T) {
	d := open(t)
	d.speech.breaks(errBroken)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	o := d.outcome(ep)
	if !strings.Contains(o.failed, errBroken.Error()) {
		t.Errorf("a search whose speech model failed says %+v", o)
	}
	// Continue once the model is there again.
	d.speech.breaks(nil)
	d.carryOn(ep)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("Continue found no clips")
	}
	if o := d.outcome(ep); o != (outcome{}) {
		t.Errorf("after Continue: %+v", o)
	}
}

func TestPathFailureWhileFinding(t *testing.T) {
	d := open(t)
	d.model.fails(true)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	if o := d.outcome(ep); o.failed == "" {
		t.Errorf("a search whose model failed says %+v", o)
	}
	d.model.fails(false)
	d.carryOn(ep)
	d.idle(ep)
	if n := len(d.clips(ep)); n == 0 {
		t.Fatal("Continue found no clips")
	}
}

func TestPathRenderedToTheEnd(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	clips := d.clips(ep)
	if len(clips) == 0 {
		t.Fatal("no clips to render")
	}
	d.render(ep, clips[0].Plan)
	d.idle(ep)
	if reason := d.failedRender(ep); reason != "" {
		t.Fatalf("the render failed: %s", reason)
	}
	if len(d.shorts()) == 0 {
		t.Error("no short was rendered")
	}
}

func TestPathARenderThatFailsSaysWhy(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	clips := d.clips(ep)
	if len(clips) == 0 {
		t.Fatal("no clips to render")
	}
	if err := d.svc.store.UpdateSettings(func(s *Settings) { s.FFmpeg = "/nowhere/ffmpeg" }); err != nil {
		t.Fatal(err)
	}
	d.render(ep, clips[0].Plan)
	d.idle(ep)
	if d.failedRender(ep) == "" {
		t.Error("a render with no ffmpeg says nothing")
	}
}

// Two episodes added at once, and a render of the first while the second
// is still searched, all under the race detector.
func TestPathTwoEpisodesAndARender(t *testing.T) {
	d := open(t)
	var a, b string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a = d.add("a", minutes5) }()
	go func() { defer wg.Done(); b = d.add("b", minutes5) }()
	wg.Wait()
	d.idle(a)
	clips := d.clips(a)
	if len(clips) == 0 {
		t.Fatal("no clips in the first episode")
	}
	d.render(a, clips[0].Plan)
	d.idle(a)
	d.idle(b)
	if len(d.clips(b)) == 0 {
		t.Error("no clips in the second episode")
	}
	if reason := d.failedRender(a); reason != "" || len(d.shorts()) == 0 {
		t.Errorf("the render: %q, %d shorts", reason, len(d.shorts()))
	}
}

// New on a window further on, in the part not heard yet, hears on from
// where the transcript ends, and never from the start of the episode. Tim
// saw a search of the half hour after the one heard start transcribing
// from the beginning, with almost six minutes left for a window that takes
// less than one.
func TestPathNewOnALaterWindowHearsOnlyWhatIsNew(t *testing.T) {
	d := open(t)
	// What the hearing reports, from a queue that tells it, set before any
	// work runs.
	var mu sync.Mutex
	var reports []float64
	listening := false
	d.svc.jobs = newQueue(d.svc.store, func(u JobUpdate) {
		mu.Lock()
		defer mu.Unlock()
		if listening && u.Job.Step == "hearing" && u.Job.Progress != nil && u.Job.Progress.Covered > 0 {
			reports = append(reports, u.Job.Progress.Covered, u.Job.Progress.Fraction)
		}
	}, func(string) {})
	t.Cleanup(func() { d.svc.jobs.shutDown() })
	path := d.episode("ep", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{path}); err != nil {
		t.Fatal(err)
	}
	d.search(path, window{0, 100})
	d.idle(path)
	heard, _ := d.heard(path)
	if heard < 99 || heard > 101 {
		t.Fatalf("the first window of 100 s was heard to %.0f s", heard)
	}
	given := d.speech.seconds()
	mu.Lock()
	listening = true
	mu.Unlock()
	d.search(path, window{150, 250})
	d.idle(path)
	again := d.speech.seconds() - given
	if again > 150+30 {
		t.Errorf("a window from 150 to 250, with 100 heard, heard %.0f s", again)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) == 0 {
		t.Fatal("the hearing never said how far it was")
	}
	if first := reports[0]; first < 100 {
		t.Errorf("the hearing said it had got to %.0f s, where the transcript was at 100", first)
	}
	// How far it has come, and with it the time left, is measured to the
	// end of the window, 250, and not to the end of the episode, 300: at
	// the end of the window it is all the way, where to the end of the
	// episode it was three quarters of it.
	for i := 0; i+1 < len(reports); i += 2 {
		covered, share := reports[i], reports[i+1]
		if covered >= 245 && share < 0.95 {
			t.Errorf("at %.0f s of a window to 250 it said it was %.2f of the way", covered, share)
		}
	}
	t.Logf("heard %.0f s again, reports (covered, share): %v", again, reports)
}
