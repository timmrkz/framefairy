package pynum

import (
	"encoding/json"
	"math"
	"testing"
)

// Each case is what Python 3.12 answers, so a plan written by the Python
// version reads and rounds the same here. Where the two differ on purpose,
// the package comment says so, and no case tests it.

func TestRoundTakesHalvesToEven(t *testing.T) {
	for x, want := range map[float64]int{0.5: 0, 1.5: 2, 2.5: 2, -0.5: 0, -1.5: -2, 2.4: 2, 2.6: 3} {
		if got := Round(x); got != want {
			t.Errorf("Round(%v) = %d, Python says %d", x, got, want)
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
			t.Errorf("RoundTo(%v, %d) = %v, Python says %v", c.x, c.n, got, c.want)
		}
	}
	if got := RoundTo(math.Inf(1), 2); !math.IsInf(got, 1) {
		t.Errorf("RoundTo(inf, 2) = %v, want inf", got)
	}
}

func TestSumCompensatesLikePython(t *testing.T) {
	// Left to right this is 0.0. Python 3.12's sum([1.0, 1e-16, -1.0])
	// gives 1e-16 back.
	if got := Sum([]float64{1, 1e-16, -1}); got != 1e-16 {
		t.Errorf("Sum = %v, Python says 1e-16", got)
	}
	if got := Sum([]float64{0.1, 0.2, 0.3}); got != 0.6 {
		t.Errorf("Sum = %v, Python says 0.6", got)
	}
}

func TestFloatsAreWrittenAsPythonWritesThem(t *testing.T) {
	for x, want := range map[float64]string{1: "1.0", 0.1: "0.1", -2.5: "-2.5", 1e15: "1000000000000000.0"} {
		if got := FloatRepr(x); got != want {
			t.Errorf("FloatRepr(%v) = %s, want %s", x, got, want)
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

func TestParseFloatReadsWhatPythonReads(t *testing.T) {
	for text, want := range map[string]float64{" 1.5\n": 1.5, "1_000": 1000, "-Infinity": math.Inf(-1)} {
		got, ok := ParseFloat(text)
		if !ok || got != want {
			t.Errorf("ParseFloat(%q) = %v, %v, want %v", text, got, ok, want)
		}
	}
	if got, ok := ParseFloat("nan"); !ok || !math.IsNaN(got) {
		t.Errorf("ParseFloat(nan) = %v, %v, want NaN", got, ok)
	}
	for _, text := range []string{"", "1,5", "one"} {
		if _, ok := ParseFloat(text); ok {
			t.Errorf("ParseFloat(%q) read a number where Python raises", text)
		}
	}
	if IsFinite(math.Inf(1)) || IsFinite(math.NaN()) || !IsFinite(0) {
		t.Error("IsFinite is wrong for inf, NaN or 0")
	}
}

func TestJSONValuesReadAndWriteAsPythonDoes(t *testing.T) {
	for v, want := range map[any]float64{2.5: 2.5, 3: 3, true: 1, " 4.5 ": 4.5, json.Number("6.25"): 6.25} {
		if got, ok := FloatOf(v); !ok || got != want {
			t.Errorf("FloatOf(%#v) = %v, %v, Python's float() says %v", v, got, ok, want)
		}
	}
	for v, want := range map[any]int{2.9: 2, -2.9: -2, 7: 7, false: 0, " 12 ": 12, json.Number("8"): 8} {
		if got, ok := IntOf(v); !ok || got != want {
			t.Errorf("IntOf(%#v) = %v, %v, Python's int() says %v", v, got, ok, want)
		}
	}
	for _, v := range []any{nil, "x", "2.5", math.NaN(), []any{1}} {
		if _, ok := IntOf(v); ok {
			t.Errorf("IntOf(%#v) read a number where Python raises", v)
		}
	}
	for v, want := range map[any]string{nil: "None", true: "True", 1.0: "1.0", 2.5: "2.5", 3: "3", "word": "word"} {
		if got := Str(v); got != want {
			t.Errorf("Str(%#v) = %q, Python's str() says %q", v, got, want)
		}
	}
}
