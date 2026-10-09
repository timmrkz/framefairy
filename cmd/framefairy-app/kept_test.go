package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// An episode opening asks for its transcript from several calls at once,
// and it is read once. Going back to one of the last few episodes reads
// nothing, one more lets go of the one asked for longest ago, a file that
// changed is read again, and a failure is not kept.
func TestTranscriptsAreReadOnceAndKeptForTheLastEpisodes(t *testing.T) {
	var k keptReads[*int]
	// a and the others fill what is kept, and one more comes after.
	others := []string{}
	for i := 1; i < keptTranscripts; i++ {
		others = append(others, fmt.Sprintf("k%d", i))
	}
	reads := map[string]*atomic.Int32{}
	for _, key := range append(others, "a", "more") {
		reads[key] = &atomic.Int32{}
	}
	read := func(key string) func() (*int, error) {
		return func() (*int, error) {
			reads[key].Add(1)
			time.Sleep(20 * time.Millisecond)
			n := len(key)
			return &n, nil
		}
	}

	var wg sync.WaitGroup
	got := make([]*int, 8)
	for i := range got {
		wg.Go(func() { got[i], _ = k.get("a", "1", read("a")) })
	}
	wg.Wait()
	if n := reads["a"].Load(); n != 1 {
		t.Errorf("eight asks at once read %d times, wanted once", n)
	}
	for _, p := range got {
		if p != got[0] {
			t.Fatal("the asks at once were given different reads")
		}
	}

	for _, key := range others {
		_, _ = k.get(key, "1", read(key))
	}
	// All are kept: asking any again reads nothing. a is asked last, so
	// the first of the others is the one asked for longest ago when one
	// more comes.
	for _, key := range append(others, "a") {
		_, _ = k.get(key, "1", read(key))
	}
	_, _ = k.get("more", "1", read("more"))
	for key, n := range reads {
		if n.Load() != 1 {
			t.Errorf("%s was read %d times, wanted once", key, n.Load())
		}
	}
	first := others[0]
	_, _ = k.get(first, "1", read(first))
	_, _ = k.get("a", "1", read("a"))
	if reads[first].Load() != 2 || reads["a"].Load() != 1 {
		t.Errorf("after one more, %s read %d times and a %d, wanted %s again and a kept", first, reads[first].Load(), reads["a"].Load(), first)
	}
	_, _ = k.get("a", "2", read("a"))
	if reads["a"].Load() != 2 {
		t.Error("a changed file was not read again")
	}

	fails := 0
	broken := func() (*int, error) { fails++; return nil, errors.New("no transcript yet") }
	for range 2 {
		if _, err := k.get("f", "1", broken); err == nil {
			t.Error("a failure came back as an answer")
		}
	}
	if fails != 2 {
		t.Errorf("a failure was read %d times in two asks, wanted it read each time", fails)
	}
}

// Everything at once on many keys, under the race detector.
func TestKeptReadsFromEverywhereAtOnce(t *testing.T) {
	var k keptReads[string]
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Go(func() {
			key := fmt.Sprint(i % 7)
			stamp := fmt.Sprint(i % 3)
			v, err := k.get(key, stamp, func() (string, error) { return key + stamp, nil })
			if err != nil || len(v) == 0 {
				t.Errorf("%s gave %q, %v", key, v, err)
			}
		})
	}
	wg.Wait()
}

// A read that panics lets go of everyone waiting on it, and is not kept.
func TestAKeptReadThatPanicsLetsGoOfItsWaiters(t *testing.T) {
	var k keptReads[int]
	started := make(chan struct{})
	go func() {
		defer func() { _ = recover() }()
		_, _ = k.get("a", "1", func() (int, error) {
			close(started)
			time.Sleep(50 * time.Millisecond)
			panic("read")
		})
	}()
	<-started
	if _, err := k.get("a", "1", func() (int, error) { return 0, nil }); err == nil {
		t.Fatal("a waiter on a read that panicked got no error")
	}
	if v, err := k.get("a", "1", func() (int, error) { return 7, nil }); err != nil || v != 7 {
		t.Fatalf("after a panic the key gave %d, %v, not a new read", v, err)
	}
}
