package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine/runlog"
)

// A model that does not fit says so in two words, which fit the row of a
// search, then what it needs, what is free, and what to do, naming the
// llama-servers that hold memory. A search that ends on it says it as it
// is, without "planning failed" in front.
func TestTooLittleMemorySaysWhatToDo(t *testing.T) {
	err := tooLittleMemory(17_900_000_000, 9_500_000_000, []string{"4534"})
	said := err.Error()
	first, _, _ := strings.Cut(said, ".")
	if first != "not enough memory" || !errors.Is(err, ErrNoRoom) {
		t.Errorf("first sentence %q", first)
	}
	for _, want := range []string{"needs about 17.9 GB and 9.5 GB are free", "process 4534"} {
		if !strings.Contains(said, want) {
			t.Errorf("no %q in %q", want, said)
		}
	}
	// The command line can stop it, the app's person is told no command.
	if line := ForCommandLine(err); !strings.Contains(line, "kill 4534") || strings.Contains(said, "kill") {
		t.Errorf("the app says %q and the command line %q", said, line)
	}
	if said := tooLittleMemory(2e9, 1e9, nil).Error(); !strings.Contains(said, "Quit other programs") {
		t.Errorf("without a server: %q", said)
	}
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), NewMachine(afero.NewMemMapFs()))
	if got := e.planFailed(context.Background(), err); got.Error() != said {
		t.Errorf("a search ended on it says %q", got)
	}
	if got := e.planFailed(context.Background(), errors.New("x")); got.Error() != "planning failed: x" {
		t.Errorf("any other reason says %q", got)
	}
}

// A search whose model does not fit in the memory free now fails before
// it listens to its window, which can take minutes, and says why. Its
// machine has a megabyte free and another llama-server running.
//
// On the real disk: ffmpeg reads the episode, and the model is a file on the disk.
func TestASearchThatCannotLoadItsModelFailsBeforeListening(t *testing.T) {
	t.Parallel()
	source := testEpisode(t, "40")
	var heard int32
	machine := NewMachine(disk)
	machine.free = func() int64 { return 1 << 20 }
	machine.servers = func() []string { return []string{"4534"} }
	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), machine)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.Planner, base.LLMURL, base.LLMServer, base.LLMModel = "local", "", "true", os.Args[0]
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	_, err := p.Search(context.Background(), PlanRequest{To: 40, Count: 1}, Host{})
	if !errors.Is(err, ErrNoRoom) || !strings.Contains(err.Error(), "process 4534") {
		t.Fatalf("the search ended on %v", err)
	}
	if n := atomic.LoadInt32(&heard); n != 0 {
		t.Errorf("it listened %d times first", n)
	}
	// With the API there is no model to load here.
	base.Planner = "api"
	if err := e.roomBeforeSearch(base, 1800); err != nil {
		t.Errorf("an API search: %v", err)
	}
}

// ps lists a process a line, its number and its program, a full path on
// macOS. Only llama-server is picked, wherever it lives.
func TestTheLlamaServersRunning(t *testing.T) {
	listing := "  1 /sbin/launchd\n4534 /Users/tim/framefairy/bin/llama-server\n 77 /bin/zsh\n" +
		"901 /Users/a b/bin/llama-server\n902 llama-server-helper\n"
	got := llamaServers(listing)
	if strings.Join(got, ",") != "4534,901" {
		t.Errorf("found %v", got)
	}
}

// A model the engine knows needs what it is judged by at that context.
// One it does not know needs its file at least, and one that is not there
// is not judged.
func TestWhatAModelNeeds(t *testing.T) {
	gemma, _ := LanguageModelByName("gemma-4-26B_q4_0-it.gguf")
	if got := modelNeeds("/models/gemma-4-26B_q4_0-it.gguf", 32768); got != gemma.NeedsAt(32768) {
		t.Errorf("Gemma needs %d", got)
	}
	if got := modelNeeds("/nowhere/own.gguf", 32768); got != 0 {
		t.Errorf("a missing model needs %d", got)
	}
	if got := modelNeeds(os.Args[0], 4096); got <= workingAllowance {
		t.Errorf("a file of its own needs %d", got)
	}
}

// What is free is asked of the machine the tests run on, never more than
// it has. On Linux and macOS it says.
func TestTheMemoryFreeNow(t *testing.T) {
	free := FreeMemory()
	if total := MachineMemory(); free < 0 || (total > 0 && free > total) {
		t.Errorf("%d free of %d", free, total)
	}
	// Windows will not say, macOS and Linux do.
	if runtime.GOOS != "windows" && free == 0 {
		t.Error("the machine did not say what is free")
	}
	t.Logf("%s free", inGB(free))
}
