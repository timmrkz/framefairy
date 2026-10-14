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
}

var captionFonts = []CaptionFont{
	{Name: DefaultFont, About: "plain and very heavy", File: "Inter-Black.ttf",
		Weight: 900, Licence: "LICENSE-Inter.txt"},
	{Name: "Anton", About: "narrow, the usual look of a short", File: "Anton-Regular.ttf",
		Weight: 400, Licence: "LICENSE-Anton.txt"},
	{Name: "Archivo Black", About: "wide and solid", File: "ArchivoBlack-Regular.ttf",
		Weight: 400, Licence: "LICENSE-Archivo.txt"},
}

// CaptionFonts are the faces the program ships with, in the order they are
// offered.
func CaptionFonts() []CaptionFont {
	return append([]CaptionFont(nil), captionFonts...)
}

// FontByName finds a bundled face by its family name.
func FontByName(name string) (CaptionFont, bool) {
	for _, font := range captionFonts {
		if font.Name == name {
			return font, true
		}
	}
	return CaptionFont{}, false
}

// FontBytes gives the file of a bundled face.
func FontBytes(name string) ([]byte, bool) {
	font, ok := FontByName(name)
	if !ok {
		return nil, false
	}
	data, err := fontBundle.ReadFile(FontsFolder + "/" + font.File)
	if err != nil {
		return nil, false
	}
	return data, true
}

// InstallFont writes the face a caption style asks for into dir/fonts, with
// its licence beside it, so the render finds it with nothing installed on
// the machine. A face the bundle does not have is left to the machine and
// is not an error.
func InstallFont(dir, name string) error {
	font, ok := FontByName(name)
	if !ok {
		return nil
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
