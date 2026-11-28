package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// The code directory and the entitlements are read the way codesign
// really writes them, not only the way these tests write them. The job
// outside of mac-nightly.yml names the bundle make outside built, signed
// ad hoc, ADHOC_APP. This signs a copy of it with harden and a certificate made
// for the test, since the Developer ID is a secret only main's
// publish.yml reads, and reads both. It proves the signing, and that the
// release tool reads what codesign wrote, not that the app opens: under
// the hardened runtime a certificate that is not Apple's has no team, and
// macOS would load no library of its own. Anywhere else it is skipped.
//
// On the real disk: codesign signs files, and openssl writes the
// certificate it signs with.
func TestARealSignatureIsReadAsCodesignWroteIt(t *testing.T) {
	adhocApp := os.Getenv("ADHOC_APP")
	if adhocApp == "" {
		t.Skip("needs a bundle make built on a Mac, see the job outside in mac-nightly.yml")
	}
	signedApp := testSigned(t, adhocApp)
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
	// And it runs its tools: what it carries is the SHA-256 of each tool
	// as signed, not as make built it, see resum.
	sums, err := toolSums(signedApp)
	if err != nil || len(sums) == 0 {
		t.Fatalf("%s carries no tools: %v", signedApp, err)
	}
	own, err := os.ReadFile(programs(t, signedApp)[0])
	if err != nil {
		t.Fatal(err)
	}
	for name, sum := range sums {
		if !bytes.Contains(own, []byte(name+"="+sum)) {
			t.Errorf("the app signed by harden carries no SHA-256 of %s as signed, so it would not run it", name)
		}
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
// The job outside of mac-nightly.yml sets APPLE_CAS, since only a Mac
// ships Apple's root, and this asks apple.com. Anywhere else it is
// skipped.
//
// On the real network: apple.com, and the Mac's own roots.
func TestApplesRealAuthorityIsTaken(t *testing.T) {
	if os.Getenv("APPLE_CAS") == "" {
		t.Skip("needs a Mac and apple.com, see the job outside in mac-nightly.yml")
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

// A library signed again by real codesign, with Apple's time stamp, is
// the same signed code as when it was signed before, so harden keeps the
// earlier bytes and an update does not download it again. What codesign
// changes between two signings is only its CMS, and it reserves room for
// the signature of the same size both times. The job outside of
// mac-nightly.yml names the bundle, as for the test above.
//
// On the real disk: codesign signs files, and harden reads the earlier
// copies from a folder, as signed-before writes them.
func TestARealLibrarySignedAgainIsKept(t *testing.T) {
	adhocApp := os.Getenv("ADHOC_APP")
	if adhocApp == "" {
		t.Skip("needs a bundle make built on a Mac, see the job outside in mac-nightly.yml")
	}
	d := testSigner(t)
	d.timestamp = true
	copyOf := func() string {
		app := filepath.Join(t.TempDir(), appName)
		if _, err := macTools.run("ditto", adhocApp, app); err != nil {
			t.Fatal(err)
		}
		return app
	}
	first := copyOf()
	if err := d.signApp(first); err != nil {
		t.Fatal(err)
	}
	inner, err := innerCode(first)
	if err != nil || len(inner) == 0 {
		t.Fatalf("%s holds nothing to sign before the app: %v", first, err)
	}
	// The first build's libraries, the way signed-before writes them.
	kept := t.TempDir()
	for _, code := range inner {
		rel, err := filepath.Rel(filepath.Dir(first), code)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(code)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(kept, rel), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(kept, rel, "0-first"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	second := copyOf()
	d.kept = kept
	// signApp verifies the bundle, strict and deep, after the keeping.
	if err := d.signApp(second); err != nil {
		t.Fatal(err)
	}
	for _, code := range inner {
		rel, _ := filepath.Rel(filepath.Dir(first), code)
		before, err := os.ReadFile(code)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(filepath.Dir(second), rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("%s was signed afresh, though its code did not change", rel)
		}
	}
	// And the app carries the SHA-256 of each tool it was kept with, so
	// it runs them, see resum.
	sums, err := toolSums(second)
	if err != nil || len(sums) == 0 {
		t.Fatalf("%s carries no tools: %v", second, err)
	}
	own, err := os.ReadFile(filepath.Join(second, "Contents", "MacOS", appProgram))
	if err != nil {
		t.Fatal(err)
	}
	for name, sum := range sums {
		if !bytes.Contains(own, []byte(name+"="+sum)) {
			t.Errorf("the app signed around kept tools carries no SHA-256 of %s as kept, so it would not run it", name)
		}
	}
}

// testSigned copies the bundle and signs the copy the way publish.yml
// does, with a certificate for code signing made here, which nothing
// trusts and which lives as long as the test.
//
// On the real disk: codesign copies and signs files.
func testSigned(t *testing.T, adhocApp string) string {
	t.Helper()
	signedApp := filepath.Join(t.TempDir(), appName)
	if _, err := macTools.run("ditto", adhocApp, signedApp); err != nil {
		t.Fatal(err)
	}
	d := testSigner(t)
	if err := d.signApp(signedApp); err != nil {
		t.Fatal(err)
	}
	return signedApp
}

// testSigner signs the way publish.yml does, with a certificate for code
// signing made here, which nothing trusts and which lives as long as the
// test, without Apple's time stamp.
//
// On the real disk: openssl writes the certificate it signs with.
func testSigner(t *testing.T) devID {
	t.Helper()
	dir := t.TempDir()
	const name = "Frame Fairy Test Signing"
	conf := filepath.Join(dir, "test.cnf")
	if err := os.WriteFile(conf, []byte("[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n"+
		"[dn]\nCN="+name+"\n[ext]\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=critical,codeSigning\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pass := make([]byte, 16)
	if _, err := rand.Read(pass); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(pass)
	key, crt, p12 := filepath.Join(dir, "test.key"), filepath.Join(dir, "test.crt"), filepath.Join(dir, "test.p12")
	for _, args := range [][]string{
		{"req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-config", conf, "-keyout", key, "-out", crt},
		{"pkcs12", "-export", "-inkey", key, "-in", crt, "-out", p12, "-passout", "pass:" + password},
	} {
		if out, err := exec.Command("/usr/bin/openssl", args...).CombinedOutput(); err != nil {
			t.Fatalf("openssl %s: %v\n%s", args[0], err, out)
		}
	}
	return devID{macOS: macTools, p12: p12, password: password, identity: name,
		entitlements: filepath.Join("..", "..", "build", "entitlements.plist"),
		requirement:  `certificate leaf[subject.CN] = "` + name + `"`}
}
