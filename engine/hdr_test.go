package engine

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
)

// An HDR episode makes an HDR short and a standard one a standard short,
// each with the episode's own colour tags, see HDR in
// docs/VIDEO-PREVIEW.md. Plan row 2.156, step 4.
//
// On the real disk: ffmpeg reads the episode and renders the short.
func TestAnHDREpisodeMakesAnHDRShort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	for _, c := range []struct {
		name, file string
		codec, trc string
	}{
		{"HLG", "colour-hlg-10.mp4", "hevc", "arib-std-b67"},
		{"PQ", "colour-pq-10.mp4", "hevc", "smpte2084"},
		{"BT.709", "colour-bt709-tv.mp4", "h264", "bt709"},
	} {
		dir := t.TempDir()
		source := fixtures.Path(t, c.file)
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
