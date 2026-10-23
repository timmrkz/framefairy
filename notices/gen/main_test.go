package main

import (
	"testing"

	"github.com/spf13/afero"
)

// A pattern file is taken under the licence the app would most want of those
// it is offered under, and a file offered only under a licence a paid app
// cannot ship is refused. The headers are as hyph-utf8 writes them.
func TestAPatternFileIsTakenUnderALicenceTheAppMayShip(t *testing.T) {
	for _, c := range []struct {
		name, head, want string
		refused          bool
	}{
		{"one named licence, written out", "% licence:\n%     name: MIT\n%     text: >\n%         Permission is hereby granted\n", "MIT", false},
		{"a choice, LPPL first", "% licence:\n%     - This file is available under any of these licences:\n%     -\n%         name: LPPL\n%         version: 1.3\n%     -\n%         name: MIT\n", "MIT", false},
		{"LPPL alone", "% licence:\n%     name: LPPL\n%     version: 1\n%     or_later: true\n", "LPPL-1.3c", false},
		{"MPL besides the GPL", "% licence:\n%     - at your option:\n%     -\n%         name: MPL\n%         version: 1.1\n%     -\n%         name: GPL\n%         version: 2\n", "MPL-1.1", false},
		{"own terms nobody has read", "% licence:\n%     text: >\n%         Copying and distribution of this file are permitted.\n", "", true},
		{"the GPL alone", "% licence:\n%     name: GPL\n%     version: 2\n%     or_later: true\n", "", true},
		{"no licence", "% licence:\n%     - text: [None]\n", "", true},
		{"a licence it cannot tell", "% licence:\n%     name: Something Else\n", "", true},
	} {
		meta, err := readPatternMeta([]byte(c.head + "% ====\n% after the metadata: not yaml\n"))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := chooseLicence(meta.licences())
		if c.refused {
			if got != nil {
				t.Errorf("%s: taken under %q", c.name, got.spdx)
			}
			continue
		}
		if got == nil || got.spdx != c.want {
			t.Errorf("%s: taken under %+v, want %q", c.name, got, c.want)
		}
	}
}

// Terms a file writes out itself are taken once they have been read, and
// the same terms with one word changed are refused until they are read
// again.
func TestAFilesOwnTermsCountOnceRead(t *testing.T) {
	read := patternLicence{text: "Copying and distribution of this file, with or without modification, are permitted in any medium without royalty provided the copyright notice and this notice are preserved."}
	if got := chooseLicence([]patternLicence{read}); got == nil {
		t.Error("terms that were read are refused")
	}
	changed := patternLicence{text: "Copying and distribution of this file are permitted."}
	if got := chooseLicence([]patternLicence{changed}); got != nil {
		t.Error("changed terms are taken without being read")
	}
}

// A Go module's notice carries the version and the text of the module the
// build really uses, and one under a licence nobody allowed stops the
// build.
func TestAModuleIsNoticedAsTheBuildHasIt(t *testing.T) {
	fsys := afero.NewMemMapFs()
	write := func(path, body string) {
		if err := afero.WriteFile(fsys, path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("/cache/zerolog@v1.35.1/LICENSE", "Permission is hereby granted, free of charge, to any person")
	write("/cache/sherpa@v1.13.8/LICENSE", "Apache License\n  Version 2.0, January 2004")
	write("/cache/gpl@v1.0.0/COPYING", "GNU GENERAL PUBLIC LICENSE Version 3")

	n, sources, err := moduleNotice(fsys, module{"github.com/rs/zerolog", "v1.35.1", "/cache/zerolog@v1.35.1"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Version != "1.35.1" || n.Licence != "MIT" || n.Part != partApp || len(sources) != 1 {
		t.Errorf("zerolog noticed as %+v with %d texts", n, len(sources))
	}
	n, _, err = moduleNotice(fsys, module{"github.com/k2-fsa/sherpa-onnx-go", "v1.13.8", "/cache/sherpa@v1.13.8"})
	if err != nil || n.Part != partSpeech || n.Licence != "Apache-2.0" {
		t.Errorf("the speech bindings noticed as %+v, %v", n, err)
	}
	if _, _, err := moduleNotice(fsys, module{"example.com/gpl", "v1.0.0", "/cache/gpl@v1.0.0"}); err == nil {
		t.Error("a module under the GPL was let in")
	}
	if _, _, err := moduleNotice(fsys, module{"example.com/gone", "v1.0.0", ""}); err == nil {
		t.Error("a module that is not in the cache was let in")
	}
}

// An npm package is noticed from the folder the bundle took it from, so a
// copy one package carries in its own node_modules is named for itself,
// at its own version. The Wails runtime, published without a licence
// file, has the Wails module's text, and is refused at any other version.
func TestAPackageIsNoticedFromTheFolderTheBundleTookItFrom(t *testing.T) {
	fsys := afero.NewMemMapFs()
	write := func(path, body string) {
		if err := afero.WriteFile(fsys, path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("/f/node_modules/bits-ui/package.json", `{"name": "bits-ui", "version": "2.20.0", "license": "MIT"}`)
	write("/f/node_modules/bits-ui/LICENSE", "MIT, bits-ui")
	write("/f/node_modules/bits-ui/node_modules/@floating-ui/dom/package.json", `{"name": "@floating-ui/dom", "version": "2.0.0", "license": "MIT"}`)
	write("/f/node_modules/bits-ui/node_modules/@floating-ui/dom/LICENSE", "MIT, floating-ui")
	write("/f/node_modules/@wailsio/runtime/package.json", `{"name": "@wailsio/runtime", "version": "3.0.0-beta.23", "license": "MIT"}`)
	write("/cache/wails/LICENSE", "MIT, Wails")
	write("/f/node_modules/copyleft/package.json", `{"name": "copyleft", "version": "1.0.0", "license": "GPL-3.0-only"}`)
	write("/f/node_modules/copyleft/LICENSE", "GPL")

	wails := module{Path: wailsModule, Version: "v3.0.0-beta.23", Dir: "/cache/wails"}
	n, sources, err := packageNotice(fsys, "/f", "node_modules/bits-ui/node_modules/@floating-ui/dom", wails)
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "@floating-ui/dom" || n.Version != "2.0.0" || n.Part != partScreen || string(sources["npm-floating-ui-dom.txt"]) != "MIT, floating-ui" {
		t.Errorf("the nested copy noticed as %+v with %q", n, sources)
	}
	n, sources, err = packageNotice(fsys, "/f", "node_modules/@wailsio/runtime", wails)
	if err != nil || string(sources["npm-wailsio-runtime.txt"]) != "MIT, Wails" {
		t.Errorf("the Wails runtime noticed as %+v with %q, %v", n, sources, err)
	}
	wails.Version = "v3.0.0-beta.24"
	if _, _, err := packageNotice(fsys, "/f", "node_modules/@wailsio/runtime", wails); err == nil {
		t.Error("a Wails runtime of another version than Wails was let in")
	}
	if _, _, err := packageNotice(fsys, "/f", "node_modules/copyleft", wails); err == nil {
		t.Error("a package under the GPL was let in")
	}
}

// Every module the programs compile, on each system they ship on, is under
// a licence the app may ship, with its text where the build reads it. The
// build checks only the system it builds for, so this is where Windows is
// checked.
//
// On the real disk: what is read is the Go module cache, where go keeps
// the modules the programs compile.
func TestEveryModuleTheProgramsCompileMayShip(t *testing.T) {
	t.Parallel()
	for _, goos := range []string{"darwin", "linux", "windows"} {
		modules, err := compiled("../..", goos)
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		for _, m := range modules {
			if _, _, err := moduleNotice(afero.NewOsFs(), m); err != nil {
				t.Errorf("%s: %v", goos, err)
			}
		}
	}
}
