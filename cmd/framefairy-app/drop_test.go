package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A drop on the app adds what Add would add: the video among the files,
// with its first search started, and every other file left out with its
// name. The same video dropped again is added no second time, and is the
// one to open, so the drop shows something rather than nothing.
func TestDropAddsTheVideosAndOpensOne(t *testing.T) {
	d := open(t)
	video := d.episode("gezogen", "12")
	notes := filepath.Join(d.home, "notizen.pdf")
	if err := os.WriteFile(notes, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(d.home, "ordner")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}

	got := d.svc.dropFiles([]string{notes, video, folder})
	if len(got.Added) != 1 || got.Added[0] != video {
		t.Fatalf("added %v, want only %s", got.Added, filepath.Base(video))
	}
	if got.Open != video {
		t.Errorf("opens %q, want %s", got.Open, filepath.Base(video))
	}
	if !strings.Contains(got.Error, "notizen.pdf and ordner are not videos") {
		t.Errorf("said %q, want it to name the files left out", got.Error)
	}
	if !slices.Contains(d.svc.store.Episodes(), video) {
		t.Errorf("%s is not in the library", filepath.Base(video))
	}
	if !slices.ContainsFunc(d.svc.Jobs(), func(j Job) bool { return j.Episode == video }) {
		t.Errorf("no first search started for %s, as Add starts one", filepath.Base(video))
	}

	again := d.svc.dropFiles([]string{video})
	if len(again.Added) != 0 || again.Open != video || again.Error != "" {
		t.Errorf("dropped again: %+v, want nothing added, no reason, and %s to open", again, filepath.Base(video))
	}
}

// What is left out is said in a sentence, one file or several.
func TestNotVideos(t *testing.T) {
	for _, c := range []struct {
		names []string
		want  string
	}{
		{[]string{"a.pdf"}, "a.pdf is not a video, so it was left out"},
		{[]string{"a.pdf", "b.png"}, "a.pdf and b.png are not videos, so they were left out"},
		{[]string{"a.pdf", "b.png", "c"}, "a.pdf, b.png and c are not videos, so they were left out"},
	} {
		if got := notVideos(c.names); got != c.want {
			t.Errorf("notVideos(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}
