//go:build ffmpeglibs && !darwin

package main

import "github.com/asticode/go-astiav"

// chipDecodes is never true off the Mac: only VideoToolbox is used.
func chipDecodes(astiav.CodecID) bool { return false }

// chipDevice is nil off the Mac: the pictures are decoded on the
// processor.
func chipDevice() *astiav.HardwareDeviceContext { return nil }
