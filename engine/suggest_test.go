package engine

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// suggestCases are the cases the engine and the workspace are both held
// to, in frontend/src/lib/suggest.cases.json, so the two never drift apart.
type suggestCases struct {
	Windows []struct {
		Duration float64 `json:"duration"`
		Window   float64 `json:"window"`
	} `json:"windows"`
	Counts []struct {
		Window float64 `json:"window"`
		Least  float64 `json:"least"`
		Most   float64 `json:"most"`
		Count  int     `json:"count"`
	} `json:"counts"`
	Thinks []struct {
		Window float64 `json:"window"`
		Think  int     `json:"think"`
	} `json:"thinks"`
}

func readSuggestCases(t *testing.T) suggestCases {
	t.Helper()
	data, err := os.ReadFile("../frontend/src/lib/suggest.cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c suggestCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Windows) == 0 || len(c.Counts) == 0 || len(c.Thinks) == 0 {
		t.Fatal("no cases")
	}
	return c
}

func TestSuggestedWindow(t *testing.T) {
	for _, c := range readSuggestCases(t).Windows {
		if got := SuggestedWindow(c.Duration); math.Abs(got-c.Window) > 0.5 {
			t.Errorf("an episode of %s: windows of %s, want %s", HMS(c.Duration), HMS(got), HMS(c.Window))
		}
	}
}

// The windows divide the episode evenly and none is shorter than the
// least, whatever the episode's length.
func TestSuggestedWindowsDivideEvenly(t *testing.T) {
	for d := 1.0; d < 6*3600; d += 37 {
		w := SuggestedWindow(d)
		n := d / w
		if math.Abs(n-math.Round(n)) > 1e-6 {
			t.Fatalf("%s: %s windows", HMS(d), trimFloat(n))
		}
		if d > leastWindow && w < leastWindow-1e-6 {
			t.Fatalf("%s: windows of %s", HMS(d), HMS(w))
		}
	}
}

func TestSuggestedCount(t *testing.T) {
	for _, c := range readSuggestCases(t).Counts {
		if got := SuggestedCount(c.Window, c.Least, c.Most); got != c.Count {
			t.Errorf("%s at %s to %s s: %d clips, want %d", HMS(c.Window), trimFloat(c.Least),
				trimFloat(c.Most), got, c.Count)
		}
	}
}

func TestSuggestedThink(t *testing.T) {
	for _, c := range readSuggestCases(t).Thinks {
		if got := SuggestedThink(c.Window); got != c.Think {
			t.Errorf("%s: %d tokens, want %d", HMS(c.Window), got, c.Think)
		}
	}
}
