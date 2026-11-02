package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/runlog"
)

// A state of the video as the Go side reads it, made by hand: its rev,
// its clips by key, of the clip set of each key, and its jobs.
func stateOf(rev uint64, keys []string, jobs ...Job) VideoState {
	st := VideoState{Rev: rev, Jobs: jobs}
	for _, k := range keys {
		plan := k[:len(k)-2]
		st.Clips = append(st.Clips, ClipEntry{Key: k, Plan: plan})
	}
	return st
}

func searchJob(state string) Job {
	return Job{ID: "s1", State: state, Result: "new", JobRecord: engine.JobRecord{Kind: engine.JobSearch}}
}

// The rules the app chooses a clip by as a search runs, opens and lands
// a card, each from the states the window would be sent, see choose.go.
func TestTheAppChoosesTheClipsASearchFinds(t *testing.T) {
	old := []string{"old/1", "old/2"}
	with := func(more ...string) []string { return append(append([]string{}, old...), more...) }
	type step struct {
		st      VideoState
		pick    string
		playing bool
		want    string
	}
	cases := []struct {
		what  string
		steps []step
	}{
		{"a video opened on nothing chosen opens on its first clip", []step{
			{st: stateOf(1, old), want: "old/1"},
		}},
		{"a search's first clip is chosen, and the next is not", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), want: "old/1"},
			{st: stateOf(3, with("new/2"), searchJob(JobRunning)), want: "new/2"},
			{st: stateOf(4, with("new/1", "new/2"), searchJob(JobRunning)), want: "new/2"},
		}},
		{"at its end the earliest it found is chosen in place of its own", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), want: "old/1"},
			{st: stateOf(3, with("new/2"), searchJob(JobRunning)), want: "new/2"},
			{st: stateOf(4, with("new/1", "new/2"), searchJob(JobDone)), want: "new/1"},
		}},
		{"a clip a hand chose while it runs stays chosen", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), want: "old/1"},
			{pick: "old/2", st: stateOf(3, with("new/2"), searchJob(JobRunning)), want: "old/2"},
			{st: stateOf(4, with("new/1", "new/2"), searchJob(JobDone)), want: "old/2"},
		}},
		{"nothing is chosen while the video plays", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), playing: true, want: "old/1"},
			{st: stateOf(3, with("new/2"), searchJob(JobRunning)), playing: true, want: "old/1"},
			{st: stateOf(4, with("new/1", "new/2"), searchJob(JobDone)), playing: true, want: "old/1"},
		}},
		{"a search that stopped chooses nothing at its end", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), playing: true, want: "old/1"},
			{st: stateOf(3, with("new/2"), searchJob(JobRunning)), playing: true, want: "old/1"},
			{st: stateOf(4, with("new/1", "new/2"), searchJob(JobStopped)), want: "old/1"},
		}},
		{"a search that failed chooses nothing at its end", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), want: "old/1"},
			{st: stateOf(3, with("new/1"), searchJob(JobFailed)), want: "old/1"},
		}},
		{"a window searched again chooses the first clip of its clip set", []step{
			{st: stateOf(1, []string{"old/1", "new/1", "new/2"}), want: "old/1"},
			{st: stateOf(2, []string{"old/1", "new/1", "new/2"}, searchJob(JobRunning)), want: "old/1"},
			{st: stateOf(3, []string{"old/1", "new/1", "new/2"}, searchJob(JobDone)), want: "new/1"},
		}},
		{"a state older than the newest chooses nothing", []step{
			{st: stateOf(5, old), want: "old/1"},
			{st: stateOf(4, with("new/1"), searchJob(JobRunning)), want: "old/1"},
		}},
		{"a state older than the newest names no clip it does not hold", []step{
			{st: stateOf(1, old), want: "old/1"},
			{st: stateOf(2, old, searchJob(JobRunning)), want: "old/1"},
			{st: stateOf(4, with("new/2"), searchJob(JobRunning)), want: "new/2"},
			{st: stateOf(3, old, searchJob(JobRunning)), want: "old/1"},
		}},
		{"a clip chosen that is gone gives way to the first", []step{
			{st: stateOf(1, old), want: "old/1"},
			{pick: "gone/1", st: stateOf(2, old), want: "old/1"},
		}},
		{"a card chosen becomes its clip once it is written", []step{
			{st: stateOf(1, old), want: "old/1"},
			{pick: "c1/1", st: stateOf(2, old, Job{ID: "c1", State: JobRunning, JobRecord: engine.JobRecord{Kind: engine.JobClip},
				Underway: []runlog.Underway{{N: 1, Clip: "hand/1"}}}), want: "c1/1"},
			{st: stateOf(3, with("hand/1"), Job{ID: "c1", State: JobRunning, JobRecord: engine.JobRecord{Kind: engine.JobClip},
				Underway: []runlog.Underway{{N: 1, Clip: "hand/1"}}}), want: "hand/1"},
		}},
		{"a card done is its result", []step{
			{st: stateOf(1, old), want: "old/1"},
			{pick: "c1/1", st: stateOf(2, with("hand/1"), Job{ID: "c1", State: JobDone, Result: "hand/1", JobRecord: engine.JobRecord{Kind: engine.JobClip}}), want: "hand/1"},
		}},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			s, eps := memoryLibrary(t, "ep.mp4")
			ep := eps[0]
			for i, st := range c.steps {
				if st.pick != "" {
					if err := s.pick(ep, st.pick); err != nil {
						t.Fatal(err)
					}
				}
				if err := s.Playing(ep, st.playing); err != nil {
					t.Fatal(err)
				}
				if got, _ := s.chosenFor(ep, st.st); got != st.want {
					t.Fatalf("step %d chose %q, not %q", i+1, got, st.want)
				}
			}
		})
	}
}

// The video opened on a clip chosen last that is still there opens on
// it, and the first clip chosen as it opens is not written: the file
// says what a hand or a search chose.
func TestAVideoOpensOnTheClipChosenLast(t *testing.T) {
	s, eps := memoryLibrary(t, "ep.mp4", "other.mp4")
	if err := s.pick(eps[0], "old/2"); err != nil {
		t.Fatal(err)
	}
	s.forget(eps[0])
	if got, _ := s.chosenFor(eps[0], stateOf(1, []string{"old/1", "old/2"})); got != "old/2" {
		t.Errorf("opened on %q, not on the clip chosen last", got)
	}
	if got, _ := s.chosenFor(eps[1], stateOf(1, []string{"old/1", "old/2"})); got != "old/1" {
		t.Errorf("opened on %q, not on the first clip", got)
	}
	if on := readChosen(s.store.files, eps[1]); on.Clip != "" {
		t.Errorf("the first clip chosen as it opened was written, %q", on.Clip)
	}
}

// Removing the clip chosen chooses the one beside it, the next, or the
// one before when it was the last, and putting it back while that one is
// still chosen chooses it again. An undo chooses the clip it changed.
func TestARemovedClipsNeighbourIsChosenAndAnUndoChoosesItsClip(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	first, second := "clips.json/01", "clips.json/02"
	if _, err := svc.ChooseClip(context.Background(), mine, first); err != nil {
		t.Fatal(err)
	}
	// The answer needs the video read, and this one is not a video.
	// The edit is made either way.
	_, _ = svc.RemoveClip(ctx, mine, plan, "01", true)
	if got := chosenClip(svc, mine); got != second {
		t.Fatalf("removing the clip chosen chose %q, not the next", got)
	}
	_, _ = svc.RemoveClip(ctx, mine, plan, "01", false)
	if got := chosenClip(svc, mine); got != first {
		t.Fatalf("putting it back chose %q, not the clip put back", got)
	}
	// The last removed chooses the one before it.
	if _, err := svc.ChooseClip(context.Background(), mine, second); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.RemoveClip(ctx, mine, plan, "02", true)
	if got := chosenClip(svc, mine); got != first {
		t.Fatalf("removing the last clip chose %q, not the one before", got)
	}
	// A hand chose another since, the clip removed beside it, so putting
	// it back chooses nothing.
	if _, err := svc.ChooseClip(context.Background(), mine, "clips.json/02"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChooseClip(context.Background(), mine, first); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.RemoveClip(ctx, mine, plan, "02", false)
	if got := chosenClip(svc, mine); got != first {
		t.Fatalf("putting back a clip after another was chosen chose %q", got)
	}
	// Removing a clip that is not chosen chooses nothing, and its undo
	// chooses it.
	if _, err := svc.ChooseClip(context.Background(), mine, first); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.RemoveClip(ctx, mine, plan, "02", true)
	if got := chosenClip(svc, mine); got != first {
		t.Fatalf("removing a clip not chosen chose %q", got)
	}
	if done, err := svc.Undo(mine); err != nil || !done.Done {
		t.Fatalf("the remove was not undone: %v", err)
	}
	if got := chosenClip(svc, mine); got != second {
		t.Errorf("the undo chose %q, not the clip it changed", got)
	}
}

// A search seen first in a state that already holds its first clip,
// because the state was read late, still chooses that clip: the clips
// there were are noted as its work begins, not when a state first sees
// it, and so at its end its earliest.
func TestASearchSeenLateStillChoosesItsFirstClip(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	if st, err := svc.State(ctx, mine); err != nil || st.Chosen != "clips.json/01" {
		t.Fatalf("the video opened on %q, %v", st.Chosen, err)
	}
	svc.searchBegins(mine)
	found := filepath.Join(filepath.Dir(plan), "clips-found.json")
	if err := afero.WriteFile(svc.store.files, found, []byte(historyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "s1", Episode: mine, State: JobRunning, JobRecord: engine.JobRecord{Kind: engine.JobSearch}}
	svc.jobs.mu.Lock()
	svc.jobs.jobs = append(svc.jobs.jobs, &job)
	svc.jobs.mu.Unlock()
	st, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if st.Chosen != "clips-found.json/01" {
		t.Errorf("a search first seen with its clip written chose %q, not its clip", st.Chosen)
	}
}

// A pick answers with the state read after it, newer than a state read
// before it, which says the clip picked is chosen.
func TestAPickAnswersWithTheStateAfterIt(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	before, err := svc.State(context.Background(), mine)
	if err != nil {
		t.Fatal(err)
	}
	after, err := svc.ChooseClip(context.Background(), mine, "clips.json/02")
	if err != nil {
		t.Fatal(err)
	}
	if after.Rev <= before.Rev {
		t.Errorf("the pick answered with a state at %d, not after the one at %d", after.Rev, before.Rev)
	}
	if after.Chosen != "clips.json/02" {
		t.Errorf("the state after the pick says %q is chosen", after.Chosen)
	}
}

// A card picked after its clip was written, because the answer to the
// press that made it came late, is answered with the clip chosen, rule 7.
func TestACardPickedAfterItsClipWasWrittenIsAnsweredWithTheClip(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	ctx := context.Background()
	if _, err := svc.State(ctx, mine); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "c1", Episode: mine, State: JobDone, Result: "clips.json/02", JobRecord: engine.JobRecord{Kind: engine.JobClip}}
	svc.jobs.mu.Lock()
	svc.jobs.jobs = append(svc.jobs.jobs, &job)
	svc.jobs.mu.Unlock()
	st, err := svc.ChooseClip(ctx, mine, "c1/1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Chosen != "clips.json/02" {
		t.Errorf("the card picked after its clip was written was answered with %q chosen, not its clip", st.Chosen)
	}
}

// Everything the window can do to what is chosen at once, from several
// goroutines, while states are read: picks, plays and pauses, removes
// and states, under the race detector.
func TestWhatIsChosenTakesEverythingAtOnce(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if _, err := svc.ChooseClip(context.Background(), mine, fmt.Sprintf("clips.json/0%d", 1+i%2)); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if err := svc.Playing(mine, i%2 == 0); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			if _, err := svc.State(ctx, mine); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, _ = svc.RemoveClip(ctx, mine, plan, "02", i%2 == 0)
		})
	}
	wg.Wait()
	st, err := svc.State(ctx, mine)
	if err != nil {
		t.Fatal(err)
	}
	if st.Chosen != chosenClip(svc, mine) {
		t.Errorf("the state says %q is chosen, the app holds %q", st.Chosen, chosenClip(svc, mine))
	}
}
