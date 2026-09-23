// Package engine turns a long-form podcast master into vertical short-form
// clips, organised so that a command-line front end and a graphical one can
// both drive it.
//
// The stages, in the order a run goes through them:
//
//  1. audio      transcribe on the machine, time every word, measure loudness
//  2. lines      group words into the numbered lines the model reads
//  3. selection  the model chooses the moments and condenses them
//  4. analysis   camera switches and framing, derived from the footage
//  5. captions   captions made of the same words, written as srt and ass
//  6. render     build the ffmpeg filter graph and run it
package engine

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Version of the engine.
const Version = "0.3.0"

// Log writes timestamped output. Every line carries wall clock and elapsed
// time, so a stalled step is obvious and a slow one can be measured after the
// fact.
type Log struct {
	mu           sync.Mutex
	out          io.Writer
	tty          bool
	colour       bool
	verbose      bool
	t0           time.Time
	progressOpen bool

	sinkMu  sync.Mutex
	sink    Sink
	onError func(string)
	busy    bool
	stageMu sync.Mutex
	stages  []string
	// held is set while one piece of work owns the progress line, a search
	// above all: what its steps report along the way, ffmpeg finding camera
	// switches for one clip, would otherwise take the line from it every
	// half second, and a person would read neither.
	held atomic.Bool
}

// NewLog makes a logger writing to w. Colour is only used when w is a
// terminal and the environment does not ask for plain output.
func NewLog(w io.Writer, colour, verbose bool) *Log {
	tty := isTerminal(w)
	return &Log{
		out: w,
		tty: tty,
		colour: colour && tty &&
			os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb",
		verbose: verbose,
		t0:      time.Now(),
	}
}

// Verbose reports whether detail lines are shown.
func (l *Log) Verbose() bool { return l.verbose }

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func (l *Log) c(code, text string) string {
	if !l.colour {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func (l *Log) stamp() string {
	return l.c("2", fmt.Sprintf("%s %6.1fs", time.Now().Format("15:04:05"),
		time.Since(l.t0).Seconds()))
}

func (l *Log) emit(kind EventKind, marker, text, colour string, tinted bool) {
	l.mu.Lock()
	shown := text
	if tinted {
		shown = l.c(colour, text)
	}
	l.clearProgressLocked()
	fmt.Fprintf(l.out, "%s  %s %s\n", l.stamp(), l.c(colour, marker), shown)
	l.mu.Unlock()
	l.send(Event{Kind: kind, Text: text, Fraction: Unknown, Remaining: Unknown})
}

// Info is an ordinary line.
func (l *Log) Info(format string, a ...any) {
	l.emit(EventInfo, "·", sprintf(format, a...), "0", false)
}

// OK marks something that finished well.
func (l *Log) OK(format string, a ...any) {
	l.emit(EventOK, "+", sprintf(format, a...), "32", false)
}

// Warn is something the operator should read.
func (l *Log) Warn(format string, a ...any) {
	l.emit(EventWarn, "!", sprintf(format, a...), "33", true)
}

// Error is a failure.
func (l *Log) Error(format string, a ...any) {
	text := sprintf(format, a...)
	l.emit(EventError, "x", text, "31", true)
	l.sinkMu.Lock()
	hook := l.onError
	l.sinkMu.Unlock()
	if hook != nil {
		hook(text)
	}
}

// SetErrorHook calls fn with the text of every error line. Nil removes it.
func (l *Log) SetErrorHook(fn func(string)) {
	l.sinkMu.Lock()
	defer l.sinkMu.Unlock()
	l.onError = fn
}

// Detail is only shown with --verbose. Use it for anything you would want in
// a bug report but not in the normal run. A sink gets it either way.
func (l *Log) Detail(format string, a ...any) {
	text := sprintf(format, a...)
	if !l.verbose {
		l.send(Event{Kind: EventDetail, Text: text, Fraction: Unknown, Remaining: Unknown})
		return
	}
	l.emit(EventDetail, " ", text, "2", true)
}

// Progress writes a single line that rewrites itself, so a long step visibly
// ticks. Use it where the share done is not known.
func (l *Log) Progress(text string) {
	if l.held.Load() {
		return
	}
	l.showProgress(text)
	l.send(Event{Kind: EventProgress, Text: text, Fraction: Unknown, Remaining: Unknown})
}

// ProgressOf reports how far a task is, as a share from 0 to 1 and the
// seconds left. Either may be Unknown. The terminal shows a bar.
func (l *Log) ProgressOf(label string, fraction, remaining float64) {
	l.ProgressTo(label, fraction, remaining, 0)
}

// ProgressTo reports the same and adds the second of the episode the work
// has reached, which is 0 where that means nothing. A transcription saves
// what it has every few seconds, so what is on disk lags a long way behind
// what the machine has already heard, and the app, if it followed the file
// alone, would step rather than move.
func (l *Log) ProgressTo(label string, fraction, remaining, covered float64) {
	l.progress(label, fraction, remaining, covered, 0)
}

// ProgressFound reports how far a search is, with how many clips it has
// written to its plan so far.
func (l *Log) ProgressFound(label string, fraction, remaining float64, found int) {
	l.report(label, fraction, remaining, 0, found)
}

// HoldProgress gives the progress line to whoever reports through
// ProgressFound until it is let go, and nothing else is shown on it
// meanwhile.
func (l *Log) HoldProgress(hold bool) {
	l.held.Store(hold)
}

func (l *Log) progress(label string, fraction, remaining, covered float64, found int) {
	if l.held.Load() {
		return
	}
	l.report(label, fraction, remaining, covered, found)
}

func (l *Log) report(label string, fraction, remaining, covered float64, found int) {
	// math.Min and math.Max hand a NaN straight back, so a share that is
	// not a number has to be caught before it is held to 0 and 1.
	fraction = sane(fraction)
	if fraction != Unknown {
		fraction = math.Max(0, math.Min(1, fraction))
	}
	remaining = sane(remaining)
	text := label
	if fraction != Unknown {
		bar := strings.Repeat("#", int(fraction*24))
		text = fmt.Sprintf("%s [%-24s] %3.0f%%", label, bar, fraction*100)
		if remaining != Unknown {
			text += fmt.Sprintf("  %4.0fs left", remaining)
		}
	}
	l.showProgress(text)
	l.send(Event{Kind: EventProgress, Text: label, Fraction: roundTo(fraction, 4),
		Remaining: roundTo(remaining, 1), Covered: roundTo(covered, 3), Found: found})
}

func (l *Log) showProgress(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.tty {
		return
	}
	fmt.Fprintf(l.out, "\r\033[K%s    %s", l.stamp(), text)
	l.progressOpen = true
}

// ClearProgress removes a progress line if one is showing. A sink is told
// that nothing is in progress any more.
func (l *Log) ClearProgress() {
	l.mu.Lock()
	l.clearProgressLocked()
	l.mu.Unlock()
	l.send(Event{Kind: EventIdle, Fraction: Unknown, Remaining: Unknown})
}

func (l *Log) clearProgressLocked() {
	if l.progressOpen && l.tty {
		fmt.Fprint(l.out, "\r\033[K")
	}
	l.progressOpen = false
}

// Step announces a named stage, runs it, and reports how long it took or that
// it failed. Events sent while it runs carry its name as their stage.
func (l *Log) Step(name string, fn func() error) error {
	l.writeLine(">", l.c("36;1", name), "36;1")
	l.send(Event{Kind: EventStepStart, Stage: name, Text: name,
		Fraction: Unknown, Remaining: Unknown})
	l.pushStage(name)
	start := time.Now()
	err := fn()
	l.popStage()
	took := roundTo(time.Since(start).Seconds(), 3)
	if err != nil {
		l.writeLine("x", l.c("31", fmt.Sprintf("%s failed after %.1fs", name, took)), "31")
		l.send(Event{Kind: EventStepFailed, Stage: name, Text: err.Error(),
			Duration: took, Fraction: Unknown, Remaining: Unknown})
		return err
	}
	l.writeLine("+", name+" "+l.c("2", fmt.Sprintf("(%.1fs)", took)), "32")
	l.send(Event{Kind: EventStepDone, Stage: name, Text: name,
		Duration: took, Fraction: 1, Remaining: 0})
	return nil
}

// writeLine writes to the terminal only, for lines whose event is sent
// separately.
func (l *Log) writeLine(marker, shown, colour string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.clearProgressLocked()
	fmt.Fprintf(l.out, "%s  %s %s\n", l.stamp(), l.c(colour, marker), shown)
}

// sprintf only formats when there are arguments, so a message that happens to
// contain a percent sign is printed as written.
func sprintf(format string, a ...any) string {
	if len(a) == 0 {
		return format
	}
	return fmt.Sprintf(format, a...)
}

// commas formats an integer with thousands separators.
func commas(n int) string {
	negative := n < 0
	if negative {
		n = -n
	}
	digits := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, ch := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	if negative {
		return "-" + b.String()
	}
	return b.String()
}
