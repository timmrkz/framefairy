package engine

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	}
	if _, ok := FontByName("Comic Sans MS"); ok {
		t.Error("a face nobody bundled was found")
	}
	// What a plan gets without saying anything has to be one of ours.
	if _, ok := FontByName(ResolveStyle(nil).Font); !ok {
		t.Errorf("the default face %q is not bundled", ResolveStyle(nil).Font)
	}
}

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
func TestABundledFaceReachesLibass(t *testing.T) {
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if err := e.Preflight(context.Background()); err != nil {
		t.Skip("no usable ffmpeg here")
	}
	width, size := 1080, 96
	measure := func(name string) *inkBox {
		s := ResolveStyle(map[string]any{"font": name, "highlight": 0.0})
		boxes, ok := e.measureMany(context.Background(),
			[]string{`{\alpha&H00&}Hamburgefonstiv`}, s, width, size)
		if !ok || len(boxes) != 1 || boxes[0] == nil {
			t.Skipf("this ffmpeg cannot measure captions")
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
