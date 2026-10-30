package engine

import (
	"strings"
	"unicode"
)

// shellQuote is Python's shlex.quote, used when printing commands. The
// numbers and text it once sat beside are in engine/pynum and
// engine/pytext, and it moves with running programs, see
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
