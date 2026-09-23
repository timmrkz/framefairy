package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RenderError is a failure the operator can act on, as opposed to a bug.
type RenderError struct{ Msg string }

func (e *RenderError) Error() string { return e.Msg }

func renderErr(format string, a ...any) error {
	return &RenderError{Msg: sprintf(format, a...)}
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

	mu               sync.Mutex
	encoder          Encoder
	subtitleTemplate string
	faces            *faceDetector
	facesLoaded      bool
}

// NewEngine makes an engine with the tools it calls: the ones named in the
// environment, then the ones beside the program, then the search path. See
// ToolPath in tools.go for why that order.
func NewEngine(log *Log) *Engine {
	return &Engine{Log: log,
		FFmpeg:  ToolPath("FRAMEFAIRY_FFMPEG", "ffmpeg"),
		FFprobe: ToolPath("FRAMEFAIRY_FFPROBE", "ffprobe"),
		NoFaces: os.Getenv("FRAMEFAIRY_NO_FACES") != ""}
}

type result struct {
	Code   int
	Stdout string
	Stderr string
}

// run executes a command as an argument list, never through a shell. A
// missing binary is a normal outcome here, so it comes back as exit code 127
// rather than as an error.
func run(ctx context.Context, cwd string, name string, args ...string) result {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
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
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.Bytes(), max(exit.ExitCode(), 1)
		}
		return nil, 127
	}
	return stdout.Bytes(), 0
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
		return 1, err.Error()
	}
	defer os.RemoveAll(tmp)
	progressPath := filepath.Join(tmp, "progress")
	stderrPath := filepath.Join(tmp, "stderr")
	full := append([]string{"-progress", progressPath, "-nostats"}, kept...)

	handle, err := os.Create(stderrPath)
	if err != nil {
		return 1, err.Error()
	}
	cmd := exec.CommandContext(ctx, e.FFmpeg, full...)
	cmd.Dir = cwd
	cmd.Stderr = handle
	started := time.Now()
	if err := cmd.Start(); err != nil {
		handle.Close()
		return 127, err.Error()
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()

	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	lastMove, lastDone, warned := time.Now(), -1.0, false
	var waitErr error
loop:
	for {
		select {
		case waitErr = <-finished:
			break loop
		case <-ticker.C:
		}
		done := parseProgress(progressPath)
		elapsed := time.Since(started).Seconds()
		if done > lastDone {
			lastDone, lastMove, warned = done, time.Now(), false
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
			left := 0.0
			if speed > 0.01 {
				left = (total - done) / speed
			}
			e.Log.ProgressOf(label, share, left)
		} else {
			e.Log.Progress(fmt.Sprintf("%s %.0fs", label, elapsed))
		}
	}
	handle.Close()
	e.Log.ClearProgress()
	stderr, _ := os.ReadFile(stderrPath)
	code := 0
	if waitErr != nil {
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			code = max(exit.ExitCode(), 1)
		} else {
			code = 1
		}
	}
	return code, strings.ToValidUTF8(string(stderr), "\uFFFD")
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

// SubtitleFilter works out how this ffmpeg wants a subtitle file named, once
// per engine.
func (e *Engine) SubtitleFilter(ctx context.Context) (string, error) {
	e.mu.Lock()
	template := e.subtitleTemplate
	e.mu.Unlock()
	if template != "" {
		return template, nil
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
	// Radius 0 keeps this out of the measuring path, which would call back
	// into this function and never return.
	name := "00_probe-name.ass"
	if err := e.WriteASS(ctx, []Caption{{Start: 0, End: 1, Text: "probe"}}, filepath.Join(tmp, name),
		320, 180, map[string]any{"radius": 0.0}); err != nil {
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
	version := run(ctx, "", e.FFmpeg, "-version")
	if version.Code != 0 {
		e.Log.ClearProgress()
		return renderErr("%s is not installed or not on PATH.\n"+
			"  brew tap homebrew-ffmpeg/ffmpeg\n"+
			"  brew install homebrew-ffmpeg/ffmpeg/ffmpeg", e.FFmpeg)
	}
	first := "ffmpeg ?"
	if lines := strings.SplitN(version.Stdout, "\n", 2); lines[0] != "" {
		first = lines[0]
	}
	e.Log.Detail("%s", first)

	if run(ctx, "", e.FFprobe, "-version").Code != 0 {
		e.Log.ClearProgress()
		return renderErr("%s is missing, which usually means a partial ffmpeg install.",
			e.FFprobe)
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
	Width    int
	Height   int
	FPSNum   int
	FPSDen   int
	Duration float64
	// Colour tags to copy to the output, as ffmpeg flag name and value.
	Colour [][2]string
}

// FPS is the frame rate as a number.
func (s SourceInfo) FPS() float64 { return float64(s.FPSNum) / float64(s.FPSDen) }

// FPSString is the frame rate as ffmpeg wants it, in lowest terms.
func (s SourceInfo) FPSString() string { return fmt.Sprintf("%d/%d", s.FPSNum, s.FPSDen) }

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

// Probe reads the dimensions, frame rate, duration and colour tags of a file.
func (e *Engine) Probe(ctx context.Context, path string) (SourceInfo, error) {
	res := run(ctx, "", e.FFprobe, "-v", "error",
		"-select_streams", "v:0",
		"-show_entries",
		"stream=width,height,r_frame_rate,color_primaries,color_transfer,color_space",
		"-show_entries", "format=duration",
		"-of", "json", path)
	if res.Code != 0 {
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

	width, okW := toInt(stream["width"])
	height, okH := toInt(stream["height"])
	if _, present := stream["width"]; !present || !okW {
		return SourceInfo{}, renderErr("%s reports no video dimensions", path)
	}
	if _, present := stream["height"]; !present || !okH {
		return SourceInfo{}, renderErr("%s reports no video dimensions", path)
	}
	return SourceInfo{Width: width, Height: height, FPSNum: num, FPSDen: den,
		Duration: duration, Colour: colour}, nil
}
