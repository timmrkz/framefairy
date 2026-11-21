package engine

import (
	"context"
	"strconv"
	"strings"

	"framefairy/engine/tool"
	"golang.org/x/sys/unix"
)

// systemMemory is hw.memsize, how much memory the Mac has, in bytes, or
// zero when it will not say. It is asked of the kernel itself: the sysctl
// command it was asked through was a program started for every read of
// the setup.
func systemMemory() int64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return int64(n)
}

// systemFreeMemory is what vm_stat says can be had, see vmStatFree.
func systemFreeMemory() int64 {
	res := tool.Runner{}.Run(context.Background(), "", "vm_stat")
	if res.Code != 0 {
		return 0
	}
	return vmStatFree(res.Stdout, MachineMemory())
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
