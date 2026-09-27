package main

import "testing"

// A pattern file is taken under the licence the app would most want of those
// it is offered under, and a file offered only under a licence a paid app
// cannot ship is refused. The headers are as hyph-utf8 writes them.
func TestAPatternFileIsTakenUnderALicenceTheAppMayShip(t *testing.T) {
	for _, c := range []struct {
		name, head, want string
		refused          bool
	}{
		{"one named licence, written out", "% licence:\n%     name: MIT\n%     text: >\n%         Permission is hereby granted\n", "MIT", false},
		{"a choice, LPPL first", "% licence:\n%     - This file is available under any of these licences:\n%     -\n%         name: LPPL\n%         version: 1.3\n%     -\n%         name: MIT\n", "MIT", false},
		{"LPPL alone", "% licence:\n%     name: LPPL\n%     version: 1\n%     or_later: true\n", "LPPL-1.3c", false},
		{"MPL besides the GPL", "% licence:\n%     - at your option:\n%     -\n%         name: MPL\n%         version: 1.1\n%     -\n%         name: GPL\n%         version: 2\n", "MPL-1.1", false},
		{"own terms nobody has read", "% licence:\n%     text: >\n%         Copying and distribution of this file are permitted.\n", "", true},
		{"the GPL alone", "% licence:\n%     name: GPL\n%     version: 2\n%     or_later: true\n", "", true},
		{"no licence", "% licence:\n%     - text: [None]\n", "", true},
		{"a licence it cannot tell", "% licence:\n%     name: Something Else\n", "", true},
	} {
		meta, err := readPatternMeta([]byte(c.head + "% ====\n% after the metadata: not yaml\n"))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := chooseLicence(meta.licences())
		if c.refused {
			if got != nil {
				t.Errorf("%s: taken under %q", c.name, got.spdx)
			}
			continue
		}
		if got == nil || got.spdx != c.want {
			t.Errorf("%s: taken under %+v, want %q", c.name, got, c.want)
		}
	}
}

// Terms a file writes out itself are taken once they have been read, and
// the same terms with one word changed are refused until they are read
// again.
func TestAFilesOwnTermsCountOnceRead(t *testing.T) {
	read := patternLicence{text: "Patterns may be freely distributed"}
	if got := chooseLicence([]patternLicence{read}); got == nil {
		t.Error("terms that were read are refused")
	}
	changed := patternLicence{text: "Patterns may be freely distributed for free"}
	if got := chooseLicence([]patternLicence{changed}); got != nil {
		t.Error("changed terms are taken without being read")
	}
}
