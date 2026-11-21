package engine

import (
	"strings"
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

// The memory that can be had is all of it, less what is wired down and
// what the compressor holds. Files cached, and what other programs hold,
// count as free, because macOS gives both back when a model asks.
func TestWhatVmStatSaysIsFree(t *testing.T) {
	const total = 32 << 30
	page := int64(16384)
	held := (1093422 + 98765) * page
	if got := vmStatFree(vmStatHeld, total); got != total-held {
		t.Errorf("free %d, want %d", got, total-held)
	}
	// About 14.8 GB, too little for Gemma 4 26B A4B beside the one loaded.
	gemma, _ := LanguageModelByName("gemma-4-26B_q4_0-it.gguf")
	if free := vmStatFree(vmStatHeld, total); gemma.NeedsAt(32768) <= free {
		t.Errorf("Gemma fits in %s beside itself", inGB(free))
	}
	// Tim's Mac on 4 October with nothing loaded and Chrome open: 16.8 GB
	// of app memory, 2.0 GB wired and 0.4 GB compressed, memory pressure
	// green. Gemma fits.
	chrome := strings.NewReplacer("1093422.", "122700.", "285249.", "1030000.", "98765.", "21800.").Replace(vmStatHeld)
	if free := vmStatFree(chrome, total); gemma.NeedsAt(32768) > free {
		t.Errorf("Gemma does not fit in %s beside Chrome", inGB(free))
	}
	for _, bad := range []string{"", "Pages free: 12.", strings.ReplaceAll(vmStatHeld, "Pages wired down", "Other")} {
		if got := vmStatFree(bad, total); got != 0 {
			t.Errorf("%q read as %d free", bad, got)
		}
	}
	if got := vmStatFree(vmStatHeld, 0); got != 0 {
		t.Errorf("a machine of no size has %d free", got)
	}
}
