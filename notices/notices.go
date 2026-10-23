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
//
// A Go module and an interface package keep no version here. Theirs is the
// one go.mod and the interface's package-lock.json name, which Dependabot
// moves, so an update that keeps its licence needs no new notice.
package notices

import (
	"embed"
	"encoding/json"
	"path"
	"strings"

	"framefairy"
)

// Notice is one piece of work and its licence.
type Notice struct {
	// Name is what the work is called by the people who make it.
	Name string `json:"name"`
	// Version is left out of notices.json for a Go module and an
	// interface package, and All fills it in from what they are built at.
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
	modules := goModules(framefairy.GoMod)
	packages, err := npmPackages(framefairy.PackageLock)
	if err != nil {
		return nil, err
	}
	for i, n := range list {
		if n.Version != "" {
			continue
		}
		if n.Part == PartScreen {
			list[i].Version = packages[n.Name]
		} else {
			list[i].Version = strings.TrimPrefix(modules[n.Name], "v")
		}
	}
	return list, nil
}

// goModules is every module go.mod requires, at the version it names. go
// writes every module the build takes a package from there, at the version
// it takes.
func goModules(mod []byte) map[string]string {
	versions := map[string]string{}
	block := false
	for _, line := range strings.Split(string(mod), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) == 2 && fields[0] == "require" && fields[1] == "(":
			block = true
		case block && len(fields) == 1 && fields[0] == ")":
			block = false
		case block && len(fields) == 2:
			versions[fields[0]] = fields[1]
		case len(fields) == 3 && fields[0] == "require":
			versions[fields[1]] = fields[2]
		}
	}
	return versions
}

// npmPackages is every package the interface's lock file installs, at the
// version it installs.
func npmPackages(lock []byte) (map[string]string, error) {
	var file struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lock, &file); err != nil {
		return nil, err
	}
	versions := map[string]string{}
	for at, p := range file.Packages {
		// A package another package carries in its own node_modules is
		// not what the bundle names it by.
		if name, ok := strings.CutPrefix(at, "node_modules/"); ok && !strings.Contains(name, "/node_modules/") {
			versions[name] = p.Version
		}
	}
	return versions, nil
}

// Text is the content of one file in texts/.
func Text(name string) (string, error) {
	raw, err := files.ReadFile(path.Join("texts", path.Base(name)))
	return string(raw), err
}
