package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/internal/fixtures"
	"framefairy/internal/standin"
)

// Add keeps a file only where the episode's decoder hands over its first
// frame. Every video given is added and answered at once, so the window
// opens it as it lands, and checked behind that: a file with no picture,
// and one that is no video at all, go out of the library again with the
// reason, and a video beside them stays. The video preview would have
// nothing to show of them and the render nothing to cut. Step 3 of
// docs/VIDEO-PREVIEW.md, and issue #832 for the order.
//
// On the real disk: the episode's decoder reads what is added.
func TestAddLeavesOutWhatItsDecoderCannotDecode(t *testing.T) {
	d := open(t)
	told := hearDrops(d.svc)
	good := d.episode("gut", "12")
	sound := fixtures.Copy(t, "tone-then-silence.m4a", filepath.Join(d.home, "nur-ton.mp4"))
	broken := filepath.Join(d.home, "kaputt.mp4")
	if err := os.WriteFile(broken, []byte("no video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := d.svc.addEpisodes([]string{sound, good, broken})
	if err != nil || len(added) != 3 {
		t.Fatalf("added %v and said %v, want all three at once and nothing said yet", added, err)
	}
	for _, v := range added {
		<-d.svc.checked(v)
	}
	said := ""
	for _, a := range told() {
		said += a.Error + "\n"
	}
	for _, name := range []string{"nur-ton.mp4", "kaputt.mp4"} {
		if !strings.Contains(said, "the picture of "+name+" cannot be decoded") {
			t.Errorf("the window was told %q, want it to say why %s was left out", said, name)
		}
	}
	if !strings.Contains(said, "has no picture") {
		t.Errorf("the window was told %q, want ffmpeg's reason for the file of sound, that it has no picture", said)
	}
	if got := d.svc.store.Episodes(); len(got) != 1 || got[0] != good {
		t.Errorf("the library holds %v, want only %s", got, filepath.Base(good))
	}
	if !slices.ContainsFunc(d.svc.Jobs(), func(j Job) bool { return j.Episode == good }) {
		t.Errorf("no first search started for %s", filepath.Base(good))
	}
}

// A video taken out again because its picture cannot be decoded takes
// only the folder its own add made. A folder that was there before, kept
// when the video was removed with its work, stays with all that is in it.
//
// On the real disk: the episode's decoder reads what is added.
func TestAVideoTakenOutAgainKeepsAFolderItHadBefore(t *testing.T) {
	d := open(t)
	broken := filepath.Join(d.home, "kaputt.mp4")
	if err := os.WriteFile(broken, []byte("no video at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(engine.WorkDir(broken), "notes.txt")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("from before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.svc.addEpisodes([]string{broken}); err != nil {
		t.Fatal(err)
	}
	<-d.svc.checked(broken)
	if eps := d.svc.store.Episodes(); len(eps) != 0 {
		t.Errorf("the library holds %v", eps)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("what was in the folder before is gone, %v", err)
	}
}

// A video whose first frame does not come in time is taken, not left out.
// On CI's Mac, with its media engine hung, three tests had a good video
// left out after 30 s as "the picture cannot be decoded, context deadline
// exceeded". A wait says nothing about the file, only about the machine,
// and only the decoder's own words leave a video out. Plan row 2.233.
//
// The decoder is a stand-in that takes every request and answers none,
// the way one hung in the media engine does.
//
// On the real disk: the stand-in decoder is a program that is run. And
// FRAMEFAIRY_FRAMES names it for the whole process, which open does
// already with HOME, so the test does not run beside others.
func TestAddTakesAVideoWhoseFirstFrameIsLate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	d := open(t)
	good := d.episode("spaet", "6")
	// It says when it hangs, and Add takes the first frame to be late
	// then, rather than after 30 s of the clock.
	pipe, hangs := standin.Hangs(t)
	hung := filepath.Join(t.TempDir(), "framefairy-frames")
	if err := standin.Write(hung, []byte("#!/bin/sh\necho hangs > '"+pipe+"'\nexec cat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRAMEFAIRY_FRAMES", hung)
	abs, _ := filepath.Abs(good)
	t.Cleanup(func() {
		d.svc.previews.mu.Lock()
		dec := d.svc.previews.decoders[abs]
		delete(d.svc.previews.decoders, abs)
		d.svc.previews.mu.Unlock()
		if dec != nil {
			dec.Close()
		}
	})
	d.svc.frameLate = hangs
	added, err := d.svc.addEpisodes([]string{good})
	if err != nil {
		t.Fatalf("Add said %v", err)
	}
	if len(added) != 1 || added[0] != good {
		t.Fatalf("added %v, want %s", added, filepath.Base(good))
	}
	<-d.svc.checked(good)
	if got := d.svc.store.Episodes(); len(got) != 1 || got[0] != good {
		t.Errorf("the library holds %v once its first frame was late, want %s", got, filepath.Base(good))
	}
}

// A video removed while its picture is still being checked has the check
// called off, and nothing the check would have started outlives the
// video: no first search, nothing taken out of a library it is added to
// again. One test removed a video and added it again while the check of
// the add before was still running, and that check started a search of
// the video being removed, which failed.
//
// The decoder is a stand-in that takes every request and answers none,
// so the check is held where a slow first frame holds it.
//
// On the real disk: the stand-in decoder is a program that is run. And
// FRAMEFAIRY_FRAMES names it for the whole process, which open does
// already with HOME, so the test does not run beside others.
func TestAVideoRemovedWhileItsPictureIsCheckedStartsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	d := open(t)
	video := d.episode("weg", "6")
	pipe, hangs := standin.Hangs(t)
	hung := filepath.Join(t.TempDir(), "framefairy-frames")
	if err := standin.Write(hung, []byte("#!/bin/sh\necho hangs > '"+pipe+"'\nexec cat >/dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRAMEFAIRY_FRAMES", hung)
	abs, _ := filepath.Abs(video)
	t.Cleanup(func() {
		d.svc.previews.mu.Lock()
		dec := d.svc.previews.decoders[abs]
		delete(d.svc.previews.decoders, abs)
		d.svc.previews.mu.Unlock()
		if dec != nil {
			dec.Close()
		}
	})
	if _, err := d.svc.addEpisodes([]string{video}); err != nil {
		t.Fatal(err)
	}
	<-hangs
	if err := d.svc.RemoveEpisode(abs, true); err != nil {
		t.Fatalf("removing the video while its picture was checked: %v", err)
	}
	select {
	case <-d.svc.checked(abs):
	default:
		t.Error("the check still runs after the video was removed")
	}
	if eps := d.svc.store.Episodes(); len(eps) != 0 {
		t.Errorf("the library holds %v", eps)
	}
	for _, j := range d.svc.Jobs() {
		if j.Episode == abs {
			t.Errorf("the video removed has a %s, %s", j.Kind, j.State)
		}
	}
}

// A label given in the sidebar comes back with the library, and the video
// keeps its name and its bytes. A label inside a video's own work folder
// is no video of the library, so it is refused.
func TestALabelComesBackWithTheLibrary(t *testing.T) {
	files := afero.NewMemMapFs()
	video := "/home/tim/podcast/015 Ben/youtube.mp4"
	if err := afero.WriteFile(files, video, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := openStoreIn(files, "/home/tim/config/FrameFairy")
	if _, err := st.AddEpisodes([]string{video}); err != nil {
		t.Fatal(err)
	}
	svc := &FrameFairy{store: st}
	l, err := svc.SetLabel(video, "Ben", "#0a64d6")
	if err != nil || l == nil || l.Text != "Ben" {
		t.Fatalf("SetLabel gave %+v, %v", l, err)
	}
	lib := svc.Library()
	if len(lib) != 1 || lib[0].Label == nil || *lib[0].Label != *l || lib[0].Name != "youtube.mp4" {
		t.Fatalf("the library says %+v", lib)
	}
	if _, err := svc.SetLabel(filepath.Join(engine.WorkDir(video), "added.json"), "x", "#0a64d6"); err == nil {
		t.Error("a file in the work folder took a label")
	}
	if l, err := svc.SetLabel(video, "", "#0a64d6"); err != nil || l != nil {
		t.Fatalf("taking the label away gave %+v, %v", l, err)
	}
	if lib := svc.Library(); len(lib) != 1 || lib[0].Label != nil {
		t.Fatalf("the label taken away is still there: %+v", lib)
	}
	if body, _ := afero.ReadFile(files, video); string(body) != "not really a video" {
		t.Fatalf("the video became %q", body)
	}
}
