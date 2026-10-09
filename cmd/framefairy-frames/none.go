//go:build !ffmpeglibs

// Command framefairy-frames is the episode's decoder, see main.go. Built
// without ffmpeg's libraries, as go vet and the tests build every package,
// it only says so.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "framefairy-frames was built without ffmpeg's libraries: build it with make")
	os.Exit(1)
}
