package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"framefairy/engine"
)

const historyPlan = `{
  "source": "ep.mp4",
  "clips": [
    {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 12.0}], "words": [[10.1, 10.5, "eins"]]},
    {"id": "02", "slug": "zwei", "segments": [{"start": 20.0, "end": 22.0}], "words": [[20.1, 20.6, "zwei"]]}
  ]
}`

// anEpisodeWithAPlan is an episode in the library with one plan of two
// clips.
func anEpisodeWithAPlan(t *testing.T) (*FrameFairy, string, string) {
	t.Helper()
	svc, mine, _ := library(t)
	logs := engine.NewProject(nil, mine, svc.store.Settings().options()).LogsDir()
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(logs, "clips.json")
	if err := os.WriteFile(plan, []byte(historyPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return svc, mine, plan
}

func removed(t *testing.T, plan, id string) bool {
	t.Helper()
	_, clips, err := engine.LoadClips(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clips {
		if c.ID == id {
			return c.Rejected
		}
	}
	t.Fatalf("no clip %s", id)
	return false
}

func TestARemovedClipComesBackWithUndo(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	// The answer describes the clip for the interface, which needs the video
	// read, and this one is not a video. The edit is made either way.
	_, _ = svc.RemoveClip(ctx, mine, plan, "02", true)
	if !removed(t, plan, "02") {
		t.Fatal("the clip was not removed")
	}
	done, err := svc.Undo(mine)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Done || done.Clip != "clips.json/02" || removed(t, plan, "02") {
		t.Errorf("undo %+v, still removed %v", done, removed(t, plan, "02"))
	}
	done, err = svc.Redo(mine)
	if err != nil || !done.Done || !removed(t, plan, "02") {
		t.Errorf("redo %+v %v, removed %v", done, err, removed(t, plan, "02"))
	}
	// Nothing further back than the first thing done.
	_, _ = svc.Undo(mine)
	if done, err := svc.Undo(mine); err != nil || done.Done {
		t.Errorf("undo past the beginning: %+v %v", done, err)
	}
}

func TestTheCaptionHeightIsTakenBack(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	was := svc.store.Settings().CaptionY
	if err := svc.SetCaptionsHeight(mine, was+200); err != nil {
		t.Fatal(err)
	}
	if svc.store.Settings().CaptionY == was {
		t.Fatal("the height did not move")
	}
	if done, err := svc.Undo(mine); err != nil || !done.Done {
		t.Fatalf("undo %+v %v", done, err)
	}
	if got := svc.store.Settings().CaptionY; got != was {
		t.Errorf("height %v after undo, was %v", got, was)
	}
}

// A new edit after an undo is a new branch, and what was undone is gone
// for good, the way every editor does it.
func TestANewEditForgetsWhatWasUndone(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	_, _ = svc.RemoveClip(ctx, mine, plan, "02", true)
	_, _ = svc.Undo(mine)
	_, _ = svc.RemoveClip(ctx, mine, plan, "01", true)
	if done, _ := svc.Redo(mine); done.Done {
		t.Error("redo brought back what a new edit replaced")
	}
}

func TestUndoIsOnlyForTheLibrary(t *testing.T) {
	svc, _, home := library(t)
	other := filepath.Join(home, "not-mine.mp4")
	if _, err := svc.Undo(other); err == nil {
		t.Error("Undo answered for a file that is not in the library")
	}
	if _, err := svc.Redo(other); err == nil {
		t.Error("Redo answered for a file that is not in the library")
	}
}

// Everything the interface can do to the history at once: edits, undos and
// redos from several goroutines. Each is whole, so the plan always loads,
// and the history never holds a step twice or loses count.
func TestEditsAndUndosAtOnce(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := []string{"01", "02"}[i%2]
			switch i % 3 {
			case 0:
				_, _ = svc.RemoveClip(ctx, mine, plan, id, i%4 == 0)
			case 1:
				_, _ = svc.Undo(mine)
			case 2:
				_, _ = svc.Redo(mine)
			}
		}()
	}
	wg.Wait()
	if _, _, err := engine.LoadClips(plan); err != nil {
		t.Fatalf("the plan no longer loads: %v", err)
	}
	// Undo all the way back always ends on the plan as it began.
	for range 100 {
		if done, err := svc.Undo(mine); err != nil || !done.Done {
			break
		}
	}
	if removed(t, plan, "01") || removed(t, plan, "02") {
		t.Error("undoing everything did not end where it began")
	}
}

// The caption colours are an edit like any other: saved to the plan the
// render reads, taken back by Undo, and a colour that is not one is refused
// before anything is written.
func TestCaptionColoursAreSavedAndUndone(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	style := func() engine.Style {
		t.Helper()
		p, _, err := engine.LoadClips(plan)
		if err != nil {
			t.Fatal(err)
		}
		return engine.ResolveStyle(p.CaptionStyle())
	}
	if err := svc.SetCaptionColours(ctx, mine, plan, "#ffcc00", 0.5, "#102030", 0.25, "#00aa00", 0.6); err != nil {
		t.Fatal(err)
	}
	if s := style(); s.Primary != "&H8000CCFF" || s.BackColour != "&HBF302010" ||
		s.HighlightColour != "&H00AA00&" || s.HighlightAlpha != "66" {
		t.Errorf("the render would draw %s on %s", s.Primary, s.BackColour)
	}
	// The video preview shows the plan's own highlight, not the one of the
	// settings, which is only for a plan without one.
	if view, err := svc.Captions(plan, "01"); err != nil || view.Style.HighlightColour != "rgba(0, 170, 0, 0.6)" {
		t.Errorf("the preview's pill: %v %v", view, err)
	}
	if _, err := svc.Undo(mine); err != nil {
		t.Fatal(err)
	}
	if s := style(); s.Primary == "&H8000CCFF" || s.BackColour == "&HBF302010" {
		t.Error("undo left the colours")
	}
	// Nothing below may write the plan. Undo counts as an edit of its own,
	// so the file to compare with is the one after it.
	before, _ := os.ReadFile(plan)
	for _, bad := range [][2]string{{"red", ""}, {"", "#12345"}, {"#ffcc00\n", ""}} {
		if err := svc.SetCaptionColours(ctx, mine, plan, bad[0], 1, bad[1], 0.5, "", 1); err == nil {
			t.Errorf("%q was taken for a colour", bad)
		}
	}
	if err := svc.SetCaptionColours(ctx, mine, plan, "", 1, "", 1, "#12", 1); err == nil {
		t.Error("a highlight that is not a colour was taken")
	}
	if err := svc.SetCaptionColours(ctx, mine, filepath.Join(filepath.Dir(plan), "..", "..", "x.json"),
		"#ffffff", 1, "", 1, "", 1); err == nil {
		t.Error("a plan outside the library was written")
	}
	after, _ := os.ReadFile(plan)
	if string(before) != string(after) {
		t.Errorf("the plan is not as it was:\n%s", after)
	}
}

// The word highlight is switched off and on for a whole clip set, saved to
// the plan the render reads, shown by the video preview and taken back by
// Undo. Off means the words alone: no pill and no bounce.
func TestCaptionHighlightIsSwitchedAndUndone(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	ctx := context.Background()
	on := func() bool {
		t.Helper()
		p, _, err := engine.LoadClips(plan)
		if err != nil {
			t.Fatal(err)
		}
		return engine.ResolveStyle(p.CaptionStyle()).Highlight
	}
	if !on() {
		t.Fatal("a new plan starts without the highlight")
	}
	if err := svc.SetCaptionHighlight(ctx, mine, plan, false); err != nil {
		t.Fatal(err)
	}
	if on() {
		t.Error("the render would still draw the highlight")
	}
	if view, err := svc.Captions(plan, "01"); err != nil || view.Style.Highlight {
		t.Errorf("the video preview would still draw the highlight: %v", err)
	}
	if _, err := svc.Undo(mine); err != nil {
		t.Fatal(err)
	}
	if !on() {
		t.Error("undo left the highlight off")
	}
	if err := svc.SetCaptionHighlight(ctx, mine, filepath.Join(filepath.Dir(plan), "..", "..", "x.json"),
		false); err == nil {
		t.Error("a plan outside the library was written")
	}
}
