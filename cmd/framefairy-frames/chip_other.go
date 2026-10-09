//go:build ffmpeglibs && !darwin

package main

import "github.com/asticode/go-astiav"

// chipDecodes is never true off the Mac: only VideoToolbox is used.
func chipDecodes(astiav.CodecID) bool { return false }
