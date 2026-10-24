package waits

import (
	"context"
	"testing"
	"time"
)

// Every way of waiting on the clock that make lint refuses in a test.
func TestWaits(t *testing.T) {
	time.Sleep(time.Millisecond)
	<-time.After(time.Millisecond)
	_ = time.Tick(time.Millisecond)
	time.NewTimer(time.Second).Stop()
	time.NewTicker(time.Second).Stop()
	time.AfterFunc(time.Second, func() {}).Stop()
	// A context with a deadline is a timer by another name.
	_, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	_, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	cancel()
}
