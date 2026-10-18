package engine

import (
	"fmt"
	"testing"
)

// The transcript reads as sentences: a pause inside one does not split
// it, a full stop ends it, and every word is in exactly one of them.
func TestTheTranscriptReadsAsSentences(t *testing.T) {
	var words []Cue
	at := 0.0
	say := func(gap float64, texts ...string) {
		at += gap
		for _, w := range texts {
			words = append(words, Cue{Start: at, End: at + 0.3, Text: w})
			at += 0.4
		}
	}
	say(0, "Ich", "war", "drei", "Jahre", "alt,")
	say(1.2, "und", "meine", "Mutter", "hat", "mich", "einfach", "in", "den", "Arm", "genommen.")
	say(0.1, "Das", "weiß", "ich", "noch?")
	say(0.8, "Ja.")
	got := Sentences(&Transcript{Words: words})
	want := []string{
		"Ich war drei Jahre alt, und meine Mutter hat mich einfach in den Arm genommen.",
		"Das weiß ich noch?",
		"Ja.",
	}
	if len(got) != len(want) {
		t.Fatalf("%d sentences, want %d: %+v", len(got), len(want), got)
	}
	for i, s := range got {
		if s.Text != want[i] {
			t.Errorf("sentence %d reads %q, want %q", i, s.Text, want[i])
		}
	}
	if got[0].Start != words[0].Start || got[2].End != words[len(words)-1].End {
		t.Errorf("the sentences run %v to %v, the words %v to %v", got[0].Start, got[2].End, words[0].Start, words[len(words)-1].End)
	}
	if got := fmt.Sprint(Sentences(&Transcript{})); got != "[]" {
		t.Errorf("an empty transcript reads %s", got)
	}
}
