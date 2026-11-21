//go:build unix

package tool

import (
	"os"

	"golang.org/x/sys/unix"
)

// Background is the nice value of a program that does work nobody waits
// on, so the work somebody waits on, like the hearing, goes first when the
// cores are taken. With cores to spare it runs as fast as ever. 10 is what
// nice gives when asked for no number.
const Background = 10

// Lower runs a program that has started at Background. A program cannot be
// raised again without the system's leave, so only one that stays in the
// background is lowered.
func Lower(p *os.Process) error {
	return unix.Setpriority(unix.PRIO_PROCESS, p.Pid, Background)
}
