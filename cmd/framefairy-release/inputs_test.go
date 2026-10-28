package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The app never reads keys meant for other apps or watches the screen or
// the camera, issue #369, and a build that could is never signed. These
// make the programs of a build by hand, as small as macOS's own loader
// would read them, with what each test needs in them.

// program is a 64-bit arm64 Mach-O program: the libraries it links, the
// functions and classes it imports, text anywhere in it, and the
// entitlements it is signed with, as the XML codesign writes, or none.
type program struct {
	// cpu is arm64 when it is 0.
	cpu      uint32
	links    []string
	imports  []string
	text     string
	entitled string
}

// bytes lays the program out: the header, its load commands, then its
// symbols and their names, then the text, then the code signature.
func (p program) bytes() []byte {
	le := binary.LittleEndian
	var cmds bytes.Buffer
	ncmds := 0
	add := func(words ...uint32) {
		for _, w := range words {
			_ = binary.Write(&cmds, le, w)
		}
		ncmds++
	}
	for _, l := range p.links {
		name := append([]byte(l), 0)
		for len(name)%8 != 0 {
			name = append(name, 0)
		}
		// LC_LOAD_DYLIB, its size, where its name starts, a time and two
		// versions, then the name.
		add(0xc, uint32(24+len(name)), 24, 2, 0x10000, 0x10000)
		cmds.Write(name)
	}
	// After the header and the commands: the symbols, the names, the
	// text, the signature. The commands for them are 24, 80 and 16 bytes.
	cmdsize := cmds.Len() + 24 + 80
	if p.entitled != "" {
		cmdsize += 16
	}
	symoff := 32 + cmdsize
	var syms, names bytes.Buffer
	names.WriteByte(0)
	for _, s := range p.imports {
		// An undefined external symbol.
		_ = binary.Write(&syms, le, uint32(names.Len()))
		syms.Write([]byte{0x01, 0})
		_ = binary.Write(&syms, le, uint16(0))
		_ = binary.Write(&syms, le, uint64(0))
		names.WriteString(s)
		names.WriteByte(0)
	}
	stroff := symoff + syms.Len()
	textoff := stroff + names.Len()
	sigoff := textoff + len(p.text)
	// LC_SYMTAB, and LC_DYSYMTAB with every symbol undefined.
	add(0x2, 24, uint32(symoff), uint32(len(p.imports)), uint32(stroff), uint32(names.Len()))
	add(0xb, 80, 0, 0, 0, 0, 0, uint32(len(p.imports)), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	var sig []byte
	if p.entitled != "" {
		be := binary.BigEndian
		blob := be.AppendUint32(nil, 0xfade7171)
		blob = be.AppendUint32(blob, uint32(8+len(p.entitled)))
		blob = append(blob, p.entitled...)
		// A blob of blobs with one, the entitlements in slot 5.
		sig = be.AppendUint32(nil, 0xfade0cc0)
		sig = be.AppendUint32(sig, uint32(20+len(blob)))
		sig = be.AppendUint32(sig, 1)
		sig = be.AppendUint32(sig, 5)
		sig = be.AppendUint32(sig, 20)
		sig = append(sig, blob...)
		add(0x1d, 16, uint32(sigoff), uint32(len(sig)))
	}
	cpu := p.cpu
	if cpu == 0 {
		cpu = 0x0100000c
	}
	var out bytes.Buffer
	for _, w := range []uint32{0xfeedfacf, cpu, 0, 2, uint32(ncmds), uint32(cmds.Len()), 0, 0} {
		_ = binary.Write(&out, le, w)
	}
	out.Write(cmds.Bytes())
	out.Write(syms.Bytes())
	out.Write(names.Bytes())
	out.WriteString(p.text)
	out.Write(sig)
	return out.Bytes()
}

// fat wraps programs in a universal file, one architecture each, each
// on a page of its own.
func fat(ps ...program) []byte {
	be := binary.BigEndian
	head := be.AppendUint32(nil, 0xcafebabe)
	head = be.AppendUint32(head, uint32(len(ps)))
	size := 8 + 20*len(ps)
	at := size
	var body []byte
	for _, p := range ps {
		at = (at + 4095) &^ 4095
		b := p.bytes()
		cpu := p.cpu
		if cpu == 0 {
			cpu = 0x0100000c
		}
		head = be.AppendUint32(head, cpu)
		head = be.AppendUint32(head, 0)
		head = be.AppendUint32(head, uint32(at))
		head = be.AppendUint32(head, uint32(len(b)))
		head = be.AppendUint32(head, 12)
		for size+len(body) < at {
			body = append(body, 0)
		}
		body = append(body, b...)
		at += len(b)
	}
	return append(head, body...)
}

// app is the app's own program as it links today, which may link what it
// does and nothing more.
var app = program{
	links: []string{
		"/System/Library/Frameworks/Cocoa.framework/Versions/A/Cocoa",
		"/System/Library/Frameworks/WebKit.framework/Versions/A/WebKit",
		"/usr/lib/libSystem.B.dylib",
		"@rpath/libonnxruntime.dylib",
	},
	imports: []string{"_NSClassFromString", "_OBJC_CLASS_$_NSEvent", "_RegisterEventHotKey", "_dlsym"},
	text:    "AudioUnit",
}

const plistHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`

func TestWhatCouldWatchInputIsFound(t *testing.T) {
	with := func(change func(p *program)) []byte {
		p := app
		p.links = slices.Clone(app.links)
		p.imports = slices.Clone(app.imports)
		change(&p)
		return p.bytes()
	}
	cases := []struct {
		name, file string
		data       []byte
		want       []string
	}{
		{"the app as it is", "framefairy-app", app.bytes(), nil},
		{"a tap on every key", "framefairy-app", with(func(p *program) { p.imports = append(p.imports, "_CGEventTapCreate") }),
			[]string{"imports _CGEventTapCreate", "names CGEventTapCreate"}},
		{"the camera", "framefairy-app", with(func(p *program) { p.imports = append(p.imports, "_OBJC_CLASS_$_AVCaptureDevice") }),
			[]string{"imports _OBJC_CLASS_$_AVCaptureDevice", "names AVCaptureDevice"}},
		// Looked up by name while it runs, with NSClassFromString.
		{"the screen, by name", "framefairy-app", with(func(p *program) { p.text += "SCShareableContent" }),
			[]string{"names SCShareableContent"}},
		{"another app's windows, by name", "ffmpeg", with(func(p *program) { p.links = nil; p.text = "AXUIElementCreateSystemWide" }),
			[]string{"names AXUIElementCreateSystemWide"}},
		{"a framework not on the list", "framefairy-app",
			with(func(p *program) {
				p.links = append(p.links, "/System/Library/Frameworks/ScreenCaptureKit.framework/Versions/A/ScreenCaptureKit")
			}),
			[]string{"links /System/Library/Frameworks/ScreenCaptureKit.framework/Versions/A/ScreenCaptureKit"}},
		{"a program the list does not know", "watcher", program{links: []string{"/usr/lib/libSystem.B.dylib"}}.bytes(),
			[]string{"is a program the list of linked libraries does not know, and links /usr/lib/libSystem.B.dylib"}},
		{"the microphone, entitled", "framefairy-app",
			with(func(p *program) {
				p.entitled = plistHead + "<key>com.apple.security.device.audio-input</key><true/>\n</dict>\n</plist>\n"
			}), nil},
		{"the camera, entitled", "framefairy-app",
			with(func(p *program) {
				p.entitled = plistHead + "<key>com.apple.security.device.audio-input</key><true/>\n" +
					"<key>com.apple.security.device.camera</key><true/>\n</dict>\n</plist>\n"
			}),
			[]string{"is entitled to com.apple.security.device.camera"}},
		{"other code let in, entitled", "framefairy-app",
			with(func(p *program) {
				p.entitled = plistHead + "<key>com.apple.security.cs.disable-library-validation</key><true/>\n</dict>\n</plist>\n"
			}),
			[]string{"is entitled to com.apple.security.cs.disable-library-validation"}},
		// A universal program is read for every architecture.
		{"a universal program, one architecture with a tap", "framefairy-app",
			fat(app, program{cpu: 0x01000007, links: app.links, imports: []string{"_IOHIDManagerOpen"}}),
			[]string{"imports _IOHIDManagerOpen", "names IOHIDManagerOpen"}},
		{"the folders and the microphone asked for", "Frame Fairy.app/Contents/Info.plist",
			[]byte(plistHead + "<key>NSDesktopFolderUsageDescription</key><string>x</string>\n" +
				"<key>NSMicrophoneUsageDescription</key><string>x</string>\n" +
				"<!-- <key>NSCameraUsageDescription</key> -->\n</dict>\n</plist>\n"), nil},
		{"the camera asked for", "Frame Fairy.app/Contents/Info.plist",
			[]byte(plistHead + "<key>NSMicrophoneUsageDescription</key><string>x</string>\n" +
				"<key>NSCameraUsageDescription</key><string>x</string>\n</dict>\n</plist>\n"),
			[]string{"asks for NSCameraUsageDescription"}},
		// A key deeper in the dictionary is no reason Info.plist gives.
		{"a key inside another", "Frame Fairy.app/Contents/Info.plist",
			[]byte(plistHead + "<key>CFBundleURLTypes</key><array><dict><key>NSScreenCaptureUsageDescription</key><string>x</string></dict></array>\n</dict>\n</plist>\n"),
			nil},
		{"a file that is no program", "Frame Fairy.app/Contents/Resources/LICENSE", []byte("CGEventTapCreate is named here"), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := inputsOf(c.file, c.data)
			if !slices.Equal(got, c.want) {
				t.Errorf("found %q, want %q", got, c.want)
			}
		})
	}
}

// What bundle-macos.sh writes as the app's Info.plist asks for nothing but
// the folders and the microphone, so a pull request that adds a reason
// for the camera or the screen fails here, before its build is refused.
//
// On the real disk: what this reads is the script that writes Info.plist.
func TestTheShippedInfoPlistAsksForNoInput(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts/bundle-macos.sh"))
	if err != nil {
		t.Fatal(err)
	}
	_, plist, ok := strings.Cut(string(script), "<<PLIST\n")
	plist, _, found := strings.Cut(plist, "\nPLIST\n")
	if !ok || !found {
		t.Fatal("scripts/bundle-macos.sh writes no Info.plist between <<PLIST and PLIST")
	}
	keys, err := plistKeys([]byte(plist))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(keys, "NSMicrophoneUsageDescription") {
		t.Fatalf("read %q from it, which is not the app's Info.plist", keys)
	}
	if got := inputsOf("Frame Fairy.app/Contents/Info.plist", []byte(plist)); got != nil {
		t.Errorf("the app's Info.plist %s", strings.Join(got, ", "))
	}
}

// zipped is a zip of files, by name.
func zipped(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(files[n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// theApp is a build of the app for a channel that watches nothing.
func theApp(channel string) map[string][]byte {
	return map[string][]byte{
		"Frame Fairy.app/Contents/MacOS/framefairy-app": app.bytes(),
		"Frame Fairy.app/Contents/Info.plist": []byte(plistHead +
			"<key>CFBundleName</key><string>Frame Fairy " + channel + "</string>\n</dict>\n</plist>\n"),
	}
}

// A build that could watch input is refused before it is signed, and no
// entry is written for it, so no app is offered it.
//
// On the real disk: the release tool reads the zip and writes the entry as files.
func TestABuildThatCouldWatchInputIsNotSigned(t *testing.T) {
	release(t)
	files := theApp("main")
	watcher := app
	watcher.imports = append(slices.Clone(app.imports), "_CGWindowListCreateImage")
	files["Frame Fairy.app/Contents/MacOS/framefairy-app"] = watcher.bytes()
	zipPath := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "main.json")
	err := sign([]string{"-zip", zipPath, "-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234",
		"-commit", "abc1234def56", "-url", "https://example.com/main.zip", "-out", out})
	if err == nil || !strings.Contains(err.Error(), "Frame Fairy.app/Contents/MacOS/framefairy-app: imports _CGWindowListCreateImage") {
		t.Errorf("signing a build that imports CGWindowListCreateImage said %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("an entry was written for it")
	}
}
