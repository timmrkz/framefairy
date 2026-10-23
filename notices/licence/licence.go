// Package licence finds the licence a Go module or an npm package carries
// at its top, and names it. notices/gen reads it to write the notices, and
// the notices test reads it to check that the text a notice shows is
// still the text its module carries.
package licence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Files are the names a licence goes by at the top of a module or
// package, in the order they are looked for.
var Files = []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "LICENCE.md", "COPYING", "LICENSE-MIT"}

// In is the name and the text of the licence at the top of dir.
func In(dir string) (string, []byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil, err
	}
	// Some packages spell it in lower case.
	for _, want := range Files {
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(e.Name(), want) {
				body, err := os.ReadFile(filepath.Join(dir, e.Name()))
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
