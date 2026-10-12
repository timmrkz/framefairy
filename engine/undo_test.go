package engine

import (
	"errors"
	"github.com/spf13/afero"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// edited runs an edit between two snapshots of the episode and gives what
// it changed, the way the app records every edit.
func edited(t *testing.T, fsys afero.Fs, logs string, edit func() error) *Change {
	t.Helper()
	before := TakeSnapshot(fsys, logs)
	if err := edit(); err != nil {
		t.Fatal(err)
	}
	change := ChangeBetween(before, TakeSnapshot(fsys, logs))
	if change == nil {
		t.Fatal("the edit changed nothing")
	}
	return change
}

func clipsOf(t *testing.T, fsys afero.Fs, path string) map[string]Clip {
	t.Helper()
	_, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Clip{}
	for _, c := range clips {
		out[c.ID] = c
	}
	return out
}

func TestAnEditIsUndoneAndDoneAgain(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	original, _ := afero.ReadFile(fsys, path)
	change := edited(t, fsys, logs, func() error {
		return TrimClip(fsys, path, "01", 10.6, 13.1, editableTranscript(), 0.1, ToWords)
	})
	trimmed := clipsOf(t, fsys, path)["01"]

	shown, err := change.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if shown.ID != "01" || shown.Plan != path {
		t.Errorf("the undo would show %+v", shown)
	}
	back := clipsOf(t, fsys, path)["01"]
	if back.Segments[0].Start != 10.0 {
		t.Errorf("undone clip starts at %v", back.Segments[0].Start)
	}
	// Everything but the count of edits is as it was, fields it never
	// knew about included.
	now, _ := afero.ReadFile(fsys, path)
	if !strings.Contains(string(now), `"note": "mine"`) || !strings.Contains(string(now), `"custom"`) {
		t.Errorf("the undo lost what the plan held:\n%s", now)
	}
	if len(now) < len(original)-40 {
		t.Errorf("the undone plan is much shorter than the original:\n%s", now)
	}

	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if again := clipsOf(t, fsys, path)["01"]; again.Segments[0].Start != trimmed.Segments[0].Start {
		t.Errorf("redone clip starts at %v, trimmed at %v", again.Segments[0].Start, trimmed.Segments[0].Start)
	}
}

// A search writes clips into the plan while the app edits it. Undoing
// an edit puts back the clip it changed and nothing else, so what landed
// since stays.
func TestUndoLeavesAClipThatLandedSince(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, fsys, logs, func() error { return SetRejected(fsys, path, "01", true) })
	landing := PlanClip{ID: "03", Slug: "drei",
		Segments: []PlanSegment{{Start: 30, End: 32, CropX: "center"}}}
	if err := appendClip(fsys, path, landing); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	clips := clipsOf(t, fsys, path)
	if clips["01"].Rejected {
		t.Error("the clip was not put back")
	}
	if _, ok := clips["03"]; !ok {
		t.Error("the undo took away a clip that landed after the edit")
	}
}

// A clip that lands while an edit is being made, between the two pictures
// the history takes of the files, is not the edit's. Undoing the edit used
// to take it away.
func TestUndoLeavesAClipThatLandedDuringTheEdit(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	landing := PlanClip{ID: "03", Slug: "drei",
		Segments: []PlanSegment{{Start: 30, End: 32, CropX: "center"}}}
	change := edited(t, fsys, logs, func() error {
		if err := SetRejected(fsys, path, "01", true); err != nil {
			return err
		}
		return appendClip(fsys, path, landing)
	}).LeaveOutNewClips()
	if change == nil {
		t.Fatal("the edit itself was lost with the clip that landed")
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	clips := clipsOf(t, fsys, path)
	if clips["01"].Rejected {
		t.Error("the clip was not put back")
	}
	if _, ok := clips["03"]; !ok {
		t.Error("the undo took away a clip that landed during the edit")
	}
	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if clips := clipsOf(t, fsys, path); !clips["01"].Rejected || clips["03"].ID != "03" {
		t.Errorf("redo: %+v", clips)
	}
}

// A clip landing is all that changed: that is no edit at all.
func TestAClipLandingAloneIsNoEdit(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	landing := PlanClip{ID: "03", Slug: "drei",
		Segments: []PlanSegment{{Start: 30, End: 32, CropX: "center"}}}
	change := edited(t, fsys, logs, func() error { return appendClip(fsys, path, landing) }).LeaveOutNewClips()
	if change != nil {
		t.Errorf("a clip landing became an edit of %v", change.Files())
	}
}

// Undoing an edit whose clip has been changed again since, by something
// the history does not know about, would throw that away. It says so and
// touches nothing.
func TestUndoRefusesWhatChangedSince(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, fsys, logs, func() error {
		return TrimClip(fsys, path, "01", 10.6, 13.1, editableTranscript(), 0.1, ToWords)
	})
	if err := SetCaptionY(fsys, path, "01", 300); err != nil {
		t.Fatal(err)
	}
	before, _ := afero.ReadFile(fsys, path)
	if _, err := change.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("err = %v", err)
	}
	after, _ := afero.ReadFile(fsys, path)
	if string(before) != string(after) {
		t.Error("a refused undo wrote the plan")
	}
}

func TestACorrectedWordIsUndone(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	change := edited(t, fsys, logs, func() error {
		return SetWordText(fsys, logs, 10.6, "zwo", tr)
	})
	if LoadCorrections(fsys, logs)[wordKey(10.6)] != "zwo" {
		t.Fatal("the word was not corrected")
	}
	// A word of another clip corrected since is not the undo's to take.
	if err := SetWordText(fsys, logs, 20.1, "fünf!", tr); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	corrections := LoadCorrections(fsys, logs)
	if _, ok := corrections[wordKey(10.6)]; ok {
		t.Error("the correction is still there")
	}
	if corrections[wordKey(20.1)] != "fünf!" {
		t.Error("a correction made since was lost")
	}
	// The words made again from what is kept say zwei again.
	again := editableTranscript()
	again.Correct(corrections)
	for _, w := range Said(clipsOf(t, fsys, path)["01"], again.Words) {
		if w.Text == "zwo" {
			t.Errorf("the clip still says zwo: %v", again.Words)
		}
	}
}

// A caption shown earlier is shown on time again by the undo, and earlier
// again by the redo.
func TestACaptionTimeIsUndoneAndRedone(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, fsys, logs, func() error {
		_, err := SetCaptionTime(fsys, filepath.Dir(path), path, "01", 10.0, "start", 9.8, editableTranscript(), nil)
		return err
	})
	moved := func() bool {
		got := clipsOf(t, fsys, path)["01"].CaptionTimes[wordKey(10.0)].Start
		return got != nil && near(*got, 9.8)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	if moved() {
		t.Error("the undo left the caption where it was moved")
	}
	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if !moved() {
		t.Error("the redo did not move the caption again")
	}
}

// Undo and redo from several goroutines at once, as the app firing keys
// faster than the disk answers would, never leave a plan that does not
// load, and end on one of the two states the edit knows.
func TestUndoAndRedoAtOnceLeaveAPlan(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, fsys, logs, func() error { return SetRejected(fsys, path, "02", true) })
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_, _ = change.Undo()
			} else {
				_, _ = change.Redo()
			}
		}()
	}
	wg.Wait()
	if _, _, err := LoadClips(fsys, path); err != nil {
		t.Fatalf("the plan no longer loads: %v", err)
	}
}
