package main

// The path tests: what a person does with the app, from adding a video to
// a finished short, told through the driver in driver_test.go. They were
// written against the app as it was before jobs became one thing each, see
// docs/JOBS.md, and passed on it. They stayed as they were while the way
// work is run underneath changed. What Cancel leaves behind changed later,
// on purpose: a search called off says so, the way one cut off does.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"framefairy/engine"
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

// Tim moved a video's file away in Finder and opened the video again. Its
// picture said the file was gone, and a toast said it a second time,
// because the clip list asked for the file it no longer had. Issue #311.
//
// On the real disk: the file is moved the way Finder moves it, and ffprobe
// is what would have failed on it.
func TestPathAVideoMovedAwayKeepsItsClips(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	before := d.clips(ep)
	if len(before) == 0 {
		t.Fatal("no clips in the list")
	}
	if err := os.Rename(ep, ep+".away"); err != nil {
		t.Fatal(err)
	}
	after, err := d.svc.Clips(context.Background(), ep)
	if err != nil {
		t.Fatalf("the clips of a video whose file is gone: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("%d clips before the file was moved away, %d after", len(before), len(after))
	}
	for i := range min(len(before), len(after)) {
		if !slices.Equal(after[i].CropLefts, before[i].CropLefts) {
			t.Errorf("clip %s: crops %v before, %v after", before[i].Key, before[i].CropLefts, after[i].CropLefts)
		}
	}
}

func TestPathClosedWhileHearing(t *testing.T) {
	d := open(t)
	hearing := d.speech.hearing()
	ep := d.add("ep", minutes5)
	<-hearing
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

// Search pressed and the app closed at once, before the search had begun:
// it was cancelled and said nothing after the restart, where it should say
// Interrupted with Continue. The walk searching.mjs found it in CI. A few
// times over, because how far a search gets before the closing reaches it
// is a matter of timing.
func TestPathClosedTheMomentASearchIsAsked(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	for i := range 5 {
		from := float64(20 * (i + 1))
		d.svc.Search(ep, engine.PlanRequest{From: from, To: from + 60, Count: 1, Min: 5}, "")
		d.close()
		d.reopen()
		o := d.outcome(ep)
		if !o.interrupted || o.window != (window{from, from + 60}) {
			t.Fatalf("try %d: after the app was closed the moment a search was asked: %+v", i+1, o)
		}
	}
}

func TestPathClosedWhileFinding(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	ep := d.add("ep", minutes5)
	<-asked
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
	<-hearing
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
	// What was heard is kept, and Search goes on from there.
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
	<-asked
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

// The steps of the walk that broke in plan row 2.222: the model holds its
// answers, a video is added, Cancel, Continue, the model answers, and a
// second video is added. While a search finds no search hears, see
// lanes.go, so the second video waits to be transcribed until the search
// Continue started has its answer. The model answers it once it answers
// again, and the second video then gets its first clips by itself.
//
// On the real disk: ffmpeg and the episode's decoder read the videos.
func TestPathAVideoAddedOnceTheModelAnswersGetsItsClips(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	first := d.add("first", "72")
	<-asked
	d.stop(first, "")
	d.idle(first)
	asked = d.model.asking()
	d.carryOn(first)
	<-asked
	d.model.hangs(false)
	second := d.add("second", "71")
	d.idle(second)
	d.idle(first)
	for _, ep := range []string{first, second} {
		if n := len(d.clips(ep)); n == 0 {
			t.Errorf("%s has no clips", filepath.Base(ep))
		}
	}
}

// Cancel pressed the moment after Search, and reaching the Go side before
// the search that Search asked for: the search is called off as it arrives, never
// takes a turn, and says Stopped with Continue, after a restart too. Plan
// row 2.131.
//
// On the real disk: the search works beside a video ffmpeg made and reads.
func TestPathCancelBeforeTheSearchIsAsked(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	d.idle(ep)
	before := len(d.clips(ep))
	d.stop(ep, "press-1")
	d.press(ep, window{60, 240}, "press-1")
	d.idle(ep)
	if o := d.outcome(ep); !o.stopped || o.window != (window{60, 240}) {
		t.Fatalf("a search Cancel reached the Go side ahead of says %+v", o)
	}
	rec := engine.ReadSearch(disk, ep)
	if rec == nil || rec.Step != engine.StepStopped {
		t.Fatalf("its record is %+v", rec)
	}
	for _, step := range rec.Steps {
		if step.Step != engine.StepWaiting && step.Step != engine.StepStopped {
			t.Errorf("a search called off before it was asked for went on to %s", step.Step)
		}
	}
	if n := len(d.clips(ep)); n != before {
		t.Errorf("%d clips before, %d after a search that never started", before, n)
	}
	d.close()
	d.reopen()
	if o := d.outcome(ep); !o.stopped {
		t.Errorf("after a restart it says %+v", o)
	}
	// A Cancel is about the press it came after and no other: the next
	// Search runs.
	d.press(ep, window{60, 240}, "press-2")
	d.idle(ep)
	if j, _ := d.svc.jobs.find(ep, engine.JobSearch); j.State != JobDone {
		t.Errorf("New after it ended %s %s", j.State, j.Error)
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

// On the real disk: ffmpeg renders the short.
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
		t.Fatal("no short was rendered")
	}
	// The short went to the folder the settings name for shorts, and the
	// clip knows it is there: Render says Render again, and Show in folder
	// shows it, which only a file of the library may be. A file beside it
	// is not one.
	short := d.clips(ep)[0].Short
	if !slices.Contains(d.shorts(), short) {
		t.Fatalf("the clip says its short is at %q, the shorts are %v", short, d.shorts())
	}
	if !d.svc.store.Known(short) {
		t.Errorf("the short at %s is refused as no file of the library", short)
	}
	beside := filepath.Join(filepath.Dir(short), "beside.mp4")
	if err := os.WriteFile(beside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if d.svc.store.Known(beside) {
		t.Errorf("%s, which no clip rendered, is taken for a file of the library", beside)
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
	// The environment is the one way left to name another ffmpeg.
	t.Setenv("FRAMEFAIRY_FFMPEG", "/nowhere/ffmpeg")
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
	// The files are made first, because a goroutine cannot end the test
	// when making one fails, see testing.T.FailNow. Adding them is what
	// is done at once.
	a, b := d.episode("a", minutes5), d.episode("b", minutes5)
	var wg sync.WaitGroup
	for _, path := range []string{a, b} {
		wg.Go(func() {
			if added, err := d.svc.addEpisodes([]string{path}); err != nil || len(added) != 1 {
				t.Errorf("adding %s: %v", path, err)
			}
		})
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
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
	// Both episodes' clips are named alike, so the first one's short is
	// where the second one's would go, and it is still not the second's.
	for _, c := range d.clips(b) {
		if c.Short != "" || c.Rendered {
			t.Errorf("%s of the second episode, never rendered, says it is, at %s", c.ID, c.Short)
		}
	}
}

// Search on a window further on, in the part not heard yet, hears on from
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

// A video added gets as many clips as its first window suggests, not the
// target typed for a window of another length. The same length keeps it.
//
// On the real disk: the search works beside a video ffmpeg made and reads.
func TestPathATargetStaysWithItsWindow(t *testing.T) {
	d := open(t)
	if err := d.svc.SetSearch(3, 1800, 20, 30); err != nil {
		t.Fatal(err)
	}
	asked := func(ep string) int {
		d.idle(ep)
		timings := engine.ReadTimings(disk, ep)
		if len(timings) != 1 {
			t.Fatalf("%d searches of %s", len(timings), ep)
		}
		return timings[0].Count
	}
	if n, want := asked(d.add("other", minutes5)), engine.SuggestedCount(300, 20, 30); n != want || n == 3 {
		t.Errorf("five minutes asked for %d, want the %d they suggest", n, want)
	}
	if err := d.svc.SetSearch(3, 300, 20, 30); err != nil {
		t.Fatal(err)
	}
	if n := asked(d.add("same", minutes5)); n != 3 {
		t.Errorf("five minutes asked for %d, with 3 typed for five minutes", n)
	}
}
