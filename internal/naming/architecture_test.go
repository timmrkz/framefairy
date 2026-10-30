package naming_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs/ARCHITECTURE.md is read before a change, so a path it names that
// is not there sends a session to look for code that moved. Every path in
// backticks that names a folder or a file of the repository has to exist.
// A name with no folder in it, like `api.ts`, and a placeholder, like
// `<video>.framefairy/` or `~/.framefairy/`, are not paths of the
// repository. The diagrams in fenced blocks are read as pictures.
//
// On the real disk: what is read is the repository's own source.
func TestTheArchitectureNamesWhatIsThere(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	doc, err := os.ReadFile(filepath.Join(root, "docs", "ARCHITECTURE.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := regexp.MustCompile("(?s)```.*?```").ReplaceAllString(string(doc), "")
	named := regexp.MustCompile("`([^`<~ ]+/[^`< ]*)`").FindAllStringSubmatch(text, -1)
	if len(named) == 0 {
		t.Fatal("docs/ARCHITECTURE.md names no path, so this test reads it wrong")
	}
	for _, m := range named {
		path := m[1]
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Errorf("docs/ARCHITECTURE.md names %s, which is not in the repository", path)
		}
	}
}

// The drawings in docs/diagrams/ are the bird's eye view a person reads
// first, so a folder they name that has moved sends them to the wrong
// place. Every word of a drawing's text that starts with a folder at the
// top of the repository and has a slash in it has to exist.
//
// On the real disk: what is read is the repository's own source.
func TestTheDrawingsNameWhatIsThere(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	drawings, err := filepath.Glob(filepath.Join(root, "docs", "diagrams", "*.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(drawings) == 0 {
		t.Fatal("docs/diagrams/ has no drawing, so this test reads the wrong folder")
	}
	tops, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	top := map[string]bool{}
	for _, e := range tops {
		top[e.Name()] = e.IsDir()
	}
	texts := regexp.MustCompile(`<text[^>]*>([^<]*)</text>`)
	paths := regexp.MustCompile(`[a-z][a-z0-9_.-]*/[A-Za-z0-9_./-]*`)
	checked := 0
	for _, name := range drawings {
		svg, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range texts.FindAllStringSubmatch(string(svg), -1) {
			for _, path := range paths.FindAllString(m[1], -1) {
				if !top[strings.SplitN(path, "/", 2)[0]] {
					continue
				}
				checked++
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
					t.Errorf("%s names %s, which is not in the repository", filepath.Base(name), path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no drawing names a path, so this test reads them wrong")
	}
}
