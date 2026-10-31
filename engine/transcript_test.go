package engine

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/runlog"
	"framefairy/engine/timed"
	"framefairy/internal/ffmpegtest"
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

// On the real disk: ffmpeg reads the episode the speech model hears.
func TestTranscriptionResumes(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	var calls int32
	heard := 0.0
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.checkpointEvery = time.Nanosecond
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return countingRecognizer{fakeRecognizer{&calls}, &heard}, nil
	}
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	covered, done := Coverage(disk, source, base.ASRModel)
	if !done || covered < 69.9 {
		t.Fatalf("coverage %v %v", covered, done)
	}
	full, err := p.Transcript()
	if err != nil {
		t.Fatal(err)
	}

	// Pretend the run stopped at the first chunk boundary after 20 s.
	path := filepath.Join(p.LogsDir(), "words.json")
	stamp, _ := stampOf(disk, source)
	file, words, frames, ok := readTranscriptFile(disk, path, stamp, filepath.Base(base.ASRModel))
	if !ok {
		t.Fatal("cannot read the transcript")
	}
	cut := 20.0
	var early []timed.Cue
	for _, w := range words {
		if w.End <= cut {
			early = append(early, w)
		}
	}
	file.Parts, file.Words = [][2]num.Float{{0, num.Float(cut)}}, storedWords(early)
	if err := writeTranscript(disk, path, file, frames); err != nil {
		t.Fatal(err)
	}
	if got, done := Coverage(disk, source, base.ASRModel); done || got != cut {
		t.Fatalf("partial coverage %v %v", got, done)
	}
	if st := Status(disk, source, base.ASRModel, nil); st.Transcribed || st.Covered != cut {
		t.Errorf("status of a partial transcript %+v", st)
	}

	// A plan inside the covered part needs no more transcription.
	sliced, ok := sliceTranscript(disk, path, stamp, filepath.Base(base.ASRModel), timed.Window{Start: 5, End: 15}, nil)
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
	if _, done := Coverage(disk, source, base.ASRModel); !done {
		t.Errorf("not finished after resuming")
	}
}

// storedTranscript writes a whole-episode transcript next to a stand-in
// episode file and gives back both paths.
func storedTranscript(t *testing.T, words []timed.Cue, frames []float32, to float64,
	partial bool) (afero.Fs, string, string) {
	t.Helper()
	fsys := afero.NewMemMapFs()
	source := "/ep.mp4"
	if err := afero.WriteFile(fsys, source, []byte("nicht wirklich ein video"), 0o644); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(WorkDir(source), "logs")
	stamp, err := stampOf(fsys, source)
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
		Mean: num.Float(-30), Parts: [][2]num.Float{{0, num.Float(to)}}, End: num.Float(end),
		Words: storedWords(words)}
	if err := writeTranscript(fsys, path, file, frames); err != nil {
		t.Fatal(err)
	}
	return fsys, source, path
}

// A transcript from before numbers stood apart is still the episode's
// transcript. Reading it as stale took the waveform off the clip timeline.
func TestAnOlderTranscriptIsStillRead(t *testing.T) {
	words := []timed.Cue{{Start: 0.5, End: 1.2, Text: "Hallo"}, {Start: 1.3, End: 2, Text: "am15."}}
	fsys, source, path := storedTranscript(t, words, make([]float32, 300), 3, false)
	stamp, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	for version, read := range map[int]bool{0: false, 1: true, transcriptVersion: true,
		transcriptVersion + 1: false} {
		file := transcriptFile{Version: version, Source: stamp, Model: "modell",
			To: 3, Mean: -30, Words: storedWords(words)}
		if err := writeTranscript(fsys, path, file, make([]float32, 300)); err != nil {
			t.Fatal(err)
		}
		if _, _, _, ok := readTranscriptFile(fsys, path, stamp, "modell"); ok != read {
			t.Errorf("version %d read %v, want %v", version, ok, read)
		}
		if st := Status(fsys, source, "modell", nil); st.TranscriptStale == read {
			t.Errorf("version %d stale %v", version, st.TranscriptStale)
		}
	}

	// A file of version 2 that stopped where a search waited is the one
	// part it heard, up to where carrying on would have started, without
	// the word it left to be heard again.
	file := transcriptFile{Version: 2, Source: stamp, Model: "modell", To: 3, Resume: 2.5,
		Partial: true, Mean: -30, Words: storedWords(append(words, timed.Cue{Start: 2.6, End: 2.9, Text: "noch"}))}
	if err := writeTranscript(fsys, path, file, make([]float32, 300)); err != nil {
		t.Fatal(err)
	}
	read, got, _, ok := readTranscriptFile(fsys, path, stamp, "modell")
	if parts := read.heard(); !ok || len(parts) != 1 || parts[0] != [2]float64{0, 2.5} || len(got) != 2 {
		t.Errorf("version 2 read as %v with %v", read.heard(), got)
	}
}

func TestTranscriptNames(t *testing.T) {
	if got := TranscriptName(nil); got != "words.json" {
		t.Errorf("whole episode: %s", got)
	}
	if got := TranscriptName(&timed.Window{Start: 65.4, End: 130}); got != "words-65-130.json" {
		t.Errorf("window: %s", got)
	}
	if got := framesPath("/a/logs/words-65-130.json"); got != "/a/logs/words-65-130.frames" {
		t.Errorf("frames file: %s", got)
	}
	// The app keeps a transcript in memory while these files stay as they
	// were, so every file a read of it touches has to be named here.
	files := TranscriptFiles("/a/logs")
	want := []string{"/a/logs/words.json", "/a/logs/words.frames"}
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
	words := []timed.Cue{{Start: 0.1, End: 0.5, Text: "eins"}, {Start: 1, End: 1.4, Text: "zwei"}, {Start: 2, End: 2.4, Text: "drei"}}
	fsys, source, path := storedTranscript(t, words, frames(3, timed.Span{Start: 0, End: 3}), 3, false)
	stamp, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := readTranscriptFile(fsys, path, stamp, "modell"); !ok {
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
		if _, _, _, ok := readTranscriptFile(fsys, path, s, "modell"); ok {
			t.Errorf("%s was served the cache", name)
		}
	}
	if _, _, _, ok := readTranscriptFile(fsys, path, stamp, "anderes-modell"); ok {
		t.Errorf("another model was served the cache")
	}
	// A frames file that went missing means transcribing again, not half a
	// transcript.
	if err := fsys.Remove(framesPath(path)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := readTranscriptFile(fsys, path, stamp, "modell"); ok {
		t.Errorf("a transcript without its loudness frames was served")
	}
}

func TestCoverageAndSlicing(t *testing.T) {
	words := []timed.Cue{{Start: 0.1, End: 0.5, Text: "eins"}, {Start: 1, End: 1.4, Text: "zwei"}, {Start: 2, End: 2.4, Text: "drei"}, {Start: 2.5, End: 2.9, Text: "vier"}}
	sound := frames(3, timed.Span{Start: 0, End: 3})
	fsys, source, path := storedTranscript(t, words, sound, 3, false)
	model := "/home/tim/.framefairy/models/modell"
	if got, done := Coverage(fsys, source, model); got != 3 || !done {
		t.Errorf("coverage %v %v", got, done)
	}
	stamp, _ := stampOf(fsys, source)

	// A window inside the transcript is cut out of it, with the noise floor
	// of the whole episode and the words that fit entirely inside.
	sliced, ok := sliceTranscript(fsys, path, stamp, "modell", timed.Window{Start: 1, End: 2.5}, nil)
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
	if _, ok := sliceTranscript(fsys, path, stamp, "modell", timed.Window{Start: 1, End: 9}, nil); ok {
		t.Errorf("a window past the end was served")
	}
	if _, ok := sliceTranscript(fsys, path, stamp, "modell", timed.Window{Start: 2, End: 2}, nil); ok {
		t.Errorf("an empty window was served")
	}

	// A transcription that stopped part way says how far it got, and is
	// never handed out as the whole episode.
	partialFS, partialSource, partialPath := storedTranscript(t, words[:2], sound[:200], 2, true)
	if got, done := Coverage(partialFS, partialSource, model); got != 2 || done {
		t.Errorf("partial coverage %v %v", got, done)
	}
	partialStamp, _ := stampOf(partialFS, partialSource)
	if _, ok := sliceTranscript(partialFS, partialPath, partialStamp, "modell", timed.Window{Start: 0, End: 3}, nil); ok {
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
	// A blank word and a word of two are heard words like any other: the
	// words said leave the one out and split the other.
	f.Add(`{"version":2,"source":{"name":"ep.mp4","size":24,"modified":"x"},"model":"m",`+
		`"from":0.0,"to":3.0,"mean":-30.0,"words":[[0,5," "]]}`, []byte{0, 0, 0, 0})
	f.Add(`{"version":2,"source":{"name":"ep.mp4","size":24,"modified":"x"},"model":"m",`+
		`"from":0.0,"to":3.0,"mean":-30.0,"words":[[0,5,"eins zwei"]]}`, []byte{0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, body string, raw []byte) {
		fsys := afero.NewMemMapFs()
		path := "/ep.framefairy/logs/words.json"
		if afero.WriteFile(fsys, path, []byte(body), 0o644) != nil ||
			afero.WriteFile(fsys, framesPath(path), raw, 0o644) != nil {
			t.Skip()
		}
		stamp := sourceStamp{Name: "ep.mp4", Size: 24, Modified: "x"}
		file, words, framesRead, ok := readTranscriptFile(fsys, path, stamp, "m")
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
			if !num.IsFinite(w.Start) || !num.IsFinite(w.End) {
				t.Fatalf("word %d is timed %v-%v", i, w.Start, w.End)
			}
		}
		// What comes back has to survive being used. Every word read is a
		// word heard. The words said are not counted: they leave a blank
		// word out and split a word of several, see Correct.
		from := fromStored(words, framesRead, float64(file.From), float64(file.Mean), nil)
		if from == nil || len(from.HeardWords) != len(words) {
			t.Fatalf("%d words became %v", len(words), from)
		}
	})
}

// beatEpisode is a test episode whose tone is gated on and off five times a
// second. A steady tone measures the same wherever you start, so a start
// that is a few milliseconds out would hide in it.
//
// On the real disk: ffmpeg makes the episode.
func beatEpisode(t *testing.T, seconds string) string {
	t.Helper()
	ffmpegtest.Need(t)
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestAPartOfAudioLinesUpWithTheWholeEpisode(t *testing.T) {
	t.Parallel()
	source := beatEpisode(t, "12")
	var calls int32
	rec := fakeRecognizer{&calls}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)

	whole, err := e.Transcribe(context.Background(), source, timed.Window{Start: 0, End: 12}, rec, nil)
	if err != nil {
		t.Fatal(err)
	}
	part, err := e.Transcribe(context.Background(), source, timed.Window{Start: 4, End: 12}, rec, nil)
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestATranscriptionSaysHowFarItHasComeWhileItRuns(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	model := t.TempDir()
	var calls int32
	var seen []float64
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.checkpointEvery = time.Nanosecond
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return watchingRecognizer{fakeRecognizer{&calls}, func() {
			st := Status(disk, source, model, nil)
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
	if st := Status(disk, source, model, nil); !st.Transcribed || st.Covered < 69.9 {
		t.Errorf("status when it is done %+v", st)
	}
}

// A transcription also says how far it has come in its events, on every
// chunk it hears rather than only when it saves. Saving rewrites the whole
// transcript, so it happens seconds apart, and a range picker that followed
// the file alone would step across the window instead of moving with the
// work.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestEveryChunkSaysHowFarTheAudioHasBeenHeard(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	var mu sync.Mutex
	var reached []float64
	log := runlog.New(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev runlog.Event) {
		if ev.Kind != runlog.EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		reached = append(reached, ev.Covered)
		mu.Unlock()
	})
	var calls int32
	e := NewEngine(log, disk)
	e.checkpointEvery = time.Hour
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

// What says when a transcription saves decides it, not the clock: a walk
// that holds the bridge's speech model lets it hear a piece at a time, far
// quicker than the seconds between two saves, and still sees a transcript
// saved part way, on any machine.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestSaveHeardDecidesWhenATranscriptionSaves(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	model := t.TempDir()
	var calls int32
	// How far the saved transcript reached as each piece was heard.
	var seen []float64
	var asked atomic.Int32
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	// The clock alone would save after every piece.
	e.checkpointEvery = time.Nanosecond
	e.OpenRecognizer = func(string) (Recognizer, error) {
		return savingRecognizer{watchingRecognizer{fakeRecognizer{&calls}, func() {
			seen = append(seen, Status(disk, source, model, nil).Covered)
		}}, func() bool { return asked.Add(1) == 2 }}, nil
	}
	base := DefaultOptions()
	base.ASRModel = model
	p := NewProject(e, source, base)
	if err := p.Transcribe(context.Background()); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}

	if len(seen) < 3 {
		t.Fatalf("a 70 s episode was heard in %d pieces, too few to save between", len(seen))
	}
	if seen[0] != 0 || seen[1] != 0 {
		t.Errorf("the transcript was saved before the second piece was heard: %v", seen)
	}
	for _, c := range seen[2:] {
		if c <= 0 || c != seen[2] {
			t.Errorf("saved after the second piece only, the transcript reached %v as the pieces were heard", seen)
			break
		}
	}
}

// savingRecognizer says when what is heard so far is saved, the way the
// walks' stand-in does.
type savingRecognizer struct {
	watchingRecognizer
	saves func() bool
}

func (r savingRecognizer) SaveHeard() bool { return r.saves() }

// Issue #444: Tim searched windows heard in part already, and the fill of
// the transcription started well past 0. What a hearing says of how far it
// has come is a share of what it hears, the parts of the window not heard
// yet, so a window heard half already starts at 0 and ends at 1 all the
// same, and each moment it reports is that far through the part left.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestHearingAWindowHeardInPartFillsFromZero(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	var mu sync.Mutex
	type moment struct{ fraction, covered float64 }
	var said []moment
	listening := false
	log := runlog.New(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev runlog.Event) {
		if ev.Kind != runlog.EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if listening {
			said = append(said, moment{ev.Fraction, ev.Covered})
		}
	})
	var calls int32
	e := NewEngine(log, disk)
	e.checkpointEvery = time.Hour
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Hear(context.Background(), timed.Window{Start: 0, End: 35}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	gaps := Unheard(disk, source, base.ASRModel, timed.Window{Start: 0, End: 70})
	if len(gaps) != 1 {
		t.Fatalf("after hearing the first half, what is left is %v, not one part", gaps)
	}
	left := gaps[0]
	mu.Lock()
	listening = true
	mu.Unlock()
	if err := p.Hear(context.Background(), timed.Window{Start: 0, End: 70}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(said) < 2 {
		t.Fatalf("hearing the second half reported %d moments, %v", len(said), said)
	}
	for i, m := range said {
		want := math.Max(m.covered-left.Start, 0) / (left.End - left.Start)
		if math.Abs(m.fraction-want) > 0.01 {
			t.Errorf("at %.2f s it said %.3f, not %.3f, how far it is through %v: %v", m.covered, m.fraction, want, left, said)
			break
		}
		if i > 0 && m.fraction < said[i-1].fraction {
			t.Errorf("it went back from %.3f to %.3f: %v", said[i-1].fraction, m.fraction, said)
			break
		}
	}
	// The share of the window heard is never what it says: the half
	// heard before counts for nothing.
	if first := said[0]; first.fraction > first.covered/70-0.1 {
		t.Errorf("at %.2f s it said %.3f, about the share of the window heard: %v", first.covered, first.fraction, said)
	}
	if last := said[len(said)-1].fraction; last < 0.99 {
		t.Errorf("it ended at %.3f, not 1: %v", last, said)
	}
}

// Issue #444, from the review of #456: a search that gives the lane of
// hearing back, to a clip made by hand that needs words sooner or to a
// search that finds, and gets it again, hears on in the same run. What it
// says of how far it has come goes on from where it got to, over all it
// set out to hear, and never starts from nothing again.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestAHearingThatLosesItsTurnCountsOnFromWhereItWas(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")

	var mu sync.Mutex
	type moment struct {
		turn              int
		fraction, covered float64
	}
	var said []moment
	turns := 0
	var takeBack context.CancelFunc
	log := runlog.New(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev runlog.Event) {
		if ev.Kind != runlog.EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		said = append(said, moment{turns, ev.Fraction, ev.Covered})
		// The lane is taken back the first time the hearing says how far
		// it has come, the way a clip made by hand takes it.
		if takeBack != nil {
			takeBack()
			takeBack = nil
		}
	})
	var calls int32
	e := NewEngine(log, disk)
	e.checkpointEvery = time.Hour
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&calls}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	turn := func(ctx context.Context, step string) (context.Context, func(), error) {
		c, cancel := context.WithCancel(ctx)
		mu.Lock()
		turns++
		if turns == 1 {
			takeBack = cancel
		}
		mu.Unlock()
		return c, cancel, nil
	}
	letGo, err := p.hear(context.Background(), turn, timed.Window{Start: 0, End: 70})
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	letGo()

	mu.Lock()
	defer mu.Unlock()
	if turns < 2 || len(said) < 3 || said[len(said)-2].turn < 2 {
		t.Fatalf("the hearing took %d turns and said %v, not two turns with more than the end in the second", turns, said)
	}
	for i, m := range said {
		if want := m.covered / 70; math.Abs(m.fraction-want) > 0.01 {
			t.Errorf("in turn %d at %.2f s it said %.3f, not %.3f, how far it is through the 70 s it set out to hear: %v",
				m.turn, m.covered, m.fraction, want, said)
			break
		}
		if i > 0 && m.fraction < said[i-1].fraction {
			t.Errorf("it went back from %.3f to %.3f: %v", said[i-1].fraction, m.fraction, said)
			break
		}
	}
	if last := said[len(said)-1].fraction; last < 0.99 {
		t.Errorf("it ended at %.3f, not 1: %v", last, said)
	}
}

// A transcription that is stopped writes down everything it heard before it
// goes, not only what the last save held. A search pauses it, and carrying
// on afterwards has to start where the work really got to, or minutes of
// audio are heard twice and the range picker's edge stands still for them.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestAStoppedTranscriptionKeepsWhatItHeard(t *testing.T) {
	t.Parallel()
	var calls int32
	stoppedKeepsWhatItHeard(t, fakeRecognizer{&calls})
}

// stoppedKeepsWhatItHeard transcribes a test episode with rec, stops it
// once more than 20 s are heard, and checks all of that was written down.
// It is shared with TestHearingSideBySideStoppedKeepsWhatItHeard.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func stoppedKeepsWhatItHeard(t *testing.T, rec Recognizer) {
	t.Helper()
	source := testEpisode(t, "70")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	heard, stoppedAt := 0.0, 0.0
	log := runlog.New(&bytes.Buffer{}, false, false)
	log.SetSink(func(ev runlog.Event) {
		if ev.Kind != runlog.EventProgress || ev.Covered <= 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		heard = ev.Covered
		if heard > 20 && stoppedAt == 0 {
			stoppedAt = heard
			cancel()
		}
	})
	e := NewEngine(log, disk)
	e.checkpointEvery = time.Hour
	e.OpenRecognizer = func(string) (Recognizer, error) { return rec, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	if err := p.Transcribe(ctx); err == nil {
		t.Fatal("a stopped transcription finished")
	}
	mu.Lock()
	want, said := stoppedAt, heard
	mu.Unlock()
	// What it said it heard as it was stopped is what is kept, and it
	// says nothing more after: the copies still hearing then are let go.
	// Taken in, they moved the range picker's edge after the stop, or
	// were saved without being shown, Tim's report of 24 October.
	covered, done := Coverage(disk, source, base.ASRModel)
	if done || want <= 20 || math.Abs(covered-want) > 0.001 {
		t.Errorf("stopped after hearing %.2f s, %.2f s written down (done %v)", want, covered, done)
	}
	if said != want {
		t.Errorf("stopped after hearing %.2f s, it said it heard %.2f s after", want, said)
	}
}

// A transcript whose loudness file falls short of what its words say they
// cover carries on from where the loudness ends, and keeps no word past
// that point, so nothing is heard twice. The two files are written one
// after the other, so a machine that stops in between, or a copy made by
// hand, can leave them out of step.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestCarryingOnFromFilesOutOfStep(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	var calls int32
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.checkpointEvery = time.Nanosecond
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
	stamp, _ := stampOf(disk, source)
	file, words, frames, ok := readTranscriptFile(disk, path, stamp, filepath.Base(base.ASRModel))
	if !ok {
		t.Fatal("cannot read the transcript")
	}
	// The words say 40 s, the loudness only 20 s.
	var early []timed.Cue
	for _, w := range words {
		if w.End <= 40 {
			early = append(early, w)
		}
	}
	file.Parts, file.Words = [][2]num.Float{{0, 40}}, storedWords(early)
	if err := writeTranscript(disk, path, file, frames[:2000]); err != nil {
		t.Fatal(err)
	}
	if covered, _ := Coverage(disk, source, base.ASRModel); covered != 20 {
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestADamagedTranscriptIsStartedAgain(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var calls int32
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
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
		if covered, _ := Coverage(disk, source, base.ASRModel); covered != 0 {
			t.Errorf("%s: a damaged transcript reads as covering %v", name, covered)
		}
		if err := p.Transcribe(context.Background()); err != nil {
			t.Errorf("%s: %v %s", name, err, p.LastError())
		}
		if covered, done := Coverage(disk, source, base.ASRModel); !done || covered < 39.9 {
			t.Errorf("%s: after starting again, %v %v", name, covered, done)
		}
		whole, _ = os.ReadFile(path)
	}
}

// The app reads how far the transcript reaches from other goroutines while
// the transcription writes it, a search waiting for it and the workspace
// drawing it. A reader never sees a file half written: what it reads only
// ever grows, and a read is never taken for the end of the transcription.
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestTheTranscriptIsReadWhileItIsWritten(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "70")
	var calls int32
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.checkpointEvery = time.Nanosecond
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
				covered, done := Coverage(disk, source, base.ASRModel)
				st := Status(disk, source, base.ASRModel, nil)
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestPartsMeetWithEveryWordOnce(t *testing.T) {
	t.Parallel()
	source := beatEpisode(t, "12")
	heard := 0.0
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.OpenRecognizer = func(string) (Recognizer, error) { return beatRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	ctx := context.Background()

	if err := p.Hear(ctx, timed.Window{Start: 4.1, End: 6.1}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(disk, source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{4.1, 6.1} {
		t.Fatalf("heard %v", parts)
	}
	// Only the part and the audio either side of it, not from the start.
	if want := 2 + 2*hearingPad; math.Abs(heard-want) > 0.05 {
		t.Errorf("heard %.2f s of audio for a part of 2 s, want %.2f", heard, want)
	}
	if covered, done := Coverage(disk, source, base.ASRModel); covered != 0 || done {
		t.Errorf("a part in the middle reads as heard from the start to %v", covered)
	}
	if err := p.Hear(ctx, timed.Window{Start: 0, End: 3}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if parts := HeardParts(disk, source, base.ASRModel); len(parts) != 2 {
		t.Fatalf("heard %v", parts)
	}
	if gaps := Unheard(disk, source, base.ASRModel, timed.Window{Start: 0, End: 12}); len(gaps) != 2 ||
		gaps[0] != (timed.Window{Start: 3, End: 4.1}) || gaps[1] != (timed.Window{Start: 6.1, End: 12}) {
		t.Errorf("unheard %v", gaps)
	}
	// What is heard already is not heard again.
	heard = 0
	if err := p.Hear(ctx, timed.Window{Start: 0.5, End: 5}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if want := 1.1 + 2*hearingPad; math.Abs(heard-want) > 0.05 {
		t.Errorf("heard %.2f s for the 1.1 s between two parts, want %.2f", heard, want)
	}
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if _, done := Coverage(disk, source, base.ASRModel); !done {
		t.Fatalf("not done, heard %v", HeardParts(disk, source, base.ASRModel))
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
//
// On the real disk: ffmpeg reads the episode the speech model hears.
func TestTwoHearingsOfOnePartAtOnce(t *testing.T) {
	t.Parallel()
	source := beatEpisode(t, "12")
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i, span := range []timed.Window{{Start: 2, End: 8}, {Start: 2, End: 8}, {Start: 5, End: 12}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			heard := 0.0
			e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
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
	if parts := HeardParts(disk, source, base.ASRModel); len(parts) != 1 || parts[0] != [2]float64{2, 12} {
		t.Fatalf("heard %v", parts)
	}
	stamp, _ := stampOf(disk, source)
	_, words, _, _ := readTranscriptFile(disk, filepath.Join(WorkDir(source), "logs", "words.json"), stamp,
		filepath.Base(base.ASRModel))
	if len(words) != 25 {
		t.Errorf("%d words for the 25 beats from 2 s on: %v", len(words), words)
	}
}

// A transcript is done when nothing is left to hear by the measure hear
// stops at, a gap too short to hold a word included. Found by the review
// of #458: a gap of 0.05 s left between two parts was never heard, and
// the video never read as transcribed, so Export transcript never came.
func TestATranscriptIsDoneWhenNothingIsLeftToHear(t *testing.T) {
	cases := []struct {
		name  string
		parts [][2]num.Float
		done  bool
	}{
		{"one part to the end", [][2]num.Float{{0, 600}}, true},
		{"a gap too short to hold a word", [][2]num.Float{{0, 100}, {100.05, 600}}, true},
		{"a gap with room for a word", [][2]num.Float{{0, 100}, {100.5, 600}}, false},
		{"short of the end by too little for a word", [][2]num.Float{{0, 599.95}}, true},
		{"short of the end", [][2]num.Float{{0, 590}}, false},
		{"from a little after the start", [][2]num.Float{{0.05, 600}}, true},
		{"nothing heard", nil, false},
	}
	for _, c := range cases {
		file := transcriptFile{Version: transcriptVersion, End: 600, Parts: c.parts}
		if got := file.done(); got != c.done {
			t.Errorf("%s: done is %v", c.name, got)
		}
		if unheard := len(gapsIn(file.heard(), timed.Window{Start: 0, End: 600})) == 0; unheard != c.done && len(c.parts) > 0 {
			t.Errorf("%s: done says %v and what is left to hear disagrees", c.name, c.done)
		}
	}
}

// The noise floor of an episode's transcript is measured over the first
// minute of the first part saved, measured again while that part is
// shorter and kept once it is not. So it is the same whether the first two
// minutes were saved in one piece, in two, or after a stop a few seconds
// in, and a part heard after it moves it no more. Measured over all that
// was heard, the quiet second minute pulled it down, issue #437. Kept from
// the first save alone, a stop in the quiet before anyone spoke set it to
// the quiet's level for good.
func TestTheNoiseFloorIsTheFirstMinutesHoweverTheSavesFall(t *testing.T) {
	// Twenty seconds of room before anyone speaks, then a loud voice to
	// the end of the first minute and a quiet one after it, a word every
	// half second.
	frames := make([]float32, int(120/timed.FrameSeconds))
	var words []timed.Cue
	for i := range frames {
		at := float64(i) * timed.FrameSeconds
		frames[i] = -60
		if at >= 20 && i%50 < 35 {
			frames[i] = -20
			if at >= 60 {
				frames[i] = -40
			}
		}
		if at >= 20 && i%50 == 0 {
			words = append(words, timed.Cue{Start: at, End: at + 0.35, Text: fmt.Sprintf("wort%d", i/50)})
		}
	}
	within := func(from, to float64) ([]timed.Cue, []float32) {
		var in []timed.Cue
		for _, w := range words {
			if w.Start >= from && w.Start < to {
				in = append(in, w)
			}
		}
		return in, frames[int(math.Round(from/timed.FrameSeconds)):int(math.Round(to/timed.FrameSeconds))]
	}
	floorAfter := func(parts ...timed.Window) float64 {
		t.Helper()
		files := afero.NewMemMapFs()
		if err := afero.WriteFile(files, "/ep.mp4", []byte("the video"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp, err := stampOf(files, "/ep.mp4")
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			heard, measured := within(part.Start, part.End)
			if err := addHeard(files, "/words.json", stamp, "parakeet", 120, part, heard, measured, part.Start); err != nil {
				t.Fatal(err)
			}
		}
		file, _, _, ok := readTranscriptFile(files, "/words.json", stamp, "parakeet")
		if !ok {
			t.Fatal("the transcript cannot be read back")
		}
		return float64(file.Mean)
	}
	of := func(from, to float64) float64 {
		return meanOfFrames(frames[int(from/timed.FrameSeconds):int(to/timed.FrameSeconds)])
	}

	first, all, stopped := of(0, 60), of(0, 120), of(0, 16)
	if math.Abs(first-all) < 1 || math.Abs(first-stopped) < 1 {
		t.Fatalf("the first minute is %.1f dB, both %.1f dB and the quiet start %.1f dB, too close for this "+
			"test to prove anything", first, all, stopped)
	}
	for _, c := range []struct {
		name  string
		parts []timed.Window
		want  float64
	}{
		{"in one piece", []timed.Window{{Start: 0, End: 120}}, first},
		{"in two", []timed.Window{{Start: 0, End: 70}, {Start: 70, End: 120}}, first},
		{"stopped in the quiet", []timed.Window{{Start: 0, End: 16}, {Start: 16, End: 120}}, first},
		{"saved every few seconds", []timed.Window{{Start: 0, End: 2}, {Start: 2, End: 16}, {Start: 16, End: 40},
			{Start: 40, End: 120}}, first},
		// The first part saved is where the floor is measured, and what is
		// heard before it later moves it no more.
		{"from the middle first", []timed.Window{{Start: 30, End: 100}, {Start: 0, End: 30}}, of(30, 90)},
	} {
		if got := floorAfter(c.parts...); math.Abs(got-c.want) > 0.001 {
			t.Errorf("saved %s, %v, the floor is made from %.3f dB, want %.3f dB", c.name, c.parts, got, c.want)
		}
	}
}
