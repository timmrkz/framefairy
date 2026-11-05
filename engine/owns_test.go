package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// A video says which paths are its own: itself and what is in its work
// folder, by the path's real place, and a plan only right in its logs
// folder. A path that only starts like the work folder, one that climbs
// out of it, and the files of another video are not its own.
func TestAVideoSaysWhichPathsAreItsOwn(t *testing.T) {
	fsys := afero.NewMemMapFs()
	m := NewMachine(fsys)
	video, other := "/v/folge.mp4", "/v/andere.mp4"
	for _, f := range []string{video, other} {
		if err := afero.WriteFile(fsys, f, []byte("video"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ep := m.Episode(video)
	work, logs := WorkDir(video), LogsDir(video)
	for _, c := range []struct {
		path      string
		owns, has bool
	}{
		{video, true, false},
		{filepath.Join(logs, "clips.json"), true, true},
		{filepath.Join(logs, "clips-0-1800.json"), true, true},
		{filepath.Join(work, "clips.json"), true, false},
		{filepath.Join(logs, "deeper", "clips.json"), true, false},
		{filepath.Join(work, "preview", "01.mp4"), true, false},
		{work, false, false},
		{work + "x/clips.json", false, false},
		{filepath.Join(logs, "..", "..", "andere.mp4"), false, false},
		{other, false, false},
		{filepath.Join(LogsDir(other), "clips.json"), false, false},
	} {
		if got := ep.Owns(c.path); got != c.owns {
			t.Errorf("Owns(%s) = %v, want %v", c.path, got, c.owns)
		}
		if got := ep.HasPlan(c.path); got != c.has {
			t.Errorf("HasPlan(%s) = %v, want %v", c.path, got, c.has)
		}
	}
}

// On the real disk, because what a link leads to is what is tested, and
// memory holds no links.
//
// A link inside the work folder is judged by where it leads, so one that
// leads out is not the video's. A video whose work folder is itself a
// link owns nothing, and says why.
func TestALinkIsJudgedByWhereItLeads(t *testing.T) {
	dir := t.TempDir()
	fsys := afero.NewOsFs()
	video := filepath.Join(dir, "folge.mp4")
	outside := filepath.Join(dir, "elsewhere")
	for _, d := range []string{LogsDir(video), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(video, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "clips.json")
	if err := os.WriteFile(secret, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(LogsDir(video), "clips-1.json")
	if err := os.Symlink(secret, out); err != nil {
		t.Fatal(err)
	}
	ep := NewMachine(fsys).Episode(video)
	if ep.Owns(out) || ep.HasPlan(out) {
		t.Error("a link in the logs folder that leads out of the work folder was taken as the video's")
	}
	if !ep.HasPlan(filepath.Join(LogsDir(video), "clips.json")) {
		t.Error("a plan in the logs folder is not the video's")
	}

	linked := filepath.Join(dir, "linked.mp4")
	if err := os.WriteFile(linked, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, WorkDir(linked)); err != nil {
		t.Fatal(err)
	}
	lep := NewMachine(fsys).Episode(linked)
	if lep.Linked() == "" {
		t.Error("a work folder that is a link was not said to be one")
	}
	if lep.Owns(linked) || lep.Owns(filepath.Join(WorkDir(linked), "clips.json")) {
		t.Error("a video whose work folder is a link owns its files")
	}
}
