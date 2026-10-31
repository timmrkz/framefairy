package tool

import "testing"

func TestShellQuoteLeavesWhatAShellLeavesAlone(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":              "''",
		"-i":            "-i",
		"/a/b.mp4":      "/a/b.mp4",
		"scale=1080:-2": "scale=1080:-2",
		"my video.mp4":  "'my video.mp4'",
		"it's":          `'it'"'"'s'`,
		"über":          "'über'",
		"a;rm -rf /":    "'a;rm -rf /'",
		"$HOME":         "'$HOME'",
	} {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}
