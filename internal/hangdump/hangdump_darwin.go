package hangdump

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
)

// stacks writes the stack of each program, the way macOS's sample tool
// sees it. Every program is sampled at once, so a slow one, or one sample
// never comes back from, holds up none of the others.
func stacks(pids []int, part func(format string, a ...any)) {
	var wg sync.WaitGroup
	for _, pid := range pids {
		wg.Go(func() {
			part("\n== process %d:\n%s", pid, sample(pid))
		})
	}
	wg.Wait()
}

// sample is the stack of every thread of pid, sampled for a second by
// macOS's sample tool, or why there is none.
//
// sample writes its report to a file, read back and removed here.
func sample(pid int) string {
	dir, err := os.MkdirTemp("", "hangdump-")
	if err != nil {
		return fmt.Sprintf("no sample: %v\n", err)
	}
	defer os.RemoveAll(dir)
	report := filepath.Join(dir, "sample.txt")
	var said bytes.Buffer
	cmd := exec.Command("sample", strconv.Itoa(pid), "1", "-mayDie", "-file", report)
	cmd.Stdout, cmd.Stderr = &said, &said
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("no sample: %v, %s\n", err, said.String())
	}
	b, err := os.ReadFile(report)
	if err != nil {
		return fmt.Sprintf("no sample: %v, %s\n", err, said.String())
	}
	return string(b)
}
