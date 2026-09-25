package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A clip plan is written by a model, read back by both front ends and edited
// by hand, so it is the one file in the project that can hold anything at
// all. What comes out of the loader has to be usable without further checks.

func TestSanitiseNameKeepsFilesAndFilterGraphsSafe(t *testing.T) {
	cases := []struct{ in, want string }{
		{"erste-wahl", "erste-wahl"},
		{"Erste Wahl", "Erste-Wahl"},
		{"../../etc/passwd", "etc-passwd"},
		{"a;rm -rf b", "a-rm--rf-b"},
		{"sch\u00f6n", "sch-n"},
		{"", "fallback"},
		{"---", "fallback"},
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}
	for _, c := range cases {
		if got := SanitiseName(c.in, "fallback"); got != c.want {
			t.Errorf("SanitiseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSafeChildStaysInsideItsFolder(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"01_a.mp4", "sub/01.mp4", "sub/../01.mp4", "./a"} {
		if _, err := SafeChild(base, name); err != nil {
			t.Errorf("SafeChild(%q): %v", name, err)
		}
	}
	for _, name := range []string{"..", "../out.mp4", "sub/../../out.mp4"} {
		if path, err := SafeChild(base, name); err == nil {
			t.Errorf("SafeChild(%q) allowed %s", name, path)
		}
	}
	// An absolute name is joined onto the folder rather than followed, so it
	// lands inside as well.
	if path, err := SafeChild(base, "/etc/passwd"); err != nil || !strings.HasPrefix(path, resolvePath(base)) {
		t.Errorf("absolute name gave %q, %v", path, err)
	}

	// A symlink planted inside the folder does not open a way out either.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "link")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if path, err := SafeChild(base, filepath.Join("link", "out.mp4")); err == nil {
		t.Errorf("a symlink led out of the folder: %s", path)
	}
}

// FuzzSafeChild makes sure the check itself cannot be talked around. Every
// name it accepts has to land inside the folder.
func FuzzSafeChild(f *testing.F) {
	for _, name := range []string{"01_a.mp4", "..", "../x", "a/../../x", "/etc/passwd",
		"a//b", ".", "", "a\x00b", "C:\\x"} {
		f.Add(name)
	}
	f.Fuzz(func(t *testing.T, name string) {
		base := t.TempDir()
		path, err := SafeChild(base, name)
		if err != nil {
			return
		}
		rel, relErr := filepath.Rel(resolvePath(base), path)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("SafeChild(%q) = %q, which is outside %q", name, path, base)
		}
	})
}

func TestParseTimeReadsSecondsAndTimecodes(t *testing.T) {
	good := []struct {
		in   any
		want float64
	}{
		{12.5, 12.5},
		{json.Number("90"), 90},
		{"90", 90},
		{"00:01:30", 90},
		{"01:30", 90},
		{"00:14:23.500", 863.5},
		{"00:14:23,500", 863.5},
		{"  7.25  ", 7.25},
	}
	for _, c := range good {
		got, err := ParseTime(c.in)
		if err != nil || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("ParseTime(%v) = %v, %v, want %v", c.in, got, err, c.want)
		}
	}
	// Negative, not finite, not a time at all. NaN and infinity matter most:
	// they compare false against everything and would pass any later check.
	for _, bad := range []any{-1.0, "-0.5", "nan", "inf", "-inf", true, nil,
		"1:2:3:4", "eins", "", map[string]any{}} {
		if got, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%v) accepted, gave %v", bad, got)
		}
	}
}

// FuzzParseTime pins down the promise the loader relies on: a time that comes
// back is a real number of seconds, never negative, never NaN.
func FuzzParseTime(f *testing.F) {
	for _, s := range []string{"12.5", "00:14:23.500", "-1", "nan", "inf", "1:2:3:4",
		"1_000", "0x10", "  9  ", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		seconds, err := ParseTime(text)
		if err != nil {
			return
		}
		if !isFinite(seconds) || seconds < 0 {
			t.Fatalf("ParseTime(%q) = %v", text, seconds)
		}
	})
}

func writePlan(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clips.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadClipsRefusesPlansThatCannotRender(t *testing.T) {
	segments := func(n int) string {
		var parts []string
		for i := 0; i < n; i++ {
			parts = append(parts, `{"start": 0, "end": 1}`)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	cases := []struct{ name, body string }{
		{"not an object", `[1, 2]`},
		{"clips is not a list", `{"clips": {"a": 1}}`},
		{"a clip is not an object", `{"clips": ["01"]}`},
		{"segments is not a list", `{"clips": [{"segments": 5}]}`},
		{"too many segments", `{"clips": [{"segments": ` + segments(MaxSegments+1) + `}]}`},
		{"a segment is not an object", `{"clips": [{"segments": [3]}]}`},
		{"no start", `{"clips": [{"segments": [{"end": 2}]}]}`},
		{"no end", `{"clips": [{"segments": [{"start": 2}]}]}`},
		{"a negative time", `{"clips": [{"segments": [{"start": -2, "end": 2}]}]}`},
		{"a time that is not a time", `{"clips": [{"segments": [{"start": "bald", "end": 2}]}]}`},
		{"backwards", `{"clips": [{"segments": [{"start": 9, "end": 2}]}]}`},
		{"empty", `{"clips": [{"segments": [{"start": 9, "end": 9}]}]}`},
		{"a crop that is not a number", `{"clips": [{"segments": [{"start": 0, "end": 1, ` +
			`"crop_x": "links"}]}]}`},
		{"a crop far outside any frame", `{"clips": [{"segments": [{"start": 0, "end": 1, ` +
			`"crop_x": 9000000}]}]}`},
		{"two clips with one name", `{"clips": [{"id": "01", "slug": "a b", "segments": ` +
			`[{"start": 0, "end": 1}]}, {"id": "01", "slug": "a/b", "segments": [{"start": 2, ` +
			`"end": 3}]}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, clips, err := LoadClips(writePlan(t, c.body)); err == nil {
				t.Errorf("accepted, gave %d clip(s)", len(clips))
			}
		})
	}
}

func TestLoadClipsCleansWhatItKeeps(t *testing.T) {
	path := writePlan(t, `{
  "source": "ep.mp4",
  "unknown": {"kept": true},
  "clips": [
    {"id": "../01", "slug": "Erste Wahl", "title": "Ein Titel",
     "segments": [{"start": "00:00:10.000", "end": 12, "crop_x": "center"},
                  {"start": 20, "end": 22, "crop_x": 300, "crop_x_auto": 100}],
     "words": [[10.0, 10.5, "eins"], [10.6, 11.0, "zwei"], ["x", 1, "kaputt"],
               [11.0, 10.0, "rückwärts"], [11.2, 11.4, ""], [1, 2]]},
    {"id": "02", "segments": []},
    {"id": "03", "rejected": true, "segments": [{"start": 30, "end": 31}]}
  ]
}`)
	plan, clips, err := LoadClips(path)
	if err != nil {
		t.Fatal(err)
	}
	// The clip without segments is skipped, the other two come back.
	if len(clips) != 2 {
		t.Fatalf("clips %+v", clips)
	}
	first := clips[0]
	if first.ID != "01" || first.Slug != "Erste-Wahl" || first.Basename() != "01_Erste-Wahl" {
		t.Errorf("names %q %q", first.ID, first.Slug)
	}
	if len(first.Segments) != 2 || first.Segments[0].Start != 10 || first.Segments[0].CropX != nil {
		t.Errorf("segments %+v", first.Segments)
	}
	if !first.Segments[1].Moved || *first.Segments[1].CropX != 300 {
		t.Errorf("hand-placed crop %+v", first.Segments[1])
	}
	// Only words with two real times and some text survive.
	if len(first.Words) != 2 || first.Words[1].Text != "zwei" {
		t.Errorf("words %+v", first.Words)
	}
	if first.Duration() != 4 {
		t.Errorf("duration %v", first.Duration())
	}
	if clips[0].Rejected || !clips[1].Rejected {
		t.Errorf("rejected flags %v %v", clips[0].Rejected, clips[1].Rejected)
	}
	// Everything the loader does not know about stays available, so an edit
	// writes it back untouched.
	if plan.Raw["unknown"] == nil || plan.Raw["source"] != "ep.mp4" {
		t.Errorf("raw plan %v", plan.Raw)
	}
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9_-]*$`)

// FuzzLoadClips throws whole plan files at the loader. Nothing it accepts may
// carry a name that could leave the output folder, a time that is not a time,
// or two clips that would overwrite each other's files.
func FuzzLoadClips(f *testing.F) {
	f.Add(`{"clips": [{"id": "01", "slug": "a", "segments": [{"start": 0, "end": 1}]}]}`)
	f.Add(`{"clips": [{"id": "01", "segments": [{"start": 0, "end": 1}],` +
		` "caption_times": {"100": {"start": 0.05, "end": "x"}, "y": 3, "200": {"end": 1e308}}}]}`)
	f.Add(`{"clips": [{"id": "../../x", "segments": [{"start": "00:01:00", "end": "1e3"}]}]}`)
	f.Add(`{"clips": [{"id": "01", "segments": [{"start": 0, "end": 1, "crop_x": -5}],` +
		`"words": [[0, 1, "eins"]]}]}`)
	f.Add(`{"clips": [{"id": "01", "segments": [{"start": 0, "end": 1}],` +
		` "thumbnails": [0.5, "0.2", 7, -1, 1e308, 0.5, null, [1]]}]}`)
	f.Add(`{"clips": [{"segments": [{"start": 1, "end": 0}]}]}`)
	f.Add(`{"clips": []}`)
	f.Fuzz(func(t *testing.T, body string) {
		path := filepath.Join(t.TempDir(), "clips.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Skip()
		}
		_, clips, err := LoadClips(path)
		if err != nil {
			return
		}
		seen := map[string]bool{}
		for _, clip := range clips {
			if len(clip.Segments) == 0 {
				t.Fatalf("a clip with no segments came back: %+v", clip)
			}
			if !safeName.MatchString(clip.ID) || !safeName.MatchString(clip.Slug) {
				t.Fatalf("unsafe name %q %q", clip.ID, clip.Slug)
			}
			if clip.ID == "" || seen[clip.Basename()] {
				t.Fatalf("two clips called %q", clip.Basename())
			}
			seen[clip.Basename()] = true
			for _, seg := range clip.Segments {
				if !isFinite(seg.Start) || !isFinite(seg.End) || seg.Start < 0 || seg.End <= seg.Start {
					t.Fatalf("segment %v-%v", seg.Start, seg.End)
				}
			}
			if len(clip.Thumbnails) > MaxThumbnails {
				t.Fatalf("%d thumbnails", len(clip.Thumbnails))
			}
			for k, th := range clip.Thumbnails {
				if !insidePieces(clip.Segments, th) || (k > 0 && th <= clip.Thumbnails[k-1]) {
					t.Fatalf("thumbnail %v in %v", th, clip.Thumbnails)
				}
			}
			for _, word := range clip.Words {
				if !isFinite(word.Start) || !isFinite(word.End) || word.End < word.Start ||
					strings.ContainsFunc(word.Text, isControl) {
					t.Fatalf("word %+v", word)
				}
			}
		}
	})
}

func TestCropWindowAndClamp(t *testing.T) {
	// 9:16 out of 1920x1080 is 608 pixels wide, rounded down to an even one.
	w, h := CropWindow(SourceInfo{Width: 1920, Height: 1080}, 1080, 1920)
	if w != 608 || h != 1080 {
		t.Errorf("crop window %dx%d", w, h)
	}
	// A source narrower than the target ratio keeps its width instead.
	w, h = CropWindow(SourceInfo{Width: 480, Height: 1080}, 1080, 1920)
	if w != 480 || h != 852 {
		t.Errorf("narrow source %dx%d", w, h)
	}
	if got := ClampCropX(nil, 608, 1920); got != 656 {
		t.Errorf("centred crop %d", got)
	}
	for _, at := range []int{-500, 0, 999, 5000} {
		x := ClampCropX(&at, 608, 1920)
		if x < 0 || x+608 > 1920 || x%2 != 0 {
			t.Errorf("crop at %d became %d", at, x)
		}
	}
}

// A plan file is untrusted: the model writes the first one and a person can
// edit it afterwards. These are the two ways a plan can look fine and still
// make the app do the wrong thing quietly.
func TestAPlanThatWouldEditTheWrongClipIsRefused(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"two clips with one name",
			`{"clips": [{"id": "01", "slug": "eins", "segments": [{"start": 1, "end": 2}]},
			            {"id": "01", "slug": "zwei", "segments": [{"start": 3, "end": 4}]}]}`,
			"both called",
		},
		{
			"a segment past any episode",
			`{"clips": [{"id": "01", "slug": "eins", "segments": [{"start": 1e300, "end": 1e301}]}]}`,
			"outside the episode",
		},
	}
	for _, c := range cases {
		_, clips, err := LoadClips(writePlan(t, c.body))
		if err == nil {
			t.Errorf("%s: loaded %d clips", c.name, len(clips))
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %s", c.name, err)
		}
	}
	// A file claiming more clips than any search could ask for.
	var many []string
	for i := 0; i < MaxClips+1; i++ {
		many = append(many, fmt.Sprintf(
			`{"id": "c%d", "slug": "s%d", "segments": [{"start": %d, "end": %d}]}`, i, i, i, i+1))
	}
	if _, _, err := LoadClips(writePlan(t, `{"clips": [`+strings.Join(many, ",")+`]}`)); err == nil {
		t.Error("a plan of a thousand and one clips was loaded")
	}

	// What is fine stays fine.
	_, clips, err := LoadClips(writePlan(t, `{"clips": [
		{"id": "01", "slug": "eins", "segments": [{"start": 1, "end": 2}]},
		{"id": "02", "slug": "zwei", "segments": [{"start": 3, "end": 4}]}]}`))
	if err != nil || len(clips) != 2 {
		t.Errorf("a good plan gave %d clips, %v", len(clips), err)
	}
}

// Settings that cannot be met are refused before anything is asked of the
// model, because the prompt would ask for clips between thirty and twenty
// seconds and the answer would be nonsense either way.
func TestASearchThatCouldFindNothingIsRefused(t *testing.T) {
	source := filepath.Join(t.TempDir(), "ep.mp4")
	if err := os.WriteFile(source, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		opts Options
		want string
	}{
		{"shortest over longest", Options{Min: 30, Max: 20, Count: 12}, "shortest clip cannot be longer"},
		{"no clips at all", Options{Min: 20, Max: 30, Count: 0}, "at least one clip"},
	} {
		var said bytes.Buffer
		e := NewEngine(NewLog(&said, false, false))
		c.opts.Source = source
		c.opts.Planner = "local"
		if code := e.Run(context.Background(), c.opts); code == 0 {
			t.Errorf("%s: the run was accepted", c.name)
		}
		if !strings.Contains(said.String(), c.want) {
			t.Errorf("%s said %q", c.name, strings.TrimSpace(said.String()))
		}
	}
}
