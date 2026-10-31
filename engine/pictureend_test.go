package engine

import (
	"bytes"
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
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
// On the real disk: ffmpeg makes the episode and ffprobe reads it.
func TestAVideoEndsWhereItsPictureEnds(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	dir := t.TempDir()
	for _, c := range []struct {
		name           string
		picture, sound string
		cutOff         bool
		frames         int
		// How long the picture is, from its first frame: the video's
		// length less VideoStart. The AAC of Linux's ffmpeg starts a
		// Matroska file 23 ms before the picture, and the Mac's starts it
		// with the picture, so the length itself differs by machine.
		seconds float64
	}{
		// One second of picture at 25 frames a second, two of sound, in
		// MPEG-4 Part 2, which the ffmpeg we ship encodes on every machine.
		{name: "sound_longer.mp4", picture: "1", sound: "2", frames: 25, seconds: 1},
		// Matroska keeps the picture's length in a tag of its own.
		{name: "sound_longer.mkv", picture: "1", sound: "2", frames: 25, seconds: 1},
		{name: "sound_longer.mov", picture: "1", sound: "2", frames: 25, seconds: 1},
		// Written the way a crash leaves it: no length, no tags.
		{name: "sound_longer_cut_off.mkv", picture: "1", sound: "2", cutOff: true, frames: 25, seconds: 1},
		// The picture longer than the sound: the file is as long as the
		// picture, and stays so.
		{name: "picture_longer.mp4", picture: "2", sound: "1", frames: 50, seconds: 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, c.name)
			made := path
			if c.cutOff {
				made = path + ".whole.mkv"
			}
			if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc=size=64x36:rate=25:duration="+c.picture,
				"-f", "lavfi", "-i", "sine=duration="+c.sound,
				"-c:v", "mpeg4", "-c:a", "aac", made).CombinedOutput(); err != nil {
				ffmpegtest.Unusable(t, "ffmpeg cannot make the video: %v %s", err, out)
			}
			if c.cutOff {
				cutOff(t, made, path)
			}
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

// cutOff copies the Matroska file whole to path the way a recording cut
// off by a crash is left: written as it is made, into a pipe, so ffmpeg
// cannot go back to say how long it is, and without the tags that say how
// long each track is. See timesEpisode.
//
// On the real disk: ffmpeg reads the file and writes the copy.
func cutOff(t *testing.T, whole, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var stderr bytes.Buffer
	cmd := exec.Command("ffmpeg", "-loglevel", "error", "-i", whole, "-map_metadata", "-1", "-c", "copy", "-f", "matroska", "-")
	cmd.Stdout, cmd.Stderr = file, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cutting the video off: %s %s", err, stderr.Bytes())
	}
}

// A recording cut off by a crash says nothing of how long it is, and when
// every track says its bit rate, as MPEG-4 Part 2 with PCM sound does,
// ffprobe guesses its length from the size of the file: 8.236 s for a
// recording of 7 with the ffmpeg of the cloud session. The guess was taken
// for its length, so the grid had more than a second of frames that are
// not there and the read through for where it ends did not happen, #270.
//
// On the real disk: ffmpeg makes the episode and ffprobe reads it.
func TestACutOffRecordingIsNotTakenAtItsGuessedLength(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	path := timesEpisode(t, timesCase{name: "crashed_mpeg4_mkv", file: "crashed.mkv", cutOff: true})
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
