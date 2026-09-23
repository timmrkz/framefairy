package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The interface calls the Go side by name and by position, and nothing in
// either language checks that the two agree. TypeScript knows the shape of
// api.ts and nothing about main.go, Go knows the methods and nothing about
// who calls them, and the runtime finds out at the moment a hand reaches
// for the control: main.FrameFairy.CutClip expects 7 arguments, got 6. That
// was shift-drag on the clip timeline, dead, with the engine, the tests and
// the fuzzing all green, because the one thing nobody looked at was whether
// the call had as many arguments as the method.
//
// This walks the whole boundary rather than that one method. Every call in
// api.ts has to name a method that exists and hand it the arguments it
// takes.

// aCall is one call in api.ts: which method, with how many arguments, and
// where to point when it is wrong.
type aCall struct {
	method string
	args   int
	line   int
}

func TestEveryCallReachesItsMethod(t *testing.T) {
	methods := serviceMethods(t)
	for _, c := range apiCalls(t) {
		takes, ok := methods[c.method]
		if !ok {
			t.Errorf("api.ts:%d calls %s, which the service does not have", c.line, c.method)
			continue
		}
		if takes != c.args {
			t.Errorf("api.ts:%d calls %s with %d argument(s), the method takes %d",
				c.line, c.method, c.args, takes)
		}
	}
}

// Every method of the service that the interface could call, and how many
// arguments it takes. The context is not one of them: Wails fills it in.
func serviceMethods(t *testing.T) map[string]int {
	t.Helper()
	set := token.NewFileSet()
	pkgs, err := parser.ParseDir(set, ".", nil, 0)
	if err != nil {
		t.Fatalf("reading the service: %v", err)
	}
	out := map[string]int{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
					continue
				}
				star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				name, ok := star.X.(*ast.Ident)
				if !ok || name.Name != "FrameFairy" || !fn.Name.IsExported() {
					continue
				}
				out[fn.Name.Name] = takes(fn.Type.Params)
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("found only %d methods on the service, the parsing is wrong", len(out))
	}
	return out
}

// How many arguments a caller hands in. A field can name several at once,
// as in "path, plan, clipID string", and a leading context is Wails's to
// fill rather than the interface's to send.
func takes(params *ast.FieldList) int {
	n := 0
	for i, f := range params.List {
		names := len(f.Names)
		if names == 0 {
			names = 1
		}
		if i == 0 && isContext(f.Type) {
			continue
		}
		n += names
	}
	return n
}

func isContext(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "context" && sel.Sel.Name == "Context"
}

// Every call in api.ts. They all go through one helper and they are all
// flat, so finding the closing bracket is enough: call<Clip>("CutClip",
// path, plan, clip, from, to).
func apiCalls(t *testing.T) []aCall {
	t.Helper()
	const path = "../../frontend/src/lib/api.ts"
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := string(body)
	var out []aCall
	for at := 0; ; {
		i := strings.Index(text[at:], "call<")
		if i < 0 {
			break
		}
		i += at
		open := strings.Index(text[i:], "(")
		if open < 0 {
			t.Fatalf("api.ts: a call with no bracket at byte %d", i)
		}
		open += i
		inside, end := bracketed(text, open)
		at = end
		parts := split(inside)
		if len(parts) == 0 || !strings.HasPrefix(parts[0], `"`) {
			t.Fatalf("api.ts: a call whose first argument is not a method name: %q", inside)
		}
		out = append(out, aCall{
			method: strings.Trim(parts[0], `"`),
			args:   len(parts) - 1,
			line:   1 + strings.Count(text[:i], "\n"),
		})
	}
	if len(out) < 20 {
		t.Fatalf("found only %d calls in api.ts, the parsing is wrong", len(out))
	}
	return out
}

// What is between an opening bracket and the one that closes it, and where
// the closing one is.
func bracketed(text string, open int) (string, int) {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[open+1 : i], i + 1
			}
		}
	}
	return text[open+1:], len(text)
}

// The arguments of one call, split on the commas that are not inside
// anything.
func split(inside string) []string {
	var out []string
	depth, last := 0, 0
	for i := 0; i < len(inside); i++ {
		switch inside[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(inside[last:i]))
				last = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(inside[last:]); rest != "" {
		out = append(out, rest)
	}
	return out
}
