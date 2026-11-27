package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The code directory and the entitlements are read the way codesign
// really writes them, not only the way these tests write them. CI's macos
// job signs the app with harden and a certificate made for
// the test, then names that bundle SIGNED_APP and the one make signed ad
// hoc ADHOC_APP. Anywhere else there is no codesign, and this is skipped.
//
// On the real disk: what this reads is the bundles codesign signed.
func TestARealSignatureIsReadAsCodesignWroteIt(t *testing.T) {
	signedApp, adhocApp := os.Getenv("SIGNED_APP"), os.Getenv("ADHOC_APP")
	if signedApp == "" || adhocApp == "" {
		t.Skip("needs bundles signed on a Mac, see the macos job in ci.yml")
	}
	read := func(t *testing.T, file string) (signing, []string) {
		t.Helper()
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		arches, err := archesOf(data)
		if err != nil || len(arches) == 0 {
			t.Fatalf("%s is no program: %v", file, err)
		}
		s, err := signingOf(arches[0])
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		keys, err := entitlements(arches[0].f, arches[0].data)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		return s, keys
	}
	programs := func(t *testing.T, app string) []string {
		t.Helper()
		libs, err := filepath.Glob(filepath.Join(app, "Contents", "Frameworks", "*.dylib"))
		if err != nil || len(libs) == 0 {
			t.Fatalf("%s carries no speech library: %v", app, err)
		}
		// The app's own program first, then llama-server and any other.
		own := filepath.Join(app, "Contents", "MacOS", "framefairy-app")
		others, err := filepath.Glob(filepath.Join(app, "Contents", "MacOS", "*"))
		if err != nil {
			t.Fatal(err)
		}
		others = slices.DeleteFunc(others, func(p string) bool { return p == own })
		return slices.Concat([]string{own}, others, libs)
	}
	for _, file := range programs(t, signedApp) {
		s, _ := read(t, file)
		if !s.signed || s.flags&flagRuntime == 0 || s.flags&flagAdhoc != 0 {
			t.Errorf("%s signed by harden reads as signed %v with flags %#x", file, s.signed, s.flags)
		}
		if s.ident != codeIdentifier(file) {
			t.Errorf("%s signed by harden reads as signed as %q, not %s", file, s.ident, codeIdentifier(file))
		}
	}
	s, keys := read(t, programs(t, signedApp)[0])
	if !slices.Contains(keys, "com.apple.security.device.audio-input") {
		t.Errorf("the app signed by harden reads as entitled to %q", keys)
	}
	if s.ident != bundleID {
		t.Errorf("the app signed by harden reads as signed as %q, not %s", s.ident, bundleID)
	}
	for _, file := range programs(t, adhocApp) {
		s, _ := read(t, file)
		if !s.signed || s.flags&flagAdhoc == 0 || s.flags&flagRuntime != 0 || s.team != "" {
			t.Errorf("%s signed by make reads as signed %v with flags %#x and team %q", file, s.signed, s.flags, s.team)
		}
	}
}

// Apple's real certificates between a Developer ID and its root are taken,
// or codesign could not build the chain on main's first real signing.
// CI's macos job sets APPLE_CAS, since only a Mac ships Apple's root, and
// this asks apple.com. Anywhere else it is skipped.
//
// On the real network: apple.com, and the Mac's own roots.
func TestApplesRealAuthorityIsTaken(t *testing.T) {
	if os.Getenv("APPLE_CAS") == "" {
		t.Skip("needs a Mac and apple.com, see the macos job in ci.yml")
	}
	d := devID{macOS: macTools}
	roots, err := d.appleRoots()
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range appleCAs {
		cer, err := d.fetch(url)
		if err != nil {
			t.Fatal(err)
		}
		if err := appleCA(cer, roots); err != nil {
			t.Errorf("%s: %v", url, err)
		}
	}
}
