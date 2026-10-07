package main

import (
	"context"
	"crypto/ed25519"
	"net/url"
	"strings"
	"testing"
	"time"

	"framefairy/engine"
	"framefairy/licence"
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

// fakeKeychain stands in for the keychain the licence row uses, so no
// test reaches the Mac's own.
type fakeKeychain struct{ key, about string }

func useFakeKeychain(t *testing.T) *fakeKeychain {
	t.Helper()
	f := &fakeKeychain{}
	read, keep, kept := savedLicence, saveLicence, keptLicence
	t.Cleanup(func() { savedLicence, saveLicence, keptLicence = read, keep, kept })
	savedLicence = func() (string, bool) { return f.about, f.key != "" }
	keptLicence = func(context.Context) (string, bool) { return f.key, f.key != "" }
	saveLicence = func(text string, test bool) (string, error) {
		if strings.TrimSpace(text) == "" {
			f.key, f.about = "", ""
			return "", nil
		}
		_, l, err := engine.CheckLicence(text, test)
		if err != nil {
			return "", err
		}
		f.key, f.about = text, engine.DescribeLicence(l)
		return f.about, nil
	}
	return f
}

// testKey is a key from the test signer, told apart by its ID.
func testKey(t *testing.T, id byte) string {
	t.Helper()
	k, err := licence.Sign(licence.Licence{
		Format: licence.Format,
		ID:     licence.ID{id},
		Signed: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}, ed25519.NewKeyFromSeed(licence.TestSeed()))
	if err != nil {
		t.Fatal(err)
	}
	return string(k)
}

// The buyer clicks Unlock in the mail, and a Mac with no key is unlocked
// by the link alone: the settings only say so.
func TestALinkUnlocksAMacWithNoKey(t *testing.T) {
	kc := useFakeKeychain(t)
	k := testKey(t, 1)
	s := &FrameFairy{}
	s.openedWith("framefairy://unlock?key=" + k)
	if st := s.Licence(); !st.Saved || !st.Waiting {
		t.Fatalf("after the link: %+v", st)
	}
	if kc.key != k {
		t.Fatalf("kept %q", kc.key)
	}
	if got := s.TakeLicenceLink(); got != (LicenceLink{What: "unlocked", About: kc.about}) {
		t.Fatalf("took %+v", got)
	}
	if s.Licence().Waiting || s.TakeLicenceLink() != (LicenceLink{}) {
		t.Fatal("the link was handed over twice")
	}
}

// Every key unlocks the same app, so a link with another key takes the
// place of the one kept at once, and says which one went. The same key
// again changes nothing.
func TestALinkReplacesAKeptKeyAndSaysWhichWent(t *testing.T) {
	kc := useFakeKeychain(t)
	first, second := testKey(t, 1), testKey(t, 2)
	if _, err := saveLicence(first, true); err != nil {
		t.Fatal(err)
	}
	s := &FrameFairy{}
	s.openedWith("framefairy://unlock?key=" + first)
	if got := s.TakeLicenceLink(); got != (LicenceLink{What: "same", About: kc.about}) {
		t.Fatalf("the same key: %+v", got)
	}
	before := kc.about
	s.openedWith("framefairy://unlock?key=" + second)
	if got := s.TakeLicenceLink(); got.What != "replaced" || got.About != kc.about || got.Before != before ||
		!strings.Contains(got.About, "0200-0000-0000-0000") {
		t.Fatalf("another key: %+v", got)
	}
	if kc.key != second {
		t.Fatal("the link did not keep its key")
	}
}

// A key that does not check leaves the key kept where it is.
func TestARefusedLinkLeavesTheKeptKey(t *testing.T) {
	kc := useFakeKeychain(t)
	first := testKey(t, 1)
	if _, err := saveLicence(first, true); err != nil {
		t.Fatal(err)
	}
	s := &FrameFairy{}
	s.openedWith("framefairy://unlock?key=" + aKey[:len(aKey)-2] + "AA")
	if got := s.TakeLicenceLink(); got.What != "refused" || got.Reason == "" {
		t.Fatalf("%+v", got)
	}
	if kc.key != first || !s.Licence().Saved {
		t.Fatal("a key that does not check replaced the one kept")
	}
}

// A key this build does not take is refused with the reason, and nothing
// is kept. A test key in a build from the workflow is the one Tim met: the
// reason has to be the whole of it.
func TestALinkWithAKeyThisBuildRefuses(t *testing.T) {
	kc := useFakeKeychain(t)
	k := testKey(t, 1)
	got := linkOutcome(licence.Key(k), false)
	if got.What != "refused" || !strings.Contains(got.Reason, "test key") {
		t.Fatalf("%+v", got)
	}
	if kc.key != "" {
		t.Fatal("a refused key was kept")
	}
}

// A link the app does not take leaves nothing waiting.
func TestALinkNotTakenLeavesNothing(t *testing.T) {
	useFakeKeychain(t)
	s := &FrameFairy{}
	s.openedWith("framefairy://unlock?key=" + aKey + "&more=1")
	if s.Licence().Waiting || s.TakeLicenceLink() != (LicenceLink{}) {
		t.Fatal("a link the app does not take left something waiting")
	}
}

// Copy puts the kept key itself on the clipboard, and says so when there
// is none to copy.
func TestCopyPutsTheKeptKeyOnTheClipboard(t *testing.T) {
	useFakeKeychain(t)
	var copied string
	s := &FrameFairy{clipboard: func(text string) bool { copied = text; return true }}
	if err := s.CopyLicence(); err == nil {
		t.Fatal("copied a key when none is kept")
	}
	k := testKey(t, 1)
	if _, err := saveLicence(k, true); err != nil {
		t.Fatal(err)
	}
	if err := s.CopyLicence(); err != nil || copied != k {
		t.Fatalf("copied %q, %v", copied, err)
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
