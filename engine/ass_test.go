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

// The app paints the captions itself while a clip plays, so the colours
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

// Nothing may leave the frame, and one long word may not make every caption
// of the clip smaller either. It is hyphenated at the size that was chosen.
func TestAWordTooWideIsHyphenated(t *testing.T) {
	long := "Donaudampfschifffahrtsgesellschaftskapitaensmuetze"
	s := ResolveStyle(map[string]any{"size": 96.0})
	short := Caption{Start: 2, End: 3, Text: "und dann", Words: []Cue{{2, 2.4, "und"}, {2.5, 3, "dann"}}}
	laid := LayOutCaptions([]Caption{
		{Start: 0, End: 2, Text: "die " + long, Words: []Cue{{0, 0.2, "die"}, {0.2, 2, long}}},
		short,
	}, s)
	if len(laid) != 2 {
		t.Fatalf("laid out as %v", laid)
	}
	var joined string
	var flat []Cue
	for _, line := range laid[0].Lines {
		width, ok := TextWidth(s.Font, cueText(line), s.Size)
		if !ok || width > CaptionRoom(s) {
			t.Errorf("%q is %v wide, the room is %v", cueText(line), width, CaptionRoom(s))
		}
		flat = append(flat, line...)
	}
	// The render highlights the caption's words against its lines, so the
	// two have to be the same pieces in the same order.
	if len(flat) != len(laid[0].Words) {
		t.Fatalf("%d pieces drawn, %d words", len(flat), len(laid[0].Words))
	}
	for i, w := range laid[0].Words {
		if flat[i] != w {
			t.Errorf("piece %d is %v in the lines and %v in the words", i, flat[i], w)
		}
		if i > 0 {
			joined += strings.TrimSuffix(w.Text, "-")
		}
	}
	if joined != long || len(laid[0].Words) < 3 {
		t.Errorf("the word came back as %v", laid[0].Words)
	}
	// The pieces share the time the word was spoken in, one after another.
	pieces := laid[0].Words[1:]
	if pieces[0].Start != 0.2 || pieces[len(pieces)-1].End != 2 {
		t.Errorf("the pieces run %v to %v", pieces[0].Start, pieces[len(pieces)-1].End)
	}
	for i := 1; i < len(pieces); i++ {
		if pieces[i].Start != pieces[i-1].End || pieces[i].End <= pieces[i].Start {
			t.Errorf("piece %d runs %v to %v after %v", i, pieces[i].Start, pieces[i].End, pieces[i-1].End)
		}
	}
	// The other caption is as it was.
	if len(laid[1].Lines) != 1 || cueText(laid[1].Lines[0]) != "und dann" || len(laid[1].Words) != 2 {
		t.Errorf("the short caption became %v", laid[1].Lines)
	}
}

// A word is broken where TeX would break it in that language, at the last
// break that still fits.
func TestAWordBreaksWhereTheLanguageAllows(t *testing.T) {
	// A face that cannot be measured, so the room is sixteen characters.
	room := captionRoom{font: "keine", size: 96, room: 888, wrapChars: 16}
	de, en := hyphenatorFor("de"), hyphenatorFor("en")
	if de == nil || en == nil {
		t.Fatal("no patterns for German or English")
	}
	if de.left != 2 || de.right != 2 || en.left != 2 || en.right != 3 {
		t.Errorf("fewest letters %d/%d in German and %d/%d in English", de.left, de.right, en.left, en.right)
	}
	for _, c := range []struct {
		word string
		h    *hyphenator
		want []string
	}{
		{"Suchmaschinenoptimierung", de, []string{"Suchmaschinen-", "optimierung"}},
		{"Persönlichkeitsentwicklung,", de, []string{"Persönlichkeits-", "entwicklung,"}},
		{"internationalization", en, []string{"international-", "ization"}},
		{"SEO-Agenturmitarbeiter", de, []string{"SEO-Agenturmit-", "arbeiter"}},
		// A hyphen the word has is a break, and not doubled.
		{"Pflanzenpflege-Onlineshop", de, []string{"Pflanzenpflege-", "Onlineshop"}},
		{"kurz", de, []string{"kurz"}},
		// With no patterns the word is broken where the line allows.
		{"abcdefghijklmnopqrstuvwxyz", nil, []string{"abcdefghijklmno-", "pqrstuvwxyz"}},
	} {
		got := breakWord(c.word, room, c.h)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s broke as %q, want %q", c.word, got, c.want)
		}
	}
	// Longer than two lines, each as full as the breaks allow.
	long := breakWord("Donaudampfschifffahrtsgesellschaftskapitänsmütze", room, de)
	if len(long) < 3 {
		t.Errorf("broke as %q", long)
	}
	for _, piece := range long {
		if !room.fits(piece) {
			t.Errorf("%q does not fit", piece)
		}
	}
	// Every language whose patterns are here has a hyphenator, found by the
	// file's name alone, and a language without patterns has none.
	for _, code := range []string{"de", "en", "el", "hr", "hu", "ru", "sv"} {
		if hyphenatorFor(code) == nil {
			t.Errorf("no hyphenator for %s", code)
		}
	}
	if hyphenatorFor("cs") != nil || hyphenatorFor("xx") != nil || hyphenatorFor("") != nil {
		t.Error("patterns where none ship")
	}
}

// The language is read off the words of the captions.
func TestTheCaptionsSayWhatLanguageTheyAreIn(t *testing.T) {
	for text, want := range map[string]string{
		"Wir haben mit Pflanzenpflege und Suchmaschinenoptimierung angefangen und dann gemerkt, dass es läuft": "de",
		"We started with plant care and search engine optimisation and then noticed that it worked":            "en",
		"Nous avons commencé avec l'entretien des plantes et ensuite nous avons vu que cela marchait":          "fr",
	} {
		var words []Cue
		for i, w := range fields(text) {
			words = append(words, Cue{float64(i), float64(i) + 1, w})
		}
		if got := languageOf([]Caption{{Words: words}}); got != want {
			t.Errorf("%q read as %s", text, got)
		}
	}
}

// A word too wide for a line gets a caption of its own, so it is read as
// two lines of one caption, and the words around it stay where they fit.
func TestAWordTooWideGetsACaptionOfItsOwn(t *testing.T) {
	clip := Clip{
		Segments: []Segment{{Start: 0, End: 5}},
		Words: []Cue{
			{0.1, 0.3, "mit"}, {0.3, 1.6, "Suchmaschinenoptimierung"}, {1.6, 2.0, "Geld"},
			{2.0, 2.5, "verdient."},
		},
	}
	alone := TooWide(ResolveStyle(map[string]any{"font": "Inter Black", "size": 96.0}))
	var texts []string
	for _, c := range Captions(clip, 38, alone) {
		texts = append(texts, c.Text)
	}
	if strings.Join(texts, "|") != "mit|Suchmaschinenoptimierung|Geld verdient." {
		t.Errorf("captions %q", texts)
	}
	// Without the rule it rides with the others, as before.
	if got := Captions(clip, 38, nil); len(got) != 2 || got[0].Text != "mit Suchmaschinenoptimierung Geld" {
		t.Errorf("captions %v", got)
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
