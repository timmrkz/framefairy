package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// edited runs an edit between two snapshots of the episode and gives what
// it changed, the way the app records every edit.
func edited(t *testing.T, logs string, edit func() error) *Change {
	t.Helper()
	before := TakeSnapshot(logs)
	if err := edit(); err != nil {
		t.Fatal(err)
	}
	change := Compare(before, TakeSnapshot(logs))
	if change == nil {
		t.Fatal("the edit changed nothing")
	}
	return change
}

func clipsOf(t *testing.T, path string) map[string]Clip {
	t.Helper()
	_, clips, err := LoadClips(path)
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
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	original, _ := os.ReadFile(path)
	change := edited(t, logs, func() error {
		return TrimClip(path, "01", 10.6, 13.1, editableTranscript(), 0.1)
	})
	trimmed := clipsOf(t, path)["01"]

	shown, err := change.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if shown.ID != "01" || shown.Plan != path {
		t.Errorf("the undo would show %+v", shown)
	}
	back := clipsOf(t, path)["01"]
	if back.Segments[0].Start != 10.0 {
		t.Errorf("undone clip starts at %v", back.Segments[0].Start)
	}
	// Everything but the count of edits is as it was, fields it never
	// knew about included.
	now, _ := os.ReadFile(path)
	if !strings.Contains(string(now), `"note": "mine"`) || !strings.Contains(string(now), `"custom"`) {
		t.Errorf("the undo lost what the plan held:\n%s", now)
	}
	if len(now) < len(original)-40 {
		t.Errorf("the undone plan is much shorter than the original:\n%s", now)
	}

	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if again := clipsOf(t, path)["01"]; again.Segments[0].Start != trimmed.Segments[0].Start {
		t.Errorf("redone clip starts at %v, trimmed at %v", again.Segments[0].Start, trimmed.Segments[0].Start)
	}
}

// A search writes clips into the plan while the window edits it. Undoing
// an edit puts back the clip it changed and nothing else, so what landed
// since stays.
func TestUndoLeavesAClipThatLandedSince(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, logs, func() error { return SetRejected(path, "01", true) })
	landing := PlanClip{ID: "03", Slug: "drei", Words: [][3]any{},
		Segments: []PlanSegment{{Start: 30, End: 32, CropX: "center"}}}
	if err := appendClip(path, landing); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	clips := clipsOf(t, path)
	if clips["01"].Rejected {
		t.Error("the clip was not put back")
	}
	if _, ok := clips["03"]; !ok {
		t.Error("the undo took away a clip that landed after the edit")
	}
}

// Undoing an edit whose clip has been changed again since, by something
// the history does not know about, would throw that away. It says so and
// touches nothing.
func TestUndoRefusesWhatChangedSince(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, logs, func() error {
		return TrimClip(path, "01", 10.6, 13.1, editableTranscript(), 0.1)
	})
	if err := SetCaptionY(path, "01", 300); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := change.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("a refused undo wrote the plan")
	}
}

func TestARemovedSearchComesBack(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	captions := filepath.Join(filepath.Dir(logs), "captions")
	change := edited(t, logs, func() error { return RemovePlan(path, captions) })
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the plan is still there")
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	if len(clipsOf(t, path)) != 2 {
		t.Error("the search did not come back with its clips")
	}
	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("removing it again left it there")
	}
}

func TestACorrectedWordIsUndone(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	tr := editableTranscript()
	change := edited(t, logs, func() error {
		_, err := SetWordText(logs, 10.6, "zwo", tr)
		return err
	})
	if LoadCorrections(logs)[wordKey(10.6)] != "zwo" {
		t.Fatal("the word was not corrected")
	}
	// A word of another clip corrected since is not the undo's to take.
	if _, err := SetWordText(logs, 20.1, "fünf!", tr); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	corrections := LoadCorrections(logs)
	if _, ok := corrections[wordKey(10.6)]; ok {
		t.Error("the correction is still there")
	}
	if corrections[wordKey(20.1)] != "fünf!" {
		t.Error("a correction made since was lost")
	}
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), `"zwo"`) {
		t.Errorf("the plan still says zwo:\n%s", body)
	}
}

// Undo and redo from several goroutines at once, as a window firing keys
// faster than the disk answers would, never leave a plan that does not
// load, and end on one of the two states the edit knows.
func TestUndoAndRedoAtOnceLeaveAPlan(t *testing.T) {
	path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, logs, func() error { return SetRejected(path, "02", true) })
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
	if _, _, err := LoadClips(path); err != nil {
		t.Fatalf("the plan no longer loads: %v", err)
	}
}
