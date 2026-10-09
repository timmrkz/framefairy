//go:build ffmpeglibs && darwin

package main

// #cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia
// #include <VideoToolbox/VideoToolbox.h>
import "C"

import "github.com/asticode/go-astiav"

// chipDecodes says whether this Mac's graphics chip decodes pictures of
// this codec, the way VideoToolbox answers it. A codec it does not, AV1
// before the M3 or MPEG-4 on any, is decoded by ffmpeg on the processor
// with threads from the start, rather than offered to the chip, refused,
// and then decoded on one thread.
func chipDecodes(id astiav.CodecID) bool {
	var kind string
	switch id {
	case astiav.CodecIDH264:
		kind = "avc1"
	case astiav.CodecIDHevc:
		kind = "hvc1"
	case astiav.CodecIDAv1:
		kind = "av01"
	case astiav.CodecIDVp9:
		kind = "vp09"
	case astiav.CodecIDProres:
		kind = "apcn"
	case astiav.CodecIDMpeg4:
		kind = "mp4v"
	case astiav.CodecIDMpeg2Video:
		kind = "mp2v"
	default:
		return false
	}
	t := C.CMVideoCodecType(uint32(kind[0])<<24 | uint32(kind[1])<<16 | uint32(kind[2])<<8 | uint32(kind[3]))
	return C.VTIsHardwareDecodeSupported(t) != 0
}
