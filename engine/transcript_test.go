package engine

import (
	"bytes"
	"context"
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
	file.Partial, file.To, file.Words = true, PyFloat(cut), storedWords(early)
	if err := writeTranscript(path, file, frames[:2000]); err != nil {
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
	if heard > 52 || heard < 48 {
		t.Errorf("resumed run heard %.1f s, want about 50", heard)
	}
	resumed, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}
	// Exactly the same, because carrying on drops samples by count rather
	// than asking ffmpeg to seek. TestAPartOfAudioLinesUpWithTheWholeEpisode
	// covers why that matters.
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
	file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: "modell",
		To: PyFloat(to), Mean: PyFloat(-30), Partial: partial, Words: storedWords(words)}
	if err := writeTranscript(path, file, frames); err != nil {
		t.Fatal(err)
	}
	return source, path
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
	if _, ok := readTranscript(partialPath, partialStamp, "modell", Window{0, 2}, nil); ok {
		t.Errorf("a partial transcript was served as a whole one")
	}
}

// FuzzReadTranscriptFile throws broken cache files at the reader. A cache is
// a convenience, so anything it cannot vouch for has to come back as a plain
// no, never as half a transcript.
func FuzzReadTranscriptFile(f *testing.F) {
	f.Add(`{"version":1,"source":{"name":"ep.mp4","size":24,"modified":"x"},"model":"m",`+
		`"from":0.0,"to":3.0,"mean":-30.0,"words":[[0.1,0.5,"eins"]]}`, []byte{0, 0, 0, 0})
	f.Add(`{"version":1,"words":[[0.1,"x",5]]}`, []byte{1, 2, 3})
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
		if file.Version != transcriptVersion || file.Source != stamp || file.Model != "m" {
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
		"-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	return path
}

// TestAPartOfAudioLinesUpWithTheWholeEpisode is the reason the engine
// counts samples itself instead of asking ffmpeg to seek. Starting part way
// in has to give exactly the audio the whole episode has there, or every
// word after that point carries a time that is out by the difference.
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
	differsAt := func(shift int) int {
		for i := range part.Frames {
			at := offset + shift + i
			if at < 0 || at >= len(whole.Frames) || whole.Frames[at] != part.Frames[i] {
				return i
			}
		}
		return -1
	}
	if i := differsAt(0); i >= 0 {
		t.Errorf("frame %d of the part reads %v, the whole episode has %v at the same moment",
			i, part.Frames[i], whole.Frames[offset+i])
	}
	// And the tone really is uneven enough that a shift would have shown.
	if differsAt(2) < 0 || differsAt(-2) < 0 {
		t.Errorf("the test tone is too even for a shift of 20 ms to show")
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
