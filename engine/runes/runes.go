// Package runes counts, cuts, trims and quotes text by its characters
// rather than its bytes, and scrubs text from outside before it is printed
// or stored. It imports nothing of the engine, see docs/ARCHITECTURE.md.
package runes

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Len counts the characters of s. Go's len() counts bytes, which would
// treat every umlaut as two characters and move caption breaks.
func Len(s string) int { return utf8.RuneCountInString(s) }

// Prefix is the first n characters of s.
func Prefix(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

// Fields splits s around each run of space, as IsSpace takes it.
func Fields(s string) []string { return strings.FieldsFunc(s, IsSpace) }

// IsSpace is Unicode's white space and the four separators \x1c to \x1f,
// which Go's unicode.IsSpace leaves out. Every file written so far was
// split and trimmed with them.
func IsSpace(r rune) bool {
	switch r {
	case '\x1c', '\x1d', '\x1e', '\x1f':
		return true
	}
	return unicode.IsSpace(r)
}

// Trim removes the space around s, as IsSpace takes it.
func Trim(s string) string { return strings.TrimFunc(s, IsSpace) }

// IsControl matches the characters Scrub removes: C0 and C1 controls other than tab, newline and carriage return, zero width
// and bidi marks, line and paragraph separators, bidi embeddings, overrides
// and isolates, and the zero width no-break space.
func IsControl(r rune) bool {
	switch {
	case r <= 0x08, r == 0x0b, r == 0x0c, r >= 0x0e && r <= 0x1f:
		return true
	case r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x200b && r <= 0x200f:
		return true
	case r == 0x2028, r == 0x2029:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0xfeff:
		return true
	}
	return false
}

func removeControl(s string) string {
	if !strings.ContainsFunc(s, IsControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// Scrub makes text from the network or the model safe to print and to store.
//
// Anything that arrives over the wire ends up in the terminal, in proof.txt
// and in the log files. Control characters there can repaint the screen or
// make a log read differently from what actually happened.
func Scrub(text string, limit int) string {
	cleaned := removeControl(strings.ToValidUTF8(text, "�"))
	cleaned = strings.Join(Fields(cleaned), " ")
	return Prefix(cleaned, limit)
}

// Quote puts s in single quotes, or in double quotes when it holds a
// single quote and no double one, for a prompt or a message.
func Quote(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = "\""
	}
	s = strings.ReplaceAll(s, "\\", "\\\\")
	if quote == "'" {
		s = strings.ReplaceAll(s, "'", "\\'")
	}
	return quote + s + quote
}

// QuoteList writes items in square brackets, each in single quotes.
func QuoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "'" + item + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
