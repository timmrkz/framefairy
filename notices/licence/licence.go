// Package licence finds the licence a Go module or an npm package carries
// at its top, names it, and says whether a paid app may ship it. The build
// reads it through notices/gen for every module and package the programs
// are made of, so the one decision a person has to make, which licences
// are allowed, is made once, in MayShip.
package licence

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// Files are the names a licence goes by at the top of a module or
// package, in the order they are looked for.
var Files = []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "LICENCE.md", "COPYING", "LICENSE-MIT"}

// In is the name and the text of the licence at the top of dir.
func In(fsys afero.Fs, dir string) (string, []byte, error) {
	entries, err := afero.ReadDir(fsys, dir)
	if err != nil {
		return "", nil, err
	}
	// Some packages spell it in lower case.
	for _, want := range Files {
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), want) {
				body, err := afero.ReadFile(fsys, filepath.Join(dir, e.Name()))
				return e.Name(), body, err
			}
		}
	}
	return "", nil, fmt.Errorf("no licence file in %s", dir)
}

// Classify names the licence a text is, from the words every copy of that
// licence has. It is only used where the maker does not say.
func Classify(text string) string {
	t := strings.Join(strings.Fields(text), " ")
	switch {
	case strings.Contains(t, "Apache License") && strings.Contains(t, "Version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "Permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "Redistribution and use in source and binary forms") &&
		strings.Contains(t, "Neither the name"):
		return "BSD-3-Clause"
	case strings.Contains(t, "Redistribution and use in source and binary forms"):
		return "BSD-2-Clause"
	case strings.Contains(t, "Permission to use, copy, modify, and/or distribute"):
		return "ISC"
	case strings.Contains(t, "CC0 1.0 Universal"):
		return "CC0-1.0"
	case strings.Contains(t, "This is free and unencumbered software released into the public domain"):
		return "Unlicense"
	}
	return ""
}

// allowed are the licences a paid, closed app may ship a module or a
// package under, as long as its text goes with every copy. Each was read
// by a person. A licence is added here only after reading it, never to
// get a build through: the build stops on any other on purpose.
var allowed = map[string]bool{
	"MIT": true, "MIT-0": true, "ISC": true, "0BSD": true,
	"BSD-2-Clause": true, "BSD-3-Clause": true,
	"Apache-2.0": true, "Unlicense": true, "CC0-1.0": true,
}

// MayShip says whether a paid app may ship code under this SPDX licence
// or expression. Of "A OR B" one allowed is enough, of "A AND B" both
// have to be.
func MayShip(spdx string) bool {
	spdx = strings.TrimSpace(strings.Trim(strings.TrimSpace(spdx), "()"))
	if spdx == "" {
		return false
	}
	for _, either := range strings.Split(spdx, " OR ") {
		all := true
		for _, both := range strings.Split(either, " AND ") {
			all = all && allowed[strings.TrimSpace(strings.Trim(strings.TrimSpace(both), "()"))]
		}
		if all {
			return true
		}
	}
	return false
}
