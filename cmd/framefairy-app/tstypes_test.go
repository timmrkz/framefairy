package main

// A reader of the types lib/api.ts declares, for the tests that hold them
// to the Go types they stand for, see channel_test.go. It reads the part
// of TypeScript api.ts writes its types in: interfaces, with extends,
// aliases, unions, string and number literals, arrays, tuples, objects
// written inline, Record and Partial. Anything else is refused by name, so a type
// the reader does not know fails the test rather than passing it.

import (
	"fmt"
	"regexp"
	"strings"
)

// tsType is one TypeScript type: the union of its members.
type tsType struct{ members []tsMember }

// tsMember is one member of a union.
type tsMember struct {
	// kind is string, number, boolean, null, any, void, array, record,
	// object or ref.
	kind string
	// elem is what an array or a record holds.
	elem *tsType
	// fields are an object's, by name.
	fields map[string]tsField
	// name is the type a ref names, and partial says it is in Partial<>.
	name    string
	partial bool
}

type tsField struct {
	optional bool
	of       *tsType
}

// tsDecls are the types a TypeScript file declares, as written, by name.
type tsDecls struct {
	// interfaces are the bodies, and the names they extend.
	interfaces map[string]tsInterface
	aliases    map[string]string
	// twice are the names declared more than once.
	twice []string
}

type tsInterface struct {
	body    string
	extends []string
}

var (
	interfaceHead = regexp.MustCompile(`(?m)^(?:export )?interface (\w+)(?: extends ([\w, ]+))? \{`)
	aliasHead     = regexp.MustCompile(`(?m)^(?:export )?type (\w+) =\s*`)
)

// readTS finds every interface and alias declared at the top of text.
func readTS(text string) *tsDecls {
	text = withoutComments(text)
	d := &tsDecls{interfaces: map[string]tsInterface{}, aliases: map[string]string{}}
	seen := map[string]bool{}
	declared := func(name string) {
		if seen[name] {
			d.twice = append(d.twice, name)
		}
		seen[name] = true
	}
	for _, m := range interfaceHead.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		var extends []string
		if m[4] >= 0 {
			for e := range strings.SplitSeq(text[m[4]:m[5]], ",") {
				extends = append(extends, strings.TrimSpace(e))
			}
		}
		open := m[1] - 1
		end := closing(text, open)
		declared(name)
		d.interfaces[name] = tsInterface{body: text[open+1 : end], extends: extends}
	}
	for _, m := range aliasHead.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		end := m[1]
		for depth := 0; end < len(text); end++ {
			c := text[end]
			if c == '{' || c == '(' || c == '<' || c == '[' {
				depth++
			} else if c == '}' || c == ')' || c == '>' || c == ']' {
				depth--
			} else if c == ';' && depth == 0 {
				break
			}
		}
		declared(name)
		d.aliases[name] = text[m[1]:end]
	}
	return d
}

// closing is where the brace that opens at open closes.
func closing(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text)
}

// withoutComments is text with every // comment taken out, the ones in a
// string kept as they are.
func withoutComments(text string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == '\\' && i+1 < len(text) {
				b.WriteByte(c)
				i++
				c = text[i]
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '/' && i+1 < len(text) && text[i+1] == '/':
			for i < len(text) && text[i] != '\n' {
				i++
			}
			if i < len(text) {
				b.WriteByte('\n')
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// parseType reads a type written in TypeScript.
func parseType(text string) (*tsType, error) {
	p := &tsParser{text: text}
	t, err := p.union()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.at < len(p.text) {
		return nil, fmt.Errorf("the type %q goes on after %q", text, p.text[:p.at])
	}
	return t, nil
}

// parseFields reads the fields of an object, as between its braces.
func parseFields(body string) (map[string]tsField, error) {
	p := &tsParser{text: body}
	return p.fields()
}

type tsParser struct {
	text string
	at   int
}

func (p *tsParser) space() {
	for p.at < len(p.text) && strings.ContainsRune(" \t\r\n", rune(p.text[p.at])) {
		p.at++
	}
}

// takes says whether what comes next is s, and steps over it if it is.
func (p *tsParser) takes(s string) bool {
	p.space()
	if strings.HasPrefix(p.text[p.at:], s) {
		p.at += len(s)
		return true
	}
	return false
}

func (p *tsParser) union() (*tsType, error) {
	t := &tsType{}
	p.takes("|")
	for {
		m, err := p.postfix()
		if err != nil {
			return nil, err
		}
		t.members = append(t.members, m.members...)
		if !p.takes("|") {
			return t, nil
		}
	}
}

func (p *tsParser) postfix() (*tsType, error) {
	t, err := p.primary()
	if err != nil {
		return nil, err
	}
	for p.takes("[]") {
		t = &tsType{members: []tsMember{{kind: "array", elem: t}}}
	}
	return t, nil
}

var (
	tsWord   = regexp.MustCompile(`^[A-Za-z_]\w*`)
	tsNumber = regexp.MustCompile(`^-?\d+(\.\d+)?`)
)

func (p *tsParser) primary() (*tsType, error) {
	p.space()
	rest := p.text[p.at:]
	one := func(m tsMember) (*tsType, error) { return &tsType{members: []tsMember{m}}, nil }
	switch {
	case rest == "":
		return nil, fmt.Errorf("a type ends early in %q", p.text)
	case rest[0] == '(':
		p.at++
		t, err := p.union()
		if err != nil {
			return nil, err
		}
		if !p.takes(")") {
			return nil, fmt.Errorf("a bracket does not close in %q", p.text)
		}
		return t, nil
	case rest[0] == '{':
		end := closing(p.text, p.at)
		fields, err := parseFields(p.text[p.at+1 : end])
		if err != nil {
			return nil, err
		}
		p.at = end + 1
		return one(tsMember{kind: "object", fields: fields})
	case rest[0] == '[':
		// A tuple, [number, number], is a list of what it holds.
		p.at++
		elem := &tsType{}
		for !p.takes("]") {
			t, err := p.union()
			if err != nil {
				return nil, err
			}
			elem.members = append(elem.members, t.members...)
			if !p.takes(",") && !strings.HasPrefix(p.text[p.at:], "]") {
				p.space()
				if !strings.HasPrefix(p.text[p.at:], "]") {
					return nil, fmt.Errorf("a tuple does not close in %q", p.text)
				}
			}
		}
		return one(tsMember{kind: "array", elem: elem})
	case rest[0] == '"' || rest[0] == '\'':
		end := strings.IndexByte(rest[1:], rest[0])
		if end < 0 {
			return nil, fmt.Errorf("a string does not end in %q", p.text)
		}
		p.at += end + 2
		return one(tsMember{kind: "string"})
	}
	if n := tsNumber.FindString(rest); n != "" {
		p.at += len(n)
		return one(tsMember{kind: "number"})
	}
	word := tsWord.FindString(rest)
	if word == "" {
		return nil, fmt.Errorf("the type %q has %q where a type was expected", p.text, rest)
	}
	p.at += len(word)
	switch word {
	case "string", "number", "boolean", "null", "void":
		return one(tsMember{kind: word})
	case "undefined":
		return one(tsMember{kind: "null"})
	case "true", "false":
		return one(tsMember{kind: "boolean"})
	case "unknown", "any":
		return one(tsMember{kind: "any"})
	}
	if !p.takes("<") {
		return one(tsMember{kind: "ref", name: word})
	}
	var args []*tsType
	for {
		a, err := p.union()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.takes(">") {
			break
		}
		if !p.takes(",") {
			return nil, fmt.Errorf("the type %q has a list of types that does not close", p.text)
		}
	}
	switch {
	case word == "Array" && len(args) == 1:
		return one(tsMember{kind: "array", elem: args[0]})
	case word == "Record" && len(args) == 2:
		return one(tsMember{kind: "record", elem: args[1]})
	case word == "Partial" && len(args) == 1 && len(args[0].members) == 1 && args[0].members[0].kind == "ref":
		return one(tsMember{kind: "ref", name: args[0].members[0].name, partial: true})
	}
	return nil, fmt.Errorf("the type %q uses %s<>, which this reader does not know", p.text, word)
}

// fields reads name: type pairs, each ended by a semicolon, a comma or
// the end of a line.
func (p *tsParser) fields() (map[string]tsField, error) {
	out := map[string]tsField{}
	for {
		p.space()
		for p.takes(";") || p.takes(",") {
		}
		if p.at >= len(p.text) {
			return out, nil
		}
		name := tsWord.FindString(p.text[p.at:])
		if name == "" {
			return nil, fmt.Errorf("a field has no name at %q", p.text[p.at:])
		}
		p.at += len(name)
		optional := p.takes("?")
		if !p.takes(":") {
			return nil, fmt.Errorf("the field %s has no type, or is a method, which this reader does not know", name)
		}
		t, err := p.union()
		if err != nil {
			return nil, fmt.Errorf("the field %s: %w", name, err)
		}
		out[name] = tsField{optional: optional, of: t}
	}
}
