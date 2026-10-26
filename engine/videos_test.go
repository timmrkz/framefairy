package engine

import (
	"strings"
	"testing"
)

// The box's filter and the words the app shows are made from
// VideoExtensions, every ending in its order, so neither can name a file
// the list does not.
func TestVideoPatternAndNames(t *testing.T) {
	for _, c := range []struct {
		list          []string
		pattern, name string
	}{
		{[]string{".a"}, "*.a", "a"},
		{[]string{".a", ".b"}, "*.a;*.b", "a or b"},
		{[]string{".a", ".b", ".c"}, "*.a;*.b;*.c", "a, b or c"},
	} {
		if got := videoPattern(c.list); got != c.pattern {
			t.Errorf("videoPattern(%v) = %q, want %q", c.list, got, c.pattern)
		}
		if got := videoNames(c.list); got != c.name {
			t.Errorf("videoNames(%v) = %q, want %q", c.list, got, c.name)
		}
	}
	if VideoPattern() != videoPattern(VideoExtensions) || VideoNames() != videoNames(VideoExtensions) {
		t.Error("the filter and the words are not made from VideoExtensions")
	}
	for _, ext := range VideoExtensions {
		if !IsVideo("ep" + strings.ToUpper(ext)) {
			t.Errorf("ep%s is no video, want one whatever the case", strings.ToUpper(ext))
		}
	}
}
