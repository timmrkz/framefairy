package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// told is every event the Go side tells the window, by its name, with the
// Go type of what it carries, nil for a cue. It is the table the tests of
// the channel read, and TestEveryEventIsHeardAndEveryHeardEventIsSent
// holds it to the constants in events.go.
var told = map[string]reflect.Type{
	string(jobEvent):              carries(jobEvent),
	string(noticeEvent):           carries(noticeEvent),
	string(chromeAppEvent):        carries(chromeAppEvent),
	string(chromeReportEvent):     carries(chromeReportEvent),
	string(undoEvent):             carries(undoEvent),
	string(quitEvent):             carries(quitEvent),
	string(acknowledgementsEvent): nil,
	string(loudnessEvent):         carries(loudnessEvent),
	string(dictationEvent):        carries(dictationEvent),
	string(reportWindowEvent):     carries(reportWindowEvent),
	string(reportDraftEvent):      carries(reportDraftEvent),
	string(reportAboutEvent):      carries(reportAboutEvent),
	string(episodeEvent):          carries(episodeEvent),
	string(droppedEvent):          carries(droppedEvent),
	string(videoEvent):            carries(videoEvent),
	string(setupEvent):            carries(setupEvent),
	string(updatesEvent):          carries(updatesEvent),
	string(licenceLinkEvent):      nil,
	string(showUpdatesEvent):      nil,
}

func carries[T any](event[T]) reflect.Type { return reflect.TypeFor[T]() }

// An event is a name both sides write: Go when it tells the window, the
// interface when it listens. Nothing in either language checks that the
// two agree, so a name spelled one way in Go and another in api.ts is an
// event that goes nowhere, and a window that waits on it waits for good.
// The walks' bridge once wrote every name again on its own, so a name
// wrong in the app passed every walk.
//
// Every event events.go names is heard in api.ts, and every event api.ts
// listens for is one events.go names. Every event is sent somewhere in
// the app, outside the tests, and nothing in the app tells the window
// past events.go, by a name of its own.
//
// On the real disk: what is read is the app's own source and the
// interface's.
func TestEveryEventIsHeardAndEveryHeardEventIsSent(t *testing.T) {
	named := eventConstants(t)
	if want := slices.Sorted(maps.Keys(told)); !slices.Equal(slices.Sorted(maps.Values(named)), want) {
		t.Fatalf("the table of events in this test is %v, events.go names %v", want, slices.Sorted(maps.Values(named)))
	}
	heard := heardEvents(t)
	for name := range told {
		if _, ok := heard[name]; !ok {
			t.Errorf("Go tells the window %q, which api.ts never listens for", name)
		}
	}
	for name, h := range heard {
		if _, ok := told[name]; !ok {
			t.Errorf("api.ts:%d listens for %q, which Go never tells the window", h.line, name)
		}
	}
	used := uses(t)
	for constant, name := range named {
		if used[constant] < 2 {
			t.Errorf("%s, the event %q, is never sent by the app", constant, name)
		}
	}
	for _, at := range emitsByName(t) {
		t.Errorf("%s tells the window by a name of its own: tell it through an event in events.go", at)
	}
}

// What tellWindow wires each part to is the event of its name: two events
// that carry a string can be swapped without a compiler noticing.
func TestEveryPartTellsTheWindowOnItsOwnEvent(t *testing.T) {
	s := &FrameFairy{store: memoryStore()}
	s.newReport()
	heard := &heardWindow{}
	s.tellWindow(heard)
	t.Cleanup(func() {
		s.news.stop()
		s.jobs.shutDown()
	})
	steps := []struct {
		event string
		tell  func()
	}{
		{string(jobEvent), func() { s.jobs.emit(JobUpdate{Job: Job{Kind: "model"}}) }},
		{string(episodeEvent), func() { s.jobs.notify("/v.mp4") }},
		{string(noticeEvent), func() { s.jobs.notice(Notice{}) }},
		{string(dictationEvent), func() { s.listening.say(Dictation{}) }},
		{string(loudnessEvent), func() { s.listening.loud(Loudness{}) }},
		{string(reportWindowEvent), func() { s.report.told(true) }},
		{string(reportDraftEvent), func() { s.report.said(ReportDraft{}) }},
		{string(reportAboutEvent), func() { s.report.about("why") }},
		{string(setupEvent), func() { s.setupNews(SetupState{}) }},
	}
	for _, step := range steps {
		heard.names = nil
		step.tell()
		if !slices.Equal(heard.names, []string{step.event}) {
			t.Errorf("the part that tells %q told the window %v", step.event, heard.names)
		}
	}
}

// heardWindow is a window that keeps the names it was told.
type heardWindow struct{ names []string }

func (h *heardWindow) Emit(name string, _ ...any) bool {
	h.names = append(h.names, name)
	return false
}

// The constants of events.go that are events, by the name of the
// constant, with the name the window hears.
//
// On the real disk: what is read is the app's own source.
func eventConstants(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "events.go", nil, 0)
	if err != nil {
		t.Fatalf("reading events.go: %v", err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			v := spec.(*ast.ValueSpec)
			for i, n := range v.Names {
				lit, ok := v.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				name, _ := strconv.Unquote(lit.Value)
				out[n.Name] = name
			}
		}
	}
	if len(out) < 10 {
		t.Fatalf("found only %d events in events.go, the parsing is wrong", len(out))
	}
	return out
}

// aListen is one Events.On in api.ts: the line it is on and what the
// handler takes the data as, "" for nothing.
type aListen struct {
	line int
	as   string
}

// Every event api.ts listens for, by name. Each is heard by its name
// written out, so a name made up as the app runs is refused.
//
// On the real disk: what is read is the interface's own source.
func heardEvents(t *testing.T) map[string]aListen {
	t.Helper()
	text := apiText(t)
	out := map[string]aListen{}
	for at := 0; ; {
		i := strings.Index(text[at:], "Events.On(")
		if i < 0 {
			break
		}
		i += at
		inside, end := bracketed(text, i+len("Events.On"))
		at = end
		line := 1 + strings.Count(text[:i], "\n")
		parts := split(inside)
		name, err := strconv.Unquote(parts[0])
		if err != nil || !strings.HasPrefix(parts[0], `"`) {
			t.Errorf("api.ts:%d listens for %s, a name not written out", line, parts[0])
			continue
		}
		if _, twice := out[name]; twice {
			t.Errorf("api.ts:%d listens for %q a second time", line, name)
		}
		out[name] = aListen{line: line, as: dataAs(parts[1])}
	}
	if len(out) < 10 {
		t.Fatalf("found only %d events api.ts listens for, the parsing is wrong", len(out))
	}
	return out
}

// dataAs is the type a handler takes the event's data as: what follows
// "ev.data as", or the type Boolean or String makes of it.
func dataAs(handler string) string {
	switch {
	case strings.Contains(handler, "Boolean(ev.data"):
		return "boolean"
	case strings.Contains(handler, "String(ev.data"):
		return "string"
	}
	i := strings.Index(handler, "ev.data as ")
	if i < 0 {
		return ""
	}
	rest := handler[i+len("ev.data as "):]
	inside, _ := bracketed("("+rest, 0)
	return strings.TrimSpace(inside)
}

// uses counts how often each name is written in the app's own files,
// its tests aside. A constant written once is only declared.
//
// On the real disk: what is read is the app's own source.
func uses(t *testing.T) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, file := range appFiles(t) {
		ast.Inspect(file, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				out[id.Name]++
			}
			return true
		})
	}
	return out
}

// emitsByName is every call of Emit in the app's own files that names
// its event with a string written there, rather than an event of
// events.go.
//
// On the real disk: what is read is the app's own source.
func emitsByName(t *testing.T) []string {
	t.Helper()
	var out []string
	set := token.NewFileSet()
	for name, file := range appFilesIn(t, set) {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Emit" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				out = append(out, name+":"+strconv.Itoa(set.Position(call.Pos()).Line))
			}
			return true
		})
	}
	return out
}

// appFiles is every Go file of the app that is not a test, by its name.
//
// On the real disk: what is read is the app's own source.
func appFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	return appFilesIn(t, token.NewFileSet())
}

// On the real disk: what is read is the app's own source.
func appFilesIn(t *testing.T, set *token.FileSet) map[string]*ast.File {
	t.Helper()
	pkgs, err := parser.ParseDir(set, ".", nil, 0)
	if err != nil {
		t.Fatalf("reading the app: %v", err)
	}
	out := map[string]*ast.File{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if !strings.HasSuffix(path, "_test.go") {
				out[path] = file
			}
		}
	}
	return out
}

// apiText is the source of lib/api.ts.
//
// On the real disk: what is read is the interface's own source.
func apiText(t *testing.T) string {
	t.Helper()
	const path = "../../frontend/src/lib/api.ts"
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(body)
}
