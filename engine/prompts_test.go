package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// Both prompt files fill in: the transcript, the count and the length are
// in them and nothing is left of the template.
func TestThePromptFilesFillIn(t *testing.T) {
	lines := said(0.0, 2.0, "Was ist deine erste Erinnerung?", 1.5, 3.0, "Der Regenschirm ist zersprungen.")
	opts := PlanOptions{Count: 6, MinLen: 20, MaxLen: 30}
	for _, recipe := range []Recipe{heartLeanRecipe, pointsRecipe} {
		text := recipe.Request(lines, recipe.units(lines), opts)
		for _, want := range []string{"Der Regenschirm ist zersprungen.", "up to 6 moments"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s has no %q:\n%s", recipe.Name, want, text)
			}
		}
		// Without switches nothing is said of seconds.
		if strings.Contains(text, "seconds") {
			t.Errorf("%s says a length:\n%s", recipe.Name, text)
		}
		if strings.Contains(text, "{{") || strings.Contains(text, "About the video") {
			t.Errorf("%s left something of its template:\n%s", recipe.Name, text)
		}
		if recipe.System != "" {
			t.Errorf("%s has a system part", recipe.Name)
		}
		// A context and lines that are clips already are said where the
		// file says them.
		with := opts
		with.Context = "Ein Podcast über erste Erinnerungen."
		with.Taken = []Window{{Start: 0, End: 2}}
		text = recipe.Request(lines, recipe.units(lines), with)
		if !strings.Contains(text, "About the video: Ein Podcast") || !strings.Contains(text, "Line 1 is in a clip already") {
			t.Errorf("%s does not say the context or the clips there are:\n%s", recipe.Name, text)
		}
		var schema map[string]any
		if err := json.Unmarshal([]byte(recipe.Schema(2, 6)), &schema); err != nil {
			t.Errorf("%s's schema is not JSON: %v", recipe.Name, err)
		}
	}
}

// The lean transcript is the words alone, and each switch adds what it
// names: the time a line starts at, and a mark before a long pause.
func TestTheLeanTranscripts(t *testing.T) {
	lines := said(0.0, 2.0, "Erste Erinnerung?", 0.5, 3.0, "Nicht so leicht.", 1.5, 70.0, "Weil ich mich nicht erinnere.",
		0.2, 1.0, "Echt.")
	for _, c := range []struct {
		sw   PromptSwitches
		want string
	}{
		{PromptSwitches{}, "[1] Erste Erinnerung?\n[2] Nicht so leicht.\n[3] Weil ich mich nicht erinnere.\n[4] Echt."},
		{PromptSwitches{Pause: 1}, "[1] Erste Erinnerung?\n[2] Nicht so leicht.\n[3] … Weil ich mich nicht erinnere.\n[4] Echt."},
		{PromptSwitches{Pause: 2}, "[1] Erste Erinnerung?\n[2] Nicht so leicht.\n[3] Weil ich mich nicht erinnere.\n[4] Echt."},
		{PromptSwitches{Pause: 1, Times: true}, "[1 0:00] Erste Erinnerung?\n[2 0:02] Nicht so leicht.\n" +
			"[3 0:07] … Weil ich mich nicht erinnere.\n[4 1:17] Echt."},
	} {
		if got := leanTranscript(lines, c.sw); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.sw, got, c.want)
		}
	}
}

// A side of a comparison names its switches after a +, and only a recipe
// asked from a prompt file takes them.
func TestSwitchesAreReadFromASidesName(t *testing.T) {
	v, err := ParseVariant("points+pause2+times+words@1024~0.3")
	if err != nil || v.Recipe != "points" || *v.Think != 1024 || *v.Temperature != 0.3 ||
		v.Switches != (PromptSwitches{Pause: 2, Times: true, Words: true}) {
		t.Fatalf("read as %+v %v", v, err)
	}
	if v.Switches.String() != "+pause2+times+words" {
		t.Errorf("written as %s", v.Switches)
	}
	if v, err := ParseVariant("heart-lean+pause"); err != nil || v.Switches.Pause != 1 {
		t.Errorf("+pause read as %+v %v", v, err)
	}
	for _, bad := range []string{"lines+times", "heart+pause", "points+loud", "points+pause0", "points+pause99", "points+"} {
		if _, err := ParseVariant(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

// Each switch changes the request only where it should: the times and
// the words say a length, the pause says what its mark means, and without
// switches nothing is said of seconds or of pauses.
func TestSwitchesChangeTheRequest(t *testing.T) {
	var parts []any
	for i := range 40 {
		parts = append(parts, 0.8, 4.0, fmt.Sprintf("Das ist der Satz Nummer %d hier.", i+1))
	}
	lines := said(parts...)
	for _, recipe := range []Recipe{heartLeanRecipe, pointsRecipe} {
		ask := func(sw PromptSwitches) string {
			return recipe.Request(lines, recipe.units(lines), PlanOptions{Count: 6, MinLen: 20, MaxLen: 30, Switches: sw})
		}
		plain := ask(PromptSwitches{})
		for _, not := range []string{"seconds", "…", "words, which is", ":0"} {
			if strings.Contains(plain, not) {
				t.Errorf("%s without switches says %q:\n%s", recipe.Name, not, plain)
			}
		}
		if text := ask(PromptSwitches{Times: true}); !strings.Contains(text, "[2 0:04]") ||
			!strings.Contains(text, "about 20 to 30 seconds, which the times tell you") {
			t.Errorf("%s+times:\n%s", recipe.Name, text)
		}
		// Seven words a line, 280 in 191.2 seconds: 29 to 44 words.
		if text := ask(PromptSwitches{Words: true}); !strings.Contains(text, "about 29 to 44 words") {
			t.Errorf("%s+words:\n%s", recipe.Name, text)
		}
		if text := ask(PromptSwitches{Pause: 2}); !strings.Contains(text, "a pause of 2 seconds or more") {
			t.Errorf("%s+pause2:\n%s", recipe.Name, text)
		}
	}
}

// A clip given as three points is the run from its start to its end, the
// payoff its heart and the start its opening, with a slug from its title.
func TestAClipGivenAsThreePoints(t *testing.T) {
	read := func(clip string) ([]PlanEntry, []string, error) {
		var data map[string]any
		if err := json.Unmarshal([]byte(`{"clips": [`+clip+`]}`), &data); err != nil {
			t.Fatal(err)
		}
		return ValidatePlan(data, lineUnits(20))
	}
	entries, problems, err := read(`{"title": "Der Regenschirm", "reason": "r", "start": 3, "payoff": 7, "end": 8}`)
	if err != nil || len(problems) > 0 {
		t.Fatalf("%v %v", err, problems)
	}
	e := entries[0]
	if e.Keep[0] != [2]int{3, 8} || e.Heart != [2]int{7, 7} || e.Opening != 3 || e.Slug != "der-regenschirm" {
		t.Errorf("read as %+v", e)
	}
	// An end before the payoff ends on the payoff.
	if entries, _, _ := read(`{"title": "T", "reason": "r", "start": 3, "payoff": 7, "end": 5}`); entries[0].Keep[0] != [2]int{3, 7} {
		t.Errorf("an end before the payoff became %v", entries[0].Keep)
	}
	for _, bad := range []string{
		`{"title": "T", "reason": "r", "start": 8, "payoff": 7, "end": 9}`,
		`{"title": "T", "reason": "r", "start": 0, "payoff": 7, "end": 9}`,
		`{"title": "T", "reason": "r", "start": 3, "payoff": 70, "end": 9}`,
		`{"title": "T", "reason": "r", "start": 3, "end": 9}`,
	} {
		if _, _, err := read(bad); err == nil {
			t.Errorf("%s was read as a clip", bad)
		}
	}
}

// A recipe asked from a prompt file sends one message and no system part,
// to the local model and to the API alike.
func TestAPromptFileIsOneMessage(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var mu sync.Mutex
	var sent [][]chatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []chatMessage }
		_ = json.NewDecoder(r.Body).Decode(&request)
		mu.Lock()
		sent = append(sent, request.Messages)
		mu.Unlock()
		writeLocalStream(w, `{"clips": [{"title": "Kurz", "reason": "r", "start": 1, "payoff": 1, "end": 1}]}`, 11)
	}))
	defer server.Close()
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Recipe = "points"
	p := NewProject(e, source, base)
	path, err := p.Plan(context.Background(), PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if len(sent) != 1 || len(sent[0]) != 1 || sent[0][0].Role != "user" {
		t.Fatalf("sent %+v", sent)
	}
	// One line is too short, and the engine takes in the one after it.
	if _, clips, err := LoadClips(path); err != nil || len(clips) != 1 || clips[0].Duration() < 20 {
		t.Errorf("clips %+v %v", clips, err)
	}

	for _, model := range []string{"claude-sonnet-5", "gpt-6-sol"} {
		body, err := bodyFor(ProviderFor(model), model, 100, "", "prompt", false, true)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(body, []byte(`"system"`)) || bytes.Contains(body, []byte(`"developer"`)) {
			t.Errorf("%s was sent a system part: %s", model, body)
		}
	}
}

// The prompts moved into files ask word for word what they asked when they
// were written in Go, kept in testdata/prompts as the code made them. A
// change to lines would be a new way of asking, with a new PromptVersion,
// and saved answers that no longer match their prompt.
func TestThePromptsInFilesAskWhatTheyAsked(t *testing.T) {
	lines := said(
		0.0, 1.1, "Was ist deine erste Erinnerung?",
		1.5, 1.1, "Erste Erinnerung?",
		1.4, 1.8, "Nicht so leicht zu beantworten, weil",
		0.3, 3.5, "ich war dann noch ein relativ kleiner Dütz, zweite, dritte Klasse.",
		3.7, 4.9, "Und dann hat er mich geschlagen und dieser Regenschirm ist zersprungen.",
	)
	for _, r := range []Recipe{linesRecipe, heartRecipe, heartOpeningRecipe} {
		for _, with := range []bool{false, true} {
			opts := PlanOptions{Count: 6, MinLen: 20, MaxLen: 30}
			name := r.Name
			if with {
				opts.Context = "Ein Podcast über erste Erinnerungen."
				opts.Taken = []Window{{Start: 0, End: 1.1}}
				name += "-with-context"
			}
			want, err := os.ReadFile("testdata/prompts/" + name + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			got := "=== system ===\n" + r.System + "\n=== user ===\n" + r.Request(lines, r.units(lines), opts) + "\n"
			if got != string(want) {
				t.Errorf("%s asks something new:\n%s", name, got)
			}
		}
	}
	if SystemPrompt != linesRecipe.System || SystemPrompt == "" {
		t.Error("the lines brief is not the one the training records name")
	}
}
