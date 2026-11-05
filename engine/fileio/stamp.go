package fileio

import (
	"fmt"
	"os"
)

// Stamp is how a file is now, for knowing whether it changed since it was
// read: its size, its time and, where the system says, which file it is.
// The time alone ticks every few milliseconds on Linux and every second on
// HFS+, so two edits in one tick that leave a file the same size share
// it. WriteAtomic and Replace put a new file in the old one's place, which
// the system numbers anew, so those two still read as a change.
func Stamp(info os.FileInfo) string {
	return fmt.Sprintf("%d,%d,%d", info.Size(), info.ModTime().UnixNano(), identity(info))
}
