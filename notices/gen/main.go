// Command gen writes notices/notices.json and notices/texts/ from what the
// programs are really built from. Run it with make notices, after make has
// built ffmpeg, llama-server and the interface once, because their source
// trees and node_modules are where the texts are read from.
//
// It reads, in this order:
//
//   - the Go toolchain and every Go module the three shipped systems
//     compile into framefairy or framefairy-app, found with go list
//   - every npm package that ends up in the interface bundle, found by
//     building the interface with source maps and reading what they name
//   - the source trees in .build/ that ffmpeg and llama-server are built
//     from, at the versions the build scripts pin
//   - the few texts nothing on disk holds, fetched from their makers
//
// What it cannot place, it refuses, rather than write a notice that says
// less than the licence asks.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// notice is notice, written again because the package it lives in
// embeds what this writes and cannot be built before it exists. The tests
// in notices read what this wrote with the real type.
type notice struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Licence string   `json:"licence"`
	URL     string   `json:"url"`
	Part    string   `json:"part"`
	Note    string   `json:"note,omitempty"`
	Texts   []string `json:"texts"`
}

// The parts, as parts has them and in its order.
const (
	partApp      = "The app"
	partScreen   = "The app's interface"
	partSpeech   = "Speech recognition"
	partFFmpeg   = "ffmpeg"
	partLlama    = "llama-server"
	partFonts    = "Caption fonts"
	partDownload = "Fetched by the app from its maker"
)

var parts = []string{partApp, partScreen, partSpeech, partFFmpeg, partLlama, partFonts, partDownload}

// onnxruntimeVersion is the onnxruntime the sherpa-onnx libraries carry.
// It is not written anywhere a script can read, so it is read off the
// library by hand: strings libonnxruntime.dylib | grep '^1\.'.
const onnxruntimeVersion = "1.28.2"

var (
	out   = "notices"
	texts = filepath.Join(out, "texts")
	list  []notice
	// written are the texts made so far, by name, so two notices that ask
	// for the same file do not write it twice.
	written = map[string]bool{}
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := os.RemoveAll(texts); err != nil {
		return err
	}
	if err := os.MkdirAll(texts, 0o755); err != nil {
		return err
	}
	steps := []func() error{goToolchain, goModules, kept, speech, ffmpeg, llama, fonts, npm, downloads}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	order := map[string]int{}
	for i, p := range parts {
		order[p] = i
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Part != list[j].Part {
			return order[list[i].Part] < order[list[j].Part]
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	body, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "notices.json"), append(body, '\n'), 0o644)
}

// add records a notice, and copies each source file into texts/ under the
// name given beside it.
func add(n notice, sources map[string][]byte) error {
	if n.Licence == "" || n.URL == "" || n.Part == "" || n.Version == "" {
		return fmt.Errorf("%s is missing its licence, address, part or version", n.Name)
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := bytes.TrimSpace(sources[name])
		if len(body) == 0 {
			return fmt.Errorf("the text %s for %s is empty", name, n.Name)
		}
		if !written[name] {
			if err := os.WriteFile(filepath.Join(texts, name), append(body, '\n'), 0o644); err != nil {
				return err
			}
			written[name] = true
		}
		n.Texts = append(n.Texts, name)
	}
	if len(n.Texts) == 0 {
		return fmt.Errorf("%s has no licence text", n.Name)
	}
	list = append(list, n)
	return nil
}

func read(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w. Run make first, so the source trees are there", err)
	}
	return body, nil
}

// slug turns a name into a file name.
func slug(s string) string {
	s = strings.ToLower(s)
	s = regexp.MustCompile(`[^a-z0-9.]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// classify names the licence a text is, from the words every copy of that
// licence has. It is only used where the maker does not say.
func classify(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	switch {
	case strings.Contains(t, "Apache License") && strings.Contains(t, "Version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "Permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "Redistribution and use in source and binary forms") &&
		strings.Contains(t, "Neither the name"):
		return "BSD-3-Clause"
	case strings.Contains(t, "Redistribution and use in source and binary forms"):
		return "BSD-2-Clause"
	case strings.Contains(t, "Permission to use, copy, modify, and/or distribute"):
		return "ISC"
	}
	return ""
}

// licenceFiles are the names a licence goes by at the top of a module or
// package, in the order they are looked for.
var licenceFiles = []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "COPYING", "LICENSE-MIT"}

func licenceIn(dir string) (string, []byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	// Some packages spell it in lower case.
	for _, want := range licenceFiles {
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), want) {
				body, err := os.ReadFile(filepath.Join(dir, e.Name()))
				return e.Name(), body, err
			}
		}
	}
	return "", nil, fmt.Errorf("no licence file in %s", dir)
}

func goEnv(key string) (string, error) {
	raw, err := exec.Command("go", "env", key).Output()
	return strings.TrimSpace(string(raw)), err
}

// goToolchain is Go itself, whose runtime and standard library are
// compiled into every Go program.
func goToolchain() error {
	root, err := goEnv("GOROOT")
	if err != nil {
		return err
	}
	// The version go.mod pins, rather than whichever toolchain ran this,
	// so the notices do not change with the machine that made them.
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	m := regexp.MustCompile(`(?m)^go (\S+)`).FindSubmatch(mod)
	if m == nil {
		return fmt.Errorf("go.mod has no go line")
	}
	version := string(m[1])
	body, err := read(filepath.Join(root, "LICENSE"))
	if err != nil {
		return err
	}
	return add(notice{Name: "Go", Version: version,
		Licence: "BSD-3-Clause", URL: "https://go.dev", Part: partApp,
		Note: "The Go runtime and standard library are compiled into the app."},
		map[string][]byte{"go.txt": body})
}

// Programs is what ships, and systems what it ships on.
var (
	programs = []string{"./cmd/framefairy-app", "./cmd/framefairy"}
	systems  = []string{"darwin", "linux", "windows"}
)

// extraTexts are licences inside a module besides the one at its top, for
// code the module carries from someone else.
var extraTexts = map[string][]string{
	"github.com/wailsapp/wails/v3": {
		"internal/go-common-file-dialog/LICENSE",
		"internal/webview2/webviewloader/LICENSE",
	},
}

// goModules is every module the shipped programs compile, on any of the
// systems they ship on. The speech bindings are a Go module whose native
// libraries travel beside the app, so they are shown under speech.
func goModules() error {
	type module struct{ Path, Version, Dir string }
	found := map[string]module{}
	for _, goos := range systems {
		args := append([]string{"list", "-deps", "-f",
			"{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}"}, programs...)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=arm64", "CGO_ENABLED=1")
		raw, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("go list for %s: %w", goos, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				found[fields[0]] = module{Path: fields[0], Version: fields[1]}
			}
		}
	}
	paths := make([]string, 0, len(found))
	for p := range found {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		m := found[p]
		raw, err := exec.Command("go", "mod", "download", "-json", m.Path+"@"+m.Version).Output()
		if err != nil {
			return fmt.Errorf("go mod download %s: %w", m.Path, err)
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		name, body, err := licenceIn(m.Dir)
		if err != nil {
			return err
		}
		licence := classify(string(body))
		if licence == "" {
			return fmt.Errorf("cannot tell what licence %s/%s is", m.Path, name)
		}
		sources := map[string][]byte{slug(m.Path) + ".txt": body}
		for _, extra := range extraTexts[m.Path] {
			more, err := read(filepath.Join(m.Dir, extra))
			if err != nil {
				return err
			}
			sources[slug(m.Path+"-"+filepath.Dir(extra))+".txt"] = more
		}
		part := partApp
		if strings.HasPrefix(m.Path, "github.com/k2-fsa/") {
			part = partSpeech
		}
		if err := add(notice{Name: m.Path, Version: strings.TrimPrefix(m.Version, "v"),
			Licence: licence, URL: "https://" + m.Path, Part: part}, sources); err != nil {
			return err
		}
	}
	return nil
}

// kept is the code taken into the engine by hand, whose notices are kept
// in kept/ because no module brings them.
func kept() error {
	pigo, err := read(filepath.Join(out, "kept", "pigo.txt"))
	if err != nil {
		return err
	}
	if err := add(notice{Name: "pigo", Version: "1.4.6", Licence: "MIT",
		URL: "https://github.com/esimov/pigo", Part: partApp,
		Note: "The face detector that places the crop is adapted from pigo."},
		map[string][]byte{"pigo.txt": pigo}); err != nil {
		return err
	}
	pico, err := read(filepath.Join(out, "kept", "pico.txt"))
	if err != nil {
		return err
	}
	return add(notice{Name: "pico", Version: "facefinder", Licence: "MIT",
		URL: "https://github.com/nenadmarkus/pico", Part: partApp,
		Note: "The trained face data is the facefinder cascade from pico."},
		map[string][]byte{"pico.txt": pico})
}

// speech is what the sherpa-onnx libraries carry besides sherpa-onnx.
func speech() error {
	license, err := fetch("https://raw.githubusercontent.com/microsoft/onnxruntime/v" +
		onnxruntimeVersion + "/LICENSE")
	if err != nil {
		return err
	}
	third, err := fetch("https://raw.githubusercontent.com/microsoft/onnxruntime/v" +
		onnxruntimeVersion + "/ThirdPartyNotices.txt")
	if err != nil {
		return err
	}
	if err := add(notice{Name: "ONNX Runtime", Version: onnxruntimeVersion, Licence: "MIT",
		URL: "https://github.com/microsoft/onnxruntime", Part: partSpeech,
		Note: "Runs the speech model. It travels beside the app as a library of its own."},
		map[string][]byte{"onnxruntime.txt": license, "onnxruntime-third-party.txt": third}); err != nil {
		return err
	}
	sherpa, err := sherpaVersion()
	if err != nil {
		return err
	}
	// The speech library that travels beside the app is sherpa-onnx's
	// release without speech synthesis, see scripts/speech-libs.sh, and
	// these are what it is built from besides sherpa-onnx itself, at the
	// versions its cmake/ pins for this release. They were read off the
	// library too: each leaves its name in it, and espeak-ng and piper,
	// which the release with synthesis carries, leave none.
	type piece struct{ name, version, licence, url, text, note string }
	gh := "https://raw.githubusercontent.com/"
	pieces := []piece{
		{"sherpa-onnx", sherpa, "Apache-2.0", "https://github.com/k2-fsa/sherpa-onnx",
			gh + "k2-fsa/sherpa-onnx/" + sherpa + "/LICENSE",
			"Recognises the speech. What travels beside the app is its release built without " +
				"speech synthesis, so nothing of espeak-ng is in it."},
		{"kaldi-native-fbank", "1.22.3", "Apache-2.0", "https://github.com/csukuangfj/kaldi-native-fbank",
			gh + "csukuangfj/kaldi-native-fbank/v1.22.3/LICENSE", "Built into sherpa-onnx."},
		{"kaldi-decoder", "0.3.0", "Apache-2.0", "https://github.com/k2-fsa/kaldi-decoder",
			gh + "k2-fsa/kaldi-decoder/v0.3.0/LICENSE", "Built into sherpa-onnx."},
		{"kaldifst", "1.8.0", "Apache-2.0", "https://github.com/k2-fsa/kaldifst",
			gh + "k2-fsa/kaldifst/v1.8.0/LICENSE", "Built into sherpa-onnx."},
		{"OpenFst", "1.8.5", "Apache-2.0", "https://github.com/csukuangfj/openfst",
			gh + "csukuangfj/openfst/v1.8.5-2026-07-09/COPYING", "Built into sherpa-onnx."},
		{"simple-sentencepiece", "0.7", "Apache-2.0", "https://github.com/pkufool/simple-sentencepiece",
			gh + "pkufool/simple-sentencepiece/v0.7/LICENSE", "Built into sherpa-onnx."},
		{"hclust-cpp", "2026-02-25", "BSD-2-Clause", "https://github.com/csukuangfj/hclust-cpp",
			gh + "csukuangfj/hclust-cpp/2026-02-25/LICENSE",
			"Built into sherpa-onnx. It carries fastcluster by Daniel Müllner."},
		{"JSON for Modern C++", "3.12.0", "MIT", "https://github.com/nlohmann/json",
			gh + "nlohmann/json/v3.12.0/LICENSE.MIT", "Built into sherpa-onnx."},
		{"Eigen", "5.0.1", "MPL-2.0", "https://gitlab.com/libeigen/eigen",
			"https://gitlab.com/libeigen/eigen/-/raw/5.0.1/COPYING.MPL2",
			"Built into sherpa-onnx, unmodified. Its source is at https://gitlab.com/libeigen/eigen."},
	}
	for _, p := range pieces {
		body, err := fetch(p.text)
		if err != nil {
			return err
		}
		if err := add(notice{Name: p.name, Version: strings.TrimPrefix(p.version, "v"), Licence: p.licence,
			URL: p.url, Part: partSpeech, Note: p.note},
			map[string][]byte{slug("speech-"+p.name) + ".txt": body}); err != nil {
			return err
		}
	}
	return nil
}

// sherpaVersion is the sherpa-onnx release go.mod pins, v and all.
func sherpaVersion() (string, error) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(`k2-fsa/sherpa-onnx-go (v\S+)`).FindSubmatch(mod)
	if m == nil {
		return "", fmt.Errorf("go.mod does not pin sherpa-onnx")
	}
	return string(m[1]), nil
}

// version reads a pinned version out of a build script, the same way
// pack-source.sh does, so the two never disagree.
func version(key, script string) (string, error) {
	body, err := os.ReadFile(script)
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(`(?m)^` + key + `=(\S+)`).FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("no %s in %s", key, script)
	}
	return string(m[1]), nil
}

func ffmpeg() error {
	const script = "scripts/build-ffmpeg.sh"
	work := filepath.Join(".build", "ffmpeg", "work")
	type lib struct {
		name, key, dir, licence, url, note string
		files                              []string
	}
	libs := []lib{
		{"ffmpeg", "FFMPEG_VERSION", "ffmpeg", "LGPL-2.1-or-later", "https://ffmpeg.org",
			"Built by us, without --enable-gpl and without libx264, and shipped unmodified beside " +
				"the app. The source it was built from, with the scripts that built it, is " +
				"framefairy-tools-source.tar.gz on the same release as the tools, at " +
				"https://github.com/timmrkz/framefairy/releases.",
			[]string{"COPYING.LGPLv2.1", "LICENSE.md"}},
		{"FreeType", "FREETYPE_VERSION", "freetype", "FTL", "https://freetype.org",
			"Portions of this software are copyright © 2024 The FreeType Project " +
				"(https://freetype.org). All rights reserved.",
			[]string{"LICENSE.TXT", "docs/FTL.TXT"}},
		{"FriBidi", "FRIBIDI_VERSION", "fribidi", "LGPL-2.1-or-later", "https://github.com/fribidi/fribidi",
			"Built into ffmpeg. Its source is in the same archive as ffmpeg's.",
			[]string{"COPYING"}},
		{"HarfBuzz", "HARFBUZZ_VERSION", "harfbuzz", "MIT-Modern-Variant", "https://harfbuzz.github.io",
			"", []string{"COPYING"}},
		{"libass", "LIBASS_VERSION", "libass", "ISC", "https://github.com/libass/libass",
			"Draws the captions into the picture.", []string{"COPYING"}},
	}
	for _, l := range libs {
		v, err := version(l.key, script)
		if err != nil {
			return err
		}
		dir := filepath.Join(work, l.dir+"-"+v)
		sources := map[string][]byte{}
		for _, f := range l.files {
			body, err := read(filepath.Join(dir, f))
			if err != nil {
				return err
			}
			sources[slug(l.dir+"-"+textName(f))+".txt"] = body
		}
		if err := add(notice{Name: l.name, Version: v, Licence: l.licence, URL: l.url,
			Part: partFFmpeg, Note: l.note}, sources); err != nil {
			return err
		}
	}
	return nil
}

func llama() error {
	v, err := version("LLAMA_VERSION", "scripts/build-llama.sh")
	if err != nil {
		return err
	}
	src := filepath.Join(".build", "llama", "work", "llama.cpp-"+v)
	body, err := read(filepath.Join(src, "LICENSE"))
	if err != nil {
		return err
	}
	if err := add(notice{Name: "llama.cpp", Version: v, Licence: "MIT",
		URL: "https://github.com/ggml-org/llama.cpp", Part: partLlama,
		Note: "Built by us and shipped beside the app, to run a language model on this machine."},
		map[string][]byte{"llama.cpp.txt": body}); err != nil {
		return err
	}
	vendor := filepath.Join(src, "vendor")
	httplibHeader, err := read(filepath.Join(vendor, "cpp-httplib", "httplib.h"))
	if err != nil {
		return err
	}
	httplibVersion := "unknown"
	if m := regexp.MustCompile(`CPPHTTPLIB_VERSION "([^"]+)"`).FindSubmatch(httplibHeader); m != nil {
		httplibVersion = string(m[1])
	}
	type piece struct {
		name, version, licence, url, file string
		// tail is where the licence starts in a header that carries it at
		// its end, and empty for a file that is only the licence.
		tail string
	}
	pieces := []piece{
		{"cpp-httplib", httplibVersion, "MIT", "https://github.com/yhirose/cpp-httplib", "cpp-httplib/LICENSE", ""},
		{"JSON for Modern C++", "vendored", "MIT", "https://github.com/nlohmann/json", "../licenses/LICENSE-jsonhpp", ""},
		{"stb_image", "vendored", "MIT OR Unlicense", "https://github.com/nothings/stb", "stb/stb_image.h", "This software is available under 2 licenses"},
		{"miniaudio", "vendored", "MIT-0 OR Unlicense", "https://github.com/mackron/miniaudio", "miniaudio/miniaudio.h", "This software is available as a choice of the following licenses"},
		{"subprocess.h", "vendored", "Unlicense", "https://github.com/sheredom/subprocess.h", "sheredom/subprocess.h", ""},
		{"xxHash", "vendored", "BSD-2-Clause", "https://github.com/Cyan4973/xxHash", "hash/xxhash/LICENSE", ""},
		{"rotate-bits", "vendored", "MIT", "https://github.com/jb55/rotate-bits.h", "hash/rotate-bits/LICENSE.md", ""},
		{"sha256", "vendored", "public domain", "https://github.com/jb55/sha256.c", "hash/sha256/LICENSE", ""},
	}
	for _, p := range pieces {
		body, err := read(filepath.Join(vendor, p.file))
		if err != nil {
			return err
		}
		switch {
		case p.tail != "":
			i := bytes.Index(body, []byte(p.tail))
			if i < 0 {
				return fmt.Errorf("the licence at the end of %s moved", p.file)
			}
			body = bytes.TrimSuffix(bytes.TrimSpace(body[i:]), []byte("*/"))
		case p.name == "subprocess.h":
			// The dedication is the comment the header opens with.
			end := bytes.Index(body, []byte("*/"))
			if end < 0 {
				return fmt.Errorf("the dedication at the top of %s moved", p.file)
			}
			body = bytes.TrimPrefix(body[:end], []byte("/*"))
		}
		if err := add(notice{Name: p.name, Version: p.version, Licence: p.licence, URL: p.url,
			Part: partLlama, Note: "Built into llama-server."},
			map[string][]byte{slug("llama-"+p.name) + ".txt": body}); err != nil {
			return err
		}
	}
	return nil
}

func fonts() error {
	type font struct{ name, file, url string }
	for _, f := range []font{
		{"Anton", "LICENSE-Anton.txt", "https://github.com/googlefonts/AntonFont"},
		{"Archivo Black", "LICENSE-Archivo.txt", "https://github.com/Omnibus-Type/ArchivoBlack"},
		{"Inter", "LICENSE-Inter.txt", "https://github.com/rsms/inter"},
	} {
		body, err := read(filepath.Join("engine", "fonts", f.file))
		if err != nil {
			return err
		}
		if err := add(notice{Name: f.name, Version: "as shipped", Licence: "OFL-1.1", URL: f.url,
			Part: partFonts, Note: "Built into the app and written out beside the captions it draws."},
			map[string][]byte{slug("font-"+f.name) + ".txt": body}); err != nil {
			return err
		}
	}
	return nil
}

// npm is every package the interface bundle holds. The interface is built
// once more with source maps into a folder of its own, and the maps say
// which files of which packages went in.
func npm() error {
	tmp, err := os.MkdirTemp("", "framefairy-notices")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cmd := exec.Command("npx", "--no-install", "vite", "build", "--sourcemap",
		"--outDir", tmp, "--emptyOutDir", "--logLevel", "error")
	cmd.Dir = "frontend"
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building the interface: %w", err)
	}
	maps, _ := filepath.Glob(filepath.Join(tmp, "*", "*.map"))
	more, _ := filepath.Glob(filepath.Join(tmp, "*.map"))
	maps = append(maps, more...)
	if len(maps) == 0 {
		return fmt.Errorf("the interface built without source maps")
	}
	inPackage := regexp.MustCompile(`node_modules/((?:@[^/]+/)?[^/]+)/`)
	packages := map[string]bool{}
	for _, m := range maps {
		raw, err := os.ReadFile(m)
		if err != nil {
			return err
		}
		var sm struct{ Sources []string }
		if err := json.Unmarshal(raw, &sm); err != nil {
			return err
		}
		for _, s := range sm.Sources {
			if hit := inPackage.FindStringSubmatch(s); hit != nil {
				packages[hit[1]] = true
			}
		}
	}
	names := make([]string, 0, len(packages))
	for p := range packages {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		dir := filepath.Join("frontend", "node_modules", p)
		raw, err := read(filepath.Join(dir, "package.json"))
		if err != nil {
			return err
		}
		var pkg struct {
			Version    string `json:"version"`
			License    string `json:"license"`
			Homepage   string `json:"homepage"`
			Repository any    `json:"repository"`
		}
		if err := json.Unmarshal(raw, &pkg); err != nil {
			return err
		}
		_, body, err := licenceIn(dir)
		if err != nil && p == "@wailsio/runtime" {
			// The Wails runtime is published without its licence file. It is
			// the same project and the same licence as the Wails module, at
			// the same version, so the text is taken from there.
			body, err = wailsLicence(pkg.Version)
		}
		if err != nil {
			return err
		}
		licence := pkg.License
		if licence == "" {
			licence = classify(string(body))
		}
		if licence == "" {
			return fmt.Errorf("cannot tell what licence %s is", p)
		}
		if err := add(notice{Name: p, Version: pkg.Version, Licence: licence,
			URL: "https://www.npmjs.com/package/" + p, Part: partScreen},
			map[string][]byte{slug("npm-"+p) + ".txt": body}); err != nil {
			return err
		}
	}
	return nil
}

// downloads are the models the app fetches from their makers on the
// user's say. Nothing of them is in the app, and the licence is between
// the user and the maker, but the attribution CC BY asks for is given
// here, where a user looks for it.
func downloads() error {
	ccby, err := fetch(spdx + "CC-BY-4.0.txt")
	if err != nil {
		return err
	}
	if err := add(notice{Name: "Parakeet TDT 0.6B v3", Version: "v3", Licence: "CC-BY-4.0",
		URL: "https://huggingface.co/nvidia/parakeet-tdt-0.6b-v3", Part: partDownload,
		Note: "The speech model, © NVIDIA. Fetched in the int8 ONNX conversion published by the " +
			"sherpa-onnx project, which converted it to ONNX and quantised it to int8. Used unmodified " +
			"from there."},
		map[string][]byte{"cc-by-4.0.txt": ccby}); err != nil {
		return err
	}
	apache, err := fetch(spdx + "Apache-2.0.txt")
	if err != nil {
		return err
	}
	for _, m := range []struct{ name, url, note string }{
		{"Gemma 4", "https://ai.google.dev/gemma", "The language models Gemma 4 26B A4B and Gemma 4 12B, © Google."},
		{"Ministral 3 8B", "https://huggingface.co/mistralai", "A language model, © Mistral AI."},
		{"Qwen3 14B", "https://huggingface.co/Qwen", "A language model, © Alibaba Cloud."},
	} {
		if err := add(notice{Name: m.name, Version: "as fetched", Licence: "Apache-2.0", URL: m.url,
			Part: partDownload, Note: m.note},
			map[string][]byte{"apache-2.0.txt": apache}); err != nil {
			return err
		}
	}
	return nil
}

// spdx is where the texts of the licences themselves are taken from, the
// SPDX project's list, pinned to a release.
const spdx = "https://raw.githubusercontent.com/spdx/license-list-data/v3.27.0/text/"

var client = &http.Client{Timeout: 2 * time.Minute}

func fetch(url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
}

func wailsLicence(version string) ([]byte, error) {
	cache, err := goEnv("GOMODCACHE")
	if err != nil {
		return nil, err
	}
	return read(filepath.Join(cache, "github.com", "wailsapp", "wails", "v3@v"+version, "LICENSE"))
}

// textName is a file's name without an ending that only says it is text.
func textName(file string) string {
	base := filepath.Base(file)
	for _, ext := range []string{".TXT", ".txt", ".md"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}
