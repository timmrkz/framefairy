package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The prompt files of one message fill in: the transcript and the count
// are in them, nothing is left of the template, and without switches
// nothing is said of seconds. A recipe answers in JSON or without it,
// never both.
func TestThePromptFilesFillIn(t *testing.T) {
	lines := said(0.0, 2.0, "Was ist deine erste Erinnerung?", 1.5, 3.0, "Der Regenschirm ist zersprungen.")
	opts := PlanOptions{Count: 6, MinLen: 20, MaxLen: 30}
	for _, recipe := range []Recipe{heartLeanRecipe, pointsRecipe, middleRecipe} {
		text := recipe.Request(lines, recipe.units(lines), opts)
		for _, want := range []string{"[2] Der Regenschirm ist zersprungen.", " 6 "} {
			if !strings.Contains(text, want) {
				t.Errorf("%s has no %q:\n%s", recipe.Name, want, text)
			}
		}
		for _, not := range []string{"seconds", "{{", "===", "About the video"} {
			if strings.Contains(text, not) {
				t.Errorf("%s says %q:\n%s", recipe.Name, not, text)
			}
		}
		if recipe.System != "" {
			t.Errorf("%s has a system part", recipe.Name)
		}
		if recipe.Schema != nil {
			var schema map[string]any
			if err := json.Unmarshal([]byte(recipe.Schema(2, 6)), &schema); err != nil {
				t.Errorf("%s's schema is not JSON: %v", recipe.Name, err)
			}
		}
		if (recipe.Schema == nil) == (recipe.Plain == nil) {
			t.Errorf("%s answers both with and without JSON, or neither", recipe.Name)
		}
	}
	// The middle prompt says nothing of videos, of shorts or of JSON, and
	// names no part of a story.
	text := strings.ToLower(middleRecipe.Request(lines, nil, opts))
	for _, not := range []string{"video", "short", "json", "clip", "heart", "payoff", "opening", "setup"} {
		if strings.Contains(text, not) {
			t.Errorf("middle says %q:\n%s", not, text)
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
	v, err := ParseVariant("points+pause2+times@1024~0.3")
	if err != nil || v.Recipe != "points" || *v.Think != 1024 || *v.Temperature != 0.3 ||
		v.Switches != (PromptSwitches{Pause: 2, Times: true}) {
		t.Fatalf("read as %+v %v", v, err)
	}
	if v.Switches.String() != "+pause2+times" {
		t.Errorf("written as %s", v.Switches)
	}
	if v, err := ParseVariant("heart-lean+pause"); err != nil || v.Switches.Pause != 1 {
		t.Errorf("+pause read as %+v %v", v, err)
	}
	for _, bad := range []string{"lines+times", "heart+pause", "points+loud", "points+pause0", "points+pause99", "points+",
		"points+plain", "middle+words"} {
		if _, err := ParseVariant(bad); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
}

// Without switches the length is said in words, from how fast the
// speaker talks, and nothing of seconds or pauses. +times says it in
// seconds instead, and the pause says what its mark means.
func TestSwitchesChangeTheRequest(t *testing.T) {
	var parts []any
	for i := range 40 {
		parts = append(parts, 0.8, 4.0, fmt.Sprintf("Das ist der Satz Nummer %d hier.", i+1))
	}
	lines := said(parts...)
	for _, recipe := range []Recipe{heartLeanRecipe, pointsRecipe, middleRecipe} {
		ask := func(sw PromptSwitches) string {
			return recipe.Request(lines, recipe.units(lines), PlanOptions{Count: 6, MinLen: 20, MaxLen: 30, Switches: sw})
		}
		plain := ask(PromptSwitches{})
		for _, not := range []string{"seconds", "…", ":0"} {
			if strings.Contains(plain, not) {
				t.Errorf("%s without switches says %q:\n%s", recipe.Name, not, plain)
			}
		}
		// Seven words a line, 280 in 191.2 seconds: 30 seconds are 44
		// words, of which three quarters are said.
		words := "up to about 33 words"
		if !strings.Contains(plain, words) {
			t.Errorf("%s says no length in words:\n%s", recipe.Name, plain)
		}
		if text := ask(PromptSwitches{Times: true}); !strings.Contains(text, "[2 0:04]") ||
			!strings.Contains(text, "about 20 to 30 seconds") || strings.Contains(text, words) {
			t.Errorf("%s+times:\n%s", recipe.Name, text)
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
		// The last line has no line break, so it is taken from the whole
		// answer rather than as it arrives.
		writeLocalStream(w, "1 1 1", 3)
	}))
	defer server.Close()
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.Recipe = "middle"
	p := NewProject(e, source, base)
	path, err := p.Plan(context.Background(), PlanRequest{Count: 1})
	if err != nil {
		t.Fatalf("plan: %v %s", err, p.LastError())
	}
	if len(sent) != 1 || len(sent[0]) != 1 || sent[0][0].Role != "user" {
		t.Fatalf("sent %+v", sent)
	}
	// One line is too short, and the engine takes in the one after it. A
	// story the model gave no title is named after its first words.
	if _, clips, err := LoadClips(path); err != nil || len(clips) != 1 || clips[0].Duration() < 20 ||
		clips[0].Title == "" || strings.HasPrefix(clips[0].Slug, "clip") {
		t.Errorf("clips %+v %v", clips, err)
	}
	// The answer is kept with how it was got, which a comparison reads.
	saved, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "..", "..", "logs", "reply-*.json"))
	if len(saved) != 1 {
		t.Fatalf("saved %v", saved)
	}
	if _, use := lastLocalAnswer(filepath.Dir(saved[0]), time.Time{}); use.Asks != 1 || use.Read != 100 {
		t.Errorf("the saved answer tells %+v", use)
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

// updatePrompts writes testdata/prompts anew from the prompt files, for a
// change to a prompt that is meant:
//
//	go test ./engine -run TestEveryPromptAsksWhatTestdataSays -update-prompts
//
// The pull request then shows exactly what the model is sent.
var updatePrompts = flag.Bool("update-prompts", false, "write testdata/prompts from the prompt files")

// Every prompt file asks word for word what testdata/prompts keeps: with
// nothing more, with a context and a line that is a clip already, and the
// files that take switches with them. lines, heart and heart-opening are
// kept as the Go code made them before they moved into files. A change to
// lines is a new way of asking, with a new PromptVersion, and saved answers
// that no longer match their prompt.
func TestEveryPromptAsksWhatTestdataSays(t *testing.T) {
	lines := said(
		0.0, 1.1, "Was ist deine erste Erinnerung?",
		1.5, 1.1, "Erste Erinnerung?",
		1.4, 1.8, "Nicht so leicht zu beantworten, weil",
		0.3, 3.5, "ich war dann noch ein relativ kleiner Dütz, zweite, dritte Klasse.",
		3.7, 4.9, "Und dann hat er mich geschlagen und dieser Regenschirm ist zersprungen.",
	)
	type side struct {
		name string
		opts PlanOptions
	}
	base := PlanOptions{Count: 6, MinLen: 20, MaxLen: 30}
	with := base
	with.Context = "Ein Podcast über erste Erinnerungen."
	with.Taken = []Window{{Start: 0, End: 1.1}}
	fromFiles := []Recipe{linesRecipe, heartRecipe, heartOpeningRecipe, heartLeanRecipe, pointsRecipe, middleRecipe}
	if files, err := promptFiles.ReadDir("prompts"); err != nil || len(files) != len(fromFiles) {
		t.Fatalf("%d prompt files, %d of them held to what they ask: %v", len(files), len(fromFiles), err)
	}
	for _, r := range fromFiles {
		sides := []side{{r.Name, base}, {r.Name + "-with-context", with}}
		if r.Switchable {
			for _, sw := range []PromptSwitches{{Pause: 1}, {Times: true}} {
				opts := base
				opts.Switches = sw
				sides = append(sides, side{r.Name + sw.String(), opts})
			}
		}
		for _, s := range sides {
			got := r.Request(lines, r.units(lines), s.opts) + "\n"
			if r.System != "" {
				got = "=== system ===\n" + r.System + "\n=== user ===\n" + got
			}
			path := "testdata/prompts/" + s.name + ".txt"
			if *updatePrompts {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s asks something new:\n%s", s.name, got)
			}
		}
	}
	if SystemPrompt != linesRecipe.System || SystemPrompt == "" {
		t.Error("the lines brief is not the one the training records name")
	}
}

// A plain answer is one line a clip, read into the clip a JSON answer
// gives. A line that is not a clip is passed over, and a line written in
// pieces is read once it is whole.
func TestAPlainAnswer(t *testing.T) {
	reply := "Hier sind die Momente:\n12 18 19 | Der Regenschirm | Ein Kind wehrt sich.\n" +
		"3 4 | zu wenige Zahlen\n30 33 33 | Spiegel |\n7 x 9 | kein Clip | nein"
	var data map[string]any
	if err := json.Unmarshal([]byte(pointsPlain.answer(reply)), &data); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, pointsPlain.answer(reply))
	}
	entries, _, err := ValidatePlan(data, lineUnits(40))
	if err != nil || len(entries) != 2 {
		t.Fatalf("%+v %v", entries, err)
	}
	if e := entries[0]; e.Keep[0] != [2]int{12, 19} || e.Heart != [2]int{18, 18} || e.Opening != 12 ||
		e.Title != "Der Regenschirm" || e.Reason != "Ein Kind wehrt sich." {
		t.Errorf("first read as %+v", e)
	}
	if e := entries[1]; e.Keep[0] != [2]int{30, 33} || e.Title != "Spiegel" || e.Reason != "" {
		t.Errorf("second read as %+v", e)
	}
	var scan lineScanner
	var got []string
	for _, piece := range []string{"12 18", " 19 | Der Re", "genschirm | Grund\n30 33 33", " | S | R\n"} {
		got = append(got, scan.feed(piece)...)
	}
	if len(got) != 2 || got[0] != "12 18 19 | Der Regenschirm | Grund" || got[1] != "30 33 33 | S | R" {
		t.Errorf("lines %q", got)
	}
}

// A middle answer is three numbers a story: its start, a line in its
// middle and its end. The line in the middle is its heart, which the
// engine never ends a story before. Three lines out of order are sorted,
// so a middle written first or last is still the one between the others.
func TestAMiddleAnswer(t *testing.T) {
	reply := "Hier:\n31 40 52\n7 9 5\n1 2\n12 18 19 | ein Titel | nein\n3 3 3\n44 39 52"
	var data map[string]any
	if err := json.Unmarshal([]byte(middlePlain.answer(reply)), &data); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, middlePlain.answer(reply))
	}
	entries, problems, err := ValidatePlan(data, lineUnits(60))
	if err != nil || len(entries) != 4 || len(problems) > 0 {
		t.Fatalf("%+v %v %v", entries, problems, err)
	}
	type story struct{ keep, heart [2]int }
	for i, want := range []story{{[2]int{31, 52}, [2]int{40, 40}}, {[2]int{5, 9}, [2]int{7, 7}},
		{[2]int{3, 3}, [2]int{3, 3}}, {[2]int{39, 52}, [2]int{44, 44}}} {
		if e := entries[i]; e.Keep[0] != want.keep || e.Heart != want.heart || e.Opening != want.keep[0] ||
			e.Title != "" {
			t.Errorf("story %d read as %+v", i+1, e)
		}
	}
	past := map[string]any{"clips": []any{map[string]any{"middle": 70.0, "start": 1.0, "end": 2.0}}}
	if _, _, err := ValidatePlan(past, lineUnits(60)); err == nil {
		t.Error("a middle past the transcript was read")
	}
	// Six stories may run a thought of 2,048 tokens and a line each.
	if got := middlePlain.answerCap(2048, 6); got != 2048+256+6*16 {
		t.Errorf("six stories may run %d tokens", got)
	}
	text := "Und dann hat er mich geschlagen und dieser Regenschirm ist zersprungen."
	if got := firstWords(text, 31); got != "Und dann hat er mich geschlagen" {
		t.Errorf("named %q", got)
	}
}

// A search asking for a plain answer holds the local model to no schema
// and no grammar, so it can think first, and takes the clips from the
// lines it writes, passing over the rest.
func TestASearchWithAPlainAnswer(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var mu sync.Mutex
	var asked map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.NewDecoder(r.Body).Decode(&asked)
		mu.Unlock()
		writeLocalStream(w, "Hier sind sie:\n1 1 2 | Kurz | Ein Grund.\n3 3 3 | Zu viel | Einer mehr.\n", 5)
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
	mu.Lock()
	_, schema := asked["response_format"]
	_, grammar := asked["grammar"]
	most, _ := asked["max_tokens"].(float64)
	mu.Unlock()
	if schema || grammar {
		t.Errorf("asked with a schema %v, a grammar %v", schema, grammar)
	}
	// It may run its thinking and a line a clip, not the 48,000 tokens a
	// JSON answer may.
	if most < 256+80 || most > 4096+256+80 {
		t.Errorf("an answer of one clip may run %v tokens", most)
	}
	if _, clips, err := LoadClips(path); err != nil || len(clips) != 1 || clips[0].Title != "Kurz" {
		t.Errorf("clips %+v %v", clips, err)
	}
}
