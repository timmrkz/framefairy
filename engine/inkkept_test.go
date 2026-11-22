package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/ffmpegtest"
	"framefairy/internal/standin"
)

// inkPath is where a test's machine keeps the ink, on its own disk.
const inkPath = "/home/tim/.framefairy/captions/ink.json"

// On the real disk: the stand-in is a program that is run, and it runs
// the real ffmpeg.
//
// countedFFmpeg is a stand-in for ffmpeg that writes down each run and
// hands it to the real one, and how many runs it has written. A stand-in
// with another mark is another ffmpeg, of other bytes.
func countedFFmpeg(t *testing.T, mark string) (path string, runs func() int) {
	t.Helper()
	real, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	path = filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\n# " + mark + "\necho run >> '" + asked + "'\nexec '" + real + "' \"$@\"\n"
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, func() int {
		data, err := os.ReadFile(asked)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(data), "\n")
	}
}

// A start of the app knows the ink the start before measured: the first
// captions of a clip after a start come without running ffmpeg, the same
// boxes as measured. Each clip's first caption file after a start
// measured every block again, 433 ms against 3.6 ms in the cloud, issue
// #891. Another ffmpeg measures again, and a machine no program set up
// keeps nothing on its disk.
//
// On the real disk: ffmpeg measures the words, through a stand-in that
// counts its runs.
func TestTheInkIsKeptForTheNextStart(t *testing.T) {
	ffmpegtest.Need(t)
	t.Parallel()
	ctx := context.Background()
	ffmpeg, runs := countedFFmpeg(t, "one")
	home := afero.NewMemMapFs()
	// Each start of the app is a machine of its own, on the same disk.
	start := func(ffmpeg string) *Engine {
		m := NewMachine(home)
		m.inkPath = inkPath
		e := NewEngine(runlog.New(io.Discard, false, false), m)
		e.FFmpeg = ffmpeg
		return e
	}
	blocks := []string{"Das ist", "respektvoll"}
	const width, size = 540, 48
	measure := func(e *Engine) []*inkBox {
		t.Helper()
		boxes, ok := e.measureMany(ctx, blocks, e.Fonts.Style(nil), width, size)
		if !ok || len(boxes) != len(blocks) || boxes[0] == nil || boxes[1] == nil {
			t.Fatalf("measured %v, %v", boxes, ok)
		}
		return boxes
	}

	first := measure(start(ffmpeg))
	asked := runs()
	if asked == 0 {
		t.Fatal("the first start ran no ffmpeg")
	}
	second := measure(start(ffmpeg))
	if got := runs() - asked; got != 0 {
		t.Errorf("the second start ran ffmpeg %d times for words measured before", got)
	}
	for i := range blocks {
		if *second[i] != *first[i] {
			t.Errorf("%q is kept as %+v, measured as %+v", blocks[i], *second[i], *first[i])
		}
	}

	other, otherRuns := countedFFmpeg(t, "two")
	measure(start(other))
	if otherRuns() == 0 {
		t.Error("another ffmpeg took the ink the first one measured")
	}

	alone := NewMachine(afero.NewMemMapFs())
	e := NewEngine(runlog.New(io.Discard, false, false), alone)
	e.FFmpeg = ffmpeg
	measure(e)
	if found, _ := afero.Glob(alone.Files, "/home/*/.framefairy/captions/*"); len(found) > 0 {
		t.Errorf("a machine no program set up wrote %v", found)
	}
}

// keptInk is the bytes of an ink file of stamp "s" with these boxes.
func keptInk(t testing.TB, boxes map[string][4]int) []byte {
	t.Helper()
	data, err := json.Marshal(inkFile{Version: inkVersion, Stamp: "s", Boxes: boxes})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The ink file is read only when it was measured with what the program
// measures with, and only when every box is one a measuring writes.
func TestKeptInkIsReadOnlyWhenItFits(t *testing.T) {
	t.Parallel()
	good := map[string][4]int{"Inter\x0048\x000\x00540\x00Das ist": {-80, -40, 80, 6}}
	if boxes, ok := parseKeptInk(keptInk(t, good), "s"); !ok || boxes["Inter\x0048\x000\x00540\x00Das ist"] != (inkBox{-80, -40, 80, 6}) {
		t.Errorf("a good file read as %v, %v", boxes, ok)
	}
	for name, data := range map[string][]byte{
		"another stamp":    keptInk(t, good),
		"another version":  []byte(`{"version":99,"stamp":"s","boxes":{}}`),
		"left of nothing":  keptInk(t, map[string][4]int{"k": {10, 0, -10, 5}}),
		"top under bottom": keptInk(t, map[string][4]int{"k": {0, 9, 5, 1}}),
		"too far":          keptInk(t, map[string][4]int{"k": {0, 0, inkReach + 1, 5}}),
		"no key":           keptInk(t, map[string][4]int{"": {0, 0, 1, 1}}),
		"not a number":     []byte(`{"version":1,"stamp":"s","boxes":{"k":[0,0,1e400,1]}}`),
		"not json":         []byte(`{"version":1,`),
	} {
		stamp := "s"
		if name == "another stamp" {
			stamp = "t"
		}
		if boxes, ok := parseKeptInk(data, stamp); ok {
			t.Errorf("%s was read: %v", name, boxes)
		}
	}
	many := map[string][4]int{}
	for i := range inkMost + 1 {
		many[fmt.Sprintf("k%d", i)] = [4]int{0, 0, 1, 1}
	}
	if _, ok := parseKeptInk(keptInk(t, many), "s"); ok {
		t.Errorf("a file of %d boxes was read, more than the %d kept", len(many), inkMost)
	}
}

// The ink file is bytes from the disk, which anything could have written.
// Whatever it reads is a box a measuring could have written.
func FuzzParseKeptInk(f *testing.F) {
	f.Add(keptInk(f, map[string][4]int{"Inter\x0048\x000\x00540\x00Das ist": {-80, -40, 80, 6}}))
	f.Add([]byte(`{"version":1,"stamp":"s","boxes":{"k":[0,0,40000,1]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		boxes, ok := parseKeptInk(data, "s")
		if !ok {
			return
		}
		if len(boxes) > inkMost {
			t.Fatalf("read %d boxes", len(boxes))
		}
		for key, b := range boxes {
			if key == "" || len(key) > inkKeyMost || b.left > b.right || b.top > b.bottom ||
				max(-b.left, b.right, -b.top, b.bottom) > inkReach {
				t.Fatalf("read a box no measuring writes: %q %+v", key, b)
			}
		}
	})
}
