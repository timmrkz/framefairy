package engine

// The caption fonts travel with the program.
//
// A caption has to look the same on every machine, and a user should never
// have to install a font before a short can be rendered. So the faces the
// captions are written in are built into the binary. Before a render, the
// face a plan asks for is written next to the caption file and libass is
// pointed at that folder, which also keeps every path out of the filter
// graph. A face the bundle does not have is left to the machine.
//
// All three faces are under the SIL Open Font Licence 1.1. Their licence
// text is copied out with them, so it travels wherever the font travels.
// See docs/THIRD_PARTY.md.

import (
	"embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed fonts
var fontBundle embed.FS

// FontsFolder is the folder beside a caption file that the render points
// libass at. It is a plain relative name on purpose.
const FontsFolder = "fonts"

// DefaultFont is the face captions are written in unless a plan or a
// setting says otherwise.
const DefaultFont = "Inter Black"

// CaptionFont is one face the captions can be written in.
type CaptionFont struct {
	// Name is the family name, which is what a caption style names.
	Name string `json:"name"`
	// About says in a few words what it looks like.
	About string `json:"about"`
	// File is the name of the face inside the bundle.
	File string `json:"file"`
	// Weight is how heavy the face says it is, the weight class in its own
	// OS/2 table. The video preview declares it with the face, so a browser
	// fakes bold exactly where libass does and nowhere else: libass thickens
	// a face only when bold asks for more than 150 above its weight, and a
	// browser thickens every face that is declared light enough.
	Weight int `json:"weight"`
	// Licence is the licence text that belongs to it.
	Licence string `json:"-"`
	// Machine is a face installed on the machine rather than built into
	// the program.
	Machine bool `json:"machine,omitempty"`
}

var captionFonts = []CaptionFont{
	{Name: DefaultFont, About: "plain and very heavy", File: "Inter-Black.ttf",
		Weight: 900, Licence: "LICENSE-Inter.txt"},
	{Name: "Anton", About: "narrow, the usual look of a short", File: "Anton-Regular.ttf",
		Weight: 400, Licence: "LICENSE-Anton.txt"},
	{Name: "Archivo Black", About: "wide and solid", File: "ArchivoBlack-Regular.ttf",
		Weight: 400, Licence: "LICENSE-Archivo.txt"},
}

// CaptionFonts are the faces captions can be written in, in the order they
// are offered: the ones the program ships with, then the machine's own by
// name.
func CaptionFonts() []CaptionFont {
	out := append([]CaptionFont(nil), captionFonts...)
	for _, face := range shelf.all() {
		out = append(out, face.font)
	}
	return out
}

// Thickening is how libass draws a face at a bold, see ass_font.c in
// libass: weight is the face's own, which a browser is to draw it at so it
// fakes no bold of its own, and thicken is how much libass grows the
// outline of every letter, as a share of the em, where the bold asks for
// more than 150 above the face's weight. It grows each outline by an em
// over 64, half of it on either side, and leaves the advances alone,
// ass_glyph_embolden. Bold is the caption style's: 1 or -1 is 700, 0 or
// less is 400, and anything else is the weight itself. A face the bundle
// nor the machine has counts as 400.
func Thickening(font string, bold float64) (weight int, thicken float64) {
	weight = 400
	if f, ok := FontByName(font); ok {
		weight = f.Weight
	}
	asked := int(bold)
	switch {
	case asked == 1 || asked == -1:
		asked = 700
	case asked <= 0:
		asked = 400
	}
	if asked > weight+150 {
		thicken = 1.0 / 64
	}
	return weight, thicken
}

// FontByName finds a face by the name it is offered under, a bundled one
// first.
func FontByName(name string) (CaptionFont, bool) {
	for _, font := range captionFonts {
		if font.Name == name {
			return font, true
		}
	}
	if face, ok := shelf.find(name); ok {
		return face.font, true
	}
	return CaptionFont{}, false
}

// FontBytes gives the file of a face. For one of the machine's it is the
// copy the captions are drawn from, see machinefonts.go.
func FontBytes(name string) ([]byte, bool) {
	font, ok := FontByName(name)
	if !ok {
		return nil, false
	}
	if font.Machine {
		face, _ := shelf.find(name)
		data, err := machineFontBytes(face)
		return data, err == nil
	}
	data, err := fontBundle.ReadFile(FontsFolder + "/" + font.File)
	if err != nil {
		return nil, false
	}
	return data, true
}

// AssFontName is the name a caption file asks libass for to get the face
// offered as name: the name itself for a bundled face, and the name of
// its copy for one of the machine's, see machinefonts.go.
func AssFontName(name string) string {
	if face, ok := shelf.find(name); ok && !isBundled(name) {
		return face.alias
	}
	return name
}

func isBundled(name string) bool {
	for _, font := range captionFonts {
		if font.Name == name {
			return true
		}
	}
	return false
}

// InstallFont writes the face a caption style asks for into dir/fonts, with
// its licence beside it when it is one of ours, so the render finds it
// with nothing else to look for. A face nobody has is not an error: the
// style has already put DefaultFont in its place.
func InstallFont(dir, name string) error {
	font, ok := FontByName(name)
	if !ok {
		return nil
	}
	if font.Machine {
		return installMachineFont(dir, name)
	}
	// Inside dir, which is there, and never dir itself, so a folder that
	// went under it stays gone, see foldersIn.
	target := filepath.Join(dir, FontsFolder)
	if err := os.Mkdir(target, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	for _, file := range []string{font.File, font.Licence} {
		data, err := fontBundle.ReadFile(FontsFolder + "/" + file)
		if err != nil {
			return err
		}
		path := filepath.Join(target, file)
		if old, err := os.ReadFile(path); err == nil && len(old) == len(data) {
			continue
		}
		// Through a name of its own, so libass opening the face while a
		// second render or preview installs it never reads half a face.
		if err := writeWhole(target, path, data); err != nil {
			return err
		}
	}
	return nil
}

// installMachineFont writes the copy of a face of the machine into
// dir/fonts. Its file is named after the face and the version of the file
// it came from, so one that is there already is the same face.
func installMachineFont(dir, name string) error {
	face, ok := shelf.find(name)
	if !ok {
		return nil
	}
	target := filepath.Join(dir, FontsFolder)
	if err := os.Mkdir(target, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	path := filepath.Join(target, face.font.File)
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return nil
	}
	data, err := machineFontBytes(face)
	if err != nil {
		return err
	}
	return writeWhole(target, path, data)
}
