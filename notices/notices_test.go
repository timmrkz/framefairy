package notices

import (
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func all(t *testing.T) []Notice {
	t.Helper()
	list, err := All()
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
			text, err := Text(name)
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

// Every Go module the shipped programs compile on this system has its
// notice, at the version they compile. CI runs this on Linux and macOS, so
// both of those are checked on every change.
func TestEveryGoModuleCompiledInHasANotice(t *testing.T) {
	list := all(t)
	cmd := exec.Command("go", "list", "-deps", "-f",
		"{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}",
		"./cmd/framefairy-app", "./cmd/framefairy")
	cmd.Dir = ".."
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		n, ok := find(list, fields[0])
		switch {
		case !ok:
			t.Errorf("%s is compiled into the app on %s and has no notice. Run make notices",
				fields[0], runtime.GOOS)
		case n.Version != strings.TrimPrefix(fields[1], "v"):
			t.Errorf("%s is compiled in at %s and its notice is for %s. Run make notices",
				fields[0], fields[1], n.Version)
		}
	}
	if len(seen) == 0 {
		t.Fatal("go list found no modules at all")
	}
}

// What ffmpeg and llama-server are built from is pinned in their build
// scripts, and the notices are for exactly those versions.
func TestTheToolsNoticesAreForTheVersionsBuilt(t *testing.T) {
	list := all(t)
	pinned := func(key, script string) string {
		body, err := os.ReadFile("../scripts/" + script)
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
		if want := pinned(key, "build-ffmpeg.sh"); !ok || n.Version != want {
			t.Errorf("ffmpeg is built with %s %s, and the notice is for %q", name, want, n.Version)
		}
	}
	n, ok := find(list, "llama.cpp")
	if want := pinned("LLAMA_VERSION", "build-llama.sh"); !ok || n.Version != want {
		t.Errorf("llama-server is built from %s, and the notice is for %q", want, n.Version)
	}
}

// The interface's own packages are checked where they are bundled, by the
// build of the interface. The one this side can check is the Wails runtime,
// which is pinned to the same version as the Wails module.
func TestTheWailsRuntimeNoticeMatchesWails(t *testing.T) {
	list := all(t)
	module, _ := find(list, "github.com/wailsapp/wails/v3")
	runtime, ok := find(list, "@wailsio/runtime")
	if !ok || runtime.Version != module.Version {
		t.Errorf("@wailsio/runtime is noticed at %q and Wails at %q", runtime.Version, module.Version)
	}
}

// The ffmpeg we ship is LGPL, and the LGPL asks that anyone who has the
// binary can get its source. The notice says where.
func TestTheFFmpegNoticeSaysWhereTheSourceIs(t *testing.T) {
	n, _ := find(all(t), "ffmpeg")
	if !strings.Contains(n.Note, "framefairy-tools-source.tar.gz") {
		t.Errorf("the ffmpeg notice does not say where its source is: %q", n.Note)
	}
}
