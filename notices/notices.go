// Package notices holds the licence of every piece of other people's work
// that Frame Fairy is made of or brings with it, and the text each licence
// asks to travel with every copy.
//
// Nothing in here is written by hand except the texts in kept/. The rest is
// made by gen/ from what the programs are really built from: the Go modules
// they compile, the packages the interface bundles, and the source trees
// make builds ffmpeg and llama-server from. The tests in this package fail
// when something the programs are built from has no notice, so the list can
// not quietly fall behind.
package notices

import (
	"embed"
	"encoding/json"
	"path"
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

// All is every notice, in the order of Parts and by name within a part.
func All() ([]Notice, error) {
	raw, err := files.ReadFile("notices.json")
	if err != nil {
		return nil, err
	}
	var list []Notice
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Text is the content of one file in texts/.
func Text(name string) (string, error) {
	raw, err := files.ReadFile(path.Join("texts", path.Base(name)))
	return string(raw), err
}
