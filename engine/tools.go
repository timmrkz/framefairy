package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// The tools a program runs are its own, or none.
//
// A shipped app carries its own ffmpeg, built without libx264 so that the
// build is LGPL, and its own llama-server. See docs/PACKAGING.md. They sit
// beside the program, and that is the only place they are taken from: not
// the search path, where Homebrew's ffmpeg is a different build with
// different encoders and every program a person ever installed can put
// one of the same name, and not a path typed into the settings. A tool
// that is missing is said to be missing. It was once looked for on the
// search path instead, which made a broken install look like a working
// one with somebody else's ffmpeg in it.
//
// And a tool beside the program is run only once it has been checked to
// be the file the program was built with. make takes the SHA-256 of each
// tool it puts there and builds it into the program, see toolSums, and a
// tool that does not match, or that no sum was built in for, is not run.
//
// The environment can still name one, for the tests and for comparing two
// builds by hand. Whoever sets a program's environment already decides
// what it runs.

// toolSums is what make builds in: name=sha256 for each tool it put beside
// the programs, separated by commas. Empty in a program built any other way,
// which then runs no tool from beside itself.
var toolSums string

// toolSum is the SHA-256 built in for a tool, or empty.
func toolSum(name string) string {
	for _, pair := range strings.Split(toolSums, ",") {
		if tool, sum, ok := strings.Cut(strings.TrimSpace(pair), "="); ok && tool == name {
			return strings.ToLower(sum)
		}
	}
	return ""
}

// exeDir is the folder the running program sits in. Empty when it cannot be
// worked out, and then there is no tool beside it.
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

// toolIn is the tool called name in the program's folder dir, or empty. It
// takes the system as well, so every branch can be read on any machine. The
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
		// run is no tool.
		if goos == "windows" || info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

// FindTool decides which copy of a tool to run: the one named in the
// environment, or else the one beside the program once it is checked. When
// there is none that may run, it says why, in words for whoever reads it.
func FindTool(envVar, name string) (string, error) {
	return findToolIn(exeDir(), envVar, name)
}

// findToolIn is FindTool for a program in dir, split out so a test can say
// where the program is.
func findToolIn(dir, envVar, name string) (string, error) {
	if named := os.Getenv(envVar); named != "" {
		return named, nil
	}
	own := toolIn(runtime.GOOS, dir, name)
	if own == "" {
		return "", renderErr("%s is not beside the program in %s, and no other is run. "+
			"Building with make puts it there. An app missing it needs installing again.", name, dir)
	}
	if err := checkTool(own, name); err != nil {
		return "", err
	}
	return own, nil
}

// ToolPath is FindTool without the reason: empty when there is no tool
// that may run. The reason is FindTool's to give, and Preflight gives it.
func ToolPath(envVar, name string) string {
	path, _ := FindTool(envVar, name)
	return path
}

// checked is every tool already found to be what it should be, with the
// size and time of the file as it was then, so a file is read once and not
// before every run, and read again the moment it changes.
var checked sync.Map

type toolStamp struct {
	size int64
	mod  time.Time
}

// checkTool says whether the tool at path is the file its built-in sum
// says it is.
func checkTool(path, name string) error {
	want := toolSum(name)
	if want == "" {
		return renderErr("%s beside the program has no checksum built into this program, so it is "+
			"not run. Building with make builds one in.", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stamp := toolStamp{info.Size(), info.ModTime()}
	if was, ok := checked.Load(path); ok && was == stamp {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return err
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != want {
		return renderErr("%s beside the program is not the one this program was built with, so it is "+
			"not run. Its SHA-256 is %s and should be %s. Installing the app again puts the right one back.",
			name, got, want)
	}
	checked.Store(path, stamp)
	return nil
}

// UseTools points the engine at an ffmpeg and an ffprobe that were chosen,
// on the command line or in the app's settings. An empty one leaves what
// NewEngine found. With only ffmpeg chosen, ffprobe is looked for beside
// it, under the name the system gives it, since the two come together.
func (e *Engine) UseTools(ffmpeg, ffprobe string) {
	if ffmpeg != "" {
		e.FFmpeg = ffmpeg
	}
	if ffprobe != "" {
		e.FFprobe = ffprobe
		return
	}
	if ffmpeg == "" {
		return
	}
	name := "ffprobe"
	if strings.EqualFold(filepath.Ext(ffmpeg), ".exe") {
		name = "ffprobe.exe"
	}
	if guess := filepath.Join(filepath.Dir(ffmpeg), name); exists(guess) {
		e.FFprobe = guess
	}
}
