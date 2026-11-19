package engine

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
)

// A start of the app knows what ffprobe said of a video at the start
// before: the probe is read from the video's work folder and asks ffprobe
// nothing, the same answer field by field. Every start ran ffprobe five
// times for every open video while the window waited, issue #891. A
// video written again, or a new ffprobe, asks again. A file with no work
// folder, a short, gets no folder made for it.
//
// On the real disk: the stand-in ffprobe is a program that is run, and it
// reads the video there.
func TestAProbeIsKeptForTheNextStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffprobe, runs := countingTool(t, "ffprobe")
	dir := t.TempDir()
	video := filepath.Join(dir, "episode.mp4")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(LogsDir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	// Each start of the app is a machine of its own.
	start := func(file string) SourceInfo {
		t.Helper()
		e := &Engine{Machine: NewMachine(afero.NewOsFs()), Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
		info, err := e.Probe(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		return info
	}

	first := start(video)
	asked := runs()
	if asked == 0 {
		t.Fatal("the first start asked ffprobe nothing")
	}
	second := start(video)
	if got := runs() - asked; got != 0 {
		t.Errorf("the second start asked ffprobe %d times about a video it probed before", got)
	}
	// What the stand-in ffprobe says, written out by hand, and what the
	// engine keeps to itself.
	want := SourceInfo{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1, Duration: 60,
		Colour: [][2]string{{"color_primaries", "bt709"}}, SoundRate: 48000, SoundChannels: 2, hasClock: true}
	if !sameInfo(first, want) || !sameInfo(second, want) {
		t.Errorf("probed as %+v, kept as %+v, want %+v", first, second, want)
	}

	// Written again, it is another video.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(video, later, later); err != nil {
		t.Fatal(err)
	}
	before := runs()
	start(video)
	if runs() == before {
		t.Error("a video written again was not probed again")
	}
	// An update of the app brings an ffprobe of its own.
	if err := os.Chtimes(ffprobe, later, later); err != nil {
		t.Fatal(err)
	}
	before = runs()
	start(video)
	if runs() == before {
		t.Error("a new ffprobe did not probe the video again")
	}

	// A short has no work folder, and a video added a moment ago no logs
	// folder yet: neither gets a folder made by a probe.
	short := filepath.Join(dir, "short.mp4")
	if err := os.WriteFile(short, []byte("a short"), 0o644); err != nil {
		t.Fatal(err)
	}
	start(short)
	if _, err := os.Stat(WorkDir(short)); !os.IsNotExist(err) {
		t.Errorf("probing a short made a work folder for it: %v", err)
	}
	added := filepath.Join(dir, "added.mp4")
	if err := os.WriteFile(added, []byte("a video just added"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(WorkDir(added), 0o755); err != nil {
		t.Fatal(err)
	}
	start(added)
	if _, err := os.Stat(LogsDir(added)); !os.IsNotExist(err) {
		t.Errorf("probing a video made its logs folder: %v", err)
	}
}

// sameInfo compares two probes in every field, the ones kept to the
// engine too.
func sameInfo(a, b SourceInfo) bool { return reflect.DeepEqual(a, b) }

// notKept are the fields of SourceInfo a render learns of each piece it
// cuts, which a kept probe leaves out, see keptInfo.
var notKept = []string{"unevenAt", "keyAt"}

// Every field of SourceInfo is kept, or named as one that is not: a field
// added to it and not to keptInfo would read as nothing on every start
// from then on, found by the review. A probe with every kept field set
// comes back the same.
func TestAKeptProbeHasEveryFieldOfAProbe(t *testing.T) {
	t.Parallel()
	full := SourceInfo{Width: 1080, Height: 1920, Turned: 90, FPSNum: 30000, FPSDen: 1001, Duration: 61.5, VideoStart: 0.2,
		Variable: true, Colour: [][2]string{{"colorspace", "bt709"}}, SoundRate: 48000, SoundChannels: 2,
		origin: 1.5, hasClock: true, unindexed: true}
	kept := reflect.TypeFor[keptInfo]()
	v := reflect.ValueOf(full)
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if slices.Contains(notKept, name) {
			continue
		}
		if _, ok := kept.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, name) }); !ok {
			t.Errorf("SourceInfo.%s is not in keptInfo, nor named in notKept", name)
		}
		if v.Field(i).IsZero() {
			t.Errorf("SourceInfo.%s is not set in this test, so it would not show a field the kept probe loses", name)
		}
	}
	fsys := afero.NewMemMapFs()
	video := "/videos/episode.mp4"
	if err := afero.WriteFile(fsys, video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fsys.MkdirAll(LogsDir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	keepProbe(fsys, video, "1 2", full)
	if back, ok := readKeptProbe(fsys, video, "1 2"); !ok || !sameInfo(back, full) {
		t.Errorf("kept as %+v, read back as %+v, %v", full, back, ok)
	}
}

// keptFor is a kept probe of the video stamped stamp as ffprobe tool
// said it, as bytes, with change made to it first.
func keptFor(t testing.TB, change func(*probeFile)) []byte {
	file := probeFile{Version: probeVersion, Source: keptStamp, Tool: "1 2", Info: keptInfo{
		Width: 1920, Height: 1080, FPSNum: 30000, FPSDen: 1001, Duration: 3600, VideoStart: 0.1,
		Colour: [][2]string{{"color_primaries", "bt709"}, {"color_trc", "arib-std-b67"}}, SoundRate: 48000, SoundChannels: 2,
		Origin: 1.5, HasClock: true,
	}}
	if change != nil {
		change(&file)
	}
	body, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

var keptStamp = sourceStamp{Name: "episode.mp4", Size: 7, Modified: "2026-11-19T12:00:00Z"}

// A kept probe is outside bytes: a sync service, a person or an older
// build may have written it. One that is not for this video, this ffprobe
// and this version, or says what no probe can say, is not read, and the
// video is probed again. Its colour tags are handed to ffmpeg, so one
// that could read as a flag is refused.
func TestAKeptProbeIsReadOnlyWhenItFits(t *testing.T) {
	t.Parallel()
	if info, ok := parseKeptProbe(keptFor(t, nil), keptStamp, "1 2"); !ok || info.FPSNum != 30000 || info.origin != 1.5 || !info.hasClock {
		t.Fatalf("a kept probe that fits was read as %+v, %v", info, ok)
	}
	for name, change := range map[string]func(*probeFile){
		"another version":          func(f *probeFile) { f.Version = probeVersion + 1 },
		"another video":            func(f *probeFile) { f.Source.Size = 8 },
		"another ffprobe":          func(f *probeFile) { f.Tool = "1 3" },
		"no width":                 func(f *probeFile) { f.Info.Width = 0 },
		"a height of a mile":       func(f *probeFile) { f.Info.Height = 1 << 20 },
		"no rate":                  func(f *probeFile) { f.Info.FPSDen = 0 },
		"a rate below nought":      func(f *probeFile) { f.Info.FPSNum = -25 },
		"a turn of an eighth":      func(f *probeFile) { f.Info.Turned = 45 },
		"a length below nought":    func(f *probeFile) { f.Info.Duration = -1 },
		"a length of years":        func(f *probeFile) { f.Info.Duration = 1e12 },
		"a start of years":         func(f *probeFile) { f.Info.VideoStart = -1e12 },
		"sound below nought":       func(f *probeFile) { f.Info.SoundRate = -1 },
		"a colour flag of its own": func(f *probeFile) { f.Info.Colour = [][2]string{{"vf", "bt709"}} },
		"a colour that is a flag":  func(f *probeFile) { f.Info.Colour = [][2]string{{"colorspace", "-filter_complex"}} },
		"a colour with a space":    func(f *probeFile) { f.Info.Colour = [][2]string{{"colorspace", "bt 709"}} },
		"a colour that is nothing": func(f *probeFile) { f.Info.Colour = [][2]string{{"colorspace", ""}} },
		"four colours":             func(f *probeFile) { f.Info.Colour = slices.Repeat([][2]string{{"colorspace", "bt709"}}, 4) },
	} {
		if info, ok := parseKeptProbe(keptFor(t, change), keptStamp, "1 2"); ok {
			t.Errorf("%s was read, as %+v", name, info)
		}
	}
	for _, bad := range []string{"", "{", "null", `{"version": 1, "info": []}`} {
		if _, ok := parseKeptProbe([]byte(bad), keptStamp, "1 2"); ok {
			t.Errorf("%q was read", bad)
		}
	}
}

// Whatever bytes a work folder holds, a kept probe is read without a
// panic, and one that is read says what a probe can say.
func FuzzParseKeptProbe(f *testing.F) {
	f.Add(keptFor(f, nil))
	f.Add(keptFor(f, func(p *probeFile) { p.Info.Colour = nil; p.Info.Turned = 270 }))
	f.Add([]byte(`{"version":1,"source":{"name":"episode.mp4","size":7,"modified":"2026-11-19T12:00:00Z"},"tool":"1 2","info":{"width":1e400}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		info, ok := parseKeptProbe(data, keptStamp, "1 2")
		if !ok {
			return
		}
		if info.Width <= 0 || info.Height <= 0 || info.FPSNum <= 0 || info.FPSDen <= 0 ||
			math.IsNaN(info.Duration) || math.IsInf(info.Duration, 0) || info.Duration < 0 {
			t.Fatalf("read a probe no probe gives: %+v", info)
		}
		for _, c := range info.Colour {
			if c[1] == "" || c[1][0] == '-' {
				t.Fatalf("read a colour tag that is a flag: %q", c)
			}
		}
	})
}
