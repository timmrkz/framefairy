package framewire

import (
	"bytes"
	"strings"
	"testing"
)

// A request for captions carries its two names whole, whatever is in
// them, and a layer comes back as it went.
func TestCaptionsRequestsAndLayers(t *testing.T) {
	file, fonts := "/Users/tim/My First Memory: it's [a], b; c\\d.ass", "/fonts dir/ä"
	id, w, h, gotFile, gotFonts, ok := ReadCaptions(strings.Fields(CaptionsRequest(7, 1080, 1920, file, fonts)))
	if !ok || id != 7 || w != 1080 || h != 1920 || gotFile != file || gotFonts != fonts {
		t.Fatalf("read back %d %d %d %q %q %v", id, w, h, gotFile, gotFonts, ok)
	}
	if id, ms, ok := ReadDraw(strings.Fields(DrawRequest(7, 12345))); !ok || id != 7 || ms != 12345 {
		t.Fatalf("read back %d %d %v", id, ms, ok)
	}
	for _, bad := range []string{"captions 1 0 10 61 62", "captions 1 10 10 zz 62", "captions 1 10 10 61",
		"captions 1 99999 10 61 62", "draw 1 -4", "draw 1"} {
		f := strings.Fields(bad)
		if f[0] == "draw" {
			if _, _, ok := ReadDraw(f); ok {
				t.Errorf("%q was taken", bad)
			}
		} else if _, _, _, _, _, ok := ReadCaptions(f); ok {
			t.Errorf("%q was taken", bad)
		}
	}
	// A picture 4 by 3 with ink at two pixels.
	pixels := make([]byte, 4*3*4)
	copy(pixels[(0*4+1)*4:], []byte{1, 2, 3, 4})
	copy(pixels[(2*4+2)*4:], []byte{5, 6, 7, 8})
	l := Inked(pixels, 4, 3)
	if l.X != 1 || l.Y != 0 || l.W != 2 || l.H != 3 {
		t.Fatalf("the ink is at %d,%d %dx%d", l.X, l.Y, l.W, l.H)
	}
	back, err := ReadLayer(l.Body())
	if err != nil || back.X != l.X || back.W != l.W || !bytes.Equal(back.Pixels, l.Pixels) {
		t.Fatalf("read back %+v %v", back, err)
	}
	if empty := Inked(make([]byte, 16), 2, 2); empty.W != 0 || len(empty.Body()) != LayerHead {
		t.Fatalf("no ink is %+v", empty)
	}
	if _, err := ReadLayer([]byte{0, 0, 0, 0, 2, 0, 2, 0, 1}); err == nil {
		t.Error("a layer short of its pixels was read")
	}
}
