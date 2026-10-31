package engine

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
)

// A video whose sound goes on after its picture ends where its picture
// does: its grid holds only frames with a picture, and no edge of a clip
// goes past the last of them. Its length was the file's, which runs to the
// end of the sound, so the range picker had frames with no picture and a
// clip's end had room to go there, #274. A video whose picture goes on
// after its sound keeps the file's length, and so does one that does not
// say how long its picture is.
//
// On the real disk: ffprobe reads the episode.
func TestAVideoEndsWhereItsPictureEnds(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	dir := t.TempDir()
	for _, c := range []struct {
		name           string
		picture, sound string
		frames         int
		length, slack  float64
	}{
		// One second of picture at 25 frames a second, two of sound, in
		// MPEG-4 Part 2, which the ffmpeg we ship encodes on every machine.
		{name: "sound_longer.mp4", picture: "1", sound: "2", frames: 25, length: 1},
		// Matroska keeps the picture's length in a tag of its own. The
		// AAC of Linux's ffmpeg starts the file 23 ms before the picture,
		// so the picture ends 23 ms after its second there, and the
		// Mac's starts it with the picture.
		{name: "sound_longer.mkv", picture: "1", sound: "2", frames: 25, length: 1.0115, slack: 0.012},
		{name: "sound_longer.mov", picture: "1", sound: "2", frames: 25, length: 1},
		// The picture longer than the sound: the file is as long as the
		// picture, and stays so.
		{name: "picture_longer.mp4", picture: "2", sound: "1", frames: 50, length: 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, c.name)
			if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
				"-f", "lavfi", "-i", "testsrc=size=64x36:rate=25:duration="+c.picture,
				"-f", "lavfi", "-i", "sine=duration="+c.sound,
				"-c:v", "mpeg4", "-c:a", "aac", path).CombinedOutput(); err != nil {
				ffmpegtest.Unusable(t, "ffmpeg cannot make the video: %v %s", err, out)
			}
			info, err := e.Probe(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Grid().Count; got != c.frames {
				t.Errorf("the grid holds %d frames, not the %d with a picture", got, c.frames)
			}
			if diff := info.Duration - c.length; diff > c.slack+1e-6 || diff < -c.slack-1e-6 {
				t.Errorf("the video is %.3f s long, not %.3f s", info.Duration, c.length)
			}
			if got, want := info.End(), info.keptAt(int64(c.frames)+info.firstFrame()); got != want {
				t.Errorf("a clip ends at %.3f s at the latest, not on the frame after the last picture at %.3f s", got, want)
			}
		})
	}
}

// A recording cut off by a crash says nothing of how long it is, and when
// every track says its bit rate, as MPEG-4 Part 2 with PCM sound does,
// ffprobe guesses its length from the size of the file: 8.443 s for a
// recording of 7. The guess was taken for its length, so the grid had a
// second and a half of frames that are not there and the read through
// for where it ends did not happen, #270.
//
// On the real disk: ffprobe reads the episode.
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
