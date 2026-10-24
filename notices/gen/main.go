// Command gen writes the licence notices, in two ways.
//
// With -built, make runs it for every build of the app, after the
// interface: it writes the notices of every Go module the programs compile
// on this system and of every npm package in the interface bundle into the
// build's own output, at the versions and with the texts that build really
// has, see built.go. Nothing of that is kept in the repository, so a
// Dependabot update needs no notice written by hand, and the build stops
// only on a licence nobody allowed, see notices/licence.
//
// Without it, make notices runs it to write notices/notices.json and
// notices/texts/ for everything else, the pieces whose versions are pinned
// by hand and change in a pull request that builds them anyway:
//
//   - the Go toolchain, at the version go.mod pins
//   - the source trees in .build/ that ffmpeg and llama-server are built
//     from, at the versions the build scripts pin, so make builds them
//     first
//   - the hyphenation patterns, the caption fonts and the code kept in
//     kept/
//   - the few texts nothing on disk holds, fetched from their makers
//
// What it cannot place, it refuses, rather than write a notice that says
// less than the licence asks.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
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

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
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
	// made are the texts made so far, by name. Two notices that ask for
	// the same file share it.
	made = map[string][]byte{}
)

func main() {
	into := flag.String("built", "", "write the notices of the Go modules and npm packages this build has into this file")
	flag.Parse()
	var err error
	if *into != "" {
		err = built(afero.NewOsFs(), *into)
	} else {
		err = run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

// run writes notices/. It makes everything first and writes it at the
// end, so a step that fails leaves notices/ as it was.
func run() error {
	steps := []func() error{goToolchain, kept, hyphenation, speech, ffmpeg, llama, fonts, downloads}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	sortList()
	body, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	fresh := texts + ".new"
	if err := os.RemoveAll(fresh); err != nil {
		return err
	}
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		return err
	}
	for name, body := range made {
		if err := os.WriteFile(filepath.Join(fresh, name), append(body, '\n'), 0o644); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(texts); err != nil {
		return err
	}
	if err := os.Rename(fresh, texts); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "notices.json"), append(body, '\n'), 0o644)
}

// sortList puts the notices in the order of parts, and by name within a
// part.
func sortList() {
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
}

// add records a notice, with each of its texts under the name given
// beside it.
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
		made[name] = body
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

// spdxTexts is where the full text of a licence is fetched from when a
// file names its licence without writing it out: the SPDX licence list,
// the register of licence texts, at a release so the texts do not move.
const spdxTexts = "https://raw.githubusercontent.com/spdx/license-list-data/v3.29.0/text/"

// patternLicence is one licence a pattern file is offered under.
type patternLicence struct {
	// spdx is its SPDX identifier, empty for a file's own terms.
	spdx string
	// text is the licence as the file writes it out, if it does.
	text string
}

// shippable are the licences a pattern file may be taken under to go into a
// paid app, the one most wanted first. The permissive ones ask for their
// notice and nothing else. A file's own terms come next, once they have
// been read, see ownTerms. The
// LPPL and MPL 1.1 allow an unchanged file inside a closed program, with
// their text and where the file comes from. The GPL and the LGPL are not
// here: they would ask for the program's source, or for a way to relink it.
var shippable = []string{"MIT", "BSD-3-Clause", "", "LPPL-1.3c", "LPPL-1.0", "MPL-1.1"}

// patternMeta is what hyph-utf8 writes at the top of every pattern file,
// YAML inside TeX comments, up to a line of equals signs.
type patternMeta struct {
	Version  yaml.Node `yaml:"version"`
	Source   string    `yaml:"source"`
	Language struct {
		Name string `yaml:"name"`
		Tag  string `yaml:"tag"`
	} `yaml:"language"`
	Licence yaml.Node `yaml:"licence"`
}

func readPatternMeta(head []byte) (patternMeta, error) {
	var lines []string
	for _, line := range strings.Split(string(head), "\n") {
		if !strings.HasPrefix(line, "%") || strings.HasPrefix(strings.TrimLeft(line, "% "), "===") {
			break
		}
		lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "%"), " "))
	}
	var meta patternMeta
	err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &meta)
	return meta, err
}

// licences are the licences the file is offered under. A licence it names
// but cannot be told apart, and a text of "[None]", are left out, so they
// can never be chosen.
func (m patternMeta) licences() []patternLicence {
	var nodes []*yaml.Node
	switch m.Licence.Kind {
	case yaml.MappingNode:
		nodes = []*yaml.Node{&m.Licence}
	case yaml.SequenceNode:
		nodes = m.Licence.Content
	}
	var out []patternLicence
	for _, node := range nodes {
		var entry struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
			OrLater bool   `yaml:"or_later"`
			Text    string `yaml:"text"`
		}
		if node.Kind != yaml.MappingNode || node.Decode(&entry) != nil {
			continue
		}
		text := strings.TrimSpace(entry.Text)
		if entry.Name == "" {
			if text != "" && text != "[None]" {
				out = append(out, patternLicence{text: text})
			}
			continue
		}
		if id := spdxOf(entry.Name, entry.Version, entry.OrLater); id != "" {
			out = append(out, patternLicence{spdx: id, text: text})
		}
	}
	return out
}

// spdxOf is the SPDX identifier of a licence as a pattern file names it.
func spdxOf(name, version string, orLater bool) string {
	switch n := strings.ToUpper(strings.TrimSpace(name)); {
	case n == "MIT" || n == "MIT/X11":
		return "MIT"
	case strings.HasPrefix(n, "BSD 3"):
		return "BSD-3-Clause"
	case n == "LPPL" && (version == "" || version == "1.3" || orLater):
		return "LPPL-1.3c"
	case n == "LPPL" && version == "1":
		return "LPPL-1.0"
	case n == "MPL" && version == "1.1":
		return "MPL-1.1"
	case n == "GPL":
		return "GPL-" + version
	case n == "LGPL":
		return "LGPL-" + version
	}
	return ""
}

// ownTerms are licences a pattern file writes out and names nowhere, so no
// tool can tell what they allow. Each was read once by a person and found
// to allow the unchanged file in a paid app with its notice, and read a
// second time by someone else, who came to the same answer. They are kept
// by the SHA-256 of the text, the way cargo-deny clarifies a licence, so a
// text that changes, or a new one, is refused until it has been read.
var ownTerms = map[string]string{
	"9fb5df34635858528cc5b3cc91180a42d64e849490319ae7447db01880ebd1bd": "Bulgarian: use, copy, distribute and sell, keeping the notice",
	"8940d49ecfdc619ab795d73e033ace38bf1d01806d39d59cc209f6bc221650f7": "English: copy and distribute in any medium, keeping the notice",
	"f8f12b3fdcd9afc6ed1353d0ab4c520f5af416b9ea72364473bfff6a9e58014e": "Croatian: deal in the files without restriction, keeping the notice",
}

// chooseLicence is the licence a file is taken under: the first of those
// the app may ship, in the order shippable wants them. A file's own terms
// count only once they have been read, see ownTerms. Nil when there is
// none.
func chooseLicence(offered []patternLicence) *patternLicence {
	for _, want := range shippable {
		for i := range offered {
			if offered[i].spdx != want {
				continue
			}
			if want == "" && ownTerms[fmt.Sprintf("%x", sha256.Sum256([]byte(offered[i].text)))] == "" {
				continue
			}
			return &offered[i]
		}
	}
	return nil
}

// hyphenation is the patterns that break long words in the captions: every
// file in engine/hyphenation, with what its own metadata says, what
// language it is, its version, where it came from and the licences it is
// offered under. The first of those the app may ship is the one it is
// taken under, and a file offered under none of them stops the notices.
// The text of each is the top of its file, which names its makers and its
// copyright, with the licence fetched in full where the file only names it.
func hyphenation() error {
	dir := filepath.Join("engine", "hyphenation")
	heads, err := filepath.Glob(filepath.Join(dir, "hyph-*.head.txt"))
	if err != nil {
		return err
	}
	patterns, err := filepath.Glob(filepath.Join(dir, "hyph-*.pat.txt"))
	if err != nil {
		return err
	}
	if len(heads) == 0 || len(heads) != len(patterns) {
		return fmt.Errorf("%s has %d pattern files and %d headers, one for each", dir,
			len(patterns), len(heads))
	}
	for _, path := range heads {
		name := strings.TrimSuffix(filepath.Base(path), ".head.txt")
		if _, err := os.Stat(filepath.Join(dir, name+".pat.txt")); err != nil {
			return fmt.Errorf("%s has a header and no patterns", name)
		}
		head, err := read(path)
		if err != nil {
			return err
		}
		meta, err := readPatternMeta(head)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		chosen := chooseLicence(meta.licences())
		if chosen == nil {
			return fmt.Errorf("%s is not offered under a licence the app may ship. "+
				"Take it out of %s", name, dir)
		}
		licence, text := chosen.spdx, head
		if licence == "" {
			licence = "Own terms, in the text"
		} else if chosen.text == "" {
			full, err := fetch(spdxTexts + licence + ".txt")
			if err != nil {
				return err
			}
			text = append(append(append([]byte{}, head...), "\n\n"...), full...)
		}
		version := strings.TrimSpace(meta.Version.Value)
		if version == "" {
			version = "as shipped"
		}
		url := "https://github.com/hyphenation/tex-hyphen"
		if strings.HasPrefix(meta.Source, "https://") {
			url = meta.Source
		}
		language := meta.Language.Name
		if language == "" {
			return fmt.Errorf("%s does not say what language it is", name)
		}
		if err := add(notice{Name: "Hyphenation patterns, " + language, Version: version,
			Licence: licence, URL: url, Part: partApp,
			Note: "Built into the app. They break a word too long for a caption line."},
			map[string][]byte{name + ".txt": text}); err != nil {
			return err
		}
	}
	return nil
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

// textName is a file's name without an ending that only says it is text.
func textName(file string) string {
	base := filepath.Base(file)
	for _, ext := range []string{".TXT", ".txt", ".md"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}
