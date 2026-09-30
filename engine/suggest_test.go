package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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

// A search given no count and no thinking budget looks for the clips its
// window suggests and thinks what its window suggests, and says so in its
// plan.
func TestASearchTakesWhatItsWindowSuggests(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var heard int32
	var mu sync.Mutex
	var budgets []float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		if b, ok := request["reasoning_budget_tokens"].(float64); ok {
			mu.Lock()
			budgets = append(budgets, b)
			mu.Unlock()
		}
		writeLocalStream(w, `{"clips": [{"slug": "erste", "title": "Erste", "reason": "Test", "keep": [[1, 1]]}]}`, 7)
	}))
	defer server.Close()

	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	if base.Count != 0 || base.Think != ThinkForWindow {
		t.Fatalf("the defaults are %d clips and %d tokens", base.Count, base.Think)
	}
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Min, base.Max = 5, 10
	p := NewProject(e, source, base)
	path, err := p.Plan(context.Background(), PlanRequest{From: 0, To: 30})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	plan, _, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := toFloat(plan.PlannedWith()["count"]); int(got) != SuggestedCount(30, 5, 10) {
		t.Errorf("the plan asked for %v clips", plan.PlannedWith()["count"])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(budgets) == 0 || int(budgets[0]) != SuggestedThink(30) {
		t.Errorf("the model was given %v tokens to think, want %d", budgets, SuggestedThink(30))
	}
}
