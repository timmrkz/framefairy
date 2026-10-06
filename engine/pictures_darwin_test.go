//go:build darwin

package engine

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
)

// Decodes an H.264 picture with VideoToolbox, scaled to half its size:
// every frame comes, the size asked for, and they differ as the picture
// moves. Skips where VideoToolbox has no encoder or decoder to offer, as
// on a machine without one.
func TestPicturesDecodeWithVideoToolbox(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	stream, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=10:d=2",
		"-c:v", "h264_videotoolbox", "-bf", "0", "-g", "10", "-f", "h264", "-").Output()
	if err != nil || len(stream) == 0 {
		t.Skipf("VideoToolbox could not encode H.264 here: %v", err)
	}
	config, samples := annexB(stream)
	if config == nil || len(samples) == 0 {
		t.Fatalf("no parameter sets or no frames in %d bytes", len(stream))
	}
	p, err := OpenPictures("avc1", config, 320, 180, 160, 90)
	if errors.Is(err, ErrNoPictureDecoder) {
		t.Skipf("no decoder here: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var frames [][]byte
	for i, s := range samples {
		f, err := p.Decode(s, int64(i)*100000, true)
		if err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
		if f != nil {
			frames = append(frames, f)
		}
	}
	if len(frames) != len(samples) {
		t.Fatalf("%d frames of %d samples", len(frames), len(samples))
	}
	for i, f := range frames {
		if len(f) != 160*90*3/2 {
			t.Fatalf("frame %d is %d bytes, not %d", i, len(f), 160*90*3/2)
		}
	}
	if bytes.Equal(frames[0], frames[len(frames)-1]) {
		t.Error("the first frame and the last are the same, and the picture moves")
	}
}

// annexB splits an H.264 stream into its avcC box and its frames, each
// with the four-byte lengths a file holds them with: a frame a slice,
// which is how VideoToolbox writes a picture this small.
func annexB(stream []byte) ([]byte, [][]byte) {
	var units [][]byte
	start := -1
	for i := 0; i+3 <= len(stream); i++ {
		if stream[i] == 0 && stream[i+1] == 0 && stream[i+2] == 1 {
			if start >= 0 {
				end := i
				if end > start && stream[end-1] == 0 {
					end--
				}
				units = append(units, stream[start:end])
			}
			start = i + 3
			i += 2
		}
	}
	if start >= 0 && start < len(stream) {
		units = append(units, stream[start:])
	}
	var sps, pps []byte
	var samples [][]byte
	for _, u := range units {
		if len(u) == 0 {
			continue
		}
		switch u[0] & 0x1f {
		case 7:
			sps = u
		case 8:
			pps = u
		case 1, 5:
			s := []byte{byte(len(u) >> 24), byte(len(u) >> 16), byte(len(u) >> 8), byte(len(u))}
			samples = append(samples, append(s, u...))
		}
	}
	if len(sps) < 4 || pps == nil {
		return nil, samples
	}
	config := []byte{1, sps[1], sps[2], sps[3], 0xff, 0xe1, byte(len(sps) >> 8), byte(len(sps))}
	config = append(config, sps...)
	config = append(config, 1, byte(len(pps)>>8), byte(len(pps)))
	config = append(config, pps...)
	return config, samples
}
