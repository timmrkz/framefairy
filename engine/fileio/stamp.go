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

// Grew says whether a file is now the file it was then, at most with more
// at its end: as long at least, and the same file where the system says
// which file it is. A file put in the old one's place, by WriteAtomic,
// Replace or a sync service bringing back an older copy, is another file
// however long it is. Where the system does not say, a caller that must
// know checks the bytes it read before.
func Grew(then, now os.FileInfo) bool {
	return now.Size() >= then.Size() && identity(now) == identity(then)
}
