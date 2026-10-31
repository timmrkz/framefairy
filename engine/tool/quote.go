package tool

import (
	"strings"
	"unicode"
)

// Quote puts s in single quotes unless it holds only letters, digits and
// the characters a shell leaves alone, for printing a command a person
// can paste into a shell.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) ||
			strings.ContainsRune("@%+=:,./-_", r))) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
