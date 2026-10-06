package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// The order: what was named, then what is beside the program, and nothing
// else.
func TestTheEnvironmentAlwaysWins(t *testing.T) {
	t.Run("a named tool is taken as it stands", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_FFMPEG", "/somewhere/else/ffmpeg")
		if got := ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg"); got != "/somewhere/else/ffmpeg" {
			t.Errorf("got %q", got)
		}
	})

	// Nothing named and nothing beside the test binary, so there is no
	// ffmpeg, whatever the search path holds. It used to be left to the
	// search path, which ran whichever ffmpeg was first on it.
	t.Run("otherwise nothing, never the search path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_FFMPEG", "")
		if got := ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg"); got != "" {
			t.Errorf("got %q from somewhere other than beside the program", got)
		}
		if _, err := FindTool("FRAMEFAIRY_FFMPEG", "ffmpeg"); err == nil {
			t.Error("no ffmpeg, and no reason given")
		}
	})
}

// The app as it ships takes no program and no model from the
// environment, see named: the tests and the command line do.
func TestTheShippedAppTakesNothingFromTheEnvironment(t *testing.T) {
	t.Setenv("FRAMEFAIRY_FFMPEG", "/somewhere/else/ffmpeg")
	if got := namedWhen(true, "FRAMEFAIRY_FFMPEG"); got != "" {
		t.Errorf("the shipped app took %q from the environment", got)
	}
	if got := namedWhen(false, "FRAMEFAIRY_FFMPEG"); got != "/somewhere/else/ffmpeg" {
		t.Errorf("the tests and the command line got %q", got)
	}
	if shipped {
		t.Error("the tests are built as the shipped app")
	}
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

// llama-server is decided the same way ffmpeg is, and for the same reason:
// a customer has no Homebrew and no terminal, so the only one they will
// ever have is the one shipped beside the program. It used to be looked
// for on the search path alone, which is a search path a shipped app does
// not have.
func TestLlamaServerIsFoundTheSameWayAsFfmpeg(t *testing.T) {
	t.Run("a named one is taken as it stands", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", "/somewhere/else/llama-server")
		if got := LlamaServerPath(); got != "/somewhere/else/llama-server" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("otherwise nothing, never the search path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", "")
		if got := LlamaServerPath(); got != "" {
			t.Errorf("got %q from somewhere other than beside the program", got)
		}
		if HasLlamaServer() {
			t.Error("said yes with nothing beside the program")
		}
	})

	// What HasLlamaServer answers has to follow from that and from nothing
	// else, or the setup screen and the thing that starts the server can
	// disagree about the same machine.
	t.Run("what the setup asks follows the same path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", filepath.Join(t.TempDir(), "not-here"))
		if HasLlamaServer() {
			t.Error("said yes about a file that is not there")
		}
	})
}

// ffprobe is looked for beside an ffmpeg that was chosen, under the name
// the system gives it, and one named on its own wins. The app looked for
// it its own way and missed ffprobe.exe.
func TestFFprobeIsFoundBesideTheFFmpegChosen(t *testing.T) {
	for _, c := range []struct{ ffmpeg, ffprobe string }{
		{"ffmpeg", "ffprobe"},
		{"ffmpeg.exe", "ffprobe.exe"},
		{"FFMPEG.EXE", "ffprobe.exe"},
	} {
		dir := t.TempDir()
		for _, name := range []string{c.ffmpeg, c.ffprobe} {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		e := NewEngine(NewLog(io.Discard, false, false))
		e.UseTools(filepath.Join(dir, c.ffmpeg), "")
		if want := filepath.Join(dir, c.ffprobe); e.FFprobe != want {
			t.Errorf("beside %s: ffprobe is %s, want %s", c.ffmpeg, e.FFprobe, want)
		}
	}

	e := NewEngine(NewLog(io.Discard, false, false))
	was := e.FFprobe
	e.UseTools("", "")
	if e.FFprobe != was {
		t.Error("no ffmpeg chosen changed ffprobe")
	}
	e.UseTools(filepath.Join(t.TempDir(), "ffmpeg"), "/opt/ffprobe")
	if e.FFprobe != "/opt/ffprobe" {
		t.Errorf("an ffprobe named on its own lost to a guess: %s", e.FFprobe)
	}
}

// A tool beside the program is run only once it is checked to be the file
// the program was built with: the SHA-256 make builds in, see toolSums. One
// with no sum built in, or one that does not match, is not run, and one
// changed after it was checked is checked again.
func TestAToolBesideTheProgramIsCheckedBeforeItRuns(t *testing.T) {
	t.Setenv("FRAMEFAIRY_FFMPEG", "")
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\necho the real one\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("#!/bin/sh\necho the real one\n"))
	was := toolSums
	t.Cleanup(func() { toolSums = was })

	toolSums = ""
	if _, err := findToolIn(dir, "FRAMEFAIRY_FFMPEG", "ffmpeg"); err == nil {
		t.Error("ran a tool no sum was built in for")
	}

	toolSums = "ffprobe=00,ffmpeg=" + hex.EncodeToString(sum[:])
	got, err := findToolIn(dir, "FRAMEFAIRY_FFMPEG", "ffmpeg")
	if err != nil || got != ffmpeg {
		t.Fatalf("the right file was refused: %q, %v", got, err)
	}

	// The same name, other bytes, a moment later: what somebody who put
	// their own file there would leave.
	later := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\necho somebody else\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(ffmpeg, later, later)
	if _, err := findToolIn(dir, "FRAMEFAIRY_FFMPEG", "ffmpeg"); err == nil {
		t.Error("ran a tool that changed after it was checked")
	} else if !strings.Contains(err.Error(), "not the one this program was built with") {
		t.Errorf("refused for another reason: %v", err)
	}

	// Named in the environment, it is somebody's own choice and is taken.
	t.Setenv("FRAMEFAIRY_FFMPEG", ffmpeg)
	if got, err := findToolIn(dir, "FRAMEFAIRY_FFMPEG", "ffmpeg"); err != nil || got != ffmpeg {
		t.Errorf("a tool named in the environment was refused: %q, %v", got, err)
	}
}

// What the settings say about each tool: the version it says it is,
// as each of them says it.
func TestAToolSaysItsVersion(t *testing.T) {
	for out, want := range map[string]string{
		"ffmpeg version n8.1.3 Copyright (c) 2000-2026 the FFmpeg developers\nbuilt with clang": "8.1.3",
		"ffprobe version n8.1.3 Copyright (c) 2007-2026 the FFmpeg developers":                  "8.1.3",
		"ffmpeg version 6.1.1-3ubuntu5 Copyright (c) 2000-2023 the FFmpeg developers":           "6.1.1-3ubuntu5",
		"0.00.001 I srv init\nversion: 0.4.1-dev (build 11105, commit 348f853)\nbuilt with":     "b11105",
		"nothing at all": "",
	} {
		if got := parseToolVersion(out); got != want {
			t.Errorf("%q read as %q, want %q", out, got, want)
		}
	}
}

// The settings read a tool again whenever they look, and say whether it is
// the file the program was built with: a tool changed since it was last
// run is caught by the look, not only by the next run.
func TestInspectingAToolReadsItAgain(t *testing.T) {
	t.Setenv("FRAMEFAIRY_FFMPEG", "")
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	body := []byte("#!/bin/sh\necho 'ffmpeg version n8.1.3 Copyright'\n")
	if err := os.WriteFile(ffmpeg, body, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	was := toolSums
	t.Cleanup(func() { toolSums = was })
	toolSums = "ffmpeg=" + hex.EncodeToString(sum[:])

	s := inspectToolIn(t.Context(), dir, "FRAMEFAIRY_FFMPEG", "ffmpeg")
	if !s.Checked || s.Err != nil || s.SHA256 != hex.EncodeToString(sum[:]) || s.Version != "8.1.3" {
		t.Fatalf("the right file read as %+v", s)
	}
	// Changed in place with the same size and time, which a stamp alone
	// would not see. The look reads it again.
	info, _ := os.Stat(ffmpeg)
	other := []byte("#!/bin/sh\necho 'ffmpeg version n6.6.6 Copyright'\n")
	if err := os.WriteFile(ffmpeg, other, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(ffmpeg, info.ModTime(), info.ModTime())
	s = inspectToolIn(t.Context(), dir, "FRAMEFAIRY_FFMPEG", "ffmpeg")
	if s.Checked || s.Err == nil || s.Version != "" {
		t.Errorf("a changed file read as %+v", s)
	}
}
