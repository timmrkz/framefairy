package engine

import (
	"context"
	"slices"
	"sync"
)

// looks is how a window asks for what is under the hand and waits for it,
// the same for every long thing a hand moves over: the frames of the video
// preview, see Schedule, and the loudness of the clip timeline. The window
// says where the hand is, and the look waits until the work brings what it
// wants. The newest look its window sent wins, and one it took the place
// of is answered as overtaken. What the work does to get there is each
// one's own rule, asked again after every piece of work, since what each
// costs differs, see
// docs/decisions/2026-11-21-the-waveform-keeps-up-with-the-hand.md.
//
// It has no lock of its own: whoever keeps it holds theirs around every
// call, and hands it to wait.
type looks[L, A any] struct {
	// last is where the newest look stood among what its window sent.
	last    Sent
	waiting []*lookWaiter[L, A]
}

type lookWaiter[L, A any] struct {
	look   L
	answer chan lookAnswer[A]
}

type lookAnswer[A any] struct {
	got A
	err error
}

// newest says whether a look its window sent at sent is the newest it
// sent, and takes it as the newest then. An older one came after a newer
// one: requests reach the Go side in any order.
func (ls *looks[L, A]) newest(sent Sent) bool {
	if sent.before(ls.last) {
		return false
	}
	ls.last = sent
	return true
}

// add puts a look among those that wait.
func (ls *looks[L, A]) add(l L) *lookWaiter[L, A] {
	w := &lookWaiter[L, A]{look: l, answer: make(chan lookAnswer[A], 1)}
	ls.waiting = append(ls.waiting, w)
	return w
}

// wait waits for the answer of a look added, with mu, the keeper's lock,
// let go, and says whether it was answered. A look called off before its
// answer is taken from those that wait. One answered as it was called off
// has its answer all the same, which may hold what the keeper wants back,
// see Schedule.Give.
func (ls *looks[L, A]) wait(ctx context.Context, mu sync.Locker, w *lookWaiter[L, A]) (got A, answered bool, err error) {
	select {
	case a := <-w.answer:
		return a.got, true, a.err
	case <-ctx.Done():
		mu.Lock()
		defer mu.Unlock()
		if i := slices.Index(ls.waiting, w); i >= 0 {
			ls.waiting = slices.Delete(ls.waiting, i, i+1)
			return got, false, ctx.Err()
		}
		// Taken from those that wait only as it was answered, under the
		// same lock.
		a := <-w.answer
		return a.got, true, a.err
	}
}

// any says whether a look waits.
func (ls *looks[L, A]) any() bool { return len(ls.waiting) > 0 }

// answer answers every look that waits for which got has an answer, and
// keeps the others waiting.
func (ls *looks[L, A]) answer(got func(L) (A, bool)) {
	kept := ls.waiting[:0]
	for _, w := range ls.waiting {
		if a, ok := got(w.look); ok {
			w.answer <- lookAnswer[A]{got: a}
			continue
		}
		kept = append(kept, w)
	}
	clear(ls.waiting[len(kept):])
	ls.waiting = kept
}

// fail answers every look that waits for which why has a reason with it.
func (ls *looks[L, A]) fail(why func(L) error) {
	kept := ls.waiting[:0]
	for _, w := range ls.waiting {
		if err := why(w.look); err != nil {
			w.answer <- lookAnswer[A]{err: err}
			continue
		}
		kept = append(kept, w)
	}
	clear(ls.waiting[len(kept):])
	ls.waiting = kept
}

// overtake answers every look that waits with ErrOvertaken: the hand went
// elsewhere.
func (ls *looks[L, A]) overtake() {
	ls.fail(func(L) error { return ErrOvertaken })
}
