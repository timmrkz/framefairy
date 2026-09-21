package engine

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Fade is the audio fade at each internal join, in seconds. It kills the
// click a hard audio cut makes.
const Fade = 0.015

// RenderSettings are the encoder choices for one run.
type RenderSettings struct {
	OutW, OutH   int
	CRF          int
	Preset       string
	AudioBitrate string
	ScaleUp      bool
	ScaleFlags   string
}

// BuildFilterGraph makes one filter chain per segment, each reading its own
// seeked input.
//
// Every segment is a separate -i with its own -ss, so ffmpeg seeks to a
// keyframe near the segment and decodes only what the clip needs, instead of
// decoding the episode from frame zero.
func (e *Engine) BuildFilterGraph(ctx context.Context, clip Clip, source SourceInfo,
	rs RenderSettings, assName string) (graph, videoLabel, audioLabel string, err error) {
	cropW, cropH := CropWindow(source, rs.OutW, rs.OutH)
	flags := rs.ScaleFlags
	if flags == "" {
		flags = "lanczos"
	}
	var parts []string
	for i, seg := range clip.Segments {
		cropY := (source.Height - cropH) / 2
		cropY -= cropY % 2
		cropX := ClampCropX(seg.CropX, cropW, source.Width)

		chain := []string{
			"setpts=PTS-STARTPTS",
			fmt.Sprintf("crop=%d:%d:%d:%d", cropW, cropH, cropX, cropY),
		}
		if rs.ScaleUp && (cropW != rs.OutW || cropH != rs.OutH) {
			chain = append(chain, fmt.Sprintf("scale=%d:%d:flags=%s", rs.OutW, rs.OutH, flags))
		}
		chain = append(chain, "fps="+source.FPSString(), "format=yuv420p", "setsar=1")
		parts = append(parts, fmt.Sprintf("[%d:v]%s[v%d];", i, strings.Join(chain, ","), i))

		fadeOutAt := math.Max(0, seg.Duration()-Fade)
		achain := []string{
			"asetpts=PTS-STARTPTS",
			"aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo",
			fmt.Sprintf("afade=t=in:st=0:d=%s", pyFloatRepr(Fade)),
			fmt.Sprintf("afade=t=out:st=%s:d=%s", fixed(fadeOutAt, 4), pyFloatRepr(Fade)),
		}
		parts = append(parts, fmt.Sprintf("[%d:a]%s[a%d];", i, strings.Join(achain, ","), i))
	}

	n := len(clip.Segments)
	if n == 1 {
		videoLabel, audioLabel = "[v0]", "[a0]"
	} else {
		var pairs strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&pairs, "[v%d][a%d]", i, i)
		}
		parts = append(parts, fmt.Sprintf("%sconcat=n=%d:v=1:a=1[vcat][acat];", pairs.String(), n))
		videoLabel, audioLabel = "[vcat]", "[acat]"
	}

	if assName != "" {
		template, err := e.SubtitleFilter(ctx)
		if err != nil {
			return "", "", "", err
		}
		parts = append(parts, videoLabel+subtitleChain(template, assName)+"[vout];")
		videoLabel = "[vout]"
	}
	graph = strings.TrimRight(strings.Join(parts, ""), ";")
	return graph, videoLabel, audioLabel, nil
}

// BuildCommand is the whole ffmpeg invocation for one clip.
func (e *Engine) BuildCommand(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outPath string, rs RenderSettings, assName string) ([]string, error) {
	graph, videoLabel, audioLabel, err := e.BuildFilterGraph(ctx, clip, source, rs, assName)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		abs = sourcePath
	}
	cmd := []string{e.FFmpeg, "-hide_banner", "-loglevel", "error", "-stats", "-y"}
	for _, seg := range clip.Segments {
		// -ss before -i seeks rather than decoding up to the point, and -t
		// limits how much is read. Both are input options on purpose.
		cmd = append(cmd, "-ss", fixed(seg.Start, 3), "-t", fixed(seg.Duration(), 3),
			"-i", abs)
	}
	cmd = append(cmd,
		"-filter_complex", graph,
		"-map", videoLabel, "-map", audioLabel,
		"-c:v", "libx264",
		"-preset", rs.Preset,
		"-crf", strconv.Itoa(rs.CRF),
		"-profile:v", "high",
		"-pix_fmt", "yuv420p",
		"-fps_mode", "cfr",
		"-r", source.FPSString(),
		"-c:a", "aac", "-b:a", rs.AudioBitrate, "-ar", "48000",
		"-movflags", "+faststart",
	)
	for _, tag := range source.Colour {
		cmd = append(cmd, "-"+tag[0], tag[1])
	}
	return append(cmd, outPath), nil
}

// RenderClip writes one finished short.
func (e *Engine) RenderClip(ctx context.Context, clip Clip, sourcePath string,
	source SourceInfo, outDir string, cues []Caption, rs RenderSettings,
	style map[string]any, captionDir string, dryRun bool) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(captionDir, 0o755); err != nil {
		return "", err
	}

	assName := ""
	if len(cues) > 0 && !e.SkipCaptions {
		// ffmpeg runs with the caption directory as its working directory, so
		// the ass filter can reference a bare filename. That sidesteps the
		// filter-graph escaping rules for colons and backslashes in paths.
		assPath, err := SafeChild(captionDir, clip.Basename()+".ass")
		if err != nil {
			return "", err
		}
		if err := e.WriteASS(ctx, cues, assPath, rs.OutW, rs.OutH, style); err != nil {
			return "", err
		}
		// The face travels with the program, so the render never depends on
		// what is installed on the machine.
		if err := InstallFont(captionDir, ResolveStyle(style).Font); err != nil {
			return "", err
		}
		assName = clip.Basename() + ".ass"
	}

	outPath, err := SafeChild(outDir, clip.Basename()+".mp4")
	if err != nil {
		return "", err
	}
	cmd, err := e.BuildCommand(ctx, clip, sourcePath, source, outPath, rs, assName)
	if err != nil {
		return "", err
	}
	if dryRun {
		quoted := make([]string, len(cmd))
		for i, c := range cmd {
			quoted[i] = shellQuote(c)
		}
		fmt.Println(strings.Join(quoted, " "))
		return outPath, nil
	}

	code, stderr := e.RunFFmpeg(ctx, cmd[1:], "rendering "+clip.Basename(),
		clip.Duration(), resolvePath(captionDir))
	if ctx.Err() != nil {
		// A truncated mp4 looks like a finished one on disk. Better to have
		// nothing than something that plays for four seconds and stops.
		os.Remove(outPath)
		return "", ctx.Err()
	}
	if code != 0 {
		os.Remove(outPath)
		tail := strip(stderr)
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		return "", renderErr("ffmpeg failed rendering %s:\n%s", clip.Basename(), tail)
	}

	// An exit code of zero is not proof that the clip is right. A filter
	// graph that quietly drops a segment still exits cleanly, and the result
	// looks like a finished file until someone plays it.
	check := run(ctx, "", e.FFprobe, "-v", "error", "-show_entries",
		"format=duration", "-of", "csv=p=0", outPath)
	written, ok := parsePyFloat(strip(check.Stdout))
	if !ok {
		return "", renderErr("%s was written but has no readable duration", clip.Basename())
	}
	if drift := math.Abs(written - clip.Duration()); drift > 0.5 {
		return "", renderErr("%s should be %.1fs but came out %.1fs. Something in the "+
			"filter graph dropped material, so the file is not trustworthy.",
			clip.Basename(), clip.Duration(), written)
	}
	return outPath, nil
}
