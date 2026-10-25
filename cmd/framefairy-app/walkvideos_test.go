package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// walkVideos keeps every video a bridge makes for a walk, by what it was
// asked for, in the folder FRAMEFAIRY_WALK_VIDEOS names, which make walks
// and TestWalks share between the bridges of a run and CI keeps between
// runs, or else, and whenever there is no recipe, in a folder of this
// bridge's own. A walk that adds the
// same video again gets a copy of it rather than waiting for ffmpeg to
// make it again: the 54 videos the sequences added took 100 s, and the
// one of ten minutes 9 s every time, issue #527. ffmpeg makes the same
// bytes from the same arguments, so a copy is the video the walk would
// have had.
//
// On the real disk: ffmpeg writes the videos and the app reads them, in another program too.
func walkVideos(t *testing.T) *made {
	// Without a recipe a video is kept by what it was asked for alone,
	// written in place, so it stays with the bridge that made it.
	dir, recipe := os.Getenv("FRAMEFAIRY_WALK_VIDEOS"), videoRecipe()
	if dir == "" || recipe == "" {
		dir = t.TempDir()
	}
	return &made{fs: afero.NewOsFs(), dir: dir, recipe: recipe}
}

// videoRecipe is the sum of what makes the bridge's videos: this file,
// which holds every function that makes one and nothing else that
// changes often, and the ffmpeg on the PATH, by the line that says its
// version. Empty when either cannot be read, and then every bridge makes
// its own, see walkVideos. CI keys its cache of the videos on this file
// too, see .github/workflows/ci.yml.
//
// On the real disk: the source of the test and the ffmpeg that runs it are files.
func videoRecipe() string {
	source, err := os.ReadFile("walkvideos_test.go")
	if err != nil {
		return ""
	}
	said, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		return ""
	}
	version, _, _ := bytes.Cut(said, []byte("\n"))
	sum := sha256.Sum256(append(append(source, 0), version...))
	return hex.EncodeToString(sum[:6])
}

// copy puts a copy of the video asked for as what at path, made by build
// the first time, so the walk can do with it what it likes.
func (m *made) copy(path, what string, build func(path string) error) error {
	kept, err := m.video(what, filepath.Ext(path), build)
	if err != nil {
		return err
	}
	src, err := m.fs.Open(kept)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := m.fs.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}

// pickedAs is what a video was asked for, the way videos keeps it: every
// value pick makes a video from, and nothing it does not, so two picks
// that make the same video find the same one.
func pickedAs(seconds, rate, switchAt, timing string, hevc bool, height int) string {
	return fmt.Sprintf("seconds=%s rate=%s switch=%s timing=%s hevc=%t height=%d", seconds, rate, switchAt, timing, hevc, height)
}

// makePicked makes the video a pick asks for, see pick: a camera's
// picture with a height, a filmed episode turned on its side, a filmed
// episode with a rate, and else the bridge's own kind.
//
// On the real disk: ffmpeg makes the video.
func makePicked(path string, seconds int, rate string, switchAt float64, timing string, hevc bool, height int) error {
	switch {
	case height > 0:
		return makeCameraEpisode(path, seconds, height, hevc)
	case rate != "" && timing == "turned":
		return makeTurnedEpisode(path, seconds, rate)
	case rate != "":
		return makeFilmedEpisode(path, seconds, rate, timing)
	}
	return makeEpisode(path, seconds, switchAt, hevc)
}

// makeEpisode makes a video of this many seconds. VP9 and Opus rather than
// the path tests' MPEG-4 and AAC, because the Chromium a walk drives is
// built without the proprietary codecs and would not play it at all. The
// picture says which frame it is, the recipe of the harness's own episodes
// in frontend/preview/open.mjs: a strip along its top, 12 pixels high, is
// the frame number in ten bars of 32 pixels, the highest bit on the left,
// light for one and dark for nought, so a walk reads back from the video
// preview's canvas the frame on screen, see recordFrames in
// walks/bridge.mjs. Below it is a grey that grows lighter. The strip is
// thin so that no two frames differ by a camera switch, which a half of
// bars did every 16 frames, and the search then split its clip into
// pieces at every one. The bars are worked out on a picture one pixel a
// bit and made large without smoothing.
//
// With a switch, at a moment on a frame, the episode is two cameras
// instead, see twoCameras. With hevc its picture is HEVC with 10-bit
// colour, which WebKit says it decodes and then fails on, and which the
// Chromium a walk drives cannot decode at all: the Go side decodes it, see
// frames.go. Its GOPs are open, the way x265 writes them and DaVinci
// Resolve wrote Tim's start.mp4: a key frame every twelve frames, the last
// of a run of four, so the three frames shown before it are decoded after
// it. With a key frame every ten, every run of four ended on the frame
// before a key frame, and no frame led up to one. Plan row 2.186.
func makeEpisode(path string, seconds int, switchAt float64, hevc bool) error {
	d := strconv.Itoa(seconds)
	below, sound := "[1]geq=lum='30+180*T/"+d+"':cb=128:cr=128,scale=320:168:flags=neighbor[g]", "220"
	size, rate, heard := "2x2", "100k", []string{"libopus", "-b:a", "24k"}
	if switchAt > 0 {
		// A chequerboard takes more to keep sharp than a grey. The sound
		// is plain samples, chosen when a render read each piece's sound
		// from where the piece starts: Opus needs 80 ms decoded ahead of
		// a seek, so every piece started a little quiet, and a walk
		// listening for a dip heard that. A render now reads the sound a
		// fifth of a second early, soundLead, which
		// TestEveryPieceIsHeardFromItsFirstMoment in engine/preroll_test.go
		// holds, so Opus would do here too. It has not been changed back.
		below, sound, size, rate = twoCameras(int(math.Round(switchAt*5))), "440", "320x168", "300k"
		heard = []string{"pcm_s16le"}
	}
	args := []string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=10x2:r=5:d=" + d,
		"-f", "lavfi", "-i", "color=c=black:s=" + size + ":r=5:d=" + d,
		"-f", "lavfi", "-i", "sine=f=" + sound + ":sample_rate=48000:d=" + d,
		"-filter_complex",
		"[0]geq=lum='if(bitand(N\\,pow(2\\,9-X))\\,220\\,30)':cb=128:cr=128,scale=320:12:flags=neighbor[b];" +
			below + ";[b][g]vstack,format=yuv420p[v]",
		"-map", "[v]", "-map", "2",
		"-shortest"}
	if hevc {
		args = append(args, "-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1",
			"-x265-params", "bframes=3:keyint=12:log-level=error")
	} else {
		args = append(args, "-c:v", "libvpx-vp9", "-b:v", rate, "-deadline", "realtime", "-cpu-used", "8", "-g", "10")
	}
	args = append(args, "-c:a")
	out, err := exec.Command("ffmpeg", append(append(args, heard...), path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// makeCameraEpisode makes a video the size a camera or an export has, 1920
// by 1080 or, with height 2160, 3840 by 2160, at 25 frames a second, with a
// key frame every two seconds and three B-frames between the others, in
// H.264 or with hevc in HEVC with 10-bit colour and open GOPs. Its picture
// moves all over, so every frame costs the decoder what a real one does. It
// is for timing how quickly the video preview follows the keys, see
// walks/speed.mjs, which a picture of 320 by 180 cannot say anything about.
// Its frames carry no number.
func makeCameraEpisode(path string, seconds, height int, hevc bool) error {
	d := strconv.Itoa(seconds)
	size := fmt.Sprintf("%dx%d", height*16/9, height)
	args := []string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=" + size + ":r=25:d=" + d,
		"-f", "lavfi", "-i", "sine=f=220:sample_rate=48000:d=" + d,
		"-map", "0", "-map", "1", "-shortest"}
	if hevc {
		args = append(args, "-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1",
			"-x265-params", "keyint=50:bframes=3:open-gop=1:log-level=error")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-g", "50", "-bf", "3")
	}
	out, err := exec.Command("ffmpeg", append(args, "-c:a", "libopus", "-b:a", "64k", path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// twoCameras is the picture below the strip of an episode filmed by two
// cameras, the first up to the frame it switches on and the second from
// it. Each looks at a subject, a chequerboard, on its own side of the
// picture, the first on the left and the second on the right, so the
// search frames each shot on its own subject and the two crops differ.
// Around the subject is a plain backdrop, dark grey for the first camera
// and a light red for the second, so a short tells the shots apart by
// their colour, and a shot framed by the other's crop shows only its
// backdrop. The switch changes the whole picture at once, which is what
// the search reads as a camera switch.
func twoCameras(frame int) string {
	n := strconv.Itoa(frame)
	checks := "if(mod(floor(X/4)+floor(Y/4)\\,2)\\,235\\,20)"
	return "[1]geq=lum='if(lt(N\\," + n + ")\\," +
		"if(between(X\\,20\\,99)\\," + checks + "\\,60)\\," +
		"if(between(X\\,220\\,299)\\," + checks + "\\,170))'" +
		":cb=128:cr='if(lt(N\\," + n + ")\\,128\\,180)'[g]"
}

// filmedBits is how many bits the frame number of a filmed episode has, so
// it says the number of every frame up to 2048, more than a minute at
// 29.97 fps. Its bands are bandHeight pixels high each, of a picture 180
// high. The walks read them back with the same numbers, see shortFrames in
// walks/bridge.mjs.
const filmedBits, bandHeight = 11, 4

// makeFilmedEpisode makes a video of this many seconds at the frame rate a
// camera records at, rate as ffmpeg takes it, 30000/1001 for 29.97 fps, so
// a walk can render a clip of it and read the short back. Its picture says
// which frame it is in a way the render keeps: a short is a narrow part of
// the picture made larger, so bars side by side would be cut away. The
// frame number is bands one above the other across the whole width, the
// highest bit at the top, each bandHeight pixels high, light for one and
// dark for nought, and whatever part of the width the crop keeps holds
// every band. They take 44 of the picture's 180 pixels, a quarter, and
// light and dark are only 100 apart, so no two frames differ by a camera
// switch, see makeEpisode: bands of 220 and 30 made six of them in 45
// seconds. They are worked out two pixels a bit, since a picture is an even
// number of pixels high. Below them is the grey that grows lighter. The
// sound is a steady tone, so padded silence at a cut can be heard.
//
// The frames come at the rate, unless timing says otherwise: "uneven" is
// a phone's frames, each 0, 8 or 16 ms late in ticks of 1/600 s, and
// "late" is a picture that starts a quarter of a second after the sound,
// between two of its frames, by an edit list. Either way each frame keeps
// its number.
func makeFilmedEpisode(path string, seconds int, rate, timing string) error {
	d := strconv.Itoa(seconds)
	bands := filmedBits * bandHeight
	shift, keep := "", []string{}
	switch timing {
	case "uneven":
		shift = fmt.Sprintf(",settb=1/600,setpts='(N/(%s)+0.008*mod(N\\,3))/TB'", rate)
		keep = []string{"-fps_mode", "passthrough", "-enc_time_base", "1/600", "-video_track_timescale", "600"}
	case "late":
		// On a clock of 1/90000 s, so the picture starts a true quarter
		// second in, 7.49 frames at 29.97 a second. On the rate's own
		// clock the quarter second came out a whole 7 frames, 0.233 s,
		// on the grid of the file's start, where frames counted from the
		// file and from the picture are the same frames and a mistake
		// between the two could not be seen.
		shift = ",settb=1/90000,setpts=PTS+0.25/TB"
		keep = []string{"-fps_mode", "passthrough", "-enc_time_base", "1/90000", "-video_track_timescale", "90000"}
	}
	args := []string{"-loglevel", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=2x%d:r=%s:d=%s", 2*filmedBits, rate, d),
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=2x2:r=%s:d=%s", rate, d),
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=" + d,
		"-filter_complex",
		fmt.Sprintf("[0]geq=lum='if(bitand(N\\,pow(2\\,%d-floor(Y/2)))\\,160\\,60)':cb=128:cr=128,scale=320:%d:flags=neighbor[b];", filmedBits-1, bands) +
			fmt.Sprintf("[1]geq=lum='30+180*T/%s':cb=128:cr=128,scale=320:%d:flags=neighbor[g];", d, 180-bands) +
			"[b][g]vstack,format=yuv420p" + shift + "[v]",
		"-map", "[v]", "-map", "2"}
	args = append(args, keep...)
	out, err := exec.Command("ffmpeg", append(args,
		"-shortest", "-c:v", "libvpx-vp9", "-crf", "30", "-b:v", "0", "-deadline", "realtime", "-cpu-used", "8",
		"-g", "15", "-c:a", "libopus", "-b:a", "64k", path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("making %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// makeTurnedEpisode is a filmed episode the way a phone that recorded it
// upright stores it: the picture lying on its side and a display matrix
// that says to turn it a quarter to the right to stand it up. Standing up,
// the frame number's bands run down its right side, and rotated a quarter
// to the left in the app they run across its top again, where a short of
// it is read. Plan row 3.25.
//
// On the real disk: ffmpeg makes the episode and turns it.
func makeTurnedEpisode(path string, seconds int, rate string) error {
	lying := path + ".lying.mp4"
	defer os.Remove(lying)
	if err := makeFilmedEpisode(lying, seconds, rate, ""); err != nil {
		return err
	}
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-display_rotation", "270",
		"-i", lying, "-c", "copy", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("turning %s: %s %s", filepath.Base(path), err, out)
	}
	return nil
}

// A video asked for again is the one made the first time, without making
// it again, also by another program that keeps its videos in the same
// folder by the same recipe. A video asked for otherwise is made, and a
// change of recipe makes it anew and lets the old go when the folder is
// pruned. One that failed to be made is made when asked for again.
func TestAVideoAskedForAgainIsTheOneMadeBefore(t *testing.T) {
	fsys := afero.NewMemMapFs()
	n := 0
	build := func(path string) error {
		n++
		return afero.WriteFile(fsys, path, []byte{byte(n)}, 0o644)
	}
	read := func(path string) byte {
		t.Helper()
		b, err := afero.ReadFile(fsys, path)
		if err != nil || len(b) != 1 {
			t.Fatalf("%s holds %v, %v", path, b, err)
		}
		return b[0]
	}
	one := &made{fs: fsys, dir: "/kept", recipe: "aaaa"}
	beside := &made{fs: fsys, dir: "/kept", recipe: "aaaa"}
	next := &made{fs: fsys, dir: "/kept", recipe: "bbbb"}
	alone := &made{fs: fsys, dir: "/alone"}
	for _, step := range []struct {
		m      *made
		path   string
		what   string
		video  byte
		builds int
	}{
		{one, "/home/folge-1.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-2.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-3/erste-erinnerung.mp4", "seconds=120", 1, 1},
		{one, "/home/folge-4.mp4", "seconds=600", 2, 2},
		{beside, "/home/folge-5.mp4", "seconds=600", 2, 2},
		{beside, "/home/folge-6.mp4", "seconds=120", 1, 2},
		{next, "/home/folge-7.mp4", "seconds=120", 3, 3},
		{alone, "/home/folge-8.mp4", "seconds=120", 4, 4},
		{alone, "/home/folge-9.mp4", "seconds=120", 4, 4},
	} {
		if err := fsys.MkdirAll(filepath.Dir(step.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := step.m.copy(step.path, step.what, build); err != nil {
			t.Fatalf("%s: %v", step.path, err)
		}
		if got := read(step.path); got != step.video || n != step.builds {
			t.Errorf("%s as %q holds video %d after %d made, want video %d after %d", step.path, step.what, got, n, step.video, step.builds)
		}
	}
	// A copy is the walk's own: what it does to it leaves the one kept.
	if err := afero.WriteFile(fsys, "/home/folge-1.mp4", []byte{99}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := one.copy("/home/folge-10.mp4", "seconds=120", build); err != nil || read("/home/folge-10.mp4") != 1 {
		t.Errorf("a video changed by a walk changed the one kept: %v", err)
	}

	fails := func(string) error { return errors.New("ffmpeg failed") }
	if err := next.copy("/home/folge-11.mp4", "seconds=30", fails); err == nil {
		t.Error("a video that failed to be made came back without the failure")
	}
	if err := next.copy("/home/folge-11.mp4", "seconds=30", build); err != nil || read("/home/folge-11.mp4") != 5 {
		t.Errorf("a video asked for again after it failed was not made anew: %v", err)
	}

	if err := (&made{fs: fsys, dir: "/kept", recipe: "bbbb"}).prune(); err != nil {
		t.Fatal(err)
	}
	entries, err := afero.ReadDir(fsys, "/kept")
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if len(left) != 2 || !strings.HasPrefix(left[0], "bbbb-") || !strings.HasPrefix(left[1], "bbbb-") {
		t.Errorf("after pruning for recipe bbbb the folder holds %v, want the two videos bbbb made", left)
	}
	if err := (&made{fs: fsys, dir: "/nothing", recipe: "bbbb"}).prune(); err != nil {
		t.Errorf("pruning a folder that is not there gives %v", err)
	}
}
