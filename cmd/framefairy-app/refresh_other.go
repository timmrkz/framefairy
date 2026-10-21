//go:build !darwin

package main

import "unsafe"

// refreshFree has nothing to do away from macOS: Chromium in WebView2 and
// WebKitGTK update a page as often as the screen refreshes.
func refreshFree(unsafe.Pointer) refreshRaw { return refreshRaw{done: refreshNotHeld} }
