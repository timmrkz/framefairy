package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// readCounted is a disk that counts the bytes read of each file.
type readCounted struct {
	afero.Fs
	mu   sync.Mutex
	read map[string]int
}

func newReadCounted(fsys afero.Fs) *readCounted {
	return &readCounted{Fs: fsys, read: map[string]int{}}
}

func (d *readCounted) Open(name string) (afero.File, error) {
	f, err := d.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return readCountedFile{File: f, d: d}, nil
}

func (d *readCounted) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := d.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return readCountedFile{File: f, d: d}, nil
}

func (d *readCounted) bytesOf(name string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.read[filepath.Base(name)]
}

type readCountedFile struct {
	afero.File
	d *readCounted
}

func (f readCountedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.d.mu.Lock()
	f.d.read[filepath.Base(f.Name())] += n
	f.d.mu.Unlock()
	return n, err
}

// A start of the app reads only the head of each video's transcript, a
// few hundred bytes, and nothing of its loudness, and the library says
// what a whole read says: done, the parts heard, how far from the start.
// Every start read and snapped every word of every video in the library
// for those few numbers, issue #953.
func TestAStartReadsOnlyTheHeadOfEachTranscript(t *testing.T) {
	t.Parallel()
	fsys, source := longVideo(t, 0.5)
	reads := newReadCounted(fsys)
	started := NewMachine(reads).Episode(source).Status("m", nil)
	words := filepath.Join(LogsDir(source), TranscriptName(nil))
	size, err := fsys.Stat(words)
	if err != nil {
		t.Fatal(err)
	}
	if read := reads.bytesOf(words); read > 4096 || read*100 > int(size.Size()) {
		t.Errorf("the start read %d bytes of a transcript of %d", read, size.Size())
	}
	if read := reads.bytesOf(framesPath(words)); read > 0 {
		t.Errorf("the start read %d bytes of the loudness", read)
	}

	opened := NewMachine(fsys).Episode(source)
	if _, err := opened.Transcript("m", nil); err != nil {
		t.Fatal(err)
	}
	whole := opened.Status("m", nil)
	if started.Transcribed != whole.Transcribed || !reflect.DeepEqual(started.Heard, whole.Heard) ||
		started.Covered != whole.Covered || started.Speechless {
		t.Errorf("the start said %+v, a whole read %+v", started, whole)
	}
}

// Where the lines start and end, and whether nobody speaks, need every
// word, so they are said once the video is opened and its transcript
// held, and not before: a video with words is never said to be
// speechless by a library that did not read them. A status read before
// the video was opened is not what it says after.
func TestAnOpenedVideoSaysWhereItsLinesAre(t *testing.T) {
	t.Parallel()
	fsys, source := longVideo(t, 0.05)
	ep := NewMachine(fsys).Episode(source)
	if st := ep.Status("m", nil); st.Said != nil || st.Speechless || !st.Transcribed {
		t.Errorf("the library said %+v, want it transcribed, with no lines and not speechless", st)
	}
	r, err := ep.Transcript("m", nil)
	if err != nil {
		t.Fatal(err)
	}
	st := ep.Status("m", nil)
	if st.Said == nil || *st.Said != *r.Said() || st.Speechless {
		t.Errorf("the opened video said lines %v and speechless %v, want %v", st.Said, st.Speechless, *r.Said())
	}
}

// The head says what the whole file says, by the same rules: a file
// heard in parts, one the loudness does not reach to its end, one of an
// older version, and one of another video or model, which neither reads.
func TestTheHeadOfATranscriptSaysWhatTheWholeFileSays(t *testing.T) {
	t.Parallel()
	fsys := afero.NewMemMapFs()
	source := "/v/ep.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	words := storedWords([]timed.Cue{{Start: 1, End: 1.4, Text: "eins"}, {Start: 31, End: 31.5, Text: "zwei."}})
	path := filepath.Join(LogsDir(source), TranscriptName(nil))
	for _, c := range []struct {
		name   string
		file   transcriptFile
		frames int
		read   bool
	}{
		{"heard whole", transcriptFile{Version: 3, Source: stamp, Model: "m", Parts: [][2]num.Float{{0, 60}}, End: 60}, 6000, true},
		{"heard in parts", transcriptFile{Version: 3, Source: stamp, Model: "m", Parts: [][2]num.Float{{0, 10}, {30, 40}}, End: 60}, 6000, true},
		{"loudness short of the parts", transcriptFile{Version: 3, Source: stamp, Model: "m", Parts: [][2]num.Float{{0, 10}, {30, 40}}, End: 60}, 3500, true},
		{"version 2", transcriptFile{Version: 2, Source: stamp, Model: "m", To: 50, Resume: 40}, 6000, true},
		{"another model", transcriptFile{Version: 3, Source: stamp, Model: "n", Parts: [][2]num.Float{{0, 60}}, End: 60}, 6000, false},
		{"another video", transcriptFile{Version: 3, Source: sourceStamp{Name: "other.mp4"}, Model: "m", Parts: [][2]num.Float{{0, 60}}, End: 60}, 6000, false},
	} {
		c.file.Words = words
		if err := writeTranscript(fsys, path, c.file, make([]float32, c.frames)); err != nil {
			t.Fatal(err)
		}
		head, headOK := readTranscriptHead(fsys, path, stamp, "m")
		whole, _, _, wholeOK := readTranscriptFile(fsys, path, stamp, "m")
		if headOK != c.read || wholeOK != c.read {
			t.Errorf("%s: the head read %v and the whole file %v, want %v", c.name, headOK, wholeOK, c.read)
			continue
		}
		if c.read && (head.done() != whole.done() || !reflect.DeepEqual(head.heard(), whole.heard())) {
			t.Errorf("%s: the head says done %v and %v, the whole file %v and %v", c.name,
				head.done(), head.heard(), whole.done(), whole.heard())
		}
	}
}

// Whatever bytes a transcript file holds, its head is read without a
// panic, and a file as the program writes it says through its head what
// it says read whole.
func FuzzTheHeadOfATranscript(f *testing.F) {
	stamp := sourceStamp{Name: "ep.mp4", Size: 5, Modified: "2026-11-21T12:00:00Z"}
	words := storedWords([]timed.Cue{{Start: 1, End: 1.4, Text: "eins"}, {Start: 31, End: 31.5, Text: "zwei."}})
	for _, file := range []transcriptFile{
		{Version: 3, Source: stamp, Model: "m", Parts: [][2]num.Float{{0, 10}, {30, 40}}, End: 60, Words: words},
		{Version: 2, Source: stamp, Model: "m", To: 50, Resume: 40, Partial: true, Words: words},
	} {
		body, err := encodeTranscript(file)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(body)
	}
	f.Add([]byte(`{"version":3,"words":[],"parts":[[0,5]]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		fsys := afero.NewMemMapFs()
		path := "/logs/words.json"
		if err := afero.WriteFile(fsys, path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := afero.WriteFile(fsys, framesPath(path), writtenFrames(make([]float32, 4000)), 0o644); err != nil {
			t.Fatal(err)
		}
		head, headOK := readTranscriptHead(fsys, path, stamp, "m")
		whole, _, _, wholeOK := readTranscriptFile(fsys, path, stamp, "m")
		if !wholeOK {
			return
		}
		written, err := encodeTranscript(whole)
		if err != nil || !bytes.Equal(written, data) {
			return
		}
		if !headOK || head.done() != whole.done() || !reflect.DeepEqual(head.heard(), whole.heard()) {
			t.Fatalf("the head read %v, done %v, %v, the whole file done %v, %v", headOK,
				head.done(), head.heard(), whole.done(), whole.heard())
		}
	})
}
