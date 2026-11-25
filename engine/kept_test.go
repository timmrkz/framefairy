package engine

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"framefairy/engine/num"
	"framefairy/engine/timed"

	"github.com/spf13/afero"
)

// keptPlan is a clip set with a rendered clip and a word fixed in its
// captions, which is what the user wants back with a video added again.
const keptPlan = `{
  "source": "/videos/Folge 12.mp4",
  "clips": [
    {"id": "01", "slug": "a", "segments": [{"start": 0.01, "end": 0.18}],
     "corrections": [{"start": 0.05, "text": "Hallo"}]}
  ],
  "shorts": {"01": {"folder": "/videos/Folge 12.framefairy/out", "name": "hallo.mp4", "size": 1234, "written": "2026-11-20T10:00:00Z"}}
}
`

// keptVideo is a video in memory with a transcript of two words, its
// loudness and one clip set, on a machine that keeps its copies in
// /kept, and the machine.
func keptVideo(t *testing.T) (*Machine, string) {
	t.Helper()
	fsys := afero.NewMemMapFs()
	source := "/videos/Folge 12.mp4"
	if err := afero.WriteFile(fsys, source, bytes.Repeat([]byte("frame"), 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp, err := stampOf(fsys, source)
	if err != nil {
		t.Fatal(err)
	}
	logs := LogsDir(source)
	frames := make([]float32, 20)
	for i := range frames {
		frames[i] = float32(-30 - i)
	}
	file := transcriptFile{Version: transcriptVersion, Source: stamp, Model: "m", Mean: -25, End: 0.2,
		Parts: [][2]num.Float{{0, 0.2}},
		Words: storedWords([]timed.Cue{{Start: 0.02, End: 0.08, Text: "hallo"}, {Start: 0.1, End: 0.16, Text: "welt"}})}
	if err := writeTranscript(fsys, filepath.Join(logs, TranscriptName(nil)), file, frames); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fsys, filepath.Join(logs, "clips-0-1800.json"), []byte(keptPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewMachine(fsys)
	m.KeepIn("/kept")
	return m, source
}

// folderGone deletes the work folder of a video, the way Remove with its
// files deleted does, or Finder.
func folderGone(t *testing.T, m *Machine, source string) {
	t.Helper()
	if err := m.Files.RemoveAll(WorkDir(source)); err != nil {
		t.Fatal(err)
	}
}

// videoMoved moves a video to another folder under another name, with a
// new time, the way a video renamed, moved or downloaded again comes back.
func videoMoved(t *testing.T, m *Machine, source, to string) {
	t.Helper()
	if err := m.Files.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Files.Rename(source, to); err != nil {
		t.Fatal(err)
	}
	later := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	if err := m.Files.Chtimes(to, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestAVideoAddedAgainUnderAnotherNameGetsItsTranscriptAndClipsBack(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	plan, _ := afero.ReadFile(m.Files, filepath.Join(LogsDir(source), "clips-0-1800.json"))
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	folderGone(t, m, source)
	again := "/downloads/episode twelve.mp4"
	videoMoved(t, m, source, again)

	put, err := m.PutBack(again)
	if err != nil || !put {
		t.Fatalf("put back %v, %v, want true", put, err)
	}
	stamp, err := stampOf(m.Files, again)
	if err != nil {
		t.Fatal(err)
	}
	logs := LogsDir(again)
	file, _, frames, ok := readTranscriptFile(m.Files, filepath.Join(logs, TranscriptName(nil)), stamp, "m")
	if !ok {
		t.Fatal("the transcript put back is not the renamed video's: it would be transcribed again")
	}
	if got := fmt.Sprint(file.Words); got != "[{0.02 0.08 hallo} {0.1 0.16 welt}]" {
		t.Errorf("words put back %s", got)
	}
	if len(frames) != 20 || frames[19] != -49 {
		t.Errorf("loudness put back %v", frames)
	}
	if back, _ := afero.ReadFile(m.Files, filepath.Join(logs, "clips-0-1800.json")); !bytes.Equal(back, plan) {
		t.Errorf("the clip set came back changed\n%s\nwant\n%s", back, plan)
	}
}

func TestAWorkFolderDeletedInFinderLeavesTheCopyAsItWas(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	folderGone(t, m, source)
	// The watch hears the folder go, and keeping again finds nothing to
	// keep, which must not empty the copy.
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	if put, err := m.PutBack(source); err != nil || !put {
		t.Fatalf("put back %v, %v, want true", put, err)
	}
}

func TestPutBackLeavesAVideoWithWorkOfItsOwnAlone(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(LogsDir(source), "clips-0-1800.json")
	mine := []byte(`{"source": "x", "clips": []}`)
	if err := afero.WriteFile(m.Files, own, mine, 0o644); err != nil {
		t.Fatal(err)
	}
	if put, err := m.PutBack(source); err != nil || put {
		t.Fatalf("put back %v, %v, want false", put, err)
	}
	if got, _ := afero.ReadFile(m.Files, own); !bytes.Equal(got, mine) {
		t.Errorf("a clip set of its own was written over: %s", got)
	}
}

func TestKeepFollowsWhatChangesInTheWorkFolder(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(LogsDir(source), "clips-0-1800.json")
	if err := editPlan(m.Files, path, func(_ *object, clips []*object) error {
		clips[0].set("slug", "b")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	folderGone(t, m, source)
	if _, err := m.PutBack(source); err != nil {
		t.Fatal(err)
	}
	_, clips, err := LoadClips(m.Files, path)
	if err != nil {
		t.Fatal(err)
	}
	if clips[0].Slug != "b" {
		t.Errorf("the clip set came back as it was before the edit, slug %q", clips[0].Slug)
	}
}

func TestAClipSetThatDoesNotReadIsNotPutBack(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	folderGone(t, m, source)
	dir, ok := m.keptFor(source)
	if !ok {
		t.Fatal("no place for the copy")
	}
	if err := afero.WriteFile(m.Files, filepath.Join(dir, "clips-0-1800.json"), []byte(`{"clips": "no"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if put, err := m.PutBack(source); err != nil || !put {
		t.Fatalf("put back %v, %v, want the transcript back", put, err)
	}
	if exists, _ := afero.Exists(m.Files, filepath.Join(LogsDir(source), "clips-0-1800.json")); exists {
		t.Error("a clip set that does not read as a plan was put back")
	}
}

func TestAMachineWithNoPlaceKeepsNothing(t *testing.T) {
	t.Parallel()
	m, source := keptVideo(t)
	m.KeepIn("")
	if err := m.Keep(source); err != nil {
		t.Fatal(err)
	}
	if exists, _ := afero.Exists(m.Files, "/kept"); exists {
		t.Error("a machine with no place for copies wrote one")
	}
	folderGone(t, m, source)
	if put, err := m.PutBack(source); err != nil || put {
		t.Fatalf("put back %v, %v, want false", put, err)
	}
}

func TestOnlyAKeyOfTheShapeEpisodeKeyMakesIsAFolder(t *testing.T) {
	t.Parallel()
	for key, want := range map[string]bool{
		"3f9a1c7e04b2d8a1":  true,
		"":                  false,
		"3F9A1C7E04B2D8A1":  false,
		"3f9a1c7e04b2d8a":   false,
		"../../../../etc1":  false,
		"3f9a1c7e04b2d8a1x": false,
	} {
		if got := validKey(key); got != want {
			t.Errorf("validKey(%q) = %v, want %v", key, got, want)
		}
	}
}
