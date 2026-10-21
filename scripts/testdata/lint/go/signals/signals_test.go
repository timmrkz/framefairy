package signals

import (
	"testing"
	"testing/synctest"
	"time"
)

// A test that waits on a signal, uses durations, or sleeps on the fake
// clock of a synctest bubble passes.
func TestSignals(t *testing.T) {
	done := make(chan struct{})
	go func() { wait(); close(done) }()
	<-done
	_ = 2 * time.Second
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Hour) //nolint:forbidigo // synctest bubble
	})
}
