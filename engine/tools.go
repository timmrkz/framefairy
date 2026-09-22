package engine

import (
	"os"
	"path/filepath"
	"runtime"
)

// Where a bundled tool is looked for, before anything on the search path.
//
// A shipped app carries its own ffmpeg, built without libx264 so that the
// build is LGPL. See docs/PACKAGING.md. The whole point is undone if the
// app finds Homebrew's ffmpeg on the search path instead: that one is a
// different build, with different encoders, and the short a customer gets
// would not be the short that was tested.
//
// So the tool beside the program wins, always, and the search path is what
// is left when there is no tool beside the program. That is also what keeps
// the development build honest, because the moment ffmpeg is in the bundle
// it is the one that runs.

// exeDir is the folder the running program sits in. Empty when it cannot be
// worked out, which is not a failure: the search path answers instead.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	// The real file, not a link to it. A development build is often reached
	// through a link, and the tools sit beside the real one.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return filepath.Dir(exe)
}

// besideTheProgram returns the path of a tool sitting next to the running
// program, or empty if there is none.
func besideTheProgram(name string) string {
	return toolIn(runtime.GOOS, exeDir(), name)
}

// Split from the above so every branch can be read on any machine. The
// macOS one matters most and a cloud session is not a Mac.
//
// On macOS the program is in Contents/MacOS of the bundle and so is the
// tool, so one folder covers it. Contents/Resources is looked at too,
// because that is where the Wails templates put what is not a program, and
// accepting both costs nothing.
func toolIn(goos, dir, name string) string {
	if dir == "" {
		return ""
	}
	if goos == "windows" {
		name += ".exe"
	}
	places := []string{dir}
	if goos == "darwin" && filepath.Base(dir) == "MacOS" {
		places = append(places, filepath.Join(filepath.Dir(dir), "Resources"))
	}
	for _, place := range places {
		candidate := filepath.Join(place, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		// Readable and runnable. A file of the right name that cannot be
		// run is worse than none, because it would hide the one on the
		// search path that works.
		if goos == "windows" || info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

// ToolPath decides which copy of a tool to run: the one named in the
// environment, then the one beside the program, then the search path.
//
// The environment comes first so a test, or a person comparing two builds,
// can always say which one they mean.
func ToolPath(envVar, name string) string {
	if named := os.Getenv(envVar); named != "" {
		return named
	}
	if own := besideTheProgram(name); own != "" {
		return own
	}
	return name
}
