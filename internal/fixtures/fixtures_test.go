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
}

func episode(seconds string) []string {
	return []string{
		"-f", "lavfi", "-i", "testsrc=s=320x180:r=25:d=" + seconds,
		"-f", "lavfi", "-i", "sine=f=220:d=" + seconds,
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac",
	}
}

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
