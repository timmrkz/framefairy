package framewire

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
)

// The captions of the video preview are drawn by libass, from the short's
// own caption file, the way the render burns them in, so the video
// preview, the thumbnails and the short show one drawing. A cursor of
// captions holds a caption file open in the subtitle filter at a size,
// and draws it at a moment:
//
//	captions <cursor> <width> <height> <file> <fonts>
//	                                        the cursor holds the caption file
//	                                        open, drawn at width by height,
//	                                        the faces in the folder fonts
//	draw <cursor> <ms>                      the captions at ms milliseconds of
//	                                        the short, as libass reads the
//	                                        frame shown then
//
// The two paths are written in hex, so a space or anything else in a name
// is no word of its own. A captions is answered with Opened or Failed, a
// draw with one Frame record whose body is a Layer, or Failed.

// CaptionsRequest is the request that opens a cursor of captions.
func CaptionsRequest(cursor uint32, width, height int, file, fonts string) string {
	return fmt.Sprintf("captions %d %d %d %s %s", cursor, width, height,
		hex.EncodeToString([]byte(file)), hex.EncodeToString([]byte(fonts)))
}

// ReadCaptions reads a captions request, split into words.
func ReadCaptions(f []string) (cursor uint32, width, height int, file, fonts string, ok bool) {
	if len(f) != 6 || f[0] != "captions" {
		return 0, 0, 0, "", "", false
	}
	id, err1 := strconv.ParseUint(f[1], 10, 32)
	w, err2 := strconv.Atoi(f[2])
	h, err3 := strconv.Atoi(f[3])
	name, err4 := hex.DecodeString(f[4])
	dir, err5 := hex.DecodeString(f[5])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil ||
		len(name) == 0 || len(dir) == 0 || !LayerFits(w, h) {
		return 0, 0, 0, "", "", false
	}
	return uint32(id), w, h, string(name), string(dir), true
}

// LayerFits says whether captions can be drawn at width by height: at
// least a pixel each way, and no more pixels than the largest frame.
func LayerFits(width, height int) bool {
	return width >= 1 && height >= 1 && width <= 7680 && height <= 7680 &&
		width*height*4 <= MaxBody-LayerHead
}

// DrawRequest is the request for the captions at ms milliseconds.
func DrawRequest(cursor uint32, ms int64) string {
	return fmt.Sprintf("draw %d %d", cursor, ms)
}

// ReadDraw reads a draw request, split into words.
func ReadDraw(f []string) (cursor uint32, ms int64, ok bool) {
	if len(f) != 3 || f[0] != "draw" {
		return 0, 0, false
	}
	id, err1 := strconv.ParseUint(f[1], 10, 32)
	ms, err2 := strconv.ParseInt(f[2], 10, 64)
	if err1 != nil || err2 != nil || ms < 0 || ms > 1<<40 {
		return 0, 0, false
	}
	return uint32(id), ms, true
}

// Layer is the captions at a moment: the box of the drawing that has any
// ink in it, and its pixels, red, green, blue and alpha, 4 bytes a pixel,
// row after row, in full range, with the alpha multiplied in: libass's
// drawing blended over a clear picture the way the render blends it over
// the short's, so laid over a frame as colour plus frame times one less
// alpha it is the short's frame. Everything outside the box is clear. A
// moment with no captions is a box of nothing.
type Layer struct {
	X, Y, W, H int
	Pixels     []byte
}

// LayerHead is the size of a layer's box in a record: four 16-bit numbers.
const LayerHead = 8

// Body is the layer as a record's body.
func (l Layer) Body() []byte {
	b := make([]byte, LayerHead+len(l.Pixels))
	for i, v := range []int{l.X, l.Y, l.W, l.H} {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(v))
	}
	copy(b[LayerHead:], l.Pixels)
	return b
}

// ReadLayer reads a layer from a record's body.
func ReadLayer(body []byte) (Layer, error) {
	if len(body) < LayerHead {
		return Layer{}, fmt.Errorf("a layer of %d bytes has no box", len(body))
	}
	var v [4]int
	for i := range v {
		v[i] = int(binary.LittleEndian.Uint16(body[2*i:]))
	}
	l := Layer{X: v[0], Y: v[1], W: v[2], H: v[3], Pixels: body[LayerHead:]}
	if len(l.Pixels) != l.W*l.H*4 {
		return Layer{}, fmt.Errorf("a layer of %d by %d has %d bytes", l.W, l.H, len(l.Pixels))
	}
	return l, nil
}

// Inked is the layer of the part of a drawing of width by height, 4 bytes
// a pixel, that has any ink, its alpha above nought.
func Inked(pixels []byte, width, height int) Layer {
	x0, y0, x1, y1 := width, height, -1, -1
	for y := 0; y < height; y++ {
		row := pixels[y*width*4 : (y+1)*width*4]
		for x := 0; x < width; x++ {
			if row[x*4+3] != 0 {
				x0, x1 = min(x0, x), max(x1, x)
				y0, y1 = min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return Layer{}
	}
	l := Layer{X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1}
	l.Pixels = make([]byte, 0, l.W*l.H*4)
	for y := y0; y <= y1; y++ {
		l.Pixels = append(l.Pixels, pixels[(y*width+x0)*4:(y*width+x1+1)*4]...)
	}
	return l
}
