package offdisk

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// reason is what a comment says when a test goes to the real disk on
// purpose, on the function or inside it, followed by why.
const reason = "On the real disk"

// files are the functions of package os that read, write or look at a
// file. A test that calls one goes to the disk.
var files = map[string]bool{
	"Chdir": true, "Chmod": true, "Chtimes": true, "Create": true, "CreateTemp": true,
	"DirFS": true, "Link": true, "Lstat": true, "Mkdir": true, "MkdirAll": true,
	"MkdirTemp": true, "Open": true, "OpenFile": true, "OpenRoot": true, "ReadDir": true,
	"ReadFile": true, "Readlink": true, "Remove": true, "RemoveAll": true, "Rename": true,
	"Stat": true, "Symlink": true, "Truncate": true, "WriteFile": true,
}

// Every test that goes to the real disk says why. A test of our own code
// works in memory, and one that cannot, because ffmpeg or another program
// reads its files, or because the disk itself is what is tested, says so
// in a comment that starts with "On the real disk", on itself. A reason at
// the top of a file is not one: it would cover every test added to the
// file later, the tests of our own code among them. A new test that writes
// to the disk without one is named here.
//
// On the real disk: what this reads is the module's own source.
func TestTestsStayOffTheDisk(t *testing.T) {
	for _, f := range everyTestFile(t, withoutAReason) {
		t.Errorf("%s. Give it an afero.MemMapFs, or say why it cannot have one in a comment that starts with %q", f, reason)
	}
}

// Every program a test writes and runs is written through standin.Write,
// see internal/standin. Written another way, it can be refused as busy
// while another test starts a program, #410. A file written or made with
// a mode that lets it run, or with a mode the check cannot read, is named
// here.
//
// On the real disk: what this reads is the module's own source.
func TestEveryProgramATestWritesIsWrittenThroughStandin(t *testing.T) {
	for _, f := range everyTestFile(t, programsWritten) {
		t.Errorf("%s with a mode that lets it run. Write it with standin.Write, or it can be refused as busy", f)
	}
}

// everyTestFile reads every test file of the module with rule, and gives
// what the rule found in them.
//
// On the real disk: what this reads is the module's own source.
func everyTestFile(t *testing.T, rule func(name string, src []byte) ([]string, error)) []string {
	t.Helper()
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
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		lines, err := rule(rel, src)
		if err != nil {
			return err
		}
		found = append(found, lines...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d test files under %s, too few to be the module", read, root)
	}
	return found
}

// The check finds a program written or made with os, and lets a file that
// is not one be.
func TestTheCheckFindsAProgram(t *testing.T) {
	src := `package x

func TestA(t *testing.T) {
	os.WriteFile(script, body, 0o755)
	os.WriteFile(script, body, 0755)
	os.WriteFile(script, body, mode)
	os.OpenFile(script, os.O_CREATE|os.O_WRONLY, 0o700)
	os.Chmod(script, 0o755)
	os.WriteFile(video, body, 0o644)
	os.WriteFile(video, body, 0600)
	os.OpenFile(video, os.O_CREATE|os.O_WRONLY, 0o644)
	os.Chmod(video, 0o600)
	standin.Write(script, body, 0o755)
}
`
	found, err := programsWritten("x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"x_test.go:4 writes a file with os.WriteFile",
		"x_test.go:5 writes a file with os.WriteFile",
		"x_test.go:6 writes a file with os.WriteFile",
		"x_test.go:7 writes a file with os.OpenFile",
		"x_test.go:8 writes a file with os.Chmod",
	}
	if fmt.Sprint(found) != fmt.Sprint(want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(found, "\n"), strings.Join(want, "\n"))
	}
}

// modeAt is where the mode is among the arguments of each function of os
// that writes or makes a file with one.
var modeAt = map[string]int{"WriteFile": 2, "OpenFile": 2, "Chmod": 1}

// programsWritten is where a file calls a function of os that writes or
// makes a file with a mode that is not a number without a bit that lets
// the file run.
func programsWritten(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		at, ok := modeAt[sel.Sel.Name]
		if pkg, isPkg := sel.X.(*ast.Ident); !ok || !isPkg || pkg.Name != "os" || len(call.Args) <= at {
			return true
		}
		if lit, ok := call.Args[at].(*ast.BasicLit); ok && lit.Kind == token.INT {
			if mode, err := strconv.ParseUint(lit.Value, 0, 32); err == nil && mode&0o111 == 0 {
				return true
			}
		}
		found = append(found, fmt.Sprintf("%s:%d writes a file with os.%s", name,
			fset.Position(call.Pos()).Line, sel.Sel.Name))
		return true
	})
	return found, nil
}

// The check finds the tests it is there for and lets the others be.
func TestTheCheckFindsATestOnTheDisk(t *testing.T) {
	src := `// On the real disk: at the top of the file, which is no reason.

package x

func TestInMemory(t *testing.T) {
	fsys := afero.NewMemMapFs()
	_ = afero.WriteFile(fsys, "/a", nil, 0o644)
	dir, _ := afero.TempDir(fsys, "", "x-")
	_ = dir
}

func TestATempDir(t *testing.T) {
	_ = t.TempDir()
}

func TestAWrite(t *testing.T) {
	_ = os.WriteFile("a", nil, 0o644)
}

func TestTheDisk(t *testing.T) {
	_, _ = LoadClips(disk, "a")
}

func TestTheOsFs(t *testing.T) {
	_ = afero.NewOsFs()
}

func TestAProgram(t *testing.T) {
	_ = standin.Write("ffmpeg", nil, 0o755)
}

// On the real disk, because the lock between two programs is the system's.
func TestSaidAbove(t *testing.T) {
	_ = t.TempDir()
}

func TestSaidInside(t *testing.T) {
	// On the real disk: ffmpeg reads the episode.
	_ = t.TempDir()
}

func TestAFieldNamedDisk(t *testing.T) {
	_ = s.disk
	_ = os.Getenv("HOME")
}
`
	got, err := withoutAReason("x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"x_test.go:12 TestATempDir goes to the real disk with t.TempDir",
		"x_test.go:16 TestAWrite goes to the real disk with os.WriteFile",
		"x_test.go:20 TestTheDisk goes to the real disk with disk",
		"x_test.go:24 TestTheOsFs goes to the real disk with afero.NewOsFs",
		"x_test.go:28 TestAProgram goes to the real disk with standin.Write",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

}

// withoutAReason names every function of a test file that goes to the real
// disk and does not say why. The reason is a comment that starts with
// reason, on the function or inside it.
func withoutAReason(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	said := func(from, to token.Pos) bool {
		for _, group := range file.Comments {
			if group.Pos() >= from && group.End() <= to && saysWhy(group) {
				return true
			}
		}
		return false
	}
	var found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		how := touches(fn.Body)
		if how == "" {
			continue
		}
		if (fn.Doc != nil && saysWhy(fn.Doc)) || said(fn.Body.Pos(), fn.Body.End()) {
			continue
		}
		found = append(found, fmt.Sprintf("%s:%d %s goes to the real disk with %s",
			name, fset.Position(fn.Pos()).Line, fn.Name.Name, how))
	}
	sort.Strings(found)
	return found, nil
}

func saysWhy(group *ast.CommentGroup) bool {
	for _, line := range strings.Split(group.Text(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), reason) {
			return true
		}
	}
	return false
}

// touches says how a function goes to the real disk, the first way it
// does, or nothing when it does not.
func touches(body *ast.BlockStmt) string {
	how := ""
	ast.Inspect(body, func(n ast.Node) bool {
		if how != "" {
			return false
		}
		switch n := n.(type) {
		case *ast.SelectorExpr:
			// A field or a method named disk is not the disk, so only
			// what the selector is taken from is looked at.
			ast.Inspect(n.X, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && id.Name == "disk" && how == "" {
					how = "disk"
				}
				return how == ""
			})
			return false
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, _ := sel.X.(*ast.Ident)
			switch {
			case sel.Sel.Name == "TempDir" && len(n.Args) == 0:
				how = exprName(sel.X) + ".TempDir"
			case pkg != nil && pkg.Name == "os" && files[sel.Sel.Name]:
				how = "os." + sel.Sel.Name
			case pkg != nil && pkg.Name == "afero" && sel.Sel.Name == "NewOsFs":
				how = "afero.NewOsFs"
			case pkg != nil && pkg.Name == "standin" && sel.Sel.Name == "Write":
				how = "standin.Write"
			}
		case *ast.Ident:
			if n.Name == "disk" {
				how = "disk"
			}
		}
		return how == ""
	})
	return how
}

func exprName(e ast.Expr) string {
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return "a test"
}
