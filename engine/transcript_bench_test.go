package engine

import (
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/runlog"
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

// BenchmarkAStartOfTheApp is what a start of the app asks of a video
// already heard, as the library asks it of every video: its Status, on a
// Machine of its own, as a new start has. After the first start, what is
// heard is read from its work folder, see heardkept.go.
func BenchmarkAStartOfTheApp(b *testing.B) {
	for _, hours := range []float64{1, 2, 4} {
		fsys, source := longVideo(b, hours)
		NewMachine(fsys).Episode(source).Status("m", nil)
		b.Run(fmt.Sprintf("%.0fh", hours), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if !NewMachine(fsys).Episode(source).Status("m", nil).Transcribed {
					b.Fatal("the video is not transcribed")
				}
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

// searchOfALongVideo is a project of a long video heard up to heardTo,
// and a search of its second hour.
func searchOfALongVideo(b *testing.B, hours, heardTo float64) (*Project, PlanRequest, func()) {
	b.Helper()
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
	var kept []timed.Cue
	for _, w := range words {
		if w.Start < heardTo {
			kept = append(kept, w)
		}
	}
	file.Parts, file.Words = [][2]num.Float{{0, num.Float(heardTo)}}, storedWords(kept)
	save := func() {
		if err := writeTranscript(fsys, path, file, frames); err != nil {
			b.Fatal(err)
		}
	}
	save()
	base := DefaultOptions()
	base.ASRModel = "/models/m"
	p := NewProject(NewEngine(runlog.New(io.Discard, false, false), NewMachine(fsys)), source, base)
	return p, PlanRequest{From: 3600, To: 7200}, save
}

// BenchmarkReadingAheadAfterASave times what one save of the transcript
// costs while a search of the second hour of a 2 h video waits for its
// window to be heard, heard to 1.5 h: the app reads the transcript again,
// and the read ahead works out the lines heard so far.
func BenchmarkReadingAheadAfterASave(b *testing.B) {
	p, req, save := searchOfALongVideo(b, 2, 5400)
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		save()
		b.StartTimer()
		if _, err := p.heldTranscript(); err != nil {
			b.Fatal(err)
		}
		if heard, ok := p.heardSoFar(req); !ok || len(heard.lines) == 0 {
			b.Fatal("nothing heard")
		}
	}
}

// BenchmarkStartingASearch times what a search of the second hour of a
// 2 h video, heard to 1.5 h, reads before it hears: whether it reads
// ahead, and what of its window is left to hear.
func BenchmarkStartingASearch(b *testing.B) {
	p, req, _ := searchOfALongVideo(b, 2, 5400)
	window := timed.Window{Start: req.From, End: req.To}
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		p.engine.Forget(p.Source)
		b.StartTimer()
		if !p.readsAhead(window) || len(p.unheard(window)) == 0 {
			b.Fatal("the window is heard")
		}
	}
}

// BenchmarkTheTranscriptSearch times one letter typed into the search in
// the clip column over the cards of a video of four hours: an answer
// that holds what was typed, one found only with a slip, and none, and
// making the cards ready, which is done once for each read of them.
func BenchmarkTheTranscriptSearch(b *testing.B) {
	fsys, source := longVideo(b, 4)
	r, err := NewMachine(fsys).Episode(source).Transcript("m", nil)
	if err != nil {
		b.Fatal(err)
	}
	cards := SaidCards(r.Transcript, nil)
	b.Run("ready", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			NewSaidSearch(cards).Answer("")
		}
	})
	search := NewSaidSearch(cards)
	search.Answer("")
	for _, typed := range []string{"ende", "wrot", "qxz"} {
		b.Run(typed, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				search.Answer(typed)
			}
		})
	}
}

// BenchmarkTheLinesOfASearch times the lines a search of half an hour
// asks about, with the default recipe, from a transcript read.
func BenchmarkTheLinesOfASearch(b *testing.B) {
	fsys, source := longVideo(b, 0.5)
	r, err := NewMachine(fsys).Episode(source).Transcript("m", nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		searchLines(r.Transcript, Options{})
	}
}
