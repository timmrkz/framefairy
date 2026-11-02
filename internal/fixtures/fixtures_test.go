package fixtures

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// update makes the fixtures that are missing or whose recipe changed,
// with go test ./internal/fixtures -update, the way Go's golden files are
// made again. Without it the test only checks that the files are the
// recipes. A file whose recipe did not change is left as it is, so Git
// keeps no second copy of it made by another ffmpeg.
var update = flag.Bool("update", false, "make the fixtures that are missing or whose recipe changed")

// sums is the file in testdata that says which recipe each fixture was
// made by, one line each, the name and the sum of its recipe.
const sums = "recipes.sum"

// recipes are the arguments ffmpeg makes each fixture with, the output
// path last, or written from what ffmpeg sends to pipe:1 where the
// arguments end in it. An argument "@" and a name is the fixture of that
// name, for a file made from another. The files are made once, here, so a recipe may ask
// for any encoder of the ffmpeg that makes them, libx264 and libx265
// among them, which the ffmpeg we ship does not have.
var recipes = map[string][]string{
	// The test picture with its counter and a tone, at the three lengths
	// the engine's tests of a search need: a video short enough to be its
	// own clip, one a window covers, and one longer than a window.
	"episode-15s.mp4": episode("15"),
	"episode-40s.mp4": episode("40"),
	"episode-70s.mp4": episode("70"),

	// The frame number as bars, with a tone that says the moment of the
	// second, made the ways cameras and recorders make their files. The
	// engine's TestARenderShowsTheFrameThatHoldsEachMoment says what each
	// is for.
	"bars-uneven.mov": bars(barsFile{
		video: []string{"settb=1/600", "setpts='(N*1001/30000+0.008*mod(N\\,3))/TB'"},
		out:   []string{"-fps_mode", "passthrough", "-enc_time_base", "1/600", "-video_track_timescale", "600"}}),
	"bars-held.mov": bars(barsFile{
		video: []string{"select='not(between(n\\,40\\,49)+between(n\\,100\\,104))'"},
		out:   []string{"-fps_mode", "passthrough"}}),
	"bars-between.mov": bars(barsFile{seconds: 20,
		video: []string{"select='not(between(n\\,95\\,104))'"},
		out:   []string{"-fps_mode", "passthrough"}}),
	"bars-late.mov": bars(barsFile{input: []string{"-itsoffset", "0.25"}, out: []string{"-fps_mode", "passthrough"}}),
	"bars-late.mkv": bars(barsFile{input: []string{"-itsoffset", "0.25"}, out: []string{"-fps_mode", "passthrough"}}),
	"bars-even.mp4": bars(barsFile{}),
	"bars-even.mkv": bars(barsFile{}),
	"bars-obs.mkv": bars(barsFile{gop: 30,
		video: []string{"select='not(between(n\\,40\\,49)+between(n\\,100\\,104))'"},
		out:   []string{"-fps_mode", "passthrough"}}),
	"bars-ts.ts": bars(barsFile{gop: 60}),
	"bars-held.ts": bars(barsFile{seconds: 20, gop: 250,
		video: []string{"select='not(between(n\\,160\\,169))'"},
		out:   []string{"-fps_mode", "passthrough"}}),
	"bars-ps.mpg":            bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "15", "-c:a", "mp2"}}),
	"bars-bframes.mpg":       bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "15", "-bf", "2", "-c:a", "mp2"}}),
	"bars-dvd.vob":           bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "18", "-bf", "2", "-c:a", "mp2", "-f", "dvd"}}),
	"bars-bframes.ts":        bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "15", "-bf", "2", "-c:a", "mp2"}}),
	"bars-intra.ts":          bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "1", "-c:a", "mp2"}}),
	"bars-intra.mpg":         bars(barsFile{codecs: []string{"-c:v", "mpeg2video", "-q:v", "2", "-g", "1", "-c:a", "mp2"}}),
	"bars-crashed.mkv":       bars(barsFile{gop: 60, cutOff: true}),
	"bars-crashed-mpeg4.mkv": bars(barsFile{cutOff: true}),

	// The test picture alone, for the episode's decoder: 25 frames a
	// second, a key frame every two seconds and two B-frames between the
	// others, which no frame is made from, the way an export has them.
	"picture.mp4": {"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=10",
		"-c:v", "mpeg4", "-q:v", "8", "-g", "50", "-bf", "2", "-an"},
	// Tones whose pitch climbs, in two channels, so a sample out of place
	// is a sample that differs. No noise: AAC carries noise as a band to
	// make noise in, and a decoder makes it up afresh, differently from
	// wherever it started.
	"tones.m4a": {"-f", "lavfi", "-i",
		"aevalsrc=0.4*sin(2*PI*(200+60*t)*t)+0.1*sin(2*PI*(1500+300*t)*t)|0.3*sin(2*PI*330*t):s=48000:d=10",
		"-c:a", "aac", "-b:a", "192k"},
	// The two put together as they are, with the picture 0.3 s after the
	// sound, the way an export from DaVinci Resolve starts, and with both
	// 0.3 s late. Copied, since only a copy keeps them late.
	"picture-sound.mp4": {"-i", "@picture.mp4", "-i", "@tones.m4a", "-map", "0:v", "-map", "1:a", "-c", "copy"},
	"picture-late.mp4": {"-itsoffset", "0.3", "-i", "@picture.mp4", "-i", "@tones.m4a",
		"-map", "0:v", "-map", "1:a", "-c", "copy"},
	"all-late.mp4": {"-itsoffset", "0.3", "-i", "@picture.mp4", "-itsoffset", "0.3", "-i", "@tones.m4a",
		"-map", "0:v", "-map", "1:a", "-c", "copy"},
	// The files people bring, H.264 and HEVC in 10 bits, which a Mac
	// decodes on its graphics chip.
	"h264.mp4": {"-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=6",
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "35", "-g", "50", "-pix_fmt", "yuv420p"},
	"hevc-10bit.mp4": {"-f", "lavfi", "-i", "testsrc2=s=640x360:r=25:d=6",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "keyint=50:log-level=error", "-tag:v", "hvc1"},
	// HEVC with open GOPs, whose frames shown before a key frame are
	// decoded after it, the way start.mp4 has them, a key frame every two
	// seconds.
	"hevc-open-gop.mp4": {"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=8",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "keyint=50:min-keyint=50:bframes=3:scenecut=0:crf=40:open-gop=1:log-level=error",
		"-tag:v", "hvc1", "-an"},
	// The same picture tagged in each of the colours a file may say it is
	// in, so only the tags tell them apart.
	"colour-bt601-tv.mp4":     colour("libx264", "smpte170m", "smpte170m", "smpte170m", "tv"),
	"colour-bt601-pc.mp4":     colour("libx264", "smpte170m", "smpte170m", "smpte170m", "pc"),
	"colour-bt709-tv.mp4":     colour("libx264", "bt709", "bt709", "bt709", "tv"),
	"colour-bt709-pc.mp4":     colour("libx264", "bt709", "bt709", "bt709", "pc"),
	"colour-none.mp4":         colour("libx264", "", "", "", ""),
	"colour-bt709-tv-10.mp4":  colour("libx265", "bt709", "bt709", "bt709", "tv"),
	"colour-bt2020-tv-10.mp4": colour("libx265", "bt2020nc", "bt2020", "bt709", "tv"),
	"colour-bt2020-pc-10.mp4": colour("libx265", "bt2020nc", "bt2020", "bt709", "pc"),
	"colour-hlg-10.mp4":       colour("libx265", "bt2020nc", "bt2020", "arib-std-b67", "tv"),
	"colour-pq-10.mp4":        colour("libx265", "bt2020nc", "bt2020", "smpte2084", "tv"),

	// Black and silent at the rates a short's captions are drawn at.
	"black-25.mp4":    black("25"),
	"black-29.97.mp4": black("30000/1001"),
	"black-50.mp4":    black("50"),
	"black-120.mp4":   black("120"),
	// Lossless, so a frame read back is that frame and no other, in
	// BT.709 the way HD video is.
	"lossless.mkv": {"-f", "lavfi", "-i", "testsrc2=s=320x180:r=50:d=3",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=3",
		"-vf", "format=yuv420p,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv",
		"-shortest", "-c:v", "ffv1", "-c:a", "flac"},
	// One picture 480 wide whose every column says how far from the left
	// it is, in 16 bits, X times 64.
	"ramp.png": {"-f", "lavfi", "-i", "color=black:s=480x270,format=gray16le,geq=lum=X*64", "-frames:v", "1"},
	// A picture a step brighter on every frame and dark again every 30
	// frames, with a steady tone in PCM, at the rates and in the
	// containers a render cuts on whole frames of.
	"steps-23.976.mov": steps("24000/1001"),
	"steps-24.mov":     steps("24"),
	"steps-25.mov":     steps("25"),
	"steps-29.97.mov":  steps("30000/1001"),
	"steps-30.mov":     steps("30"),
	"steps-60.mov":     steps("60"),
	"steps-29.97.mkv":  steps("30000/1001"),
	// Every frame a key frame, as after a camera switch, with a steady
	// tone in Opus and in MP3.
	"keyframes-opus.mp4": keyframes("libopus"),
	"keyframes-opus.mkv": keyframes("libopus"),
	"keyframes-mp3.mp4":  keyframes("libmp3lame"),
	// Shot on two cameras: the first for ten seconds, the second for ten,
	// then the first again.
	"two-cameras.mp4": {"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "smptebars=s=640x360:r=25:d=10",
		"-f", "lavfi", "-i", "testsrc=s=640x360:r=25:d=10",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0",
		"-c:v", "mpeg4", "-q:v", "5", "-g", "25"},
	// The picture a second after the sound, by an edit list, and two
	// channels that differ, a tone that climbs in the left and one that
	// falls in the right, so a sample from another moment or a channel
	// swapped does not match.
	"sound-first.mov": {"-itsoffset", "1", "-f", "lavfi", "-i", "color=c=black:s=320x180:r=30000/1001:d=7",
		"-f", "lavfi", "-i", "aevalsrc=sin(2*PI*(200*t+300*t*t))|0.5*sin(2*PI*(900*t-40*t*t)):s=48000:d=8",
		"-map", "0", "-map", "1", "-fps_mode", "passthrough", "-g", "60",
		"-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac"},
	// A second of tone, at the eighth of full scale ffmpeg plays it, about
	// -21 dB, and then two of silence.
	"tone-then-silence.m4a": {"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:d=2",
		"-filter_complex", "[0:a][1:a]concat=n=2:v=0:a=1", "-c:a", "aac"},
	// A minute whose loudness rises and falls every few seconds, so a
	// frame measured in the wrong place shows.
	"sweep.m4a": {"-f", "lavfi", "-i", "aevalsrc=sin(2*PI*440*t)*(0.5+0.45*sin(2*PI*0.37*t)):s=16000:d=60",
		"-c:a", "aac"},
	// A tone gated on and off five times a second over the test picture.
	// A steady tone measures the same wherever you start.
	"beats.mp4": {"-f", "lavfi", "-i", "testsrc=s=320x180:r=25:d=12",
		"-f", "lavfi", "-i", "aevalsrc='sin(2*PI*440*t)*lt(mod(t,0.4),0.2)':s=44100:d=12",
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac"},
	// Picture and sound of different lengths, 25 frames a second.
	"sound-longer.mp4":         ends("1", "2"),
	"sound-longer.mkv":         ends("1", "2"),
	"sound-longer.mov":         ends("1", "2"),
	"sound-longer-cut-off.mkv": append(ends("1", "2"), "-map_metadata", "-1", "-f", "matroska", "pipe:1"),
	"picture-longer.mp4":       ends("2", "1"),
	// A tone under each of the words of the engine's render timing test,
	// stopping a little before the word ends, so the next word starts out
	// of silence and its start can be heard.
	"render-timing.mp4": {"-f", "lavfi", "-i", "color=c=black:s=640x360:r=25:d=28",
		"-f", "lavfi", "-i", "aevalsrc='0.5*sin(2*PI*440*t)*(" + renderTimingTone + ")':s=48000:d=28",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", "-b:a", "192k"},
	// A video lying down 320 by 180, and the same turned by a display
	// matrix, the way a phone keeps what it records: as MPEG-4 and as
	// HEVC with B-frames, stored out of the order they show in, which is
	// what a phone writes.
	"phone-mpeg4.mp4":     {"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=2", "-c:v", "mpeg4", "-q:v", "3"},
	"phone-mpeg4-90.mp4":  {"-display_rotation", "90", "-i", "@phone-mpeg4.mp4", "-c", "copy"},
	"phone-mpeg4-180.mp4": {"-display_rotation", "180", "-i", "@phone-mpeg4.mp4", "-c", "copy"},
	"phone-mpeg4-270.mp4": {"-display_rotation", "270", "-i", "@phone-mpeg4.mp4", "-c", "copy"},
	"phone-hevc.mp4": {"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=2",
		"-c:v", "libx265", "-x265-params", "bframes=3:log-level=error", "-tag:v", "hvc1"},
	"phone-hevc-90.mp4":  {"-display_rotation", "90", "-i", "@phone-hevc.mp4", "-c", "copy"},
	"phone-hevc-180.mp4": {"-display_rotation", "180", "-i", "@phone-hevc.mp4", "-c", "copy"},
	"phone-hevc-270.mp4": {"-display_rotation", "270", "-i", "@phone-hevc.mp4", "-c", "copy"},
	// Shorts to cut thumbnails from: two seconds of red and two of blue,
	// every frame its own grey, 16 and 8 times its number, and the
	// caption purple in BT.709.
	"short-red-blue.mp4": {"-f", "lavfi", "-i", "color=c=red:s=180x320:r=25:d=2",
		"-f", "lavfi", "-i", "color=c=blue:s=180x320:r=25:d=2",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1[v]", "-map", "[v]", "-c:v", "mpeg4"},
	"short-greys.mp4": {"-f", "lavfi", "-i", "color=c=black:s=64x64:r=25:d=1,format=yuv420p,geq=lum='16+8*N':cb=128:cr=128",
		"-c:v", "mpeg4", "-q:v", "1"},
	"short-purple.mp4": {"-f", "lavfi", "-i", "color=c=0x942192:s=64x64:r=25:d=1",
		"-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p," +
			"setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv",
		"-c:v", "mpeg4", "-q:v", "1"},
}

func episode(seconds string) []string {
	return []string{
		"-f", "lavfi", "-i", "testsrc=s=320x180:r=25:d=" + seconds,
		"-f", "lavfi", "-i", "sine=f=220:d=" + seconds,
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac",
	}
}

// black is the recipe of four seconds of black at rate, 640 by 360, with
// two channels of silence.
func black(rate string) []string {
	return []string{"-f", "lavfi", "-i", "color=c=black:s=640x360:r=" + rate + ":d=4",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=4",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac"}
}

// steps is the recipe of six seconds of a picture 64 by 36 at rate that
// is a step brighter on every frame and dark again every 30, with a
// steady tone in PCM.
func steps(rate string) []string {
	const d = "6"
	return []string{"-f", "lavfi", "-i", "color=c=black:s=64x36:r=" + rate + ":d=" + d + ",geq=lum='40+mod(N*6,180)':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=" + d,
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "pcm_s16le"}
}

// keyframes is the recipe of seven seconds at 29.97 whose every frame is
// a key frame, with a steady tone coded by encoder.
func keyframes(encoder string) []string {
	return []string{"-f", "lavfi", "-i", "color=c=black:s=64x36:r=30000/1001:d=7,geq=lum='40+mod(N*6,180)':cb=128:cr=128",
		"-f", "lavfi", "-i", "sine=f=440:sample_rate=48000:d=7",
		"-shortest", "-c:v", "mpeg4", "-q:v", "2", "-g", "1", "-b:a", "64k", "-c:a", encoder}
}

// ends is the recipe of the test picture, 64 by 36, picture seconds long,
// and a tone sound seconds long.
func ends(picture, sound string) []string {
	return []string{"-f", "lavfi", "-i", "testsrc=size=64x36:rate=25:duration=" + picture,
		"-f", "lavfi", "-i", "sine=duration=" + sound, "-c:v", "mpeg4", "-c:a", "aac"}
}

// renderTimingTone is when the tone of render-timing.mp4 sounds: the
// words of renderTimingWords in the engine's tests, each 0.1 s short of
// its end.
const renderTimingTone = "between(t,1.20,1.50)+between(t,1.60,2.00)+between(t,2.10,2.50)+" +
	"between(t,6.30,6.70)+between(t,6.80,7.30)+between(t,10.10,10.35)+between(t,13.10,13.35)+" +
	"between(t,16.10,16.35)+between(t,19.10,19.35)+between(t,22.10,22.35)+between(t,25.10,25.35)"

// colour is the recipe of three seconds of the test picture and a tone,
// the picture coded by the encoder, H.264 in 8 bits or HEVC in 10, tagged
// with the colour space, primaries, transfer and range given, or with
// none where they are empty. The tags are set on the frames rather than
// asked of the encoder: a newer ffmpeg takes an encoder's tags from its
// frames and drops -color_trc and the like.
func colour(encoder, space, primaries, transfer, rng string) []string {
	args := []string{"-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=3", "-f", "lavfi", "-i", "sine=f=440:r=48000:d=3"}
	if space != "" {
		args = append(args, "-vf", "setparams=colorspace="+space+":color_primaries="+primaries+":color_trc="+transfer+":range="+rng)
	}
	if encoder == "libx265" {
		args = append(args, "-c:v", "libx265", "-preset", "ultrafast", "-crf", "35", "-x265-params", "log-level=error",
			"-pix_fmt", "yuv420p10le", "-tag:v", "hvc1")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-crf", "35", "-pix_fmt", "yuv420p")
	}
	return append(args, "-c:a", "aac", "-shortest")
}

// barsFile is how one file of bars is made.
type barsFile struct {
	// Before the picture's input, and applied to it.
	input []string
	// Filters after the bars, on the picture.
	video []string
	// After the inputs, before the codecs.
	out []string
	// How long it is, seven seconds when nought.
	seconds int
	// H.264 with two B-frames and a key frame every gop frames where it
	// is not nought, with the sound in AAC. MPEG-4 Part 2 in GOPs of 30
	// and PCM where it is.
	gop int
	// The picture's codec and the sound's, from "-c:v", where they are
	// not the ones gop picks.
	codecs []string
	// Written the way a recording cut off by a crash is left: into a
	// pipe, so with no going back to say how long it is or where its key
	// frames are, and without the tags that say how long each track is.
	cutOff bool
}

// bars is the recipe of a file whose frames carry their number as eight
// bars, top to bottom, light for one and dark for nought, and whose sound
// is a tone that grows from a tenth to nine tenths of full every second.
func bars(f barsFile) []string {
	seconds := strconv.Itoa(max(f.seconds, 7))
	picture := "color=c=black:s=2x8:r=30000/1001:d=" + seconds + "," +
		"geq=lum='if(bitand(N\\,pow(2\\,7-Y))\\,220\\,30)':cb=128:cr=128,scale=128:72:flags=neighbor"
	if len(f.video) > 0 {
		picture += "," + strings.Join(f.video, ",")
	}
	args := slices.Concat(f.input, []string{"-f", "lavfi", "-i", picture,
		"-f", "lavfi", "-i", "aevalsrc='(0.1+0.8*mod(t\\,1))*sin(2*PI*440*t)':s=48000:d=" + seconds}, f.out)
	switch {
	case len(f.codecs) > 0:
		args = append(args, f.codecs...)
	case f.gop > 0:
		args = append(args, "-c:v", "libx264", "-bf", "2", "-g", strconv.Itoa(f.gop), "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-c:a", "aac")
	default:
		args = append(args, "-c:v", "mpeg4", "-q:v", "2", "-g", "30", "-c:a", "pcm_s16le")
	}
	args = append(args, "-t", seconds)
	if f.cutOff {
		args = append(args, "-map_metadata", "-1", "-f", "matroska", "pipe:1")
	}
	return args
}

// makeFixture has ffmpeg make the fixture at path by its recipe.
//
// On the real disk: ffmpeg writes the fixture.
func makeFixture(args []string, path string) error {
	made := append([]string{"-loglevel", "error", "-y"}, args...)
	if made[len(made)-1] != "pipe:1" {
		made = append(made, path)
		if out, err := exec.Command("ffmpeg", made...).CombinedOutput(); err != nil {
			return fmt.Errorf("%w %s", err, out)
		}
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var said bytes.Buffer
	cmd := exec.Command("ffmpeg", made...)
	cmd.Stdout, cmd.Stderr = file, &said
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w %s", err, said.Bytes())
	}
	return nil
}

// made names the fixture an argument of a recipe reads, "@" and its
// name, for a file made from another, a copy of it put together otherwise.
func made(arg string) (string, bool) {
	return strings.CutPrefix(arg, "@")
}

// recipeSum is the sum of the recipe of the fixture called name, and of
// the recipes of the fixtures it is made from, so it changes when they do.
func recipeSum(name string) string {
	args := slices.Clone(recipes[name])
	for i, arg := range args {
		if from, ok := made(arg); ok {
			args[i] += " " + recipeSum(from)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// recipeSums is the line of recipes.sum each recipe should have.
func recipeSums() string {
	var lines []string
	for name := range recipes {
		lines = append(lines, name+" "+recipeSum(name))
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}

// madeBy reads recipes.sum: the sum of the recipe each fixture was made
// by.
//
// On the real disk: recipes.sum is committed beside the fixtures.
func madeBy(t *testing.T) map[string]string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(Dir(), sums))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	out := map[string]string{}
	for line := range strings.Lines(string(body)) {
		if name, sum, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			out[name] = sum
		}
	}
	return out
}

// On the real disk: the fixtures are files in testdata, and with -update
// ffmpeg writes them there.
func TestEveryFixtureIsMadeByItsRecipe(t *testing.T) {
	want := map[string]string{}
	for line := range strings.Lines(recipeSums()) {
		name, sum, _ := strings.Cut(strings.TrimSpace(line), " ")
		want[name] = sum
	}
	if *update {
		had := madeBy(t)
		done := map[string]bool{}
		var ensure func(name string)
		ensure = func(name string) {
			if done[name] {
				return
			}
			done[name] = true
			args := slices.Clone(recipes[name])
			for i, arg := range args {
				if from, ok := made(arg); ok {
					ensure(from)
					args[i] = filepath.Join(Dir(), from)
				}
			}
			path := filepath.Join(Dir(), name)
			if _, err := os.Stat(path); err == nil && had[name] == want[name] {
				return
			}
			if err := makeFixture(args, path); err != nil {
				t.Fatalf("making %s: %s", name, err)
			}
			t.Logf("made %s", name)
		}
		for name := range recipes {
			ensure(name)
		}
		if err := os.WriteFile(filepath.Join(Dir(), sums), []byte(recipeSums()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		t.Fatal(err)
	}
	var there, wanted []string
	for _, e := range entries {
		if e.Name() != sums {
			there = append(there, e.Name())
		}
	}
	for name := range recipes {
		wanted = append(wanted, name)
	}
	slices.Sort(there)
	slices.Sort(wanted)
	if !slices.Equal(there, wanted) {
		t.Errorf("testdata holds %v, the recipes make %v. go test ./internal/fixtures -update makes them again", there, wanted)
	}
	had := madeBy(t)
	for _, name := range wanted {
		if had[name] != want[name] {
			t.Errorf("%s was not made by its recipe as it is now. go test ./internal/fixtures -update makes it again", name)
		}
	}
	if len(had) != len(want) {
		t.Errorf("%s names %d fixtures, the recipes make %d", sums, len(had), len(want))
	}
}
