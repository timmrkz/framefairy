package engine

import (
	"os"
	"path/filepath"
	"testing"

	"framefairy/engine/fileio"
	"framefairy/engine/timed"
)

// namingTranscript is a few words heard from 10 s, the first of them
// heard wrong.
func namingTranscript() *Transcript {
	var words []timed.Cue
	for i, text := range []string{"Ick", "hab", "mich", "gestern", "im", "Wald", "verlaufen."} {
		at := 10 + float64(i)*0.5
		words = append(words, timed.Cue{Start: at, End: at + 0.4, Text: text})
	}
	return fromStored(words, nil, 0, 0, nil)
}

// A clip is called by the words it shows now, on its card and in the names
// of its files, so a typo corrected, a start moved or a first word removed
// renames it. It kept the name it was made with, the user's report on
// issue #1015.
func TestAClipIsCalledByTheWordsItShowsNow(t *testing.T) {
	t.Parallel()
	said := namingTranscript()
	found := Clip{ID: "01", Slug: "ick-hab", Title: "Ick hab", Segments: []Segment{{Start: 9.9, End: 14}}}
	corrected := found
	corrected.Corrections = map[string]string{wordKey(10): "Ich"}
	// The same clip corrected another way, named after the first was,
	// so a name kept for its first correction is not the one it gets.
	again := found
	again.Corrections = map[string]string{wordKey(10): "Du"}
	moved := found
	moved.Segments = []Segment{{Start: 10.45, End: 14}}
	removed := found
	removed.RemovedWords = RemovedWords{wordKey(10): true}
	elsewhere := found
	elsewhere.Segments = []Segment{{Start: 40, End: 50}}
	for _, c := range []struct {
		name        string
		clip        Clip
		said        *Transcript
		title, slug string
	}{
		{"as it was found", found, said, "Ick hab mich gestern im Wald verlaufen.", "ick-hab-mich-gestern-im-wald-verlaufen"},
		{"its first word corrected", corrected, said, "Ich hab mich gestern im Wald verlaufen.", "ich-hab-mich-gestern-im-wald-verlaufen"},
		{"its first word corrected again", again, said, "Du hab mich gestern im Wald verlaufen.", "du-hab-mich-gestern-im-wald-verlaufen"},
		{"its first word corrected back", corrected, said, "Ich hab mich gestern im Wald verlaufen.", "ich-hab-mich-gestern-im-wald-verlaufen"},
		{"its start moved past its first word", moved, said, "hab mich gestern im Wald verlaufen.", "hab-mich-gestern-im-wald-verlaufen"},
		{"its first word removed", removed, said, "hab mich gestern im Wald verlaufen.", "hab-mich-gestern-im-wald-verlaufen"},
		{"where no word is heard", elsewhere, said, "Ick hab", "ick-hab"},
		{"with no transcript", found, nil, "Ick hab", "ick-hab"},
	} {
		if title, slug := c.said.Named(c.clip); title != c.title || slug != c.slug {
			t.Errorf("%s, the clip is called %q, %q, not %q, %q", c.name, title, slug, c.title, c.slug)
		}
	}
}

// A clip whose first word was corrected after its short was rendered gets
// a short of its new words the next time it is rendered, with its pictures,
// and that short is the clip's own. The short of its old words stays where
// it was, because a short the person has may be posted already, and the
// app never removes one. Rendered again with the same words, it writes
// over its own short.
//
// On the real disk: a short is a file on the disk, see IsShort.
func TestAShortIsNamedByTheWordsItsClipShowsNow(t *testing.T) {
	t.Parallel()
	source, plan := episodeWithPlan(t, "folge")
	dir := fileio.ResolvePath(t.TempDir())
	before := placeShort(t, plan, dir, "01")
	if want := filepath.Join(dir, "wort.mp4"); before != want {
		t.Fatalf("the short was written to %s, not %s", before, want)
	}

	said := fromStored([]timed.Cue{{Start: 2, End: 2.4, Text: "Hallo"}, {Start: 2.5, End: 3, Text: "Welt"}}, nil, 0, 0, nil)
	after := placeShortSaying(t, plan, dir, "01", said)
	if want := filepath.Join(dir, "hallo-welt.mp4"); after != want {
		t.Errorf("rendered with the words Hallo Welt, the short was written to %s, not %s", after, want)
	}
	if _, err := os.Stat(filepath.Join(dir, thumbnailName("hallo-welt", 1))); err != nil {
		t.Errorf("the short's picture is not named by its words: %v", err)
	}
	if c := shortOfClip(t, plan, "01"); c.Short != after || !c.Rendered {
		t.Errorf("the clip says its short is %q, not %q", c.Short, after)
	}
	if !NewMachine(disk).Episode(source).IsShort(after) {
		t.Error("the short of the clip's new words is not taken for its short")
	}
	if _, err := os.Stat(before); err != nil {
		t.Errorf("the short of the old words is gone: %v", err)
	}
	if again := placeShortSaying(t, plan, dir, "01", said); again != after {
		t.Errorf("rendered again with the same words, the short went to %s, not over its own at %s", again, after)
	}
}
