package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The rule for a build is silent when it is wrong: a commit gets no build
// and the Updates page goes on offering the one before, or it says a build
// is coming that never will. Every case below is one a push really
// arrives as, in a repository the test makes, read by git the way the
// workflows read it.
//
// On the real disk: git reads the commits from a repository of its own.
func TestACommitNeedsABuildWhenItChangesTheApp(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal("git is needed to read which files two commits differ in")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(what string) string {
		run("add", "-A")
		run("-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false",
			"commit", "-q", "--allow-empty", "-m", what)
		return run("rev-parse", "HEAD")
	}
	run("init", "-q")
	write("engine/run.go", "a")
	app := commit("app")
	write(filepath.Join("docs", "APP.md"), "b")
	docs := commit("docs")
	write("README.md", "c")
	readme := commit("readme")
	write("engine/run.go", "d")
	code := commit("code")
	empty := commit("nothing")
	// A change taken back: the files are the ones built, whatever
	// happened on the way.
	write("engine/run.go", "a")
	back := commit("back")

	// GitHub knows nothing, the way it knows no commit that was never
	// pushed.
	t2 := gitTrees{&fakeGitHub{}}
	cases := []struct{ name, built, newest, want string }{
		{"a channel with no build", "", app, needed},
		{"the commit that is built", app, app, alreadyBuilt},
		{"the built commit, shortened", app[:12], app, alreadyBuilt},
		{"a push of the docs", app, docs, docsOnly},
		{"a README is docs too", app, readme, docsOnly},
		{"two pushes of docs, one after the other", docs, readme, docsOnly},
		{"code after docs", app, code, needed},
		{"code, measured from what is built", docs, code, needed},
		{"nothing changed in the commit", code, empty, alreadyBuilt},
		{"code changed and changed back", app, back, docsOnly},
		{"a commit nobody can read", app, "0123456789abcdef0123456789abcdef01234567", needed},
		{"a build nobody can read", "0123456789ab", code, needed},
	}
	for _, c := range cases {
		if got, why := needsBuild(t2, c.built, c.newest); got != c.want {
			t.Errorf("%s: said %s, %s, want %s", c.name, got, why, c.want)
		}
	}
}
