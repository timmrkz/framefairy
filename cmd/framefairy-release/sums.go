package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The app runs a tool beside it only when the file is the one it was
// built with: make builds the SHA-256 of each tool into the app's own
// program, name=sum, see Find in engine/tool/find.go. Signing a tool
// with the Developer ID writes a new signature into it, and with that it
// is a file of another SHA-256. So the sums are written again, once the
// tools are signed and before the app is, and the app's own signature
// then covers the sums it carries.
//
// It was left out once, and every build signed with the Developer ID
// refused all of its tools: no transcription, no video preview, no
// search.

// toolSums is the SHA-256 of each program in Contents/MacOS but the
// app's own, by name.
func toolSums(app string) (map[string]string, error) {
	programs, err := filepath.Glob(filepath.Join(app, "Contents", "MacOS", "*"))
	if err != nil {
		return nil, err
	}
	sums := map[string]string{}
	for _, p := range programs {
		name := filepath.Base(p)
		if name == appProgram {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		sums[name] = hex.EncodeToString(sum[:])
	}
	return sums, nil
}

// resum writes into the app's own program the sum of each tool as it is
// now, in place of the sum built, which is each tool's before it was
// signed. A sum is 64 characters either way, so nothing in the program
// moves. A tool whose sum is not in the program is refused, because the
// app would refuse it too and the build would go out without it.
func resum(app string, built map[string]string) error {
	now, err := toolSums(app)
	if err != nil {
		return err
	}
	own := filepath.Join(app, "Contents", "MacOS", appProgram)
	data, err := os.ReadFile(own)
	if err != nil {
		return err
	}
	var missing []string
	for _, name := range slices.Sorted(maps.Keys(built)) {
		before := []byte(name + "=" + built[name])
		if !bytes.Contains(data, before) {
			missing = append(missing, name)
			continue
		}
		after := []byte(name + "=" + now[name])
		if len(after) != len(before) {
			return fmt.Errorf("%s is gone from %s", name, filepath.Dir(own))
		}
		data = bytes.ReplaceAll(data, before, after)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s was not built with the SHA-256 of %s as it was before signing, and would run none of them", own, strings.Join(missing, ", "))
	}
	info, err := os.Stat(own)
	if err != nil {
		return err
	}
	return os.WriteFile(own, data, info.Mode().Perm())
}
