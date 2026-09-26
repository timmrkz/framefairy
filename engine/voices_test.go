package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Speakers are numbered in the order they first speak, and each line goes
// to whoever speaks most of it.
func TestEachLineHasItsSpeaker(t *testing.T) {
	turns := numberSpeakers([]Turn{{0, 4, 7}, {4.2, 9, 3}, {9.5, 12, 7}}, 100)
	if got := fmt.Sprint(turns); got != "[{100 104 1} {104.2 109 2} {109.5 112 1}]" {
		t.Fatalf("turns %s", got)
	}
	lines := []Line{
		speech(100.5, 0, "Was", "ist", "deine", "erste", "Erinnerung?"),
		speech(104.5, 0.3, "Erste", "Erinnerung?"),
		speech(108.6, 0.2, "Hm", "gut."),
		speech(120, 0.2, "Niemand."),
	}
	GiveSpeakers(lines, turns)
	var got []int
	for _, l := range lines {
		got = append(got, l.Speaker)
	}
	// The third line runs past the end of the second turn into the gap
	// before the next, and the last is in nobody's turn.
	if fmt.Sprint(got) != "[1 2 2 0]" {
		t.Errorf("speakers %v", got)
	}
}

// With speakers, a sentence ends where the speaker changes, a new paragraph
// opens, and it says who speaks. Without them nothing changes.
func TestDialogueSaysWhoSpeaks(t *testing.T) {
	lines := []Line{
		speech(0, 0, "Was", "ist", "deine", "erste"),
		speech(2, 0.3, "Erinnerung"),
		speech(3, 0.3, "Ein", "Regenschirm."),
		speech(5, 0.3, "Er", "zersprang."),
	}
	for i := range lines {
		lines[i].Index = i + 1
	}
	lines[0].Speaker, lines[1].Speaker, lines[2].Speaker, lines[3].Speaker = 1, 1, 2, 2
	units := sentenceUnits(lines)
	if got := fmt.Sprint(units); got != "[[0 1] [2 2] [3 3]]" {
		t.Fatalf("sentences %s", got)
	}
	want := "(0:00) A: [1] Was ist deine erste Erinnerung\n(0:03) B: [2] Ein Regenschirm. [3] Er zersprang."
	if got := writeSentences(lines, units); got != want {
		t.Errorf("written as\n%s\nnot\n%s", got, want)
	}
	if r, _ := RecipeNamed("dialogue"); !r.Voices || !strings.Contains(r.System, voicesBrief) {
		t.Error("the dialogue recipe does not say who speaks")
	}
	if speakerLetter(1) != "A" || speakerLetter(26) != "Z" || speakerLetter(27) != "A2" {
		t.Error("letters")
	}
}

type fakeVoices struct{ heard *int32 }

// Two voices taking turns every five seconds.
func (f fakeVoices) Turns(samples []float32) []Turn {
	atomic.AddInt32(f.heard, 1)
	var turns []Turn
	for at, speaker := 0.0, 5; at < float64(len(samples))/SampleRate; at, speaker = at+5, 12-speaker {
		turns = append(turns, Turn{at, at + 5, speaker})
	}
	return turns
}

func (fakeVoices) Close() {}

// A search with the dialogue recipe tells the voices apart once, keeps what
// it found beside the transcript, and asks with a transcript that says who
// speaks.
func TestADialogueSearchHearsTheVoicesOnce(t *testing.T) {
	source := testEpisode(t, "20")
	SetTrainingDir(t.TempDir())
	var heard, asked, voices int32
	var prompts []string
	server := fakeModel(t, &asked)
	defer server.Close()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	e.OpenDiarizer = func(string) (Diarizer, error) { return fakeVoices{&voices}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Recipe = "dialogue"
	p := NewProject(e, source, base)
	for range 2 {
		base.Replan = true
		if _, err := p.Plan(context.Background(), PlanRequest{Count: 1, Min: 1}); err != nil {
			t.Fatalf("plan: %v %s", err, p.LastError())
		}
		body, _ := os.ReadFile(filepath.Join(p.LogsDir(), "plan-prompt.txt"))
		prompts = append(prompts, string(body))
	}
	if voices != 1 {
		t.Errorf("the voices were told apart %d times", voices)
	}
	if kept, _ := filepath.Glob(filepath.Join(p.LogsDir(), "voices-*.json")); len(kept) != 1 {
		t.Errorf("kept %v", kept)
	}
	if !strings.Contains(prompts[0], " A: [1]") || !strings.Contains(prompts[0], " B: [") {
		t.Errorf("the transcript does not say who speaks:\n%s", prompts[0])
	}
}
