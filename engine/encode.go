package engine

import (
	"context"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Which encoder writes the H.264 in a finished short.
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

// Encoder is one way of writing H.264, and how to ask it for a quality.
type Encoder struct {
	// Name is the ffmpeg encoder, as `ffmpeg -encoders` lists it.
	Name string
	// Quality turns the wanted quality into this encoder's own arguments.
	// Every encoder has its own idea of what a number means and most of
	// them do not have -crf at all.
	Quality func(rs RenderSettings) []string
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
// possible. It is a starting point to be looked at on a real render rather
// than a measured equivalence, and docs/PACKAGING.md says so.
var videoToolbox = Encoder{
	Name: "h264_videotoolbox",
	Quality: func(rs RenderSettings) []string {
		q := 100 - 3*rs.CRF/2
		if q < 1 {
			q = 1
		}
		if q > 100 {
			q = 100
		}
		return []string{"-q:v", strconv.Itoa(q)}
	},
}

// known encoders by name, for --encoder.
var known = map[string]Encoder{
	x264.Name:         x264,
	videoToolbox.Name: videoToolbox,
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
	chosen, err := e.VideoEncoder(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"-c:v", chosen.Name}
	if chosen.Quality != nil {
		args = append(args, chosen.Quality(rs)...)
	}
	return args, nil
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
