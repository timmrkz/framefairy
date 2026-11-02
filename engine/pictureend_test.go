package engine

import (
	"bytes"
	"context"
	"math"
	"os/exec"
	"strings"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/fixtures"
)

// How long a video is, from what ffprobe says of it, see lengthOf: where
// its picture ends when the sound goes on after it, #274, the file's own
// length otherwise, and the file read through for where its picture ends
// when it does not say how long it is, or when ffprobe guessed, #270.
// Every answer is one ffprobe gives, written out by hand.
func TestTheLengthOfAVideo(t *testing.T) {
	t.Parallel()
	const guessed = "[matroska,webm @ 0x1] Estimating duration from bitrate, this may be inaccurate\n"
	const readTo = 7.0
	for _, c := range []struct {
		name           string
		stream, format map[string]any
		said           string
		want           float64
		read           bool
	}{
		{name: "MP4, sound longer: the picture's length from its start",
			stream: map[string]any{"start_time": "0.000000", "duration": "1.000000"},
			format: map[string]any{"start_time": "0.000000", "duration": "2.000000"}, want: 1},
		{name: "MP4, picture longer: the file's length",
			stream: map[string]any{"start_time": "0.000000", "duration": "2.000000"},
			format: map[string]any{"start_time": "0.000000", "duration": "1.500000"}, want: 1.5},
		{name: "MP4, picture starting late: from the picture's start",
			stream: map[string]any{"start_time": "0.250000", "duration": "1.000000"},
			format: map[string]any{"start_time": "0.000000", "duration": "2.000000"}, want: 1.25},
		{name: "Matroska, sound longer, the file starting 23 ms before the picture",
			stream: map[string]any{"start_time": "0.000000", "tags": map[string]any{"DURATION": "00:00:01.000000000"}},
			format: map[string]any{"start_time": "-0.023000", "duration": "2.023000"}, want: 1.023},
		{name: "Matroska, a tag of an hour and more",
			stream: map[string]any{"start_time": "0.000000", "tags": map[string]any{"DURATION": "01:02:03.500000000"}},
			format: map[string]any{"start_time": "0.000000", "duration": "4000.000000"}, want: 3723.5},
		// ffmpeg writes the tag as where the picture ends, so read as a
		// length from a late start it reaches past the file, and the
		// file's length stands rather than a frame with a picture lost.
		{name: "Matroska, picture starting late: never shorter than the picture",
			stream: map[string]any{"start_time": "0.520000", "tags": map[string]any{"DURATION": "00:00:01.520000000"}},
			format: map[string]any{"start_time": "-0.023000", "duration": "2.023000"}, want: 2.023},
		{name: "Matroska, a tag that is no time: the file's length",
			stream: map[string]any{"start_time": "0.000000", "tags": map[string]any{"DURATION": "one second"}},
			format: map[string]any{"start_time": "0.000000", "duration": "2.000000"}, want: 2},
		{name: "no length said: read through",
			stream: map[string]any{"start_time": "0.000000"},
			format: map[string]any{"start_time": "0.000000", "duration": "N/A"}, want: readTo, read: true},
		{name: "length guessed: read through, the guessed picture not taken",
			stream: map[string]any{"start_time": "0.000000", "duration": "6.500000"},
			format: map[string]any{"start_time": "0.000000", "duration": "8.236000"}, said: guessed, want: readTo, read: true},
		{name: "another warning: the length stands",
			stream: map[string]any{"start_time": "0.000000", "duration": "2.000000"},
			format: map[string]any{"start_time": "0.000000", "duration": "2.000000"},
			said:   "[mov,mp4 @ 0x1] stream 1, timescale not set\n", want: 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			read := false
			got := lengthOf(c.stream, c.format, c.said, func(float64) float64 {
				read = true
				return readTo
			})
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("the video is %.4f s long, not %.4f s", got, c.want)
			}
			if read != c.read {
				t.Errorf("read through: %v, not %v", read, c.read)
			}
		})
	}
}

// A video whose sound goes on after its picture ends where its picture
// does, as ffprobe reads the files ffmpeg makes: its grid holds only
// frames with a picture, and no edge of a clip goes past the last of
// them. Its length was the file's, which runs to the end of the sound, so
// the range picker had frames with no picture and a clip's end had room to
// go there, #274. A recording cut off by a crash is read through for where
// its picture ends, not its sound. A video whose picture goes on after its
// sound keeps the file's length.
//
// On the real disk: ffprobe reads the episode.
func TestAVideoEndsWhereItsPictureEnds(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	for _, c := range []struct {
		name   string
		frames int
		// How long the picture is, from its first frame: the video's
		// length less VideoStart, since the AAC may start a Matroska file
		// before the picture.
		seconds float64
	}{
		// One second of picture at 25 frames a second, two of sound, in
		// MPEG-4 Part 2.
		{name: "sound-longer.mp4", frames: 25, seconds: 1},
		// Matroska keeps the picture's length in a tag of its own.
		{name: "sound-longer.mkv", frames: 25, seconds: 1},
		{name: "sound-longer.mov", frames: 25, seconds: 1},
		// Written the way a crash leaves it: no length, no tags.
		{name: "sound-longer-cut-off.mkv", frames: 25, seconds: 1},
		// The picture longer than the sound: the file is as long as the
		// picture, and stays so.
		{name: "picture-longer.mp4", frames: 50, seconds: 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := fixtures.Path(t, c.name)
			info, err := e.Probe(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Grid().Count; got != c.frames {
				t.Errorf("the grid holds %d frames, not the %d with a picture", got, c.frames)
			}
			if got := info.Duration - info.VideoStart; math.Abs(got-c.seconds) > 0.001 {
				t.Errorf("the picture is %.3f s long from its first frame, not %.3f s", got, c.seconds)
			}
			if got := info.End() - info.VideoStart; math.Abs(got-c.seconds) > 0.001 {
				t.Errorf("a clip ends %.3f s after the first frame at the latest, not %.3f s", got, c.seconds)
			}
		})
	}
}

// A recording cut off by a crash says nothing of how long it is, and when
// every track says its bit rate, as MPEG-4 Part 2 with PCM sound does,
// ffprobe guesses its length from the size of the file: 8.236 s for a
// recording of 7 with the ffmpeg of the cloud session. The guess was taken
// for its length, so the grid had more than a second of frames that are
// not there and the read through for where it ends did not happen, #270.
//
// On the real disk: ffprobe reads the episode.
func TestACutOffRecordingIsNotTakenAtItsGuessedLength(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(disk))
	path := timesEpisode(t, timesCase{name: "crashed_mpeg4_mkv", file: "crashed-mpeg4.mkv"})
	// The file itself says nothing of its length, so ffprobe guesses.
	// A file that says it is not one a crash left.
	said, err := exec.Command("ffprobe", "-v", "verbose", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe: %v %s", err, said)
	}
	if !strings.Contains(string(said), "Estimating duration from bitrate") {
		t.Fatalf("%s says how long it is, so it is no recording cut off by a crash:\n%s", path, said)
	}
	info, err := e.Probe(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Duration < 6.99 || info.Duration > 7.01 {
		t.Errorf("the recording is %.3f s long, not the 7 s it holds", info.Duration)
	}
	if seconds := float64(info.Grid().Count) / info.FPS(); seconds < 7-1/info.FPS() || seconds > 7+1/info.FPS() {
		t.Errorf("the grid holds %.3f s of frames, not the 7 s the recording holds", seconds)
	}
}
