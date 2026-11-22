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

// The skills that are about this repository, its interface and its words.
// Every other skill in .claude/skills/ is written for any project, so it
// can be copied into another repository as it is, see docs/SKILLS.md.
var ownSkills = []string{"interface"}

type notSaid struct {
	what string
	re   *regexp.Regexp
}

// What no skill says, not even one about this repository: who the user
// is, or an issue or a pull request by its number. A skill speaks of the
// user, who decides, and says what was learned, not where. A link to a
// heading, `](#4-the-tests)`, is not a number, see numbers.
var noSkill = []notSaid{
	{"a person", regexp.MustCompile(`\bTim\b|timmrkz`)},
	{"an issue or pull request by number", regexp.MustCompile(`(?i)\b(issues?|pull requests?|PRs?)( number)? [0-9]+|\b(pull|issues)/[0-9]+|\bGH-[0-9]+`)},
}

// numbers finds `#12`, and with it what stands before it, so a link to a
// heading can be told from an issue.
var numbers = regexp.MustCompile(`(\]\()?#[0-9]+\b`)

// What a skill for any project does not say either: the product's name
// and the words that only mean something here. An agent in another
// repository would look for an app, a test or a part that is not there.
var notGeneric = []notSaid{
	{"the product", regexp.MustCompile(`(?i)frame[ -]?fairy|My First Memory`)},
	{"a word of this project", regexp.MustCompile(`(?i)\bwalks?\b|range picker|clip timeline|video preview|\bepisodes?\b`)},
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

// On the real disk: what is read is the repository's own skills.
func TestSkillsNameNoPersonOrNumberAndGenericOnesNoProduct(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", ".claude", "skills")
	read := map[bool]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		skill := strings.Split(filepath.ToSlash(rel), "/")[0]
		generic := !slices.Contains(ownSkills, skill)
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		read[generic]++
		for n, line := range strings.Split(string(text), "\n") {
			for _, s := range said(line, generic) {
				t.Errorf(".claude/skills/%s:%d names %s: see docs/SKILLS.md", filepath.ToSlash(rel), n+1, s)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read[true] < 2 || read[false] < 1 {
		t.Fatalf("read %d files of generic skills and %d of this repository's own, so this test reads the wrong folder", read[true], read[false])
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
		{"it walked the tree", true, false},
	}
	for _, c := range cases {
		got := said(c.line, c.generic)
		if (len(got) > 0) != c.bad {
			t.Errorf("%q, generic %v: found %q, want caught %v", c.line, c.generic, got, c.bad)
		}
	}
}
