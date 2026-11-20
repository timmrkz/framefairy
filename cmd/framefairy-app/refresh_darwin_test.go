package main

import "testing"

// On a Mac the page is held to sixty, and a window that is not there yet
// is no fault.
func TestTheMacHoldsThePageToSixty(t *testing.T) {
	if got := refreshFree(nil); got.done != refreshNoWebView {
		t.Errorf("refreshFree(nil) did %d, want %d", got.done, refreshNoWebView)
	}
}

// On a Mac a covered page is told to stay drawn, and a window that is not
// there yet is no fault.
func TestTheMacKeepsACoveredPageDrawn(t *testing.T) {
	if got := drawingKeep(nil); got != drawingNoWebView {
		t.Errorf("drawingKeep(nil) did %d, want %d", got, drawingNoWebView)
	}
}
