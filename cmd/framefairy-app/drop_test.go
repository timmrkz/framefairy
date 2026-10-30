package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// A drop on the app adds what Add would add: the video among the files,
// with its first search started, and every other file left out with its
// name. The same video dropped again is added no second time, and is the
// one to open, so the drop shows something rather than nothing.
//
// On the real disk: the frames decoder reads the video dropped.
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
	if !strings.Contains(got.Error, "notizen.pdf and ordner cannot be added because they are not videos") {
		t.Errorf("said %q, want it to name the files it cannot add", got.Error)
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
		{[]string{"a.pdf"}, "a.pdf cannot be added because it is not a video"},
		{[]string{"a.pdf", "b.png"}, "a.pdf and b.png cannot be added because they are not videos"},
		{[]string{"a.pdf", "b.png", "c"}, "a.pdf, b.png and c cannot be added because they are not videos"},
	} {
		if got := notVideos(c.names); got != c.want {
			t.Errorf("notVideos(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}

// A drop of nothing but other files adds nothing and opens nothing, and
// says why. A video whose picture cannot be decoded, dropped with them,
// is left out in the same sentence, the way Add leaves it out.
//
// On the real disk: the frames decoder reads kaputt.mp4 and finds no picture.
func TestDropOfNoVideoSaysWhy(t *testing.T) {
	d := open(t)
	notes := filepath.Join(d.home, "notizen.pdf")
	broken := filepath.Join(d.home, "kaputt.mp4")
	for _, f := range []string{notes, broken} {
		if err := os.WriteFile(f, []byte("no video at all"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := d.svc.dropFiles([]string{notes})
	if len(got.Added) != 0 || got.Open != "" || got.Error != "notizen.pdf cannot be added because it is not a video" {
		t.Errorf("a PDF dropped: %+v, want nothing added, nothing to open, and why", got)
	}
	got = d.svc.dropFiles([]string{broken, notes})
	if len(got.Added) != 0 || got.Open != "" {
		t.Errorf("added %v and opens %q, want neither", got.Added, got.Open)
	}
	for _, want := range []string{"notizen.pdf cannot be added because it is not a video", "the picture of kaputt.mp4 cannot be decoded"} {
		if !strings.Contains(got.Error, want) {
			t.Errorf("said %q, want it to say %q", got.Error, want)
		}
	}
	if eps := d.svc.store.Episodes(); len(eps) != 0 {
		t.Errorf("the library holds %v", eps)
	}
}

// Dropped with a video already in the list, the one just added is the
// one that opens: it is the one the person expects to see arrive.
func TestDropOpensTheVideoAdded(t *testing.T) {
	d := open(t)
	old := d.episode("alt", "12")
	if got := d.svc.dropFiles([]string{old}); got.Open != old {
		t.Fatalf("opens %q, want %s", got.Open, filepath.Base(old))
	}
	fresh := d.episode("neu", "12")
	if got := d.svc.dropFiles([]string{old, fresh}); got.Open != fresh {
		t.Errorf("opens %q, want %s, the one just added", got.Open, filepath.Base(fresh))
	}
}

// Every drop is taken on a goroutine of its own, beside Add and the
// queue, so many at once, while the window reads the jobs and the
// library, put each video in the library once and start one first
// search for it.
func TestDropsAtOnce(t *testing.T) {
	d := open(t)
	videos := []string{d.episode("eins", "12"), d.episode("zwei", "12")}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			got := d.svc.takeFiles([]string{"drop", "add"}[i%2], []string{videos[i%2], videos[(i+1)%2]})
			if got.Error != "" {
				t.Errorf("a drop said %q", got.Error)
			}
		})
		wg.Go(func() {
			_ = d.svc.Jobs()
			_ = d.svc.Library()
		})
	}
	wg.Wait()
	library := d.svc.store.Episodes()
	for _, v := range videos {
		if n := len(slices.DeleteFunc(slices.Clone(library), func(p string) bool { return p != v })); n != 1 {
			t.Errorf("%s is in the library %d times", filepath.Base(v), n)
		}
		searches := 0
		for _, j := range d.svc.Jobs() {
			if j.Episode == v && j.Kind == "search" {
				searches++
			}
		}
		if searches != 1 {
			t.Errorf("%s has %d first searches, want 1", filepath.Base(v), searches)
		}
	}
}

// A drop whose adding panics says so as the drop's error, which the
// window shows as it shows a file left out, and the stack is in the app's
// log. The drop is a goroutine of its own, and a panic in it ended the
// whole app, issue #604. A service with no library makes the adding
// panic, the shortest way there is to one.
//
// On the real disk: lumberjack writes the log and knows only the disk.
func TestADropThatPanicsSaysWhyNotTheAppGoing(t *testing.T) {
	_, read := testLog(t)
	got := (&FrameFairy{}).dropFiles([]string{"/home/tim/notes.txt"})
	if len(got.Added) != 0 || !strings.HasPrefix(got.Error, "the files could not be added, ") || strings.Contains(got.Error, "\n") {
		t.Errorf("the drop that panicked gave %+v, want its reason in one line", got)
	}
	line := find(read(), "adding the files stopped unexpectedly")
	has(t, line, map[string]any{"level": "error", "from": "app", "how": "drop"})
	if stack, _ := line["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("the app's log holds no stack of the panic, %v", line)
	}
}
