package main

// Every build the user updates to is signed again with their Developer ID
// and Apple's hardened runtime, by harden, which publish.yml runs on a Mac
// before it packs and signs the build for updates. See docs/UPDATES.md,
// The Developer ID.
//
// The hardened runtime keeps other programs from putting their own code
// into the app: macOS ignores the DYLD_ settings it is started with,
// refuses debuggers, and loads only libraries signed by Apple or by the
// same developer as the app. So every library and every program beside the
// app is signed with the same Developer ID first, and the app around them
// last. Inside out, as Apple asks, and as docs/PACKAGING.md plans it.
//
// Builds made on the Mac with make stay signed ad hoc and without the
// hardened runtime, the way Xcode's Sign to Run Locally does, because under
// it an app signed ad hoc may load no library but Apple's, and the speech
// libraries are not Apple's.

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// appName is the bundle in a build's zip, and appProgram its own program.
const (
	appName    = "Frame Fairy.app"
	appProgram = "framefairy-app"
)

// appleCAs are Apple's certificates between a Developer ID and Apple's
// root, which a certificate exported from Keychain Access does not carry,
// and codesign needs the whole chain. They are public, and each is taken
// only when the Mac finds it issued by Apple's root, see appleCA.
var appleCAs = []string{
	"https://www.apple.com/certificateauthority/DeveloperIDG2CA.cer",
	"https://www.apple.com/certificateauthority/DeveloperIDCA.cer",
}

// macOS runs the Mac's own tools, security, codesign and ditto, and
// fetches Apple's certificates. Tests hand in their own.
type macOS struct {
	// run runs a program and gives back what it wrote, both streams.
	run func(name string, args ...string) ([]byte, error)
	// fetch downloads a file.
	fetch func(url string) ([]byte, error)
}

// macTools are the real ones. Each tool is the Mac's own, by its full
// path, never one found through PATH, which a build's files must never be
// able to put a program of their own ahead of.
var macTools = macOS{
	run: func(name string, args ...string) ([]byte, error) {
		if !slices.Contains([]string{"security", "codesign", "ditto"}, name) {
			return nil, fmt.Errorf("%s is no tool harden runs", name)
		}
		out, err := exec.Command("/usr/bin/"+name, args...).CombinedOutput()
		if err != nil {
			return out, fmt.Errorf("%s %s: %w\n%s", name, args[0], err, strings.TrimSpace(string(out)))
		}
		return out, nil
	},
	fetch: func(url string) ([]byte, error) {
		c := http.Client{Timeout: 30 * time.Second}
		r, err := c.Get(url)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", url, r.Status)
		}
		return io.ReadAll(io.LimitReader(r.Body, 1<<20))
	},
}

// devID says how to sign: with which certificate, under which name, and
// whether with Apple's time stamp, which a real build has, so its
// signature stays good after the certificate runs out. The test in CI
// signs with a certificate made for it and no time stamp.
type devID struct {
	macOS
	p12, password, identity string
	entitlements            string
	timestamp               bool
	// requirement, when set, is what codesign checks every program and
	// the app against once they are signed: the user's team in Apple's own
	// certificate, see teamRequirement.
	requirement string
}

// harden is the command.
//
//	harden -zip F -team-file F
//	    in publish.yml: sign the app in a build's zip with the Developer ID
//	    from FRAMEFAIRY_DEVELOPER_ID, base64, and its password from
//	    FRAMEFAIRY_DEVELOPER_ID_PASSWORD, and write the team to
//	    GITHUB_OUTPUT. With no team in the file, it says so and signs
//	    nothing. With a team and no certificate, it fails.
//	harden -app A -p12 F -identity N -timestamp=false
//	    in CI: sign a bundle with a certificate made for the test.
func harden(args []string) error {
	return hardenWith(macTools, args)
}

// hardenWith is harden with the Mac's tools handed in, which tests
// stand in for.
func hardenWith(tools macOS, args []string) (err error) {
	fs := flag.NewFlagSet("harden", flag.ExitOnError)
	refusal := fs.String("refusal", "", "where to keep why the build was not signed, for not-published")
	zipPath := fs.String("zip", "", "the build's zip, signed in place")
	teamFile := fs.String("team-file", "build/team-id.txt", "the file that names the user's team")
	app := fs.String("app", "", "a bundle to sign, in place of a zip")
	p12 := fs.String("p12", "", "the certificate with its key, for -app")
	identity := fs.String("identity", "Developer ID Application", "the certificate's name, or the start of it")
	entitlements := fs.String("entitlements", "build/entitlements.plist", "what the app is signed as entitled to")
	timestamp := fs.Bool("timestamp", true, "with Apple's time stamp")
	requirement := fs.String("requirement", "", "what codesign checks the signed code against, for -app")
	_ = fs.Parse(args)
	defer func() { err = refused(*refusal, err, os.Stderr) }()
	// The secrets are read once and leave the environment at once, so no
	// program harden starts inherits them.
	cert := os.Getenv("FRAMEFAIRY_DEVELOPER_ID")
	d := devID{macOS: tools, p12: *p12, password: os.Getenv("FRAMEFAIRY_DEVELOPER_ID_PASSWORD"),
		identity: *identity, entitlements: *entitlements, timestamp: *timestamp, requirement: *requirement}
	for _, k := range []string{"FRAMEFAIRY_DEVELOPER_ID", "FRAMEFAIRY_DEVELOPER_ID_PASSWORD"} {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	if *app != "" {
		return d.signApp(*app)
	}
	team, err := teamOf(*teamFile)
	if err != nil {
		return err
	}
	if team == "" {
		fmt.Printf("::notice::No team in %s, so the build stays signed ad hoc, without the hardened runtime. See docs/UPDATES.md, The Developer ID.\n", *teamFile)
		return nil
	}
	if d.requirement, err = teamRequirement(team); err != nil {
		return fmt.Errorf("%s: %w", *teamFile, err)
	}
	if cert == "" {
		return fmt.Errorf("%s names team %s, but FRAMEFAIRY_DEVELOPER_ID is not set. See docs/UPDATES.md, The Developer ID", *teamFile, team)
	}
	return d.signZip(*zipPath, cert)
}

// teamRequirement is what codesign checks code signed with the Developer
// ID against: signed through Apple's own authority, by a certificate
// Apple gave to the team. That is read from the certificate, which
// nobody but Apple can make, not from the team a code directory names,
// which is only a string. The team goes into it as written, so only a
// team ID as Apple gives them is taken.
func teamRequirement(team string) (string, error) {
	if len(team) != 10 || strings.Trim(team, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
		return "", fmt.Errorf("%q is no team ID, which is ten capital letters and digits", team)
	}
	return `anchor apple generic and certificate leaf[subject.OU] = "` + team + `"`, nil
}

// teamOf is the team the file names, or none when there is no file.
func teamOf(file string) (string, error) {
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// signZip unpacks the build, signs the app in it and packs it again in its
// place, the way builds.yml packed it: one entry at the top, the .app, and
// no ._ files beside the files.
func (d devID) signZip(zipPath, cert string) error {
	dir, err := os.MkdirTemp("", "developer-id")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	p12, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cert))
	if err != nil {
		return fmt.Errorf("FRAMEFAIRY_DEVELOPER_ID is not base64: %w", err)
	}
	d.p12 = filepath.Join(dir, "developer-id.p12")
	if err := os.WriteFile(d.p12, p12, 0o600); err != nil {
		return err
	}
	// Unpacked by the same reader check read it with, and only what lies
	// inside the .app, so what is signed is what was checked.
	unpacked := filepath.Join(dir, "unpacked")
	if err := unpack(zipPath, unpacked); err != nil {
		return err
	}
	app := filepath.Join(unpacked, appName)
	if err := d.signApp(app); err != nil {
		return err
	}
	packed := filepath.Join(dir, "signed.zip")
	if _, err := d.run("ditto", "-c", "-k", "--norsrc", "--keepParent", app, packed); err != nil {
		return err
	}
	b, err := os.ReadFile(packed)
	if err != nil {
		return err
	}
	return os.WriteFile(zipPath, b, 0o644)
}

// signApp signs the bundle in a keychain of its own, made here and gone
// when it returns, whatever happens, the way GitHub's own guide to signing
// on its Macs does it, and reads every program back.
func (d devID) signApp(app string) (err error) {
	if _, err := os.Stat(filepath.Join(app, "Contents", "MacOS", appProgram)); err != nil {
		return fmt.Errorf("%s holds no %s: %w", app, appProgram, err)
	}
	dir, err := os.MkdirTemp("", "keychain")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	keychain := filepath.Join(dir, "signing.keychain-db")
	pass := make([]byte, 24)
	if _, err := rand.Read(pass); err != nil {
		return err
	}
	password := hex.EncodeToString(pass)
	listed, err := d.run("security", "list-keychains", "-d", "user")
	if err != nil {
		return err
	}
	before := keychains(listed)
	if _, err := d.run("security", "create-keychain", "-p", password, keychain); err != nil {
		return err
	}
	defer func() {
		_, _ = d.run("security", append([]string{"list-keychains", "-d", "user", "-s"}, before...)...)
		_, _ = d.run("security", "delete-keychain", keychain)
	}()
	// The keychain's password and the certificate's go on security's
	// command line, which takes them no other way, as GitHub's own guide
	// to signing on its Macs does. Only processes of this job can read a
	// command line on its runner, the runner is made for the job and gone
	// after it, and the keychain's password is made here for the run.
	steps := [][]string{
		{"set-keychain-settings", "-lut", "1800", keychain},
		{"unlock-keychain", "-p", password, keychain},
		// -x: the key can be used to sign but never exported again.
		{"import", d.p12, "-P", d.password, "-f", "pkcs12", "-x", "-k", keychain, "-T", "/usr/bin/codesign"},
	}
	for _, s := range steps {
		if _, err := d.run("security", s...); err != nil {
			return err
		}
	}
	roots, err := d.appleRoots()
	if err != nil {
		fmt.Printf("::warning::Apple's root: %v\n", err)
	}
	for i, url := range appleCAs {
		cer, err := d.fetch(url)
		if err == nil {
			err = appleCA(cer, roots)
		}
		if err != nil {
			fmt.Printf("::warning::%s: %v\n", url, err)
			continue
		}
		file := filepath.Join(dir, fmt.Sprintf("ca%d.cer", i))
		if err := os.WriteFile(file, cer, 0o644); err == nil {
			// One already in the keychain says so, and that is no failure.
			_, _ = d.run("security", "import", file, "-k", keychain)
		}
	}
	if _, err := d.run("security", "set-key-partition-list", "-S", "apple-tool:,apple:", "-k", password, keychain); err != nil {
		return err
	}
	if _, err := d.run("security", append([]string{"list-keychains", "-d", "user", "-s", keychain}, before...)...); err != nil {
		return err
	}

	inner, err := innerCode(app)
	if err != nil {
		return err
	}
	stamp := "--timestamp"
	if !d.timestamp {
		stamp = "--timestamp=none"
	}
	sign := func(extra ...string) error {
		_, err := d.run("codesign", append([]string{"--force", "--options", "runtime", stamp, "--keychain", keychain, "-s", d.identity}, extra...)...)
		return err
	}
	// Each part of the app is signed as a part of it, never as what its
	// name or an Info.plist built into it says, see codeIdentifier.
	for _, code := range inner {
		if err := sign("--identifier", codeIdentifier(code), code); err != nil {
			return err
		}
	}
	// Signing the bundle signs its program, with the entitlements, and
	// seals everything else in it.
	// And as com.framefairy.app whatever its Info.plist says, so no
	// build is ever signed as another app of the same team, which macOS
	// would trust with that app's permissions and keychain items.
	if err := sign("--identifier", bundleID, "--entitlements", d.entitlements, app); err != nil {
		return err
	}
	if _, err := d.run("codesign", "--verify", "--strict", "--deep", app); err != nil {
		return err
	}
	if d.requirement != "" {
		own := filepath.Join(app, "Contents", "MacOS", appProgram)
		for _, code := range slices.Concat(inner, []string{own, app}) {
			if _, err := d.run("codesign", "--verify", "--strict", "-R="+d.requirement, code); err != nil {
				return err
			}
		}
	}
	if err := readBack(app, inner); err != nil {
		return err
	}
	fmt.Printf("Signed %s with the hardened runtime: %d programs and libraries, then the app\n", app, len(inner))
	return nil
}

// keychains are the paths security lists, one a line, each in quotes.
func keychains(listed []byte) []string {
	var paths []string
	for _, l := range strings.Split(string(listed), "\n") {
		if p := strings.Trim(strings.TrimSpace(l), `"`); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// innerCode is what is signed before the app: every library in
// Contents/Frameworks, then every program in Contents/MacOS but the app's
// own, llama-server above all, which the app starts as a process of its
// own and which needs no entitlement.
func innerCode(app string) ([]string, error) {
	libs, err := filepath.Glob(filepath.Join(app, "Contents", "Frameworks", "*.dylib"))
	if err != nil {
		return nil, err
	}
	programs, err := filepath.Glob(filepath.Join(app, "Contents", "MacOS", "*"))
	if err != nil {
		return nil, err
	}
	own := filepath.Join(app, "Contents", "MacOS", appProgram)
	programs = slices.DeleteFunc(programs, func(p string) bool { return p == own })
	slices.Sort(libs)
	slices.Sort(programs)
	return slices.Concat(libs, programs), nil
}

// readBack reads every program the way the release check will, and says
// what is not signed with the hardened runtime by a developer, a program
// signed as anything but its part of the app, or an app signed without
// the microphone, which would then hear only silence.
func readBack(app string, inner []string) error {
	own := filepath.Join(app, "Contents", "MacOS", appProgram)
	var found []string
	for _, code := range append([]string{own}, inner...) {
		data, err := os.ReadFile(code)
		if err != nil {
			return err
		}
		arches, err := archesOf(data)
		if err != nil || len(arches) == 0 {
			found = append(found, fmt.Sprintf("%s is no program: %v", code, err))
			continue
		}
		for _, a := range arches {
			s, err := signingOf(a)
			switch {
			case err != nil:
				found = append(found, fmt.Sprintf("%s: %v", code, err))
			case !s.signed || s.flags&flagAdhoc != 0 || s.flags&flagRuntime == 0:
				found = append(found, fmt.Sprintf("%s is not signed with the hardened runtime, flags %#x", code, s.flags))
			}
			if err == nil && s.ident != codeIdentifier(code) {
				found = append(found, fmt.Sprintf("%s is signed as %q, not %s", code, s.ident, codeIdentifier(code)))
			}
			if code == own {
				keys, err := entitlements(a.f, a.data)
				if err != nil || !slices.Contains(keys, "com.apple.security.device.audio-input") {
					found = append(found, fmt.Sprintf("%s is signed without the microphone, so Dictate on Report a Problem would hear only silence: %q %v", code, keys, err))
				}
			}
		}
	}
	// A program anywhere else keeps whatever signature the branch gave
	// it, which nothing here made.
	err := filepath.WalkDir(app, func(file string, e os.DirEntry, err error) error {
		if err != nil || !e.Type().IsRegular() || file == own || slices.Contains(inner, file) {
			return err
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		head := make([]byte, 4)
		_, err = io.ReadFull(f, head)
		f.Close()
		if err == nil && startsLikeAProgram(head) {
			found = append(found, file+" is a program outside what harden signs, Contents/MacOS and the libraries in Contents/Frameworks")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) > 0 {
		return fmt.Errorf("the app is not signed as it should be:\n  %s", strings.Join(found, "\n  "))
	}
	return nil
}

// appleRoots are the certificates named Apple Root CA among the roots
// macOS itself ships.
func (d devID) appleRoots() ([]*x509.Certificate, error) {
	out, err := d.run("security", "find-certificate", "-a", "-p", "-c", "Apple Root CA",
		"/System/Library/Keychains/SystemRootCertificates.keychain")
	if err != nil {
		return nil, err
	}
	var roots []*x509.Certificate
	for rest := out; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil || c.Subject.CommonName != "Apple Root CA" || !c.IsCA {
			continue
		}
		roots = append(roots, c)
	}
	if len(roots) == 0 {
		return nil, errors.New("macOS's own roots hold no Apple Root CA")
	}
	return roots, nil
}

// appleCA says whether a certificate fetched from apple.com is Apple's
// Developer ID authority, signed by Apple's root as macOS ships it, so
// nothing else is put in the keychain the Developer ID signs from. The
// signature is checked here, not by the Mac's verifier, which judges a
// certificate as one for a web site and refuses Apple's own.
func appleCA(der []byte, roots []*x509.Certificate) error {
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return fmt.Errorf("it is no certificate: %w", err)
	}
	if c.Subject.CommonName != "Developer ID Certification Authority" || !c.IsCA {
		return fmt.Errorf("it is %q, not Apple's Developer ID authority", c.Subject.CommonName)
	}
	for _, root := range roots {
		if signedBy(c, root) == nil {
			return nil
		}
	}
	return fmt.Errorf("it is not signed by Apple's root, %v", c.SignatureAlgorithm)
}

// signedBy checks c's signature with parent's key. Apple's root of 2006
// signs with SHA-1, which Go's own check no longer takes, so that one is
// checked here: SHA-1 is broken for collisions, which only matter to
// someone who can get Apple to sign what they made, not for a signature
// Apple already made.
func signedBy(c, parent *x509.Certificate) error {
	key, isRSA := parent.PublicKey.(*rsa.PublicKey)
	if c.SignatureAlgorithm != x509.SHA1WithRSA || !isRSA {
		return c.CheckSignatureFrom(parent)
	}
	if !bytes.Equal(c.RawIssuer, parent.RawSubject) {
		return errors.New("it names another issuer")
	}
	sum := sha1.Sum(c.RawTBSCertificate)
	return rsa.VerifyPKCS1v15(key, crypto.SHA1, sum[:], c.Signature)
}

// unpack writes the files of the build's zip into dir, refusing any entry
// that is not a file or folder inside the .app, so nothing in it lands
// anywhere else on the runner.
func unpack(zipPath, dir string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the build is not a zip: %w", err)
	}
	defer z.Close()
	for _, f := range z.File {
		if !insideApp(f.Name) {
			return fmt.Errorf("%s lies outside %s", f.Name, appName)
		}
		target := filepath.Join(dir, filepath.FromSlash(path.Clean(f.Name)))
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		case !mode.IsRegular():
			return fmt.Errorf("%s is no file", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm()&0o755)
		if err != nil {
			r.Close()
			return err
		}
		_, err = io.Copy(w, io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}
