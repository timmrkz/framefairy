package signals

import "time"

// Code that is not a test may keep time.
func wait() { time.Sleep(time.Millisecond) }
