// Package num rounds, sums, reads and writes the numbers of a video's
// files: compensated float sums, halves rounded to even, and a float
// always written with a decimal point and never with an exponent. They
// give the same answer on every platform, which keeps plans reproducible,
// and they read every file written so far the same way. It imports
// nothing of the engine but runes, see docs/ARCHITECTURE.md.
package num

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"framefairy/engine/runes"
)

// Sum adds with Neumaier's compensated summation rather than left to
// right, and the difference is enough to change a tie.
func Sum(values []float64) float64 {
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

// Round rounds to the nearest whole number, halves to even.
func Round(x float64) int { return int(math.RoundToEven(x)) }

// RoundTo is the nearest value with n decimals, halves to even.
func RoundTo(x float64, n int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', n, 64), 64)
	if err != nil {
		return x
	}
	return v
}

// Fixed writes x with exactly n decimals.
func Fixed(x float64, n int) string { return strconv.FormatFloat(x, 'f', n, 64) }

// IsFinite says whether x is neither NaN nor infinite.
func IsFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Format writes x in the fewest digits that read back as x, always with
// a decimal point and never with an exponent, and nan, inf and -inf.
func Format(x float64) string {
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

// Parse reads a number from text, with space around it, underscores
// between its digits, and inf, infinity and nan in any case. The second
// result is false when the text holds no number.
func Parse(text string) (float64, bool) {
	t := strings.ReplaceAll(runes.Trim(text), "_", "")
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

// Float marshals to JSON the way Format writes it, always with a decimal
// point, so every plan written so far reads the same.
type Float float64

// MarshalJSON implements json.Marshaler.
func (f Float) MarshalJSON() ([]byte, error) {
	x := float64(f)
	if !IsFinite(x) {
		return []byte("null"), nil
	}
	return []byte(Format(x)), nil
}

// FormatValue writes a value that came out of JSON for a prompt or a
// message: nil as None and a boolean as True or False, as the prompts
// have always read them.
func FormatValue(v any) string {
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
		return Format(x)
	case int:
		return strconv.Itoa(x)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(raw)
}

// AsFloat reads a number from a value that came out of JSON: a number, a
// boolean as 1 or 0, or text that Parse reads. The second result is false
// for anything else.
func AsFloat(v any) (float64, bool) {
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
		return Parse(x)
	}
	return 0, false
}

// AsInt reads a whole number from a value that came out of JSON. A float
// is truncated, a boolean is 1 or 0, and text must hold a whole number.
func AsInt(v any) (int, bool) {
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
		n, err := strconv.Atoi(strings.ReplaceAll(runes.Trim(x), "_", ""))
		return n, err == nil
	}
	return 0, false
}
