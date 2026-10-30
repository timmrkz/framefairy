package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
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
	"github.com/wailsapp/wails/v3/pkg/application"
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
//	{"level":"warn","time":"2026-10-10T22:41:07.512+02:00","run":"9f3a01","from":"window","msg":"calls: Library took 208 ms"}
//
// Every line from debug up is always written, so the steps that led to a
// fault are there when it happens. What keeps the disk free is the cap
// below, not a switch. The trace lines, which come with every frame or
// every pull, are kept in memory instead, the last few thousand of them,
// and a report of a problem carries them: what a play did just before
// the person reported it is what a stutter is found by, and nobody has
// to have switched anything on beforehand. See recent.
type appLog struct {
	mu   sync.Mutex
	file *lumberjack.Logger
	// The logger lines are written through, nil while the log is not open,
	// which is so in the tests that do not open one.
	root atomic.Pointer[zerolog.Logger]
	// One start of the app, so a restart is seen and two runs never mix.
	run    string
	recent recentLines
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
	lg := zerolog.New(byLevel{file: l.file, recent: &l.recent}).Level(zerolog.TraceLevel)
	l.root.Store(&lg)
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

// LogStatus is the app's log as Settings, Advanced shows it: where the
// file in use is, how many files there are with the older ones, and how
// much they hold together. Plan row 2.245.
type LogStatus struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
	Lines int    `json:"lines"`
}

// status counts the log's files, their bytes on the disk and their lines,
// the older ones unpacked to count them.
func (l *appLog) status() LogStatus {
	files := l.files()
	if len(files) == 0 {
		return LogStatus{}
	}
	st := LogStatus{Path: files[len(files)-1]}
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		st.Files++
		st.Bytes += info.Size()
		if data, err := readLog(path); err == nil {
			st.Lines += bytes.Count(data, []byte("\n"))
		}
	}
	return st
}

// remove removes every file of the log, the older ones too, and the next
// line starts a new one. Only the log's own files go, the ones files
// gives, never anything else in its folder.
func (l *appLog) remove() (LogStatus, error) {
	was := l.status()
	files := l.files()
	l.mu.Lock()
	var failed error
	for _, path := range files {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && failed == nil {
			failed = err
		}
	}
	// Closed only once its file is gone, and lumberjack opens a new one
	// with the next line. Closed first, a line said in between opened the
	// file again, which was then removed under it, and every line after
	// went into a file that was no longer there.
	if l.file != nil {
		_ = l.file.Close()
	}
	l.mu.Unlock()
	return was, failed
}

// AppLog is the app's log for Settings, Advanced.
func (s *FrameFairy) AppLog() LogStatus { return theLog.status() }

// ShowAppLog shows the file of the log in use in Finder.
func (s *FrameFairy) ShowAppLog() error {
	path := theLog.status().Path
	if path == "" {
		return errors.New("the app keeps no log yet")
	}
	return showInFolder(path)
}

// RemoveAppLog removes every file of the app's log, after Settings asked
// first, and gives the log as it is then: the one line that says so.
func (s *FrameFairy) RemoveAppLog() (LogStatus, error) {
	was, err := theLog.remove()
	line := theLog.line(zerolog.InfoLevel, "app").Int("files", was.Files).Int64("bytes", was.Bytes).Int("lines", was.Lines)
	if err != nil {
		line.Err(err).Msg("the app's log could not be removed whole")
		return theLog.status(), fmt.Errorf("the log could not be removed whole: %w", err)
	}
	line.Msg("the app's log was removed")
	return theLog.status(), nil
}

// engineLog is the log of an engine the app makes outside a job, which
// writes into the app's log under from and the video, when there is one:
// a line at its level, a detail at debug, and no progress. The engines that draw the captions, read the frames, measure
// the loudness, probe a video, keep the model, check the setup and read a
// recording all wrote to nothing before, so what the engine warned about
// there never reached the log a report sends, issue #605. A caption file
// without its pill was kept for the rest of a run with nothing to say why.
func engineLog(from, path string) *engine.Log {
	log := engine.NewLog(io.Discard, false, false)
	video := ""
	if path != "" {
		video = filepath.Base(path)
	}
	log.SetSink(func(ev engine.Event) {
		var level zerolog.Level
		switch ev.Kind {
		case engine.EventWarn:
			level = zerolog.WarnLevel
		case engine.EventError:
			level = zerolog.ErrorLevel
		case engine.EventInfo, engine.EventOK:
			level = zerolog.InfoLevel
		case engine.EventDetail:
			level = zerolog.DebugLevel
		default:
			return
		}
		line := theLog.line(level, from)
		if video != "" {
			line = line.Str("video", video)
		}
		line.Msg(ev.Text)
	})
	return log
}

// wailsPanicked writes a panic Wails caught into the app's log, with its
// stack, see the PanicHandler in main.
func wailsPanicked(d *application.PanicDetails) {
	theLog.line(zerolog.ErrorLevel, "wails").Str("stack", d.FullStackTrace).Msgf("stopped unexpectedly: %v", d.Error)
}

// byLevel sends the trace lines to memory and every other line to the file.
// zerolog hands a writer that can tell levels apart each line with its
// level.
type byLevel struct {
	file   io.Writer
	recent *recentLines
}

func (s byLevel) Write(p []byte) (int, error) { return s.file.Write(p) }

func (s byLevel) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level == zerolog.TraceLevel {
		return s.recent.Write(p)
	}
	return s.file.Write(p)
}

// The trace lines kept: at about two a second while a video plays, the
// last hour or so, and a few MB of memory at most.
const recentMost = 8000

// recentLines are the last trace lines, the oldest given way to the
// newest.
type recentLines struct {
	mu    sync.Mutex
	lines [][]byte
	next  int
}

func (r *recentLines) Write(p []byte) (int, error) {
	line := append([]byte(nil), p...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) < recentMost {
		r.lines = append(r.lines, line)
	} else {
		r.lines[r.next] = line
		r.next = (r.next + 1) % recentMost
	}
	return len(p), nil
}

// all are the lines kept, oldest first.
func (r *recentLines) all() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []byte
	for i := range r.lines {
		out = append(out, r.lines[(r.next+i)%len(r.lines)]...)
	}
	return out
}

// forget lets the kept lines go, for the tests.
func (r *recentLines) forget() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines, r.next = nil, 0
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
	l.takeStandardLog()
	l.line(zerolog.InfoLevel, "app").Str("build", runningVersion()).Str("os", runtime.GOOS).Str("arch", runtime.GOARCH).Msg("started")
	engine.DecoderSaid = func(video string, level slog.Level, line string) {
		l.line(fromSlog(level), "decoder").Str("video", filepath.Base(video)).Msg(line)
	}
}

// takeStandardLog writes what is said through the standard library's log,
// by a library or by net/http, into the app's log as a warning, and where
// it went before, the standard error or the file the checks from outside
// read, see probe_outside.go. Only the standard error had it, which
// nobody reads on a Mac app opened from Finder, issue #605. Our own code
// logs through theLog, see the lint in .golangci.yml.
func (l *appLog) takeStandardLog() {
	// The app's log has a field for the moment.
	log.SetFlags(0)
	log.SetOutput(io.MultiWriter(log.Writer(), standardLines{l}))
}

// standardLines is the standard library's log, a line at a time.
type standardLines struct{ l *appLog }

func (s standardLines) Write(p []byte) (int, error) {
	s.l.line(zerolog.WarnLevel, "app").Msg(strings.TrimRight(string(p), "\n"))
	return len(p), nil
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
	f.report.sawVideo(a.Video)
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
	// id or why a call failed, never where it is bytes: a caption layer
	// kept 300 bytes of its picture, escaped, 30 lines a second while a
	// clip played, and filled the log in minutes.
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
	if r.keep && len(r.body) < 300 && r.Header().Get("Content-Type") != "application/octet-stream" {
		r.body = append(r.body, b[:min(n, 300-len(r.body))]...)
	}
	return n, err
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (r *recorded) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// repeatedFailures keeps the failures of the captions said lately, by
// video and reason, so one that goes on is written once a while and
// counted in between.
type repeatedFailures struct {
	mu   sync.Mutex
	last map[string]*repeat
}

type repeat struct {
	at    time.Time
	count int
}

var failures = &repeatedFailures{last: map[string]*repeat{}}

// failuresQuiet is how long the same failure is counted rather than
// written again.
const failuresQuiet = 10 * time.Second

// seen notes a failure of the video at path for reason, and gives -1
// while it is counted, or how many times it came since it was last
// written, once it is to be written again.
func (f *repeatedFailures) seen(path, reason string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := path + "\x00" + reason
	r := f.last[key]
	now := time.Now()
	if r != nil && now.Sub(r.at) < failuresQuiet {
		r.count++
		return -1
	}
	if len(f.last) > 256 {
		clear(f.last)
	}
	before := 0
	if r != nil {
		before = r.count
	}
	f.last[key] = &repeat{at: now}
	return before
}

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
	case "/frames/captions":
		// One for every frame drawn while a clip plays, so a layer drawn
		// is counted, not written, and said for a stretch of playing at
		// a time, see layerTally in captions.go. A failure is written
		// once with its reason, and the same failure again for the same
		// video is counted until it has not come for a while: the
		// interface asks once a frame, and a lasting failure wrote
		// twice the frame rate a second.
		if !failed {
			return
		}
		repeated := failures.seen(q.Get("path"), fmt.Sprintf("%d %s", rec.status, rec.body))
		if repeated < 0 {
			return
		}
		what = fmt.Sprintf("captions at %s", q.Get("at"))
		if repeated > 0 {
			what += fmt.Sprintf(", %d times more since it was last said", repeated)
		}
		if a := q.Get("ahead"); a != "" {
			what += " ahead " + a
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
