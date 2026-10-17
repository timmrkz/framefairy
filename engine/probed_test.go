package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A stand-in for ffmpeg or ffprobe that writes a line to asked for every
// run and answers with what each kind of question wants.
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
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
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
	probe := func() SourceInfo {
		t.Helper()
		e := &Engine{Log: NewLog(io.Discard, false, false), FFprobe: ffprobe}
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
	e := &Engine{Log: NewLog(io.Discard, false, false), FFprobe: ffprobe}
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
	find := func() string {
		t.Helper()
		e := &Engine{Log: NewLog(io.Discard, false, false), FFmpeg: ffmpeg}
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
