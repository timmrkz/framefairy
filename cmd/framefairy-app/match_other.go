//go:build !darwin

package main

import "unsafe"

// screenMatch changes nothing away from macOS for now: Windows and Linux
// change a screen's rate their own ways, which no one has tried yet.
func screenMatch(unsafe.Pointer, float64) (done int, from, to float64, modes []screenMode) {
	return matchNoScreen, 0, 0, nil
}
