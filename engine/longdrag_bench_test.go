package engine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/timed"
)

// talked is n seconds of speech the way people talk: words of a third of
// a second, and a pause of a second after every eleventh, so the captions
// part at pauses as they do in an episode.
func talked(seconds float64) []timed.Cue {
	var words []timed.Cue
	at := 0.0
	for i := 0; at < seconds; i++ {
		words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: fmt.Sprintf("wort%d", i%97)})
		at += 0.33
		if i%11 == 10 {
			at += 1
		}
	}
	return words
}

// benchLongDrag times one step of a drag of the end of a clip as long as
// span seconds, the call the app makes for every step of the hand.
func benchLongDrag(b *testing.B, span float64) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	plan := fmt.Sprintf(`{"source": "ep.mp4", "plan_id": "p1", "revision": 1, "clips": [
	  {"id": "01", "slug": "long", "title": "Long", "keep": true,
	   "segments": [{"start": 10, "end": %.2f}]}]}`, 10+span)
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		b.Fatal(err)
	}
	s := planAt(b, fsys, path)
	tr := fromStored(talked(span+120), nil, 0, 0, nil)
	g := Gesture{Kind: "trim", Edge: "end", From: 10 + span - 5, To: 10 + span - 5, ToWords: true, Frames: at25}
	view, err := ShapeClipView(s, "01", g, tr, 0.1, nil)
	if err != nil {
		b.Fatal(err)
	}
	sent, _ := json.Marshal(view)
	b.ReportMetric(float64(len(sent)), "B/answer")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ShapeClipView(s, "01", g, tr, 0.1, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAnEdgeOfAClipOf1Minute(b *testing.B) { benchLongDrag(b, 60) }

func BenchmarkAnEdgeOfAClipOf10Minutes(b *testing.B) { benchLongDrag(b, 600) }

func BenchmarkAnEdgeOfAClipOf60Minutes(b *testing.B) { benchLongDrag(b, 3600) }

// benchLanding times where the end of a clip as long as span seconds
// lands for one step of a drag, without its captions.
func benchLanding(b *testing.B, span float64) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/", "ep.framefairy", "logs", "clips.json")
	plan := fmt.Sprintf(`{"source": "ep.mp4", "plan_id": "p1", "revision": 1, "clips": [
	  {"id": "01", "slug": "long", "title": "Long", "keep": true,
	   "segments": [{"start": 10, "end": %.2f}]}]}`, 10+span)
	if err := afero.WriteFile(fsys, path, []byte(plan), 0o644); err != nil {
		b.Fatal(err)
	}
	s := planAt(b, fsys, path)
	tr := fromStored(talked(span+120), nil, 0, 0, nil)
	g := Gesture{Kind: "trim", Edge: "end", From: 10 + span - 5, To: 10 + span - 5, ToWords: true, Frames: at25}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ShapeClip(s, "01", g, tr, 0.1, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWhereAnEdgeOfAClipOf1MinuteLands(b *testing.B) { benchLanding(b, 60) }

func BenchmarkWhereAnEdgeOfAClipOf10MinutesLands(b *testing.B) { benchLanding(b, 600) }

func BenchmarkWhereAnEdgeOfAClipOf60MinutesLands(b *testing.B) { benchLanding(b, 3600) }
