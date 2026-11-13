package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/timed"
)

// transcriptOpens is a disk that counts how often the transcript of a
// whole video is opened, to read it.
type transcriptOpens struct {
	afero.Fs
	n atomic.Int64
}

func (f *transcriptOpens) Open(name string) (afero.File, error) {
	f.count(name)
	return f.Fs.Open(name)
}

func (f *transcriptOpens) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f.count(name)
	return f.Fs.OpenFile(name, flag, perm)
}

func (f *transcriptOpens) count(name string) {
	if filepath.Base(name) == engine.TranscriptName(nil) {
		f.n.Add(1)
	}
}

// A video's state says what its transcript heard and fits the window to
// its lines, and each read the transcript's files for itself: twice for
// every state after the transcript grew, which it does every few seconds
// while the video is heard, #800. The state reads them once.
func TestAVideosStateReadsItsTranscriptOnce(t *testing.T) {
	counted := &transcriptOpens{Fs: afero.NewMemMapFs()}
	st := openStoreIn(counted, "/home/tim/config/FrameFairy")
	video := "/home/tim/videos/ep.mp4"
	if err := afero.WriteFile(counted, video, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markAdded(counted, video); err != nil {
		t.Fatal(err)
	}
	st.episodes = []string{video}
	svc := &FrameFairy{store: st}
	svc.jobs = newQueue(svc.store, func(JobUpdate) {}, func(string) {})
	const seconds = 120.0
	svc.probeVideo = func(context.Context, string) (engine.SourceInfo, error) {
		return engine.SourceInfo{Duration: seconds, Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1}, nil
	}

	logs := engine.LogsDir(video)
	info, err := counted.Stat(video)
	if err != nil {
		t.Fatal(err)
	}
	model := st.Settings().options().ASRModel
	if model == "" {
		model = engine.DefaultModelDir()
	}
	var words [][3]any
	for at := 0.0; at+0.4 < seconds; at += 0.4 {
		w := "wort"
		if len(words)%12 == 11 {
			w = "ende."
		}
		words = append(words, [3]any{at, at + 0.3, w})
	}
	body, err := json.Marshal(map[string]any{
		"version": 3, "model": filepath.Base(filepath.Clean(model)),
		"source": map[string]any{"name": filepath.Base(video), "size": info.Size(),
			"modified": info.ModTime().UTC().Format(time.RFC3339Nano)},
		"from": 0, "to": seconds, "mean": -20, "parts": [][2]float64{{0, seconds}}, "end": seconds,
		"words": words,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(counted, filepath.Join(logs, engine.TranscriptName(nil)), body, 0o644); err != nil {
		t.Fatal(err)
	}
	frames := make([]byte, 4*int(seconds/timed.FrameSeconds))
	if err := afero.WriteFile(counted, filepath.Join(logs, "words.frames"), frames, 0o644); err != nil {
		t.Fatal(err)
	}

	state := func(when string) {
		t.Helper()
		counted.n.Store(0)
		st, err := svc.State(context.Background(), video)
		if err != nil {
			t.Fatal(err)
		}
		if !st.Status.Transcribed {
			t.Fatalf("%s, the state says the video is not transcribed: %+v", when, st.Status)
		}
		if n := counted.n.Load(); n != 1 {
			t.Errorf("%s, the state read the transcript %d times, want once", when, n)
		}
	}
	state("the first time")
	// A part saved while the video is heard: both files are new.
	at := time.Now().Add(time.Minute)
	for _, f := range engine.TranscriptFiles(logs) {
		if err := counted.Chtimes(f, at, at); err != nil {
			t.Fatal(err)
		}
	}
	state("after the transcript grew")
}
