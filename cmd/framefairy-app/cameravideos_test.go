package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
)

// makeCameraEpisode makes a video the size a camera or an export has, 1920
// by 1080 or, with height 2160, 3840 by 2160, at 25 frames a second, with a
// key frame every two seconds and three B-frames between the others, in
// H.264 or with hevc in HEVC with 10-bit colour and open GOPs. Its picture
// moves all over, so every frame costs the decoder what a real one does. It
// is for timing how quickly the video preview follows the keys, see
// walks/speed.mjs, which a picture of 320 by 180 cannot say anything about.
// Its frames carry no number.
//
// It is the one video a bridge makes rather than copies from
// internal/fixtures: only speed.mjs and smooth.mjs ask for it, run by
// hand, and a minute of 1080p moving all over is too large to commit.
//
// On the real disk: ffmpeg makes the video.
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
