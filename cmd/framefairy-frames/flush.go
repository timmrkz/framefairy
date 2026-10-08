//go:build ffmpeglibs

package main

// #cgo pkg-config: libavcodec
// #include <libavcodec/avcodec.h>
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/asticode/go-astiav"
)

// flushDecoder drops what a decoder holds from before a seek, so the
// frames after it are decoded from the key frame the seek landed on and not
// from what came before. go-astiav has no call for it.
func flushDecoder(dec *astiav.CodecContext) {
	C.avcodec_flush_buffers((*C.AVCodecContext)(unsafe.Pointer(dec.UnsafePointer())))
}

// copyFrameProps gives dst what src says about itself, its moment and its
// colour, and not its picture. go-astiav has no call for it either.
func copyFrameProps(dst, src *astiav.Frame) error {
	if ret := C.av_frame_copy_props((*C.AVFrame)(dst.UnsafePointer()), (*C.AVFrame)(src.UnsafePointer())); ret < 0 {
		return fmt.Errorf("a frame's details could not be copied: %d", int(ret))
	}
	return nil
}
