package notices

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func all(t *testing.T) []Notice {
	t.Helper()
	list, err := All(Built{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("no notices. Run make notices")
	}
	return list
}

func find(list []Notice, name string) (Notice, bool) {
	for _, n := range list {
		if n.Name == name {
			return n, true
		}
	}
	return Notice{}, false
}

// A notice says what it is, where it is and what its licence is, and its
// texts are there. Every text is some notice's, so nothing is left over
// from a piece of work that has gone.
func TestEveryNoticeIsWhole(t *testing.T) {
	used := map[string]bool{}
	for _, n := range all(t) {
		if n.Name == "" || n.Version == "" || n.Licence == "" || n.URL == "" {
			t.Errorf("%+v is missing something", n)
		}
		if !slices.Contains(Parts, n.Part) {
			t.Errorf("%s is in a part the app does not show: %q", n.Name, n.Part)
		}
		if len(n.Texts) == 0 {
			t.Errorf("%s has no licence text", n.Name)
		}
		for _, name := range n.Texts {
			text, err := Text(Built{}, name)
			if err != nil || strings.TrimSpace(text) == "" {
				t.Errorf("%s: the text %s is missing or empty", n.Name, name)
			}
			used[name] = true
		}
	}
	entries, err := fs.ReadDir(files, "texts")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !used[e.Name()] {
			t.Errorf("texts/%s belongs to no notice", e.Name())
		}
	}
}

// What ffmpeg and llama-server are built from is pinned in their build
// scripts, and the notices are for exactly those versions.
//
// On the real disk: what is read is the repository's own build scripts and go.mod.
func TestTheToolsNoticesAreForTheVersionsBuilt(t *testing.T) {
	list := all(t)
	// The scripts are named by their whole path, so make changed finds
	// this test when either of them changes, see scripts/changed.sh.
	pinned := func(key, script string) string {
		body, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`(?m)^` + key + `=(\S+)`).FindSubmatch(body)
		if m == nil {
			t.Fatalf("no %s in %s", key, script)
		}
		return string(m[1])
	}
	for name, key := range map[string]string{
		"ffmpeg": "FFMPEG_VERSION", "FreeType": "FREETYPE_VERSION", "FriBidi": "FRIBIDI_VERSION",
		"HarfBuzz": "HARFBUZZ_VERSION", "libass": "LIBASS_VERSION",
	} {
		n, ok := find(list, name)
		if want := pinned(key, "../scripts/build-ffmpeg.sh"); !ok || n.Version != want {
			t.Errorf("ffmpeg is built with %s %s, and the notice is for %q", name, want, n.Version)
		}
	}
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`k2-fsa/sherpa-onnx-go v(\S+)`).FindSubmatch(mod); m != nil {
		n, ok := find(list, "sherpa-onnx")
		if !ok || n.Version != string(m[1]) {
			t.Errorf("go.mod pins sherpa-onnx %s, and the notice is for %q", m[1], n.Version)
		}
	}
	n, ok := find(list, "llama.cpp")
	if want := pinned("LLAMA_VERSION", "../scripts/build-llama.sh"); !ok || n.Version != want {
		t.Errorf("llama-server is built from %s, and the notice is for %q", want, n.Version)
	}
}

// The notices the build wrote are shown with the ones kept here, each in
// its part and by name, and their texts are read from what the build
// wrote.
func TestTheNoticesTheBuildWroteAreShownWithTheRest(t *testing.T) {
	built, err := ReadBuilt([]byte(`{"notices": [
		{"name": "zerolog", "version": "1.35.1", "licence": "MIT", "url": "https://x", "part": "The app", "texts": ["zerolog.txt"]},
		{"name": "bits-ui", "version": "2.20.0", "licence": "MIT", "url": "https://y", "part": "The app's interface", "texts": ["npm-bits-ui.txt"]}],
		"texts": {"zerolog.txt": "MIT, zerolog", "npm-bits-ui.txt": "MIT, bits-ui"}}`))
	if err != nil {
		t.Fatal(err)
	}
	list, err := All(built)
	if err != nil {
		t.Fatal(err)
	}
	at := map[string]int{}
	for i, n := range list {
		at[n.Name] = i
	}
	if at["Go"] > at["zerolog"] || at["zerolog"] > at["bits-ui"] || at["bits-ui"] > at["ffmpeg"] {
		t.Errorf("Go, zerolog, bits-ui and ffmpeg stand at %d, %d, %d and %d", at["Go"], at["zerolog"], at["bits-ui"], at["ffmpeg"])
	}
	if text, err := Text(built, "npm-bits-ui.txt"); err != nil || text != "MIT, bits-ui" {
		t.Errorf("the built text reads %q, %v", text, err)
	}
	if text, err := Text(built, "go.txt"); err != nil || !strings.Contains(text, "Go Authors") {
		t.Errorf("the kept text reads %q, %v", text, err)
	}
	if empty, err := ReadBuilt(nil); err != nil || len(empty.Notices) != 0 {
		t.Errorf("a program built without make reads %+v, %v", empty, err)
	}
}

// The Wails runtime the interface talks to is pinned to the version of the
// Wails module, see CLAUDE.md, and its notice is the module's text.
//
// On the real disk: what is read is the repository's own go.mod and
// frontend/package.json.
func TestTheWailsRuntimeIsPinnedWithWails(t *testing.T) {
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := os.ReadFile("../frontend/package.json")
	if err != nil {
		t.Fatal(err)
	}
	module := regexp.MustCompile(`wailsapp/wails/v3 v(\S+)`).FindSubmatch(mod)
	runtime := regexp.MustCompile(`"@wailsio/runtime": "(\S+)"`).FindSubmatch(pkg)
	if module == nil || runtime == nil || string(module[1]) != string(runtime[1]) {
		t.Errorf("go.mod pins Wails %q and package.json the runtime %q", module, runtime)
	}
}

// binary can get its source. The notice says where.
func TestTheFFmpegNoticeSaysWhereTheSourceIs(t *testing.T) {
	n, _ := find(all(t), "ffmpeg")
	if !strings.Contains(n.Note, "framefairy-tools-source.tar.gz") {
		t.Errorf("the ffmpeg notice does not say where its source is: %q", n.Note)
	}
}

// Every hyphenation pattern file the engine carries has its notice, and
// every notice for one has its file, so a language added or taken out
// without make notices fails here.
func TestEveryHyphenationFileHasANotice(t *testing.T) {
	heads, err := filepath.Glob(filepath.Join("..", "engine", "hyphenation", "hyph-*.head.txt"))
	if err != nil || len(heads) == 0 {
		t.Fatalf("no hyphenation patterns found: %v", err)
	}
	want := map[string]bool{}
	for _, head := range heads {
		want[strings.TrimSuffix(filepath.Base(head), ".head.txt")+".txt"] = true
	}
	have := map[string]bool{}
	for _, n := range all(t) {
		if !strings.HasPrefix(n.Name, "Hyphenation patterns, ") {
			continue
		}
		for _, text := range n.Texts {
			have[text] = true
		}
	}
	for text := range want {
		if !have[text] {
			t.Errorf("%s has no notice. Run make notices", text)
		}
	}
	for text := range have {
		if !want[text] {
			t.Errorf("a notice for %s, which the engine does not carry. Run make notices", text)
		}
	}
}
