package main

import (
	"fmt"
	"net/http"
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

// write adds lines under this moment and one source.
func (l *appLog) write(source string, lines ...string) {
	at := time.Now()
	stamped := make([]stampedLine, len(lines))
	for i, line := range lines {
		stamped[i] = stampedLine{at, line}
	}
	l.writeAt(source, stamped)
}

type stampedLine struct {
	at   time.Time
	text string
}

// writeAt adds lines under one source, each at its own moment. A log that
// cannot be written is no reason for anything else to fail, so it says
// nothing.
func (l *appLog) writeAt(source string, lines []stampedLine) {
	if l == nil || len(lines) == 0 {
		return
	}
	path := l.path
	if path == "" {
		path = appLogPath()
	}
	var b strings.Builder
	for _, line := range lines {
		at := line.at.Format("2006-01-02 15:04:05.000")
		for _, part := range strings.Split(strings.TrimRight(line.text, "\n"), "\n") {
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

// What the window may write at once: a few hundred lines of a few hundred
// characters. The window is our own code, but a loop gone wrong in it
// should not fill the disk.
const (
	saidLines   = 400
	saidLineMax = 2000
)

// SaidLine is one line the window says, with the moment it said it, in ms
// since 1970, so the window's lines and the Go side's stand in the order
// they happened, though the window sends its own a moment later.
type SaidLine struct {
	At   float64 `json:"at"`
	Text string  `json:"text"`
}

// Said writes what the window reports into the app's log: its errors and
// warnings, and every step a video takes to its picture and its play, see
// lib/said.ts.
func (f *FrameFairy) Said(lines []SaidLine) {
	more := 0
	if len(lines) > saidLines {
		more = len(lines) - saidLines
		lines = lines[:saidLines]
	}
	now := time.Now()
	stamped := make([]stampedLine, 0, len(lines)+1)
	for _, line := range lines {
		text := line.Text
		if len(text) > saidLineMax {
			text = text[:saidLineMax] + "…"
		}
		at := now
		// A moment from the window that is far off is the window's clock,
		// not when it was said.
		if t := time.UnixMilli(int64(line.At)); line.At > 0 && t.Sub(now).Abs() < time.Minute {
			at = t
		}
		stamped = append(stamped, stampedLine{at, text})
	}
	if more > 0 {
		stamped = append(stamped, stampedLine{now, fmt.Sprintf("and %d lines more", more)})
	}
	theLog.writeAt("window", stamped)
}

// recorded is a response as it was written, for the log.
type recorded struct {
	http.ResponseWriter
	status int
	bytes  int64
	// The start of the answer, kept where it is a few words, an open's
	// id or why a call failed.
	keep bool
	body []byte
}

func (r *recorded) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorded) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	if r.keep && len(r.body) < 300 {
		r.body = append(r.body, b[:min(n, 300-len(r.body))]...)
	}
	return n, err
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (r *recorded) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// A pull of frames slower than this is said, and so is every one that
// ended its stream, failed or was the first of its stream.
const slowPull = 300 * time.Millisecond

// logged serves a request for the episode's file or its frames and says
// in the app's log how it went: which video, what was asked, the answer
// and how long it took. The pulls of a stream that plays as it should are
// many and say nothing, so only the first, a slow one and one that ends
// it are written. See the video preview's lines in lib/said.ts, which
// these stand among.
func logged(w http.ResponseWriter, r *http.Request, serve func(http.ResponseWriter, *http.Request)) {
	start := time.Now()
	rec := &recorded{ResponseWriter: w, keep: r.URL.Path != "/frames/read" && strings.HasPrefix(r.URL.Path, "/frames/")}
	serve(rec, r)
	took := time.Since(start)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	q := r.URL.Query()
	name := filepath.Base(q.Get("path"))
	id := q.Get("id")
	if len(id) > 6 {
		id = id[:6]
	}
	h := rec.Header()
	var what string
	switch r.URL.Path {
	case "/frames/read":
		first := h.Get("X-Frames-Times") != ""
		ended := h.Get("X-Frames-End") != ""
		if !first && !ended && rec.status == http.StatusOK && took < slowPull {
			return
		}
		what = fmt.Sprintf("read %s n %s skip %s", id, q.Get("n"), q.Get("skip"))
		if first {
			what += ", first frame, times " + h.Get("X-Frames-Times")
		}
		if l := h.Get("X-Frames-Light"); l != "" {
			what += ", light " + l
		}
		switch {
		case h.Get("X-Frames-Error") != "":
			what += ", failed: " + h.Get("X-Frames-Error")
		case h.Get("X-Frames-Closed") != "":
			what += ", the stream was closed"
		case ended:
			what += ", the video ended"
		}
	case "/frames/open":
		what = fmt.Sprintf("open %s from %s at %sx%s", name, q.Get("from"), q.Get("w"), q.Get("h"))
	case "/frames/sound":
		what = fmt.Sprintf("sound %s from %s at %s Hz, %s channels", name, q.Get("from"), q.Get("rate"), q.Get("ch"))
	case "/frames/close":
		// The frames' worker asks this of no stream to see that it
		// reaches the Go side at all, see pull.worker.ts.
		if id == "none" {
			return
		}
		what = "close " + id
	case "/frames/hold", "/frames/release":
		what = strings.TrimPrefix(r.URL.Path, "/frames/") + " " + name
	default:
		what = fmt.Sprintf("%s %s", strings.TrimPrefix(r.URL.Path, "/"), name)
		if rg := r.Header.Get("Range"); rg != "" {
			what += " " + rg
		}
	}
	theLog.write("files", fmt.Sprintf("%s: %d, %d bytes, %d ms%s", what, rec.status, rec.bytes, took.Milliseconds(), answered(rec)))
}

// answered is what a call of a few words said back: an open's id, or why
// it failed.
func answered(rec *recorded) string {
	said := strings.TrimSpace(string(rec.body))
	if said == "" {
		return ""
	}
	return ", " + said
}
