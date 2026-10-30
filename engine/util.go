package engine

import (
	"strings"
	"unicode"
)

// shellQuote puts s in single quotes unless it holds only letters, digits
// and the characters a shell leaves alone, for printing a command. The
// numbers and text it once sat beside are in engine/num and engine/runes,
// and it moves with running programs, see
// docs/decisions/2026-10-30-numbers-and-text-in-packages.md.
func shellQuote(s string) string {
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
