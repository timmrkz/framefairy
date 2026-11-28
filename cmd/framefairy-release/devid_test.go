package main

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"framefairy/internal/standin"
)

// The signing with the Developer ID runs the Mac's own tools, which no
// test here has. These hand in tools of their own: security answers and
// does nothing, and codesign writes into each file it signs a program
// signed the way the test says, so what harden reads back is what such a
// codesign would have left.

// fakeMac records every command and signs the way it is told.
type fakeMac struct {
	commands [][]string
	// flags and microphone are how codesign signs.
	flags      uint32
	microphone bool
	// as, when set, is what codesign signs the app as, whatever it is
	// told, and inner what it signs llama-server as, the way an
	// Info.plist built into it would have codesign do.
	as, inner string
	// failOn makes the command whose last argument ends with it fail,
	// and refuse makes codesign find code not of the requirement.
	failOn string
	refuse bool
	// fetched is what apple.com sends, and roots what macOS ships as
	// Apple Root CA, in PEM.
	fetched, roots []byte
	// certs, when set, are the certificates codesign signs with, which
	// puts a CMS blob in each signature, with a time that is new on each
	// signing, as Apple's codesign does. stamps counts the signings.
	certs  [][]byte
	stamps int
}

// On the real disk: harden hands codesign and ditto the paths of files,
// so the stand-ins for them write the files where harden will read them.
func (m *fakeMac) tools() macOS {
	return macOS{
		run: func(name string, args ...string) ([]byte, error) {
			m.commands = append(m.commands, append([]string{name}, args...))
			if m.failOn != "" && strings.HasSuffix(args[len(args)-1], m.failOn) {
				return nil, errors.New(name + " failed")
			}
			if name == "codesign" && m.refuse && slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "-R=") }) {
				return nil, errors.New("codesign: test-requirement: code failed to satisfy specified code requirement(s)")
			}
			// ditto packs the app again, here as an empty file.
			if name == "ditto" && args[0] == "-c" {
				return nil, os.WriteFile(args[len(args)-1], nil, 0o644)
			}
			if name == "security" && args[0] == "find-certificate" {
				return m.roots, nil
			}
			if name == "security" && args[0] == "list-keychains" && len(args) == 3 {
				return []byte("    \"/Users/runner/Library/Keychains/login.keychain-db\"\n    \"/Library/Keychains/System.keychain\"\n"), nil
			}
			if name == "codesign" && slices.Contains(args, "-s") {
				target := args[len(args)-1]
				p := speech
				// Told nothing, codesign signs a file as its name.
				p.ident = strings.TrimSuffix(filepath.Base(target), ".dylib")
				if i := slices.Index(args, "--identifier"); i >= 0 {
					p.ident = args[i+1]
				}
				if m.inner != "" && filepath.Base(target) == "llama-server" {
					p.ident = m.inner
				}
				if strings.HasSuffix(target, ".app") {
					p = app
					if m.microphone {
						p.entitled = plistHead + "<key>com.apple.security.device.audio-input</key><true/>\n</dict>\n</plist>\n"
					}
					target = filepath.Join(target, "Contents", "MacOS", appProgram)
					// codesign signs the bundle as its Info.plist says,
					// unless it is told what as.
					p.ident = "com.other.app"
					if i := slices.Index(args, "--identifier"); i >= 0 {
						p.ident = args[i+1]
					}
					if m.as != "" {
						p.ident = m.as
					}
				}
				p.signed, p.flags, p.team = true, m.flags, "TEAM123456"
				if m.certs != nil {
					m.stamps++
					p.cms = testCMS(m.certs, m.stamps, true)
				}
				return nil, standin.Write(target, p.bytes(), 0o755)
			}
			return nil, nil
		},
		fetch: func(url string) ([]byte, error) { return m.fetched, nil },
	}
}

// signedBy are the targets of codesign's signing commands, in order.
func (m *fakeMac) signed() [][]string {
	var out [][]string
	for _, c := range m.commands {
		if c[0] == "codesign" && slices.Contains(c, "-s") {
			out = append(out, c)
		}
	}
	return out
}

// bundle lays out the app as make builds it: the program, llama-server
// and the speech libraries, signed ad hoc.
//
// On the real disk: harden signs a bundle in a folder, because codesign
// signs files, and reads them back from there.
func bundle(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), appName)
	files := map[string]program{
		"Contents/MacOS/" + appProgram:                     asMade(adhoc),
		"Contents/MacOS/llama-server":                      asMade(speech),
		"Contents/Frameworks/libonnxruntime.dylib":         asMade(speech),
		"Contents/Frameworks/libsherpa-onnx-c-api.dylib":   asMade(speech),
		"Contents/Frameworks/libsherpa-onnx-cxx-api.dylib": asMade(speech),
	}
	for name, p := range files {
		file := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := standin.Write(file, p.bytes(), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

func signer(m *fakeMac) devID {
	return devID{macOS: m.tools(), p12: "developer-id.p12", password: "the password",
		identity: "Developer ID Application", entitlements: "build/entitlements.plist", timestamp: true}
}

// Every library and llama-server are signed before the app, each with
// the hardened runtime, Apple's time stamp and the Developer ID from the
// keychain made for it, and only the app with the entitlements. The
// keychain is gone at the end, and the Mac's own list of keychains is as
// it was.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestTheAppIsSignedInsideOutInAKeychainOfItsOwn(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true}
	app := bundle(t)
	if err := signer(m).signApp(app); err != nil {
		t.Fatal(err)
	}
	var targets []string
	signed := m.signed()
	for i, c := range signed {
		targets = append(targets, strings.TrimPrefix(c[len(c)-1], app))
		for _, want := range []string{"--options", "runtime", "--timestamp", "--keychain", "Developer ID Application"} {
			if !slices.Contains(c, want) {
				t.Errorf("%s was signed without %s: %q", c[len(c)-1], want, c)
			}
		}
		last := i == len(signed)-1
		if slices.Contains(c, "--entitlements") != last {
			t.Errorf("%s: entitlements %v", c[len(c)-1], slices.Contains(c, "--entitlements"))
		}
		if at := slices.Index(c, "--identifier"); last && (at < 0 || c[at+1] != bundleID) {
			t.Errorf("the app was not signed as %s: %q", bundleID, c)
		}
		// Each part of the app as a part of it, never as what its name
		// or an Info.plist built into it says.
		if at := slices.Index(c, "--identifier"); !last && (at < 0 || c[at+1] != codeIdentifier(c[len(c)-1])) {
			t.Errorf("%s was not signed as %s: %q", c[len(c)-1], codeIdentifier(c[len(c)-1]), c)
		}
	}
	want := []string{
		"/Contents/Frameworks/libonnxruntime.dylib",
		"/Contents/Frameworks/libsherpa-onnx-c-api.dylib",
		"/Contents/Frameworks/libsherpa-onnx-cxx-api.dylib",
		"/Contents/MacOS/llama-server",
		"",
	}
	if !slices.Equal(targets, want) {
		t.Errorf("signed %q, want %q", targets, want)
	}
	for _, c := range m.commands {
		if c[0] == "security" && c[1] == "import" && slices.Contains(c, "pkcs12") && !slices.Contains(c, "-x") {
			t.Errorf("the key was imported so it can be exported again: %q", c)
		}
	}
	last := m.commands[len(m.commands)-2:]
	if !slices.Equal(last[0], []string{"security", "list-keychains", "-d", "user", "-s",
		"/Users/runner/Library/Keychains/login.keychain-db", "/Library/Keychains/System.keychain"}) {
		t.Errorf("the list of keychains was left as %q", last[0])
	}
	if last[1][1] != "delete-keychain" {
		t.Errorf("the last command was %q, not the keychain removed", last[1])
	}
}

// A signing that fails half way leaves no keychain behind with the
// certificate in it, and the Mac's list as it was.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestAFailedSigningLeavesNoKeychainBehind(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true, failOn: "llama-server"}
	err := signer(m).signApp(bundle(t))
	if err == nil {
		t.Fatal("a signing that failed said nothing")
	}
	last := m.commands[len(m.commands)-2:]
	if last[0][1] != "list-keychains" || last[1][1] != "delete-keychain" {
		t.Errorf("after the failure it ran %q", last)
	}
}

// What codesign left is read back as the release check reads it, so an
// app it signed without the hardened runtime, ad hoc, as another app or
// without the microphone is refused here, before it is packed.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestAnAppSignedWrongIsRefused(t *testing.T) {
	cases := []struct {
		name       string
		flags      uint32
		microphone bool
		as, inner  string
		want       string
	}{
		{"without the hardened runtime", 0, true, "", "", "llama-server is not signed with the hardened runtime"},
		{"ad hoc", flagRuntime | flagAdhoc, true, "", "", "libonnxruntime.dylib is not signed with the hardened runtime"},
		{"without the microphone", flagRuntime, false, "", "", "signed without the microphone"},
		{"as another app", flagRuntime, true, "com.other.app", "", `framefairy-app is signed as "com.other.app", not com.framefairy.app`},
		{"llama-server as another app", flagRuntime, true, "", "com.other.app", `llama-server is signed as "com.other.app", not com.framefairy.app.llama-server`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := signer(&fakeMac{flags: c.flags, microphone: c.microphone, as: c.as, inner: c.inner}).signApp(bundle(t))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("signing it said %v, want %q", err, c.want)
			}
		})
	}
}

// Until build/team-id.txt names the user's team, a build stays signed ad hoc
// and the run says so. Once it does, a build without the certificate
// fails, never quietly.
//
// On the real disk: the team's file is a file.
func TestHardenGoesByTheTeamFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FRAMEFAIRY_DEVELOPER_ID", "")
	if err := harden([]string{"-zip", filepath.Join(dir, "app.zip"), "-team-file", filepath.Join(dir, "none.txt")}); err != nil {
		t.Errorf("with no team it said %v", err)
	}
	team := filepath.Join(dir, "team-id.txt")
	if err := os.WriteFile(team, []byte("TEAM123456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := harden([]string{"-zip", filepath.Join(dir, "app.zip"), "-team-file", team})
	if err == nil || !strings.Contains(err.Error(), "FRAMEFAIRY_DEVELOPER_ID is not set") {
		t.Errorf("with a team and no certificate it said %v", err)
	}
}

// What comes back from apple.com goes into the keychain the Developer ID
// signs from only when it is Apple's Developer ID authority, signed by
// the Apple Root CA macOS ships. Something else, or one made to look like
// it, is left out.
//
// On the real disk: harden writes what it imports to a file.
func TestOnlyApplesAuthorityGoesInTheKeychain(t *testing.T) {
	root, rootKey := certificate(t, "Apple Root CA", nil, nil)
	genuine, _ := certificate(t, "Developer ID Certification Authority", root, rootKey)
	lookalike, _ := certificate(t, "Developer ID Certification Authority", nil, nil)
	otherRoot, otherKey := certificate(t, "Apple Root CA", nil, nil)
	signedByOther, _ := certificate(t, "Developer ID Certification Authority", otherRoot, otherKey)
	cases := []struct {
		name    string
		fetched []byte
		taken   bool
	}{
		{"Apple's authority", genuine.Raw, true},
		{"no certificate", []byte("a certificate"), false},
		{"a lookalike signed by itself", lookalike.Raw, false},
		{"a lookalike signed by another Apple Root CA", signedByOther.Raw, false},
		{"Apple's root itself", root.Raw, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &fakeMac{flags: flagRuntime, microphone: true, fetched: c.fetched,
				roots: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})}
			if err := signer(m).signApp(bundle(t)); err != nil {
				t.Fatal(err)
			}
			imported := 0
			for _, cmd := range m.commands {
				if cmd[0] == "security" && cmd[1] == "import" && strings.HasSuffix(cmd[2], ".cer") {
					imported++
				}
			}
			if want := map[bool]int{true: len(appleCAs), false: 0}[c.taken]; imported != want {
				t.Errorf("imported %d of what apple.com sent, want %d", imported, want)
			}
		})
	}
}

// certificate is a certificate authority named name, signed by parent, or
// by itself when parent is nil.
func certificate(t *testing.T, name string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c, key
}

// harden runs only the Mac's own tools, by their full path, and the
// programs it starts never inherit the secrets.
//
// On the real disk: the team file and the zip are files harden is
// handed by their paths, as the workflow hands them.
func TestHardenKeepsItsSecretsAndToolsToItself(t *testing.T) {
	if _, err := macTools.run("sh", "-c", "true"); err == nil || !strings.Contains(err.Error(), "is no tool harden runs") {
		t.Errorf("running sh said %v", err)
	}
	dir := t.TempDir()
	t.Setenv("FRAMEFAIRY_DEVELOPER_ID", "c2VjcmV0")
	t.Setenv("FRAMEFAIRY_DEVELOPER_ID_PASSWORD", "the password")
	_ = harden([]string{"-zip", filepath.Join(dir, "app.zip"), "-team-file", filepath.Join(dir, "none.txt")})
	for _, k := range []string{"FRAMEFAIRY_DEVELOPER_ID", "FRAMEFAIRY_DEVELOPER_ID_PASSWORD"} {
		if _, set := os.LookupEnv(k); set {
			t.Errorf("%s is still in the environment the tools are started with", k)
		}
	}
}

// Once everything is signed, codesign checks each program and the app
// against the requirement harden is given, which names the user's team in
// Apple's certificate itself, not only the string a code directory
// carries.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestEverythingSignedIsCheckedAgainstTheTeam(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true}
	d := signer(m)
	d.requirement = `anchor apple generic and certificate leaf[subject.OU] = "TEAM123456"`
	app := bundle(t)
	if err := d.signApp(app); err != nil {
		t.Fatal(err)
	}
	var checked []string
	for _, c := range m.commands {
		if c[0] == "codesign" && slices.Contains(c, "--verify") && slices.Contains(c, "-R="+d.requirement) {
			checked = append(checked, strings.TrimPrefix(c[len(c)-1], app))
		}
	}
	want := []string{
		"/Contents/Frameworks/libonnxruntime.dylib",
		"/Contents/Frameworks/libsherpa-onnx-c-api.dylib",
		"/Contents/Frameworks/libsherpa-onnx-cxx-api.dylib",
		"/Contents/MacOS/llama-server",
		"/Contents/MacOS/" + appProgram,
		"",
	}
	if !slices.Equal(checked, want) {
		t.Errorf("checked %q against the team, want %q", checked, want)
	}
	m.refuse = true
	if err := d.signApp(bundle(t)); err == nil || !strings.Contains(err.Error(), "failed to satisfy") {
		t.Errorf("code not of the team said %v", err)
	}
}

// In publish.yml, harden reads the team from build/team-id.txt and has
// codesign check everything against it. A file that names no team ID as
// Apple gives them stops it before the certificate is touched.
//
// On the real disk: the build's zip, the team's file and the workflow's
// output are files.
func TestHardenChecksTheBuildAgainstTheTeamInTheFile(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "app.zip")
	files := map[string][]byte{}
	walked := bundle(t)
	_ = filepath.WalkDir(walked, func(file string, e os.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() {
			rel, _ := filepath.Rel(filepath.Dir(walked), file)
			files[filepath.ToSlash(rel)], _ = os.ReadFile(file)
		}
		return err
	})
	if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	team := filepath.Join(dir, "team-id.txt")
	for _, c := range []struct{ team, want string }{
		{"TEAM123456\n", ""},
		{"team123456\n", `"team123456" is no team ID`},
		{`T" or true` + "\n", `is no team ID`},
	} {
		if err := os.WriteFile(team, []byte(c.team), 0o644); err != nil {
			t.Fatal(err)
		}
		m := &fakeMac{flags: flagRuntime, microphone: true}
		t.Setenv("FRAMEFAIRY_DEVELOPER_ID", "c2VjcmV0")
		err := hardenWith(m.tools(), []string{"-zip", zipPath, "-team-file", team})
		if c.want != "" {
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("team %q said %v, want %q", c.team, err, c.want)
			}
			if len(m.commands) > 0 {
				t.Errorf("team %q still ran %q", c.team, m.commands[0])
			}
			continue
		}
		if err != nil {
			t.Fatalf("team %q said %v", c.team, err)
		}
		want := "-R=" + `anchor apple generic and certificate leaf[subject.OU] = "TEAM123456"`
		checked := 0
		for _, cmd := range m.commands {
			if cmd[0] == "codesign" && slices.Contains(cmd, want) {
				checked++
			}
		}
		if checked != 6 {
			t.Errorf("checked %d programs and the app against the user's team, want 6: %q", checked, m.commands)
		}
	}
}

// The team goes into codesign's requirement as it is written, so only
// a team ID Apple could have given passes.
func TestATeamThatIsNoTeamIsRefused(t *testing.T) {
	for _, team := range []string{"TEAM12345", "team123456", `T" or true`, "TEAM1234567"} {
		if _, err := teamRequirement(team); err == nil {
			t.Errorf("%q was taken as a team", team)
		}
	}
	if r, err := teamRequirement("C4XN6QQPS9"); err != nil || r != `anchor apple generic and certificate leaf[subject.OU] = "C4XN6QQPS9"` {
		t.Errorf("the user's team gave %q, %v", r, err)
	}
}

// A program outside Contents/MacOS and the libraries of
// Contents/Frameworks is not signed by harden, so it is refused before
// the build is packed, whatever signature it came with.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestAProgramLeftUnsignedInTheAppIsRefused(t *testing.T) {
	app := bundle(t)
	extra := filepath.Join(app, "Contents", "Resources", "helper")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := standin.Write(extra, devid(speech, "TEAM123456").bytes(), 0o755); err != nil {
		t.Fatal(err)
	}
	err := signer(&fakeMac{flags: flagRuntime, microphone: true}).signApp(app)
	if err == nil || !strings.Contains(err.Error(), "helper is a program outside what harden signs") {
		t.Errorf("signing it said %v", err)
	}
}

// harden unpacks a branch's build on the Mac that holds the Developer ID
// and the update key, and check refuses neither a name that leads out of
// the folder nor a link. So unpack is the one guard: every entry that is
// not a file or folder inside the .app is refused, and nothing lands
// outside the folder it unpacks to.
//
// On the real disk: unpack writes what it unpacks, and what is checked is
// that nothing landed beside its folder.
func TestABuildIsUnpackedOnlyInsideItsApp(t *testing.T) {
	cases := []struct {
		name string
		mode os.FileMode
		want string
	}{
		{appName + "/../x", 0o644, "lies outside"},
		{"/x", 0o644, "lies outside"},
		{"x", 0o644, "lies outside"},
		{appName + "/Contents/../../x", 0o644, "lies outside"},
		{appName + `\..\x`, 0o644, "lies outside"},
		{"Other.app/Contents/MacOS/x", 0o644, "lies outside"},
		{appName + "/Contents/link", os.ModeSymlink | 0o777, "is no file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			good, err := w.Create(appName + "/Contents/Info.plist")
			if err == nil {
				_, err = good.Write([]byte("plist"))
			}
			if err != nil {
				t.Fatal(err)
			}
			h := &zip.FileHeader{Name: c.name, Method: zip.Store}
			h.SetMode(c.mode)
			bad, err := w.CreateHeader(h)
			if err == nil {
				_, err = bad.Write([]byte("../../escaped"))
			}
			if err == nil {
				err = w.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(dir, "app.zip")
			if err := os.WriteFile(zipPath, b.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			err = unpack(zipPath, filepath.Join(dir, "out", "unpacked"))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("unpacking %q said %v, want %q", c.name, err, c.want)
			}
			for _, where := range []string{dir, filepath.Join(dir, "out")} {
				entries, err := os.ReadDir(where)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range entries {
					if e.Name() != "app.zip" && e.Name() != "out" && e.Name() != "unpacked" {
						t.Errorf("unpacking %q wrote %s beside its folder", c.name, filepath.Join(where, e.Name()))
					}
				}
			}
		})
	}
}
