package engine

import (
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// longVideo is a video of hours in memory, heard all the way through and
// saved the way hearing saves it: about 2.5 words a second in sentences
// of twelve, and loudness every 10 ms with a pause every 0.4 s. Two hours
// are 18,000 words and a transcript file of about 1 MB.
func longVideo(tb testing.TB, hours float64) (fsys afero.Fs, source string) {
	tb.Helper()
	fsys = afero.NewMemMapFs()
	source = "/v/ep.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		tb.Fatal(err)
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		tb.Fatal(err)
	}
	seconds := hours * 3600
	rng := rand.New(rand.NewPCG(1, 2))
	var words []timed.Cue
	for at := 0.0; at+0.4 < seconds; at += 0.4 {
		text := "wort"
		if len(words)%12 == 11 {
			text = "ende."
		}
		start := num.RoundTo(at+rng.Float64()*0.05, 3)
		words = append(words, timed.Cue{Start: start, End: num.RoundTo(start+0.3, 3), Text: text})
	}
	frames := make([]float32, int(math.Round(seconds/timed.FrameSeconds)))
	for i := range frames {
		frames[i] = -30 + float32(rng.Float64()*20)
		if i%40 < 6 {
			frames[i] = -70
		}
	}
	end := num.Float(seconds)
	if err := fsys.MkdirAll(LogsDir(source), 0o755); err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(LogsDir(source), TranscriptName(nil))
	if err := writeTranscript(fsys, path, transcriptFile{Version: transcriptVersion, Source: stamp, Model: "m",
		To: end, Mean: -25, Parts: [][2]num.Float{{0, end}}, End: end, Words: storedWords(words)}, frames); err != nil {
		tb.Fatal(err)
	}
	return fsys, source
}

// BenchmarkOpeningALongVideo times what opening a video reads of its
// transcript, and what the app reads again after every save while the
// video is heard: the files, where speech starts and ends, and the weight
// of every line.
func BenchmarkOpeningALongVideo(b *testing.B) {
	for _, hours := range []float64{1, 2, 4} {
		fsys, source := longVideo(b, hours)
		b.Run(fmt.Sprintf("%.0fh", hours), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r, err := NewMachine(fsys).Episode(source).Transcript("m", nil)
				if err != nil {
					b.Fatal(err)
				}
				r.Said()
				r.Weighed()
			}
		})
	}
}

// BenchmarkSavingAPartHeard times one save while a video is heard, every
// 8 s: the transcript read, the new part joined in and both files
// written, with the video heard up to a minute before its end.
func BenchmarkSavingAPartHeard(b *testing.B) {
	for _, hours := range []float64{1, 2, 4} {
		fsys, source := longVideo(b, hours)
		stamp, err := stampOf(fsys, source)
		if err != nil {
			b.Fatal(err)
		}
		path := filepath.Join(LogsDir(source), TranscriptName(nil))
		file, words, frames, ok := readTranscriptFile(fsys, path, stamp, "m")
		if !ok {
			b.Fatal("no transcript")
		}
		seconds := hours * 3600
		cut := seconds - 60
		var kept, raw []timed.Cue
		for _, w := range words {
			if w.Start < cut {
				kept = append(kept, w)
			}
			if w.Start >= cut-8 && w.Start < cut+8 {
				raw = append(raw, w)
			}
		}
		end := num.Float(seconds)
		file.Parts, file.Words = [][2]num.Float{{0, num.Float(cut)}}, storedWords(kept)
		m := NewMachine(fsys)
		b.Run(fmt.Sprintf("%.0fh", hours), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				if err := writeTranscript(fsys, path, file, frames); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := m.addHeard(path, stamp, "m", float64(end), -25, timed.Window{Start: cut, End: cut + 8}, raw,
					frames[int(cut/timed.FrameSeconds)-800:int(cut/timed.FrameSeconds)+800], cut-8); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
