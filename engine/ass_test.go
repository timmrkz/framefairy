package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Caption text is whatever the guest said, as heard by the recogniser and
// possibly corrected by hand. In an ASS file a brace opens an override block
// and a backslash starts a tag, so text that is passed through unchanged can
// restyle or move itself, or push lines into the header.

func TestEscapeASSTextNeutralisesOverrides(t *testing.T) {
	cases := []struct{ in, want string }{
		{`normal text`, `normal text`},
		{`{\an8}oben`, `(/an8)oben`},
		{"zwei\nzeilen", "zwei zeilen"},
		{"mit\r\numbruch", "mit  umbruch"},
		{`a\Nb`, `a/Nb`},
	}
	for _, c := range cases {
		if got := EscapeASSText(c.in); got != c.want {
			t.Errorf("EscapeASSText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveStyleValidatesEveryValue(t *testing.T) {
	s := ResolveStyle(nil)
	if s.Font != "Inter Black" || s.Size != 96 || !s.Highlight {
		t.Fatalf("defaults %+v", s)
	}
	// A colour that is not a colour, a size out of any sane range, a font
	// with a comma in it, and a key nobody knows.
	s = ResolveStyle(map[string]any{
		"primary":          "rot",
		"highlight_colour": "not a colour",
		"size":             1e9,
		"font":             "Fett, Bold\nStyle: Sneak,,,,",
		"wrap_chars":       2,
		"pad":              99,
		"border_style":     7,
		"margin_v":         "nope",
		"unknown":          "ignored",
		"highlight":        0,
	})
	if s.Primary != DefaultStyle["primary"] {
		t.Errorf("primary %q", s.Primary)
	}
	if s.HighlightColour != "&H922194&" {
		t.Errorf("highlight colour %q", s.HighlightColour)
	}
	if s.Size != 400 || s.WrapChars != 8 || s.Pad != 8 || s.BorderStyle != 4 {
		t.Errorf("bounds %+v", s)
	}
	if strings.ContainsAny(s.Font, ",\n\r{}:") {
		t.Errorf("font %q could shift the style line", s.Font)
	}
	if s.MarginV != DefaultStyle["margin_v"] {
		t.Errorf("margin_v %v", s.MarginV)
	}
	if s.Highlight {
		t.Errorf("highlight should be off")
	}
	// The colours a plan may set are accepted in the forms people write.
	for _, value := range []any{"#B4236F", "B4236F", "&H6F23B4&"} {
		if got := ResolveStyle(map[string]any{"highlight_colour": value}).HighlightColour; got != "&H6F23B4&" {
			t.Errorf("highlight_colour %v gave %q", value, got)
		}
	}
}

// The window paints the captions itself while a clip plays, so the colours
// of the render have to come out as colours a browser understands.
func TestWebColourTurnsAnASSColourAround(t *testing.T) {
	cases := []struct{ in, want string }{
		{"&H00FFFFFF", "rgba(255, 255, 255, 1)"},
		{"&H80000000", "rgba(0, 0, 0, 0.498)"},
		{"&H6F23B4&", "rgba(180, 35, 111, 1)"},
		{"&HFF000000", "rgba(0, 0, 0, 0)"},
		{"", "rgba(255, 255, 255, 1)"},
		{"rot", "rgba(255, 255, 255, 1)"},
		{"&H12345", "rgba(255, 255, 255, 1)"},
	}
	for _, c := range cases {
		if got := WebColour(c.in); got != c.want {
			t.Errorf("WebColour(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCaptionLinesWrapSoTheyFitTheFrame(t *testing.T) {
	words := []Cue{{0, 1, "Als"}, {1, 2, "Kind"}, {2, 3, "stand"}, {3, 4, "ich"}, {4, 5, "da"}}
	caption := Caption{Start: 0, End: 5, Text: "Als Kind stand ich da", Words: words}

	// A face the program carries is measured, so where a line breaks depends
	// on how wide the words really are at that size.
	s := ResolveStyle(map[string]any{"size": 96.0})
	lines := CaptionLines(caption, s)
	held := 0
	for _, line := range lines {
		held += len(line)
		width, ok := TextWidth(s.Font, cueText(line), s.Size)
		if !ok || width > CaptionRoom(s) {
			t.Errorf("%q is %v wide, the room is %v", cueText(line), width, CaptionRoom(s))
		}
	}
	if held != len(words) {
		t.Errorf("%d of %d words came back", held, len(words))
	}

	// The same caption at a bigger size needs more lines.
	big := ResolveStyle(map[string]any{"size": 200.0})
	if len(CaptionLines(caption, big)) <= len(lines) {
		t.Errorf("at size 200 it still takes %d lines", len(CaptionLines(caption, big)))
	}

	// A face the program does not carry cannot be measured, and then the
	// character count decides, as it always did.
	plain := ResolveStyle(map[string]any{"font": "Helvetica", "wrap_chars": 12})
	lines = CaptionLines(caption, plain)
	if len(lines) != 2 || len(lines[0]) != 2 || len(lines[1]) != 3 {
		t.Fatalf("lines %v", lines)
	}
	if lines[0][0] != words[0] || lines[1][2] != words[4] {
		t.Errorf("the words moved: %v", lines)
	}
	// A caption without word timings still comes back wrapped.
	lines = CaptionLines(Caption{Start: 0, End: 5, Text: "Als Kind stand ich da"}, plain)
	if len(lines) != 2 || lines[0][0].Text != "Als Kind" || lines[1][0].Text != "stand ich da" {
		t.Errorf("lines without timings %v", lines)
	}
}

// Nothing may leave the frame, so a word too wide to break brings the size
// of the whole clip down until it fits.
func TestAWordTooWideBringsTheSizeDown(t *testing.T) {
	long := "Donaudampfschifffahrtsgesellschaftskapitaensmuetze"
	caption := Caption{Start: 0, End: 2, Text: long, Words: []Cue{{0, 2, long}}}
	s := ResolveStyle(map[string]any{"size": 96.0})
	laid, fitted := LayOutCaptions([]Caption{caption}, s)
	if fitted.Size >= s.Size {
		t.Errorf("the size stayed at %v", fitted.Size)
	}
	if len(laid) != 1 || len(laid[0].Lines) != 1 {
		t.Fatalf("laid out as %v", laid)
	}
	width, ok := TextWidth(fitted.Font, long, fitted.Size)
	if !ok || width > CaptionRoom(fitted) {
		t.Errorf("%v wide at size %v, the room is %v", width, fitted.Size, CaptionRoom(fitted))
	}
	// A caption that fits is left at the size it was given.
	_, kept := LayOutCaptions([]Caption{{Start: 0, End: 2, Text: "kurz",
		Words: []Cue{{0, 2, "kurz"}}}}, s)
	if kept.Size != s.Size {
		t.Errorf("a short caption came back at %v", kept.Size)
	}
}

// The measurement has to agree with what libass draws, or a line that fits
// here runs over the box there. Ink is narrower than the pen travels, by
// the bearings at either end, so the two are compared with room to spare.
func TestTheMeasureAgreesWithLibass(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		t.Skip("no usable ffmpeg here")
	}
	for _, font := range CaptionFonts() {
		s := ResolveStyle(map[string]any{"font": font.Name, "size": 96.0, "highlight": 0.0})
		text := "Hamburgefonstiv und Wagenrad"
		boxes, ok := e.measureMany(context.Background(),
			[]string{`{\alpha&H00&}` + text}, s, 1920, int(s.Size))
		if !ok || len(boxes) != 1 || boxes[0] == nil {
			t.Skip("this ffmpeg cannot measure captions")
		}
		ink := float64(boxes[0].right - boxes[0].left)
		pen, _ := TextWidth(font.Name, text, s.Size)
		if ink > pen || ink < pen*0.92 {
			t.Errorf("%s: libass draws %v wide, the measure says %v", font.Name, ink, pen)
		}
	}
}

// writeCaptionFile writes an ASS file without measuring, which needs no
// ffmpeg: no rounded box and no highlight.
func writeCaptionFile(t *testing.T, captions []Caption) string {
	t.Helper()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	path := filepath.Join(t.TempDir(), "clip.ass")
	err := e.WriteASS(context.Background(), captions, path, 1080, 1920,
		map[string]any{"radius": 0.0, "highlight": 0.0})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestWriteASSKeepsCaptionTextInert(t *testing.T) {
	body := writeCaptionFile(t, []Caption{
		{Start: 0, End: 2, Text: `{\pos(0,0)\fs200}versteckt`},
		{Start: 2, End: 4, Text: "zeile eins\nzeile zwei"},
		{Start: 4, End: 6, Text: `ein \N tag`},
	})
	var dialogue []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "Dialogue:") {
			dialogue = append(dialogue, line)
		}
	}
	if len(dialogue) != 3 {
		t.Fatalf("%d dialogue lines in\n%s", len(dialogue), body)
	}
	for _, line := range dialogue {
		// Everything after the ninth comma is the caption text itself.
		parts := strings.SplitN(line, ",", 10)
		if len(parts) != 10 {
			t.Fatalf("dialogue line has too few fields: %s", line)
		}
		if left := withoutOwnTags(parts[9]); strings.ContainsAny(left, `{}\`) {
			t.Errorf("override syntax reached the text: %s", parts[9])
		}
	}
}

// withoutOwnTags removes the two tags the writer puts in itself, the hard
// spaces that pad a caption and the line break between its lines. What is
// left came from the caption text.
func withoutOwnTags(text string) string {
	return strings.NewReplacer(`\h`, "", `\N`, "").Replace(text)
}

// FuzzWriteASS throws caption text at the writer and insists the file stays
// a caption file: one Dialogue line per caption, with no override syntax in
// the text and nothing that could open a new section.
func FuzzWriteASS(f *testing.F) {
	f.Add(`{\an8}oben`, 0.0, 2.0)
	f.Add("zwei\nzeilen", 1.0, 1.5)
	f.Add("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Caption,,0,0,0,,x", 0.0, 1.0)
	f.Add(strings.Repeat("ü", 300), 0.0, 90000.0)
	f.Fuzz(func(t *testing.T, text string, start, end float64) {
		if !isFinite(start) || !isFinite(end) || start < 0 || end < start || end > 100_000 {
			t.Skip()
		}
		body := writeCaptionFile(t, []Caption{{Start: start, End: end, Text: text}})
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		events := 0
		for i, line := range lines {
			if strings.HasPrefix(line, "Dialogue:") {
				events++
				parts := strings.SplitN(line, ",", 10)
				if len(parts) != 10 {
					t.Fatalf("dialogue line %d has too few fields: %q", i, line)
				}
				if left := withoutOwnTags(parts[9]); strings.ContainsAny(left, `{}\`) {
					t.Fatalf("override syntax in the text: %q", parts[9])
				}
				continue
			}
			// Nothing the caption carried may start a line of its own, which
			// is how a caption would add a style or a second event section.
			if strings.HasPrefix(line, "Style:") && i > 12 {
				t.Fatalf("an extra style line appeared at %d: %q", i, line)
			}
		}
		if events != 1 {
			t.Fatalf("%d dialogue lines for one caption", events)
		}
	})
}

// FuzzResolveStyle checks the other half: values from a plan's caption_style
// object end up in the file's header, so every one of them has to come back
// inside its bounds and free of anything that could shift a field.
func FuzzResolveStyle(f *testing.F) {
	f.Add("Inter Black", "#B4236F", "&H00FFFFFF", 96.0, 20)
	f.Add("Fett,Bold", "rot", "x", 1e9, -4)
	f.Add("a\nStyle: b", "&HFFFFFFFF&", "&H", 0.0, 9999)
	f.Fuzz(func(t *testing.T, font, highlight, primary string, size float64, wrap int) {
		s := ResolveStyle(map[string]any{
			"font": font, "highlight_colour": highlight, "primary": primary,
			"size": size, "wrap_chars": wrap,
		})
		if strings.ContainsAny(s.Font, ",{}\n\r:") || runeLen(s.Font) > 64 || s.Font == "" {
			t.Fatalf("font %q", s.Font)
		}
		if s.Size < 8 || s.Size > 400 || !isFinite(s.Size) {
			t.Fatalf("size %v", s.Size)
		}
		if s.WrapChars < 8 || s.WrapChars > 200 {
			t.Fatalf("wrap %d", s.WrapChars)
		}
		for _, colour := range []string{s.Primary, s.OutlineColour, s.BackColour, s.HighlightColour} {
			if strings.TrimLeft(colour, "&H0123456789abcdefABCDEFh") != "" || colour == "" {
				t.Fatalf("colour %q", colour)
			}
		}
	})
}
