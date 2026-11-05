//go:build windows

package fileio

import "os"

// identity is nought on Windows, where the file's number is not in what
// Stat gives. NTFS keeps times to a tenth of a microsecond, so the time
// tells two edits apart there.
func identity(os.FileInfo) uint64 { return 0 }
