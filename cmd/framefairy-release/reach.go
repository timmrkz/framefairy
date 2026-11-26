package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
)

// What the app is built from, asked of the code itself on every run, so
// no list of folders says it and none can go stale. The programs the app
// ships are the ones linked names, which check holds to the real build.
// Go names a program after the last element of its package's path, so a
// package main of that name is that program, wherever it lives. Every
// package of this module it imports, and they import in turn, is the app,
// by every file but the tests and whatever its build tags, and so is
// every program of ours their go generate runs, which writes something
// the app is built with. See
// docs/decisions/2026-11-25-the-release-tool-knows-what-the-app-is-built-from.md.

// source is a commit's Go files by their paths, and the path of its module.
type source struct {
	module string
	files  map[string][]byte
}

// gitSource reads them out of a commit with git archive, in one pass.
func gitSource(commit string) (source, error) {
	// From the top of the checkout, since git archive run in a folder
	// would keep only that folder.
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return source{}, fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	cmd := exec.Command("git", "archive", "--format=tar", commit, "--", "go.mod", ":(glob)**/*.go")
	cmd.Dir = strings.TrimSpace(string(top))
	out, err := cmd.Output()
	if err != nil {
		return source{}, fmt.Errorf("git archive %s: %w", commit, err)
	}
	return readSource(bytes.NewReader(out))
}

func readSource(r io.Reader) (source, error) {
	s := source{files: map[string][]byte{}}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return s, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return s, err
		}
		if h.Name == "go.mod" {
			s.module = modulePath(b)
			continue
		}
		s.files[h.Name] = b
	}
	if s.module == "" {
		return s, errors.New("go.mod names no module")
	}
	return s, nil
}

// modulePath is what the module line of go.mod says.
func modulePath(gomod []byte) string {
	for line := range strings.Lines(string(gomod)) {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "module" {
			if p, err := strconv.Unquote(f[1]); err == nil {
				return p
			}
			return f[1]
		}
	}
	return ""
}

// programs is what a set of programs is built from: the folders of the
// packages they import, and the programs' own folders, whose tests go
// in too when tests is set, since the walks run the app's own tests.
type programs struct {
	dirs  map[string]bool
	mains map[string]bool
	tests bool
}

// ignored is a folder go build passes over: one whose name starts with a
// dot or an underscore, or is testdata.
func ignored(dir string) bool {
	for _, e := range strings.Split(dir, "/") {
		if strings.HasPrefix(e, ".") || strings.HasPrefix(e, "_") || e == "testdata" {
			return true
		}
	}
	return false
}

// appFrom works out what the app's programs are built from in s.
func appFrom(s source, tests bool) (programs, error) {
	b := programs{dirs: map[string]bool{}, mains: map[string]bool{}, tests: tests}
	pkgs := packages(s)
	for dir, files := range pkgs {
		if _, ships := linked[path.Base(dir)]; !ships {
			continue
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, s.files[f], parser.PackageClauseOnly)
			if err == nil && file.Name.Name == "main" {
				b.mains[dir] = true
				break
			}
		}
	}
	if len(b.mains) == 0 {
		return b, errors.New("no package main is named after a program the app ships")
	}
	return closure(s, b, pkgs)
}

// packages are the folders of s that go build builds, with their files.
func packages(s source) map[string][]string {
	pkgs := map[string][]string{}
	for name := range s.files {
		dir := path.Dir(name)
		if !ignored(dir) {
			pkgs[dir] = append(pkgs[dir], name)
		}
	}
	return pkgs
}

// closure fills b.dirs with the folders of b.mains and of every package
// of ours they import or have go generate run, and those in turn.
func closure(s source, b programs, pkgs map[string][]string) (programs, error) {
	tests := b.tests
	queue := slices.Sorted(maps.Keys(b.mains))
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if b.dirs[dir] {
			continue
		}
		b.dirs[dir] = true
		for _, f := range pkgs[dir] {
			if strings.HasSuffix(f, "_test.go") && !(tests && b.mains[dir]) {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, s.files[f], parser.ImportsOnly)
			if err != nil {
				return b, fmt.Errorf("%s cannot be read as Go: %w", f, err)
			}
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if own, ok := s.own(p); ok {
					queue = append(queue, own)
				}
			}
			runs, err := generates(s, dir, f)
			if err != nil {
				return b, err
			}
			queue = append(queue, runs...)
		}
	}
	return b, nil
}

// own is the folder of a package of the module, by its import path.
func (s source) own(importPath string) (string, bool) {
	if importPath == s.module {
		return ".", true
	}
	return strings.CutPrefix(importPath, s.module+"/")
}

// generates are the folders of the programs a file has go generate run,
// the way the build of a package runs a program of ours it does not
// import, like the one that writes the app's licences. go generate reads
// a directive as a line of text, so it is read the same way here. A
// directive that runs anything but a package of ours with go run cannot
// be judged, and is an error, so everything runs.
func generates(s source, dir, file string) ([]string, error) {
	var runs []string
	for line := range strings.Lines(string(s.files[file])) {
		args, ok := strings.CutPrefix(line, "//go:generate ")
		if !ok {
			continue
		}
		f := strings.Fields(args)
		from := dir
		if len(f) >= 3 && f[0] == "go" && f[1] == "-C" {
			from = path.Join(dir, f[2])
			f = append(f[:1], f[3:]...)
		}
		if len(f) < 3 || f[0] != "go" || f[1] != "run" {
			return nil, fmt.Errorf("%s has go generate run %q, which is no go run of a package of ours", file, strings.TrimSpace(args))
		}
		pkg := ""
		for _, a := range f[2:] {
			if !strings.HasPrefix(a, "-") {
				pkg = a
				break
			}
		}
		switch {
		case strings.HasPrefix(pkg, "./"), strings.HasPrefix(pkg, "../"):
			runs = append(runs, path.Join(from, pkg))
		default:
			own, ok := s.own(pkg)
			if !ok {
				return nil, fmt.Errorf("%s has go generate run %q, which is no go run of a package of ours", file, strings.TrimSpace(args))
			}
			runs = append(runs, own)
		}
	}
	return runs, nil
}

// docs is a file nothing is built from: the docs, a README, the agents'
// own folder.
func docs(file string) bool {
	return strings.HasPrefix(file, "docs/") || strings.HasPrefix(file, ".claude/") || strings.HasSuffix(file, ".md")
}

// reaches says whether a change to file can change what the programs are
// built from. Only Go is judged, by the packages the programs import, the
// way the compiler judges it. Every other file reaches them, a script,
// the Makefile, the interface, a file a package embeds: what reads it is
// not something Go can say.
func (b programs) reaches(file string) bool {
	if docs(file) {
		return false
	}
	if !strings.HasSuffix(file, ".go") {
		return true
	}
	dir := path.Dir(file)
	if ignored(dir) {
		// Go builds no file of such a folder, so it is data, which
		// something may read.
		return true
	}
	if strings.HasSuffix(file, "_test.go") {
		return b.tests && b.mains[dir]
	}
	return b.dirs[dir]
}

// reaching is the files of changed that reach the programs.
func (b programs) reaching(changed []string) []string {
	var r []string
	for _, f := range changed {
		if b.reaches(f) {
			r = append(r, f)
		}
	}
	return r
}

// walksPlan says, for ci.yml, whether a pull request's change reaches
// what the walks run: the app's programs, with the app's own tests, which
// is where the walks are. It compares the commit to with from, the pull
// request's merge commit with main's commit under it. Every doubt walks.
func walksPlan(t trees, from, to string) plan {
	var p plan
	walk := func(why string) plan {
		p.set("walks", "true")
		p.notice = why
		return p
	}
	files, err := t.changed(from, to)
	if err != nil {
		return walk("What the pull request changed cannot be read, so it is walked: " + err.Error())
	}
	src, err := t.source(to)
	if err != nil {
		return walk("The pull request's Go cannot be read, so it is walked: " + err.Error())
	}
	app, err := appFrom(src, true)
	if err != nil {
		return walk("What the app is built from cannot be worked out, so it is walked: " + err.Error())
	}
	if r := app.reaching(files); len(r) > 0 {
		return walk("The pull request changes what the walks build, " + sample(r) + ".")
	}
	p.set("walks", "false")
	p.notice = "The pull request changes nothing the walks build, only the docs, tests or code the app does not ship."
	return p
}
