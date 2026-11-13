package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// What crosses between Go and the window is a type written twice, once as
// a Go struct and once as an interface in lib/api.ts, and nothing in
// either language checks that the two agree. A json tag renamed in Go, a
// field api.ts reads that Go never sends, or one Go leaves out when it is
// empty that api.ts reads as always there, compiles on both sides and
// reads as undefined in the window, until a walk happens to look at it.
//
// Every type an event carries, every answer of a call and every argument
// of one is held to its interface in api.ts, field by field and both
// ways, all the way down: the name, whether it may be missing, and its
// kind, a string, a number, a boolean, a list, a map or an object.
//
// On the real disk: what is read is the interface's own source.
func TestTheInterfaceReadsTheFieldsGoSends(t *testing.T) {
	text := apiText(t)
	c := &channel{ts: readTS(text), seen: map[string]bool{}}
	for _, name := range c.ts.twice {
		c.errorf("api.ts declares %s twice, and TypeScript merges the two into one type with the fields of both", name)
	}
	for name, h := range heardEvents(t) {
		g, ok := told[name]
		if !ok {
			continue
		}
		where := fmt.Sprintf("the event %q", name)
		if g == nil {
			if h.as != "" {
				c.errorf("%s carries nothing, api.ts:%d reads it as %s", where, h.line, h.as)
			}
			continue
		}
		c.check(g, h.as, where, true)
	}
	service := reflect.TypeFor[*FrameFairy]()
	for _, call := range typedCalls(t, text) {
		m, ok := service.MethodByName(call.method)
		if !ok {
			// TestEveryCallReachesItsMethod says so.
			continue
		}
		where := fmt.Sprintf("api.ts:%d, %s", call.line, call.method)
		answer := answerOf(m.Type)
		switch {
		case answer == nil && call.answer != "void":
			c.errorf("%s answers with nothing, api.ts reads %s", where, call.answer)
		case answer != nil && call.answer != "void":
			c.check(answer, call.answer, where, true)
		}
		params := paramsOf(m.Type)
		for i, arg := range call.args {
			if i < len(params) && arg != "" {
				c.check(params[i], arg, fmt.Sprintf("%s, argument %d", where, i+1), false)
			}
		}
	}
	for _, e := range c.errs {
		t.Error(e)
	}
}

// channel compares the Go types with the interfaces of api.ts.
type channel struct {
	ts   *tsDecls
	seen map[string]bool
	errs []string
}

func (c *channel) errorf(format string, args ...any) {
	c.errs = append(c.errs, fmt.Sprintf(format, args...))
}

// check holds g to the type written ts, where says what it is, and
// goSends whether Go sends it to the window or reads it from there.
func (c *channel) check(g reflect.Type, ts, where string, goSends bool) {
	t, err := parseType(ts)
	if err != nil {
		c.errorf("%s: %v", where, err)
		return
	}
	c.compare(g, t, where, goSends)
}

var (
	timeType = reflect.TypeFor[time.Time]()
	rawType  = reflect.TypeFor[json.RawMessage]()
)

func (c *channel) compare(g reflect.Type, t *tsType, where string, goSends bool) {
	members, anything := c.flatten(t, where)
	if anything || len(members) == 0 {
		return
	}
	for g.Kind() == reflect.Pointer {
		g = g.Elem()
	}
	kind := tsKindOf(g)
	if kind == "any" {
		return
	}
	for _, m := range members {
		if kind == "record" && m.kind == "object" {
			// An object written field by field is a map to Go, which
			// reads each field as what the map holds.
			fields := m.fields
			if m.name != "" {
				fields = c.fieldsOf(m.name, where)
			}
			for _, name := range slices.Sorted(maps.Keys(fields)) {
				c.compare(g.Elem(), fields[name].of, where+"."+name, goSends)
			}
			continue
		}
		if m.kind != kind {
			c.errorf("%s: Go has %s, a %s, api.ts %s", where, g, kind, describe(m))
			return
		}
	}
	switch kind {
	case "array":
		for _, m := range members {
			c.compare(g.Elem(), m.elem, where+"[]", goSends)
		}
	case "record":
		for _, m := range members {
			if m.kind == "record" {
				c.compare(g.Elem(), m.elem, where+"{}", goSends)
			}
		}
	case "object":
		for _, m := range members {
			c.object(g, m, where, goSends)
		}
	}
}

// tsKindOf is the kind of TypeScript type a Go type is written as in
// JSON.
func tsKindOf(g reflect.Type) string {
	switch {
	case g == timeType:
		return "string"
	case g == rawType:
		return "any"
	}
	switch g.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		if g.Elem().Kind() == reflect.Uint8 {
			return "string"
		}
		return "array"
	case reflect.Map:
		return "record"
	case reflect.Struct:
		return "object"
	}
	return "any"
}

func describe(m tsMember) string {
	if m.kind == "object" && m.name != "" {
		return m.name
	}
	return "a " + m.kind
}

// flatten is the members of t that are not null, with every alias and
// interface it names read, and whether any of them takes anything.
func (c *channel) flatten(t *tsType, where string) ([]tsMember, bool) {
	var out []tsMember
	for _, m := range t.members {
		switch m.kind {
		case "null", "void":
			continue
		case "any":
			return nil, true
		case "ref":
			if alias, ok := c.ts.aliases[m.name]; ok {
				a, err := parseType(alias)
				if err != nil {
					c.errorf("%s, the type %s: %v", where, m.name, err)
					return nil, true
				}
				inner, anything := c.flatten(a, where)
				if anything {
					return nil, true
				}
				out = append(out, inner...)
				continue
			}
			if _, ok := c.ts.interfaces[m.name]; !ok {
				c.errorf("%s: api.ts names %s, which it does not declare", where, m.name)
				return nil, true
			}
			out = append(out, tsMember{kind: "object", name: m.name, partial: m.partial})
		default:
			out = append(out, m)
		}
	}
	return out, false
}

// fieldsOf are the fields of an interface, those it extends included.
func (c *channel) fieldsOf(name, where string) map[string]tsField {
	in := c.ts.interfaces[name]
	out := map[string]tsField{}
	for _, e := range in.extends {
		if _, ok := c.ts.interfaces[e]; !ok {
			c.errorf("%s: %s extends %s, which api.ts does not declare as an interface", where, name, e)
			continue
		}
		for k, v := range c.fieldsOf(e, where) {
			out[k] = v
		}
	}
	own, err := parseFields(in.body)
	if err != nil {
		c.errorf("%s, the interface %s: %v", where, name, err)
	}
	for k, v := range own {
		out[k] = v
	}
	return out
}

func (c *channel) object(g reflect.Type, m tsMember, where string, goSends bool) {
	ts := m.fields
	if m.name != "" {
		key := fmt.Sprintf("%s|%s|%v|%v", g, m.name, m.partial, goSends)
		if c.seen[key] {
			return
		}
		c.seen[key] = true
		ts = c.fieldsOf(m.name, where)
		where = m.name
	}
	gos := jsonFields(g)
	for _, name := range slices.Sorted(maps.Keys(gos)) {
		f := gos[name]
		tf, ok := ts[name]
		if !ok {
			if goSends {
				c.errorf("Go sends %s.%s (%s), which api.ts does not name", where, name, g)
			}
			continue
		}
		optional := tf.optional || m.partial
		if goSends && f.left && !optional {
			c.errorf("Go leaves %s.%s out when it is empty, api.ts reads it as always there", where, name)
		}
		c.compare(f.of, tf.of, where+"."+name, goSends)
	}
	for _, name := range slices.Sorted(maps.Keys(ts)) {
		if _, ok := gos[name]; ok {
			continue
		}
		if goSends {
			c.errorf("api.ts reads %s.%s, which Go (%s) never sends", where, name, g)
		} else {
			c.errorf("api.ts sends %s.%s, which Go (%s) does not read", where, name, g)
		}
	}
}

// goField is a field as encoding/json writes it: its type, and whether
// it is left out when it is empty.
type goField struct {
	of   reflect.Type
	left bool
}

// jsonFields are the fields encoding/json writes of a struct, by the name
// it writes them under, with the fields of an embedded struct among them.
func jsonFields(g reflect.Type) map[string]goField {
	out := map[string]goField{}
	for f := range g.Fields() {
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		inner := f.Type
		for inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if f.Anonymous && name == "" && inner.Kind() == reflect.Struct {
			for k, v := range jsonFields(inner) {
				if _, ok := out[k]; !ok {
					out[k] = v
				}
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		omitEmpty := slices.Contains(strings.Split(opts, ","), "omitempty")
		omitZero := slices.Contains(strings.Split(opts, ","), "omitzero")
		left := omitZero || (omitEmpty && inner.Kind() != reflect.Struct) || (omitEmpty && f.Type.Kind() == reflect.Pointer)
		out[name] = goField{of: f.Type, left: left}
	}
	return out
}

// answerOf is what a method answers with, its first result that is not
// an error, or nil.
func answerOf(m reflect.Type) reflect.Type {
	for i := range m.NumOut() {
		if out := m.Out(i); out != reflect.TypeFor[error]() {
			return out
		}
	}
	return nil
}

// paramsOf are what a method takes from the window: its arguments, past
// the receiver and a leading context.
func paramsOf(m reflect.Type) []reflect.Type {
	var out []reflect.Type
	for i := 1; i < m.NumIn(); i++ {
		if i == 1 && m.In(i) == reflect.TypeFor[context.Context]() {
			continue
		}
		out = append(out, m.In(i))
	}
	return out
}

// aTypedCall is one call in api.ts with the types it is written with:
// what it reads the answer as, and the type of each argument that is a
// parameter of the function it is in, "" for one that is not.
type aTypedCall struct {
	method string
	answer string
	args   []string
	line   int
}

// typedCalls are the calls in api.ts with their types. Every call is the
// body of an arrow function, (path: string, at: number) =>
// call<ClipEntry>("Method", path, at), so the type of an argument is the
// type of the parameter of that name.
func typedCalls(t *testing.T, text string) []aTypedCall {
	t.Helper()
	text = withoutComments(text)
	var out []aTypedCall
	for at := 0; ; {
		i := strings.Index(text[at:], "call<")
		if i < 0 {
			break
		}
		i += at
		at = i + len("call<")
		line := 1 + strings.Count(text[:i], "\n")
		answer, open := angled(text, i+len("call"))
		if open >= len(text) || text[open] != '(' {
			continue
		}
		inside, _ := bracketed(text, open)
		parts := split(inside)
		if len(parts) == 0 || !strings.HasPrefix(parts[0], `"`) {
			continue
		}
		method, _ := strconv.Unquote(parts[0])
		params := arrowParams(text, i)
		var args []string
		for _, a := range parts[1:] {
			args = append(args, params[a])
		}
		out = append(out, aTypedCall{method: method, answer: answer, args: args, line: line})
	}
	if len(out) < 20 {
		t.Fatalf("found only %d calls in api.ts, the parsing is wrong", len(out))
	}
	return out
}

// angled is what is between the angle bracket at open and the one that
// closes it, and where the text goes on after it.
func angled(text string, open int) (string, int) {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '<':
			depth++
		case '>':
			if text[i-1] == '=' {
				continue
			}
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[open+1 : i]), i + 1
			}
		}
	}
	return "", len(text)
}

// arrowParams are the parameters of the arrow function a call at i is
// the body of, each with its type.
func arrowParams(text string, i int) map[string]string {
	arrow := strings.LastIndex(text[:i], "=>")
	if arrow < 0 || strings.TrimSpace(text[arrow+2:i]) != "" {
		return nil
	}
	end := strings.LastIndex(text[:arrow], ")")
	if end < 0 {
		return nil
	}
	depth := 0
	start := -1
	for j := end; j >= 0; j-- {
		switch text[j] {
		case ')', ']', '}', '>':
			depth++
		case '(', '[', '{', '<':
			depth--
		}
		if depth == 0 {
			start = j
			break
		}
	}
	if start < 0 {
		return nil
	}
	out := map[string]string{}
	for _, p := range splitTop(text[start+1 : end]) {
		name, of, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		out[strings.TrimSuffix(strings.TrimSpace(name), "?")] = strings.TrimSpace(of)
	}
	return out
}

// splitTop splits on the commas that are not inside any bracket, angle
// brackets included.
func splitTop(s string) []string {
	var out []string
	depth, last := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{', '<':
			depth++
		case ')', ']', '}', '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[last:i]))
				last = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(s[last:]); rest != "" {
		out = append(out, rest)
	}
	return out
}
