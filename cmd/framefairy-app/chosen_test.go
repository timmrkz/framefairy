package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"framefairy/engine"
)

// The key of a clip comes from the interface, so it is checked before it is
// written down, and checked again when it is read back. It never reaches a
// path, but a file in an episode's folder is a file all the same.
func TestOnlyAKeyIsEverKept(t *testing.T) {
	cases := []struct {
		what string
		key  string
		ok   bool
	}{
		{"a key", "clips/abc123", true},
		{"nothing, which means no clip", "", true},
		{"a key with spaces in its names", "clips 5400 7200/moment two", true},
		{"a way out of the folder", "../../etc/passwd", false},
		{"a way out with one slash", "..%2f..", false},
		{"no clip after the slash", "clips/", false},
		{"no clip set before it", "/abc123", false},
		{"no slash at all", "clips", false},
		{"two slashes, which is a path and not a key", "clips/a/b", false},
		{"a newline, to write a second line into the file", "clips/a\nb", false},
		{"a line ending of the other kind", "clips/a\rb", false},
		{"something that goes on forever", "clips/" + string(make([]byte, 400)), false},
	}
	for _, c := range cases {
		if looksLikeClipKey(c.key) != c.ok {
			t.Errorf("%s: %q was %v, wanted %v", c.what, c.key, !c.ok, c.ok)
		}
	}
}

// What is written down comes back, and a folder holding anything else
// comes back as no clip rather than as an error the interface has to
// handle.
func TestTheChosenClipSurvivesAndNothingElseDoes(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &store{dir: t.TempDir(), settings: defaultSettings(), episodes: []string{source}}
	s := &FrameFairy{store: st}

	if got := s.ChosenClip(source); got != "" {
		t.Errorf("an episode nobody has opened gave %q, wanted nothing", got)
	}
	if err := s.ChooseClip(source, "clips/abc123"); err != nil {
		t.Fatal(err)
	}
	if got := s.ChosenClip(source); got != "clips/abc123" {
		t.Errorf("got %q, wanted clips/abc123", got)
	}
	if err := s.ChooseClip(source, "clips/def456"); err != nil {
		t.Fatal(err)
	}
	if got := s.ChosenClip(source); got != "clips/def456" {
		t.Errorf("choosing another clip gave %q, wanted clips/def456", got)
	}

	if err := s.ChooseClip(source, "../../escape"); err == nil {
		t.Error("a key that is a path was taken")
	}
	if got := s.ChosenClip(source); got != "clips/def456" {
		t.Errorf("a refused key changed what was kept, now %q", got)
	}

	// A file written by hand, or by something else entirely.
	file := filepath.Join(engine.WorkDir(source), chosenFile)
	for _, junk := range []string{"", "not json at all", `{"clip":"../../etc/passwd"}`, `{"clip":123}`, `{}`} {
		if err := os.WriteFile(file, []byte(junk), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := s.ChosenClip(source); got != "" {
			t.Errorf("a file holding %q gave %q, wanted nothing", junk, got)
		}
	}

	// An episode that is not in the library is not read and not written.
	other := filepath.Join(dir, "other.mp4")
	if err := s.ChooseClip(other, "clips/abc"); err == nil {
		t.Error("a file outside the library was written to")
	}
	if got := s.ChosenClip(other); got != "" {
		t.Errorf("a file outside the library gave %q", got)
	}
}

// The window is kept beside the clip, each changed without losing the
// other, and one that could not be a window is neither kept nor read back.
func TestTheWindowIsKeptBesideTheClip(t *testing.T) {
	s, source := chosenApp(t)
	if got := s.ChosenWindow(source); got != nil {
		t.Fatalf("a fresh episode had a window: %+v", got)
	}
	if err := s.ChooseClip(source, "clips/abc123"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseWindow(source, 120, 180, 60, false); err != nil {
		t.Fatal(err)
	}
	if got := s.ChosenWindow(source); got == nil || *got != (KeptWindow{From: 120, To: 180, Length: 60}) {
		t.Errorf("the window came back as %+v", got)
	}
	if got := s.ChosenClip(source); got != "clips/abc123" {
		t.Errorf("choosing a window lost the clip: %q", got)
	}
	if err := s.ChooseClip(source, "clips/def456"); err != nil {
		t.Fatal(err)
	}
	if got := s.ChosenWindow(source); got == nil || got.From != 120 {
		t.Errorf("choosing a clip lost the window: %+v", got)
	}
	// Cut short at the end of the episode, it is shorter than it was made.
	if err := s.ChooseWindow(source, 300, 325, 60, false); err != nil {
		t.Errorf("a window cut short: %v", err)
	}
	for _, bad := range [][3]float64{
		{180, 120, 60},
		{-1, 60, 61},
		{0, math.Inf(1), 60},
		{0, math.NaN(), 60},
		{0, 60, 30},
		{0, engine.MaxEpisodeSeconds + 1, engine.MaxEpisodeSeconds + 1},
	} {
		if err := s.ChooseWindow(source, bad[0], bad[1], bad[2], false); err == nil {
			t.Errorf("%v was kept", bad)
		}
	}
	if got := s.ChosenWindow(source); got == nil || got.From != 300 {
		t.Errorf("a refused window changed the kept one: %+v", got)
	}
	// Whatever else is written into the file is not a window.
	file := filepath.Join(engine.WorkDir(source), chosenFile)
	if err := os.WriteFile(file, []byte(`{"clip":"clips/a","window":{"from":50,"to":10,"length":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.ChosenWindow(source); got != nil {
		t.Errorf("a window written backwards was read back: %+v", got)
	}
	if got := s.ChosenClip(source); got != "clips/a" {
		t.Errorf("a bad window took the clip with it: %q", got)
	}
}

// A clip chosen while the window is let go, the two at once and many
// times over: neither change ever loses the other.
func TestAClipAndAWindowChosenAtOnce(t *testing.T) {
	s, source := chosenApp(t)
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := s.ChooseClip(source, fmt.Sprintf("clips/c%d", i)); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := s.ChooseWindow(source, float64(i), float64(i+60), 60, i%2 == 0); err != nil {
				t.Error(err)
			}
			_ = s.ChosenWindow(source)
		}()
	}
	wg.Wait()
	if s.ChosenClip(source) == "" || s.ChosenWindow(source) == nil {
		t.Errorf("one change lost the other: clip %q, window %+v", s.ChosenClip(source), s.ChosenWindow(source))
	}
}

func chosenApp(t *testing.T) (*FrameFairy, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &store{dir: t.TempDir(), settings: defaultSettings(), episodes: []string{source}}
	return &FrameFairy{store: st}, source
}

// A window moved by hand is a step: undo puts it back and redo moves it
// again, whatever a search did with it in between. One the app moved by
// itself is no step.
func TestTheWindowMovedByHandIsUndone(t *testing.T) {
	s, source := chosenApp(t)
	if err := s.ChooseWindow(source, 0, 60, 60, false); err != nil {
		t.Fatal(err)
	}
	if err := s.ChooseWindow(source, 120, 180, 60, true); err != nil {
		t.Fatal(err)
	}
	// A search moves it on by itself.
	if err := s.ChooseWindow(source, 180, 240, 60, false); err != nil {
		t.Fatal(err)
	}
	done, err := s.Undo(source)
	if err != nil || !done.Done || done.Window == nil || *done.Window != (KeptWindow{0, 60, 60}) {
		t.Fatalf("undo gave %+v, %v", done, err)
	}
	if got := s.ChosenWindow(source); got == nil || got.From != 0 {
		t.Errorf("after undo the window is kept at %+v", got)
	}
	done, err = s.Redo(source)
	if err != nil || done.Window == nil || done.Window.From != 120 {
		t.Fatalf("redo gave %+v, %v", done, err)
	}
	if done, _ := s.Redo(source); done.Done {
		t.Error("a search moving the window on was a step")
	}
	// The same place again is no step.
	if err := s.ChooseWindow(source, 120, 180, 60, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(source); err != nil {
		t.Fatal(err)
	}
	if done, _ := s.Undo(source); done.Done {
		t.Error("a window put where it already was became a step")
	}
}
