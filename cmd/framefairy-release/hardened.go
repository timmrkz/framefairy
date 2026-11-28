package main

// A build the user updates to is signed with their Developer ID and the
// hardened runtime, see devid.go and docs/UPDATES.md. Once
// build/team-id.txt names their team, sign reads it there and refuses a
// build that lost it on the way: a program signed ad hoc,
// without the hardened runtime, or by another team. Under the hardened
// runtime macOS loads only libraries of the app's own team, so a library
// of another would not even open.

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
)

// The flags of a code directory that say how a program was signed.
const (
	flagAdhoc   = 0x2
	flagRuntime = 0x10000
)

// signing is how one program, or one architecture of it, was signed.
type signing struct {
	signed bool
	flags  uint32
	team   string
	// ident is what the program is signed as, com.framefairy.app for
	// the app.
	ident string
}

// signingOf reads the code directory codesign writes into slot 0 of the
// signature: its flags, at 12, the offset of its identifier, at 20, and
// from version 0x20200 on the offset of its team's name, at 48, all
// big-endian.
func signingOf(a arch) (signing, error) {
	cd, err := signatureBlob(a.f, a.data, 0, 0xfade0c02)
	if err != nil || cd == nil {
		return signing{}, err
	}
	be := binary.BigEndian
	if len(cd) < 44 {
		return signing{}, errors.New("its code directory is too short")
	}
	s := signing{signed: true, flags: be.Uint32(cd[12:])}
	name, err := cString(cd, be.Uint32(cd[20:]))
	if err != nil {
		return signing{}, fmt.Errorf("its identifier: %w", err)
	}
	s.ident = name
	if be.Uint32(cd[8:]) >= 0x20200 && len(cd) >= 52 {
		if at := be.Uint32(cd[48:]); at != 0 {
			name, err := cString(cd, at)
			if err != nil {
				return signing{}, fmt.Errorf("its team: %w", err)
			}
			s.team = name
		}
	}
	return s, nil
}

// cString is the name that starts at offset at of a code directory and
// ends with a zero byte.
func cString(cd []byte, at uint32) (string, error) {
	if at == 0 || uint64(at) >= uint64(len(cd)) {
		return "", errors.New("it is not where the code directory says")
	}
	name, _, found := strings.Cut(string(cd[at:]), "\x00")
	if !found {
		return "", errors.New("it has no end")
	}
	return name, nil
}

// codeIdentifier is what a program or library of the build is signed
// as, by the name of its file: com.framefairy.app for the app's own
// program, and a part of it for everything it carries, like
// com.framefairy.app.llama-server. Told nothing, codesign takes it from
// the file's name or from an Info.plist built into the file, both of
// which the branch writes, and macOS trusts code of the user's team as
// whatever app it is signed as, with that app's keychain items.
func codeIdentifier(name string) string {
	base := path.Base(filepath.ToSlash(name))
	if base == appProgram {
		return bundleID
	}
	return bundleID + "." + strings.TrimSuffix(base, ".dylib")
}

// signedHere says whether a program at name in the build's zip is one
// harden signs itself: one in Contents/MacOS, or a library in
// Contents/Frameworks, see innerCode. A Mac's disk does not tell capitals
// from small letters in the folders' names.
func signedHere(name string) bool {
	dir, base := path.Split(name)
	dir = strings.TrimSuffix(dir, "/")
	return strings.EqualFold(dir, appName+"/Contents/MacOS") ||
		strings.EqualFold(dir, appName+"/Contents/Frameworks") && strings.HasSuffix(base, ".dylib")
}

// checkHardened reads every program of the zip and says each that is not
// signed with the hardened runtime by the team given, as its part of the
// app.
func checkHardened(zipPath, team string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the build is not a zip: %w", err)
	}
	defer z.Close()
	var found []string
	programs := 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > mostUnpacked {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		data, err := io.ReadAll(io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		arches, err := archesOf(data)
		if err != nil {
			found = append(found, f.Name+": "+err.Error())
			continue
		}
		for _, a := range arches {
			s, err := signingOf(a)
			switch {
			case err != nil:
				found = append(found, fmt.Sprintf("%s: its signature cannot be read: %v", f.Name, err))
			case !s.signed:
				found = append(found, f.Name+": is not signed")
			case s.flags&flagAdhoc != 0:
				found = append(found, f.Name+": is signed ad hoc")
			case s.flags&flagRuntime == 0:
				found = append(found, f.Name+": is signed without the hardened runtime")
			case s.team == "":
				found = append(found, f.Name+": is signed by no team")
			case s.team != team:
				found = append(found, f.Name+": is signed by team "+s.team)
			case !signedHere(f.Name):
				found = append(found, f.Name+": is a program outside what harden signs, Contents/MacOS and the libraries in Contents/Frameworks")
			case s.ident != codeIdentifier(f.Name):
				found = append(found, fmt.Sprintf("%s: is signed as %q, not %s", f.Name, s.ident, codeIdentifier(f.Name)))
			default:
				programs++
			}
		}
	}
	if programs == 0 && len(found) == 0 {
		found = append(found, "it holds no program")
	}
	if found = tidy(found); len(found) > 0 {
		return fmt.Errorf("the build is not signed with the Developer ID of team %s and the hardened runtime, so it is not signed for updates. See devid.go:\n  %s", team, strings.Join(found, "\n  "))
	}
	return nil
}
