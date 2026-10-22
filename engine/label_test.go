package engine

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/afero"
)

// A label is written beside the video, read back with its status, and
// taken away again by giving it no words. The video is never touched.
func TestLabelRoundTrip(t *testing.T) {
	fsys := afero.NewMemMapFs()
	source := "/podcast/015 Ben/youtube.mp4"
	if err := afero.WriteFile(fsys, source, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Status(fsys, source, "", nil).Label; got != nil {
		t.Fatalf("a new video has the label %+v", got)
	}
	l, err := WriteLabel(fsys, source, Label{Text: "  Ben,\n Kindergarten ", Colour: "#0A64D6"})
	if err != nil {
		t.Fatal(err)
	}
	want := Label{Text: "Ben, Kindergarten", Colour: "#0a64d6"}
	if l == nil || *l != want {
		t.Fatalf("written %+v, want %+v", l, want)
	}
	if got := Status(fsys, source, "", nil).Label; got == nil || *got != want {
		t.Fatalf("read back %+v, want %+v", got, want)
	}
	if _, err := WriteLabel(fsys, source, want); err != nil {
		t.Fatal(err)
	}
	// Control characters are no words either, so they take the label
	// away rather than being refused for the colour.
	if l, err := WriteLabel(fsys, source, Label{Text: "\x00\t\x1b", Colour: "#0a64d6"}); err != nil || l != nil {
		t.Fatalf("a label of control characters gave %+v, %v", l, err)
	}
	if got := Status(fsys, source, "", nil).Label; got != nil {
		t.Fatalf("the label taken away by control characters is still %+v", got)
	}
	if _, err := WriteLabel(fsys, source, want); err != nil {
		t.Fatal(err)
	}
	if l, err := WriteLabel(fsys, source, Label{Text: " ", Colour: "#0a64d6"}); err != nil || l != nil {
		t.Fatalf("a label with no words gave %+v, %v", l, err)
	}
	if got := Status(fsys, source, "", nil).Label; got != nil {
		t.Fatalf("the label taken away is still %+v", got)
	}
	if body, _ := afero.ReadFile(fsys, source); string(body) != "video" {
		t.Fatalf("the video became %q", body)
	}
}

// What is not a colour is refused, and a long label is cut to its length.
func TestLabelTidy(t *testing.T) {
	fsys := afero.NewMemMapFs()
	for _, colour := range []string{"", "red", "#12345", "#1234567", "#zzzzzz", "0a64d6"} {
		if _, err := WriteLabel(fsys, "/ep.mp4", Label{Text: "Anna", Colour: colour}); err == nil {
			t.Errorf("the colour %q was taken", colour)
		}
	}
	l, err := WriteLabel(fsys, "/ep.mp4", Label{Text: strings.Repeat("é", 60), Colour: "#942192"})
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(l.Text); n != LabelLength {
		t.Errorf("a long label kept %d characters, want %d", n, LabelLength)
	}
}

// A label file is read like any file the app did not just write: what is
// read is a tidy label or none.
func FuzzReadLabel(f *testing.F) {
	f.Add([]byte(`{"text": "Ben", "colour": "#0a64d6"}`))
	f.Add([]byte(`{"text": "` + strings.Repeat("‮", 80) + `", "colour": "#FFFFFF"}`))
	f.Add([]byte(`{"text": 3, "colour": null}`))
	fsys := afero.NewMemMapFs()
	source := "/ep.mp4"
	path := WorkDir(source) + "/" + labelFile
	f.Fuzz(func(t *testing.T, body []byte) {
		if err := afero.WriteFile(fsys, path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		l := ReadLabel(fsys, source)
		if l == nil {
			return
		}
		if tidy, ok := TidyLabel(*l); !ok || tidy != *l {
			t.Fatalf("read %+v, which is not tidy", *l)
		}
		if utf8.RuneCountInString(l.Text) > LabelLength {
			t.Fatalf("read a label of %d characters", utf8.RuneCountInString(l.Text))
		}
	})
}
