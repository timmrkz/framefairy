//go:build ffmpeglibs

package main

// #cgo pkg-config: libavcodec
// #include <libavcodec/avcodec.h>
import "C"

import (
	"unsafe"

	"github.com/asticode/go-astiav"
)

// flushDecoder drops what a decoder holds from before a seek, so the
// frames after it are decoded from the key frame the seek landed on and not
// from what came before. go-astiav has no call for it.
func flushDecoder(dec *astiav.CodecContext) {
	C.avcodec_flush_buffers((*C.AVCodecContext)(unsafe.Pointer(dec.UnsafePointer())))
}
