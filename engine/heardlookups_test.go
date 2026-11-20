package engine

import (
	"math"
	"strings"
	"sync"
	"testing"

	"framefairy/engine/timed"
)

// inLanguage is a transcript of words made by hand, in a language told
// rather than read off words too few to tell.
func inLanguage(words []timed.Cue, language string) *Transcript {
	l := &heardLookups{words: words, language: language}
	l.languageOnce.Do(func() {})
	return &Transcript{Words: words, lookups: l}
}

// englishHeard is a few seconds of English heard from 59.1 s, with two
// words the recogniser heard start in the same step, the first no longer
// than nothing.
func englishHeard() *Transcript {
	floor := -40.0
	frames, start := framesLoud(func(float64) bool { return true })
	var words []timed.Cue
	for i, text := range strings.Fields("the people who came that night were the ones who stayed with us until the very end of it") {
		at := 59.1 + float64(i)*0.2
		words = append(words, timed.Cue{Start: at, End: at + 0.15, Text: text})
	}
	words[4].End = words[4].Start
	words[5].Start = words[4].Start
	return fromStored(words, frames, start, -20, &floor)
}

// eagerKeys is the key of each heard word as hear made them before it
// left them until asked.
func eagerKeys(raw []timed.Cue) []string {
	keys := make([]string, len(raw))
	taken := map[int64]bool{}
	for i, w := range raw {
		ms := int64(math.Round(w.Start * 1000))
		for taken[ms] {
			ms++
		}
		taken[ms] = true
		keys[i] = heardKey(ms)
	}
	return keys
}

// A read of a transcript no longer works out its language and the key of
// every word until something asks. Whoever asks first, the video or a
// clip that corrects every word into another language, the language is
// the one of the words as heard, and every heard word keeps the key it
// had, the second of two heard in the same step too.
func TestTheLanguageAndTheKeysAreTheSameWhoeverAsksFirst(t *testing.T) {
	t.Parallel()
	want := wordsLanguage(englishHeard().Words)
	if want != "en" {
		t.Fatalf("the test needs words read as English, they read as %q", want)
	}
	for _, first := range []string{"the video", "a clip"} {
		tr := englishHeard()
		corrections := map[string]string{}
		for i, key := range eagerKeys(tr.RawWords) {
			corrections[key] = []string{"die", "Leute", "kamen", "und", "blieben"}[i%5]
		}
		clip := Clip{Corrections: corrections}
		var own *Transcript
		if first == "a clip" {
			own = tr.Of(clip)
		}
		if got := tr.Language(); got != want {
			t.Errorf("%s asked first: the video's words are in %q, want %q", first, got, want)
		}
		if own == nil {
			own = tr.Of(clip)
		}
		if got := own.Language(); got != want {
			t.Errorf("%s asked first: the clip's words are in %q, want %q", first, got, want)
		}
		for i, key := range eagerKeys(tr.RawWords) {
			if got := tr.keyAt(i); got != key {
				t.Errorf("%s asked first: heard word %d is kept by %q, want %q", first, i, got, key)
			}
			if got, ok := tr.heardIndex(key); !ok || got != i {
				t.Errorf("%s asked first: key %q finds word %d, want %d", first, key, got, i)
			}
		}
		if got := own.HeardWords[5].Text; got != "die" {
			t.Errorf("%s asked first: the second of two words heard at once reads %q, want its own fix die", first, got)
		}
		if got := own.HeardWords[4].Text; got != "blieben" {
			t.Errorf("%s asked first: the first of two words heard at once reads %q, want its own fix blieben", first, got)
		}
	}
}

// One transcript is read from several goroutines at once, a search, the
// captions of the clip timeline and an edit of a word, so its language
// and keys are worked out once however many ask together. Run under
// the race detector.
func TestALanguageAndKeysAskedForTogetherAreWorkedOutOnce(t *testing.T) {
	t.Parallel()
	tr := englishHeard()
	keys := eagerKeys(tr.RawWords)
	var wg sync.WaitGroup
	languages := make([]string, 16)
	found := make([]bool, 16)
	for g := range 16 {
		wg.Go(func() {
			own := tr.Of(Clip{Corrections: map[string]string{keys[g%len(keys)]: "Wort"}})
			languages[g] = own.Language()
			i, ok := tr.heardIndex(keys[g%len(keys)])
			found[g] = ok && i == g%len(keys)
		})
	}
	wg.Wait()
	for g := range 16 {
		if languages[g] != "en" || !found[g] {
			t.Errorf("goroutine %d read the language %q and found its word: %v", g, languages[g], found[g])
		}
	}
}
