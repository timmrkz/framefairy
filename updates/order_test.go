package updates

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"
)

// made signs b as made at t.
func made(key ed25519.PrivateKey, b Build, t time.Time) Build {
	b.Made = t
	b.Order = SignOrder(key, b)
	return b
}

// kept is what an Order keeps between two starts of the app, in memory,
// the way the app's store keeps it in a file.
type kept struct {
	mu    sync.Mutex
	made  map[string]time.Time
	saves int
}

func (k *kept) order() *Order {
	return &Order{
		Load: func() map[string]time.Time {
			k.mu.Lock()
			defer k.mu.Unlock()
			return maps.Clone(k.made)
		},
		Save: func(m map[string]time.Time) error {
			k.mu.Lock()
			defer k.mu.Unlock()
			k.made, k.saves = m, k.saves+1
			return nil
		},
	}
}

// The order says when a build was made, with which build it is. Move the
// time, change what the claim says, or leave it out, and it no longer
// holds.
func TestAnOrderSaysWhenWhichBuildWasMade(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	public := key.Public().(ed25519.PublicKey)
	at := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	b := made(key, build(t, key, "pr-5", "0.3.0-pr5.1", "https://example.com/pr-5.zip", appZip(t, "x")), at)
	if err := VerifyOrder(public, b); err != nil {
		t.Fatalf("the order made for it does not hold: %v", err)
	}
	for name, change := range map[string]func(*Build){
		"made later":      func(b *Build) { b.Made = b.Made.Add(time.Hour) },
		"another channel": func(b *Build) { b.Channel = "main" },
		"another version": func(b *Build) { b.Version = "9.9.9" },
		"another zip":     func(b *Build) { b.SHA256 = strings.Repeat("0", 64) },
		"no order":        func(b *Build) { b.Order = "" },
		"a claim instead": func(b *Build) { b.Order = b.Claim },
	} {
		c := b
		change(&c)
		if err := VerifyOrder(public, c); err == nil {
			t.Errorf("%s: the order still held", name)
		}
	}
}

// Once a build of a channel was taken, one made before it is refused, and
// so is one of that channel without a signed time. A newer one is taken,
// and what was taken is still known when the app starts again. Another
// channel is not held to it.
func TestABuildMadeBeforeOneTakenIsRefused(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	public := key.Public().(ed25519.PublicKey)
	at := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	zip := appZip(t, "x")
	of := func(channel, version string, when time.Time) Build {
		return made(key, build(t, key, channel, version, "https://example.com/"+channel+".zip", zip), when)
	}
	k := &kept{}
	o := k.order()
	// Before anything with a time was taken, an entry from before orders
	// is taken, so the builds published before this change still work.
	if err := o.Take(public, build(t, key, "main", "0.1.0-main.1", "https://example.com/main.zip", zip)); err != nil {
		t.Fatalf("an entry from before orders was refused: %v", err)
	}
	if err := o.Take(public, of("main", "0.1.0-main.2", at)); err != nil {
		t.Fatal(err)
	}
	if err := o.Take(public, of("main", "0.1.0-main.1", at.Add(-time.Minute))); err == nil {
		t.Error("an older build of main was taken after a newer one")
	}
	if err := o.Take(public, build(t, key, "main", "0.1.0-main.0", "https://example.com/main.zip", zip)); err == nil {
		t.Error("an entry without a signed time was taken after one with")
	}
	if err := o.Take(public, of("pr-5", "0.1.0-pr5.1", at.Add(-time.Hour))); err != nil {
		t.Errorf("another channel was held to main's time: %v", err)
	}
	if err := o.Take(public, of("main", "0.1.0-main.3", at.Add(time.Minute))); err != nil {
		t.Errorf("a newer build was refused: %v", err)
	}
	again := k.order()
	if err := again.Take(public, of("main", "0.1.0-main.2", at)); err == nil {
		t.Error("after a start, a build older than the one taken was taken")
	}
	forged := of("main", "0.1.0-main.9", at.Add(time.Hour))
	forged.Made = forged.Made.Add(time.Hour)
	if err := again.Take(public, forged); err == nil {
		t.Error("a build whose time was moved was taken")
	}
	// The same build taken again moves nothing, so nothing is written.
	before := k.saves
	if err := again.Take(public, of("main", "0.1.0-main.3", at.Add(time.Minute))); err != nil {
		t.Errorf("the build taken last was refused: %v", err)
	}
	if k.saves != before {
		t.Errorf("taking the same build again wrote the order %d times", k.saves-before)
	}
}

// The source asks the order before it offers a build, so an older entry
// put back on the release is never downloaded.
func TestTheSourceOffersNoBuildOlderThanOneTaken(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	at := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	s := &served{zip: appZip(t, "main")}
	srv := serve(t, s)
	newer := made(key, build(t, key, "main", "0.3.0-main.5", srv.URL+"/dev/app.zip", s.zip), at)
	older := made(key, build(t, key, "main", "0.3.0-main.4", srv.URL+"/dev/app.zip", s.zip), at.Add(-time.Hour))
	src := &Source{URL: srv.URL + "/dev/channels.json", Client: srv.Client(), Own: "main",
		Key: key.Public().(ed25519.PublicKey), Order: &Order{}}
	s.list = listJSON(t, newer)
	if rel, err := newUpdater(t, src, "0.3.0-main.3", key).Check(context.Background()); err != nil || rel == nil {
		t.Fatalf("the newer build was not offered: %v, %v", rel, err)
	}
	s.list = listJSON(t, older)
	rel, err := newUpdater(t, src, "0.3.0-main.5", key).Check(context.Background())
	if err == nil || rel != nil {
		t.Fatalf("an older build put back was offered: %v, %v", rel, err)
	}
	if !strings.Contains(err.Error(), "before the one this app already took") {
		t.Errorf("refused for another reason: %v", err)
	}
}

// Checks for updates run on their own goroutines, the one on a timer and
// the one Check now starts, so the order is taken from several at once.
// Whatever order they come in, the newest time of each channel is what
// is kept, and nothing older is taken after it.
func TestTheOrderIsTakenFromSeveralGoroutinesAtOnce(t *testing.T) {
	t.Parallel()
	key := testKey(t)
	public := key.Public().(ed25519.PublicKey)
	at := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	zip := appZip(t, "x")
	const builds = 16
	var each []Build
	for _, channel := range []string{"main", "pr-5"} {
		for i := range builds {
			b := build(t, key, channel, fmt.Sprintf("0.1.0-%s.%d", channel, i), "https://example.com/"+channel+".zip", zip)
			each = append(each, made(key, b, at.Add(time.Duration(i)*time.Minute)))
		}
	}
	k := &kept{}
	o := k.order()
	var wg sync.WaitGroup
	for _, b := range each {
		wg.Go(func() { _ = o.Take(public, b) })
	}
	wg.Wait()
	newest := at.Add((builds - 1) * time.Minute)
	again := k.order()
	for _, channel := range []string{"main", "pr-5"} {
		if err := again.Take(public, made(key, build(t, key, channel, "0.1.0-old", "https://example.com/old.zip", zip), newest.Add(-time.Second))); err == nil {
			t.Errorf("%s: a build older than the newest taken was taken after a start", channel)
		}
		if err := again.Take(public, made(key, build(t, key, channel, "0.1.0-same", "https://example.com/same.zip", zip), newest)); err != nil {
			t.Errorf("%s: the newest time was not what was kept: %v", channel, err)
		}
	}
}
