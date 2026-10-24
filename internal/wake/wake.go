// Package wake tells whoever waits that something changed. A waiter takes
// the channel and waits on it, and the next change closes it. The app's
// lanes, queue and updates each hold one, and so does the engine's model
// host. Their own lock is held for both calls, so a change between a
// waiter's look and its wait still reaches it.
//
// The channel is made only when somebody waits. A job's progress changes
// it several times a second, and almost always nobody waits.
package wake

// Wake is the news of one thing. Its zero value is ready to use.
type Wake struct {
	c chan struct{}
}

// WakeLocked tells everyone waiting that something changed. The owner's
// lock is held.
func (w *Wake) WakeLocked() {
	if w.c != nil {
		close(w.c)
		w.c = nil
	}
}

// WaitLocked is the channel the next change closes. The owner's lock is
// held.
func (w *Wake) WaitLocked() <-chan struct{} {
	if w.c == nil {
		w.c = make(chan struct{})
	}
	return w.c
}
