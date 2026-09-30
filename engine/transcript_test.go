package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// transcript.go keeps what the recogniser heard, so the same audio is never
// listened to twice. A cached transcript that does not belong to this
// episode, this model or this version of the format is worse than none, so
// most of this is about what the cache refuses to serve.

// countingRecognizer remembers how much audio it was given.
type countingRecognizer struct {
	fakeRecognizer
	seconds *float64
}

func (c countingRecognizer) Recognize(samples []float32, rate int) []Token {
	*c.seconds += float64(len(samples)) / float64(rate)
	return c.fakeRecognizer.Recognize(samples, rate)
}

func TestTranscriptionResumes(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = 0
	defer func() { checkpointEvery = saved }()

	var calls int32
	heard := 0.0
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return countingRecognizer{fakeRecognizer{&calls}, &heard}, nil
	}
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	covered, done := Coverage(source, base.ASRModel)
	if !done || covered < 69.9 {
		t.Fatalf("coverage %v %v", covered, done)
	}
	full, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}

	// Pretend the run stopped at the first chunk boundary after 20 s.
	path := filepath.Join(p.LogsDir(), "words.json")
	stamp, _ := stampOf(source)
	file, words, frames, ok := readTranscriptFile(path, stamp, filepath.Base(base.ASRModel))
	if !ok {
		t.Fatal("cannot read the transcript")
	}
	cut := 20.0
	var early []Cue
	for _, w := range words {
		if w.End <= cut {
			early = append(early, w)
		}
	}
	file.Parts, file.Words = [][2]PyFloat{{0, PyFloat(cut)}}, storedWords(early)
	if err := writeTranscript(path, file, frames); err != nil {
		t.Fatal(err)
	}
	if got, done := Coverage(source, base.ASRModel); done || got != cut {
		t.Fatalf("partial coverage %v %v", got, done)
	}
	if st := Status(source, base.ASRModel); st.Transcribed || st.Covered != cut {
		t.Errorf("status of a partial transcript %+v", st)
	}

	// A plan inside the covered part needs no more transcription.
	sliced, ok := sliceTranscript(path, stamp, filepath.Base(base.ASRModel), Window{5, 15}, nil)
	if !ok || len(sliced.Words) == 0 {
		t.Fatalf("slice of a partial transcript failed")
	}

	heard = 0
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	// The 50 s not heard, and hearingPad before them.
	if want := 50 + hearingPad; math.Abs(heard-want) > 0.5 {
		t.Errorf("resumed run heard %.1f s, want about %.1f", heard, want)
	}
	resumed, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	// The same, see TestAPartOfAudioLinesUpWithTheWholeEpisode.
	if len(resumed.Frames) != len(full.Frames) {
		t.Errorf("frames %d after resuming, %d in one go", len(resumed.Frames), len(full.Frames))
	}
	if n, m := len(resumed.Words), len(full.Words); n < m-3 || n > m+3 {
		t.Errorf("words %d after resuming, %d in one go", n, m)
	}
	if _, done := Coverage(source, base.ASRModel); !done {
		t.Errorf("not finished after resuming")
	}
}

// storedTranscript writes a whole-episode transcript next to a stand-in
// episode file and gives back both paths.
func storedTranscript(t *testing.T, words []Cue, frames []float32, to float64,
	partial bool) (string, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "ep.mp4")
	if err := os.WriteFile(source, []byte("nicht wirklich ein video"), 0o644); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(WorkDir(source), "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	stamp, err := stampOf(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, TranscriptName(nil))
	// Heard from the start to to, of an episode that is longer when the
	// transcription is not done.
	end := to
	if partial {
		end = to + 10
	}
	file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: "modell",
		Mean: PyFloat(-30), Parts: [][2]PyFloat{{0, PyFloat(to)}}, End: PyFloat(end),
		Words: storedWords(words)}
	if err := writeTranscript(path, file, frames); err != nil {
		t.Fatal(err)
	}
	return source, path
}

// A transcript from before numbers stood apart is still the episode's
// transcript. Reading it as stale took the waveform off the clip timeline.
func TestAnOlderTranscriptIsStillRead(t *testing.T) {
	words := []Cue{{0.5, 1.2, "Hallo"}, {1.3, 2, "am15."}}
	source, path := storedTranscript(t, words, make([]float32, 300), 3, false)
	stamp, err := stampOf(source)
	if err != nil {
		t.Fatal(err)
	}
	for version, read := range map[int]bool{0: false, 1: true, transcriptVersion: true,
		transcriptVersion + 1: false} {
		file := transcriptFile{Version: version, Source: stamp, Model: "modell",
			To: 3, Mean: -30, Words: storedWords(words)}
		if err := writeTranscript(path, file, make([]float32, 300)); err != nil {
			t.Fatal(err)
		}
		if _, _, _, ok := readTranscriptFile(path, stamp, "modell"); ok != read {
			t.Errorf("version %d read %v, want %v", version, ok, read)
		}
		if st := Status(source, "modell"); st.TranscriptStale == read {
			t.Errorf("version %d stale %v", version, st.TranscriptStale)
		}
	}

	// A file of version 2 that stopped where a search waited is the one
	// part it heard, up to where carrying on would have started, without
	// the word it left to be heard again.
	file := transcriptFile{Version: 2, Source: stamp, Model: "modell", To: 3, Resume: 2.5,
		Partial: true, Mean: -30, Words: storedWords(append(words, Cue{2.6, 2.9, "noch"}))}
	if err := writeTranscript(path, file, make([]float32, 300)); err != nil {
		t.Fatal(err)
	}
	read, got, _, ok := readTranscriptFile(path, stamp, "modell")
	if parts := read.heard(); !ok || len(parts) != 1 || parts[0] != [2]float64{0, 2.5} || len(got) != 2 {
		t.Errorf("version 2 read as %v with %v", read.heard(), got)
	}
}

func TestTranscriptNames(t *testing.T) {
	if got := TranscriptName(nil); got != "words.json" {
		t.Errorf("whole episode: %s", got)
	}
	if got := TranscriptName(&Window{65.4, 130}); got != "words-65-130.json" {
		t.Errorf("window: %s", got)
	}
	if got := framesPath("/a/logs/words-65-130.json"); got != "/a/logs/words-65-130.frames" {
		t.Errorf("frames file: %s", got)
	}
	// The app keeps a transcript in memory while these files stay as they
	// were, so every file a read of it touches has to be named here.
	files := TranscriptFiles("/a/logs")
	want := []string{"/a/logs/words.json", "/a/logs/words.frames", "/a/logs/corrections.json"}
	if len(files) != len(want) {
		t.Fatalf("files %v", files)
	}
	for i, name := range want {
		if filepath.ToSlash(files[i]) != name {
			t.Errorf("file %d is %s, not %s", i, files[i], name)
		}
	}
}

func TestTheCacheOnlyServesItsOwnEpisode(t *testing.T) {
	words := []Cue{{0.1, 0.5, "eins"}, {1, 1.4, "zwei"}, {2, 2.4, "drei"}}
	source, path := storedTranscript(t, words, frames(3, Span{0, 3}), 3, false)
	stamp, err := stampOf(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := readTranscriptFile(path, stamp, "modell"); !ok {
		t.Fatal("its own transcript was not accepted")
	}
	// Another episode, another model, another version of the format, and an
	// episode that was edited after it was transcribed.
	other := stamp
	other.Name = "andere.mp4"
	edited := stamp
	edited.Size += 1
	touched := stamp
	touched.Modified = "2020-01-01T00:00:00Z"
	for name, s := range map[string]sourceStamp{"another episode": other,
		"an edited episode": edited, "a touched episode": touched} {
		if _, _, _, ok := readTranscriptFile(path, s, "modell"); ok {
			t.Errorf("%s was served the cache", name)
		}
	}
	if _, _, _, ok := readTranscriptFile(path, stamp, "anderes-modell"); ok {
		t.Errorf("another model was served the cache")
	}
	// A frames file that went missing means transcribing again, not half a
	// transcript.
	if err := os.Remove(framesPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := readTranscriptFile(path, stamp, "modell"); ok {
		t.Errorf("a transcript without its loudness frames was served")
	}
}

func TestCoverageAndSlicing(t *testing.T) {
	words := []Cue{{0.1, 0.5, "eins"}, {1, 1.4, "zwei"}, {2, 2.4, "drei"}, {2.5, 2.9, "vier"}}
	sound := frames(3, Span{0, 3})
	source, path := storedTranscript(t, words, sound, 3, false)
	model := filepath.Join(t.TempDir(), "modell")
	if got, done := Coverage(source, model); got != 3 || !done {
		t.Errorf("coverage %v %v", got, done)
	}
	stamp, _ := stampOf(source)

	// A window inside the transcript is cut out of it, with the noise floor
	// of the whole episode and the words that fit entirely inside.
	sliced, ok := sliceTranscript(path, stamp, "modell", Window{1, 2.5}, nil)
	if !ok {
		t.Fatal("slicing failed")
	}
	if len(sliced.Words) != 2 || sliced.Words[0].Text != "zwei" {
		t.Errorf("words %v", sliced.Words)
	}
	if sliced.Start != 1 || len(sliced.Frames) != 150 {
		t.Errorf("slice starts at %v with %d frames", sliced.Start, len(sliced.Frames))
	}
	// A window past the end is not served, and neither is an empty one.
	if _, ok := sliceTranscript(path, stamp, "modell", Window{1, 9}, nil); ok {
		t.Errorf("a window past the end was served")
	}
	if _, ok := sliceTranscript(path, stamp, "modell", Window{2, 2}, nil); ok {
		t.Errorf("an empty window was served")
	}

	// A transcription that stopped part way says how far it got, and is
	// never handed out as the whole episode.
	partialSource, partialPath := storedTranscript(t, words[:2], sound[:200], 2, true)
	if got, done := Coverage(partialSource, model); got != 2 || done {
		t.Errorf("partial coverage %v %v", got, done)
	}
	partialStamp, _ := stampOf(partialSource)
	if _, ok := sliceTranscript(partialPath, partialStamp, "modell", Window{0, 3}, nil); ok {
		t.Errorf("a window past what was heard was served")
	}
}

// FuzzReadTranscriptFile throws broken cache files at the reader. A cache is
// a convenience, so anything it cannot vouch for has to come back as a plain
// no, never as half a transcript.
func FuzzReadTranscriptFile(f *testing.F) {
	f.Add(`{"version":2,"source":{"name":"ep.mp4","size":24,"modified":"x"},"model":"m",`+
		`"from":0.0,"to":3.0,"mean":-30.0,"words":[[0.1,0.5,"eins"]]}`, []byte{0, 0, 0, 0})
	f.Add(`{"version":2,"words":[[0.1,"x",5]]}`, []byte{1, 2, 3})
	f.Add(`nicht json`, []byte{})
	f.Fuzz(func(t *testing.T, body string, raw []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "words.json")
		if os.WriteFile(path, []byte(body), 0o644) != nil ||
			os.WriteFile(framesPath(path), raw, 0o644) != nil {
			t.Skip()
		}
		stamp := sourceStamp{Name: "ep.mp4", Size: 24, Modified: "x"}
		file, words, framesRead, ok := readTranscriptFile(path, stamp, "m")
		if !ok {
			return
		}
		if file.Version < oldestTranscript || file.Version > transcriptVersion ||
			file.Source != stamp || file.Model != "m" {
			t.Fatalf("served a cache for %+v", file)
		}
		if len(framesRead)*4 != len(raw) {
			t.Fatalf("%d frames out of %d bytes", len(framesRead), len(raw))
		}
		for i, w := range words {
			if !isFinite(w.Start) || !isFinite(w.End) {
				t.Fatalf("word %d is timed %v-%v", i, w.Start, w.End)
			}
		}
		// What comes back has to survive being used.
		from := fromStored(words, framesRead, float64(file.From), float64(file.Mean), nil)
		if from == nil || len(from.Words) != len(words) {
			t.Fatalf("%d words became %v", len(words), from)
		}
	})
}

// beatEpisode is a test episode whose tone is gated on and off five times a
// second. A steady tone measures the same wherever you start, so a start
// that is a few milliseconds out would hide in it.
func beatEpisode(t *testing.T, seconds string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	path := filepath.Join(t.TempDir(), "beats.mp4")
	out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=320x180:r=25:d="+seconds,
		"-f", "lavfi", "-i", "aevalsrc='sin(2*PI*440*t)*lt(mod(t,0.4),0.2)':s=44100:d="+seconds,
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	return path
}

// Starting part way in has to give exactly the audio the whole episode has
// there, or every word after that point carries a time that is out by the
// difference. A part is read with a seek, see audioFrom, which lands on the
// sample: what differs is the last digits of a float, a hundred thousandth
// of a dB, where a shift of a single frame is a dB and more on this tone.
func TestAPartOfAudioLinesUpWithTheWholeEpisode(t *testing.T) {
	source := beatEpisode(t, "12")
	var calls int32
	rec := fakeRecognizer{&calls}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))

	whole, err := e.Transcribe(context.Background(), source, Window{0, 12}, rec, nil)
	if err != nil {
		t.Fatal(err)
	}
	part, err := e.Transcribe(context.Background(), source, Window{4, 12}, rec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if part.Start != 4 {
		t.Errorf("the part starts at %v", part.Start)
	}
	const offset = 400 // 4 seconds of 10 ms frames
	if len(whole.Frames)-offset != len(part.Frames) {
		t.Fatalf("%d frames from 4 s on, %d in the whole episode",
			len(part.Frames), len(whole.Frames))
	}
	// How many frames of the part read otherwise than the whole episode
	// does at the same moment, shifted by some frames.
	differ := func(shift int) int {
		n := 0
		for i := range part.Frames {
			at := offset + shift + i
			if at < 0 || at >= len(whole.Frames) || math.Abs(float64(whole.Frames[at]-part.Frames[i])) > 0.01 {
				n++
			}
		}
		return n
	}
	// A frame or two on an edge of the tone come out of the decoder a dB
	// or so apart, the way they do for the loudness, see
	// TestMeasureLevelsGoesWhereTheClipTimelineLooks. A shift differs on
	// every edge, five times a second.
	if n := differ(0); n > len(part.Frames)/100 {
		t.Errorf("%d frames of %d read otherwise than the whole episode at the same moment", n, len(part.Frames))
	}
	for _, shift := range []int{-2, -1, 1, 2} {
		if n := differ(shift); n < len(part.Frames)/10 {
			t.Errorf("shifted by %d frames only %d differ: the tone is too even for a shift to show", shift, n)
		}
	}
}

// watchingRecognizer runs a hook after every chunk it hears.
type watchingRecognizer struct {
	fakeRecognizer
	seen func()
}

func (w watchingRecognizer) Recognize(samples []float32, rate int) []Token {
	out := w.fakeRecognizer.Recognize(samples, rate)
	w.seen()
	return out
}

// A transcription says on disk how far it has come while it runs, not only
// when it finishes. The app waits for exactly that before it looks for the
// first clips, so an episode of four hours must not keep its progress to
// itself until the end.
func TestATranscriptionSaysHowFarItHasComeWhileItRuns(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = 0
	defer func() { checkpointEvery = saved }()

	model := t.TempDir()
	var calls int32
	var seen []float64
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return watchingRecognizer{fakeRecognizer{&calls}, func() {
			st := Status(source, model)
			if st.Covered > 0 && !st.Transcribed {
				seen = append(seen, st.Covered)
			}
		}}, nil
	}
	base := DefaultOptions()
	base.ASRModel = model
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}

	if len(seen) == 0 {
		t.Fatal("the transcription said nothing about itself until it was finished")
	}
	if seen[len(seen)-1] <= seen[0] {
		t.Errorf("coverage did not grow while it ran: %v", seen)
	}
	if st := Status(source, model); !st.Transcribed || st.Covered < 69.9 {
		t.Errorf("status when it is done %+v", st)
	}
}

// A transcription also says how far it has come in its events, on every
// chunk it hears rather than only when it saves. Saving rewrites the whole
// transcript, so it happens seconds apart, and a range picker that followed
// the file alone would step across the window instead of moving with the
// work.
func TestEveryChunkSaysHowFarTheAudioHasBeenHeard(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = time.Hour
	defer func() { checkpointEvery = saved }()

	var mu sync.Mutex
	var reached []float64
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev Event) {
		if ev.Kind != EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		reached = append(reached, ev.Covered)
		mu.Unlock()
	})
	var calls int32
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}

	// Nothing was saved in the whole run, so anything watching the file saw
	// no progress at all. The events still have to say where it got to.
	if len(reached) < 2 {
		t.Fatalf("a 70 s episode reported %d moments, %v", len(reached), reached)
	}
	for i := 1; i < len(reached); i++ {
		if reached[i] <= reached[i-1] {
			t.Errorf("the second reported went backwards: %v", reached)
			break
		}
	}
	if last := reached[len(reached)-1]; last < 69.9 || last > 70.1 {
		t.Errorf("the last moment reported is %v, the episode is 70 s", last)
	}
}

// A transcription that is stopped writes down everything it heard before it
// goes, not only what the last save held. A search pauses it, and carrying
// on afterwards has to start where the work really got to, or minutes of
// audio are heard twice and the range picker's edge stands still for them.
func TestAStoppedTranscriptionKeepsWhatItHeard(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = time.Hour
	defer func() { checkpointEvery = saved }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	heard := 0.0
	log := NewLog(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev Event) {
		if ev.Kind != EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		heard = ev.Covered
		if heard > 20 {
			cancel()
		}
	})
	var calls int32
	e := NewEngine(log)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(ctx); err == nil {
		t.Fatal("a stopped transcription finished")
	}
	mu.Lock()
	want := heard
	mu.Unlock()
	covered, done := Coverage(source, base.ASRModel)
	if done || want <= 20 || covered < want-0.001 {
		t.Errorf("stopped after hearing %.2f s, %.2f s written down (done %v)", want, covered, done)
	}
}

// A transcript whose loudness file falls short of what its words say they
// cover carries on from where the loudness ends, and keeps no word past
// that point, so nothing is heard twice. The two files are written one
// after the other, so a machine that stops in between, or a copy made by
// hand, can leave them out of step.
func TestCarryingOnFromFilesOutOfStep(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = 0
	defer func() { checkpointEvery = saved }()
	var calls int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	full, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.LogsDir(), "words.json")
	stamp, _ := stampOf(source)
	file, words, frames, ok := readTranscriptFile(path, stamp, filepath.Base(base.ASRModel))
	if !ok {
		t.Fatal("cannot read the transcript")
	}
	// The words say 40 s, the loudness only 20 s.
	var early []Cue
	for _, w := range words {
		if w.End <= 40 {
			early = append(early, w)
		}
	}
	file.Parts, file.Words = [][2]PyFloat{{0, 40}}, storedWords(early)
	if err := writeTranscript(path, file, frames[:2000]); err != nil {
		t.Fatal(err)
	}
	if covered, _ := Coverage(source, base.ASRModel); covered != 20 {
		t.Errorf("reads as heard to %v, the loudness reaches 20 s", covered)
	}
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	again, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	if n, m := len(again.Words), len(full.Words); n < m-3 || n > m+3 {
		t.Errorf("%d words after carrying on, %d in one go: words were heard twice", n, m)
	}
	for i := 1; i < len(again.Words); i++ {
		if again.Words[i].Start < again.Words[i-1].Start-0.001 {
			t.Errorf("words out of order at %d: %v after %v", i, again.Words[i], again.Words[i-1])
			break
		}
	}
}

// A transcript that cannot be read, cut short or damaged, is started again
// rather than failing the episode.
func TestADamagedTranscriptIsStartedAgain(t *testing.T) {
	source := testEpisode(t, "40")
	var calls int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	path := filepath.Join(p.LogsDir(), "words.json")
	whole, _ := os.ReadFile(path)
	for name, damage := range map[string]func() error{
		"cut short":      func() error { return os.WriteFile(path, whole[:len(whole)/2], 0o644) },
		"not json":       func() error { return os.WriteFile(path, []byte("\x00\x01garbage"), 0o644) },
		"frames missing": func() error { return os.Remove(framesPath(path)) },
		"frames odd":     func() error { return os.WriteFile(framesPath(path), []byte{1, 2, 3}, 0o644) },
	} {
		if err := damage(); err != nil {
			t.Fatal(err)
		}
		if covered, _ := Coverage(source, base.ASRModel); covered != 0 {
			t.Errorf("%s: a damaged transcript reads as covering %v", name, covered)
		}
		if err := p.Transcribe(context.Background()); err != nil {
			t.Errorf("%s: %v %s", name, err, p.LastError())
		}
		if covered, done := Coverage(source, base.ASRModel); !done || covered < 39.9 {
			t.Errorf("%s: after starting again, %v %v", name, covered, done)
		}
		whole, _ = os.ReadFile(path)
	}
}

// The app reads how far the transcript reaches from other goroutines while
// the transcription writes it, a search waiting for it and the workspace
// drawing it. A reader never sees a file half written: what it reads only
// ever grows, and a read is never taken for the end of the transcription.
func TestTheTranscriptIsReadWhileItIsWritten(t *testing.T) {
	source := testEpisode(t, "70")
	saved := checkpointEvery
	checkpointEvery = 0
	defer func() { checkpointEvery = saved }()
	var calls int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var problems []string
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			last := 0.0
			for {
				select {
				case <-stop:
					return
				default:
				}
				covered, done := Coverage(source, base.ASRModel)
				st := Status(source, base.ASRModel)
				mu.Lock()
				if covered < last-0.001 {
					problems = append(problems, fmt.Sprintf("coverage went back from %v to %v", last, covered))
				}
				if done && covered < 69.9 {
					problems = append(problems, fmt.Sprintf("done at %v of 70 s", covered))
				}
				if st.Transcribed && st.Covered < 69.9 {
					problems = append(problems, fmt.Sprintf("status transcribed at %v", st.Covered))
				}
				mu.Unlock()
				last = max(last, covered)
			}
		}()
	}
	err := p.Transcribe(context.Background())
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if len(problems) > 0 {
		t.Errorf("%d bad reads, the first: %s", len(problems), problems[0])
	}
}

// straddlingRecognizer hears a word every 0.6 s to the very end of what it
// is given, so the last one can run past the end, the way a real model
// hears half a word as a word when the audio is cut inside it.
type straddlingRecognizer struct{ seconds *float64 }

func (s straddlingRecognizer) Recognize(samples []float32, rate int) []Token {
	length := float64(len(samples)) / float64(rate)
	*s.seconds += length
	var tokens []Token
	for at := 0.1; at < length; at += 0.6 {
		tokens = append(tokens, Token{Text: " wort", Start: at, Duration: 0.3})
	}
	return tokens
}

func (straddlingRecognizer) Close() {}

// beatRecognizer hears a word wherever the tone of a beatEpisode sounds,
// five times a second, so its words are where the sound is, whatever part
// of the audio it is given, the way a real model's are. A beat cut by the
// edge of what it is given is heard as part of a word, the way a real
// model hears half a word.
type beatRecognizer struct{ seconds *float64 }

func (b beatRecognizer) Recognize(samples []float32, rate int) []Token {
	*b.seconds += float64(len(samples)) / float64(rate)
	var tokens []Token
	step := rate / 100
	start := -1
	for i := 0; i+step <= len(samples); i += step {
		power := 0.0
		for _, v := range samples[i : i+step] {
			power += float64(v) * float64(v)
		}
		loud := power/float64(step) > 0.01
		switch {
		case loud && start < 0:
			start = i
		case !loud && start >= 0:
			tokens = append(tokens, Token{Text: " ton", Start: float64(start) / float64(rate),
				Duration: float64(i-start) / float64(rate)})
			start = -1
		}
	}
	if start >= 0 {
		tokens = append(tokens, Token{Text: " ton", Start: float64(start) / float64(rate),
			Duration: float64(len(samples)-start) / float64(rate)})
	}
	return tokens
}

func (beatRecognizer) Close() {}

// The transcript is heard in parts, where it is needed first, and they
// meet with every word once. The parts here start and end inside a beat,
// so each join has a word across it, heard whole on one side and in part
// on the other.
func TestPartsMeetWithEveryWordOnce(t *testing.T) {
	source := beatEpisode(t, "12")
	heard := 0.0
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return beatRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	ctx := context.Background()

	if err := p.Hear(ctx, Window{4.1, 6.1}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{4.1, 6.1} {
		t.Fatalf("heard %v", parts)
	}
	// Only the part and the audio either side of it, not from the start.
	if want := 2 + 2*hearingPad; math.Abs(heard-want) > 0.05 {
		t.Errorf("heard %.2f s of audio for a part of 2 s, want %.2f", heard, want)
	}
	if covered, done := Coverage(source, base.ASRModel); covered != 0 || done {
		t.Errorf("a part in the middle reads as heard from the start to %v", covered)
	}
	if err := p.Hear(ctx, Window{0, 3}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(source, base.ASRModel); len(parts) != 2 {
		t.Fatalf("heard %v", parts)
	}
	if gaps := Unheard(source, base.ASRModel, Window{0, 12}); len(gaps) != 2 ||
		gaps[0] != (Window{3, 4.1}) || gaps[1] != (Window{6.1, 12}) {
		t.Errorf("unheard %v", gaps)
	}
	// What is heard already is not heard again.
	heard = 0
	if err := p.Hear(ctx, Window{0.5, 5}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if want := 1.1 + 2*hearingPad; math.Abs(heard-want) > 0.05 {
		t.Errorf("heard %.2f s for the 1.1 s between two parts, want %.2f", heard, want)
	}
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if _, done := Coverage(source, base.ASRModel); !done {
		t.Fatalf("not done, heard %v", HeardParts(source, base.ASRModel))
	}
	whole, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	// A beat every 0.4 s, each heard once, whole, where it is.
	if len(whole.RawWords) != 30 {
		t.Errorf("%d words for 30 beats: %v", len(whole.RawWords), whole.RawWords)
	}
	for i, w := range whole.RawWords {
		at := float64(i) * 0.4
		if math.Abs(w.Start-at) > 0.03 || math.Abs(w.End-(at+0.2)) > 0.03 {
			t.Errorf("word %d is %.2f to %.2f, the beat is %.2f to %.2f", i, w.Start, w.End, at, at+0.2)
		}
	}
}

// What is added to a transcript lands whole even when two hearings of the
// same part write at once: each adds only what the other has not.
func TestTwoHearingsOfOnePartAtOnce(t *testing.T) {
	source := beatEpisode(t, "12")
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i, span := range []Window{{2, 8}, {2, 8}, {5, 12}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			heard := 0.0
			e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
			e.OpenRecognizer = func(string) (Recognizer, error) { return beatRecognizer{&heard}, nil }
			errs[i] = NewProject(e, source, base).Hear(context.Background(), span)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("hearing %d: %v", i, err)
		}
	}
	if parts := HeardParts(source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{2, 12} {
		t.Fatalf("heard %v", parts)
	}
	stamp, _ := stampOf(source)
	_, words, _, _ := readTranscriptFile(filepath.Join(WorkDir(source), "logs", "words.json"), stamp,
		filepath.Base(base.ASRModel))
	if len(words) != 25 {
		t.Errorf("%d words for the 25 beats from 2 s on: %v", len(words), words)
	}
}

// A file is replaced whole or not at all: a write its check refuses
// leaves the file that was there, and nothing beside it.
func TestAFileIsReplacedWholeOrNotAtAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	if err := writeAtomic(path, []byte("before")); err != nil {
		t.Fatal(err)
	}
	refuse := func(string) error { return errors.New("not a plan") }
	if err := replaceFile(path, ".plan-*", []byte("after"), refuse); err == nil {
		t.Fatal("a refused write went through")
	}
	if got, _ := os.ReadFile(path); string(got) != "before" {
		t.Errorf("the file is now %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*")); len(left) != 0 {
		t.Errorf("left beside it: %v", left)
	}
	if err := writeAtomic(path, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "after" {
		t.Errorf("the file is now %q", got)
	}
}
