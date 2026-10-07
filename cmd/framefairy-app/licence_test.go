package main

import (
	"net/url"
	"strings"
	"testing"
)

const aKey = "FF1-AQB1hJkc8BOzIgABEgDpH6mFEY4jtWIXAkzbzbWarU9VYz24xu7iX9u50d-AnveI4fmll34Ra9oefyKed91c2yA2c7zhMUUoVQjCV_4O"

func TestKeyFromLink(t *testing.T) {
	good := []string{
		"framefairy://unlock?key=" + aKey,
		"framefairy://unlock/?key=" + aKey,
		"FrameFairy://unlock?key=" + url.QueryEscape(aKey),
		"framefairy://unlock?key=%20" + aKey + "%20",
	}
	for _, link := range good {
		k, err := keyFromLink(link)
		if err != nil || string(k) != aKey {
			t.Errorf("%q: %q %v", link, k, err)
		}
	}
	bad := []string{
		"",
		"https://unlock?key=" + aKey,
		"framefairy://open?key=" + aKey,
		"framefairy://unlock/elsewhere?key=" + aKey,
		"framefairy://unlock?key=" + aKey + "&then=more",
		"framefairy://unlock?key=" + aKey + "&key=" + aKey,
		"framefairy://unlock?key=" + aKey + "#frag",
		"framefairy://me@unlock?key=" + aKey,
		"framefairy:unlock?key=" + aKey,
		"framefairy://unlock?key=hello",
		"framefairy://unlock?key=FF1-" + url.QueryEscape("a/../b"),
		"framefairy://unlock?key=FF1-" + strings.Repeat("A", 600),
		"framefairy://unlock",
		"framefairy://unlock?key=" + aKey + strings.Repeat("A", 1100),
		"framefairy://un lock?key=" + aKey,
		"framefairy://unlock?key=" + aKey + "%zz",
		"framefairy://unlock?key=" + aKey + ";x",
	}
	for _, link := range bad {
		k, err := keyFromLink(link)
		if err == nil {
			t.Errorf("%q was taken as %q", link, k)
			continue
		}
		// The reason is logged, so it must not carry the key.
		if strings.Contains(err.Error(), aKey[len(aKey)-16:]) {
			t.Errorf("%q: the reason quotes the key: %v", link, err)
		}
	}
}

// A link waits for the settings, which take it once, and a link the app
// does not take leaves nothing waiting.
func TestALinkWaitsToBeTakenOnce(t *testing.T) {
	s := &FrameFairy{}
	s.openedWith("framefairy://unlock?key=" + aKey + "&more=1")
	if s.Licence().Waiting || s.TakeLicenceLink() != "" {
		t.Fatal("a refused link left a key waiting")
	}
	s.openedWith("framefairy://unlock?key=" + aKey)
	if !s.Licence().Waiting {
		t.Fatal("no key waiting after a link")
	}
	if got := s.TakeLicenceLink(); got != aKey {
		t.Fatalf("took %q", got)
	}
	if s.Licence().Waiting || s.TakeLicenceLink() != "" {
		t.Fatal("the key was handed over twice")
	}
}

func TestOnlyABuildFromTheCodeTakesTestKeys(t *testing.T) {
	ch, co := buildChannel, buildCommit
	t.Cleanup(func() { buildChannel, buildCommit = ch, co })
	buildChannel, buildCommit = "", ""
	if !testKeys() {
		t.Fatal("a build made here does not take test keys")
	}
	for _, b := range [][2]string{{"main", ""}, {"", "abc123"}, {"stable", "abc123"}} {
		buildChannel, buildCommit = b[0], b[1]
		if testKeys() {
			t.Fatalf("a build with channel %q and commit %q takes test keys", b[0], b[1])
		}
	}
}

func FuzzKeyFromLink(f *testing.F) {
	f.Add("framefairy://unlock?key=" + aKey)
	f.Add("framefairy://unlock?key=FF1-a&key=b")
	f.Add("framefairy://%zz")
	f.Fuzz(func(t *testing.T, link string) {
		k, err := keyFromLink(link)
		if err != nil {
			return
		}
		if !strings.HasPrefix(string(k), "FF1-") || len(k) > 512 || strings.ContainsAny(string(k), " /?&#%\n") {
			t.Fatalf("%q was taken as %q", link, k)
		}
	})
}
