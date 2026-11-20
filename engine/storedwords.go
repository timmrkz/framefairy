package engine

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"framefairy/engine/num"
	"framefairy/engine/timed"
)

// storedWord is one word as a transcript file keeps it, [start, end, text],
// its times rounded to the millisecond. A long video has tens of thousands
// of them, read on every read of its transcript and written on every save
// while it is heard, so each is read and written straight into its place
// rather than as three values of any, which took twice as long.
type storedWord timed.Cue

// storedWords are words as a transcript file keeps them.
func storedWords(words []timed.Cue) []storedWord {
	out := make([]storedWord, len(words))
	for i, w := range words {
		out[i] = storedWord{Start: num.RoundTo(w.Start, 3), End: num.RoundTo(w.End, 3), Text: w.Text}
	}
	return out
}

// errNotPlain is a word that is not two numbers and a text, read again
// the slow way, see decodeTranscript.
var errNotPlain = errors.New("not a plain word")

// UnmarshalJSONFrom reads a word that is two numbers and a text, and
// nothing else.
func (w *storedWord) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != '[' {
		return errNotPlain
	}
	var times [2]float64
	for i := range times {
		tok, err := dec.ReadToken()
		if err != nil || tok.Kind() != '0' {
			return errNotPlain
		}
		if times[i], err = strconv.ParseFloat(tok.String(), 64); err != nil {
			return errNotPlain
		}
	}
	tok, err := dec.ReadToken()
	if err != nil || tok.Kind() != '"' {
		return errNotPlain
	}
	text := tok.String()
	if tok, err := dec.ReadToken(); err != nil || tok.Kind() != ']' {
		return errNotPlain
	}
	*w = storedWord{Start: times[0], End: times[1], Text: text}
	return nil
}

// MarshalJSON writes the word the way writeTranscript does.
func (w storedWord) MarshalJSON() ([]byte, error) {
	return appendWord(nil, w, ""), nil
}

// decodeTranscript reads a transcript file. A file of plain words is read
// with each word in its place. Anything else is read as every file was
// before: each word three values, the first two read as numbers by
// num.AsFloat, the last a text, so the two read the same.
func decodeTranscript(data []byte, file *transcriptFile) error {
	if jsonv2.Unmarshal(data, file, json.DefaultOptionsV1()) == nil {
		return nil
	}
	var any3 struct {
		transcriptFile
		Words [][3]any `json:"words"`
	}
	if err := decodeJSON(data, &any3); err != nil {
		return err
	}
	*file = any3.transcriptFile
	file.Words = make([]storedWord, len(any3.Words))
	for i, item := range any3.Words {
		low, ok1 := num.AsFloat(item[0])
		high, ok2 := num.AsFloat(item[1])
		text, ok3 := item[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return fmt.Errorf("word %d of a transcript is %v", i+1, item)
		}
		file.Words[i] = storedWord{Start: low, End: high, Text: text}
	}
	return nil
}

// encodeTranscript is the file as MarshalPlan writes it, with the words
// written straight into it.
func encodeTranscript(file transcriptFile) ([]byte, error) {
	words := file.Words
	file.Words = []storedWord{}
	head, err := MarshalPlan(file)
	if err != nil {
		return nil, err
	}
	// The words are the last field, and MarshalPlan ends with them.
	const empty = "[]\n}"
	if !bytes.HasSuffix(head, []byte(empty)) {
		return nil, fmt.Errorf("a transcript file that does not end with its words: %q", head[max(0, len(head)-40):])
	}
	body := make([]byte, 0, len(head)+len(words)*48)
	body = append(body, head[:len(head)-len(empty)]...)
	if len(words) == 0 {
		body = append(body, "[]"...)
	} else {
		body = append(body, '[')
		for i, w := range words {
			if i > 0 {
				body = append(body, ',')
			}
			body = append(body, "\n    "...)
			body = appendWord(body, w, "    ")
		}
		body = append(body, "\n  ]"...)
	}
	return append(body, "\n}"...), nil
}

// appendWord writes w as MarshalPlan writes a [3]any of two num.Float and
// a text, indented by indent, or all on one line when indent is empty.
func appendWord(b []byte, w storedWord, indent string) []byte {
	open, comma, end := "[", ",", "]"
	if indent != "" {
		open, comma, end = "[\n"+indent+"  ", ",\n"+indent+"  ", "\n"+indent+"]"
	}
	b = append(b, open...)
	b = appendTime(b, w.Start)
	b = append(b, comma...)
	b = appendTime(b, w.End)
	b = append(b, comma...)
	b = appendWordText(b, w.Text)
	return append(b, end...)
}

// appendTime writes x as num.Float.MarshalJSON does.
func appendTime(b []byte, x float64) []byte {
	if !num.IsFinite(x) {
		return append(b, "null"...)
	}
	n := len(b)
	b = strconv.AppendFloat(b, x, 'f', -1, 64)
	if bytes.IndexByte(b[n:], '.') < 0 {
		b = append(b, ".0"...)
	}
	return b
}

// appendWordText quotes text as MarshalPlan does. A text of plain characters
// is written as it is, and anything the encoder would escape, a quote, a
// backslash, a control character, a broken character or a line or
// paragraph separator, which start with 0xE2, is quoted by the encoder.
func appendWordText(b []byte, text string) []byte {
	plain := utf8.ValidString(text)
	for i := 0; plain && i < len(text); i++ {
		if c := text[i]; c < 0x20 || c == '"' || c == '\\' || c == 0xE2 {
			plain = false
		}
	}
	if !plain {
		quoted, _ := marshalNoEscape(text)
		return append(b, quoted...)
	}
	b = append(b, '"')
	b = append(b, text...)
	return append(b, '"')
}

// readLoudness reads the loudness frames of a transcript, four bytes each.
func readLoudness(raw []byte) ([]float32, bool) {
	if len(raw)%4 != 0 {
		return nil, false
	}
	frames := make([]float32, len(raw)/4)
	for i := range frames {
		frames[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	return frames, true
}

// writtenFrames is frames as readLoudness reads them.
func writtenFrames(frames []float32) []byte {
	raw := make([]byte, 4*len(frames))
	for i, f := range frames {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(f))
	}
	return raw
}
