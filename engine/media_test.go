package engine

import (
	"context"
	"io"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/framewire"
)

// A machine handed no media engine, as every test's is, reaches nothing
// of the system's: no encoder of Apple's, and an episode's decoder that is
// never told it may use the graphics chip. Handed a Mac's, it reaches for
// both. Framing reaches for neither, see TestAMacFramesOnTheProcessor. That is the whole difference between a
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
	if args := own.EpisodeFrames("framefairy-frames", "folge.mp4").args(); !slices.Equal(args, []string{"folge.mp4"}) {
		t.Errorf("a machine handed no media engine starts the episode's decoder with %v", args)
	}

	// ffmpeg's own encoder is the only one: one that fails or stalls is
	// not handed on, so a test whose encoder fails fails.
	if _, ok := own.encoderAfter(context.Background(), x264, framewire.SDR); ok {
		t.Error("ffmpeg's own libx264 goes on with another encoder")
	}
	if d := own.stallFor(x264, framewire.SDR); d != 0 {
		t.Errorf("ffmpeg's own libx264 is given up after %s without a frame", d)
	}

	// Each program's main hands its machine the system's, see UseSystem.
	program := &Engine{Machine: NewMachine(afero.NewMemMapFs())}
	program.UseSystem()
	if want := SystemMedia(runtime.GOOS); !slices.EqualFunc(program.order(false), want.Encoders, Encoder.is) ||
		program.mediaOf().Decodes != want.Decodes {
		t.Errorf("UseSystem hands the machine %v, not this system's %v", names(program.order(false)), names(want.Encoders))
	}

	mac := &Engine{Machine: NewMachine(afero.NewMemMapFs())}
	mac.useMedia(SystemMedia("darwin"))
	if order := mac.order(false); len(order) == 0 || !order[0].is(videoToolbox) {
		t.Errorf("a Mac reaches for %v first", names(order))
	}
	if order := mac.order(true); len(order) == 0 || !order[0].is(hevcVideoToolbox) {
		t.Errorf("a Mac reaches for %v first for HDR", names(order))
	}
	if args := mac.EpisodeFrames("framefairy-frames", "folge.mp4").args(); !slices.Equal(args, []string{"-chip", "folge.mp4"}) {
		t.Errorf("a Mac starts the episode's decoder with %v", args)
	}
}

// What Apple's encoders are handed for a short, argument by argument, the
// way they took it on Tim's Mac. No test runs them, so a change to any of
// these is a change of what a real Mac is asked, and shows here first.
// The face detector is the program's to hand in, the way the media engine
// is: UseSystem hands it, a test's machine has none and frames by what is
// in focus, and the command line takes it away when FRAMEFAIRY_NO_FACES
// is set. The engine reads no switch of its own for it.
func TestOnlyAMachineHandedTheFaceDetectorLooksForFaces(t *testing.T) {
	t.Setenv("FRAMEFAIRY_NO_FACES", "1")
	if own := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs())); own.faceDetector() != nil {
		t.Error("a machine NewMachine makes looks for faces")
	}
	program := NewEngine(runlog.New(io.Discard, false, false), NewMachine(afero.NewMemMapFs()))
	program.UseSystem()
	if program.faceDetector() == nil {
		t.Error("UseSystem hands the machine no face detector, so the app frames every clip by what is in focus, whatever the environment says")
	}
	program.FrameByFocus()
	if program.faceDetector() != nil {
		t.Error("FrameByFocus leaves the machine its face detector")
	}
}

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
