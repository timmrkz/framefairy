package naming_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type notSaid struct {
	what string
	re   *regexp.Regexp
}

// What no skill says, and no doc either: who the user is, or an issue or a pull request by its number. A skill speaks of the
// user, who decides, and says what was learned, not where. A link to a
// heading, `](#4-the-tests)`, is not a number, see numbers.
var noSkill = []notSaid{
	{"a person", regexp.MustCompile(`\bTim\b|timmrkz(?:$|[^/]|/[^f]|/f[^r])`)},
	{"an issue or pull request by number", regexp.MustCompile(`(?i)\b(issues?|pull requests?|PRs?)( number)? [0-9]+|\b(pull|issues)/[0-9]+|\bGH-[0-9]+`)},
}

// numbers finds `#12`, and with it what stands before it, so a link to a
// heading, here or in another file, can be told from an issue. Six digits or more are a colour,
// `#942192`, since no issue here has that many.
var numbers = regexp.MustCompile(`(\]\([^)\s#]*)?#[0-9]{1,5}\b`)

// What a skill does not say either, since every skill is written for any
// project: the product's name, the words that only mean something here,
// and the media and tools this kind of app has and most do not. An agent
// in another repository would look for an app, a test or a part that is
// not there. A word this does not know is still the review's to catch.
var notGeneric = []notSaid{
	{"the product", regexp.MustCompile(`(?i)frame[ -]?fairy|My First Memory`)},
	{"a word of this project", regexp.MustCompile(`(?i)\bwalks?\b|range picker|clip timeline|video preview|\bepisodes?\b`)},
	{"a word of this kind of app", regexp.MustCompile(`(?i)ffmpeg|libass|llama|\bwails\b|svelte|\bvideos?\b|\baudio\b|\bcaptions?\b|\bsubtitles?\b|\bcodecs?\b|\bdecoders?\b|podcast|transcri|waveform|\bshorts\b|licen[cs]e keys?`)},
}

// said returns what a line says that it should not.
func said(line string, generic bool) []string {
	var found []string
	rules := noSkill
	if generic {
		rules = append(slices.Clone(noSkill), notGeneric...)
	}
	for _, r := range rules {
		if m := r.re.FindString(line); m != "" {
			found = append(found, r.what+", "+m)
		}
	}
	for _, m := range numbers.FindAllStringSubmatch(line, -1) {
		if m[1] == "" {
			found = append(found, "an issue or pull request by number, "+m[0])
		}
	}
	return found
}

// Every skill in .claude/skills/ is written for any project, so its folder
// can be copied into another repository as it is, see docs/SKILLS.md.
//
// On the real disk: what is read is the repository's own skills.
func TestSkillsAreGeneric(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", ".claude", "skills")
	read := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		read[strings.Split(filepath.ToSlash(rel), "/")[0]] = true
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(text), "\n") {
			for _, s := range said(line, true) {
				t.Errorf(".claude/skills/%s:%d names %s: see docs/SKILLS.md", filepath.ToSlash(rel), n+1, s)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !read["testing"] || !read["interface"] {
		t.Fatalf("read the skills %v without testing or interface, so this test reads the wrong folder", read)
	}
}

// The check above fails on what it should, and on nothing a skill would
// rightly say.
func TestSaidCatchesWhatItShould(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line    string
		generic bool
		bad     bool
	}{
		{"Tim asked for it.", false, true},
		{"assigned to timmrkz", false, true},
		{"see #742", false, true},
		{"(#97)", false, true},
		{"as pull request 12 did", false, true},
		{"Issue 7 says so.", false, true},
		{"what is issue number 678", false, true},
		{"github.com/o/r/pull/980", false, true},
		{"github.com/o/r/issues/5", false, true},
		{"fixed in GH-12", false, true},
		{"Frame Fairy makes shorts.", true, true},
		{"the frame-fairy app", true, true},
		{"the framefairy app", true, true},
		{"the walks run in CI", true, true},
		{"the range picker and the clip timeline", true, true},
		{"the video preview of an episode", true, true},
		{"the walks run in CI", false, false},
		{"the framefairy app", false, false},
		{"The user decides.", true, false},
		{"A timer in a test is a guess.", true, false},
		{"Time the product defines runs on a fake clock.", true, false},
		{"one frame in 60, 5 % slower", true, false},
		{"the first issue opened", true, false},
		{"see [Proof](#4-proof-at-a-fair-price)", true, false},
		{"the accent, `#942192`", false, false},
		{"as [INSTALL.md](INSTALL.md#7-before-the-first-build) says", false, false},
		{"the releases of timmrkz/framefairy", false, false},
		{"github.com/timmrkz/framefairy/issues/603", false, true},
		{"only when its author is timmrkz.", false, true},
		{"it walked the tree", true, false},
		{"an episode made by ffmpeg", true, true},
		{"VP9 video and Opus audio", true, true},
		{"burned-in captions", true, true},
		{"the speech decoder", true, true},
		{"YouTube Shorts", true, true},
		{"a licence key", true, true},
		{"the page's Wails runtime", true, true},
		{"the transcript grows", true, true},
		{"ffmpeg builds in make", false, false},
		{"one animation frame, painted", true, false},
		{"the panel's corners clip the rows", true, false},
		{"short and concrete", true, false},
	}
	for _, c := range cases {
		got := said(c.line, c.generic)
		if (len(got) > 0) != c.bad {
			t.Errorf("%q, generic %v: found %q, want caught %v", c.line, c.generic, got, c.bad)
		}
	}
}

// The docs, CLAUDE.md and the templates for issues and pull requests name
// no person and no issue either: they say the user, and what was learned
// rather than where. docs/GUI-PLAN.md is the history up to 16 October
// 2026 and does not change, see `make lint`.
//
// On the real disk: what is read is the repository's own docs.
func TestDocsNameNoPersonOrNumber(t *testing.T) {
	t.Parallel()
	repo := filepath.Join("..", "..")
	files := []string{"CLAUDE.md", "README.md", ".github/pull_request_template.md"}
	for _, glob := range []string{"docs/*.md", "docs/*/*.md", ".github/ISSUE_TEMPLATE/*.md"} {
		found, err := filepath.Glob(filepath.Join(repo, filepath.FromSlash(glob)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range found {
			rel, err := filepath.Rel(repo, f)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, filepath.ToSlash(rel))
		}
	}
	if !slices.Contains(files, "docs/decisions/2026-11-01-committed-test-fixtures.md") || !slices.Contains(files, "docs/ARCHITECTURE.md") {
		t.Fatalf("read %d files without docs/ARCHITECTURE.md or a decision record, so this test reads the wrong folder", len(files))
	}
	for _, rel := range files {
		if rel == "docs/GUI-PLAN.md" {
			continue
		}
		text, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(text), "\n") {
			for _, s := range said(line, false) {
				t.Errorf("%s:%d names %s: say the user, and what was learned rather than where", rel, n+1, s)
			}
		}
	}
}
