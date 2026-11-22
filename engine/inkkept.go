package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/spf13/afero"

	"framefairy/engine/fileio"
)

// The ink of the caption blocks is kept between runs in
// ~/.framefairy/captions/ink.json, so the first captions of a clip after
// a start come without ffmpeg. In memory alone it died with the program,
// and each clip's first caption file after a start measured every block
// again, issue #891. See
// docs/decisions/2026-11-22-the-caption-ink-kept-between-runs.md.

// inkVersion is the version of ink.json. A file of another version is
// not read, and the next measuring writes it again.
const inkVersion = 1

// inkMost is how many boxes are kept at most, in memory and in the file.
const inkMost = 4096

// inkKeyMost is how long a key of a kept box may be, in bytes: a block of
// a few lines of words, its face and its size.
const inkKeyMost = 4096

// inkReach is how far from its anchor a box may reach, in pixels: further
// than any frame of measuring, measureFrameMost.
const inkReach = 1 << 15

// InkFile is where a program keeps the ink of the caption blocks, see
// Machine.UseSystem, or nothing when there is no home folder.
func InkFile() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".framefairy", "captions", "ink.json")
}

// inkFile is what ink.json holds: the stamp of what the ink was measured
// with, and each box by its measureKey, as left, top, right and bottom.
type inkFile struct {
	Version int               `json:"version"`
	Stamp   string            `json:"stamp"`
	Boxes   map[string][4]int `json:"boxes"`
}

// inkStamp is what the ink measured by this engine depends on beyond its
// key: the bytes of its ffmpeg, which carries libass and FreeType, and
// those of the faces the program carries. It is worked out once a run,
// see askOnce, and is empty when ffmpeg cannot be read, and then the ink
// is kept in memory only.
func (e *Engine) inkStamp(ctx context.Context) string {
	key, ok := keyFor("ink stamp", e.FFmpeg, "")
	if !ok {
		return ""
	}
	stamp, err := askOnce(ctx, e.Machine, key, func(context.Context) (string, error) {
		return hashOf(key.tool.path)
	})
	if err != nil {
		return ""
	}
	return stamp + bundledFacesStamp()
}

// hashOf is the first 16 bytes of the SHA-256 of the file at path, in hex.
// It reads the real disk, where ffmpeg is.
func hashOf(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)[:16]), nil
}

// bundledFacesStamp is the hash of the faces the program carries, the
// same for the whole run.
var bundledFacesStamp = sync.OnceValue(func() string {
	h := sha256.New()
	_ = fs.WalkDir(fontBundle, FontsFolder, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fontBundle.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(data)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)[:16])
})

// readKept takes the boxes kept in the file into memory, once a run, if
// they were measured with what stamp says. Boxes measured since are kept
// over the file's.
func (c *inkCache) readKept(stamp string) {
	if c.path == "" || stamp == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stamp != "" {
		return
	}
	c.stamp = stamp
	data, err := afero.ReadFile(c.files, c.path)
	if err != nil {
		return
	}
	kept, ok := parseKeptInk(data, stamp)
	if !ok {
		return
	}
	for key, box := range kept {
		if len(c.boxes) >= inkMost {
			break
		}
		if _, there := c.boxes[key]; !there {
			c.boxes[key] = box
		}
	}
}

// parseKeptInk reads the bytes of a kept ink file measured with stamp.
// Anything a measuring could not have written is as if the file were not
// there.
func parseKeptInk(data []byte, stamp string) (map[string]inkBox, bool) {
	var file inkFile
	if decodeJSON(data, &file) != nil || file.Version != inkVersion || file.Stamp != stamp || len(file.Boxes) > inkMost {
		return nil, false
	}
	boxes := make(map[string]inkBox, len(file.Boxes))
	for key, b := range file.Boxes {
		if key == "" || len(key) > inkKeyMost ||
			slices.ContainsFunc(b[:], func(v int) bool { return v < -inkReach || v > inkReach }) ||
			b[0] > b[2] || b[1] > b[3] {
			return nil, false
		}
		boxes[key] = inkBox{b[0], b[1], b[2], b[3]}
	}
	return boxes, true
}

// writeKept writes every box in memory to the file, in one step, when
// they were measured with what the file was read for. Two writes take
// turns, and the last one has every box.
func (c *inkCache) writeKept(stamp string) {
	if c.path == "" || stamp == "" {
		return
	}
	c.writing.Lock()
	defer c.writing.Unlock()
	c.mu.Lock()
	if c.stamp != stamp {
		c.mu.Unlock()
		return
	}
	file := inkFile{Version: inkVersion, Stamp: stamp, Boxes: make(map[string][4]int, len(c.boxes))}
	for key, b := range c.boxes {
		file.Boxes[key] = [4]int{b.left, b.top, b.right, b.bottom}
	}
	c.mu.Unlock()
	body, err := json.Marshal(file)
	if err != nil {
		return
	}
	if c.files.MkdirAll(filepath.Dir(c.path), 0o755) != nil {
		return
	}
	_ = fileio.WriteUnflushed(c.files, c.path, body)
}
