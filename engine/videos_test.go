package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The box's filter and the words the app shows are made from
// VideoExtensions, every ending in its order, so neither can name a file
// the list does not.
func TestVideoPatternAndNames(t *testing.T) {
	for _, c := range []struct {
		list          []string
		pattern, name string
	}{
		{[]string{".a"}, "*.a", "a"},
		{[]string{".a", ".b"}, "*.a;*.b", "a or b"},
		{[]string{".a", ".b", ".c"}, "*.a;*.b;*.c", "a, b or c"},
	} {
		if got := videoPattern(c.list); got != c.pattern {
			t.Errorf("videoPattern(%v) = %q, want %q", c.list, got, c.pattern)
		}
		if got := videoNames(c.list); got != c.name {
			t.Errorf("videoNames(%v) = %q, want %q", c.list, got, c.name)
		}
	}
	if VideoPattern() != videoPattern(VideoExtensions) || VideoNames() != videoNames(VideoExtensions) {
		t.Error("the filter and the words are not made from VideoExtensions")
	}
	for _, ext := range VideoExtensions {
		if !IsVideo("ep" + strings.ToUpper(ext)) {
			t.Errorf("ep%s is no video, want one whatever the case", strings.ToUpper(ext))
		}
	}
}

// Which files are videos is said in one place in the code,
// VideoExtensions, and one place in the docs, the section of docs/APP.md
// named so. The section names exactly the list, and no other doc, code or
// interface text names three of the endings, which is what a second list
// looks like. Two endings side by side are an example, as ep.mp4 and
// ep.mov sharing a work folder is. Tests and the preview's fake of the Go
// side may name them, they check the list rather than say it.
func TestVideosAreNamedOnce(t *testing.T) {
	root := ".."
	app, err := os.ReadFile(filepath.Join(root, "docs", "APP.md"))
	if err != nil {
		t.Fatal(err)
	}
	const head = "### Which files are videos\n"
	start := strings.Index(string(app), head)
	if start < 0 {
		t.Fatalf("docs/APP.md has no section %q", strings.TrimSpace(head))
	}
	section := string(app)[start+len(head):]
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}
	var named []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "| Its name ends in") {
			for _, m := range regexp.MustCompile("`(\\.[a-z0-9]+)`").FindAllStringSubmatch(line, -1) {
				named = append(named, m[1])
			}
		}
	}
	if !slices.Equal(named, VideoExtensions) {
		t.Errorf("Which files are videos in docs/APP.md names %v, VideoExtensions is %v", named, VideoExtensions)
	}

	words := make([]*regexp.Regexp, len(VideoExtensions))
	for i, ext := range VideoExtensions {
		words[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(strings.TrimPrefix(ext, ".")) + `\b`)
	}
	most := min(3, len(VideoExtensions))
	check := func(rel, text string) {
		lines := strings.Split(text, "\n")
		for i := range lines {
			near := strings.Join(lines[i:min(i+3, len(lines))], "\n")
			n := 0
			for _, w := range words {
				if w.MatchString(near) {
					n++
				}
			}
			if n >= most {
				t.Errorf("%s:%d to %d names the video endings itself, say them through VideoExtensions or point to Which files are videos in docs/APP.md", rel, i+1, i+3)
				return
			}
		}
	}

	for _, top := range []string{"README.md", "CLAUDE.md"} {
		b, err := os.ReadFile(filepath.Join(root, top))
		if err != nil {
			t.Fatal(err)
		}
		check(top, string(b))
	}
	check("docs/APP.md", strings.Replace(string(app), section, strings.Repeat("\n", strings.Count(section, "\n")), 1))
	for _, dir := range []string{"docs", "engine", "asr", "train", "cmd", "frontend/src", "scripts"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "dist" || strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			switch {
			case rel == "docs/APP.md", rel == "engine/episode.go", strings.HasSuffix(rel, "_test.go"):
				return nil
			}
			switch filepath.Ext(rel) {
			case ".go", ".ts", ".svelte", ".md", ".mjs", ".js", ".sh", ".css", ".html":
			default:
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			check(rel, string(b))
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}
