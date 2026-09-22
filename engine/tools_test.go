package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// A bundled app carries its own ffmpeg, built without libx264 so the build
// is LGPL. Finding Homebrew's on the search path instead undoes the whole
// point of it: that one is a different build with different encoders, so
// the short a customer gets is not the short that was tested.
func TestAToolBesideTheProgramWinsOverTheSearchPath(t *testing.T) {
	// The shape of a real bundle, so the macOS branch is read on whatever
	// machine this runs on rather than only on a Mac.
	root := t.TempDir()
	macos := filepath.Join(root, "framefairy.app", "Contents", "MacOS")
	resources := filepath.Join(root, "framefairy.app", "Contents", "Resources")
	for _, dir := range []string{macos, resources} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	put := func(dir, name string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("beside the program", func(t *testing.T) {
		want := put(macos, "ffmpeg", 0o755)
		if got := toolIn("darwin", macos, "ffmpeg"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("nothing there is nothing, not a guess", func(t *testing.T) {
		if got := toolIn("darwin", macos, "ffprobe"); got != "" {
			t.Errorf("invented %q", got)
		}
	})

	// A bundle puts what is not a program in Contents/Resources, so both
	// are looked at. This only holds inside a bundle, where the program is
	// in MacOS, which is what makes the pair meaningful.
	t.Run("Contents/Resources counts too", func(t *testing.T) {
		want := put(resources, "ffprobe", 0o755)
		if got := toolIn("darwin", macos, "ffprobe"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("Resources is not looked at outside a bundle", func(t *testing.T) {
		plain := t.TempDir()
		beside := filepath.Join(filepath.Dir(plain), "Resources")
		_ = os.MkdirAll(beside, 0o755)
		put(beside, "ffmpeg", 0o755)
		if got := toolIn("darwin", plain, "ffmpeg"); got != "" {
			t.Errorf("reached out of the folder for %q", got)
		}
	})

	// A file of the right name that cannot be run is worse than none,
	// because it hides the one on the search path that works.
	t.Run("a file that cannot be run is not a tool", func(t *testing.T) {
		dir := t.TempDir()
		put(dir, "ffmpeg", 0o644)
		if got := toolIn("darwin", dir, "ffmpeg"); got != "" {
			t.Errorf("took a file it cannot run: %q", got)
		}
	})

	t.Run("a folder of that name is not a tool", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "ffmpeg"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := toolIn("darwin", dir, "ffmpeg"); got != "" {
			t.Errorf("took a folder: %q", got)
		}
	})

	// Windows names it with the extension and has no executable bit.
	t.Run("windows adds the extension", func(t *testing.T) {
		dir := t.TempDir()
		want := put(dir, "ffmpeg.exe", 0o644)
		if got := toolIn("windows", dir, "ffmpeg"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if got := toolIn("linux", dir, "ffmpeg"); got != "" {
			t.Errorf("linux took a .exe: %q", got)
		}
	})

	t.Run("no folder at all is not a failure", func(t *testing.T) {
		if got := toolIn("darwin", "", "ffmpeg"); got != "" {
			t.Errorf("got %q", got)
		}
	})
}

// The order: what was named, then what is beside the program, then the bare
// name for the search path to answer.
func TestTheEnvironmentAlwaysWins(t *testing.T) {
	t.Run("a named tool is taken as it stands", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_FFMPEG", "/somewhere/else/ffmpeg")
		if got := ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg"); got != "/somewhere/else/ffmpeg" {
			t.Errorf("got %q", got)
		}
	})

	// Nothing named and nothing beside the test binary, so the bare name is
	// left for the search path to resolve. A path invented here would be a
	// path that does not exist.
	t.Run("otherwise the bare name, for the search path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_FFMPEG", "")
		got := ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg")
		if got != "ffmpeg" && !filepath.IsAbs(got) {
			t.Errorf("got %q, which is neither the bare name nor a real path", got)
		}
	})
}

// NewEngine has to go through all of that rather than reaching for the
// environment itself, which is what it used to do.
func TestNewEngineTakesItsToolsTheSameWay(t *testing.T) {
	t.Setenv("FRAMEFAIRY_FFMPEG", "/named/ffmpeg")
	t.Setenv("FRAMEFAIRY_FFPROBE", "/named/ffprobe")
	e := NewEngine(NewLog(os.Stderr, false, false))
	if e.FFmpeg != "/named/ffmpeg" || e.FFprobe != "/named/ffprobe" {
		t.Errorf("ffmpeg %q, ffprobe %q", e.FFmpeg, e.FFprobe)
	}
}
