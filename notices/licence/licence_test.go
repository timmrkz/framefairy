package licence

import "testing"

// Every licence a module of the app is under is told from its text, so
// make notices never stops on one. malgo, which Report a Problem records
// with, is under the Unlicense, and make notices stopped on it.
func TestEveryLicenceTextOfAModuleIsTold(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"This is free and unencumbered software released into the public domain.\n\nAnyone is free to copy", "Unlicense"},
		{"Permission is hereby granted, free of charge, to any person", "MIT"},
		{"Permission to use, copy, modify, and/or distribute this software", "ISC"},
		{"Apache License\n  Version 2.0, January 2004", "Apache-2.0"},
		{"Some terms of its own", ""},
	} {
		if got := Classify(c.text); got != c.want {
			t.Errorf("%q told as %q, want %q", c.text, got, c.want)
		}
	}
}
