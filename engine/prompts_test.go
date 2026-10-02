package engine

import (
	"bytes"
	"context"
	"encoding/json"
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
		// Only a transcript with times can say anything of seconds.
		if says := strings.Contains(text, "20 to 30 seconds"); says != (recipe.Name == "points") {
			t.Errorf("%s says the length %v:\n%s", recipe.Name, says, text)
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

// The lean transcript is the words and a mark before a long pause, the
// timed one has the minute and second each line starts at as well.
func TestTheLeanTranscripts(t *testing.T) {
	lines := said(0.0, 2.0, "Erste Erinnerung?", 0.5, 3.0, "Nicht so leicht.", 1.5, 70.0, "Weil ich mich nicht erinnere.",
		0.2, 1.0, "Echt.")
	if got, want := bareTranscript(lines), "[1] Erste Erinnerung?\n[2] Nicht so leicht.\n"+
		"[3] … Weil ich mich nicht erinnere.\n[4] Echt."; got != want {
		t.Errorf("bare:\n%s\nwant:\n%s", got, want)
	}
	if got, want := timedTranscript(lines), "[1 0:00] Erste Erinnerung?\n[2 0:02] Nicht so leicht.\n"+
		"[3 0:07] … Weil ich mich nicht erinnere.\n[4 1:17] Echt."; got != want {
		t.Errorf("timed:\n%s\nwant:\n%s", got, want)
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
