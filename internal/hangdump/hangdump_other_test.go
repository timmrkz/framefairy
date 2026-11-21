//go:build !darwin

package hangdump

import "testing"

// hasStack checks nothing off the Mac, where no stack is sampled.
func hasStack(*testing.T, string, int) {}
