package maccode

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Code that only a Mac builds is in a file named for the Mac. A file
// whose build constraint lets it build on macOS and not on Linux, with
// the same build tags, is named *_darwin.go or *_darwin_test.go, so the
// filter of the job macos in ci.yml, which looks for those names, sees
// every change to it.
//
// On the real disk: what this reads is the module's own source.
func TestCodeOnlyAMacBuildsIsNamedForIt(t *testing.T) {
	for _, f := range everyGoFile(t, unnamedMacFiles) {
		t.Errorf("%s. Name it *_darwin.go, or *_darwin_test.go for a test, so a pull request that changes it runs the Go tests on a Mac", f)
	}
}

// No file but one named for the Mac asks runtime.GOOS a question a Mac
// and Linux answer differently. Such a branch is code only a Mac runs,
// hidden in a file every system builds, so the filter of the job macos
// in ci.yml would not see a change to it. A question a Mac and Linux
// answer alike, runtime.GOOS == "windows", is no Mac code. Nor is
// runtime.GOOS handed to a function as a value: the function takes the
// system as a parameter, and a test on Linux hands it "darwin".
//
// On the real disk: what this reads is the module's own source.
func TestOnlyAFileForTheMacAsksWhetherItIsOnAMac(t *testing.T) {
	for _, f := range everyGoFile(t, macBranches) {
		t.Errorf("%s. Put what the Mac does in a *_darwin.go file and what the others do in a file built for them, //go:build !darwin", f)
	}
}

// goos is every system Go builds for. A build tag that names one is true
// only on it, see go help buildconstraint.
var goos = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios",
	"js", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1",
	"windows", "zos",
}

// unix is true on these, see go help buildconstraint.
var unix = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios",
	"linux", "netbsd", "openbsd", "solaris",
}

// goarch is every processor Go builds for. A file name that ends in one
// says nothing about the system.
var goarch = []string{
	"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le",
	"mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm",
}

// namedSystem is the system a file's name builds it for, foo_darwin.go
// or foo_darwin_arm64_test.go, or "" when the name names none.
func namedSystem(name string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(name), ".go"), "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return ""
	}
	last := parts[len(parts)-1]
	if slices.Contains(goarch, last) {
		if len(parts) < 3 {
			return ""
		}
		last = parts[len(parts)-2]
	}
	if slices.Contains(goos, last) {
		return last
	}
	return ""
}

// namedForTheMac says whether a file's name builds it only on a Mac.
func namedForTheMac(name string) bool { return namedSystem(name) == "darwin" }

// buildConstraint is a file's //go:build line, or nil when it has none.
func buildConstraint(f *ast.File) (constraint.Expr, error) {
	for _, group := range f.Comments {
		if group.Pos() > f.Package {
			break
		}
		for _, c := range group.List {
			if constraint.IsGoBuild(c.Text) {
				return constraint.Parse(c.Text)
			}
		}
	}
	return nil, nil
}

// builds says whether a file builds on system, with the build tags that
// name neither a system nor a processor set as in tags.
func builds(name string, expr constraint.Expr, system string, tags map[string]bool) bool {
	if named := namedSystem(name); named != "" && named != system {
		return false
	}
	if expr == nil {
		return true
	}
	return expr.Eval(func(tag string) bool {
		switch {
		case slices.Contains(goos, tag):
			return tag == system
		case tag == "unix":
			return slices.Contains(unix, system)
		case slices.Contains(goarch, tag):
			return true
		}
		return tags[tag]
	})
}

// ownTags are the tags of a constraint that name neither a system nor a
// processor, the ones a build sets by hand, like ffmpeglibs.
func ownTags(expr constraint.Expr) []string {
	var tags []string
	var walk func(constraint.Expr)
	walk = func(e constraint.Expr) {
		switch e := e.(type) {
		case *constraint.AndExpr:
			walk(e.X)
			walk(e.Y)
		case *constraint.OrExpr:
			walk(e.X)
			walk(e.Y)
		case *constraint.NotExpr:
			walk(e.X)
		case *constraint.TagExpr:
			if !slices.Contains(goos, e.Tag) && !slices.Contains(goarch, e.Tag) && e.Tag != "unix" && !slices.Contains(tags, e.Tag) {
				tags = append(tags, e.Tag)
			}
		}
	}
	if expr != nil {
		walk(expr)
	}
	return tags
}

// onAMac says whether some build builds the file on a Mac, and whether
// one builds it on a Mac and not on Linux with the same tags.
func onAMac(name string, expr constraint.Expr) (some, only bool) {
	own := ownTags(expr)
	for set := 0; set < 1<<len(own); set++ {
		tags := map[string]bool{}
		for i, tag := range own {
			tags[tag] = set&(1<<i) != 0
		}
		mac := builds(name, expr, "darwin", tags)
		some = some || mac
		only = only || mac && !builds(name, expr, "linux", tags)
	}
	return some, only
}

// unnamedMacFiles finds a file only a Mac builds whose name does not say
// so.
func unnamedMacFiles(name string, fset *token.FileSet, f *ast.File) ([]string, error) {
	expr, err := buildConstraint(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if _, only := onAMac(name, expr); only && !namedForTheMac(name) {
		return []string{name + " builds on a Mac and not on Linux"}, nil
	}
	return nil, nil
}

// macBranches finds every use of runtime.GOOS that could tell a Mac from
// Linux, in a file a Mac builds and that is not named for it. It does not
// list the uses it forbids, it lists the ones it allows, so a new way of
// asking fails until it is judged:
//
//   - compared, == or !=, with a written system other than darwin and
//     linux, runtime.GOOS == "windows",
//   - the subject of a switch whose cases are all written systems and
//     that sends a Mac and Linux to the same clause,
//   - handed to a function or method of this module, set in a field of a
//     composite literal or returned, as a value. The code that receives
//     it takes the system as a parameter, and a test on Linux hands it
//     "darwin".
//
// Handed to a function of another package, like strings.EqualFold, kept
// in a variable or compared with a constant, it could be a question a Mac
// and Linux answer differently, so it is found.
func macBranches(name string, fset *token.FileSet, f *ast.File) ([]string, error) {
	expr, err := buildConstraint(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if some, _ := onAMac(name, expr); !some || namedForTheMac(name) {
		return nil, nil
	}
	// The names the file imports runtime under, "." for a dot import, and
	// the names of the packages it imports from outside this module.
	var runtimeNames, foreign []string
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		as := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			as = imp.Name.Name
		}
		if path == "runtime" {
			runtimeNames = append(runtimeNames, as)
		}
		if path != module && !strings.HasPrefix(path, module+"/") {
			foreign = append(foreign, as)
		}
	}
	if len(runtimeNames) == 0 {
		return nil, nil
	}
	isGOOS := func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			id, ok := n.X.(*ast.Ident)
			return ok && n.Sel.Name == "GOOS" && slices.Contains(runtimeNames, id.Name)
		case *ast.Ident:
			return n.Name == "GOOS" && slices.Contains(runtimeNames, ".")
		}
		return false
	}
	system := func(e ast.Expr) (string, bool) {
		lit, ok := ast.Unparen(e).(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(lit.Value)
		return s, err == nil
	}
	differs := func(s string) bool { return s == "darwin" || s == "linux" }
	// ownCall says whether a call goes to code of this module: a function
	// of this package or of a package of the module, or a method.
	ownCall := func(fun ast.Expr) bool {
		switch fun := ast.Unparen(fun).(type) {
		case *ast.Ident:
			return !slices.Contains(builtins, fun.Name)
		case *ast.SelectorExpr:
			id, ok := fun.X.(*ast.Ident)
			return !ok || !slices.Contains(foreign, id.Name)
		}
		return false
	}
	var found []string
	at := func(n ast.Node, what string) {
		found = append(found, fmt.Sprintf("%s:%d %s", name, fset.Position(n.Pos()).Line, what))
	}
	// judge looks at one use of runtime.GOOS, with the nodes it stands in,
	// innermost last.
	judge := func(use ast.Node, outer []ast.Node) {
		i := len(outer) - 1
		for ; i >= 0; i-- {
			if _, ok := outer[i].(*ast.ParenExpr); !ok {
				break
			}
			use = outer[i]
		}
		if i < 0 {
			at(use, "uses runtime.GOOS")
			return
		}
		switch p := outer[i].(type) {
		case *ast.BinaryExpr:
			other := p.X
			if p.X == use {
				other = p.Y
			}
			if s, ok := system(other); ok && (p.Op == token.EQL || p.Op == token.NEQ) {
				if differs(s) {
					at(p, fmt.Sprintf("asks whether runtime.GOOS is %q", s))
				}
				return
			}
		case *ast.SwitchStmt:
			// The clause each system lands in, -1 for the default.
			lands := map[string]int{"darwin": -1, "linux": -1}
			for c, stmt := range p.Body.List {
				for _, e := range stmt.(*ast.CaseClause).List {
					s, ok := system(e)
					if !ok {
						at(p, "switches on runtime.GOOS with a case that is not a written system")
						return
					}
					if differs(s) {
						lands[s] = c
					}
				}
			}
			if lands["darwin"] != lands["linux"] {
				at(p, "switches on runtime.GOOS with a Mac and Linux in different cases")
			}
			return
		case *ast.CallExpr:
			if p.Fun != use && ownCall(p.Fun) {
				return
			}
		case *ast.KeyValueExpr:
			if _, ok := outer[max(i-1, 0)].(*ast.CompositeLit); ok && p.Value == use {
				return
			}
		case *ast.ReturnStmt:
			return
		}
		at(use, "uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently")
	}
	var outer []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			outer = outer[:len(outer)-1]
			return true
		}
		if isGOOS(n) {
			judge(n, outer)
		}
		outer = append(outer, n)
		return true
	})
	return found, nil
}

// module is this module's path, from go.mod.
const module = "framefairy"

// builtins are Go's own functions that could read a system's name.
var builtins = []string{"len", "cap", "min", "max", "string", "append", "copy"}

// everyGoFile reads every Go file of the module with rule, and gives what
// the rule found in them.
//
// On the real disk: what this reads is the module's own source.
func everyGoFile(t *testing.T, rule func(name string, fset *token.FileSet, f *ast.File) ([]string, error)) []string {
	t.Helper()
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
			case ".git", "node_modules", "frontend", "bin", ".build", "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		read++
		rel, _ := filepath.Rel(root, path)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, mustRead(t, path), parser.ParseComments)
		if err != nil {
			return err
		}
		lines, err := rule(rel, fset, f)
		if err != nil {
			return err
		}
		found = append(found, lines...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if read < 100 {
		t.Fatalf("read %d Go files under %s, too few to be the module", read, root)
	}
	return found
}

// mustRead reads a file of the module.
//
// On the real disk: what this reads is the module's own source.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// check runs rule over one source file in memory.
func check(t *testing.T, rule func(string, *token.FileSet, *ast.File) ([]string, error), name, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	found, err := rule(name, fset, f)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// The check finds a file only a Mac builds by its constraint, whatever
// tags it needs besides, and lets be a file named for the Mac, one every
// system builds, and one a Mac never builds.
func TestTheCheckFindsAFileOnlyAMacBuilds(t *testing.T) {
	cases := []struct {
		name, build string
		found       bool
	}{
		{"keys.go", "darwin", true},
		{"chip.go", "ffmpeglibs && darwin", true},
		{"run_test.go", "outside && darwin", true},
		{"tidy.go", "!linux", true},
		{"probe.go", "!outside && darwin", true},
		{"keys_darwin.go", "", false},
		{"keys_darwin_test.go", "", false},
		{"chip_darwin_arm64.go", "ffmpeglibs", false},
		{"keys_other.go", "!darwin", false},
		{"cores_linux.go", "", false},
		{"unix.go", "unix", false},
		{"main.go", "ffmpeglibs", false},
		{"main.go", "", false},
		{"win.go", "windows", false},
	}
	for _, c := range cases {
		src := "package x\n"
		if c.build != "" {
			src = "//go:build " + c.build + "\n\n" + src
		}
		got := check(t, unnamedMacFiles, c.name, src)
		if (len(got) > 0) != c.found {
			t.Errorf("%s with //go:build %s: found %v, want found %v", c.name, c.build, got, c.found)
		}
	}
}

// The check finds the questions a Mac and Linux answer differently, under
// another name for runtime too, and every use it cannot judge: kept in a
// variable, compared with a constant, handed to another package's
// function. It lets be the questions a Mac and Linux answer alike,
// runtime.GOOS handed on as a value, and any use in a file for the Mac or
// one a Mac never builds.
func TestTheCheckFindsABranchForTheMac(t *testing.T) {
	src := `package x

import (
	"runtime"
	rt "runtime"
	"strings"

	"framefairy/engine"
)

const mac = "darwin"

func f() string {
	if runtime.GOOS == "darwin" {
	}
	if runtime.GOOS != "linux" {
	}
	if "darwin" == (rt.GOOS) {
	}
	if runtime.GOOS == "windows" {
	}
	switch runtime.GOOS {
	case "darwin":
	case "linux":
	}
	switch runtime.GOOS {
	case "darwin", "linux":
	case "windows":
	}
	switch runtime.GOOS {
	case "linux":
	}
	switch runtime.GOOS {
	case "windows":
	}
	goos := runtime.GOOS
	if runtime.GOOS == mac {
	}
	switch runtime.GOOS {
	case mac:
	}
	if strings.EqualFold(runtime.GOOS, "darwin") || strings.HasPrefix(runtime.GOOS, "dar") {
	}
	if len(runtime.GOOS) == 6 {
	}
	g(runtime.GOOS)
	engine.SystemMedia(runtime.GOOS)
	s.setUp(goos, runtime.GOOS)
	_ = info{OS: runtime.GOOS}
	return runtime.GOOS
}
`
	want := []string{
		`x.go:14 asks whether runtime.GOOS is "darwin"`,
		`x.go:16 asks whether runtime.GOOS is "linux"`,
		`x.go:18 asks whether runtime.GOOS is "darwin"`,
		`x.go:22 switches on runtime.GOOS with a Mac and Linux in different cases`,
		`x.go:30 switches on runtime.GOOS with a Mac and Linux in different cases`,
		`x.go:36 uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently`,
		`x.go:37 uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently`,
		`x.go:39 switches on runtime.GOOS with a case that is not a written system`,
		`x.go:42 uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently`,
		`x.go:42 uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently`,
		`x.go:44 uses runtime.GOOS in a way this check cannot tell from a question a Mac and Linux answer differently`,
	}
	if got := check(t, macBranches, "x.go", src); !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, name := range []string{"x_darwin.go", "x_windows.go"} {
		if got := check(t, macBranches, name, src); len(got) > 0 {
			t.Errorf("%s: found %v, want nothing", name, got)
		}
	}
	if got := check(t, macBranches, "x.go", "//go:build !darwin\n\n"+src); len(got) > 0 {
		t.Errorf("a file a Mac never builds: found %v, want nothing", got)
	}
}
