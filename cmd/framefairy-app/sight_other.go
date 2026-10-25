//go:build !darwin

package main

import "unsafe"

// drawingKeep has nothing to do away from macOS: WebView2 and WebKitGTK
// do not stop drawing a window only because another covers it.
func drawingKeep(unsafe.Pointer) int { return drawingNotDropped }
