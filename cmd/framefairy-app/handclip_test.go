package main

import (
	"strings"
	"testing"
	"time"

	"framefairy/engine"
)

// madeByHand are the clips of an episode made with I and O.
func (d *desk) madeByHand(path string) []ClipEntry {
	var out []ClipEntry
	for _, c := range d.clips(path) {
		if strings.HasPrefix(c.Key, engine.HandPlanName+"/") {
			out = append(out, c)
		}
	}
	return out
}

// waitFor waits until a job is over and hands back how it ended.
func (d *desk) waitFor(id string) Job {
	d.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range d.svc.jobs.list() {
			if j.ID == id && j.State != JobQueued && j.State != JobRunning {
				return j
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.t.Fatalf("job %s never ended", id)
	return Job{}
}

// I and O pressed while a search is still finding make their clips there
// and then: each has its card from the moment it is pressed, at the
// playhead, none waits for the search, and each lands in the clips made by
// hand with an id of its own.
func TestPathClipsMadeByHandWhileASearchFinds(t *testing.T) {
	d := open(t)
	d.model.hangs(true)
	asked := d.model.asking()
	ep := d.add("hand", minutes5)
	within(t, asked, "the first search asking the model")

	var jobs []Job
	for _, press := range []struct {
		at  float64
		out bool
	}{{20, false}, {90, false}, {200, true}} {
		var j Job
		if press.out {
			j = d.out(ep, press.at)
		} else {
			j = d.in(ep, press.at)
		}
		if j.State != JobRunning || len(j.Underway) != 1 || j.Underway[0].Start != press.at {
			t.Fatalf("pressed at %v, the job is %+v", press.at, j)
		}
		jobs = append(jobs, j)
	}
	var keys []string
	for _, j := range jobs {
		done := d.waitFor(j.ID)
		if done.State != JobDone || len(done.Underway) != 0 {
			t.Fatalf("a clip made by hand ended %+v", done)
		}
		keys = append(keys, done.Result)
	}
	if s, ok := d.svc.jobs.find(ep, engine.JobSearch); !ok || s.State != JobRunning {
		t.Errorf("the search is %+v, still finding", s)
	}
	made := d.madeByHand(ep)
	if len(made) != 3 {
		t.Fatalf("%d clips made by hand", len(made))
	}
	listed := map[string]bool{}
	for _, c := range made {
		listed[c.Key] = true
	}
	for _, key := range keys {
		if !listed[key] {
			t.Errorf("%s is not in the list: %v", key, listed)
		}
	}
	d.model.hangs(false)
	d.cancel(ep)
	d.idle(ep)
}

// The app closed on a clip made by hand while its part of the episode is
// being heard says so where the clip would have been, after it opens
// again, and Continue makes the clip, hearing nothing twice.
func TestPathClosedWhileAClipIsHeard(t *testing.T) {
	d := open(t)
	ep := d.episode("late", minutes5)
	if _, err := d.svc.store.AddEpisodes([]string{ep}); err != nil {
		t.Fatal(err)
	}
	hearing := d.speech.hearing()
	j := d.in(ep, 250)
	within(t, hearing, "hearing for the clip")
	d.close()
	given := d.speech.seconds()

	d.reopen()
	var stopped *Job
	for _, k := range d.svc.jobs.list() {
		if k.Episode == ep && k.Kind == engine.JobClip {
			stopped = &k
		}
	}
	if stopped == nil || stopped.State != JobInterrupted || stopped.At != 250 ||
		len(stopped.Underway) != 1 || stopped.Underway[0].Start != 250 {
		t.Fatalf("after the app closed on %s: %+v", j.ID, stopped)
	}
	again := d.svc.Continue(stopped.ID)
	done := d.waitFor(again.ID)
	if done.State != JobDone {
		t.Fatalf("Continue ended %+v", done)
	}
	if made := d.madeByHand(ep); len(made) != 1 || made[0].Start > 250 {
		t.Fatalf("made %+v", made)
	}
	if heard := d.speech.seconds(); heard > 300+30 {
		t.Errorf("%.0f s heard for a 300 s episode, %.0f s of them before the app closed", heard, given)
	}
	if left := engine.ReadJobs(ep); len(left) != 0 {
		t.Errorf("records left: %+v", left)
	}
}
