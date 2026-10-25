//go:build outside && darwin

package outside

import (
	"testing"
	"time"
)

// A check from outside the app, which builds on a Mac with the outside
// tag only, waits on the clock.
func TestWaitsFromOutside(t *testing.T) {
	time.Sleep(time.Millisecond)
}
