package num

import (
	"encoding/json"
	"math"
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
	}
	for _, c := range cases {
		if got := RoundTo(c.x, c.n); got != c.want {
			t.Errorf("RoundTo(%v, %d) = %v, want %v", c.x, c.n, got, c.want)
		}
	}
	if got := RoundTo(math.Inf(1), 2); !math.IsInf(got, 1) {
		t.Errorf("RoundTo(inf, 2) = %v, want inf", got)
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
