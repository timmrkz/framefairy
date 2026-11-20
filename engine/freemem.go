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

	"framefairy/engine/tool"
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
// bytes, or zero when it will not say, see Machine.free.
func (m *Machine) freeMemory() int64 {
	if m.free != nil {
		return m.free()
	}
	return FreeMemory()
}

// FreeMemory is how much memory the system can give a program now, in
// bytes, or zero when it cannot tell. Memory the system only uses to cache
// files counts as free, because it gives that back as soon as it is asked,
// and on macOS so does what other programs hold, see vmStatFree.
func FreeMemory() int64 { return systemFreeMemory() }

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
func (m *Machine) roomForModel(model string, ctx int) error {
	// A model whose needs are not known is not checked, so the system is
	// not asked what is free.
	need := modelNeeds(model, ctx)
	if need <= 0 {
		return nil
	}
	free := m.freeMemory()
	if free <= 0 || need <= free {
		return nil
	}
	return tooLittleMemory(need, free, m.otherServers())
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
	said := fmt.Sprintf(". The model needs about %s and %s are free.", inGB(need), inGB(free))
	way := twoWays(true, said+" Quit other programs, or choose a smaller clip model in the settings.",
		said+" Quit other programs, or choose a smaller model.")
	if len(servers) > 0 {
		held := said + " Another llama-server holds memory, process " + strings.Join(servers, ", ") +
			". Quit the program that started it"
		way = twoWays(true, held+".", held+", or stop it with kill "+strings.Join(servers, " ")+".")
	}
	return fmt.Errorf("%w%w", ErrNoRoom, way)
}

// roomBeforeSearch says whether the local model a search will ask fits in
// the memory free now, before the search transcribes its window, so a
// search that cannot finish fails at once rather than after minutes of
// listening. The window is seconds long, and the context is reckoned the
// way a warm-up reckons it. A model this program holds already, loaded or
// loading, is not checked: its memory is counted as used, and the engine
// checks again when it starts a server, see startServer.
func (e *Engine) roomBeforeSearch(opts Options, seconds float64) error {
	if opts.Planner != "local" || opts.LLMURL != "" {
		return nil
	}
	local, err := e.resolveLocal(opts)
	if err != nil {
		return nil
	}
	e.host.mu.Lock()
	held := e.host.model != nil
	e.host.mu.Unlock()
	if held {
		return nil
	}
	return e.roomForModel(local.Model, e.warmSize(local.Model, seconds, opts.MaxTokens))
}

// inGB is a size in gigabytes the way the app says a model's size, in
// thousands, to a tenth.
func inGB(n int64) string {
	return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + " GB"
}

// otherServers is the process numbers of the llama-servers running now,
// see Machine.servers. Before a model is started none of this program's
// own is, so any there is holds memory somebody else started it for.
func (m *Machine) otherServers() []string {
	if m.servers != nil {
		return m.servers()
	}
	return runningServers()
}

// runningServers is the process numbers of the llama-servers the system
// runs now.
func runningServers() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	res := tool.Runner{}.Run(context.Background(), "", "ps", "-axo", "pid=,comm=")
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
