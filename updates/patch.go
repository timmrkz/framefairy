package updates

import (
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A file that changed between two builds changes by a few bytes, not as a
// whole: a change to the code moves every address after it, and the rest
// of the program stays. So beside a file's piece a build publishes patches
// of it, each against the same file of a build the channel list held when
// it was published, and the parts name each patch by the SHA-256 of the
// file it starts from. An app that runs one of those builds downloads the
// patch, tens or hundreds of kilobytes, where the piece is megabytes.
//
// A patch is in bsdiff's own format, BSDIFF40, which the release tool
// makes with github.com/gabstv/go-bsdiff. The app reads it with the code
// below and Go's compress/bzip2, so the bytes it downloads reach no code
// but ours and the standard library's. A patch is untrusted like a piece:
// what it makes is checked against the file's SHA-256 in the parts, and a
// patch that cannot make the file costs the piece, never the update. See
// docs/decisions/2026-11-28-an-update-downloads-the-bytes-that-changed.md.

// Patch is a patch of a file of the build, from the file with the SHA-256
// From. Size is what it weighs, less than the file's piece.
type Patch struct {
	From string `json:"from"`
	Size int64  `json:"size"`
}

// MaxPatches is the most patches a file may have, one for each build the
// channel list held, with room to spare.
const MaxPatches = 32

// PatchName is the name the patch from the file with the SHA-256 from to
// the file with the SHA-256 to is published under.
func PatchName(from, to string) string { return "patch-" + from + "-" + to + ".bsdiff" }

// checkPatches says whether a part's patches can be patches of its file.
func (part Part) checkPatches() error {
	if len(part.Patches) > MaxPatches {
		return fmt.Errorf("it has %d patches", len(part.Patches))
	}
	seen := map[string]bool{}
	for _, pt := range part.Patches {
		if !sha256Pattern.MatchString(pt.From) || pt.From == part.SHA256 || seen[pt.From] {
			return errors.New("a patch starts from no other file")
		}
		seen[pt.From] = true
		if pt.Size <= patchHeader || pt.Size >= part.Packed {
			return errors.New("a patch weighs what no patch worth having does")
		}
	}
	return nil
}

// patchFrom is the part's patch from the file with this SHA-256.
func (part Part) patchFrom(sum string) (Patch, bool) {
	for _, pt := range part.Patches {
		if pt.From == sum {
			return pt, true
		}
	}
	return Patch{}, false
}

// patchHeader is the size of a BSDIFF40 header: the magic, the sizes of
// the control and diff blocks, and the size of the file it makes.
const patchHeader = 32

// ApplyPatch makes the file of size bytes that patch, in bsdiff's format,
// makes from old. It refuses a patch that says it makes another size, or
// that would read or write outside the blocks and the file it names. What
// it makes still has to be checked against the file's SHA-256.
func ApplyPatch(old, patch []byte, size int64) ([]byte, error) {
	if len(patch) < patchHeader || !bytes.Equal(patch[:8], []byte("BSDIFF40")) {
		return nil, errors.New("the patch is no bsdiff patch")
	}
	ctrlLen, diffLen, newSize := offtin(patch[8:16]), offtin(patch[16:24]), offtin(patch[24:32])
	rest := int64(len(patch) - patchHeader)
	if ctrlLen < 0 || diffLen < 0 || ctrlLen > rest || diffLen > rest-ctrlLen {
		return nil, errors.New("the patch's blocks are not in it")
	}
	if newSize != size || size < 0 || size > MaxBuild {
		return nil, fmt.Errorf("the patch makes %d bytes, the file is %d", newSize, size)
	}
	at := int64(patchHeader)
	ctrl := bzip2.NewReader(bytes.NewReader(patch[at : at+ctrlLen]))
	diff := bzip2.NewReader(bytes.NewReader(patch[at+ctrlLen : at+ctrlLen+diffLen]))
	extra := bzip2.NewReader(bytes.NewReader(patch[at+ctrlLen+diffLen:]))

	out := make([]byte, size)
	var newPos, oldPos int64
	oldSize := int64(len(old))
	var triple [24]byte
	// Each step of a patch made by bsdiff writes something, so a patch
	// with more steps than bytes to write is one that would only spin.
	for steps := int64(0); newPos < size; steps++ {
		if steps > size {
			return nil, errors.New("the patch takes more steps than the file has bytes")
		}
		if _, err := io.ReadFull(ctrl, triple[:]); err != nil {
			return nil, fmt.Errorf("the patch's control block ends early: %w", err)
		}
		add, copied, seek := offtin(triple[0:8]), offtin(triple[8:16]), offtin(triple[16:24])
		if add < 0 || copied < 0 || add > size-newPos {
			return nil, errors.New("the patch writes outside the file")
		}
		if _, err := io.ReadFull(diff, out[newPos:newPos+add]); err != nil {
			return nil, fmt.Errorf("the patch's diff block ends early: %w", err)
		}
		// The diff block holds the difference to the old bytes, which are
		// added where the old file has them, as bsdiff does.
		for i := int64(0); i < add; i++ {
			if o := oldPos + i; o >= 0 && o < oldSize {
				out[newPos+i] += old[o]
			}
		}
		newPos += add
		oldPos += add
		if copied > size-newPos {
			return nil, errors.New("the patch writes outside the file")
		}
		if _, err := io.ReadFull(extra, out[newPos:newPos+copied]); err != nil {
			return nil, fmt.Errorf("the patch's extra block ends early: %w", err)
		}
		newPos += copied
		oldPos += seek
		if oldPos < -MaxBuild || oldPos > MaxBuild {
			return nil, errors.New("the patch reads far outside the old file")
		}
	}
	return out, nil
}

// offtin reads a number the way bsdiff writes it: eight bytes, little
// endian, the top bit the sign.
func offtin(b []byte) int64 {
	v := int64(binary.LittleEndian.Uint64(b) &^ (1 << 63))
	if b[7]&0x80 != 0 {
		return -v
	}
	return v
}
