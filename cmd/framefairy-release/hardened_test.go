package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// devid is a program signed the way harden signs it: with
// a Developer ID of the team given, and the hardened runtime.
func devid(p program, team string) program {
	p.signed, p.flags, p.team = true, flagRuntime, team
	return p
}

// asMade is a program signed ad hoc, the way make signs it.
func asMade(p program) program {
	p.signed, p.flags = true, flagAdhoc
	return p
}

// speech is the speech library the app carries, as it links today.
var speech = program{links: []string{"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib"}}

const (
	exe = "Frame Fairy.app/Contents/MacOS/framefairy-app"
	lib = "Frame Fairy.app/Contents/Frameworks/libsherpa-onnx-c-api.dylib"
)

// Once build/team-id.txt names the user's team, a build is signed for updates
// only when every program in it is signed with the hardened runtime by
// that team. A build that lost it on the way is refused, and no entry is
// written for it, so no app is offered it. With no team named, a build
// signed ad hoc is signed as before.
//
// On the real disk: the release tool reads the zip and writes the entry as files.
func TestABuildWithoutTheHardenedRuntimeIsNotSigned(t *testing.T) {
	adhocRuntime := devid(app, "")
	adhocRuntime.flags |= flagAdhoc
	cases := []struct {
		name     string
		exe, lib program
		team     string
		want     string
	}{
		{"one team, the hardened runtime", devid(app, "TEAM123456"), devid(speech, "TEAM123456"), "TEAM123456", ""},
		{"signed ad hoc, as made, with no team named", asMade(adhoc), asMade(speech), "", ""},
		{"signed ad hoc, as made", asMade(adhoc), asMade(speech), "TEAM123456", exe + ": is signed ad hoc"},
		{"ad hoc with the hardened runtime", adhocRuntime, devid(speech, "TEAM123456"), "TEAM123456", exe + ": is signed ad hoc"},
		{"the app without the hardened runtime",
			func() program { p := devid(app, "TEAM123456"); p.flags = 0; return p }(),
			devid(speech, "TEAM123456"), "TEAM123456", exe + ": is signed without the hardened runtime"},
		{"a library left unsigned", devid(app, "TEAM123456"), speech, "TEAM123456", lib + ": is not signed"},
		{"a library of another team", devid(app, "TEAM123456"), devid(speech, "OTHER98765"), "TEAM123456", lib + ": is signed by team OTHER98765"},
		{"all of another team", devid(app, "OTHER98765"), devid(speech, "OTHER98765"), "TEAM123456", exe + ": is signed by team OTHER98765"},
		{"signed by no team", devid(app, ""), devid(speech, ""), "TEAM123456", exe + ": is signed by no team"},
		// macOS trusts an app of the team as the app it is signed as, with
		// that app's permissions and keychain items.
		{"the app signed as another app of the team",
			func() program { p := devid(app, "TEAM123456"); p.ident = "com.other.app"; return p }(),
			devid(speech, "TEAM123456"), "TEAM123456", exe + `: is signed as "com.other.app", not com.framefairy.app`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release(t)
			files := theApp("main")
			files[exe] = c.exe.bytes()
			// The library as harden signs it, a part of the app.
			l := c.lib
			if l.ident == "" {
				l.ident = codeIdentifier(lib)
			}
			files[lib] = l.bytes()
			zipPath := filepath.Join(t.TempDir(), "app.zip")
			if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "main.json")
			args := []string{"-zip", zipPath, "-channel", "main", "-name", "main", "-version", "0.1.0-main.abc1234",
				"-commit", "abc1234def56", "-url", "https://example.com/main.zip", "-out", out}
			// The team as main's build/team-id.txt names it, or no file.
			teamFile := filepath.Join(t.TempDir(), "team-id.txt")
			if c.team != "" {
				if err := os.WriteFile(teamFile, []byte(c.team+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args = append(args, "-team-file", teamFile)
			err := sign(args)
			_, written := os.Stat(out)
			if c.want == "" {
				if err != nil || written != nil {
					t.Errorf("signing it said %v, and the entry %v", err, written)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("signing it said %v, want %q", err, c.want)
			}
			if !os.IsNotExist(written) {
				t.Error("an entry was written for it")
			}
		})
	}
}

// Every program and library of a build is signed as a part of the app,
// never as another app. codesign would otherwise take what it signs a
// program as from the program's name or from an Info.plist built into
// it, which the branch writes, and macOS gives code of the user's team the
// keychain items of whatever app it is signed as.
//
// On the real disk: the release tool reads the zip and writes the entry as files.
func TestNoProgramIsSignedAsAnotherAppOfTheTeam(t *testing.T) {
	const server = "Frame Fairy.app/Contents/MacOS/llama-server"
	as := func(p program, ident string) program { p.ident = ident; return p }
	cases := []struct {
		name   string
		server program
		lib    program
		want   string
	}{
		{"each as a part of the app",
			as(devid(speech, "TEAM123456"), "com.framefairy.app.llama-server"),
			as(devid(speech, "TEAM123456"), "com.framefairy.app.libsherpa-onnx-c-api"), ""},
		{"llama-server as another app",
			as(devid(speech, "TEAM123456"), "com.other.app"),
			as(devid(speech, "TEAM123456"), "com.framefairy.app.libsherpa-onnx-c-api"),
			server + `: is signed as "com.other.app", not com.framefairy.app.llama-server`},
		{"llama-server as the app itself",
			as(devid(speech, "TEAM123456"), bundleID),
			as(devid(speech, "TEAM123456"), "com.framefairy.app.libsherpa-onnx-c-api"),
			server + `: is signed as "com.framefairy.app", not com.framefairy.app.llama-server`},
		{"a library by its name alone, as codesign signs it untold",
			as(devid(speech, "TEAM123456"), "com.framefairy.app.llama-server"),
			as(devid(speech, "TEAM123456"), "libsherpa-onnx-c-api"),
			lib + `: is signed as "libsherpa-onnx-c-api", not com.framefairy.app.libsherpa-onnx-c-api`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			release(t)
			files := theApp("main")
			files[exe] = devid(app, "TEAM123456").bytes()
			files[server] = c.server.bytes()
			files[lib] = c.lib.bytes()
			zipPath := filepath.Join(t.TempDir(), "app.zip")
			if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
				t.Fatal(err)
			}
			err := checkHardened(zipPath, "TEAM123456")
			if c.want == "" {
				if err != nil {
					t.Errorf("it said %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("it said %v, want %q", err, c.want)
			}
		})
	}
}

// The folders harden signs are found the way a Mac's disk finds them,
// whatever capitals they are spelled with, and nothing else is.
func TestWhereHardenSignsIsReadAsAMacReadsIt(t *testing.T) {
	cases := map[string]bool{
		"Frame Fairy.app/Contents/MacOS/llama-server":                  true,
		"Frame Fairy.app/contents/macos/llama-server":                  true,
		"Frame Fairy.app/Contents/Frameworks/libonnxruntime.dylib":     true,
		"Frame Fairy.app/CONTENTS/FRAMEWORKS/libonnxruntime.dylib":     true,
		"Frame Fairy.app/Contents/Frameworks/llama-server":             false,
		"Frame Fairy.app/Contents/Frameworks/sub/libonnxruntime.dylib": false,
		"Frame Fairy.app/Contents/Resources/llama-server":              false,
		"Frame Fairy.app/Contents/MacOS/sub/llama-server":              false,
	}
	for name, want := range cases {
		if got := signedHere(name); got != want {
			t.Errorf("%s: harden signs it %v, want %v", name, got, want)
		}
	}
}

// harden signs the programs in Contents/MacOS and the libraries in
// Contents/Frameworks. A program anywhere else would keep whatever
// signature the branch gave it, and the team a code directory names is
// only a string the branch can write, so such a program is refused
// however it reads.
//
// On the real disk: the release tool reads the zip and writes the entry as files.
func TestAProgramHardenDoesNotSignIsRefused(t *testing.T) {
	cases := []string{
		"Frame Fairy.app/Contents/Resources/llama-server",
		"Frame Fairy.app/Contents/Frameworks/sub/libonnxruntime.dylib",
		"Frame Fairy.app/Contents/Frameworks/llama-server",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			files := theApp("main")
			files[exe] = devid(app, "TEAM123456").bytes()
			p := devid(speech, "TEAM123456")
			p.ident = codeIdentifier(name)
			files[name] = p.bytes()
			zipPath := filepath.Join(t.TempDir(), "app.zip")
			if err := os.WriteFile(zipPath, zipped(t, files), 0o644); err != nil {
				t.Fatal(err)
			}
			err := checkHardened(zipPath, "TEAM123456")
			want := name + ": is a program outside what harden signs"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("it said %v, want %q", err, want)
			}
		})
	}
}
