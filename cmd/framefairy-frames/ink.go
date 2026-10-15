package main

import (
	"bytes"

	"framefairy/internal/framewire"
)

// inked is the captions drawn on black and on white, both width by height
// in red, green and blue, as the layer of their ink, see framewire.Inked.
// Only the box where the two drawings differ is worked out pixel by
// pixel: where nothing is drawn the black stays black and the white stays
// white, which is an alpha of nought, and a caption is a strip of a
// frame, so most of a frame is rows that are compared whole and passed.
// Each frame the video preview plays asks for its own layer, see
// docs/CAPTIONS.md.
func inked(black, white []byte, width, height int, video bool) framewire.Layer {
	stride := width * 3
	clear := func(y int) bool {
		row := y * stride
		return allOf(black[row:row+stride], 0) && allOf(white[row:row+stride], 255)
	}
	y0, y1 := 0, height
	for y0 < y1 && clear(y0) {
		y0++
	}
	for y1 > y0 && clear(y1-1) {
		y1--
	}
	if y0 == y1 {
		return framewire.Layer{}
	}
	x0, x1 := width, 0
	for y := y0; y < y1; y++ {
		for x := range width {
			at := y*stride + x*3
			if black[at]|black[at+1]|black[at+2] != 0 || white[at]&white[at+1]&white[at+2] != 255 {
				x0, x1 = min(x0, x), max(x1, x+1)
			}
		}
	}
	bw, bh := x1-x0, y1-y0
	b, w := make([]byte, 0, bw*bh*3), make([]byte, 0, bw*bh*3)
	for y := y0; y < y1; y++ {
		from, to := y*stride+x0*3, y*stride+x1*3
		b, w = append(b, black[from:to]...), append(w, white[from:to]...)
	}
	l := framewire.Inked(layerOf(b, w, video), bw, bh)
	if l.W > 0 {
		l.X += x0
		l.Y += y0
	}
	return l
}

// allOf says whether every byte of b is v.
func allOf(b []byte, v byte) bool {
	if len(b) == 0 {
		return true
	}
	return b[0] == v && bytes.Equal(b[1:], b[:len(b)-1])
}

// layerOf is the captions drawn on black and on white as a layer of red,
// green, blue and alpha, with the alpha multiplied in, see
// framewire.Layer. The alpha is what of the white is hidden, the mean of
// the three colours. In video range, see videoRange, a colour c times its
// alpha a is put back in full range as (c − 16·a/255)·255/219, since the
// 16 the range adds is blended by the alpha too.
func layerOf(black, white []byte, video bool) []byte {
	n := len(black) / 3
	out := make([]byte, 4*n)
	for i := range n {
		b, w := black[3*i:3*i+3], white[3*i:3*i+3]
		shown := int(w[0]) - int(b[0]) + int(w[1]) - int(b[1]) + int(w[2]) - int(b[2])
		a := max(0, min(255, 255-(shown+1)/3))
		for k := range 3 {
			v := int(b[k])
			if video {
				v = (255*v - 16*a + 109) / 219
			}
			out[4*i+k] = byte(max(0, min(a, v)))
		}
		out[4*i+3] = byte(a)
	}
	return out
}
