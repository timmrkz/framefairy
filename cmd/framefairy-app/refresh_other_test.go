//go:build !darwin

package main

import "testing"

// Away from the Mac nothing holds the page.
func TestOnlyTheMacHoldsThePageToSixty(t *testing.T) {
	if got := refreshFree(nil); got.done != refreshNotHeld {
		t.Errorf("refreshFree(nil) did %d, want %d", got.done, refreshNotHeld)
	}
}

// Away from the Mac the page is left as the system draws it.
func TestOnlyTheMacIsToldToKeepACoveredPageDrawn(t *testing.T) {
	if got := drawingKeep(nil); got != drawingNotDropped {
		t.Errorf("drawingKeep(nil) did %d, want %d", got, drawingNotDropped)
	}
}
