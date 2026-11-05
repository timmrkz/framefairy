package engine

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// A machine handed no media engine, as every test's is, reaches nothing
// of the system's: no encoder of Apple's, no -hwaccel, and an episode's
// decoder that is never told it may use the graphics chip. Handed a
// Mac's, it reaches for all three. That is the whole difference between a
// test's machine and the app's, see Media.
func TestOnlyAMachineHandedTheMediaEngineReachesIt(t *testing.T) {
	own := &Engine{Machine: NewMachine(afero.NewMemMapFs())}
	for _, hdr := range []bool{false, true} {
		for _, c := range own.order(hdr) {
			if strings.HasSuffix(c.Name, "_videotoolbox") {
				t.Errorf("a machine handed no media engine reaches for %s", c)
			}
		}
	}
	if flags := own.decodeFlags(); flags != nil {
		t.Errorf("a machine handed no media engine decodes with %v", flags)
	}
	if args := own.EpisodeFrames("framefairy-frames", "folge.mp4").args(); !slices.Equal(args, []string{"folge.mp4"}) {
		t.Errorf("a machine handed no media engine starts the episode's decoder with %v", args)
	}

	mac := &Engine{Machine: NewMachine(afero.NewMemMapFs())}
	mac.useMedia(SystemMedia("darwin"))
	if order := mac.order(false); len(order) == 0 || !order[0].is(videoToolbox) {
		t.Errorf("a Mac reaches for %v first", names(order))
	}
	if order := mac.order(true); len(order) == 0 || !order[0].is(hevcVideoToolbox) {
		t.Errorf("a Mac reaches for %v first for HDR", names(order))
	}
	if flags := mac.decodeFlags(); !slices.Equal(flags, []string{"-hwaccel", "auto"}) {
		t.Errorf("a Mac decodes with %v", flags)
	}
	if args := mac.EpisodeFrames("framefairy-frames", "folge.mp4").args(); !slices.Equal(args, []string{"-chip", "folge.mp4"}) {
		t.Errorf("a Mac starts the episode's decoder with %v", args)
	}
}

// What Apple's encoders are handed for a short, argument by argument, the
// way they took it on Tim's Mac. No test runs them, so a change to any of
// these is a change of what a real Mac is asked, and shows here first.
func TestWhatApplesEncodersAreHanded(t *testing.T) {
	ctx := context.Background()
	rs := RenderSettings{OutW: 1080, OutH: 1920, CRF: DefaultOptions().CRF, Rate: 30}
	e := encodingEngine(t, "h264_videotoolbox", "hevc_videotoolbox")
	e.useMedia(SystemMedia("darwin"))
	standard, err := e.VideoEncoder(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := e.HDREncoder(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		encoder Encoder
		want    string
	}{
		{"H.264 on the media engine", standard, "-c:v h264_videotoolbox -q:v 85 -profile:v high -pix_fmt yuv420p"},
		{"HEVC in 10 bits on the media engine", hdr,
			"-c:v hevc_videotoolbox -q:v 85 -profile:v main10 -pix_fmt p010le -tag:v hvc1"},
		{"H.264 on the processor", onProcessor(videoToolbox),
			"-c:v h264_videotoolbox -b:v 12441600 -profile:v high -pix_fmt yuv420p -require_sw 1"},
		{"HEVC in 10 bits on the processor", onProcessor(hevcVideoToolbox),
			"-c:v hevc_videotoolbox -b:v 12441600 -profile:v main10 -pix_fmt p010le -tag:v hvc1 -require_sw 1"},
	} {
		if got := strings.Join(c.encoder.args(rs), " "); got != c.want {
			t.Errorf("%s is handed %q, want %q", c.name, got, c.want)
		}
	}
}
