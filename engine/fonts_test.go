package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"framefairy/internal/ffmpegtest"
)

// The captions are written in faces that travel with the program, so a
// machine with nothing installed still renders a short that looks right.

func TestEveryFaceWeOfferIsInTheBundle(t *testing.T) {
	fonts := CaptionFonts()
	if len(fonts) < 3 {
		t.Fatalf("only %d faces are offered", len(fonts))
	}
	for _, font := range fonts {
		data, ok := FontBytes(font.Name)
		if !ok || len(data) < 10_000 {
			t.Errorf("%s is %d bytes", font.Name, len(data))
			continue
		}
		// A font file starts with one of three signatures.
		switch magic := string(data[:4]); magic {
		case "\x00\x01\x00\x00", "true", "OTTO":
		default:
			t.Errorf("%s does not look like a font: %q", font.Name, magic)
		}
		if _, err := fontBundle.ReadFile(FontsFolder + "/" + font.Licence); err != nil {
			t.Errorf("%s ships without its licence: %v", font.Name, err)
		}
		if font.About == "" {
			t.Errorf("%s says nothing about itself", font.Name)
		}
		if got := weightClass(data); got != font.Weight {
			t.Errorf("%s is offered as weight %d, the face says %d", font.Name, font.Weight, got)
		}
	}
	if _, ok := FontByName("Comic Sans MS"); ok {
		t.Error("a face nobody bundled was found")
	}
	// What a plan gets without saying anything has to be one of ours.
	if _, ok := FontByName(ResolveStyle(nil).Font); !ok {
		t.Errorf("the default face %q is not bundled", ResolveStyle(nil).Font)
	}
}

// libass thickens a face only where the bold asks for more than 150 above
// the face's own weight, by an em over 64, and the video preview draws the
// word typed the same way, see Thickening. Found by Tim: Archivo Black, a
// face of weight 400, was thickened by libass for bold captions and faked
// bold another way by the Mac, and the word typed was lighter and narrower.
func TestTheFacesLibassThickensAreKnown(t *testing.T) {
	for _, c := range []struct {
		font    string
		bold    float64
		weight  int
		thicken bool
	}{
		{"Inter Black", 1, 900, false},
		{"Archivo Black", 1, 400, true},
		{"Archivo Black", -1, 400, true},
		{"Archivo Black", 0, 400, false},
		{"Archivo Black", 550, 400, false},
		{"Archivo Black", 551, 400, true},
		{"Anton", 1, 400, true},
		{"Comic Sans MS", 1, 400, true},
		{"Comic Sans MS", 0, 400, false},
	} {
		weight, thicken := Thickening(c.font, c.bold)
		if weight != c.weight || (thicken != 0) != c.thicken || (thicken != 0 && thicken != 1.0/64) {
			t.Errorf("%s at bold %g is weight %d thickened by %g, want weight %d thickened %t", c.font, c.bold, weight, thicken, c.weight, c.thicken)
		}
	}
}

// On the real disk: InstallFont puts the face where ffmpeg's captions read it.
func TestInstallFontLeavesTheFaceAndItsLicenceBehind(t *testing.T) {
	dir := t.TempDir()
	if err := InstallFont(dir, DefaultFont); err != nil {
		t.Fatal(err)
	}
	font, _ := FontByName(DefaultFont)
	for _, name := range []string{font.File, font.Licence} {
		info, err := os.Stat(filepath.Join(dir, FontsFolder, name))
		if err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Installing again keeps the file that is already there, so a render
	// does not rewrite a megabyte every time.
	path := filepath.Join(dir, FontsFolder, font.File)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	at := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	if err := InstallFont(dir, DefaultFont); err != nil {
		t.Fatal(err)
	}
	if after, err := os.Stat(path); err != nil || !after.ModTime().Equal(at) {
		t.Errorf("the face was written again")
	}

	// A face we do not ship is the machine's business, not an error.
	if err := InstallFont(dir, "Helvetica"); err != nil {
		t.Errorf("an unknown face: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, FontsFolder)); len(entries) != 2 {
		t.Errorf("the folder holds %d files", len(entries))
	}
}

// Measuring goes through libass with the very font the render uses, so it
// is the cheapest honest proof that a bundled face reaches it. On a machine
// that has none of these faces installed, a narrow one and a wide one can
// only come out differently when the bundle was found.
//
// On the real disk: ffmpeg reads the bundled face.
func TestABundledFaceReachesLibass(t *testing.T) {
	t.Parallel()
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(context.Background()); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	width, size := 1080, 96
	measure := func(name string) *inkBox {
		s := ResolveStyle(map[string]any{"font": name, "highlight": 0.0})
		boxes, ok := e.measureMany(context.Background(),
			[]string{`{\alpha&H00&}Hamburgefonstiv`}, s, width, size)
		if !ok || len(boxes) != 1 || boxes[0] == nil {
			ffmpegtest.Unusable(t, "this ffmpeg cannot measure captions")
		}
		return boxes[0]
	}
	wide := measure(DefaultFont)
	narrow := measure("Anton")
	if wide.right-wide.left == narrow.right-narrow.left {
		t.Errorf("%s and Anton draw the same width, %d, so neither face was found",
			DefaultFont, wide.right-wide.left)
	}
}

// The filter the render uses has to be the one that was probed, fonts and
// all, or a syntax that works in the probe fails in the render.
func TestTheSubtitleProbeTriesTheBundledFontsFirst(t *testing.T) {
	if len(subtitleCandidates) == 0 || fontsOption == "" {
		t.Fatal("no subtitle syntax to try")
	}
	chain := subtitleChain(subtitleCandidates[0]+fontsOption, "01_a.ass")
	if chain != "ass=filename=01_a.ass:fontsdir=fonts" {
		t.Errorf("the chain reads %q", chain)
	}
}

// weightClass reads usWeightClass from the OS/2 table of a TrueType face:
// the table directory starts at 12, sixteen bytes a table, and the weight is
// the third field of OS/2, at 4.
func weightClass(face []byte) int {
	be := func(b []byte) int { return int(b[0])<<8 | int(b[1]) }
	tables := be(face[4:6])
	for i := 0; i < tables; i++ {
		entry := face[12+16*i:]
		if string(entry[:4]) != "OS/2" {
			continue
		}
		at := int(entry[8])<<24 | int(entry[9])<<16 | int(entry[10])<<8 | int(entry[11])
		return be(face[at+4 : at+6])
	}
	return 0
}
