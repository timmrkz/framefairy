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
// nothing, a fifth lets go of the one asked for longest ago, a file that
// changed is read again, and a failure is not kept.
func TestTranscriptsAreReadOnceAndKeptForTheLastEpisodes(t *testing.T) {
	var k keptReads[*int]
	reads := map[string]*atomic.Int32{}
	for _, key := range []string{"a", "b", "c", "d", "e"} {
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

	for _, key := range []string{"b", "c", "d"} {
		_, _ = k.get(key, "1", read(key))
	}
	// a, b, c and d are kept: asking any again reads nothing. a is asked
	// last, so b is the one asked for longest ago when e comes.
	for _, key := range []string{"b", "c", "d", "a"} {
		_, _ = k.get(key, "1", read(key))
	}
	_, _ = k.get("e", "1", read("e"))
	for key, want := range map[string]int32{"a": 1, "b": 1, "c": 1, "d": 1, "e": 1} {
		if n := reads[key].Load(); n != want {
			t.Errorf("%s was read %d times, wanted %d", key, n, want)
		}
	}
	_, _ = k.get("b", "1", read("b"))
	_, _ = k.get("a", "1", read("a"))
	if reads["b"].Load() != 2 || reads["a"].Load() != 1 {
		t.Errorf("after a fifth, b read %d times and a %d, wanted b again and a kept", reads["b"].Load(), reads["a"].Load())
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
