package engine

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// formerFile is a transcript file as it was read and written before its
// words were typed: every word three values of any. A file written now
// must be byte for byte what it wrote, so a build before this one reads
// it, and a file read now must read as it read, so nothing is heard again.
type formerFile struct {
	Version int            `json:"version"`
	Source  sourceStamp    `json:"source"`
	Model   string         `json:"model"`
	From    num.Float      `json:"from"`
	To      num.Float      `json:"to"`
	Mean    num.Float      `json:"mean"`
	Partial bool           `json:"partial,omitempty"`
	Resume  num.Float      `json:"resume,omitempty"`
	Parts   [][2]num.Float `json:"parts,omitempty"`
	End     num.Float      `json:"end,omitempty"`
	Words   [][3]any       `json:"words"`
}

// formerBytes is the file as it was written.
func formerBytes(t testing.TB, file transcriptFile, words []timed.Cue) []byte {
	t.Helper()
	former := formerFile{Version: file.Version, Source: file.Source, Model: file.Model, From: file.From, To: file.To,
		Mean: file.Mean, Partial: file.Partial, Resume: file.Resume, Parts: file.Parts, End: file.End,
		Words: make([][3]any, len(words))}
	for i, w := range words {
		former.Words[i] = [3]any{num.Float(num.RoundTo(w.Start, 3)), num.Float(num.RoundTo(w.End, 3)), w.Text}
	}
	body, err := MarshalPlan(former)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// formerRead is the file as it was read: its head, and its words, or
// false when it was not read.
func formerRead(data []byte) (formerFile, []timed.Cue, bool) {
	var file formerFile
	if decodeJSON(data, &file) != nil {
		return file, nil, false
	}
	words := make([]timed.Cue, 0, len(file.Words))
	for _, item := range file.Words {
		low, ok1 := num.AsFloat(item[0])
		high, ok2 := num.AsFloat(item[1])
		text, ok3 := item[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return file, nil, false
		}
		words = append(words, timed.Cue{Start: low, End: high, Text: text})
	}
	return file, words, true
}

// readsAsBefore checks data reads now as it read before.
func readsAsBefore(t *testing.T, data []byte) {
	t.Helper()
	want, wantWords, wantOK := formerRead(data)
	var got transcriptFile
	gotOK := decodeTranscript(data, &got) == nil
	if gotOK != wantOK {
		t.Fatalf("%q read %v, before %v", data, gotOK, wantOK)
	}
	if !gotOK {
		return
	}
	gotWords := make([]timed.Cue, len(got.Words))
	for i, w := range got.Words {
		gotWords[i] = timed.Cue(w)
	}
	got.Words, want.Words = nil, nil
	head := formerFile{Version: got.Version, Source: got.Source, Model: got.Model, From: got.From, To: got.To,
		Mean: got.Mean, Partial: got.Partial, Resume: got.Resume, Parts: got.Parts, End: got.End}
	if fmt.Sprintf("%#v", head) != fmt.Sprintf("%#v", want) {
		t.Errorf("%q read as\n%#v\nbefore\n%#v", data, head, want)
	}
	if fmt.Sprintf("%#v", gotWords) != fmt.Sprintf("%#v", wantWords) {
		t.Errorf("%q read words %#v, before %#v", data, gotWords, wantWords)
	}
}

// Words with every kind of text and time are written byte for byte as
// before, and read back as they were.
func TestATranscriptIsWrittenAndReadAsBefore(t *testing.T) {
	t.Parallel()
	file := transcriptFile{Version: transcriptVersion, Source: sourceStamp{Name: "ep.mp4", Size: 12, Modified: "x"},
		Model: "m", Mean: -25.5, Parts: [][2]num.Float{{0, 12.25}}, End: 7200}
	for _, words := range [][]timed.Cue{
		nil,
		{{Start: 0, End: 0.3, Text: "Hallo"}},
		{{Start: 1, End: 2.5, Text: "Grüße"}, {Start: 3.0004, End: 3.2496, Text: `"so"`},
			{Start: 4, End: 5, Text: "a b<&>"}, {Start: 1234.5675, End: 7199.999, Text: "wort,"},
			{Start: 5, End: 6, Text: "zwei Zeilen "}, {Start: 6, End: 7, Text: "tab\there\\"},
			{Start: 7, End: 8, Text: "\xff\xfe"}, {Start: 8, End: 8, Text: ""}, {Start: 1e21, End: -0.0004, Text: "x"},
			{Start: math.Inf(1), End: math.NaN(), Text: "→"}},
	} {
		file.Words = storedWords(words)
		got, err := encodeTranscript(file)
		if err != nil {
			t.Fatal(err)
		}
		if want := formerBytes(t, file, words); !bytes.Equal(got, want) {
			t.Errorf("written as\n%s\nbefore\n%s", got, want)
		}
		readsAsBefore(t, got)
	}
}

// Files not written by this build are read as before: odd words, odd
// keys and broken files.
func TestAnOddTranscriptIsReadAsBefore(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		`{"version": 3, "words": [[1.0, 2.0, "a"]]}`,
		`{"version": 3, "words": [["1.5", true, "a"]]}`,
		`{"version": 3, "words": [[1, 2, "a", "extra"]]}`,
		`{"version": 3, "words": [[1, 2]]}`,
		`{"version": 3, "words": [[1, null, "a"]]}`,
		`{"version": 3, "words": [[1e400, 2, "a"]]}`,
		`{"version": 3, "words": [[1, 2, 3]]}`,
		`{"version": 3, "words": [{"start": 1}]}`,
		`{"Version": 3, "MODEL": "m", "words": []}`,
		`{"version": 2, "version": 3, "words": []}`,
		`{"version": 3, "words": []} trailing`,
		`{"version": 3, "model": "\xff", "words": [[1, 2, "\xff"]]}`,
		`{"version": 3, "unknown": {"a": [1]}, "words": [[0.0001, 2, "a"]]}`,
		`{"version": 3, "words": null}`,
		`{"version": "3", "words": []}`,
		`{"version": 3, "words": [[1, 2, "a"]`,
		``,
	} {
		readsAsBefore(t, []byte(data))
	}
}

// Whatever bytes a transcript file holds, they are read as before.
func FuzzATranscriptIsReadAsBefore(f *testing.F) {
	f.Add([]byte(`{"version": 3, "parts": [[0, 2]], "words": [[1.0, 2.0, "a"], ["1", 2, "b"]]}`))
	f.Add([]byte(`{"version": 3, "words": [[1, 2, "a "]]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		readsAsBefore(t, data)
	})
}

// Whatever a word is, it is written as before, and read back the same.
func FuzzATranscriptIsWrittenAsBefore(f *testing.F) {
	f.Add(1.5, 2.25, "Grüße")
	f.Add(0.0, -0.0004, "\"\\ ")
	f.Fuzz(func(t *testing.T, start, end float64, text string) {
		words := []timed.Cue{{Start: start, End: end, Text: text}, {Start: end, End: start, Text: "wort"}}
		file := transcriptFile{Version: transcriptVersion, Words: storedWords(words)}
		got, err := encodeTranscript(file)
		if err != nil {
			t.Fatal(err)
		}
		if want := formerBytes(t, file, words); !bytes.Equal(got, want) {
			t.Fatalf("written as\n%s\nbefore\n%s", got, want)
		}
		readsAsBefore(t, got)
	})
}
