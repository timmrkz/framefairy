package engine

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"framefairy/engine/num"
	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// BenchmarkKeep is what keeping a copy of a four hour video costs each
// time the watch hears its transcript written, which transcription does
// every 8 seconds, checkpointDefault: with nothing new, and with the
// words and their loudness written again.
//
// On the real disk: a copy flushed to the drive is what it measures.
func BenchmarkKeep(b *testing.B) {
	fsys := afero.NewOsFs()
	dir := b.TempDir()
	source := filepath.Join(dir, "Folge 12.mp4")
	if err := afero.WriteFile(fsys, source, make([]byte, 4<<20), 0o644); err != nil {
		b.Fatal(err)
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		b.Fatal(err)
	}
	const hours = 4
	seconds := float64(hours * 3600)
	words := make([]timed.Cue, 0, hours*3600*5/2)
	for i := range cap(words) {
		at := float64(i) * 0.4
		words = append(words, timed.Cue{Start: at, End: at + 0.3, Text: fmt.Sprintf("wort%d", i%50)})
	}
	frames := make([]float32, int(seconds/timed.FrameSeconds))
	file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: "m", Mean: -25, End: num.Float(seconds),
		Parts: [][2]num.Float{{0, num.Float(seconds)}}, Words: storedWords(words)}
	path := filepath.Join(LogsDir(source), TranscriptName(nil))
	if err := MakeDir(fsys, filepath.Dir(path)); err != nil {
		b.Fatal(err)
	}
	if err := writeTranscript(fsys, path, file, frames); err != nil {
		b.Fatal(err)
	}
	m := NewMachine(fsys)
	m.KeepIn(filepath.Join(dir, "kept"))
	if err := m.Keep(source); err != nil {
		b.Fatal(err)
	}
	b.Run("nothing new", func(b *testing.B) {
		for b.Loop() {
			if err := m.Keep(source); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("transcript written", func(b *testing.B) {
		at := time.Now()
		for b.Loop() {
			at = at.Add(time.Second)
			b.StopTimer()
			_ = fsys.Chtimes(path, at, at)
			_ = fsys.Chtimes(framesPath(path), at, at)
			b.StartTimer()
			if err := m.Keep(source); err != nil {
				b.Fatal(err)
			}
		}
	})
}
