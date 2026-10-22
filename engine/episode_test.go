package engine

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if st := Status(fsys, source, ""); st.EverSearched || st.Work {
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
	if st := Status(fsys, source, ""); !st.EverSearched || !st.Work {
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
	if got := Status(disk, source, "").Rendered; got != 1 {
		t.Errorf("%d rendered, want 1", got)
	}
}

// A short rendered into the folder the settings name for shorts is the
// clip's short: the clip says it is rendered and where, the episode counts
// it, and the app may show it, see IsShort. It stays the clip's short when
// the folder in the settings changes afterwards, and it is gone when the
// file is, while the clip still says it was rendered. A file of the same name that another episode put in that
// folder is not this clip's short, and neither is anything beside it, and
// the render gives the short a number rather than write over it.
//
// On the real disk: ffmpeg reads the episode and writes the short.
func TestAShortInTheOutputFolderIsTheClipsShort(t *testing.T) {
	t.Parallel()
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	clip := func() ClipView {
		t.Helper()
		view, err := ReadPlan(disk, plan)
		if err != nil || len(view.Clips) == 0 {
			t.Fatalf("the plan: %v", err)
		}
		return view.Clips[0]
	}
	rendered := func() int { return Status(disk, p.Source, p.Base.ASRModel).Rendered }
	shorts := t.TempDir()
	other := filepath.Join(ResolvePath(shorts), clip().Slug+".mp4")
	short := filepath.Join(ResolvePath(shorts), clip().Slug+" 2.mp4")

	if err := os.WriteFile(other, []byte("another episode's short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := clip().Short; got != "" || clip().Rendered {
		t.Errorf("a clip never rendered says it is, at %q", got)
	}
	if IsShort(disk, p.Source, other) {
		t.Error("another episode's short is taken for this clip's")
	}

	p.Base.Out = shorts
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if got := clip().Short; got != short {
		t.Errorf("rendered into the folder for shorts, the clip says %q, not %q", got, short)
	}
	if !IsShort(disk, p.Source, short) {
		t.Error("the short in the folder for shorts is not the clip's")
	}
	if n := rendered(); n != 1 {
		t.Errorf("the episode counts %d shorts, not 1", n)
	}

	p.Base.Out = t.TempDir()
	if got := clip().Short; got != short {
		t.Errorf("after the folder in the settings changed the clip says %q, not %q", got, short)
	}
	beside := filepath.Join(shorts, "notes.mp4")
	if err := os.WriteFile(beside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsShort(disk, p.Source, beside) || IsShort(disk, p.Source, shorts) {
		t.Error("what is beside the short is taken for one")
	}

	if err := os.Remove(short); err != nil {
		t.Fatal(err)
	}
	if got := clip().Short; got != "" {
		t.Errorf("a short removed by hand is still the clip's, at %s", got)
	}
	if IsShort(disk, p.Source, short) {
		t.Error("a short removed by hand is still taken for the clip's")
	}
	// The short was uploaded and its folder cleaned up, #384: the clip was
	// rendered all the same, and the episode still counts it.
	if !clip().Rendered {
		t.Error("a clip whose short was removed by hand no longer says it was rendered")
	}
	if n := rendered(); n != 1 {
		t.Errorf("with its short removed by hand the episode counts %d shorts, not 1", n)
	}
}

// shortsEpisode is an episode searched for one clip, under a file name of
// its own, with a thumbnail on the clip.
type shortsEpisode struct {
	p    *Project
	plan string
}

// On the real disk: ffmpeg reads the episode.
func newShortsEpisode(t *testing.T, name string) shortsEpisode {
	t.Helper()
	p, _ := searchProject(t, nil)
	named := filepath.Join(filepath.Dir(p.Source), name+".mp4")
	if err := os.Rename(p.Source, named); err != nil {
		t.Fatal(err)
	}
	p = NewProject(p.engine, named, p.Base)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	ep := shortsEpisode{p, plan}
	c := ep.clip(t)
	if err := SetThumbnail(disk, plan, c.ID, -1, (c.Segments[0].Start+c.Segments[0].End)/2); err != nil {
		t.Fatal(err)
	}
	return ep
}

// On the real disk: the plan is beside the episode ffmpeg reads.
func (ep shortsEpisode) clip(t *testing.T) ClipView {
	t.Helper()
	view, err := ReadPlan(disk, ep.plan)
	if err != nil || len(view.Clips) == 0 {
		t.Fatalf("the plan: %v", err)
	}
	return view.Clips[0]
}

// render renders the clip and gives the short it says it has.
func (ep shortsEpisode) render(t *testing.T) string {
	t.Helper()
	if err := ep.p.Render(context.Background(), RenderRequest{Plan: ep.plan}); err != nil {
		t.Fatalf("%v %s", err, ep.p.LastError())
	}
	short := ep.clip(t).Short
	if short == "" {
		t.Fatal("a clip just rendered says it has no short")
	}
	return short
}

// files are the short and its first picture as they are on disk now.
//
// On the real disk: the short is a file ffmpeg wrote.
func files(t *testing.T, short string) [2]os.FileInfo {
	t.Helper()
	var out [2]os.FileInfo
	for i, path := range []string{short, strings.TrimSuffix(short, ".mp4") + "-1.jpg"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s is gone", path)
		}
		out[i] = info
	}
	return out
}

// untouched says whether the short and its picture are still the files
// they were.
func untouched(t *testing.T, short string, was [2]os.FileInfo) bool {
	t.Helper()
	now := files(t, short)
	return os.SameFile(was[0], now[0]) && os.SameFile(was[1], now[1]) &&
		was[0].ModTime().Equal(now[0].ModTime()) && was[1].ModTime().Equal(now[1].ModTime())
}

// neitherClaims says whether each episode leaves the other's short alone.
//
// On the real disk: the shorts are files ffmpeg wrote.
func neitherClaims(t *testing.T, a, b shortsEpisode, aShort, bShort string) {
	t.Helper()
	if IsShort(disk, a.p.Source, bShort) {
		t.Errorf("%s takes %s for its own", filepath.Base(a.p.Source), bShort)
	}
	if IsShort(disk, b.p.Source, aShort) {
		t.Errorf("%s takes %s for its own", filepath.Base(b.p.Source), aShort)
	}
}

// Two episodes that render into one folder for shorts keep a short each,
// even when a clip of one has the same id and slug as a clip of the
// other: each episode's shorts go into a folder of its own in it, under
// the clip's own name. The second render leaves the first episode's short
// and its picture as they were, and neither clip takes the other's short
// for its own.
//
// On the real disk: ffmpeg reads the episodes and writes the shorts.
func TestTwoEpisodesKeepTheirOwnShorts(t *testing.T) {
	t.Parallel()
	shorts := ResolvePath(t.TempDir())
	first, second := newShortsEpisode(t, "erste"), newShortsEpisode(t, "zweite")
	first.p.Base.Shorts, second.p.Base.Shorts = shorts, shorts
	name := first.clip(t).Slug
	if other := second.clip(t).Slug; other != name {
		t.Fatalf("the clips are %s and %s, which never meet in one folder", name, other)
	}

	firstShort := first.render(t)
	was := files(t, firstShort)
	secondShort := second.render(t)

	for short, want := range map[string]string{
		firstShort:  filepath.Join(shorts, "erste", name+".mp4"),
		secondShort: filepath.Join(shorts, "zweite", name+".mp4"),
	} {
		if short != want {
			t.Errorf("a short is at %s, not %s", short, want)
		}
	}
	if !untouched(t, firstShort, was) {
		t.Errorf("the second render wrote over the first episode's short at %s", firstShort)
	}
	if got := first.clip(t).Short; got != firstShort {
		t.Errorf("the first episode's clip says its short is %q, not %q", got, firstShort)
	}
	neitherClaims(t, first, second, firstShort, secondShort)
}

// Two episodes of the same file name share their folder in the folder for
// shorts, so the second short of the same name gets a number the way
// Finder gives one, and its pictures go with it. A render again writes
// over the clip's own short, under the name it has, and nothing else.
//
// On the real disk: ffmpeg reads the episodes and writes the shorts.
func TestTwoEpisodesOfOneNameNumberTheirShorts(t *testing.T) {
	t.Parallel()
	shorts := ResolvePath(t.TempDir())
	first, second := newShortsEpisode(t, "episode"), newShortsEpisode(t, "episode")
	first.p.Base.Shorts, second.p.Base.Shorts = shorts, shorts
	name := first.clip(t).Slug

	firstShort := first.render(t)
	was := files(t, firstShort)
	secondShort := second.render(t)
	if want := filepath.Join(shorts, "episode", name+" 2.mp4"); secondShort != want {
		t.Errorf("the second short is at %s, not %s", secondShort, want)
	}
	if !untouched(t, firstShort, was) {
		t.Errorf("the second render wrote over the first episode's short at %s", firstShort)
	}
	neitherClaims(t, first, second, firstShort, secondShort)

	wasSecond := files(t, secondShort)
	if again := second.render(t); again != secondShort {
		t.Errorf("rendered again, the second short went to %s, not %s", again, secondShort)
	}
	if untouched(t, secondShort, wasSecond) {
		t.Error("rendered again, the second short was not written again")
	}
	wasSecond = files(t, secondShort)
	if again := first.render(t); again != firstShort {
		t.Errorf("rendered again, the first short went to %s, not %s", again, firstShort)
	}
	if !untouched(t, secondShort, wasSecond) {
		t.Errorf("the first episode's render again wrote over %s", secondShort)
	}
	neitherClaims(t, first, second, firstShort, secondShort)
}

// On the command line, --out names the folder itself, and a second
// episode's short of the same words gets a number there. A short written
// over by hand is no longer the clip's, because it is not the file it
// wrote.
//
// On the real disk: ffmpeg reads the episodes and writes the shorts.
func TestAShortWrittenOverIsNoLongerTheClips(t *testing.T) {
	t.Parallel()
	out := ResolvePath(t.TempDir())
	first, second := newShortsEpisode(t, "erste"), newShortsEpisode(t, "zweite")
	first.p.Base.Out, second.p.Base.Out = out, out
	name := first.clip(t).Slug

	firstShort := first.render(t)
	secondShort := second.render(t)
	if firstShort != filepath.Join(out, name+".mp4") || secondShort != filepath.Join(out, name+" 2.mp4") {
		t.Errorf("with --out the shorts are %s and %s", firstShort, secondShort)
	}
	neitherClaims(t, first, second, firstShort, secondShort)
	if err := os.WriteFile(firstShort, []byte("written over by hand"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := first.clip(t).Short; got != "" {
		t.Errorf("the first episode's clip takes a file written over by hand for its own, at %s", got)
	}
	if !first.clip(t).Rendered {
		t.Error("the first episode's clip, its short written over by hand, no longer says it was rendered")
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

// What a render makes is no edit, so an edit made before a render is
// undone after it as it would have been without it, and the clip is still
// rendered afterwards.
//
// On the real disk: ffmpeg reads the episode and writes the short.
func TestAnEditIsUndoneAfterARender(t *testing.T) {
	t.Parallel()
	p, _ := searchProject(t, nil)
	plan, err := p.Search(context.Background(), PlanRequest{Count: 1, Min: 5}, nil)
	if err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	view, err := ReadPlan(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	id := view.Clips[0].ID
	change := edited(t, disk, filepath.Dir(plan), func() error { return SetCaptionY(disk, plan, id, 400) })
	p.Base.Out = t.TempDir()
	if err := p.Render(context.Background(), RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	if _, err := change.Undo(); err != nil {
		t.Fatalf("the edit before the render cannot be undone: %v", err)
	}
	view, err = ReadPlan(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	if c := view.Clips[0]; c.CaptionYMoved {
		t.Error("the undo left the caption where the edit put it")
	} else if c.Short == "" {
		t.Error("the undo took the short away from the clip")
	}
}

// A short is named after its clip's words, without the id. Two clips of
// one episode with the same words, as two searches can find, get a number
// in the episode's own out folder and in --out alike, and a render again
// writes each over in place.
//
// On the real disk: ffmpeg reads the episode and writes the shorts.
func TestTwoClipsOfTheSameWordsAreNumbered(t *testing.T) {
	t.Parallel()
	for _, into := range []string{"out", "--out"} {
		t.Run(into, func(t *testing.T) {
			t.Parallel()
			ep := newShortsEpisode(t, "folge")
			out := filepath.Join(ResolvePath(WorkDir(ep.p.Source)), "out")
			if into == "--out" {
				out = ResolvePath(t.TempDir())
				ep.p.Base.Out = out
			}
			c := ep.clip(t)
			// The first clip again, under the id of another search.
			if err := editPlan(disk, ep.plan, func(top *object, clips []*object) error {
				again := newObject()
				for _, k := range clips[0].keys {
					again.set(k, clips[0].values[k])
				}
				again.set(keyID, "t60-01")
				top.set(keyClips, append(top.values[keyClips].([]any), again))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			want := []string{filepath.Join(out, c.Slug+".mp4"), filepath.Join(out, c.Slug+" 2.mp4")}
			var was [][2]os.FileInfo
			for range 2 {
				ep.render(t)
				view, err := ReadPlan(disk, ep.plan)
				if err != nil || len(view.Clips) != 2 {
					t.Fatalf("the plan: %v", err)
				}
				if got := []string{view.Clips[0].Short, view.Clips[1].Short}; got[0] != want[0] || got[1] != want[1] {
					t.Fatalf("the shorts are %q, not %q", got, want)
				}
				for i, short := range want {
					if was != nil && untouched(t, short, was[i]) {
						t.Errorf("rendered again, %s was not written again", short)
					}
				}
				was = [][2]os.FileInfo{files(t, want[0]), files(t, want[1])}
			}
		})
	}
}

// A preview rendered into the folder --out names never writes over the
// short there: the preview keeps the clip's id in its name and the short
// does not. The clip still says the short is its own afterwards.
//
// On the real disk: ffmpeg writes the short and the preview.
func TestAPreviewLeavesTheShortAlone(t *testing.T) {
	t.Parallel()
	ep := newShortsEpisode(t, "folge")
	ep.p.Base.Out = ResolvePath(t.TempDir())
	short := ep.render(t)
	was := files(t, short)
	if err := ep.p.Render(context.Background(), RenderRequest{Plan: ep.plan, Preview: true}); err != nil {
		t.Fatalf("%v %s", err, ep.p.LastError())
	}
	if !untouched(t, short, was) {
		t.Errorf("the preview wrote over the short at %s", short)
	}
	if got := ep.clip(t).Short; got != short {
		t.Errorf("after the preview the clip says its short is %q, not %q", got, short)
	}
	entries, err := os.ReadDir(ep.p.Base.Out)
	if err != nil {
		t.Fatal(err)
	}
	var shorts []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".mp4") {
			shorts = append(shorts, e.Name())
		}
	}
	if len(shorts) != 2 {
		t.Errorf("the folder holds %q, not the short and the preview beside it", shorts)
	}
}

// What a video says runs from the start of its first word to the end of
// its last, whatever order the words were heard in, and is nothing while
// no word is heard.
func TestWhatIsSaidSpansTheWordsHeard(t *testing.T) {
	if got := saidSpan(nil); got != nil {
		t.Errorf("no words said %v", got)
	}
	words := []Cue{{40, 41.5, "später"}, {2.25, 3, "Es"}, {10, 12.75, "war"}}
	if got := saidSpan(words); got == nil || *got != [2]float64{2.25, 41.5} {
		t.Errorf("said %v, not 2.25 to 41.5", got)
	}
}
