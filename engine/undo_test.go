package engine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
)

// edited runs an edit between two snapshots of the episode and gives what
// it changed, the way the app records every edit.
func edited(t *testing.T, fsys afero.Fs, logs string, edit func() error) *Change {
	t.Helper()
	before := TakeSnapshot(fsys, nil, logs)
	if err := edit(); err != nil {
		t.Fatal(err)
	}
	change := ChangeBetween(before, TakeSnapshot(fsys, nil, logs))
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
	change := edited(t, fsys, logs, func() error { return SetRejected(fsys, nil, path, "01", true) })
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
		if err := SetRejected(fsys, nil, path, "01", true); err != nil {
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
		return SetWordText(fsys, path, "01", 10.6, "zwo", tr)
	})
	if clipsOf(t, fsys, path)["01"].Corrections[heardKey(10600)] != "zwo" {
		t.Fatal("the word was not corrected")
	}
	// A word of another clip corrected since is not the undo's to take.
	if err := SetWordText(fsys, path, "02", 20.1, "fünf!", tr); err != nil {
		t.Fatal(err)
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	clips := clipsOf(t, fsys, path)
	if _, ok := clips["01"].Corrections[heardKey(10600)]; ok {
		t.Error("the correction is still there")
	}
	if clips["02"].Corrections[heardKey(20100)] != "fünf!" {
		t.Error("a correction made since was lost")
	}
	// The clip's words say zwei again.
	for _, w := range Said(clips["01"], tr.Of(clips["01"]).Words) {
		if w.Text == "zwo" {
			t.Errorf("the clip still says zwo: %v", tr.Of(clips["01"]).Words)
		}
	}
}

// A caption shown earlier is shown on time again by the undo, and earlier
// again by the redo.
func TestACaptionTimeIsUndoneAndRedone(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	change := edited(t, fsys, logs, func() error {
		_, err := SetCaptionTime(fsys, path, "01", 10.0, "start", 9.8, false, editableTranscript(), nil)
		return err
	})
	moved := func() bool {
		got := clipsOf(t, fsys, path)["01"].CaptionTimes[heardKey(10000)].Start
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
	change := edited(t, fsys, logs, func() error { return SetRejected(fsys, nil, path, "02", true) })
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

// A search over a window searched before writes its plan anew, in place of
// the one there was, and that can happen while an edit is being made,
// between the two pictures the history takes. The plan written anew is
// not the edit's. Undoing the edit put the clips of the search before back
// into the new plan, beside the new search's clips, and its id with them
// (#309).
func TestUndoLeavesAPlanASearchWroteAnewDuringTheEdit(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	anew := `{"source": "ep.mp4", "plan_id": "p2", "clips": [
  {"id": "t10-01", "slug": "neu", "segments": [{"start": 30.0, "end": 32.0}], "words": [[30.1, 30.5, "neu"]]}]}`
	change := edited(t, fsys, logs, func() error {
		if err := SetRejected(fsys, nil, path, "01", true); err != nil {
			return err
		}
		return writePlanFile(fsys, path, []byte(anew))
	}).LeaveOutNewClips()
	if change != nil {
		if _, err := change.Undo(); err != nil && !errors.Is(err, ErrChangedSince) {
			t.Fatal(err)
		}
	}
	plan, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range clips {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, " ") != "t10-01" {
		t.Errorf("after the undo the plan holds %v, the new search's clip alone", ids)
	}
	if id, _ := plan.Raw["plan_id"].(string); id != "p2" {
		t.Errorf("after the undo the plan is %q, not the new search's p2", id)
	}
}

// A search takes a removed clip out of its plan once it has made it again,
// see DropRemoved, and that too can land during an edit. Undoing the edit
// puts back what the edit did, and not the clip the search took out.
func TestUndoLeavesAClipASearchTookOutDuringTheEdit(t *testing.T) {
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	if err := SetRejected(fsys, nil, path, "02", true); err != nil {
		t.Fatal(err)
	}
	change := edited(t, fsys, logs, func() error {
		if err := SetRejected(fsys, nil, path, "01", true); err != nil {
			return err
		}
		return DropRemoved(fsys, path, "02")
	}).LeaveOutNewClips()
	if change == nil {
		t.Fatal("the edit itself was lost with the clip the search took out")
	}
	if _, err := change.Undo(); err != nil {
		t.Fatal(err)
	}
	clips := clipsOf(t, fsys, path)
	if clips["01"].Rejected {
		t.Error("the undo did not put the clip back")
	}
	if _, back := clips["02"]; back {
		t.Error("the undo brought back the clip the search took out")
	}
	if _, err := change.Redo(); err != nil {
		t.Fatal(err)
	}
	if clips := clipsOf(t, fsys, path); !clips["01"].Rejected || len(clips) != 1 {
		t.Errorf("redo: %+v", clips)
	}
}

// A search lands its clips in a plan, and takes out a removed clip it has
// made again, while the app edits the plan, removes clips and puts them
// back, and undoes and redoes, from several goroutines at once, the way
// the app's history does it: every edit between two snapshots, one at a
// time, and a step that would write over something changed since given
// up with the steps before it. No clip the search landed is lost, none it
// took out comes back, no id is there twice, and the plan always loads
// (#309).
func TestASearchLandsWhileItsPlanIsEditedUndoneAndRemoved(t *testing.T) {
	t.Parallel()
	fsys, path := editablePlanPath(t)
	logs := filepath.Dir(path)
	if err := SetRejected(fsys, nil, path, "02", true); err != nil {
		t.Fatal(err)
	}

	// The app's history of the episode, see cmd/framefairy-app/history.go.
	var hist sync.Mutex
	var undo, redo []*Change
	edit := func(fn func() error) {
		hist.Lock()
		defer hist.Unlock()
		before := TakeSnapshot(fsys, nil, logs)
		_ = fn()
		if change := ChangeBetween(before, TakeSnapshot(fsys, nil, logs)).LeaveOutNewClips(); change != nil {
			undo, redo = append(undo, change), nil
		}
	}
	step := func(back bool) {
		hist.Lock()
		defer hist.Unlock()
		from, to := &undo, &redo
		if !back {
			from, to = to, from
		}
		if len(*from) == 0 {
			return
		}
		change := (*from)[len(*from)-1]
		var err error
		if back {
			_, err = change.Undo()
		} else {
			_, err = change.Redo()
		}
		if errors.Is(err, ErrChangedSince) {
			undo, redo = nil, nil
			return
		}
		if err != nil {
			t.Errorf("undo or redo: %v", err)
			return
		}
		*from, *to = (*from)[:len(*from)-1], append(*to, change)
	}

	// The search lands a clip every few reads of the hands, so the two
	// meet however slow the machine is.
	const landing = 16
	var reads atomic.Int64
	// read has a value once a hand has read the plan since it was last
	// taken.
	read := make(chan struct{}, 1)
	searched := make(chan struct{})
	go func() {
		defer close(searched)
		for i := 1; i <= landing; i++ {
			for reads.Load() < int64(3*i) {
				<-read
			}
			clip := PlanClip{ID: fmt.Sprintf("s%02d", i), Slug: "neu",
				Segments: []PlanSegment{{Start: PyFloat(40 + 3*i), End: PyFloat(42 + 3*i)}}}
			if err := appendClip(fsys, path, clip); err != nil {
				t.Errorf("landing %s: %v", clip.ID, err)
			}
			if i == landing/2 {
				if err := DropRemoved(fsys, path, "02"); err != nil {
					t.Errorf("taking out 02: %v", err)
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for hand := range 6 {
		wg.Go(func() {
			for k := 0; ; k++ {
				select {
				case <-searched:
					return
				default:
				}
				_, clips, err := LoadClips(fsys, path)
				if err != nil {
					t.Errorf("the plan does not load: %v", err)
					return
				}
				reads.Add(1)
				select {
				case read <- struct{}{}:
				default:
				}
				// 02 stays removed until the search takes it out.
				id := clips[(hand+k)%len(clips)].ID
				if id == "02" {
					id = "01"
				}
				switch (hand + k) % 5 {
				case 0, 1:
					edit(func() error { return SetRejected(fsys, nil, path, id, k%2 == 0) })
				case 2:
					edit(func() error { return SetCaptionY(fsys, path, id, float64(k%9)/10) })
				case 3:
					step(true)
				case 4:
					step(false)
				}
			}
		})
	}
	wg.Wait()

	// Undone all the way back, as far as the history goes, and done again.
	for range 2 * len(undo) {
		step(true)
	}
	for range 2 * len(redo) {
		step(false)
	}
	plan, clips, err := LoadClips(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, c := range clips {
		seen[c.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("%s is in the plan %d times", id, n)
		}
	}
	for i := 1; i <= landing; i++ {
		if seen[fmt.Sprintf("s%02d", i)] == 0 {
			t.Errorf("s%02d, landed by the search, is gone", i)
		}
	}
	if seen["02"] > 0 {
		t.Error("02, taken out by the search, came back")
	}
	if seen["01"] == 0 {
		t.Error("01 is gone, and no search took it out")
	}
	if id, _ := plan.Raw["plan_id"].(string); id != "p1" {
		t.Errorf("the plan is %q now", id)
	}
}
