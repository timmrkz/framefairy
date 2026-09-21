package engine

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// These helpers started as a way to reproduce the Python version exactly:
// character counts rather than bytes, compensated float sums and rounding
// halves to even. They stay because they give the same answer on every
// platform, which keeps plans reproducible.

// runeLen is Python's len() on a str. Go's len() counts bytes, which would
// treat every umlaut as two characters and move caption breaks.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// runePrefix is Python's s[:n].
func runePrefix(s string, n int) string {
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

// pysum is Python's sum() over floats. Since 3.12 it uses Neumaier's
// compensated summation rather than adding left to right, and the difference
// is enough to change a tie.
func pysum(values []float64) float64 {
	total, c := 0.0, 0.0
	for _, x := range values {
		t := total + x
		if math.Abs(total) >= math.Abs(x) {
			c += (total - t) + x
		} else {
			c += (x - t) + total
		}
		total = t
	}
	if c != 0 && !math.IsInf(c, 0) && !math.IsNaN(c) {
		total += c
	}
	return total
}

// pyround is Python's round(x), which rounds halves to even.
func pyround(x float64) int { return int(math.RoundToEven(x)) }

// roundTo is Python's round(x, n): the nearest value with n decimals.
func roundTo(x float64, n int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', n, 64), 64)
	if err != nil {
		return x
	}
	return v
}

// fixed formats like Python's f"{x:.Nf}".
func fixed(x float64, n int) string { return strconv.FormatFloat(x, 'f', n, 64) }

// fields is Python's str.split() with no argument.
func fields(s string) []string { return strings.FieldsFunc(s, isPySpace) }

func isPySpace(r rune) bool {
	switch r {
	case '\x1c', '\x1d', '\x1e', '\x1f':
		return true
	}
	return unicode.IsSpace(r)
}

// strip is Python's str.strip() with no argument.
func strip(s string) string { return strings.TrimFunc(s, isPySpace) }

// isControl matches the characters the Python version removed with _CONTROL:
// C0 and C1 controls other than tab, newline and carriage return, zero width
// and bidi marks, line and paragraph separators, bidi embeddings, overrides
// and isolates, and the zero width no-break space.
func isControl(r rune) bool {
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
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
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
	cleaned := removeControl(strings.ToValidUTF8(text, "\uFFFD"))
	cleaned = strings.Join(fields(cleaned), " ")
	return runePrefix(cleaned, limit)
}

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
		return pyFloatRepr(x)
	case int:
		return strconv.Itoa(x)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

func pyFloatRepr(x float64) string {
	switch {
	case math.IsNaN(x):
		return "nan"
	case math.IsInf(x, 1):
		return "inf"
	case math.IsInf(x, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(x, 'f', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
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
		return parsePyFloat(x)
	}
	return 0, false
}

func parsePyFloat(text string) (float64, bool) {
	t := strings.ReplaceAll(strip(text), "_", "")
	switch strings.ToLower(t) {
	case "inf", "+inf", "infinity", "+infinity":
		return math.Inf(1), true
	case "-inf", "-infinity":
		return math.Inf(-1), true
	case "nan", "+nan", "-nan":
		return math.NaN(), true
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return f, true
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
		n, err := strconv.Atoi(strings.ReplaceAll(strip(x), "_", ""))
		return n, err == nil
	}
	return 0, false
}

func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

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

// PyFloat marshals to JSON the way Python writes a float, always with a
// decimal point, so a plan written by either version reads the same.
type PyFloat float64

// MarshalJSON implements json.Marshaler.
func (f PyFloat) MarshalJSON() ([]byte, error) {
	x := float64(f)
	if !isFinite(x) {
		return []byte("null"), nil
	}
	return []byte(pyFloatRepr(x)), nil
}

func medianIndex(n int) int { return n / 2 }

// A note on arm64, which is what Apple Silicon runs. The Go compiler may fuse
// a multiplication and an addition into one instruction there, and a fused
// result can round differently from Python, which computes them one after
// the other. An explicit float64() conversion prevents the fusion, which is
// why some products in this package are wrapped in one that looks redundant.
