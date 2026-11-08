package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/standin"
)

// A stand-in for ffmpeg or ffprobe that writes a line to asked for every
// run and answers with what each kind of question wants.
//
// On the real disk: the stand-in is a program the engine runs.
func countingTool(t *testing.T, name string) (path string, runs func() int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	asked := filepath.Join(dir, "asked")
	path = filepath.Join(dir, name)
	script := `#!/bin/sh
echo "$*" >> '` + asked + `'
case "$*" in
*-filters*) printf ' ... ass               V->V       Render ASS subtitles\n' ;;
*-read_intervals*) printf '0.000000\n0.040000\n0.080000\n' ;;
*a:0*) printf '{"streams":[{"sample_rate":"48000","channels":2}]}' ;;
*stream=start_time*) printf '{"streams":[{"start_time":"0.000000"}],"format":{"start_time":"0.000000"}}' ;;
*v:0*) printf '{"streams":[{"width":1920,"height":1080,"r_frame_rate":"25/1","avg_frame_rate":"25/1","start_time":"0.000000","color_primaries":"bt709"}],"format":{"duration":"60.0","start_time":"0.000000","format_name":"mov,mp4"}}' ;;
esac
`
	if err := standin.Write(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, func() int {
		data, err := os.ReadFile(asked)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(data), "\n")
	}
}

// On the real disk: the stand-in ffprobe is a program that is run, and
// what is kept is kept by the file's size and time there.
func TestAFileIsProbedOnceUntilItChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffprobe, runs := countingTool(t, "ffprobe")
	video := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	machine := &Machine{}
	probe := func() SourceInfo {
		t.Helper()
		e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
		info, err := e.Probe(ctx, video)
		if err != nil {
			t.Fatal(err)
		}
		return info
	}

	first := probe()
	once := runs()
	if once == 0 {
		t.Fatal("the stand-in was never asked")
	}
	if first.Width != 1920 || first.SoundRate != 48000 || first.Duration != 60 {
		t.Fatalf("the probe read %+v", first)
	}
	first.Colour[0][1] = "changed by a caller"
	second := probe()
	if got := runs(); got != once {
		t.Errorf("a second engine asked ffprobe %d more times about a file that did not change", got-once)
	}
	if second.Width != first.Width || second.SoundRate != first.SoundRate || second.Duration != first.Duration {
		t.Errorf("the kept answer %+v is not the one given %+v", second, first)
	}
	if second.Colour[0][1] != "bt709" {
		t.Errorf("a caller changed the kept answer, it now says %q", second.Colour[0][1])
	}

	// Written again, it is another file.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(video, later, later); err != nil {
		t.Fatal(err)
	}
	probe()
	if got := runs(); got != 2*once {
		t.Errorf("a file written again was asked about %d times, not %d", got-once, once)
	}

	// Another machine, another program's or another test's, keeps its own.
	machine = &Machine{}
	probe()
	if got := runs(); got != 3*once {
		t.Errorf("a second machine was asked about the file %d times, not %d", got-2*once, once)
	}
}

// saysStopped is a context that never stops a program but says it was
// stopped, the way one stopped just as its last question was answered:
// every question is answered, and what the answers say may be half.
type saysStopped struct{ context.Context }

func (saysStopped) Err() error { return context.Canceled }

// On the real disk: the stand-in ffprobe is a program that is run.
func TestAProbeThatWasStoppedIsNotKept(t *testing.T) {
	t.Parallel()
	ffprobe, runs := countingTool(t, "ffprobe")
	video := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Engine{Machine: &Machine{}, Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
	if _, err := e.Probe(saysStopped{context.Background()}, video); err != nil {
		t.Fatal(err)
	}
	before := runs()
	if before == 0 {
		t.Fatal("the stand-in was never asked")
	}
	if _, err := e.Probe(context.Background(), video); err != nil {
		t.Fatal(err)
	}
	if runs() == before {
		t.Error("the answer of a probe that was stopped was kept and given again")
	}
}

// On the real disk: the stand-in ffmpeg is a program that is run.
func TestTheSubtitleSyntaxIsFoundOnceForEveryEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffmpeg, runs := countingTool(t, "ffmpeg")
	machine := &Machine{}
	find := func() string {
		t.Helper()
		e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFmpeg: ffmpeg}
		template, err := e.SubtitleFilter(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return template
	}
	first := find()
	once := runs()
	if once < 2 {
		t.Fatalf("the stand-in was asked %d times, not for its filters and then to burn in a caption", once)
	}
	if got := find(); got != first {
		t.Errorf("the second engine was told %q, the first %q", got, first)
	}
	if got := runs(); got != once {
		t.Errorf("a second engine asked the same ffmpeg %d more times", got-once)
	}
}

// Preflight asks ffmpeg and ffprobe for their versions once for the
// program's machine, not once for every engine the app makes.
//
// On the real disk: the stand-ins are programs the engine runs.
func TestEveryToolIsAskedItsVersionOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffmpeg, _ := countingTool(t, "ffmpeg")
	ffprobe, _ := countingTool(t, "ffprobe")
	versions := func() int {
		n := 0
		for _, tool := range []string{ffmpeg, ffprobe} {
			data, _ := os.ReadFile(filepath.Join(filepath.Dir(tool), "asked"))
			n += strings.Count(string(data), "-version\n")
		}
		return n
	}
	machine := &Machine{}
	for range 3 {
		e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFmpeg: ffmpeg, FFprobe: ffprobe,
			SkipCaptions: true}
		// The stand-in knows no encoder, so Preflight fails past the
		// versions. It must fail there, not on either tool.
		err := e.Preflight(ctx)
		if err == nil || strings.Contains(err.Error(), "does not run") || strings.Contains(err.Error(), "partial") {
			t.Fatalf("preflight said %v", err)
		}
	}
	if got := versions(); got != 2 {
		t.Errorf("three engines asked for a version %d times, not once for each tool", got)
	}
}

// A machine that learned from another asks ffmpeg and ffprobe nothing the
// other was already told, and keeps none of the other's jobs. The walks'
// bridge opens the app again for every walk, and each walk asked the same
// ffmpeg again what the walk before it had asked.
//
// On the real disk: the stand-ins are programs the engine runs, and what
// is kept is kept by the file's size and time there.
func TestAMachineThatLearnedFromAnotherAsksNothingAgain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffmpeg, ffmpegRuns := countingTool(t, "ffmpeg")
	ffprobe, ffprobeRuns := countingTool(t, "ffprobe")
	video := filepath.Join(t.TempDir(), "episode.mp4")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	ask := func(machine *Machine) {
		t.Helper()
		e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFmpeg: ffmpeg, FFprobe: ffprobe}
		if _, err := e.SubtitleFilter(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Probe(ctx, video); err != nil {
			t.Fatal(err)
		}
	}
	before := NewMachine(afero.NewMemMapFs())
	ask(before)
	ffmpegOnce, ffprobeOnce := ffmpegRuns(), ffprobeRuns()
	if ffmpegOnce == 0 || ffprobeOnce == 0 {
		t.Fatalf("the stand-ins were asked %d and %d times", ffmpegOnce, ffprobeOnce)
	}
	before.episodes = map[string]*Episode{video: {}}

	after := NewMachine(afero.NewMemMapFs())
	after.LearnFrom(before)
	ask(after)
	if got := ffmpegRuns(); got != ffmpegOnce {
		t.Errorf("the machine that learned asked ffmpeg %d more times", got-ffmpegOnce)
	}
	if got := ffprobeRuns(); got != ffprobeOnce {
		t.Errorf("the machine that learned asked ffprobe %d more times", got-ffprobeOnce)
	}
	if after.Tools != before.Tools {
		t.Error("the machine that learned checks the programs again")
	}
	if len(after.episodes) != 0 {
		t.Errorf("the machine that learned holds the other's videos %v", after.episodes)
	}
	if _, tracks := after.captions(); tracks == nil {
		t.Error("the machine that learned has nowhere to keep its own caption files")
	} else if _, theirs := before.captions(); tracks == theirs {
		t.Error("the machine that learned shares the other's caption files")
	}
}

// Two engines that ask a machine at the same moment how its ffmpeg wants
// a subtitle file named wait for one answer: the workspace asks for the
// captions of a clip twice as a video opens, and on a machine that knew
// nothing yet each asked ffmpeg for itself.
//
// On the real disk: the stand-in ffmpeg is a program the engine runs.
func TestTwoAsksAtOnceRunOneSubtitleProbe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffmpeg, runs := countingTool(t, "ffmpeg")
	once := func() int {
		e := &Engine{Machine: &Machine{}, Log: runlog.New(io.Discard, false, false), FFmpeg: ffmpeg}
		if _, err := e.SubtitleFilter(ctx); err != nil {
			t.Fatal(err)
		}
		return runs()
	}()
	machine := &Machine{}
	const asks = 8
	answers := make(chan string, asks)
	var start sync.WaitGroup
	start.Add(1)
	for range asks {
		go func() {
			e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFmpeg: ffmpeg}
			start.Wait()
			template, err := e.SubtitleFilter(ctx)
			if err != nil {
				t.Error(err)
			}
			answers <- template
		}()
	}
	start.Done()
	first := <-answers
	for range asks - 1 {
		if got := <-answers; got != first {
			t.Errorf("one ask was told %q, another %q", got, first)
		}
	}
	if got := runs() - once; got != once {
		t.Errorf("%d asks at once ran ffmpeg %d times, one ask runs it %d times", asks, got, once)
	}
}

// How the frames come at the edges of a piece, and where a piece is read
// from in a file with no index of its key frames, is read once for a
// file. A render asked ffprobe at every piece edge of every clip, every
// time, issue #609.
//
// On the real disk: the stand-in ffprobe is a program that is run, and
// what is kept is kept by the file's size and time there.
func TestThePiecesOfAClipAreReadOnceUntilTheFileChanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ffprobe, runs := countingTool(t, "ffprobe")
	video := filepath.Join(t.TempDir(), "episode.ts")
	if err := os.WriteFile(video, []byte("a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	machine := &Machine{}
	pieces := []Segment{{Start: 2, End: 4}, {Start: 10, End: 12.5}}
	read := func(info SourceInfo) {
		t.Helper()
		e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
		e.keyedPieces(ctx, video, e.unevenPieces(ctx, video, info, pieces), pieces)
	}
	even := SourceInfo{FPSNum: 25, FPSDen: 1, Duration: 60, hasClock: true}
	read(even)
	// One read for each piece.
	if got := runs(); got != 2 {
		t.Fatalf("ffprobe was asked %d times for two pieces", got)
	}
	read(even)
	if got := runs(); got != 2 {
		t.Errorf("the same pieces read again asked ffprobe %d more times", got-2)
	}
	// Another piece is read on its own.
	read2 := append(slices.Clone(pieces), Segment{Start: 20, End: 21})
	e := &Engine{Machine: machine, Log: runlog.New(io.Discard, false, false), FFprobe: ffprobe}
	e.unevenPieces(ctx, video, even, read2)
	if got := runs(); got != 3 {
		t.Errorf("a new piece asked ffprobe %d times, not once", got-2)
	}

	// In a file with no index of its key frames, where each piece is read
	// from is kept too.
	unindexed := even
	unindexed.unindexed = true
	before := runs()
	read(unindexed)
	asked := runs() - before
	if asked == 0 {
		t.Fatal("the pieces of a file with no index were not read")
	}
	read(unindexed)
	if got := runs() - before; got != asked {
		t.Errorf("the key frames of the same pieces were asked again, %d runs, not %d", got, asked)
	}

	// Written again, it is another file.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(video, later, later); err != nil {
		t.Fatal(err)
	}
	before = runs()
	read(even)
	if got := runs() - before; got != 2 {
		t.Errorf("a file written again was read %d times for two pieces", got)
	}
}
