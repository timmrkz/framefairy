package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"framefairy/engine"
)

// The app's own log: what went wrong where nobody sees it, the window's
// errors and what the episode's decoder says, so a fault on a Mac the
// cloud cannot run is read from a file and not guessed at. It is the log
// of plan row R.8, in the place Mac apps keep theirs, which the Console
// app shows: ~/Library/Logs/Frame Fairy/app.log. Elsewhere it is in the
// user's cache folder. Plan row 2.185.
type appLog struct {
	mu sync.Mutex
	// Where it is, or empty for the place above, looked up at each write,
	// since the tests set HOME after the program has started.
	path string
}

// The log is kept to a few megabytes: past that it starts again, with the
// one before kept beside it as app.1.log.
const appLogMax = 4 << 20

var theLog = &appLog{}

func appLogPath() string {
	home, err := os.UserHomeDir()
	if runtime.GOOS == "darwin" && err == nil {
		return filepath.Join(home, "Library", "Logs", "Frame Fairy", "app.log")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "FrameFairy", "logs", "app.log")
}

// write adds lines under one moment and one source. A log that cannot be
// written is no reason for anything else to fail, so it says nothing.
func (l *appLog) write(source string, lines ...string) {
	if l == nil || len(lines) == 0 {
		return
	}
	path := l.path
	if path == "" {
		path = appLogPath()
	}
	var b strings.Builder
	at := time.Now().Format("2006-01-02 15:04:05.000")
	for _, line := range lines {
		for _, part := range strings.Split(strings.TrimRight(line, "\n"), "\n") {
			fmt.Fprintf(&b, "%s %s %s\n", at, source, part)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && info.Size() > appLogMax {
		_ = os.Rename(path, strings.TrimSuffix(path, ".log")+".1.log")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(b.String())
}

// started notes a start of the app, with its build, so what follows is
// known to be from that build.
func (l *appLog) started() {
	l.write("app", fmt.Sprintf("started, %s on %s %s", runningVersion(), runtime.GOOS, runtime.GOARCH))
	engine.DecoderSaid = func(video, line string) {
		l.write("decoder", filepath.Base(video)+": "+line)
	}
}

// What the window may write at once: a few dozen lines of a few hundred
// characters. The window is our own code, but a loop gone wrong in it
// should not fill the disk.
const (
	saidLines   = 50
	saidLineMax = 2000
)

// Said writes what the window reports into the app's log: its errors and
// warnings, and what the video preview had when it showed no picture.
func (f *FrameFairy) Said(lines []string) {
	if len(lines) > saidLines {
		lines = append(lines[:saidLines], fmt.Sprintf("and %d lines more", len(lines)-saidLines))
	}
	for i, line := range lines {
		if len(line) > saidLineMax {
			lines[i] = line[:saidLineMax] + "…"
		}
	}
	theLog.write("window", lines...)
}
