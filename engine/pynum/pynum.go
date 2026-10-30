// Package pynum rounds, sums, reads and writes numbers the way the Python
// version of the engine did: compensated float sums, halves rounded to
// even, and a float always written with a decimal point. They give the
// same answer on every platform, which keeps plans reproducible. It
// imports nothing of the engine but pytext, see docs/ARCHITECTURE.md.
//
// A note on arm64, which is what Apple Silicon runs. The Go compiler may fuse
// a multiplication and an addition into one instruction there, and a fused
// result can round differently from Python, which computes them one after
// the other. An explicit float64() conversion prevents the fusion, which is
// why some products in the engine are wrapped in one that looks redundant.
package pynum

import (
	"math"
	"strconv"
	"strings"

	"framefairy/engine/pytext"
)

// Sum is Python's sum() over floats. Since 3.12 it uses Neumaier's
// compensated summation rather than adding left to right, and the difference
// is enough to change a tie.
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

// Round is Python's round(x), which rounds halves to even.
func Round(x float64) int { return int(math.RoundToEven(x)) }

// RoundTo is Python's round(x, n): the nearest value with n decimals.
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

// Fixed formats like Python's f"{x:.Nf}".
func Fixed(x float64, n int) string { return strconv.FormatFloat(x, 'f', n, 64) }

// IsFinite says whether x is neither NaN nor infinite.
func IsFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// FloatRepr is Python's repr() of a float.
func FloatRepr(x float64) string {
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

// ParseFloat is Python's float() on a str. The second result is false
// where Python would raise.
func ParseFloat(text string) (float64, bool) {
	t := strings.ReplaceAll(pytext.Strip(text), "_", "")
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

// Float marshals to JSON the way Python writes a float, always with a
// decimal point, so a plan written by either version reads the same.
type Float float64

// MarshalJSON implements json.Marshaler.
func (f Float) MarshalJSON() ([]byte, error) {
	x := float64(f)
	if !IsFinite(x) {
		return []byte("null"), nil
	}
	return []byte(FloatRepr(x)), nil
}
