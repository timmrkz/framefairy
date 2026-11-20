package num

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
)

// Each case is how the files written so far round, sum, read and write
// their numbers, so they read the same here.

func TestRoundTakesHalvesToEven(t *testing.T) {
	for x, want := range map[float64]int{0.5: 0, 1.5: 2, 2.5: 2, -0.5: 0, -1.5: -2, 2.4: 2, 2.6: 3} {
		if got := Round(x); got != want {
			t.Errorf("Round(%v) = %d, want %d", x, got, want)
		}
	}
}

func TestRoundToKeepsNDecimals(t *testing.T) {
	cases := []struct {
		x    float64
		n    int
		want float64
	}{
		{2.675, 2, 2.67}, // 2.675 is 2.67499999... as a float
		{1.005, 2, 1.0},
		{0.125, 2, 0.12},
		{12.3456, 1, 12.3},
		{-0.05, 1, -0.1},
		{12.3456, 3, 12.346},
		{3599.25, 1, 3599.2},
		{-3599.35, 1, -3599.3}, // -3599.35 is -3599.34999999999990905 as a float
	}
	for _, c := range cases {
		if got := RoundTo(c.x, c.n); got != c.want {
			t.Errorf("RoundTo(%v, %d) = %v, want %v", c.x, c.n, got, c.want)
		}
	}
	if got := RoundTo(math.Inf(1), 2); !math.IsInf(got, 1) {
		t.Errorf("RoundTo(inf, 2) = %v, want inf", got)
	}
	// Written out, a small negative number keeps its sign as -0.000.
	if got := RoundTo(-0.0001, 3); got != 0 || !math.Signbit(got) {
		t.Errorf("RoundTo(-0.0001, 3) = %v, want -0", got)
	}
}

// TestRoundToGivesWhatTheTextGives holds RoundTo to its definition, the
// number written with n decimals and read back, bit for bit. Every time
// the engine keeps is rounded so, and a plan or a transcript written
// before must read the same.
func TestRoundToGivesWhatTheTextGives(t *testing.T) {
	t.Parallel()
	byText := func(x float64, n int) float64 {
		v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', n, 64), 64)
		if err != nil {
			return x
		}
		return v
	}
	check := func(x float64, n int) {
		if !IsFinite(x) {
			return // kept as they are, see TestRoundToKeepsNDecimals
		}
		if want, got := byText(x, n), RoundTo(x, n); math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("RoundTo(%s, %d) = %v, written and read back %v", strconv.FormatFloat(x, 'g', -1, 64), n, got, want)
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 300_000 {
		n := rng.IntN(6)
		var x float64
		switch rng.IntN(5) {
		case 0: // any time in a long video
			x = rng.Float64() * 400000
		case 1: // a time already on the millisecond grid
			x = float64(rng.IntN(400000000)) / 1000
		case 2: // a half of the last decimal kept
			x = (float64(rng.IntN(4000000)) + 0.5) / math.Pow(10, float64(n))
		case 3: // any bit pattern
			x = math.Float64frombits(rng.Uint64())
		default:
			x = (rng.Float64() - 0.5) * 200
		}
		check(x, n)
	}
	for _, x := range []float64{0, math.Copysign(0, -1), -0.0001, 1.005, 2.675, 0.125, -0.125, 1e17, 5e-324, 4503599627370495.5, math.MaxFloat64} {
		for n := range 18 {
			check(x, n)
		}
	}
}

func TestSumCompensatesForWhatIsLost(t *testing.T) {
	// Left to right this is 0.0. The compensation gives 1e-16 back.
	if got := Sum([]float64{1, 1e-16, -1}); got != 1e-16 {
		t.Errorf("Sum = %v, want 1e-16", got)
	}
	if got := Sum([]float64{0.1, 0.2, 0.3}); got != 0.6 {
		t.Errorf("Sum = %v, want 0.6", got)
	}
}

func TestFloatsAreWrittenWithAPointAndNoExponent(t *testing.T) {
	for x, want := range map[float64]string{1: "1.0", 0.1: "0.1", -2.5: "-2.5", 1e15: "1000000000000000.0"} {
		if got := Format(x); got != want {
			t.Errorf("Format(%v) = %s, want %s", x, got, want)
		}
	}
	got, err := json.Marshal([]Float{1, 0.25, Float(math.NaN())})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[1.0,0.25,null]" {
		t.Errorf("JSON = %s, want [1.0,0.25,null]", got)
	}
	if Fixed(2.5, 0) != "2" || Fixed(0.1, 3) != "0.100" {
		t.Errorf("Fixed = %s and %s, want 2 and 0.100", Fixed(2.5, 0), Fixed(0.1, 3))
	}
}

func TestParseReadsNumbersInText(t *testing.T) {
	for text, want := range map[string]float64{" 1.5\n": 1.5, "1_000": 1000, "-Infinity": math.Inf(-1)} {
		got, ok := Parse(text)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %v, %v, want %v", text, got, ok, want)
		}
	}
	if got, ok := Parse("nan"); !ok || !math.IsNaN(got) {
		t.Errorf("Parse(nan) = %v, %v, want NaN", got, ok)
	}
	for _, text := range []string{"", "1,5", "one"} {
		if _, ok := Parse(text); ok {
			t.Errorf("Parse(%q) read a number where there is none", text)
		}
	}
	if IsFinite(math.Inf(1)) || IsFinite(math.NaN()) || !IsFinite(0) {
		t.Error("IsFinite is wrong for inf, NaN or 0")
	}
}

func TestJSONValuesAreReadAndWritten(t *testing.T) {
	for v, want := range map[any]float64{2.5: 2.5, 3: 3, true: 1, " 4.5 ": 4.5, json.Number("6.25"): 6.25} {
		if got, ok := AsFloat(v); !ok || got != want {
			t.Errorf("AsFloat(%#v) = %v, %v, want %v", v, got, ok, want)
		}
	}
	for v, want := range map[any]int{2.9: 2, -2.9: -2, 7: 7, false: 0, " 12 ": 12, json.Number("8"): 8} {
		if got, ok := AsInt(v); !ok || got != want {
			t.Errorf("AsInt(%#v) = %v, %v, want %v", v, got, ok, want)
		}
	}
	for _, v := range []any{nil, "x", "2.5", math.NaN(), []any{1}} {
		if _, ok := AsInt(v); ok {
			t.Errorf("AsInt(%#v) read a number where there is none", v)
		}
	}
	for v, want := range map[any]string{nil: "None", true: "True", 1.0: "1.0", 2.5: "2.5", 3: "3", "word": "word"} {
		if got := FormatValue(v); got != want {
			t.Errorf("FormatValue(%#v) = %q, want %q", v, got, want)
		}
	}
}

func TestThousandsAreGroupedByThree(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -45678: "-45,678"} {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %s, want %s", n, got, want)
		}
	}
}
