package main

// wake tells whoever waits that something changed. A waiter takes the
// channel and waits on it, and the next change closes it. The lanes, the
// queue and the updates each hold one, and their own lock is held for both
// calls, so a change between a waiter's look and its wait still reaches it.
//
// The channel is made only when somebody waits. A job's progress changes
// it several times a second, and almost always nobody waits.
type wake struct {
	c chan struct{}
}

// wakeLocked tells everyone waiting that something changed.
func (w *wake) wakeLocked() {
	if w.c != nil {
		close(w.c)
		w.c = nil
	}
}

// waitLocked is the channel the next change closes.
func (w *wake) waitLocked() <-chan struct{} {
	if w.c == nil {
		w.c = make(chan struct{})
	}
	return w.c
}
