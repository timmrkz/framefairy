package main

// The app must never read keys typed into other apps, or watch the screen
// or the camera, issue #369. macOS asks the person before any of that
// works, so what is checked here is that a build never asks: a build that
// can is refused before it is signed, and no app installs a build that is
// not signed, see updates.Verify. sign runs from main, see publish.yml, so
// these lists are main's, and a branch that changes them changes nothing
// about how its own build is judged.
//
// What this cannot catch is code that hides the name it looks up, built
// from pieces at run time. Then macOS is the wall that holds: none of
// these inputs works without the person allowing it in System Settings.

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// watching are the functions and the Objective-C classes that read input
// meant for other apps or watch the screen or the camera. Each is refused
// as an import, and as text anywhere in a program, which catches one looked
// up by name at run time, with dlsym or NSClassFromString.
// RegisterEventHotKey is not one of them: Wails links it for global
// shortcuts, which take only the keys they are registered for, and the app
// registers none, see the test in internal/inputs.
var watching = []string{
	// Every key and click of every app, with Input Monitoring.
	"CGEventTapCreate", "CGEventTapCreateForPid", "CGRequestListenEventAccess",
	"IOHIDManagerOpen", "IOHIDDeviceRegisterInputValueCallback",
	// Other apps' windows and what is typed into them, with Accessibility.
	"AXIsProcessTrusted", "AXIsProcessTrustedWithOptions", "AXUIElementCreateSystemWide",
	// The screen, with Screen Recording.
	"CGWindowListCreateImage", "CGDisplayStreamCreate", "SCStream", "SCShareableContent",
	// The camera.
	"AVCaptureDevice",
}

// linked are the libraries each program of the app links, by the name of
// its file. A library not on its list, ScreenCaptureKit, AVFoundation or
// IOKit for the keyboard's own reports, fails until the list is changed on
// main, in a pull request that says why. A program not on the list fails
// the same way.
var linked = map[string][]string{
	"framefairy-app": {
		"/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit",
		"/System/Library/Frameworks/Carbon.framework/Versions/A/Carbon",
		"/System/Library/Frameworks/Cocoa.framework/Versions/A/Cocoa",
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/CoreGraphics.framework/Versions/A/CoreGraphics",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/System/Library/Frameworks/QuartzCore.framework/Versions/A/QuartzCore",
		"/System/Library/Frameworks/Security.framework/Versions/A/Security",
		"/System/Library/Frameworks/WebKit.framework/Versions/A/WebKit",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libobjc.A.dylib", "/usr/lib/libresolv.9.dylib",
		"@rpath/libonnxruntime.dylib", "@rpath/libsherpa-onnx-c-api.dylib",
	},
	"framefairy-frames": {
		"/System/Library/Frameworks/ApplicationServices.framework/Versions/A/ApplicationServices",
		"/System/Library/Frameworks/AudioToolbox.framework/Versions/A/AudioToolbox",
		"/System/Library/Frameworks/CoreAudio.framework/Versions/A/CoreAudio",
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/CoreMedia.framework/Versions/A/CoreMedia",
		"/System/Library/Frameworks/CoreServices.framework/Versions/A/CoreServices",
		"/System/Library/Frameworks/CoreText.framework/Versions/A/CoreText",
		"/System/Library/Frameworks/CoreVideo.framework/Versions/A/CoreVideo",
		"/System/Library/Frameworks/Security.framework/Versions/A/Security",
		"/System/Library/Frameworks/VideoToolbox.framework/Versions/A/VideoToolbox",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libiconv.2.dylib", "/usr/lib/libresolv.9.dylib", "/usr/lib/libz.1.dylib",
	},
	"ffmpeg":  ffmpegLinks,
	"ffprobe": ffmpegLinks,
	"llama-server": {
		"/System/Library/Frameworks/Accelerate.framework/Versions/A/Accelerate",
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/System/Library/Frameworks/Metal.framework/Versions/A/Metal",
		"/System/Library/Frameworks/MetalKit.framework/Versions/A/MetalKit",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "/usr/lib/libobjc.A.dylib",
	},
	"libonnxruntime.dylib": {
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/CoreML.framework/Versions/A/CoreML",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "/usr/lib/libiconv.2.dylib", "/usr/lib/libobjc.A.dylib",
	},
	"libsherpa-onnx-c-api.dylib": {
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib",
	},
	"libsherpa-onnx-cxx-api.dylib": {
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib", "@rpath/libsherpa-onnx-c-api.dylib",
	},
}

var ffmpegLinks = []string{
	"/System/Library/Frameworks/ApplicationServices.framework/Versions/A/ApplicationServices",
	"/System/Library/Frameworks/AudioToolbox.framework/Versions/A/AudioToolbox",
	"/System/Library/Frameworks/CoreAudio.framework/Versions/A/CoreAudio",
	"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
	"/System/Library/Frameworks/CoreMedia.framework/Versions/A/CoreMedia",
	"/System/Library/Frameworks/CoreServices.framework/Versions/A/CoreServices",
	"/System/Library/Frameworks/CoreText.framework/Versions/A/CoreText",
	"/System/Library/Frameworks/CoreVideo.framework/Versions/A/CoreVideo",
	"/System/Library/Frameworks/VideoToolbox.framework/Versions/A/VideoToolbox",
	"/usr/lib/libSystem.B.dylib", "/usr/lib/libiconv.2.dylib", "/usr/lib/libz.1.dylib",
}

// asks are the reasons Info.plist may give the person for what the app
// asks to reach: the four folders a video is opened from, and the
// microphone, for Dictate on Report a Problem. A new one, the camera or
// the screen, fails.
var asks = []string{
	"NSDesktopFolderUsageDescription", "NSDocumentsFolderUsageDescription",
	"NSDownloadsFolderUsageDescription", "NSRemovableVolumesUsageDescription",
	"NSMicrophoneUsageDescription",
}

// entitled are the entitlements a program may be signed with: the
// microphone, for Dictate, and nothing else. Above all not the camera,
// Apple events, or what lets other code into the app, library validation
// turned off, the dyld environment or get-task-allow.
var entitled = []string{"com.apple.security.device.audio-input"}

// checkInputs reads every file of the zip and says everything in it that
// could read input meant for other apps or watch the screen or the camera.
func checkInputs(zipPath string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the build is not a zip: %w", err)
	}
	defer z.Close()
	var found []string
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		for _, p := range inputsOf(f.Name, data) {
			found = append(found, f.Name+": "+p)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("the build could read input meant for other apps, or watch the screen or the camera, so it is not signed. If that is meant, the lists in cmd/framefairy-release/inputs.go change on main first, in a pull request that says why:\n  %s", strings.Join(found, "\n  "))
	}
	return nil
}

// inputsOf says what in one file of the build could read input, or
// nothing.
func inputsOf(name string, data []byte) []string {
	if path.Base(name) == "Info.plist" {
		keys, err := plistKeys(data)
		if err != nil {
			return []string{fmt.Sprintf("cannot be read: %v", err)}
		}
		var found []string
		for _, k := range keys {
			if strings.HasPrefix(k, "NS") && strings.HasSuffix(k, "UsageDescription") && !slices.Contains(asks, k) {
				found = append(found, "asks for "+k)
			}
		}
		return found
	}
	// Each architecture with its own bytes: the offsets inside one, like
	// where its signature is, count from the start of its slice.
	type arch struct {
		f    *macho.File
		data []byte
	}
	var files []arch
	if fat, err := macho.NewFatFile(bytes.NewReader(data)); err == nil {
		for _, a := range fat.Arches {
			end := uint64(a.Offset) + uint64(a.Size)
			if end > uint64(len(data)) {
				return []string{fmt.Sprintf("its %v part reaches past the end of the file", a.Cpu)}
			}
			files = append(files, arch{a.File, data[a.Offset:end]})
		}
	} else if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		files = append(files, arch{f, data})
	} else {
		return nil
	}
	var found []string
	for _, n := range watching {
		if bytes.Contains(data, []byte(n)) {
			found = append(found, "names "+n)
		}
	}
	for _, a := range files {
		found = append(found, machoInputs(path.Base(name), a.f, a.data)...)
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// machoInputs says what one program, or one architecture of it, imports,
// links or is entitled to that could read input.
func machoInputs(base string, f *macho.File, data []byte) []string {
	var found []string
	syms, err := f.ImportedSymbols()
	if err != nil {
		found = append(found, fmt.Sprintf("its imports cannot be read: %v", err))
	}
	for _, s := range syms {
		bare := strings.TrimPrefix(strings.TrimPrefix(s, "_"), "OBJC_CLASS_$_")
		if slices.Contains(watching, bare) {
			found = append(found, "imports "+s)
		}
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		found = append(found, fmt.Sprintf("its libraries cannot be read: %v", err))
	}
	known, ok := linked[base]
	for _, l := range libs {
		if !ok {
			found = append(found, "is a program the list of linked libraries does not know, and links "+l)
		} else if !slices.Contains(known, l) {
			found = append(found, "links "+l)
		}
	}
	keys, err := entitlements(f, data)
	if err != nil {
		found = append(found, fmt.Sprintf("its signature cannot be read: %v", err))
	}
	for _, k := range keys {
		if !slices.Contains(entitled, k) {
			found = append(found, "is entitled to "+k)
		}
	}
	return found
}

// entitlements are the keys of what the program was signed as entitled
// to, from the code signature codesign writes into it, or none when it
// was signed without any or not at all. The signature is a blob of blobs,
// big-endian whatever the program is, and the entitlements are the XML
// one, slot 5.
func entitlements(f *macho.File, data []byte) ([]string, error) {
	const codeSignature = 0x1d
	be := binary.BigEndian
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) < 16 || f.ByteOrder.Uint32(raw) != codeSignature {
			continue
		}
		off, size := uint64(f.ByteOrder.Uint32(raw[8:])), uint64(f.ByteOrder.Uint32(raw[12:]))
		if off+size > uint64(len(data)) || size < 12 {
			return nil, errors.New("it reaches past the end of the file")
		}
		sig := data[off : off+size]
		if be.Uint32(sig) != 0xfade0cc0 {
			return nil, errors.New("it is no blob of blobs")
		}
		count := uint64(be.Uint32(sig[8:]))
		if 12+8*count > size {
			return nil, errors.New("its index reaches past its end")
		}
		for i := range count {
			kind, at := be.Uint32(sig[12+8*i:]), uint64(be.Uint32(sig[16+8*i:]))
			if kind != 5 {
				continue
			}
			if at+8 > size || be.Uint32(sig[at:]) != 0xfade7171 {
				return nil, errors.New("its entitlements are not where its index says")
			}
			end := at + uint64(be.Uint32(sig[at+4:]))
			if end > size || end < at+8 {
				return nil, errors.New("its entitlements reach past its end")
			}
			return plistKeys(sig[at+8 : end])
		}
	}
	return nil, nil
}

// plistKeys are the keys of the dictionary at the top of a property list
// in XML, which is how Info.plist and the entitlements are written.
// Comments are no keys.
func plistKeys(data []byte) ([]string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	var keys []string
	depth := 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			return keys, nil
		}
		if err != nil {
			return nil, err
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			// plist, then dict, then the dictionary's own keys.
			if e.Name.Local == "key" && depth == 3 {
				var k string
				if err := d.DecodeElement(&k, &e); err != nil {
					return nil, err
				}
				keys = append(keys, strings.TrimSpace(k))
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
}
