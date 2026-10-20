// Package standin is the one way a test writes a program it then runs: a
// stand-in for ffmpeg, ffprobe or llama-server, as a shell script.
//
// A program that is open for writing cannot be run. Linux refuses it with
// "text file busy". The file is open only while it is written, but a fork
// made by another test in that moment copies the open file into the child,
// where it stays until the child has started its own program. The tests
// run side by side and start programs all the time, so now and then a
// stand-in was refused the first time it was run. That is how
// TestTheSubtitleSyntaxIsFoundOnceForEveryEngine saw its ffmpeg asked once
// and not twice, #410: the question for its filters was refused before the
// script ran.
//
// Write holds syscall.ForkLock for reading while the file is open. Every
// fork holds it for writing until its child has started its program, so
// no child can hold the file, and the program can be run the moment Write
// returns. TestEveryProgramATestWritesIsWrittenHere holds the other tests
// to it.
package standin

import (
	"io/fs"
	"os"
	"syscall"
)

// Write writes a program the way os.WriteFile writes a file.
func Write(path string, body []byte, mode fs.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(path, body, mode)
}
