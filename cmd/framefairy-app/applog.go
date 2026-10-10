package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"

	"framefairy/engine"
)

// The app's own log: what the app did, the window's errors and every step
// a picture and a play take, and what the episode's decoder says, so a
// fault on a customer's Mac is found by reading what happened, not by
// asking them to try again. It is in the place Mac apps keep theirs, which
// the Console app shows: ~/Library/Logs/Frame Fairy/app.log. Elsewhere it
// is in the user's cache folder. Plan rows 2.185 and 2.188, and the design
// in docs/LOGGING.md.
//
// zerolog writes the lines, one JSON object each, and lumberjack keeps the
// file to its size. Nothing here does either job itself.
//
//	{"level":"warn","time":"2026-10-10T22:41:07.512+02:00","run":"9f3a01","from":"window","msg":"stood still for 812 ms"}
//
// Every line from debug up is always written, so the steps that led to a
// fault are there when it happens. What keeps the disk free is the cap
// below, not a switch. The trace lines, which come with every frame or
// every pull, are written only while Detailed Log in the Help menu is
// ticked, or with FRAMEFAIRY_LOG=trace for a build from the checkout.
type appLog struct {
	mu   sync.Mutex
	file *lumberjack.Logger
	// The logger lines are written through, nil while the log is not open,
	// which is so in the tests that do not open one.
	root atomic.Pointer[zerolog.Logger]
	// One start of the app, so a restart is seen and two runs never mix.
	run   string
	trace atomic.Bool
	// The person's last act, which the Go side's lines carry, see Acted.
	act atomic.Pointer[string]
}

// The log is at most 10 MB of lines in use and three older files
// compressed beside it, a few MB more, whatever happens.
const (
	appLogMB    = 10
	appLogOlder = 3
)

var theLog = &appLog{run: runID()}

func init() {
	// A moment to the millisecond, with the Mac's offset from UTC.
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05.000Z07:00"
	// msg, as log/slog and most tools that read logs call it.
	zerolog.MessageFieldName = "msg"
	// zerolog holds every logger to debug and up unless told otherwise.
	// Which lines are written is the logger's own level, see leveled.
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
}

func runID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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

// open starts writing the log at path. Lines said before it go nowhere.
func (l *appLog) open(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
	}
	l.file = &lumberjack.Logger{Filename: path, MaxSize: appLogMB, MaxBackups: appLogOlder, Compress: true}
	l.leveled()
}

// close stops writing the log, for the tests.
func (l *appLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.root.Store(nil)
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// leveled puts a logger of the level asked for in place. It is called
// with mu held.
func (l *appLog) leveled() {
	if l.file == nil {
		return
	}
	level := zerolog.DebugLevel
	if l.trace.Load() {
		level = zerolog.TraceLevel
	}
	lg := zerolog.New(l.file).Level(level)
	l.root.Store(&lg)
}

// files are the log's files, the older ones first, the one in use last,
// or none while it is not open.
func (l *appLog) files() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	path := l.file.Filename
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	older, _ := filepath.Glob(filepath.Join(filepath.Dir(path), base+"-*"))
	sort.Strings(older)
	return append(older, path)
}

// detailed says whether the trace lines are written as well.
func (l *appLog) detailed(on bool) {
	l.trace.Store(on || os.Getenv("FRAMEFAIRY_LOG") == "trace")
	l.mu.Lock()
	defer l.mu.Unlock()
	l.leveled()
}

// line starts a line of one part of the app at this moment, with the act
// it belongs to, or gives nil, which takes fields and writes nothing,
// where its level is not written.
func (l *appLog) line(level zerolog.Level, from string) *zerolog.Event {
	return withAct(l.lineAt(level, from, time.Now()), l.acting())
}

// jobLine starts a line about a job, with the act that asked for it, not
// the one the person is at by now.
func (l *appLog) jobLine(level zerolog.Level, job *Job) *zerolog.Event {
	ev := l.lineAt(level, "jobs", time.Now()).Str("job", job.ID).Str("kind", job.Kind)
	if job.Episode != "" {
		ev = ev.Str("video", filepath.Base(job.Episode))
	}
	return withAct(ev, job.act)
}

func withAct(ev *zerolog.Event, act string) *zerolog.Event {
	if act == "" {
		return ev
	}
	return ev.Str("act", act)
}

// acting is the person's last act, or empty before the first.
func (l *appLog) acting() string {
	if a := l.act.Load(); a != nil {
		return *a
	}
	return ""
}

// lineAt starts a line at a moment of its own, with no act.
func (l *appLog) lineAt(level zerolog.Level, from string, at time.Time) *zerolog.Event {
	lg := l.root.Load()
	if lg == nil {
		return nil
	}
	return lg.WithLevel(level).Time(zerolog.TimestampFieldName, at).Str("run", l.run).Str("from", from)
}

// slog is a logger for code that speaks log/slog, whose lines go into the
// same file as the part named, from the level given. Wails, which runs
// the app's window, logs that way. Lines said before the log is open go
// nowhere.
func (l *appLog) slog(from string, least zerolog.Level) *slog.Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return slog.New(slog.DiscardHandler)
	}
	lg := zerolog.New(l.file).Level(least).With().Str("run", l.run).Str("from", from).Logger()
	return slog.New(zerolog.NewSlogHandler(lg))
}

// started opens the log in its place and notes a start of the app, with
// its build, so what follows is known to be from that build.
func (l *appLog) started() {
	l.open(appLogPath())
	l.line(zerolog.InfoLevel, "app").Str("build", runningVersion()).Str("os", runtime.GOOS).Str("arch", runtime.GOARCH).
		Bool("detailed", l.trace.Load()).Msg("started")
	engine.DecoderSaid = func(video string, level slog.Level, line string) {
		l.line(fromSlog(level), "decoder").Str("video", filepath.Base(video)).Msg(line)
	}
}

// fromSlog is the level of a line the engine said in log/slog's terms.
func fromSlog(level slog.Level) zerolog.Level {
	switch {
	case level >= slog.LevelError:
		return zerolog.ErrorLevel
	case level >= slog.LevelWarn:
		return zerolog.WarnLevel
	case level >= slog.LevelInfo:
		return zerolog.InfoLevel
	case level >= slog.LevelDebug:
		return zerolog.DebugLevel
	}
	return zerolog.TraceLevel
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
// they happened, though the window sends its own a moment later. Its
// level is error, warn, info, debug or trace, debug when it says none,
// and Video is the file name of the video it is about, where there is one.
type SaidLine struct {
	At    float64 `json:"at"`
	Level string  `json:"level,omitempty"`
	Video string  `json:"video,omitempty"`
	Act   string  `json:"act,omitempty"`
	Text  string  `json:"text"`
}

// levelOf is the level the window named.
func levelOf(name string) zerolog.Level {
	switch name {
	case "error":
		return zerolog.ErrorLevel
	case "warn":
		return zerolog.WarnLevel
	case "info":
		return zerolog.InfoLevel
	case "trace":
		return zerolog.TraceLevel
	}
	return zerolog.DebugLevel
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
	for _, line := range lines {
		text := strings.TrimRight(line.Text, "\n")
		if len(text) > saidLineMax {
			text = text[:saidLineMax] + "…"
		}
		at := now
		// A moment from the window that is far off is the window's clock,
		// not when it was said.
		if t := time.UnixMilli(int64(line.At)); line.At > 0 && t.Sub(now).Abs() < time.Minute {
			at = t
		}
		ev := theLog.lineAt(levelOf(line.Level), "window", at)
		if line.Video != "" {
			ev = ev.Str("video", filepath.Base(line.Video))
		}
		withAct(ev, clipped(line.Act, actMax)).Msg(text)
	}
	if more > 0 {
		theLog.line(zerolog.WarnLevel, "window").Int("more", more).Msgf("and %d lines more", more)
	}
}

// An act's id and what it says are the window's own few words.
const (
	actMax     = 16
	actWhatMax = 300
)

func clipped(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Act is one thing the person did: a click, a key or a video picked, with
// the id every line it sets off carries, the moment, in ms since 1970, a
// few words on what it was and the video in front. See lib/said.ts.
type Act struct {
	ID    string  `json:"id"`
	At    float64 `json:"at"`
	What  string  `json:"what"`
	Video string  `json:"video,omitempty"`
}

// Acted notes what the person did, at once rather than with the window's
// next lines, so that what the Go side does for it carries its id: the
// streams and the decoder it starts, the job it asks for. The window says
// each act before the calls it makes for it.
func (f *FrameFairy) Acted(a Act) {
	id := clipped(a.ID, actMax)
	theLog.act.Store(&id)
	at := time.Now()
	if t := time.UnixMilli(int64(a.At)); a.At > 0 && t.Sub(at).Abs() < time.Minute {
		at = t
	}
	ev := theLog.lineAt(zerolog.InfoLevel, "window", at)
	if a.Video != "" {
		ev = ev.Str("video", filepath.Base(a.Video))
	}
	withAct(ev, id).Msg(clipped(a.What, actWhatMax))
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

// A pull of frames slower than this is a debug line, and so is every one
// that ended its stream or was the first of its stream. The others are
// trace lines, see appLog.
const slowPull = 300 * time.Millisecond

// logged serves a request for the episode's file or its frames and says
// in the app's log how it went: which video, what was asked, the answer
// and how long it took. See the video preview's lines in lib/said.ts,
// which these stand among.
func logged(w http.ResponseWriter, r *http.Request, serve func(http.ResponseWriter, *http.Request)) {
	start := time.Now()
	rec := &recorded{ResponseWriter: w, keep: r.URL.Path != "/frames/read" && strings.HasPrefix(r.URL.Path, "/frames/")}
	serve(rec, r)
	took := time.Since(start)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	q := r.URL.Query()
	h := rec.Header()
	failed := rec.status >= 400 || h.Get("X-Frames-Error") != ""
	level := zerolog.DebugLevel
	if failed {
		level = zerolog.WarnLevel
	}
	var what string
	switch r.URL.Path {
	case "/frames/read":
		first := h.Get("X-Frames-Times") != ""
		ended := h.Get("X-Frames-End") != ""
		if !first && !ended && !failed && took < slowPull {
			level = zerolog.TraceLevel
		}
		what = fmt.Sprintf("read n %s skip %s", q.Get("n"), q.Get("skip"))
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
		what = fmt.Sprintf("open from %s at %sx%s", q.Get("from"), q.Get("w"), q.Get("h"))
	case "/frames/sound":
		what = fmt.Sprintf("sound from %s at %s Hz, %s channels", q.Get("from"), q.Get("rate"), q.Get("ch"))
	case "/frames/close":
		// The frames' worker asks this of no stream to see that it
		// reaches the Go side at all, see pull.worker.ts.
		if q.Get("id") == "none" {
			return
		}
		what = "close"
	case "/frames/hold", "/frames/release":
		what = strings.TrimPrefix(r.URL.Path, "/frames/")
	default:
		what = strings.TrimPrefix(r.URL.Path, "/")
		if rg := r.Header.Get("Range"); rg != "" {
			what += " " + rg
		}
	}
	// A line that is not written is not put together.
	ev := theLog.line(level, "files")
	if ev == nil {
		return
	}
	if p := q.Get("path"); p != "" {
		ev = ev.Str("video", filepath.Base(p))
	}
	if id := q.Get("id"); id != "" {
		ev = ev.Str("stream", id[:min(6, len(id))])
	}
	if said := strings.TrimSpace(string(rec.body)); said != "" {
		ev = ev.Str("said", said)
	}
	ev.Int("status", rec.status).Int64("bytes", rec.bytes).Int64("ms", took.Milliseconds()).Msg(what)
}
