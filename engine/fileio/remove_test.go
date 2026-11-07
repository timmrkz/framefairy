package fileio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// aWorkFolder makes a video and a work folder beside it in dir, with a
// file in the work folder and one in a folder inside it, and gives back
// the video and the work folder.
//
// On the real disk: for the tests of what the disk refuses or follows,
// a folder that may not be listed and a link.
func aWorkFolder(t *testing.T, dir string) (string, string) {
	t.Helper()
	video := filepath.Join(dir, "Screen Recording.mov")
	work := filepath.Join(dir, "Screen Recording.framefairy")
	if err := os.MkdirAll(filepath.Join(work, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		video:                                 "the video",
		filepath.Join(work, "added.json"):     `{"added":"2026-11-06T10:31:57Z"}`,
		filepath.Join(work, "logs", "a.json"): "{}",
		filepath.Join(work, "logs", "b.json"): "{}",
		filepath.Join(work, "thumbnail.webp"): "picture",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return video, work
}

// gone fails the test when anything is still at path.
//
// On the real disk: a link deleted is told from what it led to only there.
func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s is still there: %v", filepath.Base(path), err)
	}
}

// kept fails the test when the file at path is not there with its words.
//
// On the real disk: the file is what a link on the disk led to.
func kept(t *testing.T, path, body string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != body {
		t.Errorf("%s is %q, %v, and was %q", filepath.Base(path), got, err, body)
	}
}

// A screen recording dragged from its thumbnail in the corner of a Mac
// sits in a temporary folder of macOS's own, which the app may write in
// and may not list. Removing the video from the app deletes its work
// folder there, and the recording stays. Tim could not remove one: the
// delete opened the folder the work folder is in, and macOS refused.
//
// On the real disk: what is tested is a folder the system refuses to
// list. A folder that may be written and searched but not read stands
// for it, which the system refuses to open for listing to everybody but
// root, as macOS refuses its own.
func TestAWorkFolderIsDeletedFromAFolderThatCannotBeListed(t *testing.T) {
	dir := t.TempDir()
	video, work := aWorkFolder(t, dir)
	if err := os.Chmod(dir, os.ModeDir|0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, os.ModeDir|0o755) })
	if f, err := os.Open(dir); err == nil {
		f.Close()
		t.Skip("this process lists any folder, as root does, so no folder here stands for macOS's own")
	}

	if err := RemoveFolderIn(afero.NewOsFs(), work); err != nil {
		t.Fatalf("the work folder of a video in a folder that cannot be listed was not deleted: %v", err)
	}
	gone(t, work)
	kept(t, video, "the video")
}

// A link in the work folder, or a link where the work folder was, is
// deleted itself. What it leads to stays, whatever it is: the folder of
// the videos, another video's work, the user's home.
//
// On the real disk: the links that must not be followed are links on the
// disk, and memory has none.
func TestDeletingAWorkFolderFollowsNoLink(t *testing.T) {
	outside := t.TempDir()
	precious := filepath.Join(outside, "episode.mp4")
	if err := os.WriteFile(precious, []byte("someone's episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("in the folder", func(t *testing.T) {
		_, work := aWorkFolder(t, t.TempDir())
		if err := os.Symlink(outside, filepath.Join(work, "out")); err != nil {
			t.Skipf("links are not available here: %v", err)
		}
		if err := os.Symlink(precious, filepath.Join(work, "logs", "clips.json")); err != nil {
			t.Fatal(err)
		}
		if err := RemoveFolderIn(afero.NewOsFs(), work); err != nil {
			t.Fatal(err)
		}
		gone(t, work)
		kept(t, precious, "someone's episode")
	})

	t.Run("in its place", func(t *testing.T) {
		work := filepath.Join(t.TempDir(), "episode.framefairy")
		if err := os.Symlink(outside, work); err != nil {
			t.Skipf("links are not available here: %v", err)
		}
		if err := RemoveFolderIn(afero.NewOsFs(), work); err != nil {
			t.Fatal(err)
		}
		gone(t, work)
		kept(t, precious, "someone's episode")
	})
}

// A work folder that is not there is deleted already, so removing a video
// whose folder went in Finder still works.
//
// On the real disk: only there does the delete look for the folder
// itself, in memory it is afero's.
func TestAWorkFolderThatIsNotThereIsDeletedAlready(t *testing.T) {
	work := filepath.Join(t.TempDir(), "never.framefairy")
	if err := RemoveFolderIn(afero.NewOsFs(), work); err != nil {
		t.Error(err)
	}
}

// In memory the folder and everything in it goes, and what is beside it
// stays.
func TestAWorkFolderInMemoryIsDeleted(t *testing.T) {
	fsys := afero.NewMemMapFs()
	video, work := "/videos/ep.mp4", "/videos/ep.framefairy"
	for path, body := range map[string]string{video: "the video", work + "/logs/clips.json": "{}"} {
		if err := afero.WriteFile(fsys, path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveFolderIn(fsys, work); err != nil {
		t.Fatal(err)
	}
	if ok, _ := afero.Exists(fsys, work); ok {
		t.Error("the work folder is still there")
	}
	if got, err := afero.ReadFile(fsys, video); err != nil || string(got) != "the video" {
		t.Errorf("the video is %q, %v", got, err)
	}
}
