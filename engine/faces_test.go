package engine

import (
	"testing"
)

// Looked at by several workers at once, the frames of a clip give the same
// faces in the same places as one after another, a short frame included,
// which is passed over the way framing passes over it.
func TestFacesLookedForSideBySideAreTheSame(t *testing.T) {
	t.Parallel()
	d, ok := loadFaceDetector(facefinderCascade)
	if !ok {
		t.Fatal("the face detection data could not be read")
	}
	const w, h = FaceWidth, FaceHeight
	var frames [][]byte
	seed := uint32(7)
	for k := range 9 {
		frame := make([]byte, w*h)
		for i := range frame {
			switch k % 3 {
			case 0:
				seed = seed*1664525 + 1013904223
				frame[i] = byte(seed >> 24)
			case 1:
				frame[i] = byte((i%w)*255/w + k)
			default:
				frame[i] = byte(40 + 20*k)
			}
		}
		frames = append(frames, frame)
	}
	frames = append(frames, make([]byte, w))

	one, oneFound := d.faceCentres(frames, w, h, 1)
	many, manyFound := d.faceCentres(frames, w, h, 4)
	for i, frame := range frames {
		want, wantFound := 0.0, false
		if len(frame) >= w*h {
			want, wantFound = d.largestFace(frame, w, h)
		}
		if one[i] != want || oneFound[i] != wantFound || many[i] != want || manyFound[i] != wantFound {
			t.Errorf("frame %d: one worker %v %v, four %v %v, on its own %v %v",
				i, one[i], oneFound[i], many[i], manyFound[i], want, wantFound)
		}
	}
	if got, _ := d.faceCentres(nil, w, h, 4); len(got) != 0 {
		t.Errorf("no frames gave %v", got)
	}
}
