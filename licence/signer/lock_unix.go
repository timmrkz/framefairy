//go:build !windows

package signer

import (
	"errors"
	"os"
	"syscall"
)

// lock holds f for this program alone until unlock is called, and refuses
// at once when another program holds it. Two signers writing one record
// could each draw an ID the other already gave out. The system gives the
// lock back when a program ends, so a crash leaves nothing held.
func lock(f *os.File) (unlock func(), err error) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another signer has the record open")
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, nil
}
