package engine

import (
	"cmp"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"weak"

	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// The transcript reads as sentences: a pause inside one does not split
// it, a full stop ends it, and every word is in exactly one of them.
func TestTheTranscriptReadsAsSentences(t *testing.T) {
	var words []timed.Cue
	at := 0.0
	say := func(gap float64, texts ...string) {
		at += gap
		for _, w := range texts {
			words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: w})
			at += 0.4
		}
	}
	say(0, "Ich", "war", "drei", "Jahre", "alt,")
	say(1.2, "und", "meine", "Mutter", "hat", "mich", "einfach", "in", "den", "Arm", "genommen.")
	say(0.1, "Das", "weiß", "ich", "noch?")
	say(0.8, "Ja.")
	got := Sentences(&Transcript{Words: words}, nil)
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
	if got := fmt.Sprint(Sentences(&Transcript{}, nil)); got != "[]" {
		t.Errorf("an empty transcript reads %s", got)
	}
}

// A long sentence is read in cards of a few seconds, ended at its lines,
// and never one longer than cardSecondsMost, found by Tim.
func TestALongSentenceReadsInShortCards(t *testing.T) {
	var words []timed.Cue
	at := 0.0
	for i := range 60 {
		text := fmt.Sprintf("w%d", i)
		if i%8 == 7 {
			text += ","
		}
		words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: text})
		at += 0.4
		// A pause after every twelve words ends a line.
		if i%12 == 11 {
			at += 0.6
		}
	}
	words[len(words)-1].Text += "."
	got := Sentences(&Transcript{Words: words}, nil)
	if len(got) < 3 {
		t.Fatalf("a sentence of %.0f s reads as %d cards: %+v", words[len(words)-1].End, len(got), got)
	}
	n := 0
	for _, c := range got {
		if c.End-c.Start > cardSecondsMost {
			t.Errorf("a card runs %.1f s, from %.1f", c.End-c.Start, c.Start)
		}
		n += len(strings.Fields(c.Text))
	}
	if n != len(words) {
		t.Errorf("the cards hold %d words of %d", n, len(words))
	}
}

// A card is cut where a clip keeps from or to, so a card is all in the
// clip or all out of it, found by Tim. A word is on the side its middle
// is on.
func TestACardIsCutAtTheEdgesOfAClip(t *testing.T) {
	var words []timed.Cue
	for i, w := range strings.Fields("Und dieser Regenschirm ist zersprungen. Also hat sich einfach zerlegt, ne?") {
		words = append(words, timed.Cue{Start: float64(i) * 0.4, End: float64(i)*0.4 + 0.3, Text: w})
	}
	// The clip keeps from between Regenschirm and ist, and the next from
	// a little before the middle of einfach, which is in it.
	cuts := []float64{3.1, 1.15}
	got := Sentences(&Transcript{Words: words}, cuts)
	var texts []string
	for _, c := range got {
		texts = append(texts, c.Text)
	}
	want := []string{"Und dieser Regenschirm", "ist zersprungen.", "Also hat sich", "einfach zerlegt, ne?"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("the cards read %q, want %q", texts, want)
	}
}

// A clip's cards are its captions, one card a caption, with the words the
// caption shows, a correction typed in the caption box among them, and
// named for the clip. What no clip keeps reads in sentences. Found by
// Tim, who corrected a word of a clip and read it uncorrected in the list.
func TestTheCardsOfAClipAreItsCaptions(t *testing.T) {
	fsys, path := editablePlanPath(t)
	words := append(editableTranscript().Words, timed.Cue{Start: 30, End: 30.4, Text: "Und"}, timed.Cue{Start: 30.5, End: 30.9, Text: "dann."})
	tr := fromStored(words, nil, 0, 0, nil)
	if err := SetWordText(fsys, path, "01", 12, "Dreimal", tr); err != nil {
		t.Fatal(err)
	}
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range SaidCards(tr, summary.Said(tr, nil)) {
		got = append(got, fmt.Sprintf("%.1f-%.1f %s %s", c.Start, c.End, c.Clip, c.Text))
	}
	want := []string{"10.0-11.0 01 eins zwei", "12.0-13.1 01 Dreimal vier", "20.1-21.5 02 fünf sechs", "30.0-30.9  Und dann."}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("the cards read\n%q\nwant\n%q", got, want)
	}
}

// What a clip cuts out between two of its pieces is a card of no clip
// between the clip's cards, and in none of its captions.
func TestWordsCutOutOfAClipAreACardOfTheirOwn(t *testing.T) {
	fsys, path := editablePlanPath(t)
	words := append(editableTranscript().Words, timed.Cue{Start: 11.3, End: 11.6, Text: "ähm"})
	slices.SortFunc(words, func(a, b timed.Cue) int { return cmp.Compare(a.Start, b.Start) })
	tr := fromStored(words, nil, 0, 0, nil)
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range SaidCards(tr, summary.Said(tr, nil)) {
		got = append(got, fmt.Sprintf("%s %s", c.Clip, c.Text))
	}
	want := "01 eins zwei |  ähm | 01 drei vier | 02 fünf sechs"
	if strings.Join(got, " | ") != want {
		t.Errorf("the cards read %q, want %q", strings.Join(got, " | "), want)
	}
}

// Two clips over the same words each have their cards of them, the
// captions each shows.
func TestTwoClipsOverTheSameWordsEachHaveTheirCards(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	plan := `{"source": "ep.mp4", "plan_id": "p1", "clips": [
	  {"id": "01", "slug": "eins", "segments": [{"start": 10.0, "end": 13.6}]},
	  {"id": "02", "slug": "zwei", "segments": [{"start": 11.9, "end": 13.6}]}]}`
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := editableTranscript()
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range SaidCards(tr, summary.Said(tr, nil)) {
		got = append(got, fmt.Sprintf("%.1f %s %s", c.Start, c.Clip, c.Text))
	}
	for _, want := range []string{"10.0 01 eins zwei", "12.0 02 drei vier", "20.1  fünf sechs"} {
		if !slices.Contains(got, want) {
			t.Errorf("no card %q among %q", want, got)
		}
	}
	if n := len(slices.DeleteFunc(slices.Clone(got), func(s string) bool { return !strings.Contains(s, "drei") })); n != 2 {
		t.Errorf("%d cards read drei, want one of each clip: %q", n, got)
	}
}

// A word drawn in halves over two lines is one word of its card, and so is
// a part of a correction that reads as several words.
func TestACaptionCardReadsWholeWords(t *testing.T) {
	clip := Clip{Segments: []Segment{{Start: 10, End: 20}}}
	said, other, part0, part1 := 12.0, 15.0, 0, 1
	caption := CaptionView{Lines: []CaptionLineView{
		{Words: []WordView{{Start: 0, End: 1, Text: "Ich"}, {Start: 1, End: 1.5, Text: "Judo-", Said: &said, Whole: "Judostyle"}}},
		{Words: []WordView{
			{Start: 1.5, End: 2, Text: "style", Said: &said, Whole: "Judostyle"},
			{Start: 2, End: 2.3, Text: "hab", Said: &other, Whole: "hab gesagt", Part: &part0},
			{Start: 2.3, End: 2.8, Text: "gesagt", Said: &other, Whole: "hab gesagt", Part: &part1},
		}},
	}}
	card, ok := captionCard(ClipSaid{Key: "01", Clip: clip}, caption)
	if !ok || card.Text != "Ich Judostyle hab gesagt" || card.Start != 10 || card.End != 12.8 {
		t.Errorf("the card is %+v, %v", card, ok)
	}
}

// The transcript search asks for its cards again after every edit, and
// lays out the captions of each plan only when the plan, the transcript,
// the look or the faces changed since: one corrected word laid out every
// clip of every plan of the video.
func TestTheCaptionsOfAPlanAreLaidOutAgainOnlyWhenWhatTheyAreMadeOfChanged(t *testing.T) {
	t.Parallel()
	fsys, path := editablePlanPath(t)
	tr := editableTranscript()
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	fonts := new(Fonts)
	look := &Look{Style: map[string]any{"font": "Anton", "size": 80.0}, Fonts: fonts}
	first := summary.Said(tr, look)
	if len(first) != 2 || len(first[0].Captions) == 0 {
		t.Fatalf("laid out %d clips", len(first))
	}
	laidOut := func(said []ClipSaid) bool {
		return &said[0].Captions[0] != &first[0].Captions[0]
	}

	again := summary.Said(tr, &Look{Style: map[string]any{"size": 80.0, "font": "Anton"}, Fonts: fonts})
	if laidOut(again) {
		t.Error("the same plan, transcript and look were laid out again")
	}
	again[0].Key = "elsewhere/01"
	if summary.Said(tr, look)[0].Key != "01" {
		t.Error("a key changed by one caller reached the next")
	}

	for _, c := range []struct {
		name string
		said func() []ClipSaid
	}{
		{"another look", func() []ClipSaid {
			return summary.Said(tr, &Look{Style: map[string]any{"font": "Anton", "size": 60.0}, Fonts: fonts})
		}},
		{"another video", func() []ClipSaid {
			return summary.Said(tr, &Look{Style: look.Style, Fonts: fonts, Source: SourceInfo{Width: 1080, Height: 1920}})
		}},
		{"the transcript read again", func() []ClipSaid { return summary.Said(editableTranscript(), look) }},
		{"the faces changed", func() []ClipSaid {
			fonts.Use(afero.NewMemMapFs(), nil)
			return summary.Said(tr, look)
		}},
	} {
		if !laidOut(c.said()) {
			t.Errorf("%s: the captions were not laid out again", c.name)
		}
		first = summary.Said(tr, look)
	}
}

// The window asks for the cards from several calls at once, in looks
// that differ, while the faces are read again.
func TestTheCaptionsOfAPlanAreLaidOutFromSeveralCallsAtOnce(t *testing.T) {
	t.Parallel()
	fsys, path := editablePlanPath(t)
	tr := editableTranscript()
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	fonts := new(Fonts)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			look := &Look{Style: map[string]any{"size": float64(60 + 10*(i%2))}, Fonts: fonts}
			for range 10 {
				said := summary.Said(tr, look)
				if len(said) != 2 {
					t.Errorf("laid out %d clips", len(said))
				}
				said[0].Key = "taken"
			}
		})
	}
	wg.Go(func() { fonts.Use(afero.NewMemMapFs(), nil) })
	wg.Wait()
}

// Every plan read is kept, and the captions laid out with it, while a
// transcript is megabytes the Episode lets go of after the last ten
// videos. The captions kept do not keep their transcript.
func TestTheCaptionsKeptDoNotKeepTheirTranscript(t *testing.T) {
	t.Parallel()
	fsys, path := editablePlanPath(t)
	summary, err := summaryOf(fsys, path)
	if err != nil {
		t.Fatal(err)
	}
	tr := editableTranscript()
	gone := weak.Make(tr)
	if len(summary.Said(tr, nil)) != 2 {
		t.Fatal("the clips were not laid out")
	}
	tr = nil
	runtime.GC()
	if gone.Value() != nil {
		t.Error("the transcript is kept alive by the captions laid out in it")
	}
	runtime.KeepAlive(summary)
}

// The captions kept for the cards are known by the key of their look, so
// everything a look holds is in its key: a look that differs in any of
// its fields lays out again. A field added to Look and left out of its
// key would hand out stale cards, so a new field fails here first.
func TestEveryFieldOfALookIsInItsKey(t *testing.T) {
	t.Parallel()
	if n := reflect.TypeFor[Look]().NumField(); n != 3 {
		t.Fatalf("Look has %d fields, and this test and Look.key know 3", n)
	}
	fonts := new(Fonts)
	base := Look{Style: map[string]any{"font": "Anton"}, Source: SourceInfo{Width: 1920, Height: 1080}, Fonts: fonts}
	key := func(l Look) string {
		k, ok := l.key()
		if !ok {
			t.Fatal("no key")
		}
		return k
	}
	was := key(base)
	for name, other := range map[string]Look{
		"Style":  {Style: map[string]any{"font": "Inter"}, Source: base.Source, Fonts: fonts},
		"Source": {Style: base.Style, Source: SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}, Fonts: fonts},
		"Fonts":  {Style: base.Style, Source: base.Source, Fonts: new(Fonts)},
	} {
		if key(other) == was {
			t.Errorf("a look with another %s has the same key", name)
		}
	}
	fonts.Use(afero.NewMemMapFs(), nil)
	if key(base) == was {
		t.Error("the look's faces read again with a change have the same key")
	}
}
