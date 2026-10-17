package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/internal/ffmpegtest"
)

// The fonts installed on the machine are offered beside ours, see
// machinefonts.go. These tests put faces on the shelf, which every test of
// the package sees, so none of them runs beside another.

// namedFace is a bundled face under another name, the way a face the
// machine has would be.
func namedFace(t testing.TB, bundled, name string) []byte {
	t.Helper()
	data, ok := FontBytes(bundled)
	if !ok {
		t.Fatalf("%s is not bundled", bundled)
	}
	face, err := copyFace(bytes.NewReader(data), int64(len(data)), 0, name)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// collection puts faces into one file, the way a .ttc holds a family.
func collection(faces ...[]byte) []byte {
	head := make([]byte, 12+4*len(faces))
	copy(head, "ttcf")
	binary.BigEndian.PutUint32(head[4:], 0x00010000)
	binary.BigEndian.PutUint32(head[8:], uint32(len(faces)))
	out := head
	for i, face := range faces {
		at := uint32(len(out))
		binary.BigEndian.PutUint32(out[12+4*i:], at)
		count := int(binary.BigEndian.Uint16(face[4:]))
		moved := append([]byte(nil), face...)
		for k := range count {
			entry := moved[12+16*k:]
			binary.BigEndian.PutUint32(entry[8:], binary.BigEndian.Uint32(entry[8:])+at)
		}
		out = append(out, moved...)
	}
	return out
}

// onShelf puts files on a shelf in memory, one folder each, in the order
// given, and takes them off again when the test ends.
func onShelf(t *testing.T, folders ...map[string][]byte) {
	t.Helper()
	fsys := afero.NewMemMapFs()
	var dirs []string
	for i, files := range folders {
		dir := filepath.Join(string(filepath.Separator), "fonts", string(rune('a'+i)))
		dirs = append(dirs, dir)
		for name, data := range files {
			path := filepath.Join(dir, name)
			if err := fsys.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := afero.WriteFile(fsys, path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	useFontFolders(fsys, dirs)
	t.Cleanup(func() { useFontFolders(nil, nil) })
}

func fontNames(fonts []CaptionFont) []string {
	var out []string
	for _, font := range fonts {
		out = append(out, font.Name)
	}
	return out
}

func TestTheMachinesFontsAreOfferedAfterOurs(t *testing.T) {
	narrow := namedFace(t, "Anton", "Test Narrow")
	onShelf(t,
		map[string][]byte{
			"Narrow.ttf": narrow,
			// One file with two faces, the way macOS keeps a family.
			"Family/Pair.ttc": collection(namedFace(t, "Archivo Black", "Test Wide"),
				namedFace(t, DefaultFont, "Test Heavy")),
			// A face of ours installed on the machine too stays ours.
			"Anton.ttf": namedFace(t, "Anton", "Anton"),
			// macOS keeps the faces of its own interface under a name with a
			// dot in front, and they are not for captions.
			"SFNS.ttf": namedFace(t, "Anton", ".SF NS"),
			// A style line cannot hold a comma.
			"Comma.ttf":  namedFace(t, "Anton", "Comma, Inc"),
			"notes.txt":  []byte("not a font"),
			"Broken.ttf": narrow[:100],
		},
		// The same name in a folder read later is the first one's.
		map[string][]byte{"Again.otf": namedFace(t, "Archivo Black", "Test Narrow")},
	)
	got := strings.Join(fontNames(CaptionFonts()), ", ")
	want := "Inter Black, Anton, Archivo Black, Test Heavy, Test Narrow, Test Wide"
	if got != want {
		t.Fatalf("offered %s, want %s", got, want)
	}
	for _, c := range []struct {
		name   string
		weight int
		like   string
	}{
		{"Test Narrow", 400, "Anton"},
		{"Test Wide", 400, "Archivo Black"},
		{"Test Heavy", 900, DefaultFont},
	} {
		font, ok := FontByName(c.name)
		if !ok || !font.Machine || font.Weight != c.weight {
			t.Errorf("%s is %+v", c.name, font)
		}
		// Measured from its own file, so a line breaks where libass breaks
		// it, the same as the face it was made from.
		ours, _ := TextWidth(c.like, "Hamburgefonstiv", 96)
		theirs, ok := TextWidth(c.name, "Hamburgefonstiv", 96)
		if !ok || theirs != ours {
			t.Errorf("%s measures %g, %s %g", c.name, theirs, c.like, ours)
		}
		if weight, _ := Thickening(c.name, 1); weight != c.weight {
			t.Errorf("%s thickens as weight %d", c.name, weight)
		}
	}
	if font, _ := FontByName("Anton"); font.Machine {
		t.Error("Anton installed on the machine took the place of ours")
	}
}

// The copy a caption file is drawn from answers to its alias and to
// nothing else, so libass cannot take another face of the family for it,
// and keeps the face's own weight and letters.
func TestTheCopyOfAMachineFaceIsThatFaceAlone(t *testing.T) {
	onShelf(t, map[string][]byte{"Pair.ttc": collection(
		namedFace(t, "Archivo Black", "Test Wide"), namedFace(t, DefaultFont, "Test Heavy"))})
	face, ok := shelf.find("Test Heavy")
	if !ok {
		t.Fatal("Test Heavy is not on the shelf")
	}
	data, ok := FontBytes("Test Heavy")
	if !ok {
		t.Fatal("no copy of Test Heavy")
	}
	faces := readSfntFaces(bytes.NewReader(data), int64(len(data)))
	if len(faces) != 1 || faces[0].name != face.alias || faces[0].weight != 900 {
		t.Fatalf("the copy holds %+v, want one face named %q of weight 900", faces, face.alias)
	}
	if got := AssFontName("Test Heavy"); got != face.alias {
		t.Errorf("a caption file asks for %q, want %q", got, face.alias)
	}
	if got := AssFontName("Anton"); got != "Anton" {
		t.Errorf("a caption file asks for %q for Anton", got)
	}
	// Its checksums add up, as a font file's should.
	if sum := tableSum(data); sum != 0xB1B0AFBA {
		t.Errorf("the file sums to %#x", sum)
	}
}

// A style that names a face nobody has is drawn in ours, and says which one
// it asked for, so the Font field can tell. One the machine has is kept.
func TestAFaceNobodyHasIsDrawnInOurs(t *testing.T) {
	onShelf(t, map[string][]byte{"Narrow.ttf": namedFace(t, "Anton", "Test Narrow")})
	s := ResolveStyle(map[string]any{"font": "Gone Grotesk"})
	if s.Font != DefaultFont || s.Missing != "Gone Grotesk" {
		t.Errorf("drawn in %q with %q missing", s.Font, s.Missing)
	}
	s = ResolveStyle(map[string]any{"font": "Test Narrow"})
	if s.Font != "Test Narrow" || s.Missing != "" {
		t.Errorf("drawn in %q with %q missing", s.Font, s.Missing)
	}
	header := assHeader(s, 1080, 1920, 96, 4, 0, 0, 70, 300)
	if !strings.Contains(header, "Style: Caption,"+AssFontName("Test Narrow")+",") {
		t.Errorf("the caption file asks for another face:\n%s", header)
	}
	if err := SetCaptionStyle(afero.NewMemMapFs(), "/plan.json", map[string]any{"font": "Gone Grotesk"}); err == nil {
		t.Error("a face nobody has was set as the caption font")
	}
}

// On the real disk: the copy is written where ffmpeg's captions read it,
// once.
func TestInstallFontCopiesAMachineFace(t *testing.T) {
	onShelf(t, map[string][]byte{"Narrow.ttf": namedFace(t, "Anton", "Test Narrow")})
	dir := t.TempDir()
	if err := InstallFont(dir, "Test Narrow"); err != nil {
		t.Fatal(err)
	}
	font, _ := FontByName("Test Narrow")
	path := filepath.Join(dir, FontsFolder, font.File)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := FontBytes("Test Narrow"); !bytes.Equal(data, want) {
		t.Error("the copy beside the captions is not the face")
	}
	if err := os.WriteFile(path, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InstallFont(dir, "Test Narrow"); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(path); string(again) != "kept" {
		t.Error("the copy was written again")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, FontsFolder)); len(entries) != 1 {
		t.Errorf("the folder holds %d files, a machine face has no licence of ours", len(entries))
	}
}

// libass draws a face of the machine from its copy: the copy of Anton under
// another name comes out exactly as wide as Anton, and not as our default.
//
// On the real disk: ffmpeg reads the copy.
func TestAMachineFaceReachesLibass(t *testing.T) {
	onShelf(t, map[string][]byte{"Narrow.ttf": namedFace(t, "Anton", "Test Narrow")})
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(context.Background()); err != nil {
		ffmpegtest.Unusable(t, "no usable ffmpeg here: %v", err)
	}
	measure := func(name string) *inkBox {
		s := ResolveStyle(map[string]any{"font": name, "highlight": 0.0})
		boxes, ok := e.measureMany(context.Background(),
			[]string{`{\alpha&H00&}Hamburgefonstiv`}, s, 1080, 96)
		if !ok || len(boxes) != 1 || boxes[0] == nil {
			ffmpegtest.Unusable(t, "this ffmpeg cannot measure captions")
		}
		return boxes[0]
	}
	anton, copied, ours := measure("Anton"), measure("Test Narrow"), measure(DefaultFont)
	if copied.right-copied.left != anton.right-anton.left {
		t.Errorf("the copy of Anton draws %d wide, Anton %d",
			copied.right-copied.left, anton.right-anton.left)
	}
	if copied.right-copied.left == ours.right-ours.left {
		t.Errorf("the copy of Anton draws as wide as %s, so it was not found", DefaultFont)
	}
}

// A font file is bytes from outside the program. Whatever it holds, reading
// it neither panics nor reads past it, and a face read from it is copied
// into a file that reads back as one face under its alias.
func FuzzMachineFace(f *testing.F) {
	f.Add(tinyFace("Tiny", 400))
	f.Add(collection(tinyFace("Tiny", 400), tinyFace("Tiny Heavy", 9)))
	f.Add([]byte("ttcf\x00\x01\x00\x00\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, data []byte) {
		size := int64(len(data))
		for _, face := range readSfntFaces(bytes.NewReader(data), size) {
			copied, err := copyFace(bytes.NewReader(data), size, face.offset, "Frame Fairy fuzz")
			if err != nil {
				t.Fatalf("a face that was read could not be copied: %v", err)
			}
			again := readSfntFaces(bytes.NewReader(copied), int64(len(copied)))
			if len(again) != 1 || again[0].name != "Frame Fairy fuzz" || again[0].weight != face.weight {
				t.Fatalf("the copy of %q reads back as %+v", face.name, again)
			}
		}
	})
}

// tinyFace is the smallest face readSfntFaces takes: a cmap that gives
// every letter and digit a glyph, empty outlines, and a name. Fuzzing
// works on a few hundred bytes far faster than on a real face.
func tinyFace(name string, weight uint16) []byte {
	be16 := func(v ...uint16) []byte {
		var out []byte
		for _, x := range v {
			out = binary.BigEndian.AppendUint16(out, x)
		}
		return out
	}
	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head, 0x00010000)
	binary.BigEndian.PutUint16(head[18:], 1000)
	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:], 2)
	hmtx := be16(500, 0, 600, 0)
	// One segment from 0 to z, every character on glyph 1, and the end.
	sub := be16(4, 32, 0, 4, 4, 1, 0, 'z', 0xFFFF, 0, '0', 0xFFFF, 0x10001-'0', 1, 0, 0)
	cmap := append(be16(0, 1, 3, 1, 0, 12), sub...)
	os2 := make([]byte, 78)
	binary.BigEndian.PutUint16(os2[4:], weight)
	tables := []struct {
		tag  string
		data []byte
	}{{"OS/2", os2}, {"cmap", cmap}, {"glyf", nil}, {"head", head},
		{"hhea", hhea}, {"hmtx", hmtx}, {"name", aliasNames(name)}}
	out := make([]byte, 12+16*len(tables))
	binary.BigEndian.PutUint32(out, 0x00010000)
	binary.BigEndian.PutUint16(out[4:], uint16(len(tables)))
	for i, t := range tables {
		at := 12 + 16*i
		copy(out[at:], t.tag)
		binary.BigEndian.PutUint32(out[at+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[at+12:], uint32(len(t.data)))
		out = append(out, t.data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	return out
}

func TestATinyFaceIsAFace(t *testing.T) {
	data := tinyFace("Tiny Bold", 700)
	faces := readSfntFaces(bytes.NewReader(data), int64(len(data)))
	if len(faces) != 1 || faces[0].name != "Tiny Bold" || faces[0].weight != 700 {
		t.Fatalf("read %+v", faces)
	}
}
