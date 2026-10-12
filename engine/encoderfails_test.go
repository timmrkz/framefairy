package engine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"framefairy/internal/ffmpegtest"
	"framefairy/internal/framewire"
)

// A render goes on when the Mac's media engine fails to encode. VideoToolbox
// failed a render with -12912 on CI's virtual Mac, and the short failed with
// nothing to fall back on. Now the same encoder goes on on the processor,
// Apple's own encoder in macOS, and the short is made. Plan row 2.223.
//
// The media engine is made to fail by a stand-in for ffmpeg that fails
// every render not asked for the processor, the way VideoToolbox does. A
// render on the processor goes to the real ffmpeg: on a Mac with Apple's
// encoder as it is asked, which proves the processor takes what the
// render asks of it, and elsewhere with libx264 in its place, which proves
// the way round on any machine.
//
// On the real disk: ffmpeg makes the episode and renders the short.
func TestARenderGoesOnWhenTheEncoderFails(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	listing, _ := exec.Command(e.FFmpeg, "-hide_banner", "-encoders").Output()
	onMac := hasEncoder(string(listing), "h264_videotoolbox")
	if !onMac && !hasEncoder(string(listing), "libx264") {
		ffmpegtest.Unusable(t, "this ffmpeg has neither VideoToolbox nor libx264")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "episode.mp4")
	if out, err := exec.Command(e.FFmpeg, "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=3",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=3",
		"-c:v", "mpeg4", "-q:v", "4", "-c:a", "aac", "-shortest", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer
	e.Log = NewLog(&log, false, false)
	calls := filepath.Join(dir, "calls")
	e.FFmpeg = failingMediaEngine(t, e.FFmpeg, calls, onMac)
	// The encoder Preflight found for this system is let go, so the
	// render reaches for the Mac's.
	e.system, e.encoder = "darwin", Encoder{}

	clip := info.OnFrames(Clip{ID: "01", Slug: "chip", Segments: []Segment{{Start: 0.5, End: 2.5}}})
	rs := RenderSettings{OutW: 90, OutH: 160, CRF: 30, Preset: "ultrafast", AudioBitrate: "96k", ScaleUp: true}
	short, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "out"), nil, rs, nil,
		filepath.Join(dir, "captions"), false)
	if err != nil {
		said, _ := os.ReadFile(calls + ".said")
		t.Fatalf("the short failed with the media engine: %v\nffmpeg on the processor said:\n%s", err, said)
	}
	if _, err := os.Stat(short); err != nil {
		t.Fatal(err)
	}
	made, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	renders := strings.Split(strings.TrimSpace(string(made)), "\n")
	if len(renders) != 2 || renders[0] != "media engine" || renders[1] != "processor" {
		t.Errorf("the renders were %q, not one on the media engine and one on the processor", renders)
	}
	said := log.String()
	if !strings.Contains(said, "h264_videotoolbox failed") || !strings.Contains(said, "-12912") {
		t.Errorf("the log does not say the media engine failed and why:\n%s", said)
	}

	// The next short tries the media engine again.
	os.Remove(calls)
	if _, err := e.RenderClip(ctx, clip, source, info, filepath.Join(dir, "again"), nil, rs, nil,
		filepath.Join(dir, "captions"), false); err != nil {
		t.Fatal(err)
	}
	made, _ = os.ReadFile(calls)
	if !strings.HasPrefix(string(made), "media engine\n") {
		t.Errorf("the next short started with %q, not on the media engine", made)
	}
}

// A failure that is not the encoder's own, of the filters or of reading the
// video, is not rendered again: another encoder would not mend it.
func TestOnlyAnEncoderThatFailedItselfIsLeft(t *testing.T) {
	e := encodingEngine(t, "h264_videotoolbox", "libx264")
	e.system = "darwin"
	ctx := context.Background()
	failed := videoToolbox
	cases := []struct {
		said string
		next string
	}{
		{"[h264_videotoolbox @ 0x6000013c] Error encoding frame: -12912\n", "h264_videotoolbox -require_sw 1"},
		{"[h264_videotoolbox @ 0x6000013c] Cannot create compression session: -12908\n", "h264_videotoolbox -require_sw 1"},
		{"[Parsed_crop_3 @ 0x6000013c] Invalid too big or non positive size\n", ""},
		{"episode.mp4: No such file or directory\n", ""},
	}
	for _, c := range cases {
		next, ok := e.nextEncoder(ctx, failed, framewire.SDR, c.said)
		got := ""
		if ok {
			got = next.String()
		}
		if got != c.next {
			t.Errorf("after %q the render goes on with %q, not %q", c.said, got, c.next)
		}
	}
	// On the processor it goes on with libx264 where this ffmpeg has it,
	// and an encoder with nothing after it is the end.
	if next, ok := e.nextEncoder(ctx, onProcessor(videoToolbox), framewire.SDR,
		"[h264_videotoolbox @ 0x1] Error encoding frame: -12902"); !ok || next.String() != "libx264" {
		t.Errorf("after the processor the render goes on with %q", next.String())
	}
	if _, ok := e.nextEncoder(ctx, x264, framewire.SDR, "[libx264 @ 0x1] broken"); ok {
		t.Error("the last encoder of the order had one after it")
	}
	// HDR goes on with HEVC on the processor.
	if next, ok := encodingEngineOn(t, "darwin", "hevc_videotoolbox").nextEncoder(ctx, hevcVideoToolbox,
		framewire.PQ, "[hevc_videotoolbox @ 0x1] Error encoding frame: -12912"); !ok ||
		next.String() != "hevc_videotoolbox -require_sw 1" {
		t.Errorf("after HEVC on the media engine the render goes on with %q", next.String())
	}
}

func encodingEngineOn(t *testing.T, goos string, encoders ...string) *Engine {
	e := encodingEngine(t, encoders...)
	e.system = goos
	return e
}

// failingMediaEngine is a stand-in for ffmpeg at real that lists
// VideoToolbox's encoders and fails every render that does not ask for the
// processor, the way the media engine fails, and writes which it was to
// calls. A render on the processor goes to real, as it is on a Mac and
// with libx264 for Apple's encoder elsewhere.
//
// On the real disk: the stand-in ffmpeg is a program that is run.
func failingMediaEngine(t *testing.T, real, calls string, onMac bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	toX264 := "1"
	if onMac {
		toX264 = ""
	}
	script := `#!/bin/sh
real='` + real + `'
calls='` + calls + `'
case " $* " in
*" -encoders "*)
  "$real" "$@"
  printf ' V....D h264_videotoolbox    VideoToolbox H.264 Encoder (codec h264)\n'
  printf ' V....D hevc_videotoolbox    VideoToolbox H.265 Encoder (codec hevc)\n'
  exit 0 ;;
*" -require_sw 1 "*)
  echo processor >> "$calls" ;;
*" h264_videotoolbox "*)
  echo 'media engine' >> "$calls"
  echo '[h264_videotoolbox @ 0x6000013c] Error encoding frame: -12912' >&2
  echo '[vost#0:0/h264_videotoolbox @ 0x6000013d] Error submitting video frame to the encoder' >&2
  exit 187 ;;
*) exec "$real" "$@" ;;
esac
# On the processor: all ffmpeg says is kept for the test, and elsewhere
# than on a Mac libx264 encodes in Apple's place.
x264='` + toX264 + `'
for a in "$@"; do
  if [ -n "$skip" ]; then skip=; shift; continue; fi
  case "$a" in
    error) set -- "$@" verbose ;;
    h264_videotoolbox) if [ -n "$x264" ]; then set -- "$@" libx264; else set -- "$@" "$a"; fi ;;
    -require_sw|-q:v) if [ -n "$x264" ]; then skip=1; else set -- "$@" "$a"; fi ;;
    *) set -- "$@" "$a" ;;
  esac
  shift
done
"$real" "$@" 2>"$calls.said"
code=$?
cat "$calls.said" >&2
exit $code
`
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// Apple's encoder on the processor knows no quality, and asked for -q:v it
// refused to start on CI's Mac, "Error setting bitrate property: -12900".
// It is asked for a bit rate as generous as the media engine's quality,
// and less for a worse quality.
func TestTheProcessorIsAskedForABitRate(t *testing.T) {
	args := func(crf int) []string {
		return onProcessor(videoToolbox).args(RenderSettings{OutW: 1080, OutH: 1920, CRF: crf, Rate: 30})
	}
	rate := func(args []string) int {
		t.Helper()
		line := strings.Join(args, " ")
		if strings.Contains(line, "-q:v") {
			t.Fatalf("Apple's encoder on the processor was asked for a quality: %v", args)
		}
		for i, a := range args {
			if a == "-b:v" && i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err != nil {
					t.Fatalf("-b:v is %q", args[i+1])
				}
				return n
			}
		}
		t.Fatalf("no bit rate in %v", args)
		return 0
	}
	def := rate(args(DefaultOptions().CRF))
	if def < 8_000_000 || def > 20_000_000 {
		t.Errorf("a short of 1080 by 1920 at 30 frames a second gets %d b/s, not about 12 Mb/s", def)
	}
	if worse := rate(args(30)); worse >= def {
		t.Errorf("crf 30 gets %d b/s and crf 18 %d, which is upside down", worse, def)
	}
	for _, crf := range []int{-5, 0, 51, 200} {
		if n := rate(args(crf)); n <= 0 {
			t.Errorf("crf %d gets %d b/s", crf, n)
		}
	}
	if got := strings.Join(args(18), " "); !strings.HasSuffix(got, "-require_sw 1") ||
		!strings.HasPrefix(got, "-c:v h264_videotoolbox") {
		t.Errorf("the processor is asked %q", got)
	}
}

// Apple's encoders on the processor take what a short asks of them, the
// HEVC one for HDR in 10 bits as well as the H.264 one. The render above
// only goes through H.264, and without this nothing would have rendered
// HDR on the processor before a customer's media engine failed. Mac only,
// where these encoders are.
//
// On the real disk: ffmpeg writes the short it encodes.
func TestTheProcessorEncodesWhatAShortAsks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	listing, _ := exec.Command(e.FFmpeg, "-hide_banner", "-encoders").Output()
	rs := RenderSettings{OutW: 180, OutH: 320, CRF: 18, Rate: 30}
	cases := []struct {
		name    string
		encoder Encoder
		input   string
		want    string
	}{
		{"H.264", onProcessor(videoToolbox), "yuv420p", "h264"},
		{"HEVC in 10 bits for HDR", onProcessor(hevcVideoToolbox), "p010le", "hevc (Main 10)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !hasEncoder(string(listing), c.encoder.Name) {
				t.Skipf("this ffmpeg has no %s, which only a Mac has", c.encoder.Name)
			}
			out := filepath.Join(t.TempDir(), "short.mp4")
			args := []string{"-hide_banner", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc2=s=180x320:r=30:d=1", "-vf", "format=" + c.input}
			args = append(append(args, c.encoder.args(rs)...), out)
			if said, err := exec.Command(e.FFmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("%s on the processor refused %v: %v\n%s", c.encoder.Name, args, err, said)
			}
			// ffmpeg asked about a file with no output says what it holds
			// and exits with an error, which is not a failure here.
			about, _ := exec.Command(e.FFmpeg, "-hide_banner", "-i", out).CombinedOutput()
			if !strings.Contains(string(about), "Video: "+c.want) {
				t.Errorf("the short is not %s:\n%s", c.want, about)
			}
		})
	}
}
