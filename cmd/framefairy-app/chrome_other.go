//go:build !darwin

package main

import "unsafe"

// chromeMeasure has nothing to answer away from macOS: no other system
// puts its window buttons inside the page, and the system draws its own
// title bar above it. Zeros leave the stylesheet holding its own numbers.
func chromeMeasure(unsafe.Pointer) chromeRaw { return chromeRaw{} }
