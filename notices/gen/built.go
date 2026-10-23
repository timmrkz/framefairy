package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"framefairy/notices/licence"

	"github.com/spf13/afero"
)

// programs is what ships. framefairy-frames is built with the tag
// ffmpeglibs, without which it compiles to a program that only says it was
// built without them.
var programs = []string{"./cmd/framefairy-app", "./cmd/framefairy", "./cmd/framefairy-frames"}

// extraTexts are licences inside a module besides the one at its top, for
// code the module carries from someone else.
var extraTexts = map[string][]string{
	"github.com/wailsapp/wails/v3": {
		"internal/go-common-file-dialog/LICENSE",
		"internal/webview2/webviewloader/LICENSE",
	},
}

// bundled is the file the interface build writes beside itself, naming
// the folder of every npm package the bundle took code from, see
// frontend/vite.config.ts.
const bundled = "cmd/framefairy-app/dist/packages.json"

// module is a Go module as go list names it: the version and the folder
// are the ones the build really uses, a replacement's where go.mod
// replaces it.
type module struct{ Path, Version, Dir string }

// built writes the notices of every Go module the programs compile on this
// system and of every npm package in the interface bundle into one file,
// with their texts in it. notices.All reads it beside notices.json.
func built(fsys afero.Fs, into string) error {
	modules, err := compiled(".", "")
	if err != nil {
		return err
	}
	for _, m := range modules {
		n, sources, err := moduleNotice(fsys, m)
		if err != nil {
			return err
		}
		if err := add(n, sources); err != nil {
			return err
		}
	}
	raw, err := afero.ReadFile(fsys, bundled)
	if err != nil {
		return fmt.Errorf("%w. Build the interface first, with make", err)
	}
	var folders []string
	if err := json.Unmarshal(raw, &folders); err != nil {
		return fmt.Errorf("%s: %w", bundled, err)
	}
	cache, err := goEnv("GOMODCACHE")
	if err != nil {
		return err
	}
	for _, folder := range folders {
		n, sources, err := packageNotice(fsys, "frontend", folder, cache)
		if err != nil {
			return err
		}
		if err := add(n, sources); err != nil {
			return err
		}
	}
	sortList()
	body, err := json.MarshalIndent(struct {
		Notices []notice          `json:"notices"`
		Texts   map[string]string `json:"texts"`
	}{list, textsOf(made)}, "", "  ")
	if err != nil {
		return err
	}
	return afero.WriteFile(fsys, into, append(body, '\n'), 0o644)
}

func textsOf(made map[string][]byte) map[string]string {
	texts := make(map[string]string, len(made))
	for name, body := range made {
		texts[name] = string(body) + "\n"
	}
	return texts
}

// compiled is every module the programs in the module at dir compile for
// goos, or for the system go builds for when goos is empty.
func compiled(dir, goos string) ([]module, error) {
	args := append([]string{"list", "-tags", "ffmpeglibs", "-deps", "-f",
		`{{with .Module}}{{if not .Main}}{{.Path}}{{"\t"}}` +
			`{{with .Replace}}{{or .Version "a copy of its own"}}{{"\t"}}{{.Dir}}{{else}}{{.Version}}{{"\t"}}{{.Dir}}{{end}}` +
			`{{end}}{{end}}`}, programs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if goos != "" {
		cmd.Env = append(cmd.Env, "GOOS="+goos, "GOARCH=arm64", "CGO_ENABLED=1")
	}
	cmd.Stderr = os.Stderr
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	found := map[string]module{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 3 {
			found[fields[0]] = module{Path: fields[0], Version: fields[1], Dir: fields[2]}
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("go list found no modules at all")
	}
	modules := make([]module, 0, len(found))
	for _, m := range found {
		modules = append(modules, m)
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Path < modules[j].Path })
	return modules, nil
}

// moduleNotice is a Go module's notice, refused when its licence is not
// one a paid app may ship. The speech bindings are a Go module whose
// native libraries travel beside the app, so they are shown under speech.
func moduleNotice(fsys afero.Fs, m module) (notice, map[string][]byte, error) {
	if m.Dir == "" {
		return notice{}, nil, fmt.Errorf("%s is not in the module cache. Run go mod download", m.Path)
	}
	name, body, err := licence.In(fsys, m.Dir)
	if err != nil {
		return notice{}, nil, err
	}
	kind := licence.Classify(string(body))
	if !licence.MayShip(kind) {
		return notice{}, nil, refused(m.Path, kind, filepath.Join(m.Dir, name))
	}
	sources := map[string][]byte{slug(m.Path) + ".txt": body}
	for _, extra := range extraTexts[m.Path] {
		more, err := afero.ReadFile(fsys, filepath.Join(m.Dir, extra))
		if err != nil {
			return notice{}, nil, err
		}
		sources[slug(m.Path+"-"+filepath.Dir(extra))+".txt"] = more
	}
	part := partApp
	if strings.HasPrefix(m.Path, "github.com/k2-fsa/") {
		part = partSpeech
	}
	return notice{Name: m.Path, Version: strings.TrimPrefix(m.Version, "v"), Licence: kind,
		URL: "https://" + m.Path, Part: part}, sources, nil
}

// packageNotice is the notice of the npm package in folder, under root,
// refused when its licence is not one a paid app may ship. cache is the
// Go module cache, where the Wails runtime's text is.
func packageNotice(fsys afero.Fs, root, folder, cache string) (notice, map[string][]byte, error) {
	dir := filepath.Join(root, filepath.FromSlash(folder))
	raw, err := afero.ReadFile(fsys, filepath.Join(dir, "package.json"))
	if err != nil {
		return notice{}, nil, err
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		License string `json:"license"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return notice{}, nil, fmt.Errorf("%s: %w", folder, err)
	}
	// The folder is what the bundle took the code from, so a copy one
	// package carries in its own node_modules is named for itself.
	if want := path.Base(path.Dir(folder)) + "/" + path.Base(folder); pkg.Name != path.Base(folder) && pkg.Name != want {
		return notice{}, nil, fmt.Errorf("%s holds the package %q", folder, pkg.Name)
	}
	name, body, err := licence.In(fsys, dir)
	if err != nil && pkg.Name == "@wailsio/runtime" {
		// The Wails runtime is published without its licence file. It is
		// the same project and the same licence as the Wails module, at
		// the same version, so the text is taken from there.
		name = "LICENSE"
		dir = filepath.Join(cache, "github.com", "wailsapp", "wails", "v3@v"+pkg.Version)
		body, err = afero.ReadFile(fsys, filepath.Join(dir, name))
	}
	if err != nil {
		return notice{}, nil, err
	}
	kind := pkg.License
	if kind == "" {
		kind = licence.Classify(string(body))
	}
	if !licence.MayShip(kind) {
		return notice{}, nil, refused(pkg.Name, kind, filepath.Join(dir, name))
	}
	return notice{Name: pkg.Name, Version: pkg.Version, Licence: kind,
			URL: "https://www.npmjs.com/package/" + pkg.Name, Part: partScreen},
		map[string][]byte{slug("npm-"+pkg.Name) + ".txt": body}, nil
}

func refused(name, kind, text string) error {
	if kind == "" {
		kind = "a licence that cannot be told from its text"
	}
	return fmt.Errorf("%s is under %s, which is not a licence the app may ship yet. "+
		"Read %s, and if a paid app may ship it, add it to allowed in notices/licence", name, kind, text)
}
