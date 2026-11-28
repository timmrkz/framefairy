package updates

// Order keeps, for each channel, when the newest build the app took of it
// was made, so a build made before it is never taken again, unasked or
// picked. Whoever can write the release can put back an older entry of a
// channel, signed as it was then, with a bug fixed since. The claim does
// not say when a build was made, so before this the app took whatever
// build its channel had when it was not the one running. The publish
// refuses an older commit by its git history, see publishPlan in
// cmd/framefairy-release, and this is the same rule where the build is
// installed, so a release written by anything else still cannot step
// the app back. Speaker Trail found it, and this is its design.

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"
)

// Order is the newest made time taken of each channel. Load and Save keep
// it, in the app a file of the store beside updates.json. With neither it
// keeps what it takes in memory only.
type Order struct {
	// Load reads what was kept. Save writes it whole. A Save that fails
	// costs at most a step back the file would have stopped, never an
	// update, so its error is not the update's.
	Load func() map[string]time.Time
	Save func(map[string]time.Time) error

	mu     sync.Mutex
	loaded bool
	made   map[string]time.Time
}

// Take says whether the build may be taken, and when it may, notes when
// it was made. A build signed as made before the newest taken of its
// channel is refused. One without a signed time is taken only while
// nothing of its channel has been taken with one, so the entries from
// before orders were signed still work until their next build.
func (o *Order) Take(public ed25519.PublicKey, b Build) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.load()
	newest, known := o.made[b.Channel]
	err := VerifyOrder(public, b)
	switch {
	case errors.Is(err, errNoOrder) && !known:
		return nil
	case errors.Is(err, errNoOrder):
		return fmt.Errorf("the build of %s %s, and this app already took one that was, so it is not installed", b.Channel, err)
	case err != nil:
		return err
	case b.Made.Before(newest):
		return fmt.Errorf("the build of %s was made %s, before the one this app already took, made %s, "+
			"so it is not installed", b.Channel, b.Made.UTC().Format(time.RFC3339), newest.UTC().Format(time.RFC3339))
	}
	if !b.Made.Equal(newest) {
		o.made[b.Channel] = b.Made.UTC()
		if o.Save != nil {
			_ = o.Save(maps.Clone(o.made))
		}
	}
	return nil
}

func (o *Order) load() {
	if o.loaded {
		return
	}
	o.loaded = true
	o.made = map[string]time.Time{}
	if o.Load == nil {
		return
	}
	for ch, t := range o.Load() {
		if ValidChannel(ch) && !t.IsZero() {
			o.made[ch] = t
		}
	}
}
