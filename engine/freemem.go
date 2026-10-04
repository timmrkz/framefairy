package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Whether a model fits in the memory that is free now
//
// A model is offered by the memory the machine has, see FitsIn. What is
// free when a search starts is another matter: a llama-server a run left
// behind, or any program holding memory, can leave too little. macOS then
// kills the new llama-server while it loads, and all a search could say
// was that it stopped. On 3 October a comparison failed on every side that
// way, with a server from an earlier run holding 16 GB of a 32 GB Mac.
//
// So before llama-server starts, what the model needs at the context it is
// started with is held against the memory the system can give it now. One
// that does not fit is not started, and the search says why in a line
// that fits its row: the first sentence is short, the rest is for hover.
// ---------------------------------------------------------------------------

// freeMemory is how much memory the system can give a program now, in
// bytes, or zero when it will not say. A variable, so a test can be any
// machine in any state.
var freeMemory = FreeMemory

// FreeMemory is how much memory the system can give a program now, in
// bytes, or zero when it cannot tell. Memory the system only uses to cache
// files counts as free, because it gives that back as soon as it is asked,
// and on macOS so does what other programs hold, see vmStatFree.
func FreeMemory() int64 {
	switch runtime.GOOS {
	case "darwin":
		res := run(context.Background(), "", "vm_stat")
		if res.Code != 0 {
			return 0
		}
		return vmStatFree(res.Stdout, MachineMemory())
	case "linux":
		return meminfo("/proc/meminfo", "MemAvailable:")
	}
	return 0
}

// vmStatFree reads what vm_stat says into the memory that can be had: all
// of it, less what the system has wired down and what the compressor
// holds. Wired is where a model sits once Metal has it, and nothing can
// have it until that program lets go. The memory other programs hold is
// not counted: macOS compresses it, or moves it to disk, the moment a
// model asks for room. It was counted at first, the way Activity Monitor
// counts the memory used, and a 32 GB Mac with Chrome open and its memory
// pressure green had 12.8 GB "free" for a model of 17. Zero when the text
// is not what vm_stat writes.
func vmStatFree(text string, total int64) int64 {
	if total <= 0 {
		return 0
	}
	page := int64(0)
	if _, rest, ok := strings.Cut(text, "page size of "); ok {
		if n, err := strconv.ParseInt(strings.Fields(rest)[0], 10, 64); err == nil {
			page = n
		}
	}
	if page <= 0 {
		return 0
	}
	pages := map[string]int64{}
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64)
		if err == nil {
			pages[strings.TrimSpace(name)] = n
		}
	}
	wired, ok1 := pages["Pages wired down"]
	compressed, ok2 := pages["Pages occupied by compressor"]
	if !ok1 || !ok2 {
		return 0
	}
	return max(total-(wired+compressed)*page, 0)
}

// modelNeeds is the memory a model takes started with a context of ctx
// tokens: what the engine knows of it, or for a model it does not know
// its file, which it takes at least.
func modelNeeds(model string, ctx int) int64 {
	if m, ok := LanguageModelByName(filepath.Base(model)); ok {
		return m.NeedsAt(ctx)
	}
	if info, err := os.Stat(model); err == nil {
		return info.Size() + workingAllowance
	}
	return 0
}

// roomForModel says why a model cannot be started with a context of ctx
// tokens in the memory that is free now, or nothing when it can, or when
// the system does not say what is free.
func roomForModel(model string, ctx int) error {
	free := freeMemory()
	need := modelNeeds(model, ctx)
	if free <= 0 || need <= 0 || need <= free {
		return nil
	}
	return tooLittleMemory(need, free, otherServers())
}

// ErrNoRoom is a model that does not fit in the memory free now. A search
// that ends on it says so as it is, without "planning failed" in front,
// so its first sentence fits the row of the search.
var ErrNoRoom = errors.New("not enough memory")

// tooLittleMemory says that a model needing need bytes does not fit in
// free, and what to do about it: stop the llama-servers that are running,
// by their process numbers, or quit other programs. The first sentence is
// the one the row of a search shows, so it is two words, and the rest is
// read on hover.
func tooLittleMemory(need, free int64, servers []string) error {
	what := "Quit other programs, or choose a smaller model."
	if len(servers) > 0 {
		what = "Another llama-server holds memory, process " + strings.Join(servers, ", ") +
			". Quit the program that started it, or stop it with kill " + strings.Join(servers, " ") + "."
	}
	return fmt.Errorf("%w. The model needs about %s and %s are free. %s",
		ErrNoRoom, inGB(need), inGB(free), what)
}

// roomBeforeSearch says whether the local model a search will ask fits in
// the memory free now, before the search transcribes its window, so a
// search that cannot finish fails at once rather than after minutes of
// listening. The window is seconds long, and the context is reckoned the
// way a warm-up reckons it. A model this program holds already, loaded or
// loading, is not checked: its memory is counted as used, and the engine
// checks again when it starts a server, see startServer.
func roomBeforeSearch(opts Options, seconds float64) error {
	if opts.Planner != "local" || opts.LLMURL != "" {
		return nil
	}
	local, err := resolveLocal(opts)
	if err != nil {
		return nil
	}
	host.mu.Lock()
	held := host.model != nil
	host.mu.Unlock()
	if held {
		return nil
	}
	chars := int(max(seconds, 60)*warmChars) + runeLen(SystemPrompt)
	return roomForModel(local.Model, localContextFor(local.Model, chars, opts.MaxTokens))
}

// inGB is a size in gigabytes the way the app says a model's size, in
// thousands, to a tenth.
func inGB(n int64) string {
	return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + " GB"
}

// otherServers is the process numbers of the llama-servers running now.
// Before a model is started none of this program's own is, so any there
// is holds memory somebody else started it for.
var otherServers = func() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	res := run(context.Background(), "", "ps", "-axo", "pid=,comm=")
	if res.Code != 0 {
		return nil
	}
	return llamaServers(res.Stdout)
}

// llamaServers picks the process numbers of llama-server out of what ps
// lists, one process a line, its number and then its program.
func llamaServers(listing string) []string {
	var pids []string
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && filepath.Base(fields[len(fields)-1]) == "llama-server" {
			pids = append(pids, fields[0])
		}
	}
	return pids
}
