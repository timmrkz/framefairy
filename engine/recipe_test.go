package engine

import (
	"fmt"
	"testing"
)

// The default recipe asks exactly what was always asked, so a saved reply
// is found again and a search made before recipes existed reads the same.
func TestTheDefaultRecipeAsksWhatWasAlwaysAsked(t *testing.T) {
	r, err := RecipeNamed("")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != DefaultRecipe || r.System != SystemPrompt || r.Version != PromptVersion {
		t.Fatalf("the default recipe is %s, version %d", r.Name, r.Version)
	}
	lines := []Line{
		{Index: 1, Cues: []Cue{{Start: 0, End: 1, Text: "Hallo"}}},
		{Index: 2, Cues: []Cue{{Start: 1.8, End: 2.4, Text: "Welt."}}, GapBefore: 0.8},
	}
	opts := PlanOptions{Count: 3, MinLen: 20, MaxLen: 30}
	if got, want := r.Request(lines, r.units(lines), opts), buildPrompt(lines, opts); got != want {
		t.Errorf("the default recipe asks something new:\n%s\nrather than\n%s", got, want)
	}
	if got, want := r.Schema(2, 3), planSchema(2, 3); got != want {
		t.Error("the default recipe's answer has a new shape")
	}
}

func TestAnUnknownRecipeIsRefusedByName(t *testing.T) {
	if _, err := RecipeNamed("nonsense"); err == nil {
		t.Fatal("a recipe that does not exist was taken")
	}
	// Options that name one anyway are searched with the default, rather
	// than with nothing. The name is refused where it is given.
	if got := (PlanOptions{Recipe: "nonsense"}).recipe().Name; got != DefaultRecipe {
		t.Errorf("got %s", got)
	}
}

// A recipe may number runs of lines, sentences say. What the model answers
// in those numbers is read back into the lines they stand for.
func TestAnAnswerInUnitsBecomesLines(t *testing.T) {
	// Three units over seven lines: 1-2, 3-5, 6-7.
	units := [][2]int{{0, 1}, {2, 4}, {5, 6}}
	answer := map[string]any{"slug": "a", "title": "t", "reason": "r",
		"keep": []any{[]any{1, 1}, []any{3, 3}}}
	entry, problems, ok := readEntry(answer, 1, units)
	if !ok || len(problems) > 0 {
		t.Fatalf("not read: %v", problems)
	}
	if got := fmt.Sprint(entry.Keep); got != "[[1 2] [6 7]]" {
		t.Errorf("units 1 and 3 became lines %s", got)
	}
	// A unit that does not exist is not a line either.
	answer["keep"] = []any{[]any{1, 4}}
	if _, _, ok := readEntry(answer, 1, units); ok {
		t.Error("unit 4 of 3 was taken")
	}
}
