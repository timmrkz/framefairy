package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/spf13/afero"
)

// The app searches by itself only for an episode nobody has ever searched.
// What says so lives with the episode, so it survives a restart, it is not
// undone by removing the clips again, and deleting the work folder makes
// the episode new.
func TestAnEpisodeRemembersItHasBeenSearched(t *testing.T) {
	fsys, source := afero.NewMemMapFs(), "/home/tim/ep.mp4"
	if err := afero.WriteFile(fsys, source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if EverSearched(fsys, source) {
		t.Fatal("an episode nobody has searched should not say it has been")
	}
	if st := Status(fsys, source, "", nil); st.EverSearched || st.Work {
		t.Errorf("status of a fresh episode %+v", st)
	}

	// Clips made by hand search nothing: neither their set nor their
	// jobs' timings say the episode was searched.
	p := NewProject(NewEngine(NewLog(io.Discard, false, false), fsys), source, DefaultOptions())
	hand := PlanFile{Source: "ep.mp4", PlannedWith: PlannedWith{By: ByHand}, Clips: []PlanClip{{ID: "h01",
		Slug: "hand", Keep: [][2]int{{1, 1}},
		Segments: []PlanSegment{{Start: 1, End: 20, CropX: "center"}}}}}
	if _, err := addClip(fsys, p.HandPlanPath(), hand, hand.Clips[0], "h"); err != nil {
		t.Fatal(err)
	}
	p.addTimings(JobRecord{ID: "clip-1", Kind: JobClip})
	if EverSearched(fsys, source) {
		t.Fatal("clips made by hand made the episode searched")
	}

	// A search that was asked for keeps its record, and its timings once
	// it is done, and either is enough.
	if err := WriteJob(fsys, source, JobRecord{ID: SearchID, Kind: JobSearch, Step: StepWaiting}); err != nil {
		t.Fatal(err)
	}
	if !EverSearched(fsys, source) {
		t.Fatal("an episode being searched says it was not searched")
	}
	if err := RemoveJob(fsys, source, SearchID); err != nil {
		t.Fatal(err)
	}
	p.addTimings(JobRecord{ID: SearchID, Kind: JobSearch})
	if !EverSearched(fsys, source) {
		t.Fatal("an episode that was searched says it was not")
	}
	if st := Status(fsys, source, "", nil); !st.EverSearched || !st.Work {
		t.Errorf("status after a search %+v", st)
	}

	if err := DeleteWork(fsys, source); err != nil {
		t.Fatal(err)
	}
	if EverSearched(fsys, source) {
		t.Error("an episode whose work folder is gone is new again")
	}
}

// A short is a file a render wrote and noted in the plan. One still being
// written is not a short, and neither is a file in the out folder no clip
// is named for, nor one of a clip's name that the plan says nothing of.
//
// On the real disk: a short is a file ffmpeg wrote, looked for on the disk.
func TestAShortBeingWrittenIsNotCounted(t *testing.T) {
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(WorkDir(source), "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(LogsDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(LogsDir(source), "clips.json")
	if err := os.WriteFile(plan, []byte(`{"clips": [
  {"id": "01", "slug": "done", "segments": [{"start": 1, "end": 21}]},
  {"id": "02", "slug": "half", "segments": [{"start": 30, "end": 50}]},
  {"id": "03", "slug": "upper", "segments": [{"start": 60, "end": 80}]},
  {"id": "05", "slug": "unnoted", "segments": [{"start": 90, "end": 110}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"done.mp4", "half.part.mp4", "upper.PART.MP4", "gone.mp4", "unnoted.mp4"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Noted the way a render notes its short once it is written.
	if err := recordShort(disk, plan, "01", filepath.Join(out, "done.mp4")); err != nil {
		t.Fatal(err)
	}
	if got := Status(disk, source, "", nil).Rendered; got != 1 {
		t.Errorf("%d rendered, want 1", got)
	}
}

// Where a short goes and whose it is needs no picture. ffmpeg writes the
// short, and everything around it, the folder, the name, the note in the
// plan, is the engine's, so the tests below lay out the files by hand
// and ask the engine. One test, TestARenderPutsItsShortWhereItsNameSays,
// renders for real, to prove a render goes by the same rules.

// episodeWithPlan is an episode of the given file name in a folder of its
// own, with a plan of one clip of the words wort and a picture taken at
// 2 s, and gives the episode and its plan. Nothing is a video: where
// shorts go is decided by names alone.
//
// On the real disk: the shorts beside it are judged by what is on the
// disk, see IsShort.
func episodeWithPlan(t *testing.T, name string) (source, plan string) {
	t.Helper()
	source = filepath.Join(ResolvePath(t.TempDir()), name+".mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(LogsDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	plan = filepath.Join(LogsDir(source), "clips.json")
	if err := os.WriteFile(plan, []byte(`{"clips": [
  {"id": "01", "slug": "wort", "segments": [{"start": 1, "end": 21}], "thumbnails": [2]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return source, plan
}

// placeShort does what a render does around ffmpeg, see Run: the short of
// clip id goes into dir under the name shortStem gives it, with a picture
// beside it for each the clip asks for, see WriteThumbnails, and the plan
// notes where it went. What ffmpeg would write is a few bytes.
//
// On the real disk: a short is a file on the disk, see IsShort.
func placeShort(t *testing.T, plan, dir, id string) string {
	t.Helper()
	loaded, clips, err := LoadClips(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(clips, func(c Clip) bool { return c.ID == id })
	if i < 0 {
		t.Fatalf("the plan has no clip %s", id)
	}
	stem := shortStem(dir, loaded, clips[i])
	short := filepath.Join(dir, stem+".mp4")
	written := []string{short}
	for n := range clips[i].Thumbnails {
		written = append(written, filepath.Join(dir, thumbnailName(stem, n+1)))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range written {
		if err := os.WriteFile(path, []byte("a short"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := recordShort(disk, plan, id, short); err != nil {
		t.Fatal(err)
	}
	return short
}

// shortOfClip is what the plan says of the short of clip id.
//
// On the real disk: the plan is beside the shorts, which are judged by
// what is on the disk, see shortOf.
func shortOfClip(t *testing.T, plan, id string) ClipView {
	t.Helper()
	view, err := ReadPlan(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range view.Clips {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("the plan has no clip %s", id)
	return ClipView{}
}

// The shorts go into the folder --out names, as it is. Without it they go
// into a folder of the episode's own name in the folder the app's settings
// name for shorts, so two episodes never meet there, and without that
// into the episode's own work folder.
//
// On the real disk: the folder for shorts is judged by where it really
// leads, see SafeChild, and on a Mac /home leads elsewhere.
func TestTheShortsGoWhereTheSettingsSay(t *testing.T) {
	t.Parallel()
	source := "/home/tim/Podcast/folge.mp4"
	shorts := ResolvePath(t.TempDir())
	for _, c := range []struct {
		name, out, shorts, want string
	}{
		{"--out", "/tmp/out", shorts, "/tmp/out"},
		{"the folder for shorts", "", shorts, filepath.Join(shorts, "folge")},
		{"neither", "", "", "/home/tim/Podcast/folge.framefairy/out"},
	} {
		got, err := shortsFolder(c.out, c.shorts, source)
		if err != nil || got != c.want {
			t.Errorf("%s: the shorts go to %q, %v, not %q", c.name, got, err, c.want)
		}
	}
	if got, err := shortsFolder("", shorts, "/home/tim/.."); err == nil {
		t.Errorf("an episode named .. puts its shorts in %s, outside the folder for shorts", got)
	}
}

// A short is the clip's when the plan notes it and it is still the file
// that was noted. The clip says so, and so does the episode, and the app
// may show it, see IsShort. A file of the same name another episode put in
// the folder is not the clip's, and the short gets a number rather than
// write over it, slug 2, the way Finder numbers a copy. Nothing beside the
// short is taken for one. A short removed by hand is no longer the clip's,
// but the clip was rendered all the same, #384, and the episode counts it.
//
// On the real disk: a short is a file on the disk, see IsShort.
func TestAShortInTheOutputFolderIsTheClipsShort(t *testing.T) {
	t.Parallel()
	source, plan := episodeWithPlan(t, "folge")
	shorts := ResolvePath(t.TempDir())
	other := filepath.Join(shorts, "wort.mp4")
	if err := os.WriteFile(other, []byte("another episode's short"), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered := func() int { return Status(disk, source, "", nil).Rendered }
	if c := shortOfClip(t, plan, "01"); c.Short != "" || c.Rendered {
		t.Errorf("a clip never rendered says it is, at %q", c.Short)
	}
	if IsShort(disk, source, other) {
		t.Error("another episode's short is taken for this clip's")
	}

	short := placeShort(t, plan, shorts, "01")
	if want := filepath.Join(shorts, "wort 2.mp4"); short != want {
		t.Errorf("beside another episode's wort.mp4 the short is %s, not %s", short, want)
	}
	if got := shortOfClip(t, plan, "01").Short; got != short {
		t.Errorf("the clip says its short is %q, not %q", got, short)
	}
	if !IsShort(disk, source, short) {
		t.Error("the short in the folder for shorts is not the clip's")
	}
	if n := rendered(); n != 1 {
		t.Errorf("the episode counts %d shorts, not 1", n)
	}
	beside := filepath.Join(shorts, "notes.mp4")
	if err := os.WriteFile(beside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsShort(disk, source, beside) || IsShort(disk, source, shorts) || IsShort(disk, source, other) {
		t.Error("what is beside the short is taken for one")
	}

	if err := os.Remove(short); err != nil {
		t.Fatal(err)
	}
	if c := shortOfClip(t, plan, "01"); c.Short != "" || !c.Rendered {
		t.Errorf("a short removed by hand is still the clip's, at %q, or the clip no longer says it was rendered", c.Short)
	}
	if IsShort(disk, source, short) {
		t.Error("a short removed by hand is still taken for the clip's")
	}
	if n := rendered(); n != 1 {
		t.Errorf("with its short removed by hand the episode counts %d shorts, not 1", n)
	}
}

// A render again writes over the clip's own short, under the name it has,
// and over nothing else. A name is taken by a short or by its first
// picture alone, so no short takes another's pictures either. A short
// written over by hand is not the file the clip wrote, so it is no longer
// the clip's, and a render again gives the clip a name of its own.
//
// On the real disk: a short is a file on the disk, see IsShort.
func TestARenderAgainWritesOverOnlyItsOwnShort(t *testing.T) {
	t.Parallel()
	source, plan := episodeWithPlan(t, "folge")
	dir := ResolvePath(t.TempDir())
	short := placeShort(t, plan, dir, "01")
	if again := placeShort(t, plan, dir, "01"); again != short {
		t.Errorf("rendered again, the short went to %s, not over its own at %s", again, short)
	}

	if err := os.WriteFile(short, []byte("written over by hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := shortOfClip(t, plan, "01"); c.Short != "" || !c.Rendered {
		t.Errorf("a short written over by hand is still the clip's, at %q, or the clip no longer says it was rendered", c.Short)
	}
	if again, want := placeShort(t, plan, dir, "01"), filepath.Join(dir, "wort 2.mp4"); again != want {
		t.Errorf("rendered again after its short was written over by hand, the short went to %s, not %s", again, want)
	}
	if IsShort(disk, source, short) {
		t.Error("the file written over by hand is still taken for the clip's short")
	}

	// The short is gone and its first picture is left: the picture holds
	// the name.
	if err := os.Remove(filepath.Join(dir, "wort 2.mp4")); err != nil {
		t.Fatal(err)
	}
	if again, want := placeShort(t, plan, dir, "01"), filepath.Join(dir, "wort 3.mp4"); again != want {
		t.Errorf("past a picture left of wort 2, the short went to %s, not %s", again, want)
	}
}

// Two episodes that render into one folder for shorts keep a short each,
// even when their clips have the same id and words, because each
// episode's shorts go into a folder of its own name there. Two episodes of
// the same file name share that folder, so the second short gets a
// number. Either way, neither clip takes the other's short for its own,
// and a render of one never writes over the other's.
//
// On the real disk: a short is a file on the disk, see IsShort.
func TestTwoEpisodesKeepTheirOwnShorts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, first, second, firstShort, secondShort string
	}{
		{"of other names", "erste", "zweite", "erste/wort.mp4", "zweite/wort.mp4"},
		{"of one name", "folge", "folge", "folge/wort.mp4", "folge/wort 2.mp4"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			shorts := ResolvePath(t.TempDir())
			type episode struct{ source, plan, short string }
			var eps [2]episode
			for i, name := range []string{c.first, c.second} {
				source, plan := episodeWithPlan(t, name)
				dir, err := shortsFolder("", shorts, source)
				if err != nil {
					t.Fatal(err)
				}
				eps[i] = episode{source, plan, placeShort(t, plan, dir, "01")}
			}
			for i, want := range []string{c.firstShort, c.secondShort} {
				if want = filepath.Join(shorts, want); eps[i].short != want {
					t.Errorf("a short is at %s, not %s", eps[i].short, want)
				}
			}
			for i, ep := range eps {
				if got := shortOfClip(t, ep.plan, "01").Short; got != ep.short {
					t.Errorf("the clip of episode %d says its short is %q, not %q", i+1, got, ep.short)
				}
				if other := eps[1-i]; IsShort(disk, ep.source, other.short) {
					t.Errorf("episode %d takes %s for its own", i+1, other.short)
				}
			}
		})
	}
}

// Two clips of one episode with the same words, as two searches can find,
// get a number too, and each keeps its own short when rendered again.
//
// On the real disk: a short is a file on the disk, see IsShort.
func TestTwoClipsOfTheSameWordsAreNumbered(t *testing.T) {
	t.Parallel()
	_, plan := episodeWithPlan(t, "folge")
	if err := os.WriteFile(plan, []byte(`{"clips": [
  {"id": "01", "slug": "wort", "segments": [{"start": 1, "end": 21}], "thumbnails": [2]},
  {"id": "t60-01", "slug": "wort", "segments": [{"start": 1, "end": 21}], "thumbnails": [2]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := ResolvePath(t.TempDir())
	want := []string{filepath.Join(dir, "wort.mp4"), filepath.Join(dir, "wort 2.mp4")}
	for range 2 {
		for i, id := range []string{"01", "t60-01"} {
			if got := placeShort(t, plan, dir, id); got != want[i] {
				t.Errorf("the short of clip %s is %s, not %s", id, got, want[i])
			}
		}
	}
}

// Noting where a short went is no edit, so an edit made before a render is
// undone after it as it would have been without it, and the clip is still
// rendered afterwards.
//
// On the real disk: a short is a file on the disk, see recordShort.
func TestAnEditIsUndoneAfterARender(t *testing.T) {
	t.Parallel()
	_, plan := episodeWithPlan(t, "folge")
	change := edited(t, disk, filepath.Dir(plan), func() error { return SetCaptionY(disk, plan, "01", 400) })
	placeShort(t, plan, ResolvePath(t.TempDir()), "01")
	if _, err := change.Undo(); err != nil {
		t.Fatalf("the edit before the render cannot be undone: %v", err)
	}
	if c := shortOfClip(t, plan, "01"); c.CaptionYMoved {
		t.Error("the undo left the caption where the edit put it")
	} else if c.Short == "" {
		t.Error("the undo took the short away from the clip")
	}
}

// A render of a clip puts its short where the rules above say, and a
// render again writes a new file over it. A preview rendered into the
// same folder keeps the clip's id in its name, so it never writes over
// the short, which stays the clip's. The one test here that renders, a
// clip of one second.
//
// On the real disk: ffmpeg reads the episode and writes the shorts.
func TestARenderPutsItsShortWhereItsNameSays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	source := testEpisode(t, "2")
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 180, 320
	base.Preset = "ultrafast"
	base.NoCaptions = true
	base.Out = ResolvePath(t.TempDir())
	if err := os.MkdirAll(LogsDir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(LogsDir(source), "clips.json")
	if err := os.WriteFile(plan, []byte(`{"clips": [
  {"id": "01", "slug": "wort", "segments": [{"start": 0.5, "end": 1.5}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetThumbnail(disk, plan, "01", -1, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base.Out, "wort.mp4"), []byte("another episode's short"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewProject(e, source, base)
	render := func(preview bool) {
		t.Helper()
		if err := p.Render(ctx, RenderRequest{Plan: plan, Preview: preview}); err != nil {
			t.Fatalf("%v %s", err, p.LastError())
		}
	}

	render(false)
	short := filepath.Join(base.Out, "wort 2.mp4")
	if got := shortOfClip(t, plan, "01").Short; got != short {
		t.Fatalf("beside another episode's wort.mp4 the clip says its short is %q, not %q", got, short)
	}
	if _, err := os.Stat(filepath.Join(base.Out, thumbnailName("wort 2", 1))); err != nil {
		t.Errorf("the short's first picture is not beside it: %v", err)
	}
	was, err := os.Stat(short)
	if err != nil {
		t.Fatal(err)
	}

	render(false)
	now, err := os.Stat(short)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(was, now) && was.ModTime().Equal(now.ModTime()) {
		t.Error("rendered again, the short was not written again")
	}
	if got := shortOfClip(t, plan, "01").Short; got != short {
		t.Errorf("rendered again, the clip says its short is %q, not %q", got, short)
	}

	render(true)
	after, err := os.Stat(short)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(now, after) || !now.ModTime().Equal(after.ModTime()) {
		t.Errorf("the preview wrote over the short at %s", short)
	}
	if got := shortOfClip(t, plan, "01").Short; got != short {
		t.Errorf("after the preview the clip says its short is %q, not %q", got, short)
	}
	videos, err := filepath.Glob(filepath.Join(base.Out, "*.mp4"))
	if err != nil || len(videos) != 3 {
		t.Errorf("the folder holds %q, not the other episode's short, this one and the preview", videos)
	}
}

// A plan is untrusted, so what it says of a short names a file of the
// clip's own name, or that with a number, and nothing else, whatever the
// folder.
//
// On the real disk: a short is a file ffmpeg wrote, looked for on the disk.
func TestAPlanNamesNoFileButAShort(t *testing.T) {
	t.Parallel()
	dir := ResolvePath(t.TempDir())
	c := Clip{ID: "01", Slug: "erste", Segments: []Segment{{Start: 1, End: 2}}}
	for _, name := range []string{"erste.mp4", "erste 2.mp4", "erste 10.mp4", "01_erste.mp4",
		"secret.mp4", "erste 02.mp4", "erste 2.mov", "erste .mp4", "erste 2 3.mp4"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		plan := Plan{Raw: map[string]any{keyShorts: map[string]any{c.ID: map[string]any{
			keyFolder: dir, keyName: name, keySize: json.Number(strconv.FormatInt(info.Size(), 10)),
			keyModified: json.Number(strconv.FormatInt(info.ModTime().UnixNano(), 10))}}}}
		want := ""
		if name == "erste.mp4" || name == "erste 2.mp4" || name == "erste 10.mp4" {
			want = path
		}
		if got := shortOf(plan, c); got != want {
			t.Errorf("a plan naming %q gives %q, not %q", name, got, want)
		}
	}
}

// What the app is told is said, and where I and O can make a clip, are the
// edges of the lines handEntry goes by: the words placed on the sound, not
// as the recogniser timed them. The recogniser folds the silence round a
// video's speech into its first and last word, here 0.4 s before and 0.6 s
// after, so the words as heard ran wider, and I was offered just after the
// last word where the clip could only fail, found by the second review of
// #456. Just inside the edges a clip is made, just outside it is not.
func TestWhatIsSaidIsWhereAClipMadeByHandCanBe(t *testing.T) {
	if got := saidOf(nil); got != nil {
		t.Errorf("no lines said %v", got)
	}
	frames := make([]float32, 1000)
	for i := range frames {
		frames[i] = -90
		if i >= 100 && i < 300 {
			frames[i] = -20
		}
	}
	floor := -50.0
	heard := fromStored([]Cue{{0.6, 1.8, "Es"}, {1.8, 3.6, "war."}}, frames, 0, -40, &floor)
	lines := BuildLines(heard.Words, heard.Levels(), nil)
	said := saidOf(lines)
	if said == nil || said[0] <= 0.6 || said[1] >= 3.6 {
		t.Fatalf("said %v, not inside the words as heard, 0.6 to 3.6", said)
	}
	seconds := func(keep [][2]int) float64 {
		total := 0.0
		for _, run := range keep {
			total += lines[run[1]-1].End() - lines[run[0]-1].Start()
		}
		return total
	}
	for _, c := range []struct {
		name string
		req  ClipRequest
		made bool
	}{
		{"I just before the last word ends", ClipRequest{At: said[1] - 0.05}, true},
		{"I where the last word ends", ClipRequest{At: said[1]}, false},
		{"I between the line and the word as heard", ClipRequest{At: said[1] + 0.3}, false},
		{"O where the first word starts", ClipRequest{At: said[0], Backward: true}, true},
		{"O just before the first word starts", ClipRequest{At: said[0] - 0.05, Backward: true}, false},
	} {
		_, err := handEntry(lines, c.req, 1, 30, seconds)
		if made := err == nil; made != c.made {
			t.Errorf("%s, at %.2f s with %v said: made a clip %v, %v", c.name, c.req.At, *said, made, err)
		}
	}
}
