package engine

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/afero"
)

// noWords is a disk on which the files of a transcript cannot be opened,
// only found: what a start reads of them, it has to read from elsewhere.
type noWords struct{ afero.Fs }

func (d noWords) Open(name string) (afero.File, error) {
	if isTranscriptFile(name) {
		return nil, os.ErrPermission
	}
	return d.Fs.Open(name)
}

func (d noWords) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if isTranscriptFile(name) {
		return nil, os.ErrPermission
	}
	return d.Fs.OpenFile(name, flag, perm)
}

func isTranscriptFile(name string) bool {
	base := filepath.Base(name)
	return base == TranscriptName(nil) || base == filepath.Base(framesPath(TranscriptName(nil)))
}

// A start of the app knows what a video's transcript has heard without
// reading its words: the start before kept it in the work folder, and it
// says the same as the transcript, whether it is done, the parts heard
// and from the first line to the last. Every start read and snapped every
// word of every video in the library to learn those few numbers, issue
// #953.
func TestAStartKnowsWhatIsHeardWithoutTheWords(t *testing.T) {
	t.Parallel()
	fsys, source := longVideo(t, 0.05)
	first := NewMachine(fsys).Episode(source).Status("m", nil)
	if !first.Transcribed || first.Said == nil || len(first.Heard) != 1 {
		t.Fatalf("the first start read %+v, want the whole video heard and said", first)
	}
	next := NewMachine(noWords{fsys}).Episode(source).Status("m", nil)
	if next.Transcribed != first.Transcribed || !reflect.DeepEqual(next.Heard, first.Heard) ||
		!reflect.DeepEqual(next.Said, first.Said) || next.Speechless != first.Speechless || next.Covered != first.Covered {
		t.Errorf("the next start read %+v, want what the first read, %+v", next, first)
	}
}

// What is kept is read only for the transcript it was read from, as it
// is now, at the floor asked for: a transcript heard further, or another
// floor, which places the words elsewhere, is read again, and kept again.
func TestWhatIsHeardIsKeptOnlyForTheTranscriptAsItIs(t *testing.T) {
	t.Parallel()
	fsys, source := longVideo(t, 0.05)
	NewMachine(fsys).Episode(source).Status("m", nil)
	kept := filepath.Join(LogsDir(source), heardName)
	var file heardFile
	data, err := afero.ReadFile(fsys, kept)
	if err != nil || json.Unmarshal(data, &file) != nil {
		t.Fatalf("nothing was kept: %v", err)
	}
	// A kept file that says something else, at a stamp that is not the
	// transcript's, is not read.
	file.Stamp, file.Done, file.Heard, file.Said = "another", false, [][2]float64{{0, 1}}, nil
	body, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fsys, kept, body, 0o644); err != nil {
		t.Fatal(err)
	}
	st := NewMachine(fsys).Episode(source).Status("m", nil)
	if !st.Transcribed || st.Said == nil {
		t.Errorf("read what another transcript kept: %+v", st)
	}
	// And a floor other than the one kept reads the words again.
	floor := -10.0
	if st := NewMachine(noWords{fsys}).Episode(source).Status("m", &floor); st.Transcribed {
		t.Errorf("read what was kept at another floor: %+v", st)
	}
}

// Whatever bytes a work folder holds, what is kept as heard is read
// without a panic, and what is read says what a transcript can say.
func FuzzParseKeptHeard(f *testing.F) {
	f.Add([]byte(`{"version":1,"stamp":"s","done":true,"heard":[[0,7200]],"said":[0.4,7199.5]}`))
	f.Add([]byte(`{"version":1,"stamp":"s","done":false,"heard":[[0,10],[30,60]]}`))
	f.Add([]byte(`{"version":1,"stamp":"s","heard":[[10,5]],"said":[1e400,0]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		h, ok := parseKeptHeard(data, "s")
		if !ok {
			return
		}
		last := 0.0
		for _, p := range h.heard {
			if p[0] < last || p[1] <= p[0] || p[1] > MaxEpisodeSeconds {
				t.Fatalf("read parts no transcript hears: %v", h.heard)
			}
			last = p[1]
		}
		if s := h.said; s != nil && (s[1] < s[0] || s[0] < 0 || s[1] > MaxEpisodeSeconds || math.IsNaN(s[0])) {
			t.Fatalf("read lines no transcript has: %v", *s)
		}
	})
}
