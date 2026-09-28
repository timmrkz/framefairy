package main

// A clip made by hand, In or Out, is a job like a search: it runs beside
// one, Cancel leaves it Stopped with Continue, the app closing leaves it
// cut off with Continue, and Continue finishes it. What it writes, Undo
// takes back.

import (
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/engine"
)

// quiet adds an episode to the library without its first search, so a
// test hears only what a clip made by hand hears. The part it hears is as
// wide as the settings allow, so it takes long enough to be called off.
func (d *desk) quiet(name, seconds string) string {
	d.t.Helper()
	path := d.episode(name, seconds)
	if _, err := d.svc.store.AddEpisodes([]string{path}); err != nil {
		d.t.Fatal(err)
	}
	d.svc.jobs.openEpisode(path)
	if err := d.svc.store.UpdateSettings(func(s *Settings) { s.IslandMargin = engine.MaxIslandMargin }); err != nil {
		d.t.Fatal(err)
	}
	return path
}

// hand is the episode's clip made by hand, as the queue has it.
func (d *desk) hand(path string) Job {
	j, _ := d.svc.jobs.find(path, engine.JobHand)
	return j
}

// step waits until the clip made by hand is in a step.
func (d *desk) step(path, step string) {
	d.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if j := d.hand(path); j.Step == step && j.State == JobRunning {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	d.t.Fatalf("the clip made by hand never came to %s: %+v", step, d.hand(path))
}

// handClips are the clips of the list made by hand.
func (d *desk) handClips(path string) []ClipEntry {
	var out []ClipEntry
	for _, c := range d.clips(path) {
		if strings.HasPrefix(c.Key, engine.HandPlanName+"/") {
			out = append(out, c)
		}
	}
	return out
}

func TestPathAClipMadeByHandWhileASearchHears(t *testing.T) {
	d := open(t)
	hearing := d.speech.hearing()
	ep := d.add("ep", minutes5)
	within(t, hearing, "hearing")
	made := d.svc.MakeClip(ep, 250, false)
	if made.Kind != engine.JobHand || made.Record != engine.HandID || made.At != 250 {
		t.Fatalf("the clip made by hand is queued as %+v", made)
	}
	d.idle(ep)
	j := d.hand(ep)
	if j.State != JobDone || j.Result == "" {
		t.Fatalf("the clip made by hand ended as %+v", j)
	}
	clips := d.handClips(ep)
	if len(clips) != 1 || clips[0].Key != j.Result {
		t.Fatalf("the list holds %+v, the job made %s", clips, j.Result)
	}
	if len(d.clips(ep)) < 2 {
		t.Error("the search beside it found nothing")
	}
	// Undo takes the clip back, and only the clip.
	if done, err := d.svc.Undo(ep); err != nil || !done.Done {
		t.Fatalf("undo: %+v %v", done, err)
	}
	if n := len(d.handClips(ep)); n != 0 {
		t.Errorf("undo left %d clips made by hand", n)
	}
}

func TestPathCancelAClipMadeByHand(t *testing.T) {
	d := open(t)
	ep := d.quiet("ep", minutes5)
	d.svc.MakeClip(ep, 150, true)
	d.step(ep, engine.StepHearing)
	d.svc.CancelJob(d.hand(ep).ID)
	d.idle(ep)
	j := d.hand(ep)
	if j.State != JobInterrupted || j.Step != engine.StepStopped {
		t.Fatalf("a clip made by hand called off says %+v", j)
	}
	if n := len(d.handClips(ep)); n != 0 {
		t.Fatalf("%d clips made by hand after Cancel", n)
	}
	given := d.speech.seconds()
	d.svc.Continue(j.ID)
	d.idle(ep)
	if j := d.hand(ep); j.State != JobDone || len(d.handClips(ep)) != 1 {
		t.Fatalf("Continue on the clip made by hand ended as %+v", j)
	}
	if again := d.speech.seconds() - given; again > 300 {
		t.Errorf("Continue heard %.0f s of a five minute episode", again)
	}
	for _, rec := range engine.ReadJobs(ep) {
		if rec.Kind == engine.JobHand {
			t.Errorf("the record stays after the clip is made: %+v", rec)
		}
	}
}

func TestPathClosedWhileMakingAClipByHand(t *testing.T) {
	d := open(t)
	ep := d.quiet("ep", minutes5)
	d.svc.MakeClip(ep, 150, false)
	d.step(ep, engine.StepHearing)
	d.close()
	d.reopen()
	j := d.hand(ep)
	if j.State != JobInterrupted || j.At != 150 || j.Backward {
		t.Fatalf("after the app was closed while it made a clip: %+v", j)
	}
	d.svc.Continue(j.ID)
	d.idle(ep)
	if j := d.hand(ep); j.State != JobDone || len(d.handClips(ep)) != 1 {
		t.Fatalf("Continue after the app was closed ended as %+v", j)
	}
}

// Everything the window can do to a clip made by hand, at once, while a
// search runs: make, cancel, continue, list, find.
func TestClipsMadeByHandFromEverywhereAtOnce(t *testing.T) {
	d := open(t)
	ep := d.add("ep", minutes5)
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			for k := range 8 {
				switch (i + k) % 5 {
				case 0:
					d.svc.MakeClip(ep, float64(60+30*i), k%2 == 0)
				case 1:
					if j := d.hand(ep); j.ID != "" {
						d.svc.CancelJob(j.ID)
					}
				case 2:
					if j := d.hand(ep); j.State == JobInterrupted {
						d.svc.Continue(j.ID)
					}
				case 3:
					_ = d.svc.jobs.list()
				case 4:
					if j, ok := d.svc.jobs.find(ep, engine.JobSearch); ok {
						d.svc.CancelJob(j.ID)
					}
				}
			}
		})
	}
	wg.Wait()
	d.idle(ep)
	running := 0
	for _, j := range d.svc.jobs.list() {
		if j.Kind == engine.JobHand && (j.State == JobRunning || j.State == JobQueued) {
			running++
		}
	}
	if running != 0 {
		t.Errorf("%d clips made by hand still running", running)
	}
	if j := d.hand(ep); j.State == JobInterrupted {
		d.svc.Continue(j.ID)
		d.idle(ep)
		if j := d.hand(ep); j.State != JobDone {
			t.Errorf("the last clip made by hand, carried on, ended as %+v", j)
		}
	}
}
