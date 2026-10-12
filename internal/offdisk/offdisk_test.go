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
	"strings"
	"testing"
)

// reason is what a comment says when a test goes to the real disk on
// purpose, on the function or at the top of its file, followed by why.
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
// in a comment that starts with "On the real disk". A new test that writes
// to the disk without one is named here.
//
// On the real disk: what this reads is the module's own source.
func TestTestsStayOffTheDisk(t *testing.T) {
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
		unsaid, err := withoutAReason(rel, src)
		if err != nil {
			return err
		}
		found = append(found, unsaid...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 50 {
		t.Fatalf("read %d test files under %s, too few to be the module", read, root)
	}
	for _, f := range found {
		t.Errorf("%s. Give it an afero.MemMapFs, or say why it cannot have one in a comment that starts with %q", f, reason)
	}
}

// The check finds the tests it is there for and lets the others be.
func TestTheCheckFindsATestOnTheDisk(t *testing.T) {
	src := `// On the real disk: nothing, this only says the top of a file is read.
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
		"x_test.go:11 TestATempDir goes to the real disk with t.TempDir",
		"x_test.go:15 TestAWrite goes to the real disk with os.WriteFile",
		"x_test.go:19 TestTheDisk goes to the real disk with disk",
		"x_test.go:23 TestTheOsFs goes to the real disk with afero.NewOsFs",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// The doc comment of the package is about the package, not a reason,
	// and the top of the file apart from it is a reason for all of it.
	top := strings.Replace(src, "// On the real disk: nothing, this only says the top of a file is read.",
		"// On the real disk: every test here renders with ffmpeg.", 1)
	if got, _ := withoutAReason("x_test.go", []byte(top)); len(got) != len(want) {
		t.Errorf("the package's doc comment counted as a reason for the file: %v", got)
	}
	whole := strings.Replace(src, "// On the real disk: nothing, this only says the top of a file is read.\npackage x",
		"// On the real disk: every test here renders with ffmpeg.\n\npackage x", 1)
	if got, _ := withoutAReason("x_test.go", []byte(whole)); len(got) != 0 {
		t.Errorf("a file that says why at its top still named %v", got)
	}
}

// withoutAReason names every function of a test file that goes to the real
// disk and does not say why. The reason is a comment that starts with
// reason, on the function, inside it, or at the top of the file, apart
// from the package clause, where it covers the whole file.
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
	// The top of the file is what comes before the package clause and is
	// not its doc comment, which is about the package.
	for _, group := range file.Comments {
		if group.End() < file.Package && group != file.Doc && saysWhy(group) {
			return nil, nil
		}
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
