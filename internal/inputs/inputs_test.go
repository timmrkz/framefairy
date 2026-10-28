package inputs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skipped are the folders that hold no code of ours.
var skipped = map[string]bool{".git": true, ".build": true, "node_modules": true, "dist": true, "testdata": true}

// walk calls each with every file of ours under the module's root.
func walk(t *testing.T, each func(path string)) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join("..", ".."), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipped[d.Name()] {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			each(path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Wails can register a global shortcut, which hears a key pressed in any
// app. The app has none, and gains one only by a decision that changes
// this test.
//
// On the real disk: what this reads is the module's own source.
func TestNoGlobalShortcut(t *testing.T) {
	fset := token.NewFileSet()
	walk(t, func(path string) {
		if !strings.HasSuffix(path, ".go") {
			return
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Error(err)
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "GlobalShortcut" {
				t.Errorf("%s registers a global shortcut, which hears keys pressed in other apps", fset.Position(s.Pos()))
			}
			return true
		})
	})
}

// The interface opens no microphone, camera or screen of its own and reads
// no clipboard by itself. Dictate on Report a Problem records through the
// Go side, see cmd/framefairy-app/listen.go, and a paste is the person's
// own, handed to the page by the system.
//
// On the real disk: what this reads is the interface's own source.
func TestTheInterfaceTakesNoInputItIsNotGiven(t *testing.T) {
	calls := []string{"getUserMedia", "getDisplayMedia", "clipboard.read"}
	src := filepath.Join("..", "..", "frontend", "src")
	walk(t, func(path string) {
		if !strings.HasPrefix(path, src+string(filepath.Separator)) {
			return
		}
		switch filepath.Ext(path) {
		case ".ts", ".js", ".svelte":
		default:
			return
		}
		if strings.HasSuffix(path, ".test.ts") {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			return
		}
		for _, c := range calls {
			if strings.Contains(string(data), c) {
				t.Errorf("%s calls %s", path, c)
			}
		}
	})
}
