package engine

import (
	"math"
	"strings"
	"testing"
)

// pieceRecognizer hears each piece it is given as one word, the piece's
// number, and keeps how long each piece was.
type pieceRecognizer struct{ lengths []float64 }

func (p *pieceRecognizer) Recognize(samples []float32, rate int) []Token {
	p.lengths = append(p.lengths, float64(len(samples))/float64(rate))
	return []Token{{Text: " piece" + string(rune('0'+len(p.lengths))), Start: 0.2, Duration: 0.3}, {Text: ".", Start: 0.5}}
}

func (*pieceRecognizer) Close() {}

// A recording is written down the way an episode is heard: cut into
// pieces no longer than the speech model takes, at the quietest moment,
// and the words of every piece in the order they were said. Report a
// Problem's microphone, plan row 2.189.
func TestHearWritesDownARecording(t *testing.T) {
	// 70 s of a tone, quiet from 20 s to 20.5 s and from 44 s to 44.5 s.
	samples := make([]float32, 70*SampleRate)
	for i := range samples {
		at := float64(i) / SampleRate
		if (at >= 20 && at < 20.5) || (at >= 44 && at < 44.5) {
			continue
		}
		samples[i] = float32(0.3 * math.Sin(2*math.Pi*220*at))
	}
	rec := &pieceRecognizer{}
	got := Hear(rec, samples)
	if got != "piece1. piece2. piece3." {
		t.Errorf("heard %q", got)
	}
	if len(rec.lengths) != 3 || math.Abs(rec.lengths[0]-20.25) > 0.2 || math.Abs(rec.lengths[1]-24) > 0.2 {
		t.Errorf("the pieces were %v s long, want cut in the middle of each quiet, at 20.25 s and 44.25 s", rec.lengths)
	}
	for _, l := range rec.lengths {
		if l > chunkMax {
			t.Errorf("a piece of %.1f s is longer than the speech model takes", l)
		}
	}

	// A few seconds are one piece, and nothing heard is nothing written.
	rec = &pieceRecognizer{}
	if got := Hear(rec, samples[:3*SampleRate]); got != "piece1." || len(rec.lengths) != 1 {
		t.Errorf("3 s heard as %q in %d pieces", got, len(rec.lengths))
	}
	if got := Hear(fakeRecognizer{new(int32)}, nil); got != "" {
		t.Errorf("no recording heard as %q", got)
	}
	if got := Hear(fakeRecognizer{new(int32)}, samples[:2*SampleRate]); strings.Count(got, "wort") != 3 {
		t.Errorf("2 s heard as %q, want three words", got)
	}
}
