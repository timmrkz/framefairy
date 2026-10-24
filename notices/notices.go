// Package notices holds the licence of every piece of other people's work
// that Frame Fairy is made of or brings with it, and the text each licence
// asks to travel with every copy.
//
// Two lists make up what the app shows. notices.json, with its texts, is
// kept here: Go itself, ffmpeg and llama-server with what is in them, the
// speech runtime, the fonts, the hyphenation patterns and the models,
// whose versions are pinned by hand. make notices writes it with gen/ from
// what make builds them from, and nothing in it is written by hand except
// the texts in kept/. The tests in this package fail when a pin moves
// without it.
//
// The Go modules the programs compile and the packages the interface
// bundles are not kept here. The build writes their notices into its own
// output, from what that build really has, with gen/ -built, and the app
// hands them to All and Text. So a Dependabot update needs no notice
// written, and no version can fall behind.
package notices

import (
	"embed"
	"encoding/json"
	"path"
	"sort"
	"strings"
)

// Notice is one piece of work and its licence.
type Notice struct {
	// Name is what the work is called by the people who make it.
	Name    string `json:"name"`
	Version string `json:"version"`
	// Licence is the SPDX identifier, like MIT or Apache-2.0.
	Licence string `json:"licence"`
	URL     string `json:"url"`
	// Part is where it is in Frame Fairy, one of the Part constants.
	Part string `json:"part"`
	// Note is anything the licence asks to be said besides its text, like
	// where the source is or what was changed.
	Note string `json:"note,omitempty"`
	// Texts are the files in texts/ that hold its licence and notices.
	Texts []string `json:"texts"`
}

// Where a piece of work is in Frame Fairy.
const (
	PartApp      = "The app"
	PartScreen   = "The app's interface"
	PartSpeech   = "Speech recognition"
	PartFFmpeg   = "ffmpeg"
	PartLlama    = "llama-server"
	PartFonts    = "Caption fonts"
	PartDownload = "Fetched by the app from its maker"
)

// Parts is the order the parts are shown in.
var Parts = []string{PartApp, PartScreen, PartSpeech, PartFFmpeg, PartLlama, PartFonts, PartDownload}

//go:embed notices.json texts
var files embed.FS

// Built is what the build wrote about the Go modules and the interface
// packages it is made of: their notices, and their texts by name.
type Built struct {
	Notices []Notice          `json:"notices"`
	Texts   map[string]string `json:"texts"`
}

// ReadBuilt reads what gen -built wrote. Nothing, from a program built
// without make, is no notices.
func ReadBuilt(raw []byte) (Built, error) {
	var b Built
	if len(raw) == 0 {
		return b, nil
	}
	err := json.Unmarshal(raw, &b)
	return b, err
}

// All is every notice, the ones kept here and the ones the build wrote, in
// the order of Parts and by name within a part.
func All(built Built) ([]Notice, error) {
	raw, err := files.ReadFile("notices.json")
	if err != nil {
		return nil, err
	}
	var list []Notice
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	list = append(list, built.Notices...)
	order := map[string]int{}
	for i, p := range Parts {
		order[p] = i
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Part != list[j].Part {
			return order[list[i].Part] < order[list[j].Part]
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list, nil
}

// Text is one text a notice names, from texts/ or from the build.
func Text(built Built, name string) (string, error) {
	if text, ok := built.Texts[name]; ok {
		return text, nil
	}
	raw, err := files.ReadFile(path.Join("texts", path.Base(name)))
	return string(raw), err
}
