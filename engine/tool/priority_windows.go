package tool

import (
	"os"

	"golang.org/x/sys/windows"
)

// Lower runs a program that has started below the normal priority, so the
// work somebody waits on, like the hearing, goes first when the cores are
// taken. See priority_unix.go.
func Lower(p *os.Process) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetPriorityClass(h, windows.BELOW_NORMAL_PRIORITY_CLASS)
}
