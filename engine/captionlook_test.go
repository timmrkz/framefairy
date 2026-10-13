package engine

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"framefairy/internal/ffmpegtest"
)

// The caption file the video preview draws is the caption file the render
// writes for the same clip, byte for byte, made by the app's own way to
// it, ClipCaptionsView with the look the app puts together. At 29.97
// frames a second, with pieces whose edges do not start on a frame, the
// app once made its captions from the clip as the plan has it while the
// render made them on the frames, and some words lit a frame apart. The
// clip's own caption line is where both put the captions, over the one
// in the settings, which the app once put first.
//
// On the real disk: ffmpeg makes the episode, and the render writes its
// caption file where ffmpeg would read it.
func TestTheVideoPreviewsCaptionFileIsTheShorts(t *testing.T) {
	t.Parallel()
	ffmpegtest.Need(t)
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "episode.mp4")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=s=320x180:r=30000/1001:d=12",
		"-f", "lavfi", "-i", "sine=f=220:d=12",
		"-shortest", "-c:v", "mpeg4", "-q:v", "5", "-c:a", "aac", source).CombinedOutput(); err != nil {
		t.Fatalf("making the test episode: %s %s", err, out)
	}
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false), disk)
	if err := e.Preflight(ctx); err != nil {
		ffmpegtest.Unusable(t, "%s", err)
	}
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	base.DryRun = true
	// What the app's settings set, a caption line and a highlight colour.
	y := 400
	base.MarginV = &y
	base.HighlightColour = "#123456"
	p := NewProject(e, source, base)
	if err := p.Transcribe(ctx); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	plan := filepath.Join(LogsDir(source), "clips.json")
	if err := os.WriteFile(plan, []byte(`{"clips": [{"id": "01", "slug": "drei", "caption_y": 800, "segments": [
  {"start": 1.234, "end": 3.517}, {"start": 4.821, "end": 6.999}, {"start": 8.003, "end": 10.467}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.Render(ctx, RenderRequest{Plan: plan}); err != nil {
		t.Fatalf("%v %s", err, p.LastError())
	}
	rendered, err := filepath.Glob(filepath.Join(WorkDir(source), "captions", "*.ass"))
	if err != nil || len(rendered) != 1 {
		t.Fatalf("the render wrote caption files %v, %v", rendered, err)
	}
	short, err := os.ReadFile(rendered[0])
	if err != nil {
		t.Fatal(err)
	}

	words, err := SavedTranscript(disk, source, LogsDir(source), base.ASRModel, base.SilenceDB)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadClips(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	style, err := base.CaptionLook(loaded)
	if err != nil {
		t.Fatal(err)
	}
	info, err := e.Probe(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	view, err := ClipCaptionsView(disk, plan, "01", words, &Look{Style: style, Source: info})
	if err != nil {
		t.Fatal(err)
	}
	d := view.Drawing()
	rs := base.RenderSettings()
	file, err := e.previewTrack(ctx, t.TempDir(), d, d.drafted(nil), ResolveStyle(d.Style).Font, info.Light(), rs.OutW, rs.OutH)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(preview, short) {
		a, b := bytes.Split(short, []byte("\n")), bytes.Split(preview, []byte("\n"))
		for i := 0; i < len(a) && i < len(b); i++ {
			if !bytes.Equal(a[i], b[i]) {
				t.Fatalf("the video preview's caption file is not the short's, from line %d:\nshort:   %s\npreview: %s", i+1, a[i], b[i])
			}
		}
		t.Fatalf("the video preview's caption file has %d lines, the short's %d", len(b), len(a))
	}
}
