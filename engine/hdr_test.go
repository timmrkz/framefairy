package engine

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// An HDR episode makes an HDR short and a standard one a standard short,
// each with the episode's own colour tags, see HDR in
// docs/VIDEO-PREVIEW.md. Plan row 2.156, step 4.
func TestAnHDREpisodeMakesAnHDRShort(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	encoders, _ := exec.Command(e.FFmpeg, "-hide_banner", "-encoders").Output()
	if !hasEncoder(string(encoders), "libx265") && !hasEncoder(string(encoders), "hevc_videotoolbox") {
		ffmpegtest.Unusable(t, "this ffmpeg has no encoder of HEVC in 10 bits")
	}
	hevc := []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error", "-pix_fmt", "yuv420p10le"}
	if !hasEncoder(string(encoders), "libx265") {
		hevc = []string{"-c:v", "hevc_videotoolbox", "-profile:v", "main10", "-pix_fmt", "p010le", "-b:v", "4M"}
	}
	h264 := []string{"-c:v", "mpeg4", "-q:v", "2", "-pix_fmt", "yuv420p"}
	for _, c := range []struct {
		name, tags string
		args       []string
		codec, trc string
	}{
		{"HLG", "color_primaries=bt2020:color_trc=arib-std-b67:colorspace=bt2020nc", hevc, "hevc", "arib-std-b67"},
		{"PQ", "color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc", hevc, "hevc", "smpte2084"},
		{"BT.709", "color_primaries=bt709:color_trc=bt709:colorspace=bt709", h264, "h264", "bt709"},
	} {
		dir := t.TempDir()
		source := filepath.Join(dir, "episode.mp4")
		args := append([]string{"-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=4",
			"-f", "lavfi", "-i", "sine=f=440:r=48000:d=4",
			"-vf", "setparams=" + c.tags + ":range=tv"}, c.args...)
		args = append(args, "-c:a", "aac", "-shortest", source)
		if out, err := exec.Command(e.FFmpeg, args...).CombinedOutput(); err != nil {
			ffmpegtest.Unusable(t, "%s: making the episode: %s %s", c.name, err, out)
		}
		info, err := e.Probe(ctx, source)
		if err != nil {
			t.Fatal(err)
		}
		clip := info.OnFrames(Clip{ID: "01", Slug: "hdr", Segments: []Segment{{Start: 0.5, End: 2.5}}})
		rs := RenderSettings{OutW: 180, OutH: 320, CRF: 30, Preset: "ultrafast", AudioBitrate: "128k", ScaleUp: true}
		short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), nil, rs, nil,
			filepath.Join(dir, "captions"), false)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		out, err := exec.Command(e.FFprobe, "-v", "error", "-select_streams", "v:0", "-show_entries",
			"stream=codec_name,codec_tag_string,pix_fmt,color_transfer,color_primaries,color_space,bits_per_raw_sample",
			"-of", "default=nw=1", short).Output()
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			k, v, _ := strings.Cut(line, "=")
			got[k] = v
		}
		want := map[string]string{"codec_name": c.codec, "color_transfer": c.trc}
		if c.codec == "hevc" {
			want["codec_tag_string"] = "hvc1"
			want["color_primaries"] = "bt2020"
			want["color_space"] = "bt2020nc"
		} else {
			want["pix_fmt"] = "yuv420p"
			want["color_primaries"] = "bt709"
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: the short's %s is %q, want %q", c.name, k, got[k], v)
			}
		}
		if c.codec == "hevc" && !strings.Contains(got["pix_fmt"], "10") {
			t.Errorf("%s: the short is %s, not 10 bits", c.name, got["pix_fmt"])
		}
	}
}
