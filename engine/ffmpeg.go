package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/afero"

	"framefairy/internal/framewire"
)

// RenderError is a failure the operator can act on, as opposed to a bug.
// Said is one already in the words a person reads, with what to do, and a
// search says it as it is, see planFailed.
type RenderError struct {
	Msg  string
	Said bool
}

func (e *RenderError) Error() string { return e.Msg }

func renderErr(format string, a ...any) error {
	return &RenderError{Msg: sprintf(format, a...)}
}

// saidErr is a failure said in a person's words, from its first sentence,
// which is what the row of a search shows. Issue #311.
func saidErr(format string, a ...any) error {
	return &RenderError{Msg: sprintf(format, a...), Said: true}
}

// videoGone is a video whose file is no longer where it was added from:
// moved, renamed, or on a drive that is not there now.
func videoGone(path string) error {
	return saidErr("The video file is no longer at %s. Put it back there, then try again.", path)
}

// inWords says whether err is a failure already said in a person's words.
func inWords(err error) bool {
	var r *RenderError
	return errors.As(err, &r) && r.Said
}

// APICalls counts what a run asked of the API. A retry is a request you are
// billed for if it reached the model, so it should never be invisible.
type APICalls struct {
	Attempts int
	Requests int
	Spent    float64
}

// Engine holds everything a run shares: the binaries it calls, the log it
// writes to and what it has spent. One Engine per run keeps two runs in the
// same process, as a GUI will have, from stepping on each other.
type Engine struct {
	Log     *Log
	FFmpeg  string
	FFprobe string

	// Files is where the files of episodes are read and written, the real
	// disk in the programs, see files.go.
	Files afero.Fs
	// Records is where the training records and the timings of past
	// searches are kept, on Files in the default places unless the
	// program hands it its own, see records.go.
	Records *Records

	// SkipCaptions renders without burning in captions.
	SkipCaptions bool
	// Prefill starts the model's reply with an opening brace. Models with
	// thinking enabled reject it, and it switches itself off when they do.
	Prefill bool
	// NoFaces turns face detection off, leaving the in-focus fallback.
	NoFaces bool

	Calls APICalls

	// OpenRecognizer loads the speech model. The command-line front end sets
	// it, which keeps the native speech library out of this package.
	OpenRecognizer func(modelDir string) (Recognizer, error)

	// WantEncoder names the video encoder instead of picking one, for a
	// machine whose ffmpeg is unusual and for comparing two on one clip.
	WantEncoder string

	// softDecode is set once decoding through the system's own video
	// decoder has failed on this machine, and every decode after it is
	// done on the processor.
	softDecode atomic.Bool
	// decoders holds, per episode file, which decoder framing was found to
	// use, so it is looked up and reported once.
	decoders sync.Map

	// checkpointEvery is how often a transcription saves what it has, and
	// levelsReads how many reads of the sound apart the loudness measure
	// saves what it has, in place of levelsDefault of the clock, when not
	// the defaults. A test sets them on its own engine, so it can still
	// run beside the others, where a package variable made every test that
	// changed it wait.
	checkpointEvery time.Duration
	levelsReads     int
	// system is the system whose encoders a render reaches for, see
	// encoderOrderFor, this one unless a test says otherwise, so the order
	// of a Mac can be run on any machine.
	system string
	// stuck, when set, says a render has stopped making frames, and its
	// encoder is given up then as well as after encoderStall of the
	// clock. A test sets it, so it waits for no stall of the clock.
	stuck <-chan struct{}

	// ahead lets the transcript be read into the local model ahead of
	// the search this engine runs, until the search asks, see
	// readahead.go.
	ahead aheadGate

	mu sync.Mutex
	// hearingWhole is how much audio, in seconds, the job hearing in
	// turns set out to hear when it started, see Project.hear, so a
	// hearing that gave the lane back and has it again counts on from
	// where it got to. Zero is a hearing on its own. It is the engine's
	// and not the call's because one engine runs one job: the app makes
	// an engine for each, see runJob in cmd/framefairy-app/jobs.go, and the
	// command line runs one job at a time. A second hearing on the same
	// engine at once would take the first one's whole.
	hearingWhole     float64
	encoder          Encoder
	hdrEncoder       Encoder
	subtitleTemplate string
	faces            *faceDetector
	facesLoaded      bool
}

// NewEngine makes an engine with the tools it calls: the ones named in the
// environment, or else the ones beside the program, checked. See FindTool
// in tools.go. A tool that may not run is left empty, and Preflight says
// why.
func NewEngine(log *Log, files afero.Fs) *Engine {
	return &Engine{Log: log,
		Files:   files,
		Records: NewRecords(files, "", ""),
		FFmpeg:  ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg"),
		FFprobe: ToolPath("FRAMEFAIRY_FFPROBE", "ffprobe"),
		NoFaces: os.Getenv("FRAMEFAIRY_NO_FACES") != ""}
}

type result struct {
	Code   int
	Stdout string
	Stderr string
	// Left is set for a program that did not end when it was stopped,
	// see leaveAfter.
	Left bool
}

// run executes a command as an argument list, never through a shell. A
// missing binary is a normal outcome here, so it comes back as exit code 127
// rather than as an error.
func run(ctx context.Context, cwd string, name string, args ...string) result {
	if name == "" {
		return result{Code: 127, Stderr: "no tool that may run, see Preflight"}
	}
	cmd := command(ctx, name, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	out, said := &cutWriter{w: &stdout}, &cutWriter{w: &stderr}
	cmd.Stdout, cmd.Stderr = out, said
	if err := cmd.Start(); err != nil {
		return result{Code: 127, Stderr: err.Error()}
	}
	left, err := finish(ctx, cmd, out, said)
	if left {
		return result{Code: 1, Stderr: filepath.Base(name) + " " + leftBehind, Left: true}
	}
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
			if code < 0 {
				code = 1
			}
		} else {
			return result{Code: 127, Stderr: err.Error()}
		}
	}
	return result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func runBytes(ctx context.Context, cwd string, name string, args ...string) ([]byte, int) {
	cmd := command(ctx, name, args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	out := &cutWriter{w: &stdout}
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		return nil, 127
	}
	left, err := finish(ctx, cmd, out)
	if left {
		return nil, 1
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.Bytes(), max(exit.ExitCode(), 1)
		}
		return nil, 127
	}
	return stdout.Bytes(), 0
}

// leaveAfter is how long a program that was stopped has to end before it
// is left behind. A killed program ends at once, unless it is inside the
// system and the system will not let it go: ffmpeg in VideoToolbox on CI's
// virtual Mac went on for eight minutes after it was killed, and the
// search that waited for it waited for good.
var leaveAfter = 5 * time.Second

// leaving is the wait for a program that was asked to stop to end,
// leaveAfter for ffmpeg and stopGrace for llama-server, time.After unless
// a test hands in a wait of its own.
var leaving = time.After

// leftBehind is what is said of a program that was left behind.
const leftBehind = "did not end when it was stopped, so it was left behind"

// stopProcess stops a program whose work is called off, by killing it. A
// variable so a test can stand in a stop that does not end the program.
var stopProcess = func(p *os.Process) error { return p.Kill() }

// command is exec.CommandContext that stops the program with stopProcess
// once ctx is done. run, runBytes and runFFmpegWatched start their
// programs with it and wait for them with finish.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	// Read here rather than in the stop, which may run after a test has
	// put the variable back.
	stop := stopProcess
	cmd.Cancel = func() error { return stop(cmd.Process) }
	return cmd
}

// finish waits for a command that was started to end. Once ctx is done
// the command is stopped, and one that has still not ended leaveAfter
// later is left behind and left says so: what it writes from then on goes
// to none of outs, which the caller may read at once.
func finish(ctx context.Context, cmd *exec.Cmd, outs ...*cutWriter) (left bool, err error) {
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err := <-finished:
		return false, err
	case <-ctx.Done():
	}
	select {
	case err := <-finished:
		return false, err
	case <-leaving(leaveAfter):
	}
	for _, out := range outs {
		out.cut()
	}
	return true, nil
}

// cutWriter passes what is written on to w until it is cut, and drops it
// after, so a program left behind writes into nothing anyone reads.
type cutWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (c *cutWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.w == nil {
		return len(p), nil
	}
	return c.w.Write(p)
}

func (c *cutWriter) cut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.w = nil
}

// parseProgress reads seconds of media processed so far from an ffmpeg
// -progress file.
func parseProgress(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	seconds := 0.0
	for _, line := range strings.Split(string(data), "\n") {
		// Only out_time_us. ffmpeg's out_time_ms field is a long-standing
		// misnomer that also holds microseconds, so trusting the name gives a
		// value a thousand times too large.
		if raw, ok := strings.CutPrefix(line, "out_time_us="); ok {
			raw = strings.TrimSpace(raw)
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil && raw != "" &&
				!strings.HasPrefix(raw, "-") {
				seconds = float64(n) / 1_000_000
			}
		}
	}
	return seconds
}

// RunFFmpeg runs ffmpeg and shows it moving.
//
// stderr goes to a file rather than a pipe, because silencedetect on a long
// episode produces more output than a pipe buffer holds.
func (e *Engine) RunFFmpeg(ctx context.Context, args []string, label string,
	total float64, cwd string) (int, string) {
	return e.runFFmpegTo(ctx, args, label, total, cwd, nil)
}

// runFFmpegTo is RunFFmpeg with what ffmpeg writes to its standard output
// kept in stdout, for a run that pipes frames out while it reports.
func (e *Engine) runFFmpegTo(ctx context.Context, args []string, label string,
	total float64, cwd string, stdout io.Writer) (int, string) {
	code, said, _ := e.runFFmpegWatched(ctx, args, label, total, cwd, stdout, 0)
	return code, said
}

// runFFmpegWatched is runFFmpegTo that stops ffmpeg once it has gone stall
// without moving on, and says so, for a run that has somewhere else to
// go. Zero waits as long as it takes.
func (e *Engine) runFFmpegWatched(ctx context.Context, args []string, label string,
	total float64, cwd string, stdout io.Writer, stall time.Duration) (code int, said string, stalled bool) {
	var kept []string
	for _, a := range args {
		if a != "-stats" {
			kept = append(kept, a)
		}
	}
	quoted := make([]string, len(kept))
	for i, a := range kept {
		quoted[i] = shellQuote(a)
	}
	e.Log.Detail("%s", "ffmpeg "+strings.Join(quoted, " "))

	tmp, err := os.MkdirTemp("", "framefairy-")
	if err != nil {
		return 1, err.Error(), false
	}
	defer os.RemoveAll(tmp)
	progressPath := filepath.Join(tmp, "progress")
	stderrPath := filepath.Join(tmp, "stderr")
	full := append([]string{"-progress", progressPath, "-nostats"}, kept...)

	handle, err := os.Create(stderrPath)
	if err != nil {
		return 1, err.Error(), false
	}
	// Its own context, so a stall ends ffmpeg without ending the work it
	// is part of.
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	cmd := command(ctx, e.FFmpeg, full...)
	cmd.Dir = cwd
	cmd.Stderr = handle
	var out *cutWriter
	if stdout != nil {
		out = &cutWriter{w: stdout}
		cmd.Stdout = out
	}
	started := time.Now()
	if err := cmd.Start(); err != nil {
		handle.Close()
		return 127, err.Error(), false
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()

	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	lastMove, lastDone, warned := time.Now(), -1.0, false
	var waitErr error
	// gone runs from the moment ffmpeg is stopped, see leaveAfter, and
	// stopping is heard at once rather than on the next tick.
	var gone <-chan time.Time
	stopping := ctx.Done()
	var stuck <-chan struct{}
	if stall > 0 {
		stuck = e.stuck
	}
	left := false
loop:
	for {
		if gone == nil && ctx.Err() != nil {
			gone, stopping = leaving(leaveAfter), nil
		}
		said := false
		select {
		case <-stopping:
			continue
		case <-stuck:
			said = true
		case waitErr = <-finished:
			break loop
		case <-gone:
			left = true
			if out != nil {
				out.cut()
			}
			name := label
			if name == "" {
				name = "ffmpeg"
			}
			e.Log.Warn("%s %s", name, leftBehind)
			break loop
		case <-ticker.C:
		}
		done := parseProgress(progressPath)
		elapsed := time.Since(started).Seconds()
		// A render said to be stuck has not moved, whatever its progress
		// says last, and is stopped the way a stall of the clock is.
		if done > lastDone && !said {
			lastDone, lastMove, warned = done, time.Now(), false
		} else if stall > 0 && !stalled && (said || time.Since(lastMove) > stall) {
			e.Log.Warn("%s has not advanced for %s, so it is stopped", label, stall)
			stalled = true
			stop()
		} else if !warned && time.Since(lastMove) > 2*time.Minute {
			name := label
			if name == "" {
				name = "ffmpeg"
			}
			e.Log.Warn("%s has not advanced for two minutes. It may be working "+
				"on a difficult section, or it may be stuck. Ctrl-C is safe.", name)
			warned = true
		}
		if total > 0 && done > 0 {
			share := min(done/total, 1.0)
			speed := 0.0
			if elapsed > 0 {
				speed = done / elapsed
			}
			remaining := 0.0
			if speed > 0.01 {
				remaining = (total - done) / speed
			}
			e.Log.ProgressOf(label, share, remaining)
		} else {
			e.Log.Progress(fmt.Sprintf("%s %.0fs", label, elapsed))
		}
	}
	handle.Close()
	e.Log.ClearProgress()
	stderr, _ := os.ReadFile(stderrPath)
	if left {
		code = 1
	} else if waitErr != nil {
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			code = max(exit.ExitCode(), 1)
			// A program ended by a signal, a crash above all, says nothing
			// of it itself. On CI's Mac a render ended with one line of
			// macOS's and no word from ffmpeg, and nobody could tell
			// whether it had crashed, issue #570.
			if exit.ExitCode() < 0 {
				stderr = append(stderr, "\nffmpeg was ended by a "+exit.Error()+"\n"...)
			}
		} else {
			code = 1
		}
	}
	return code, strings.ToValidUTF8(string(stderr), "\uFFFD"), stalled
}

// --------------------------------------------------------------------------
// subtitle support
// --------------------------------------------------------------------------

// The ass filter's option syntax is not stable across ffmpeg builds. Some
// accept the shorthand "ass=file.ass", others demand "ass=filename=file.ass"
// and answer the shorthand with "No option name near". Rather than guess a
// version, the tool renders one frame with each candidate and keeps what
// works.
var subtitleCandidates = []string{
	"ass=filename={name}",
	"ass={name}",
	"subtitles=filename={name}",
	"subtitles={name}",
}

// fontsOption points libass at the folder the bundled face is written to,
// beside the caption file. It is a relative name, so no path reaches the
// filter graph. Old builds that do not know the option fall back to the
// plain form, and then the machine's own fonts are all there is.
const fontsOption = ":fontsdir=" + FontsFolder

func subtitleChain(template, name string) string {
	return strings.ReplaceAll(template, "{name}", name)
}

const noLibassHelp = "this ffmpeg was built without libass, so it cannot burn in " +
	"subtitles at all. Homebrew's default formula ships a lite build " +
	"with no --enable-libass.\n" +
	"  Install one that has it:\n" +
	"    brew tap homebrew-ffmpeg/ffmpeg\n" +
	"    brew unlink ffmpeg\n" +
	"    brew install homebrew-ffmpeg/ffmpeg/ffmpeg\n" +
	"  Check it worked:\n" +
	"    ffmpeg -filters | grep ass\n" +
	"  Or keep both and point this tool at the other one:\n" +
	"    framefairy ... --ffmpeg /opt/homebrew/opt/ffmpeg-full/bin/ffmpeg\n" +
	"  Or render without burned-in captions for now:\n" +
	"    framefairy ... --no-captions"

// SubtitleFilter works out how this ffmpeg wants a subtitle file named,
// once for as long as the ffmpeg stays as it is, see probed.go.
func (e *Engine) SubtitleFilter(ctx context.Context) (string, error) {
	e.mu.Lock()
	template := e.subtitleTemplate
	e.mu.Unlock()
	if template != "" {
		return template, nil
	}
	key, keep := keyFor("subtitle filter", e.FFmpeg, "")
	if keep {
		if template, ok := recall[string](key); ok {
			e.mu.Lock()
			e.subtitleTemplate = template
			e.mu.Unlock()
			return template, nil
		}
	}

	filters := run(ctx, "", e.FFmpeg, "-hide_banner", "-filters")
	hasASS := false
	for _, line := range strings.Split(filters.Stdout, "\n") {
		parts := strings.Fields(line)
		if len(parts) > 1 && (parts[1] == "ass" || parts[1] == "subtitles") {
			hasASS = true
			break
		}
	}
	if filters.Code == 0 && !hasASS {
		return "", renderErr(noLibassHelp)
	}

	tmp, err := os.MkdirTemp("", "framefairy-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	// A name shaped like a real one, so anything the parser dislikes about
	// digits, underscores or hyphens shows up here rather than mid-render.
	// No highlight and radius 0 keep this out of the measuring path, which
	// would call back into this function and never return.
	name := "00_probe-name.ass"
	if err := e.WriteASS(ctx, []LaidCaption{{Caption: Caption{Start: 0, End: 1, Text: "probe",
		Words: []Cue{{0, 1, "probe"}}}, Lines: [][]Cue{{{0, 1, "probe"}}}}}, filepath.Join(tmp, name),
		320, 180, map[string]any{"radius": 0.0, "highlight": 0.0}); err != nil {
		return "", err
	}
	if err := InstallFont(tmp, DefaultFont); err != nil {
		return "", err
	}
	// The bundled font first, so that what is probed is what is run.
	candidates := make([]string, 0, 2*len(subtitleCandidates))
	for _, candidate := range subtitleCandidates {
		candidates = append(candidates, candidate+fontsOption)
	}
	candidates = append(candidates, subtitleCandidates...)
	var failures []string
	for _, candidate := range candidates {
		chain := subtitleChain(candidate, name)
		res := run(ctx, tmp, e.FFmpeg, "-hide_banner", "-loglevel", "error",
			"-nostats", "-y", "-f", "lavfi",
			"-i", "color=c=black:s=320x180:d=0.1:r=25",
			"-filter_complex", "[0:v]"+chain+"[v]",
			"-map", "[v]", "-frames:v", "1", "-f", "null", "-")
		if res.Code == 0 {
			e.mu.Lock()
			e.subtitleTemplate = candidate
			e.mu.Unlock()
			if keep {
				remember(key, candidate)
			}
			e.Log.Detail("subtitle filter syntax: %s", chain)
			return candidate, nil
		}
		failures = append(failures, fmt.Sprintf("  %s\n    %s", chain,
			runePrefix(strip(res.Stderr), 200)))
	}
	return "", renderErr("%s", "this ffmpeg cannot burn in subtitles with any syntax I know.\n"+
		strings.Join(failures, "\n")+
		"\n  Check that ffmpeg was built with libass: ffmpeg -filters | grep ass")
}

// Preflight checks the tools before anything expensive happens.
//
// This runs before the API call on purpose. Discovering a broken ffmpeg after
// paying to plan an episode is the wrong order to find out.
func (e *Engine) Preflight(ctx context.Context) error {
	e.Log.Progress("checking ffmpeg")
	for _, tool := range []struct{ path, env, name string }{
		{e.FFmpeg, "FRAMEFAIRY_FFMPEG", "ffmpeg"},
		{e.FFprobe, "FRAMEFAIRY_FFPROBE", "ffprobe"},
	} {
		if tool.path == "" {
			e.Log.ClearProgress()
			_, err := FindTool(tool.env, tool.name)
			if err == nil {
				err = renderErr("there is no %s to run", tool.name)
			}
			return err
		}
	}
	// A tool that ran once runs again until it is replaced, so each is
	// asked once for the process, the way the subtitle syntax is.
	ffmpegKey, keepFFmpeg := keyFor("ffmpeg version", e.FFmpeg, "")
	first, ran := recall[string](ffmpegKey)
	if !ran {
		version := run(ctx, "", e.FFmpeg, "-version")
		if version.Code != 0 {
			e.Log.ClearProgress()
			return renderErr("%s does not run: %s", e.FFmpeg, Scrub(version.Stderr, 200))
		}
		first = "ffmpeg ?"
		if lines := strings.SplitN(version.Stdout, "\n", 2); lines[0] != "" {
			first = lines[0]
		}
		if keepFFmpeg {
			remember(ffmpegKey, first)
		}
	}
	e.Log.Detail("%s", first)

	ffprobeKey, keepFFprobe := keyFor("ffprobe version", e.FFprobe, "")
	if _, ran := recall[bool](ffprobeKey); !ran {
		if run(ctx, "", e.FFprobe, "-version").Code != 0 {
			e.Log.ClearProgress()
			return renderErr("%s is missing, which usually means a partial ffmpeg install.",
				e.FFprobe)
		}
		if keepFFprobe {
			remember(ffprobeKey, true)
		}
	}
	// Which encoder, before anything is spent. It used to insist on
	// libx264, which is the one library that makes an ffmpeg build GPL and
	// is deliberately absent from the one we ship.
	if _, err := e.VideoEncoder(ctx); err != nil {
		e.Log.ClearProgress()
		return err
	}
	if !e.SkipCaptions {
		e.Log.Progress("checking subtitle support")
		if _, err := e.SubtitleFilter(ctx); err != nil {
			e.Log.ClearProgress()
			return err
		}
	}
	e.Log.ClearProgress()
	return nil
}

// --------------------------------------------------------------------------
// source inspection
// --------------------------------------------------------------------------

// SourceInfo describes the episode's video stream.
type SourceInfo struct {
	// The size of the picture standing up, the way ffmpeg shows it, after
	// the turn the file asks for, see Turned.
	Width  int
	Height int
	// Turned is how far the file asks for its picture to be turned to
	// stand up, in degrees counterclockwise, 0, 90, 180 or 270, the way
	// ffprobe reports a phone's display matrix. ffmpeg turns it by
	// itself. Width and Height are already the picture turned.
	Turned   int
	FPSNum   int
	FPSDen   int
	Duration float64
	// When the picture's first frame begins, in seconds after the start of
	// the file, which is where ffmpeg's -ss and every time in a clip count
	// from. Most files start both together. One whose picture starts after
	// its sound, by an edit list or a first timestamp, has its frames on a
	// grid that begins here.
	VideoStart float64
	// Whether the frames come at uneven times, so that frame k is not k
	// over the rate after the first: a phone that records a frame a little
	// early or late, or a screen recorder that leaves out frames where
	// nothing moved. A render then takes the frame that holds each moment
	// by the frames' own timestamps, see cutOf, and the rate is not the one
	// the file reports but the one its short is made at, see shortRate.
	Variable bool
	// Colour tags to copy to the output, as ffmpeg flag name and value.
	Colour [][2]string
	// The first sound of the file: its samples a second as ffmpeg decodes
	// it and its channels, both nought for a file with no sound.
	SoundRate     int
	SoundChannels int
	// Where the file starts on its own clock, which ffprobe's
	// -read_intervals counts in, and whether that is known.
	origin   float64
	hasClock bool
	// How long a tick of the picture's timestamps is, tickNum/tickDen
	// seconds, its time base, 1/1000 in Matroska. Nought where ffprobe
	// does not say.
	tickNum, tickDen int64
	// The pieces of a clip whose own frames come at uneven times in a
	// file Probe found even, see unevenPieces.
	unevenAt [][2]float64
	// Whether the file keeps no index of its key frames, so a seek lands
	// between two, see unindexed, and the key frame each piece of a clip
	// is read from in such a file, see keyedPieces.
	unindexed bool
	keyAt     []pieceKey
}

// fraction is a positive fraction ffprobe writes as "1/1000", in lowest
// terms, or nought for anything else.
func fraction(v any) (num, den int64) {
	text, _ := v.(string)
	a, b, _ := strings.Cut(text, "/")
	n, err1 := strconv.ParseInt(a, 10, 64)
	d, err2 := strconv.ParseInt(b, 10, 64)
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return 0, 0
	}
	g := int64(gcd(int(n), int(d)))
	return n / g, d / g
}

// FPS is the frame rate as a number.
func (s SourceInfo) FPS() float64 { return float64(s.FPSNum) / float64(s.FPSDen) }

// FPSString is the frame rate as ffmpeg wants it, in lowest terms.
func (s SourceInfo) FPSString() string { return fmt.Sprintf("%d/%d", s.FPSNum, s.FPSDen) }

// Light is how the episode's picture codes its brightness, from its
// transfer tag: standard video, or HDR in PQ or HLG.
func (s SourceInfo) Light() framewire.Light {
	for _, c := range s.Colour {
		if c[0] == "color_trc" {
			return framewire.LightOf(c[1])
		}
	}
	return framewire.SDR
}

// colourParams is the file's colour tags as setparams takes them, which
// puts them on every frame of a short, or nothing for a file without tags.
// An encoder of a newer ffmpeg, ours among them, takes its tags from its
// frames and drops -color_trc and the like. The names are ffprobe's, from
// ffmpeg's own lists, and anything else is left out.
func (s SourceInfo) colourParams() string {
	var set []string
	for _, c := range s.Colour {
		if c[1] == "" || strings.Trim(c[1], "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			continue
		}
		set = append(set, c[0]+"="+c[1])
	}
	if len(set) == 0 {
		return ""
	}
	return "setparams=" + strings.Join(set, ":")
}

func gcd(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// videoStartOf is where the picture's first frame begins, from what
// ffprobe says of the picture's stream and of the file. ffmpeg counts -ss
// from the start of the file, the earliest of its streams, so that is
// where the picture's start is measured from.
func videoStartOf(stream, format map[string]any) float64 {
	first, ok := toFloat(stream["start_time"])
	if !ok {
		return 0
	}
	if from, ok := toFloat(format["start_time"]); ok && first-from > 1e-6 {
		return first - from
	}
	return 0
}

// pictureStart is SourceInfo.VideoStart on its own, for a reading of the
// sound, which needs nothing else Probe works out and so asks ffprobe for
// nothing else. Nought for a file with no picture or one ffprobe cannot
// read, where a seek finds the sound as well as anything does.
func (e *Engine) pictureStart(ctx context.Context, path string) float64 {
	key, keep := keyFor("picture start", e.FFprobe, path)
	if keep {
		if start, ok := recall[float64](key); ok {
			return start
		}
	}
	start, read := e.readPictureStart(ctx, path)
	if read && keep && ctx.Err() == nil {
		remember(key, start)
	}
	return start
}

// readPictureStart is pictureStart asked of ffprobe, and whether it
// answered.
func (e *Engine) readPictureStart(ctx context.Context, path string) (float64, bool) {
	res := run(ctx, "", e.FFprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=start_time", "-show_entries", "format=start_time",
		"-of", "json", path)
	if res.Code != 0 {
		return 0, false
	}
	var data struct {
		Streams []map[string]any `json:"streams"`
		Format  map[string]any   `json:"format"`
	}
	decoder := json.NewDecoder(strings.NewReader(res.Stdout))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil {
		return 0, false
	}
	if len(data.Streams) == 0 {
		return 0, true
	}
	return videoStartOf(data.Streams[0], data.Format), true
}

// Probe reads the dimensions, frame rate, duration and colour tags of a
// file, once for as long as the file stays as it is, see probed.go.
func (e *Engine) Probe(ctx context.Context, path string) (SourceInfo, error) {
	key, keep := keyFor("probe", e.FFprobe, path)
	if keep {
		if info, ok := recall[SourceInfo](key); ok {
			return info.clone(), nil
		}
	}
	info, err := e.probe(ctx, path)
	// A probe stopped halfway may have read no sound or no uneven frames
	// without failing, which is not what the file holds.
	if err == nil && keep && ctx.Err() == nil {
		remember(key, info.clone())
	}
	return info, err
}

func (e *Engine) probe(ctx context.Context, path string) (SourceInfo, error) {
	res := run(ctx, "", e.FFprobe, "-v", "error",
		"-select_streams", "v:0",
		"-show_entries",
		"stream=width,height,r_frame_rate,avg_frame_rate,start_time,time_base,color_primaries,color_transfer,color_space",
		"-show_entries", "stream_side_data=rotation",
		"-show_entries", "format=duration,start_time,format_name",
		"-of", "json", path)
	if res.Code != 0 {
		if !exists(e.Files, path) {
			return SourceInfo{}, videoGone(path)
		}
		return SourceInfo{}, renderErr("ffprobe failed on %s:\n%s", path, strip(res.Stderr))
	}
	var data struct {
		Streams []map[string]any `json:"streams"`
		Format  map[string]any   `json:"format"`
	}
	decoder := json.NewDecoder(strings.NewReader(res.Stdout))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return SourceInfo{}, fmt.Errorf("ffprobe output is not JSON: %w", err)
	}
	if len(data.Streams) == 0 {
		return SourceInfo{}, renderErr("no video stream found in %s", path)
	}
	stream := data.Streams[0]

	// Some containers report 0/0 here, which is no frame rate at all.
	rate, _ := stream["r_frame_rate"].(string)
	numText, denText, _ := strings.Cut(rate, "/")
	if denText == "" {
		denText = "1"
	}
	num, err1 := strconv.Atoi(numText)
	den, err2 := strconv.Atoi(denText)
	if err1 != nil || err2 != nil || den == 0 || num == 0 || (num < 0) != (den < 0) {
		return SourceInfo{}, renderErr("%s reports no usable frame rate, so the "+
			"output frame rate cannot be matched to the source", path)
	}
	if den < 0 {
		num, den = -num, -den
	}
	g := gcd(num, den)
	num, den = num/g, den/g

	var colour [][2]string
	for _, pair := range [][2]string{
		{"color_primaries", "color_primaries"},
		{"color_transfer", "color_trc"},
		{"color_space", "colorspace"},
	} {
		value, _ := stream[pair[0]].(string)
		if value != "" && value != "unknown" && value != "reserved" {
			colour = append(colour, [2]string{pair[1], value})
		}
	}

	duration := 0.0
	if raw, ok := data.Format["duration"]; ok {
		// Some containers report N/A, and the range check just skips then.
		if d, ok := toFloat(raw); ok {
			duration = d
		}
	}
	formatStart, _ := toFloat(data.Format["start_time"])
	if duration <= 0 {
		// A Matroska file whose writing was cut off, the way OBS leaves a
		// recording when it crashes, has no duration and no index. It is
		// read through once for where its last packet ends.
		duration = e.lastMoment(ctx, path, formatStart)
	}
	format, _ := data.Format["format_name"].(string)

	width, okW := toInt(stream["width"])
	height, okH := toInt(stream["height"])
	if _, present := stream["width"]; !present || !okW {
		return SourceInfo{}, renderErr("%s reports no video dimensions", path)
	}
	if _, present := stream["height"]; !present || !okH {
		return SourceInfo{}, renderErr("%s reports no video dimensions", path)
	}
	// A phone stores the picture the way its sensor lies and says how to
	// stand it up, and ffmpeg stands it up before anything else sees it.
	// So every size the engine works with is the picture standing up.
	turned := fileRotation(stream["side_data_list"])
	if turned == 90 || turned == 270 {
		width, height = height, width
	}
	videoStart := videoStartOf(stream, data.Format)
	first, hasFirst := toFloat(stream["start_time"])
	info := SourceInfo{Width: width, Height: height, Turned: turned, FPSNum: num, FPSDen: den,
		Duration: duration, VideoStart: videoStart, Colour: colour,
		origin: first - videoStart, hasClock: hasFirst, unindexed: unindexed(format)}
	info.tickNum, info.tickDen = fraction(stream["time_base"])
	info.SoundRate, info.SoundChannels = e.soundOf(ctx, path)
	info.Variable = hasFirst && e.uneven(ctx, path, info)
	if info.Variable {
		// The rate ffmpeg reports for uneven frames is the finest step
		// they keep to, 50 or more for a phone's, so a short is made at
		// their average instead, rounded to a rate shorts are played at.
		average := float64(num) / float64(den)
		if text, _ := stream["avg_frame_rate"].(string); text != "" {
			a, b, _ := strings.Cut(text, "/")
			n, err1 := strconv.ParseFloat(a, 64)
			d, err2 := strconv.ParseFloat(b, 64)
			if err1 == nil && err2 == nil && n > 0 && d > 0 {
				average = n / d
			}
		}
		info.FPSNum, info.FPSDen = shortRate(average), 1
	}
	return info, nil
}

// soundOf is the rate and the channels of a file's first sound, or
// nought for a file with no sound or one ffprobe cannot read.
func (e *Engine) soundOf(ctx context.Context, path string) (rate, channels int) {
	res := run(ctx, "", e.FFprobe, "-v", "error", "-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,channels", "-of", "json", path)
	if res.Code != 0 {
		return 0, 0
	}
	var data struct {
		Streams []map[string]any `json:"streams"`
	}
	decoder := json.NewDecoder(strings.NewReader(res.Stdout))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil || len(data.Streams) == 0 {
		return 0, 0
	}
	rate, ok1 := toInt(data.Streams[0]["sample_rate"])
	channels, ok2 := toInt(data.Streams[0]["channels"])
	if !ok1 || !ok2 || rate <= 0 || channels <= 0 {
		return 0, 0
	}
	return rate, channels
}

// fileRotation is the turn a file's display matrix asks for, from
// ffprobe's side data, as 0, 90, 180 or 270 degrees counterclockwise. A
// turn that is not a quarter of a circle is no turn: ffmpeg leaves it too.
func fileRotation(sideData any) int {
	list, _ := sideData.([]any)
	for _, item := range list {
		entry, _ := item.(map[string]any)
		value, ok := toFloat(entry["rotation"])
		if !ok || !isFinite(value) {
			continue
		}
		return framewire.Quarter(value)
	}
	return 0
}

// shortRate is the rate a short of uneven frames is made at: the nearest
// of the rates shorts are played at to the frames' average, 30 for a phone
// that averages 29.75.
func shortRate(average float64) int {
	best := 0
	for _, rate := range []int{24, 25, 30, 50, 60} {
		if best == 0 || math.Abs(float64(rate)-average) < math.Abs(float64(best)-average) {
			best = rate
		}
	}
	return best
}

// frameHair is how far a frame may begin from its place on the grid of
// the rate and still be on it, see framewire.Hair, which the episode's
// decoder shows the video preview's frames by.
const frameHair = framewire.Hair

// uneven tells whether the frames of a file come off the grid of its rate,
// by their timestamps in four stretches of two seconds across it. Only the
// index is read there, not the picture, and reading the whole of it would
// read the whole file, gigabytes for an episode of a few hours, every time
// a file is probed. Frames that are uneven only elsewhere are found where
// they are cut, see unevenPieces.
func (e *Engine) uneven(ctx context.Context, path string, info SourceInfo) bool {
	for _, at := range []float64{0, 0.25, 0.5, 0.75} {
		if e.unevenIn(ctx, path, info, at*info.Duration, 2) {
			return true
		}
	}
	return false
}

// unevenIn tells whether the frames from a moment on, for span seconds,
// come off the grid of the rate: a frame that begins off the grid, or two
// frames one after the other that are not one frame apart, because a
// frame was left out or came twice. ffprobe reads from the key frame
// before the moment, and the end is given as a moment too: a length is
// counted from the key frame, so a read for a piece stopped short of it.
// A file it cannot read is taken as even, the way it was taken before.
func (e *Engine) unevenIn(ctx context.Context, path string, info SourceInfo, at, span float64) bool {
	frame := float64(info.FPSDen) / float64(info.FPSNum)
	first := info.origin + info.VideoStart
	res := run(ctx, "", e.FFprobe, "-v", "error", "-select_streams", "v:0",
		"-read_intervals", fixed(info.origin+at, 3)+"%"+fixed(info.origin+at+span, 3),
		"-show_entries", "packet=pts_time", "-of", "csv=p=0", path)
	if res.Code != 0 {
		return false
	}
	var begins []float64
	unstamped := 0
	for _, line := range strings.Fields(res.Stdout) {
		if t, err := strconv.ParseFloat(strings.Trim(line, ","), 64); err == nil {
			begins = append(begins, t)
		} else {
			unstamped++
		}
	}
	// In the order they are shown, which B-frames change.
	slices.Sort(begins)
	for i, t := range begins {
		k := (t - first) / frame
		if math.Abs(k-math.Round(k))*frame > frameHair {
			return true
		}
		if i == 0 {
			continue
		}
		// MPEG-PS stamps no time on some of its frames, and ffmpeg works
		// theirs out as it decodes them, one frame after the frame before.
		// So a gap of whole frames is filled by frames with no stamp, as
		// long as there are as many of them, #268.
		gap := math.Round((t - begins[i-1]) / frame)
		if gap < 1 || int(gap)-1 > unstamped {
			return true
		}
		unstamped -= int(gap) - 1
	}
	return false
}

// unevenPieces is the episode with the pieces of a clip marked whose own
// frames come at uneven times, in a file whose frames Probe found even
// where it looked: a screen recorder that left out frames only somewhere
// in the middle. Each piece reads the index of its own few seconds, from
// the frame before it to the frame after. A marked piece is cut by the
// frames' own times, see cutOf, and the short keeps the file's rate.
func (e *Engine) unevenPieces(ctx context.Context, path string, s SourceInfo, segs []Segment) SourceInfo {
	if s.Variable || !s.hasClock || s.FPSNum <= 0 || s.FPSDen <= 0 {
		return s
	}
	frame := float64(s.FPSDen) / float64(s.FPSNum)
	s.unevenAt = nil
	for _, seg := range segs {
		from := max(seg.Start-frame, 0)
		if e.unevenIn(ctx, path, s, from, seg.End+frame-from) {
			s.unevenAt = append(s.unevenAt, [2]float64{seg.Start, seg.End})
		}
	}
	return s
}

// unevenOver tells whether a piece was marked by unevenPieces.
func (s SourceInfo) unevenOver(seg Segment) bool {
	for _, at := range s.unevenAt {
		if at[0] == seg.Start && at[1] == seg.End {
			return true
		}
	}
	return false
}

// unindexed tells whether files of a container, by ffprobe's name for it,
// keep no index of their key frames: MPEG-TS, which OBS records and
// camcorders write as .MTS. ffmpeg seeks in it by the times of the
// packets, so a seek lands on whatever packet is stamped before the
// moment, mostly one between two key frames. The decoder drops everything
// up to the next key frame, and a short showed that key frame for every
// frame before it, plan row 2.200. MPEG-PS, .mpg and .vob, which ffprobe
// calls mpeg, has no index either, #268.
func unindexed(format string) bool {
	names := strings.Split(format, ",")
	return slices.Contains(names, "mpegts") || slices.Contains(names, "mpeg")
}

// pieceKey is where a piece of a clip is read from in a file with no
// index of its key frames, see keyBefore.
type pieceKey struct {
	start, end float64
	keyRead
}

// keyRead is a key frame of a file with no index of its key frames, in
// microseconds after the start of the file: at, where a read lands just
// before it is decoded, and key, when it is shown.
type keyRead struct{ at, key int64 }

// keyedPieces is the episode with the key frame found that each piece of
// a clip is read from, in a file with no index of its key frames, see
// unindexed. ffprobe reads the packets of the few seconds before the
// piece, and further back while there is no key frame among them. A piece
// whose key frame is not found is read the way it always was.
func (e *Engine) keyedPieces(ctx context.Context, path string, s SourceInfo, segs []Segment) SourceInfo {
	if !s.unindexed || !s.hasClock || s.FPSNum <= 0 || s.FPSDen <= 0 {
		return s
	}
	s.keyAt = nil
	for _, seg := range segs {
		cut := s.plainCut(seg)
		if k, ok := e.keyBefore(ctx, path, s, cut.seekUS); ok {
			s.keyAt = append(s.keyAt, pieceKey{seg.Start, seg.End, k})
		}
	}
	return s
}

// keyAtOf is where keyedPieces found a piece is read from.
func (s SourceInfo) keyAtOf(seg Segment) (keyRead, bool) {
	for _, k := range s.keyAt {
		if k.start == seg.Start && k.end == seg.End {
			return k.keyRead, true
		}
	}
	return keyRead{}, false
}

// keyBefore is the last key frame shown at or before a moment, in
// microseconds after the start of the file, and where a read lands just
// before it is decoded: a millisecond before its packet, so the seek,
// which lands on the last packet stamped at or before where it is sent,
// lands after the key frame before it. ffprobe reads the packets of the
// few seconds before the moment, and further back while there is no key
// frame among them. None is found where the picture has no key frame
// before the moment at all.
func (e *Engine) keyBefore(ctx context.Context, path string, s SourceInfo, at int64) (keyRead, bool) {
	moment := float64(at) / 1e6
	for span := 2.0; span <= 128; span *= 2 {
		from := max(moment-span, 0)
		// Close to the start the file is read from its first packet, whose
		// key frame is decoded before the file's first moment.
		interval := "%" + fixed(s.origin+moment+frameHair, 6)
		if from > 0 {
			interval = fixed(s.origin+from, 6) + interval
		}
		res := run(ctx, "", e.FFprobe, "-v", "error", "-select_streams", "v:0",
			"-read_intervals", interval,
			"-show_entries", "packet=pts_time,dts_time,flags", "-of", "csv=p=0", path)
		if res.Code != 0 {
			return keyRead{}, false
		}
		// Every packet read, in the order they are decoded. MPEG-PS
		// stamps no time on some, a key frame's among them, and the
		// decoder shows such a frame as long after it is decoded as it
		// holds frames back: the most any stamped frame is, #268.
		type packet struct {
			pts, dts     float64
			key, stamped bool
		}
		var packets []packet
		held := 0.0
		for _, line := range strings.Fields(res.Stdout) {
			f := strings.Split(line, ",")
			if len(f) < 3 {
				continue
			}
			dts, err := strconv.ParseFloat(f[1], 64)
			if err != nil {
				continue
			}
			pts, err := strconv.ParseFloat(f[0], 64)
			p := packet{pts: pts, dts: dts, key: strings.HasPrefix(f[2], "K"), stamped: err == nil}
			if p.stamped {
				held = max(held, pts-dts)
			}
			packets = append(packets, p)
		}
		best, found := math.Inf(-1), false
		var decoded float64
		for i, p := range packets {
			// The seek lands on the packet before the key frame, which the
			// decoder drops. Where that one is a key frame too, as an
			// encoder of MPEG-2 can make two in a row, the decoder starts
			// there, so a piece is read from the first of such a run.
			if !p.key || i > 0 && packets[i-1].key {
				continue
			}
			if !p.stamped {
				p.pts = p.dts + held
			}
			if p.pts-s.origin > moment+frameHair || p.pts <= best {
				continue
			}
			best, decoded, found = p.pts, p.dts, true
		}
		if found {
			return keyRead{
				at:  max(int64(math.Round((decoded-s.origin-frameHair)*1e6)), 0),
				key: int64(math.Round((best - s.origin) * 1e6)),
			}, true
		}
		if from == 0 {
			return keyRead{}, false
		}
	}
	return keyRead{}, false
}

// lastMoment is where the last packet of a file ends, counted from start,
// the start of the file, for a file that does not say how long it is.
// Nought for one that cannot be read. Every packet is read, a few seconds
// for an episode of hours, so the answer is kept for as long as the file
// stays as it is.
func (e *Engine) lastMoment(ctx context.Context, path string, start float64) float64 {
	stamp := ""
	if info, err := os.Stat(path); err == nil {
		stamp = fmt.Sprintf("%s|%d|%d|%v", path, info.Size(), info.ModTime().UnixNano(), start)
		if end, ok := lastMoments.Load(stamp); ok {
			return end.(float64)
		}
	}
	res := run(ctx, "", e.FFprobe, "-v", "error",
		"-show_entries", "packet=pts_time,duration_time", "-of", "csv=p=0", path)
	if res.Code != 0 {
		return 0
	}
	end := 0.0
	for _, line := range strings.Fields(res.Stdout) {
		f := strings.Split(line, ",")
		if len(f) < 2 {
			continue
		}
		pts, err1 := strconv.ParseFloat(f[0], 64)
		length, err2 := strconv.ParseFloat(f[1], 64)
		if err1 != nil {
			continue
		}
		if err2 != nil {
			length = 0
		}
		end = max(end, pts+length-start)
	}
	if stamp != "" {
		lastMoments.Store(stamp, end)
	}
	return end
}

// lastMoments is what lastMoment found, by the file's path, size, time
// and start.
var lastMoments sync.Map
