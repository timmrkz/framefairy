package engine

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
	"framefairy/internal/standin"
)

// NewEngine has to go through all of that rather than reaching for the
// environment itself, which is what it used to do.
func TestNewEngineTakesItsToolsTheSameWay(t *testing.T) {
	t.Setenv("FRAMEFAIRY_FFMPEG", "/named/ffmpeg")
	t.Setenv("FRAMEFAIRY_FFPROBE", "/named/ffprobe")
	e := NewEngine(runlog.New(os.Stderr, false, false), afero.NewMemMapFs())
	if e.FFmpeg != "/named/ffmpeg" || e.FFprobe != "/named/ffprobe" {
		t.Errorf("ffmpeg %q, ffprobe %q", e.FFmpeg, e.FFprobe)
	}
}

// llama-server is decided the same way ffmpeg is, and for the same reason:
// a customer has no Homebrew and no terminal, so the only one they will
// ever have is the one shipped beside the program. It used to be looked
// for on the search path alone, which is a search path a shipped app does
// not have.
//
// On the real disk: llama-server is a program that is found on the disk and run.
func TestLlamaServerIsFoundTheSameWayAsFfmpeg(t *testing.T) {
	t.Run("a named one is taken as it stands", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", "/somewhere/else/llama-server")
		if got := LlamaServerPath(); got != "/somewhere/else/llama-server" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("otherwise nothing, never the search path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", "")
		if got := LlamaServerPath(); got != "" {
			t.Errorf("got %q from somewhere other than beside the program", got)
		}
		if HasLlamaServer() {
			t.Error("said yes with nothing beside the program")
		}
	})

	// What HasLlamaServer answers has to follow from that and from nothing
	// else, or the setup screen and the thing that starts the server can
	// disagree about the same machine.
	t.Run("what the setup asks follows the same path", func(t *testing.T) {
		t.Setenv("FRAMEFAIRY_LLAMA_SERVER", filepath.Join(t.TempDir(), "not-here"))
		if HasLlamaServer() {
			t.Error("said yes about a file that is not there")
		}
	})
}

// ffprobe is looked for beside an ffmpeg that was chosen, under the name
// the system gives it, and one named on its own wins. The app looked for
// it its own way and missed ffprobe.exe.
//
// On the real disk: a tool is a program that is found on the disk and run.
func TestFFprobeIsFoundBesideTheFFmpegChosen(t *testing.T) {
	for _, c := range []struct{ ffmpeg, ffprobe string }{
		{"ffmpeg", "ffprobe"},
		{"ffmpeg.exe", "ffprobe.exe"},
		{"FFMPEG.EXE", "ffprobe.exe"},
	} {
		dir := t.TempDir()
		for _, name := range []string{c.ffmpeg, c.ffprobe} {
			if err := standin.Write(filepath.Join(dir, name), nil, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		e := NewEngine(runlog.New(io.Discard, false, false), disk)
		e.UseTools(filepath.Join(dir, c.ffmpeg), "")
		if want := filepath.Join(dir, c.ffprobe); e.FFprobe != want {
			t.Errorf("beside %s: ffprobe is %s, want %s", c.ffmpeg, e.FFprobe, want)
		}
	}

	e := NewEngine(runlog.New(io.Discard, false, false), disk)
	was := e.FFprobe
	e.UseTools("", "")
	if e.FFprobe != was {
		t.Error("no ffmpeg chosen changed ffprobe")
	}
	e.UseTools(filepath.Join(t.TempDir(), "ffmpeg"), "/opt/ffprobe")
	if e.FFprobe != "/opt/ffprobe" {
		t.Errorf("an ffprobe named on its own lost to a guess: %s", e.FFprobe)
	}
}
