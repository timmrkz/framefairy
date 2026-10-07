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
	// decoders holds, per episode file, which decoder framing was found to
	// use, so it is looked up and reported once.
	decoders sync.Map

	// checkpointEvery and levelsEvery are how often a transcription and the
	// loudness measure save what they have, when not the defaults. A test
	// sets them on its own engine, so it can still run beside the others,
	// where a package variable made every test that changed it wait.
	checkpointEvery time.Duration
	levelsEvery     time.Duration

	mu               sync.Mutex
	encoder          Encoder
	subtitleTemplate string
	faces            *faceDetector
	facesLoaded      bool
}

// NewEngine makes an engine with the tools it calls: the ones named in the
// environment, or else the ones beside the program, checked. See FindTool
// in tools.go. A tool that may not run is left empty, and Preflight says
// why.
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
	if name == "" {
		return result{Code: 127, Stderr: "no tool that may run, see Preflight"}
	}
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
	return e.runFFmpegTo(ctx, args, label, total, cwd, nil)
}

// runFFmpegTo is RunFFmpeg with what ffmpeg writes to its standard output
// kept in stdout, for a run that pipes frames out while it reports.
func (e *Engine) runFFmpegTo(ctx context.Context, args []string, label string,
	total float64, cwd string, stdout io.Writer) (int, string) {
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
	cmd.Stdout = stdout
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
	version := run(ctx, "", e.FFmpeg, "-version")
	if version.Code != 0 {
		e.Log.ClearProgress()
		return renderErr("%s does not run: %s", e.FFmpeg, Scrub(version.Stderr, 200))
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
	// Where the file starts on its own clock, which ffprobe's
	// -read_intervals counts in, and whether that is known.
	origin   float64
	hasClock bool
	// The pieces of a clip whose own frames come at uneven times in a
	// file Probe found even, see unevenPieces.
	unevenAt [][2]float64
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
	res := run(ctx, "", e.FFprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=start_time", "-show_entries", "format=start_time",
		"-of", "json", path)
	if res.Code != 0 {
		return 0
	}
	var data struct {
		Streams []map[string]any `json:"streams"`
		Format  map[string]any   `json:"format"`
	}
	decoder := json.NewDecoder(strings.NewReader(res.Stdout))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil || len(data.Streams) == 0 {
		return 0
	}
	return videoStartOf(data.Streams[0], data.Format)
}

// Probe reads the dimensions, frame rate, duration and colour tags of a file.
func (e *Engine) Probe(ctx context.Context, path string) (SourceInfo, error) {
	res := run(ctx, "", e.FFprobe, "-v", "error",
		"-select_streams", "v:0",
		"-show_entries",
		"stream=width,height,r_frame_rate,avg_frame_rate,start_time,color_primaries,color_transfer,color_space",
		"-show_entries", "format=duration,start_time",
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
	videoStart := videoStartOf(stream, data.Format)
	first, hasFirst := toFloat(stream["start_time"])
	info := SourceInfo{Width: width, Height: height, FPSNum: num, FPSDen: den,
		Duration: duration, VideoStart: videoStart, Colour: colour,
		origin: first - videoStart, hasClock: hasFirst}
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
// the rate and still be on it: more than ffmpeg's rounding of a time to
// the ticks of a stream, half of Matroska's millisecond or of a phone's
// 1/600 s, and much less than a frame.
const frameHair = 0.001

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
	for _, line := range strings.Fields(res.Stdout) {
		if t, err := strconv.ParseFloat(strings.Trim(line, ","), 64); err == nil {
			begins = append(begins, t)
		}
	}
	// In the order they are shown, which B-frames change.
	slices.Sort(begins)
	for i, t := range begins {
		k := (t - first) / frame
		if math.Abs(k-math.Round(k))*frame > frameHair {
			return true
		}
		if i > 0 && math.Abs(t-begins[i-1]-frame) > frameHair {
			return true
		}
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
