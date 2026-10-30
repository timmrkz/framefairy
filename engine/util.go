package engine

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"

	"framefairy/engine/pynum"
	"framefairy/engine/pytext"
)

// These helpers read and write the values of a plan's JSON and a command
// the way the Python version did. The numbers and the text they rest on
// are in engine/pynum and engine/pytext. The values move with the plan
// file and shellQuote with running programs, see
// docs/decisions/2026-10-30-numbers-and-text-in-packages.md.

// pyStr is Python's str() for values that came out of JSON.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case json.Number:
		return x.String()
	case float64:
		return pynum.FloatRepr(x)
	case int:
		return strconv.Itoa(x)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

// toFloat is Python's float() for values that came out of JSON. The second
// result is false where Python would raise.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case json.Number:
		f, err := strconv.ParseFloat(x.String(), 64)
		return f, err == nil
	case string:
		return pynum.ParseFloat(x)
	}
	return 0, false
}

// toInt is Python's int() for values that came out of JSON. A float is
// truncated, a string must hold a whole number.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		return int(x), true
	case int:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case json.Number:
		if n, err := strconv.Atoi(x.String()); err == nil {
			return n, true
		}
		f, err := strconv.ParseFloat(x.String(), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return int(f), true
	case string:
		n, err := strconv.Atoi(strings.ReplaceAll(pytext.Strip(x), "_", ""))
		return n, err == nil
	}
	return 0, false
}

// pyListRepr formats a list of strings the way Python prints one.
func pyListRepr(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "'" + item + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// pyRepr formats a string the way Python's repr() usually does.
func pyRepr(s string) string {
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

// shellQuote is Python's shlex.quote, used when printing commands.
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
