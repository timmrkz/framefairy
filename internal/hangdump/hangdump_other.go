//go:build !darwin

package hangdump

// stacks writes nothing off the Mac. The stacks come from macOS's sample
// tool, which no other system has.
func stacks([]int, func(format string, a ...any)) {}
