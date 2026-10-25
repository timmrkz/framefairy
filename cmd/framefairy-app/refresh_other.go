//go:build !darwin

package main

import "unsafe"

// refreshFree has nothing to do away from macOS: Chromium in WebView2 and
// WebKitGTK update a page as often as the screen refreshes.
func refreshFree(unsafe.Pointer) refreshRaw { return refreshRaw{done: refreshNotHeld} }

// drawingKeep does nothing away from macOS. Whether WebView2 and WebKitGTK
// stop drawing a window another covers was not looked at, see #536.
func drawingKeep(unsafe.Pointer) int { return drawingNotDropped }

// webCPU is not asked away from macOS, where nothing keeps a covered app
// drawn.
func webCPU(unsafe.Pointer) cpuUse { return cpuUse{web: -1, gpu: -1} }
