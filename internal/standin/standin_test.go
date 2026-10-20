package standin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// A program written while other programs start can be run the moment it
// is written. Written with os.WriteFile, 133 of 3200 were refused as busy
// in a cloud session, and none are here.
//
// On the real disk: a program that is run is a file on the disk.
func TestAProgramWrittenWhileOthersStartRunsAtOnce(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-ins are shell scripts")
	}
	dir := t.TempDir()
	var wg sync.WaitGroup
	refused := make(chan error, 16*50)
	for side := range 16 {
		wg.Go(func() {
			for n := range 50 {
				path := filepath.Join(dir, fmt.Sprintf("program-%d-%d", side, n))
				if err := Write(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					refused <- err
					return
				}
				if err := exec.Command(path).Run(); err != nil {
					refused <- err
				}
			}
		})
	}
	wg.Wait()
	close(refused)
	for err := range refused {
		t.Error(err)
	}
}

// Every program a test writes is written through Write. A file written
// with os.WriteFile and a mode that can be run, or a mode the check cannot
// read, is named here.
//
// On the real disk: what this reads is the module's own source.
func TestEveryProgramATestWritesIsWrittenHere(t *testing.T) {
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
		lines, err := programsWritten(rel, src)
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
	for _, f := range found {
		t.Errorf("%s writes a program with os.WriteFile. Use standin.Write, or it can be refused as busy", f)
	}
}

// The check finds a program written with os.WriteFile, and lets a file
// that is not one be.
func TestTheCheckFindsAProgram(t *testing.T) {
	src := `package x

func TestA(t *testing.T) {
	os.WriteFile(script, body, 0o755)
	os.WriteFile(script, body, 0755)
	os.WriteFile(script, body, mode)
	os.WriteFile(video, body, 0o644)
	os.WriteFile(video, body, 0600)
	standin.Write(script, body, 0o755)
}
`
	found, err := programsWritten("x_test.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x_test.go:4", "x_test.go:5", "x_test.go:6"}
	if !slices.Equal(found, want) {
		t.Errorf("found %v, not %v", found, want)
	}
}

// programsWritten is where a file calls os.WriteFile with a mode that is
// not a number without a bit that lets the file run.
func programsWritten(name string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WriteFile" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" {
			return true
		}
		if lit, ok := call.Args[2].(*ast.BasicLit); ok && lit.Kind == token.INT {
			if mode, err := strconv.ParseUint(lit.Value, 0, 32); err == nil && mode&0o111 == 0 {
				return true
			}
		}
		found = append(found, fmt.Sprintf("%s:%d", name, fset.Position(call.Pos()).Line))
		return true
	})
	return found, nil
}
