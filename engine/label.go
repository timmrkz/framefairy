package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/afero"
)

// Label is the few words and the colour a person gives a video, so that
// videos with the same file name, a folder of youtube.mp4s, can be told
// apart in the sidebar. The video itself is never renamed: the label is
// kept in the video's work folder, with everything else the app knows
// about it.
type Label struct {
	Text   string `json:"text"`
	Colour string `json:"colour"`
}

// LabelLength is how many characters a label keeps. A label is read at a
// glance beside a file name, in a row of the sidebar.
const LabelLength = 40

// labelFile is the label in the work folder.
const labelFile = "label.json"

// errLabelColour is a label whose colour is not #rrggbb.
var errLabelColour = errors.New("a label's colour is written #rrggbb")

// TidyLabel is a label the way it is kept: on one line, without the
// spaces around it, at most LabelLength characters, and the colour in
// small letters. A label with no words is no label, and says so with ok.
func TidyLabel(l Label) (Label, bool) {
	text := labelWords(l.Text)
	if r := []rune(text); len(r) > LabelLength {
		text = strings.TrimSpace(string(r[:LabelLength]))
	}
	colour := strings.ToLower(l.Colour)
	if text == "" || len(colour) != 7 || colour[0] != '#' || !isHex(colour[1:]) {
		return Label{}, false
	}
	return Label{Text: text, Colour: colour}, true
}

// labelWords is the words of a label on one line, with what is not
// written, spaces and control characters, only ever between two words.
func labelWords(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
}

// ReadLabel is the label of a video, or nil when it has none. A label
// file is read like any file the app did not just write: whatever is in
// it that is not a label is no label.
func ReadLabel(fsys afero.Fs, source string) *Label {
	path, err := SafeChildIn(fsys, WorkDir(source), labelFile)
	if err != nil {
		return nil
	}
	body, err := afero.ReadFile(fsys, path)
	if err != nil {
		return nil
	}
	var l Label
	if json.Unmarshal(body, &l) != nil {
		return nil
	}
	tidy, ok := TidyLabel(l)
	if !ok {
		return nil
	}
	return &tidy
}

// WriteLabel gives a video its label. A label with no words takes the
// label away, and so does one of nothing but spaces and control
// characters, which has none either. The colour has to be one, or nothing
// is written.
func WriteLabel(fsys afero.Fs, source string, l Label) (*Label, error) {
	work := WorkDir(source)
	path, err := SafeChildIn(fsys, work, labelFile)
	if err != nil {
		return nil, err
	}
	if labelWords(l.Text) == "" {
		if err := fsys.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, nil
	}
	tidy, ok := TidyLabel(l)
	if !ok {
		return nil, errLabelColour
	}
	if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(tidy, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := WriteAtomic(fsys, path, body); err != nil {
		return nil, err
	}
	return &tidy, nil
}
