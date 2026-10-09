//go:build ffmpeglibs

package main

// #cgo pkg-config: libavcodec libavutil
// #include <libavcodec/avcodec.h>
// #include <libavutil/channel_layout.h>
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

// bestEffort is ffmpeg's best guess at when a frame starts, which the
// ffmpeg program takes as every frame's moment, whether or not the frame
// carries one of its own.
func bestEffort(f *astiav.Frame) int64 {
	return int64((*C.AVFrame)(f.UnsafePointer()).best_effort_timestamp)
}

// defaultLayout is the name of the layout the ffmpeg program's -ac gives so
// many channels, the first ffmpeg knows of that many.
func defaultLayout(channels int) string {
	var l C.AVChannelLayout
	C.av_channel_layout_default(&l, C.int(channels))
	defer C.av_channel_layout_uninit(&l)
	var buf [64]C.char
	if C.av_channel_layout_describe(&l, &buf[0], C.size_t(len(buf))) < 0 {
		return fmt.Sprintf("%dc", channels)
	}
	return C.GoString(&buf[0])
}
