package engine

import (
	"bytes"
	"encoding/json/jsontext"
	"strings"

	"github.com/spf13/afero"
)

// readTranscriptHead is what a transcript file says of itself before its
// words: whether it is done and the parts it heard, with no word read.
// The library asks it of every video as the app starts, which read every
// word of every video before, issue #953. The words are the last field,
// see encodeTranscript, so the file is read only up to them, a few
// hundred bytes, and of the loudness beside it only how long it is. What
// it says goes by the same rules as a whole read, see transcriptFile.fit.
// A file whose head cannot be read this way, one not written here, is
// not read: the caller reads it whole.
func readTranscriptHead(fsys afero.Fs, path string, stamp sourceStamp, model string) (transcriptFile, bool) {
	var file transcriptFile
	frames, err := fsys.Stat(framesPath(path))
	if err != nil || frames.Size()%4 != 0 {
		return file, false
	}
	f, err := fsys.Open(path)
	if err != nil {
		return file, false
	}
	defer f.Close()
	head, ok := headOf(jsontext.NewDecoder(f))
	if !ok || decodeTranscript(head, &file) != nil {
		return file, false
	}
	if _, ok := file.fit(stamp, model, int(frames.Size()/4)); !ok {
		return file, false
	}
	return file, true
}

// headOf is the members of the object dec reads that come before its
// words, as an object of their own with no words. Any name may be read
// as words, the way decodeTranscript reads names, without minding case.
func headOf(dec *jsontext.Decoder) ([]byte, bool) {
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '{' {
		return nil, false
	}
	var head bytes.Buffer
	head.WriteByte('{')
	for {
		tok, err := dec.ReadToken()
		if err != nil || tok.Kind() != '"' {
			// An object that ends with no words was not written here.
			return nil, false
		}
		name := tok.String()
		if strings.EqualFold(name, "words") {
			break
		}
		value, err := dec.ReadValue()
		if err != nil {
			return nil, false
		}
		quoted, err := jsontext.AppendQuote(nil, name)
		if err != nil {
			return nil, false
		}
		head.Write(quoted)
		head.WriteByte(':')
		head.Write(value)
		head.WriteByte(',')
	}
	head.WriteString(`"words":[]}`)
	return head.Bytes(), true
}
