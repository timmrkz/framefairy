package naming_test

import (
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

// What a skill written for any project never says: who the user is, what
// the product is called, or an issue or pull request by its number. An
// agent in another repository would read them as its own and look for a
// person, an app or an issue that is not there.
var notGeneric = []struct {
	what string
	re   *regexp.Regexp
}{
	{"a person", regexp.MustCompile(`\bTim\b|timmrkz`)},
	{"the product", regexp.MustCompile(`(?i)frame ?fairy|My First Memory`)},
	{"an issue or pull request by number", regexp.MustCompile(`#[0-9]+|(?i)\b(issue|pull request|PR)s? [0-9]+`)},
}

// On the real disk: what is read is the repository's own skills.
func TestGenericSkillsNameNoPersonProductOrNumber(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", ".claude", "skills")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, e := range entries {
		if !e.IsDir() || slices.Contains(ownSkills, e.Name()) {
			continue
		}
		file := filepath.Join(root, e.Name(), "SKILL.md")
		text, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		read++
		for n, line := range strings.Split(string(text), "\n") {
			for _, bad := range notGeneric {
				if m := bad.re.FindString(line); m != "" {
					t.Errorf(".claude/skills/%s/SKILL.md:%d names %s, %q: a skill for any project names none", e.Name(), n+1, bad.what, m)
				}
			}
		}
	}
	if read < 2 {
		t.Fatalf("read %d generic skills in .claude/skills/, so this test reads the wrong folder", read)
	}
}

// The check above fails on what it should, and on nothing a skill for any
// project would say.
func TestNotGenericCatchesWhatItShould(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line string
		bad  bool
	}{
		{"Tim asked for it.", true},
		{"assigned to timmrkz", true},
		{"Frame Fairy makes shorts.", true},
		{"the framefairy app", true},
		{"see #742", true},
		{"as pull request 12 did", true},
		{"Issue 7 says so.", true},
		{"The user decides.", false},
		{"A timer in a test is a guess.", false},
		{"Time the product defines runs on a fake clock.", false},
		{"one frame in 60, 5 % slower", false},
		{"the first issue opened", false},
	}
	for _, c := range cases {
		hit := false
		for _, bad := range notGeneric {
			hit = hit || bad.re.MatchString(c.line)
		}
		if hit != c.bad {
			t.Errorf("%q: caught %v, want %v", c.line, hit, c.bad)
		}
	}
}
