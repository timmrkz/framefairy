package main

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"framefairy/internal/framewire"
)

// The layer worked out in the box where the two drawings differ is the
// layer of the whole frame, pixel for pixel, wherever the ink is: none,
// one pixel in a corner, a strip of captions, a faint edge that comes to
// no alpha, in full range and in video range.
func TestTheLayerOfTheBoxIsTheLayerOfTheFrame(t *testing.T) {
	const w, h = 37, 23
	r := rand.New(rand.NewPCG(1, 2))
	cases := map[string]func(black, white []byte){
		"nothing": func(black, white []byte) {},
		"a corner": func(black, white []byte) {
			at := ((h-1)*w + w - 1) * 3
			copy(black[at:], []byte{40, 0, 50})
			copy(white[at:], []byte{80, 40, 90})
		},
		"the first pixel": func(black, white []byte) {
			copy(black, []byte{9, 9, 9})
			copy(white, []byte{200, 200, 200})
		},
		"a faint edge": func(black, white []byte) {
			at := (5*w + 3) * 3
			copy(white[at:], []byte{254, 255, 254})
		},
		"a strip of captions": func(black, white []byte) {
			for y := 12; y < 19; y++ {
				for x := 4; x < 30; x++ {
					a := r.IntN(256)
					at := (y*w + x) * 3
					for k := range 3 {
						c := r.IntN(256) * a / 255
						black[at+k] = byte(c)
						white[at+k] = byte(c + 255 - a)
					}
				}
			}
		},
	}
	for name, draw := range cases {
		for _, video := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s video %v", name, video), func(t *testing.T) {
				black, white := make([]byte, w*h*3), bytes.Repeat([]byte{255}, w*h*3)
				draw(black, white)
				want := framewire.Inked(layerOf(black, white, video), w, h)
				got := inked(black, white, w, h, video)
				if got.X != want.X || got.Y != want.Y || got.W != want.W || got.H != want.H || !bytes.Equal(got.Pixels, want.Pixels) {
					t.Errorf("the box gives %d,%d %dx%d, the whole frame %d,%d %dx%d", got.X, got.Y, got.W, got.H, want.X, want.Y, want.W, want.H)
				}
			})
		}
	}
}

// A layer of a short's size with a line of captions, as the video preview
// asks for one each frame it plays.
func BenchmarkInked(b *testing.B) {
	const w, h = 1080, 1920
	black, white := make([]byte, w*h*3), bytes.Repeat([]byte{255}, w*h*3)
	for y := 1500; y < 1620; y++ {
		for x := 100; x < 980; x++ {
			at := (y*w + x) * 3
			black[at], white[at] = 200, 200
		}
	}
	b.Run("box", func(b *testing.B) {
		for b.Loop() {
			inked(black, white, w, h, true)
		}
	})
	b.Run("whole frame", func(b *testing.B) {
		for b.Loop() {
			framewire.Inked(layerOf(black, white, true), w, h)
		}
	})
}
