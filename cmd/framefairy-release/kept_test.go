package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"framefairy/updates"
)

// ber writes one element of BER with a definite length.
func ber(tag byte, content ...[]byte) []byte {
	body := bytes.Join(content, nil)
	out := []byte{tag}
	switch n := len(body); {
	case n < 0x80:
		out = append(out, byte(n))
	case n < 0x100:
		out = append(out, 0x81, byte(n))
	default:
		out = append(out, 0x82, byte(n>>8), byte(n))
	}
	return append(out, body...)
}

// berOpen writes one constructed element of BER with its length left
// open, the way Apple's codesign writes most of a CMS.
func berOpen(tag byte, content ...[]byte) []byte {
	out := append([]byte{tag, 0x80}, bytes.Join(content, nil)...)
	return append(out, 0, 0)
}

// testCMS is a CMS as a code signature carries it: a SignedData with
// certs, and a signer whose signing time is stamp. With open, every
// constructed element has its length left open.
func testCMS(certs [][]byte, stamp int, open bool) []byte {
	cons := ber
	if open {
		cons = berOpen
	}
	signedData := []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x07, 0x02}
	data := []byte{0x06, 0x09, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x07, 0x01}
	signer := cons(0x30, ber(0x02, []byte{1}), ber(0x17, fmt.Appendf(nil, "261127%06dZ", stamp)))
	return cons(0x30, signedData, cons(0xa0, cons(0x30,
		ber(0x02, []byte{1}),
		cons(0x31),
		cons(0x30, data),
		cons(0xa0, certs...),
		cons(0x31, signer),
	)))
}

// testCerts are two certificates as a CMS carries them, a leaf and the
// one that issued it. Their insides are never read, only compared.
var testCerts = [][]byte{
	ber(0x30, ber(0x30, []byte("Developer ID Application: an invented developer"))),
	ber(0x30, ber(0x30, []byte("Developer ID Certification Authority"))),
}

// otherCerts are what a renewed certificate would be, of the same length,
// so a signature with it is as long and only the certificate tells them
// apart.
var otherCerts = [][]byte{
	ber(0x30, ber(0x30, []byte("Developer ID Application: an invented developeR"))),
	testCerts[1],
}

// The certificates are read from a CMS whether its lengths are open, as
// Apple writes them, or not, and a CMS that is not one is refused.
func TestTheCertificatesAreReadFromTheCMS(t *testing.T) {
	for _, open := range []bool{false, true} {
		got, err := cmsCertificates(append(make([]byte, 8), testCMS(testCerts, 1, open)...))
		if err != nil {
			t.Fatalf("open %v: %v", open, err)
		}
		if len(got) != 2 || !bytes.Equal(got[0], testCerts[0]) || !bytes.Equal(got[1], testCerts[1]) {
			t.Errorf("open %v: read %q", open, got)
		}
	}
	deep := []byte{}
	for range berMaxDepth + 2 {
		deep = append([]byte{0x30, 0x80}, deep...)
	}
	for name, blob := range map[string][]byte{
		"too short":   {1, 2, 3},
		"cut off":     append(make([]byte, 8), testCMS(testCerts, 1, true)[:40]...),
		"no sequence": append(make([]byte, 8), ber(0x04, []byte("text"))...),
		"nested deep": append(make([]byte, 8), deep...),
		"no content":  append(make([]byte, 8), ber(0x30, ber(0x06, []byte{1}))...),
		"no certificates": append(make([]byte, 8), ber(0x30, ber(0x06, []byte{1}),
			ber(0xa0, ber(0x30, ber(0x02, []byte{1}))))...),
	} {
		if _, err := cmsCertificates(blob); err == nil {
			t.Errorf("%s: read as a CMS", name)
		}
	}
}

// A library signed again is the same signed code as before, though its
// signing time differs. codesign leaves room for the signature, so its
// size in the program's commands is the same both times. Its code, its identifier, its other blobs or its
// certificates changed, or no developer's signature, and it is not.
func TestALibrarySignedAgainIsTheSameSignedCode(t *testing.T) {
	signedAt := func(p program, stamp int, certs [][]byte, open bool) []byte {
		return signedLib(p, stamp, certs, open).bytes()
	}
	fresh := signedAt(speech, 2, testCerts, true)
	if err := sameSignedCode(fresh, signedAt(speech, 1, testCerts, true)); err != nil {
		t.Errorf("signed again: %v", err)
	}
	changed := speech
	changed.text = "a new line of code"
	other := speech
	other.ident = "com.framefairy.app.llama-server"
	other.signed, other.flags, other.team = true, flagRuntime, "TEAM123456"
	other.cms = testCMS(testCerts, 1, false)
	adhoc := speech
	adhoc.signed, adhoc.flags, adhoc.ident = true, flagAdhoc, "com.framefairy.app.libonnxruntime"
	// Of the same length as fresh, so only the blob tells them apart.
	sameLength := func(p program) []byte {
		p.signed, p.flags, p.team = true, flagRuntime, "TEAM123456"
		if p.ident == "" {
			p.ident = "com.framefairy.app.libonnxruntime"
		}
		p.cms = testCMS(testCerts, 1, true)
		return p.bytes()
	}
	otherIdent, entitled, otherEntitled := speech, speech, speech
	otherIdent.ident = "com.framefairy.app.libonnxruntimX"
	entitled.entitled, otherEntitled.entitled = "<a/>", "<b/>"
	if err := sameSignedCode(sameLength(entitled), sameLength(entitled)); err != nil {
		t.Errorf("the same entitlements: %v", err)
	}
	if err := sameSignedCode(sameLength(entitled), sameLength(otherEntitled)); err == nil {
		t.Error("other entitlements of the same length: taken as the same signed code")
	}
	cases := map[string][]byte{
		"its code changed":                      signedAt(changed, 1, testCerts, false),
		"another identifier":                    other.bytes(),
		"another identifier of the same length": sameLength(otherIdent),
		"a renewed certificate":                 signedAt(speech, 1, otherCerts, true),
		"signed ad hoc":                         adhoc.bytes(),
		"no program":                            []byte("a licence"),
		"another architecture":                  signedAt(program{cpu: 0x01000007, links: speech.links}, 1, testCerts, false),
		"an extra architecture":                 fat(signedLib(speech, 1, testCerts, true), signedLib(intel, 1, testCerts, true)),
		"bytes after the signature":             append(signedAt(speech, 1, testCerts, true), "unsigned"...),
	}
	for name, earlier := range cases {
		if err := sameSignedCode(fresh, earlier); err == nil {
			t.Errorf("%s: taken as the same signed code", name)
		}
	}
	// A fat library: signed again it is the same, with an architecture
	// fewer it is not, and the other way round neither, without a panic,
	// and with anything in the padding between its slices it is not.
	freshFat := fat(signedLib(speech, 2, testCerts, true), signedLib(intel, 2, testCerts, true))
	earlierFat := fat(signedLib(speech, 1, testCerts, true), signedLib(intel, 1, testCerts, true))
	if err := sameSignedCode(freshFat, earlierFat); err != nil {
		t.Errorf("a fat library signed again: %v", err)
	}
	if err := sameSignedCode(freshFat, signedAt(speech, 1, testCerts, true)); err == nil {
		t.Error("a thin copy of a fat library: taken as the same signed code")
	}
	padded := slices.Clone(earlierFat)
	padded[1000] = 1
	if err := sameSignedCode(freshFat, padded); err == nil {
		t.Error("a fat library with something in its padding: taken as the same signed code")
	}
}

// intel is the speech library for an Intel Mac, for a fat library.
var intel = program{cpu: 0x01000007, links: speech.links}

// signedLib is a library signed with the Developer ID by certs, at the
// time stamp, its CMS written with open lengths or not.
func signedLib(p program, stamp int, certs [][]byte, open bool) program {
	p.signed, p.flags, p.team = true, flagRuntime, "TEAM123456"
	p.ident = "com.framefairy.app.libonnxruntime"
	p.cms = testCMS(certs, stamp, open)
	return p
}

// harden keeps the earlier copy of a library that is the same signed
// code, so the library is the same file as in the build before, and
// signs afresh what changed, and the app's own program always. The app is
// sealed after, around what was kept, and every file is checked.
//
// On the real disk: codesign's stand-in writes the files it signs.
func TestHardenKeepsTheSignedBytesOfALibraryThatDidNotChange(t *testing.T) {
	m := &fakeMac{flags: flagRuntime, microphone: true, certs: testCerts}
	d := signer(m)
	before := bundle(t)
	if err := d.signApp(before); err != nil {
		t.Fatal(err)
	}
	// What signed-before would write: each library and program of the
	// build before, under its name in the zip and its SHA-256.
	kept := t.TempDir()
	keep := func(rel string, data []byte) {
		sum := sha256.Sum256(data)
		dir := filepath.Join(kept, appName, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, hex.EncodeToString(sum[:])), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(app, rel string) []byte {
		b, err := os.ReadFile(filepath.Join(app, rel))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	libs := []string{"Contents/Frameworks/libonnxruntime.dylib", "Contents/Frameworks/libsherpa-onnx-c-api.dylib", "Contents/MacOS/llama-server"}
	for _, rel := range libs {
		keep(rel, read(before, rel))
	}
	// One signed with a certificate since renewed is not kept.
	renewed := speech
	renewed.signed, renewed.flags, renewed.team = true, flagRuntime, "TEAM123456"
	renewed.ident = codeIdentifier("libsherpa-onnx-cxx-api.dylib")
	renewed.cms = testCMS(otherCerts, 9, true)
	keep("Contents/Frameworks/libsherpa-onnx-cxx-api.dylib", renewed.bytes())

	after := bundle(t)
	d.kept = kept
	if err := d.signApp(after); err != nil {
		t.Fatal(err)
	}
	for _, rel := range libs {
		if !bytes.Equal(read(after, rel), read(before, rel)) {
			t.Errorf("%s was signed afresh, though its code did not change", rel)
		}
	}
	if bytes.Equal(read(after, "Contents/Frameworks/libsherpa-onnx-cxx-api.dylib"), renewed.bytes()) {
		t.Error("a library signed with a renewed certificate was kept")
	}
	// Sealed and checked after the keeping: the app's own signing is the
	// last, and codesign verifies the bundle after it.
	signed := m.signed()
	if last := signed[len(signed)-1]; !strings.HasSuffix(last[len(last)-1], ".app") {
		t.Errorf("the app was not sealed last: %q", last)
	}
	verified := false
	for _, c := range m.commands {
		if c[0] == "codesign" && c[1] == "--verify" {
			verified = true
		}
	}
	if !verified {
		t.Error("nothing was verified after the keeping")
	}
}

// signed-before downloads main's copy of each library first, then the
// channel's, at most keptPerFile and each file once, never the app's own
// program, and leaves out a piece that is not the file its parts say. It
// reads an entry the way publish writes it: the parts are those of the
// zip that stores its files, not of the zip at its address. A channel
// with no build costs nothing but its copies.
//
// On the real disk: signed-before writes the copies for harden to read.
func TestSignedBeforeDownloadsTheLibrariesOfEachBuildOnce(t *testing.T) {
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	served := map[string][]byte{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := served[strings.TrimPrefix(r.URL.Path, "/dev/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()
	part := func(name string, data []byte) updates.Part {
		sum := sha256.Sum256(data)
		packed := enc.EncodeAll(data, nil)
		p := updates.Part{Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Packed: int64(len(packed))}
		served[updates.PieceName(p.SHA256)] = packed
		return p
	}
	signature := base64.StdEncoding.EncodeToString(make([]byte, 64))
	// build serves a channel's entry and its parts, the way sign and
	// publish write them: the zip at the address deflates its files, and
	// the parts make the stored zip beside it, whose size and checksum
	// are in Stored.
	build := func(channel string, parts ...updates.Part) {
		p := updates.Parts{Parts: append([]updates.Part{{Bytes: []byte("PK")}}, parts...)}
		for _, part := range p.Parts {
			p.Size += int64(len(part.Bytes)) + part.Size
		}
		p.SHA256 = strings.Repeat("ab", 32)
		zip := "app-" + channel + ".zip"
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		served[updates.PartsName(zip)] = data
		entry, err := json.Marshal(updates.Build{Channel: channel, Name: channel, Version: "0.3.0", Commit: "c7fb725",
			URL: srv.URL + "/dev/" + zip, Size: p.Size / 2, SHA256: strings.Repeat("cd", 32), Signature: signature,
			Stored: &updates.Stored{Size: p.Size, SHA256: p.SHA256, Signature: signature, Claim: signature, PackedSize: p.Size / 2}})
		if err != nil {
			t.Fatal(err)
		}
		served["channel-"+channel+".json"] = entry
	}
	lib := appName + "/Contents/Frameworks/libonnxruntime.dylib"
	other := appName + "/Contents/Frameworks/libsherpa-onnx-c-api.dylib"
	llama := appName + "/Contents/MacOS/llama-server"
	own := appName + "/Contents/MacOS/" + appProgram
	same := part(other, []byte("a library no build changed"))
	build("main",
		part(lib, []byte("main's library")),
		same,
		part(own, []byte("main's program")),
		part(appName+"/Contents/Info.plist", []byte("<plist/>")))
	bad := part(llama, []byte("what the parts say"))
	served[updates.PieceName(bad.SHA256)] = enc.EncodeAll([]byte("another file"), nil)
	build("pr-7", part(lib, []byte("the pull request's library")), same, bad)
	build("pr-9", part(lib, []byte("a third library")))

	out := t.TempDir()
	var log bytes.Buffer
	src := &updates.Source{Entries: srv.URL + "/dev/", Client: srv.Client()}
	if err := signedBefore(t.Context(), src, []string{"main", "pr-7", "pr-8", "pr-9"}, out, &log); err != nil {
		t.Fatal(err)
	}
	got := func(name string) []string {
		entries, _ := os.ReadDir(filepath.Join(out, filepath.FromSlash(name)))
		var data []string
		for _, e := range entries {
			b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name), e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, string(b))
		}
		return data
	}
	// In the order harden tries them: main's first.
	if libs := got(lib); !slices.Equal(libs, []string{"main's library", "the pull request's library"}) {
		t.Errorf("the library's copies are %q, want main's and then the pull request's, and no third", libs)
	}
	if copies := got(other); len(copies) != 1 {
		t.Errorf("a library two builds share was downloaded %d times", len(copies))
	}
	if n := len(got(own)); n != 0 {
		t.Errorf("the app's own program was downloaded %d times, though it changes every build", n)
	}
	if n := len(got(llama)); n != 0 || !strings.Contains(log.String(), "no earlier copy of "+llama) {
		t.Errorf("a piece that is another file was kept %d times, and said: %s", n, log.String())
	}
	if !strings.Contains(log.String(), "no earlier copies from pr-8") {
		t.Errorf("a channel with no build was not said: %s", log.String())
	}
}

// What keepable takes: the libraries and the programs beside the app's
// own, and nothing outside the .app.
func TestOnlyLibrariesAndProgramsBesideTheAppAreKept(t *testing.T) {
	for name, want := range map[string]bool{
		appName + "/Contents/Frameworks/libonnxruntime.dylib": true,
		appName + "/Contents/MacOS/llama-server":              true,
		appName + "/Contents/MacOS/" + appProgram:             false,
		appName + "/Contents/Info.plist":                      false,
		appName + "/Contents/Frameworks/notes.txt":            false,
		appName + "/Contents/Frameworks/x/../../MacOS/a":      false,
		"Other.app/Contents/MacOS/llama-server":               false,
	} {
		if got := keepable(name); got != want {
			t.Errorf("keepable(%q) = %v, want %v", name, got, want)
		}
	}
}

// What the release holds is untrusted, and the CMS is read by hand, so
// whatever it holds is read without a panic.
func FuzzCMSCertificates(f *testing.F) {
	f.Add(append(make([]byte, 8), testCMS(testCerts, 1, true)...))
	f.Add(append(make([]byte, 8), testCMS(testCerts, 1, false)...))
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0x30, 0x80, 0x30, 0x80})
	f.Fuzz(func(t *testing.T, blob []byte) {
		_, _ = cmsCertificates(blob)
	})
}

// An earlier copy from the release is untrusted, and its signature is read
// by hand, so whatever it holds is compared without a panic, and only a
// copy that is the same signed code is taken.
func FuzzSameSignedCode(f *testing.F) {
	fresh := signedLib(speech, 2, testCerts, true).bytes()
	f.Add(signedLib(speech, 1, testCerts, true).bytes())
	f.Add(signedLib(speech, 1, otherCerts, true).bytes())
	f.Add(fat(signedLib(speech, 1, testCerts, true), signedLib(intel, 1, testCerts, true)))
	f.Fuzz(func(t *testing.T, earlier []byte) {
		if sameSignedCode(fresh, earlier) == nil && len(earlier) != len(fresh) {
			t.Errorf("a copy of %d bytes taken for one of %d", len(earlier), len(fresh))
		}
	})
}
