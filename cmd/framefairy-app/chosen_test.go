package main

import (
	"os"
	"path/filepath"
	"testing"

	"framefairy/engine"
)

// The key of a clip comes from the window, so it is checked before it is
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
// comes back as no clip rather than as an error the window has to handle.
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
