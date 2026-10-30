package pytext

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextIsCountedInCharacters(t *testing.T) {
	if got := Len("Grüße"); got != 5 {
		t.Errorf("Len(Grüße) = %d, Python says 5", got)
	}
	for n, want := range map[int]string{-1: "", 0: "", 2: "Gr", 3: "Grü", 9: "Grüße"} {
		if got := Prefix("Grüße", n); got != want {
			t.Errorf("Prefix(Grüße, %d) = %q, Python says %q", n, got, want)
		}
	}
}

func TestWhitespaceIsPythons(t *testing.T) {
	// \x1c to \x1f are whitespace to Python and not to Go.
	if got := Strip("\x1c a b \x1f\n"); got != "a b" {
		t.Errorf("Strip = %q, Python says %q", got, "a b")
	}
	if got := Fields("a\x1db c"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("Fields = %q, Python says [a b c]", got)
	}
}

func TestScrubLeavesOnlyPrintableText(t *testing.T) {
	cases := map[string]string{
		"plain words":             "plain words",
		"\x1b[2Jcleared\x07":      "[2Jcleared",
		"right‮to left​ here":     "rightto left here",
		"two\n\nlines\tand\ttabs": "two lines and tabs",
		"bad \xff byte":           "bad � byte",
	}
	for in, want := range cases {
		if got := Scrub(in, 100); got != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Scrub("ä b c d", 3); got != "ä b" {
		t.Errorf("Scrub cut to 3 = %q, want %q", got, "ä b")
	}
}

func TestReprQuotesAsPythonDoes(t *testing.T) {
	for in, want := range map[string]string{"plain": "'plain'", "it's": `"it's"`, `a\b`: `'a\\b'`, `say "hi"`: `'say "hi"'`} {
		if got := Repr(in); got != want {
			t.Errorf("Repr(%q) = %s, Python says %s", in, got, want)
		}
	}
	if got := ListRepr([]string{"a", "b"}); got != "['a', 'b']" {
		t.Errorf("ListRepr = %s, Python says ['a', 'b']", got)
	}
}

// Scrub reads what a model or the network sent, so it is fuzzed: whatever
// comes in, what comes out is valid, free of controls, on one line and no
// longer than asked. A cut may end on a space, as Python's [:limit] does.
func FuzzScrub(f *testing.F) {
	for _, seed := range []string{"", "a b", "\x1b[31mred", "‮⁦x", "\xff\xfe", "line break"} {
		f.Add(seed, 8)
	}
	f.Fuzz(func(t *testing.T, text string, limit int) {
		got := Scrub(text, limit)
		if !utf8.ValidString(got) {
			t.Fatalf("Scrub(%q) = %q, not valid UTF-8", text, got)
		}
		if strings.ContainsFunc(got, IsControl) || strings.ContainsAny(got, "\n\r\t") {
			t.Fatalf("Scrub(%q) = %q, still has a control or a break", text, got)
		}
		if Len(got) > max(limit, 0) {
			t.Fatalf("Scrub(%q, %d) = %q, longer than asked", text, limit, got)
		}
	})
}
