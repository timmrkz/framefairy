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
)

// vm_stat as it writes on Apple silicon, pages of 16 KB, with numbers made
// up for a 32 GB Mac where a llama-server already holds a model: 17.9 GB
// wired, 4.5 GB held by programs and 1.6 GB by the compressor.
const vmStatHeld = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                                9871.
Pages active:                            401233.
Pages inactive:                          389102.
Pages speculative:                         7215.
Pages throttled:                              0.
Pages wired down:                       1093422.
Pages purgeable:                          12044.
"Translation faults":                 812345678.
Pages copy-on-write:                   23456789.
Pages zero filled:                    345678901.
Pages reactivated:                      1234567.
Pages purged:                            234567.
File-backed pages:                       512301.
Anonymous pages:                         285249.
Pages stored in compressor:              301234.
Pages occupied by compressor:             98765.
Decompressions:                         1234567.
Compressions:                           2345678.
Pageins:                                3456789.
Pageouts:                                 45678.
Swapins:                                      0.
Swapouts:                                     0.
`

// The memory that can be had is all of it, less what programs hold for
// themselves, what is wired down and what the compressor holds. Files
// cached and memory that can be purged count as free.
func TestWhatVmStatSaysIsFree(t *testing.T) {
	const total = 32 << 30
	page := int64(16384)
	held := ((285249 - 12044) + 1093422 + 98765) * page
	if got := vmStatFree(vmStatHeld, total); got != total-held {
		t.Errorf("free %d, want %d", got, total-held)
	}
	// About 10.4 GB, too little for Gemma 4 26B A4B beside the one loaded.
	gemma, _ := LanguageModelByName("gemma-4-26B_q4_0-it.gguf")
	if free := vmStatFree(vmStatHeld, total); gemma.NeedsAt(32768) <= free {
		t.Errorf("Gemma fits in %s beside itself", inGB(free))
	}
	for _, bad := range []string{"", "Pages free: 12.", strings.ReplaceAll(vmStatHeld, "Anonymous pages", "Other")} {
		if got := vmStatFree(bad, total); got != 0 {
			t.Errorf("%q read as %d free", bad, got)
		}
	}
	if got := vmStatFree(vmStatHeld, 0); got != 0 {
		t.Errorf("a machine of no size has %d free", got)
	}
}

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
	for _, want := range []string{"needs about 17.9 GB and 9.5 GB are free", "process 4534", "kill 4534"} {
		if !strings.Contains(said, want) {
			t.Errorf("no %q in %q", want, said)
		}
	}
	if said := tooLittleMemory(2e9, 1e9, nil).Error(); !strings.Contains(said, "Quit other programs") {
		t.Errorf("without a server: %q", said)
	}
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	if got := e.planFailed(context.Background(), err); got.Error() != said {
		t.Errorf("a search ended on it says %q", got)
	}
	if got := e.planFailed(context.Background(), errors.New("x")); got.Error() != "planning failed: x" {
		t.Errorf("any other reason says %q", got)
	}
}

// A search whose model does not fit in the memory free now fails before
// it listens to its window, which can take minutes, and says why. Not in
// parallel: it puts its own machine in place of the one the tests run on,
// and the tests that run in parallel wait until it is done.
func TestASearchThatCannotLoadItsModelFailsBeforeListening(t *testing.T) {
	wasFree, wasServers := freeMemory, otherServers
	freeMemory = func() int64 { return 1 << 20 }
	otherServers = func() []string { return []string{"4534"} }
	t.Cleanup(func() { freeMemory, otherServers = wasFree, wasServers })

	source := testEpisode(t, "40")
	var heard int32
	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.Planner, base.LLMURL, base.LLMServer, base.LLMModel = "local", "", "true", os.Args[0]
	base.ASRModel = t.TempDir()
	p := NewProject(e, source, base)
	_, err := p.Search(context.Background(), PlanRequest{To: 40, Count: 1}, nil)
	if !errors.Is(err, ErrNoRoom) || !strings.Contains(err.Error(), "kill 4534") {
		t.Fatalf("the search ended on %v", err)
	}
	if n := atomic.LoadInt32(&heard); n != 0 {
		t.Errorf("it listened %d times first", n)
	}
	// With the API there is no model to load here.
	base.Planner = "api"
	if err := roomBeforeSearch(base, 1800); err != nil {
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
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && free == 0 {
		t.Error("the machine did not say what is free")
	}
	t.Logf("%s free", inGB(free))
}
