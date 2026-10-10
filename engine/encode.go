package engine

import (
	"context"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"framefairy/internal/framewire"
)

// Which encoder writes the picture of a finished short: H.264 for standard
// video, and HEVC in 10 bits for HDR, see HDREncoder.
//
// It used to be libx264 and nothing else, and Preflight refused to run
// without it. libx264 is the one thing that makes an ffmpeg build GPL, and
// the ffmpeg we ship is built without it, so the encoder has to be the one
// the system already has. See docs/PACKAGING.md.
//
// It is also the same choice on Tim's machine as in the product, which is
// the point: an encoder chosen at render time from what this ffmpeg can do
// means the development build and the shipped one take the same path
// through the same code.

// Encoder is one way of writing a short's picture, and how to ask it for a
// quality.
type Encoder struct {
	// Name is the ffmpeg encoder, as `ffmpeg -encoders` lists it.
	Name string
	// Quality turns the wanted quality into this encoder's own arguments.
	// Every encoder has its own idea of what a number means and most of
	// them do not have -crf at all.
	Quality func(rs RenderSettings) []string
	// Picture is how an encoder of HDR is asked for 10 bits, its profile
	// and the tag Apple's players look for. Standard video is H.264 in
	// 8 bits, asked the same of every encoder, see BuildCommand.
	Picture []string
}

// x264 takes the quality as it is written, because CRF is its own idea and
// the flags are named after it.
var x264 = Encoder{
	Name: "libx264",
	Quality: func(rs RenderSettings) []string {
		return []string{"-preset", rs.Preset, "-crf", strconv.Itoa(rs.CRF)}
	},
}

// videoToolbox is Apple's encoder, in the media engine rather than the
// cores, so it is faster than libx264 as well as free of it.
//
// It has no CRF and no preset. It takes -q:v, from 1 to 100, where higher
// is better, which is the other way round from CRF. The mapping below is
// deliberately generous, because a short is twenty to thirty seconds and
// the product rule is that the picture stays as close to the original as
// possible. It started at 73 for the default CRF 18. The first real renders
// on Tim's Mac showed blocks in the shadows, where an encoder saves first,
// so the default is 85 now: each step of CRF is five sixths of a step of
// q, and CRF 0 is still 100.
var videoToolbox = Encoder{
	Name:    "h264_videotoolbox",
	Quality: toolboxQuality,
}

func toolboxQuality(rs RenderSettings) []string {
	q := 100 - (5*rs.CRF+3)/6
	if q < 1 {
		q = 1
	}
	if q > 100 {
		q = 100
	}
	return []string{"-q:v", strconv.Itoa(q)}
}

// An HDR episode makes an HDR short, see HDR in docs/VIDEO-PREVIEW.md:
// HEVC with 10-bit colour, the form an iPhone films in and Instagram and
// YouTube take, tagged hvc1 so QuickTime and Photos play it. On the Mac it
// is VideoToolbox's, asked for quality the same way as its H.264.
var hevcVideoToolbox = Encoder{
	Name:    "hevc_videotoolbox",
	Quality: toolboxQuality,
	Picture: []string{"-profile:v", "main10", "-pix_fmt", "p010le", "-tag:v", "hvc1"},
}

// x265 takes CRF as x264 does. It is GPL like x264, so the ffmpeg we ship
// does not have it, and an ffmpeg that does makes HDR shorts with it.
var x265 = Encoder{
	Name: "libx265",
	Quality: func(rs RenderSettings) []string {
		return []string{"-preset", rs.Preset, "-crf", strconv.Itoa(rs.CRF), "-x265-params", "log-level=error"}
	},
	Picture: []string{"-profile:v", "main10", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1"},
}

// known encoders by name, for --encoder.
var known = map[string]Encoder{
	x264.Name:         x264,
	videoToolbox.Name: videoToolbox,
}

// hdrEncoderOrder is what to try for an HDR short, best first. Windows
// and Linux have no encoder of HEVC in 10 bits in the ffmpeg we ship, which
// is a question for docs/PACKAGING.md.
func hdrEncoderOrder() []Encoder {
	if runtime.GOOS == "darwin" {
		return []Encoder{hevcVideoToolbox, x265}
	}
	return []Encoder{x265}
}

// encoderOrder is what to try on this system, best first. The first one
// this ffmpeg actually has is the one used.
//
// Windows and Linux get their own entries when those builds are made.
// Windows has h264_mf and Linux has VA-API or openh264, and neither is
// written here yet because neither has been run. A guess in this table
// would render every short on that system.
func encoderOrder() []Encoder { return encoderOrderFor(runtime.GOOS) }

// Split from the above so the table can be read on any machine. The order
// for macOS is the one that matters most and the machine that runs the
// tests in a cloud session is not a Mac.
func encoderOrderFor(goos string) []Encoder {
	switch goos {
	case "darwin":
		return []Encoder{videoToolbox, x264}
	default:
		return []Encoder{x264}
	}
}

// VideoEncoder is the encoder this run uses, found once and kept.
//
// Named rather than found when Options.Encoder says so, which is the escape
// hatch for a machine whose ffmpeg is unusual, and for comparing two
// encoders on the same clip.
func (e *Engine) VideoEncoder(ctx context.Context) (Encoder, error) {
	e.mu.Lock()
	if e.encoder.Name != "" {
		chosen := e.encoder
		e.mu.Unlock()
		return chosen, nil
	}
	e.mu.Unlock()

	have := run(ctx, "", e.FFmpeg, "-hide_banner", "-encoders")
	if have.Code != 0 {
		return Encoder{}, renderErr("%s cannot list its encoders.", e.FFmpeg)
	}

	if e.WantEncoder != "" {
		if !hasEncoder(have.Stdout, e.WantEncoder) {
			return Encoder{}, renderErr("this ffmpeg has no %s encoder.", e.WantEncoder)
		}
		chosen, ok := known[e.WantEncoder]
		if !ok {
			// Something we have no quality mapping for. Take it at its
			// word and let it use its own defaults rather than passing a
			// number that means something else to it.
			chosen = Encoder{Name: e.WantEncoder, Quality: func(RenderSettings) []string { return nil }}
		}
		e.keepEncoder(chosen)
		return chosen, nil
	}

	var tried []string
	for _, candidate := range encoderOrder() {
		if hasEncoder(have.Stdout, candidate.Name) {
			e.keepEncoder(candidate)
			e.Log.Detail("video encoder: %s", candidate.Name)
			return candidate, nil
		}
		tried = append(tried, candidate.Name)
	}
	return Encoder{}, renderErr("this ffmpeg has none of the encoders this system can use: %s.",
		strings.Join(tried, ", "))
}

// HDREncoder is the encoder this run uses for an HDR short, found once
// and kept. --encoder names the encoder of standard video only.
func (e *Engine) HDREncoder(ctx context.Context) (Encoder, error) {
	e.mu.Lock()
	if e.hdrEncoder.Name != "" {
		chosen := e.hdrEncoder
		e.mu.Unlock()
		return chosen, nil
	}
	e.mu.Unlock()
	have := run(ctx, "", e.FFmpeg, "-hide_banner", "-encoders")
	if have.Code != 0 {
		return Encoder{}, renderErr("%s cannot list its encoders.", e.FFmpeg)
	}
	var tried []string
	for _, candidate := range hdrEncoderOrder() {
		if hasEncoder(have.Stdout, candidate.Name) {
			e.mu.Lock()
			e.hdrEncoder = candidate
			e.mu.Unlock()
			e.Log.Detail("video encoder for HDR: %s", candidate.Name)
			return candidate, nil
		}
		tried = append(tried, candidate.Name)
	}
	return Encoder{}, renderErr("this video is HDR, and this ffmpeg has no encoder of HEVC in 10 bits "+
		"to keep it HDR: %s.", strings.Join(tried, ", "))
}

func (e *Engine) keepEncoder(chosen Encoder) {
	e.mu.Lock()
	e.encoder = chosen
	e.mu.Unlock()
}

// hasEncoder looks for the name in an `ffmpeg -encoders` listing. Each line
// is a row of flags, then the name, then a description, and a description
// may well mention another encoder, so the name is matched as its own word
// in the second column rather than anywhere in the line.
func hasEncoder(listing, name string) bool {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return true
		}
	}
	return false
}

// VideoArgs is the -c:v and the quality for one render.
func (e *Engine) VideoArgs(ctx context.Context, rs RenderSettings) ([]string, error) {
	return e.videoArgs(ctx, rs, framewire.SDR)
}

// videoArgs is VideoArgs for a short of standard video or of HDR, with
// what an encoder of HDR is asked for its 10 bits.
func (e *Engine) videoArgs(ctx context.Context, rs RenderSettings, light framewire.Light) ([]string, error) {
	pick := e.VideoEncoder
	if light != framewire.SDR {
		pick = e.HDREncoder
	}
	chosen, err := pick(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"-c:v", chosen.Name}
	if chosen.Quality != nil {
		args = append(args, chosen.Quality(rs)...)
	}
	return append(args, chosen.Picture...), nil
}

// EncoderNames is every encoder this build knows how to drive, for the
// command line's help. This system's order first, then anything else, so
// the first name read is the one that would be chosen.
func EncoderNames() string {
	var names []string
	seen := map[string]bool{}
	for _, candidate := range encoderOrder() {
		names = append(names, candidate.Name)
		seen[candidate.Name] = true
	}
	rest := make([]string, 0, len(known))
	for name := range known {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return strings.Join(append(names, rest...), ", ")
}
