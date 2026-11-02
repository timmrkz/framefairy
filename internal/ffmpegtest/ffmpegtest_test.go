package ffmpegtest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// spy is a test that records how it was ended rather than ending.
type spy struct {
	testing.TB
	skipped, failed string
}

func (s *spy) Helper()                           {}
func (s *spy) Skip(args ...any)                  { s.skipped = fmt.Sprint(args...) }
func (s *spy) Fatalf(format string, args ...any) { s.failed = fmt.Sprintf(format, args...) }

// Without CI a test that cannot have its ffmpeg skips with the reason, and
// in CI it fails with it.
func TestUnusableSkipsHereAndFailsInCI(t *testing.T) {
	t.Setenv("CI", "")
	here := &spy{TB: t}
	Unusable(here, "no usable ffmpeg here: %s", "no encoder")
	if here.skipped != "no usable ffmpeg here: no encoder" || here.failed != "" {
		t.Errorf("without CI it skipped %q and failed %q", here.skipped, here.failed)
	}
	t.Setenv("CI", "true")
	ci := &spy{TB: t}
	Unusable(ci, "no usable ffmpeg here: %s", "no encoder")
	if ci.skipped != "" || !strings.HasPrefix(ci.failed, "no usable ffmpeg here: no encoder. ") {
		t.Errorf("in CI it skipped %q and failed %q", ci.skipped, ci.failed)
	}
	// A reason that ends a sentence of its own, as the engine's errors do,
	// is not given a second full stop.
	ended := &spy{TB: t}
	Unusable(ended, "no usable ffmpeg here: %v", "it is not beside the program.")
	if !strings.HasPrefix(ended.failed, "no usable ffmpeg here: it is not beside the program. In CI") {
		t.Errorf("in CI it failed %q", ended.failed)
	}
}

// Need looks on the search path for both programs, and a search path
// without them is a skip here and a failure in CI.
//
// On the real disk: the search path is a folder on the disk.
func TestNeedLooksOnTheSearchPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CI", "")
	here := &spy{TB: t}
	Need(here)
	if here.skipped != "ffmpeg is not installed" || here.failed != "" {
		t.Errorf("without CI it skipped %q and failed %q", here.skipped, here.failed)
	}
	t.Setenv("CI", "1")
	ci := &spy{TB: t}
	Need(ci)
	if ci.skipped != "" || !strings.HasPrefix(ci.failed, "ffmpeg is not installed. ") {
		t.Errorf("in CI it skipped %q and failed %q", ci.skipped, ci.failed)
	}
}

// aboutFFmpeg is what a skip that is about ffmpeg mentions, in its
// condition or in its reason.
var aboutFFmpeg = regexp.MustCompile(`(?i)ffmpeg|ffprobe|Preflight|SubtitleFilter|VideoEncoder`)

// No test skips for want of ffmpeg by itself: a skip whose condition or
// reason is about ffmpeg goes through this package, so CI fails where a
// test would have skipped. A future test that skips on its own is read
// here and named.
//
// On the real disk: what this reads is the module's own source.
func TestNoTestSkipsForFFmpegByItself(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("the top of the module is not at %s: %v", root, err)
	}
	var found []string
	read := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", "bin", ".build", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		read++
		skips, err := ffmpegSkips(path, nil)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, s := range skips {
			found = append(found, rel+":"+s)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d test files under %s, too few to be the module", read, root)
	}
	for _, f := range found {
		t.Errorf("%s skips for want of ffmpeg by itself. Use ffmpegtest.Need or ffmpegtest.Unusable, so CI fails instead", f)
	}
}

// The check finds the skips it is there for, and lets the others be.
func TestTheCheckFindsASkipAboutFFmpeg(t *testing.T) {
	src := `package x

func TestA(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("not here")
	}
	if err := e.Preflight(ctx); err != nil {
		t.Skipf("%v", err)
	}
	if !ok {
		t.Skip("this ffmpeg cannot measure captions")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	if testing.Short() {
		t.SkipNow()
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		ffmpegtest.Unusable(t, "no ffprobe")
	}
}
`
	skips, err := ffmpegSkips("x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"5", "8", "11"}
	if fmt.Sprint(skips) != fmt.Sprint(want) {
		t.Errorf("found skips on lines %v, want %v", skips, want)
	}
}

// ffmpegSkips gives the line of every t.Skip, Skipf or SkipNow in a file
// whose reason, or the condition of the if it stands in, is about ffmpeg.
// The file is src when it is given, and read from path when it is nil.
func ffmpegSkips(path string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	var from any
	if src != nil {
		from = src
	}
	file, err := parser.ParseFile(fset, path, from, 0)
	if err != nil {
		return nil, err
	}
	text := func(n ast.Node) string {
		if n == nil {
			return ""
		}
		var b bytes.Buffer
		_ = printer.Fprint(&b, fset, n)
		return b.String()
	}
	var lines []string
	var ifs []*ast.IfStmt
	var visit func(n ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt:
			ifs = append(ifs, n)
			ast.Inspect(n.Body, visit)
			ifs = ifs[:len(ifs)-1]
			if n.Else != nil {
				ast.Inspect(n.Else, visit)
			}
			return false
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Skip", "Skipf", "SkipNow":
			default:
				return true
			}
			about := text(n)
			if len(ifs) > 0 {
				inner := ifs[len(ifs)-1]
				about += " " + text(inner.Init) + " " + text(inner.Cond)
			}
			if aboutFFmpeg.MatchString(about) {
				lines = append(lines, fmt.Sprint(fset.Position(n.Pos()).Line))
			}
		}
		return true
	}
	ast.Inspect(file, visit)
	return lines, nil
}

// stillMakeTheirOwn are the test files that still make a video or a sound
// with ffmpeg while they run, which the parts of #670 move onto the
// committed fixtures in internal/fixtures one by one. Each part takes its
// files off this list, and the list only shrinks.
var stillMakeTheirOwn = []string{
	"cmd/framefairy-app/bridge_test.go",
	"cmd/framefairy-app/captions_test.go",
	"cmd/framefairy-app/driver_test.go",
	"cmd/framefairy-app/frames_test.go",
	"cmd/framefairy-app/harness_test.go",
	"cmd/framefairy-app/library_test.go",
	"cmd/framefairy-app/recording_test.go",
	"cmd/framefairy-app/walkvideos_test.go",
	"engine/bounce_test.go",
	"engine/captionlayer_test.go",
	"engine/captionlook_test.go",
	"engine/cropmarks_test.go",
	"engine/encoderfails_test.go",
	"engine/framecuts_test.go",
	"engine/framing_test.go",
	"engine/hdrcolour_test.go",
	"engine/latesound_test.go",
	"engine/levels_test.go",
	"engine/pictureend_test.go",
	"engine/preroll_test.go",
	"engine/previewsound_test.go",
	"engine/rendertiming_test.go",
	"engine/rotate_test.go",
	"engine/seam_test.go",
	"engine/thumbnails_test.go",
	"engine/transcript_test.go",
	"internal/outside/run_test.go",
}

// No test makes its own video or sound while it runs, it reads a
// committed fixture from internal/fixtures. A test file that asks ffmpeg
// for lavfi, the sources it makes pictures and tones from, is named,
// unless it is still on stillMakeTheirOwn. One that is on the list and
// makes none any more is named too, so the list is kept short.
//
// On the real disk: what this reads is the module's own source.
func TestNoTestMakesItsOwnFixture(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	makes := map[string]bool{}
	read := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", "bin", ".build", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		// The recipes the fixtures are made by, and this check itself.
		if strings.HasPrefix(rel, "internal/fixtures/") || rel == "internal/ffmpegtest/ffmpegtest_test.go" {
			return nil
		}
		read++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, []byte("lavfi")) {
			makes[rel] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d test files under %s, too few to be the module", read, root)
	}
	listed := map[string]bool{}
	for _, f := range stillMakeTheirOwn {
		listed[f] = true
		if !makes[f] {
			t.Errorf("%s makes no fixture any more. Take it off stillMakeTheirOwn", f)
		}
	}
	for f := range makes {
		if !listed[f] {
			t.Errorf("%s makes a video or a sound with ffmpeg while it runs. Read a committed one with fixtures.Path, see internal/fixtures", f)
		}
	}
}
