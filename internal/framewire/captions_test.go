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
	sent := Captions{Width: 540, Height: 960, ShortW: 1080, ShortH: 1920, File: file, Fonts: fonts}
	id, got, ok := ReadCaptions(strings.Fields(CaptionsRequest(7, sent)))
	if !ok || id != 7 || got != sent {
		t.Fatalf("read back %d %+v %v", id, got, ok)
	}
	if id, ms, ok := ReadDraw(strings.Fields(DrawRequest(7, 12345))); !ok || id != 7 || ms != 12345 {
		t.Fatalf("read back %d %d %v", id, ms, ok)
	}
	for _, bad := range []string{"captions 1 0 10 10 10 61 62", "captions 1 10 10 10 10 zz 62", "captions 1 10 10 10 10 61",
		"captions 1 99999 10 10 10 61 62", "captions 1 10 10 0 10 61 62", "captions 1 10 10 61 62", "draw 1 -4", "draw 1"} {
		f := strings.Fields(bad)
		if f[0] == "draw" {
			if _, _, ok := ReadDraw(f); ok {
				t.Errorf("%q was taken", bad)
			}
		} else if _, _, ok := ReadCaptions(f); ok {
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

// What comes back from the decoder is read only when its box and its
// pixels agree: a body short of its box, short of its pixels or with
// pixels to spare is refused, whatever the box says.
func TestALayerIsReadOnlyWhole(t *testing.T) {
	box := func(x, y, w, h uint16, pixels int) []byte {
		b := Layer{X: int(x), Y: int(y), W: int(w), H: int(h)}.Head()
		return append(b, make([]byte, pixels)...)
	}
	for _, c := range []struct {
		name string
		body []byte
		ok   bool
	}{
		{"nothing", nil, false},
		{"half a box", []byte{1, 0, 2, 0}, false},
		{"a box of nothing", box(0, 0, 0, 0, 0), true},
		{"a box of nothing with pixels", box(0, 0, 0, 0, 4), false},
		{"a pixel", box(3, 4, 1, 1, 4), true},
		{"a pixel short of a byte", box(3, 4, 1, 1, 3), false},
		{"a pixel and a byte", box(3, 4, 1, 1, 5), false},
		{"the largest box with no pixels", box(65535, 65535, 65535, 65535, 0), false},
		{"2 by 3", box(0, 0, 2, 3, 24), true},
		{"2 by 3 read as 3 by 3", box(0, 0, 3, 3, 24), false},
	} {
		l, err := ReadLayer(c.body)
		if (err == nil) != c.ok {
			t.Errorf("%s: read %+v, %v", c.name, l, err)
		}
		if err == nil && len(l.Pixels) != l.W*l.H*4 {
			t.Errorf("%s: %d by %d with %d bytes", c.name, l.W, l.H, len(l.Pixels))
		}
	}
}

// A draw names its cursor and a moment in milliseconds, and nothing
// else is taken for one.
func TestADrawIsReadOnlyWhole(t *testing.T) {
	for _, c := range []struct {
		line string
		ok   bool
	}{
		{"draw 0 0", true},
		{"draw 4294967295 1099511627776", true},
		{"draw 4294967296 1", false},
		{"draw 1 1099511627777", false},
		{"draw 1 -1", false},
		{"draw x 1", false},
		{"draw 1 1.5", false},
		{"draw 1 2 3", false},
		{"drew 1 2", false},
		{"", false},
	} {
		if _, _, ok := ReadDraw(strings.Fields(c.line)); ok != c.ok {
			t.Errorf("%q was taken: %v", c.line, ok)
		}
	}
}
