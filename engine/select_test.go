package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Everything in this file works on what a model sent back. That text is the
// only thing standing between a good episode and a bad set of clips, and it
// arrives as whatever the model felt like writing, so both the recovery and
// the refusal are worth pinning down.

func TestExtractJSONObjectFindsThePlan(t *testing.T) {
	plan := `{"clips": [{"slug": "a", "keep": [[1, 2]]}]}`
	cases := []struct {
		name, reply, note string
	}{
		{"plain", plan, ""},
		{"prose around it", "Sure! Here you go:\n" + plan + "\nHope that helps.",
			"extracted from surrounding prose"},
		{"fenced", "```json\n" + plan + "\n```", ""},
		{"fenced without a tag", "```\n" + plan + "\n```", ""},
		{"a brace inside a string",
			`{"clips": [{"slug": "a", "title": "a {b} c", "keep": [[1, 2]]}]}`, ""},
		{"an object before it without the key",
			`{"thinking": "let me see"}\n` + plan, "extracted from surrounding prose"},
		{"cut off mid reply",
			`{"clips": [{"slug": "a", "keep": [[1, 2]]}, {"slug": "b", "keep`,
			"reply was cut off, recovered what was complete"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, note, err := ExtractJSONObject(c.reply, "clips")
			if err != nil {
				t.Fatalf("%v", err)
			}
			if _, ok := data["clips"].([]any); !ok {
				t.Fatalf("no clips list in %v", data)
			}
			if note != c.note {
				t.Errorf("note %q, want %q", note, c.note)
			}
		})
	}

	// The first complete object wins only when it holds the key, so a model
	// that thinks out loud in JSON first still gets read correctly.
	data, _, err := ExtractJSONObject(`{"plan": 1}`+"\n"+plan, "clips")
	if err != nil || len(data["clips"].([]any)) != 1 {
		t.Errorf("two objects: %v %v", data, err)
	}
	// The longest fenced block is the payload, not a short example above it.
	data, _, err = ExtractJSONObject("```\n{\"clips\": []}\n```\nand now really:\n```json\n"+
		plan+"\n```", "clips")
	if err != nil || len(data["clips"].([]any)) != 1 {
		t.Errorf("two fences: %v %v", data, err)
	}
}

func TestExtractJSONObjectRefusesWhatItCannotRead(t *testing.T) {
	for _, reply := range []string{
		"",
		"I could not find a good clip in this episode.",
		`{"clips": }`,
		`{"other": [1]}`,
		"{{{{",
	} {
		if _, _, err := ExtractJSONObject(reply, "clips"); err == nil {
			t.Errorf("accepted %q", reply)
		}
	}
	// The message shows the start of the reply, scrubbed, so a reply full of
	// escape codes cannot repaint the terminal it is reported in.
	_, _, err := ExtractJSONObject("\x1b[2Jno plan here", "clips")
	if err == nil || strings.Contains(err.Error(), "\x1b") {
		t.Errorf("error text: %v", err)
	}
}

func planWith(keep string) map[string]any {
	data, _, err := ExtractJSONObject(`{"clips": [{"slug": "a", "title": "t", "reason": "r", `+
		`"keep": `+keep+`}]}`, "clips")
	if err != nil {
		panic(err)
	}
	return data
}

func TestValidatePlanRefusesRangesThatAreNotLines(t *testing.T) {
	cases := []struct {
		name, keep string
	}{
		{"past the end", "[[1, 99]]"},
		{"below one", "[[0, 2]]"},
		{"backwards", "[[5, 3]]"},
		{"not a pair", "[[1, 2, 3]]"},
		{"not numeric", `[["one", "two"]]`},
		{"empty", "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ValidatePlan(planWith(c.keep), 10); err == nil {
				t.Errorf("accepted %s", c.keep)
			}
		})
	}
	// A clip with one bad range among good ones keeps the good ones and says
	// what was wrong, rather than throwing the whole answer away.
	good, problems, err := ValidatePlan(planWith("[[1, 2], [99, 100], [4, 5]]"), 10)
	if err != nil || len(good) != 1 || len(good[0].Keep) != 2 || len(problems) != 1 {
		t.Fatalf("clips %v, problems %v, err %v", good, problems, err)
	}
	if good[0].Keep[0] != [2]int{1, 2} || good[0].Keep[1] != [2]int{4, 5} {
		t.Errorf("kept %v", good[0].Keep)
	}
	// Two runs may touch, which is how the model cuts the pause between them.
	if _, _, err := ValidatePlan(planWith("[[1, 2], [3, 4]]"), 10); err != nil {
		t.Errorf("touching runs: %v", err)
	}
	// A range that overlaps the one before it, or goes backwards in the
	// transcript, is dropped and what is left still runs forwards. Keeping
	// both would play a part twice.
	for _, keep := range []string{"[[1, 4], [3, 6]]", "[[5, 6], [1, 2]]"} {
		good, problems, err := ValidatePlan(planWith(keep), 10)
		if err != nil || len(good) != 1 || len(problems) != 1 {
			t.Fatalf("%s: clips %v, problems %v, err %v", keep, good, problems, err)
		}
		if len(good[0].Keep) != 1 {
			t.Errorf("%s kept %v", keep, good[0].Keep)
		}
	}
}

func TestValidatePlanTamesTheTextItIsGiven(t *testing.T) {
	long := strings.Repeat("ü", 400)
	data, _, err := ExtractJSONObject(`{"clips": [`+
		`{"slug": "Erste Wahl", "title": "a`+`\u001b`+`b", "reason": "`+long+`", "keep": [[1, 2]]},`+
		`{"slug": "Erste Wahl", "title": "zwei", "reason": "r", "keep": [[3, 4]]}]}`, "clips")
	if err != nil {
		t.Fatal(err)
	}
	good, problems, err := ValidatePlan(data, 10)
	if err != nil || len(good) != 2 {
		t.Fatalf("clips %v, err %v", good, err)
	}
	if strings.ContainsRune(good[0].Title, '\x1b') {
		t.Errorf("control character kept: %q", good[0].Title)
	}
	if runeLen(good[0].Reason) != 300 {
		t.Errorf("reason is %d characters", runeLen(good[0].Reason))
	}
	if good[0].Slug == good[1].Slug {
		t.Errorf("both clips are called %q, their files would overwrite each other", good[0].Slug)
	}
	if len(problems) == 0 {
		t.Errorf("the repeated slug was not reported")
	}
}

// FuzzExtractJSONObject checks that no reply, however mangled, makes the
// reader panic, and that anything it hands back really is an object with the
// key that was asked for.
func FuzzExtractJSONObject(f *testing.F) {
	f.Add(`{"clips": [{"slug": "a", "keep": [[1, 2]]}]}`)
	f.Add("prose ```json\n{\"clips\": []}\n``` more")
	f.Add(`{"clips": [{"keep": [[1, 2]]}, {"keep`)
	f.Add(`{"a": "}", "clips": [1]}`)
	f.Add("\x1b[2J{{{[[[\"\\")
	f.Fuzz(func(t *testing.T, reply string) {
		data, note, err := ExtractJSONObject(reply, "clips")
		if err != nil {
			if data != nil {
				t.Errorf("data came back with an error")
			}
			return
		}
		if _, present := data["clips"]; !present {
			t.Fatalf("no clips key in %v", data)
		}
		if _, err := json.Marshal(data); err != nil {
			t.Errorf("what came back does not re-encode: %v", err)
		}
		if note != "" && !strings.Contains(note, "prose") && !strings.Contains(note, "cut off") {
			t.Errorf("unexpected note %q", note)
		}
	})
}

// FuzzValidatePlan is the important one. Whatever the model sends, every clip
// that comes out has to be usable: ranges that name real lines, in order,
// without overlap, and a slug that is safe to put in a file name.
func FuzzValidatePlan(f *testing.F) {
	f.Add(`{"clips": [{"slug": "a", "keep": [[1, 2]]}]}`, 10)
	f.Add(`{"clips": [{"slug": "a", "keep": [[1, 2]]}, {"slug": "a", "keep": [[2, 3]]}]}`, 3)
	f.Add(`{"clips": [{"keep": [[1, 1e9]]}]}`, 5)
	f.Add(`{"clips": [{"slug": 1, "title": null, "keep": [["2", 3.7]]}]}`, 20)
	f.Add(`{"clips": [{"slug": "a", "keep": [[1, 2], [2, 4]]}]}`, 8)
	f.Add(`{"clips": [{"keep": [[1, 2]]}, {"keep": [[3, 4]]}]}`, 8)
	f.Fuzz(func(t *testing.T, reply string, lineCount int) {
		if lineCount < 1 || lineCount > 10_000 {
			t.Skip()
		}
		data, _, err := ExtractJSONObject(reply, "clips")
		if err != nil {
			return
		}
		good, _, err := ValidatePlan(data, lineCount)
		if err != nil {
			return
		}
		if len(good) == 0 {
			t.Fatalf("no error and no clips")
		}
		seen := map[string]bool{}
		for _, clip := range good {
			if len(clip.Keep) == 0 {
				t.Fatalf("clip %q kept nothing", clip.Slug)
			}
			previous := 0
			for _, pair := range clip.Keep {
				switch {
				case pair[0] < 1 || pair[1] > lineCount:
					t.Fatalf("range %v is outside 1-%d", pair, lineCount)
				case pair[1] < pair[0]:
					t.Fatalf("range %v is backwards", pair)
				case pair[0] <= previous:
					t.Fatalf("range %v overlaps the one before it in %v", pair, clip.Keep)
				}
				previous = pair[1]
			}
			if runeLen(clip.Slug) > 64 || runeLen(clip.Title) > 200 || runeLen(clip.Reason) > 300 {
				t.Fatalf("text too long: %d %d %d",
					runeLen(clip.Slug), runeLen(clip.Title), runeLen(clip.Reason))
			}
			for _, field := range []string{clip.Slug, clip.Title, clip.Reason} {
				if strings.ContainsFunc(field, isControl) || strings.ContainsAny(field, "\n\r\t") {
					t.Fatalf("control characters survived in %s", fmt.Sprintf("%q", field))
				}
			}
			// A clip the model gave no usable name keeps none, and its file is
			// named after its number instead, so two of those are not a clash.
			// Two clips that do carry the same name would be.
			if clip.Slug != "" && seen[clip.Slug] {
				t.Fatalf("two clips called %q", clip.Slug)
			}
			seen[clip.Slug] = true
		}
	})
}
