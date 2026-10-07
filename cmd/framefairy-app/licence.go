package main

import (
	"errors"
	"log"
	"net/url"
	"strings"

	"framefairy/engine"
	"framefairy/licence"
)

// testKeys says whether this build takes the test signer's keys: a build
// made from the code on this machine, which the build workflow has given
// neither a channel nor a commit. Anyone could sign keys for it, and
// anyone building from the code could take the check out anyway. A build
// from the workflow never takes them. See docs/LICENCE.md.
func testKeys() bool { return buildChannel == "" && buildCommit == "" }

// LicenceState is the Licence row in the settings.
type LicenceState struct {
	// Saved says whether a key is kept, and About how it was described
	// when it was, read without reading the key.
	Saved bool   `json:"saved"`
	About string `json:"about"`
	// Waiting says a link came and what became of it waits to be shown in
	// the settings, without handing it over.
	Waiting bool `json:"waiting"`
}

// LicenceLink is what became of a key that came in a framefairy:// link,
// for the settings to show, handed over once.
type LicenceLink struct {
	// What is "unlocked" when the key was checked and kept, no key being
	// kept before, "same" when it is the key already kept, "replaces" when
	// another key is kept and Unlock puts this one in its place, and
	// "refused" when this build does not take it, Reason saying why. It is
	// empty when no link waits.
	What string `json:"what"`
	// Key is the key itself, for the field, when it is not kept.
	Key string `json:"key"`
	// About is the key in a few words, its ID and whom it is for, when it
	// is one this build takes.
	About  string `json:"about"`
	Reason string `json:"reason"`
}

// The keychain, as the licence row uses it. Tests put a map in its place,
// so they never reach the machine's own keychain, whose locked box nobody
// answers in CI.
var (
	savedLicence = engine.SavedLicence
	saveLicence  = engine.SaveLicence
)

// Licence is the Licence row as it stands.
func (s *FrameFairy) Licence() LicenceState {
	about, saved := savedLicence()
	s.mu.Lock()
	waiting := s.link != nil
	s.mu.Unlock()
	return LicenceState{Saved: saved, About: about, Waiting: waiting}
}

// SaveLicence checks a key and keeps it. An empty key takes the kept one
// off the machine. The errors start small, to sit inside a sentence.
func (s *FrameFairy) SaveLicence(key string) (LicenceState, error) {
	if _, err := saveLicence(key, testKeys()); err != nil {
		return s.Licence(), err
	}
	return s.Licence(), nil
}

// TakeLicenceLink hands over what became of the last link, once.
func (s *FrameFairy) TakeLicenceLink() LicenceLink {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.link == nil {
		return LicenceLink{}
	}
	l := *s.link
	s.link = nil
	return l
}

// openedWith is a framefairy:// link macOS handed the app, from a browser,
// a mail or any other app. Only an unlock link with one key in it is
// taken, see linkOutcome, and the settings show what became of it.
func (s *FrameFairy) openedWith(raw string) {
	k, err := keyFromLink(raw)
	if err != nil {
		log.Printf("a link the app does not take: %v", err)
		return
	}
	l := linkOutcome(k, testKeys())
	s.mu.Lock()
	s.link = &l
	s.mu.Unlock()
	if s.app != nil {
		s.app.Event.Emit("licence-link", nil)
	}
	if s.window != nil {
		s.window.Show()
		s.window.Focus()
	}
}

// linkOutcome decides what a key from a link does. On a Mac with no key
// it unlocks the app at once: the buyer clicked Unlock in the mail, and
// there is nothing to lose. A key already kept is never replaced by a
// link alone, because any page and any app on the Mac can open one: the
// settings show the new key and Unlock puts it in place.
func linkOutcome(k licence.Key, testKeys bool) LicenceLink {
	_, l, err := engine.CheckLicence(string(k), testKeys)
	if err != nil {
		return LicenceLink{What: "refused", Key: string(k), Reason: err.Error()}
	}
	about, saved := savedLicence()
	switch {
	case !saved:
		if _, err := saveLicence(string(k), testKeys); err != nil {
			return LicenceLink{What: "refused", Key: string(k), Reason: err.Error()}
		}
		return LicenceLink{What: "unlocked", About: engine.DescribeLicence(l)}
	case about == engine.DescribeLicence(l):
		return LicenceLink{What: "same", About: about}
	}
	return LicenceLink{What: "replaces", Key: string(k), About: engine.DescribeLicence(l)}
}

// The longest link taken. A key is at most a little over 200 characters.
const maxLink = 1024

// keyFromLink reads framefairy://unlock?key=FF1-..., and nothing else: one
// key, no other parameters, no path. linkOutcome checks the key.
func keyFromLink(raw string) (licence.Key, error) {
	if len(raw) > maxLink {
		return "", errors.New("too long")
	}
	// The errors never quote the link: it is logged, and it may hold a key.
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("not a link")
	}
	if !strings.EqualFold(u.Scheme, "framefairy") || u.Host != "unlock" || (u.Path != "" && u.Path != "/") ||
		u.User != nil || u.Opaque != "" || u.Fragment != "" || u.RawFragment != "" {
		return "", errors.New("not an unlock link")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("an unlock link the app cannot read")
	}
	if len(q) != 1 || len(q["key"]) != 1 {
		return "", errors.New("an unlock link holds one key and nothing else")
	}
	k := strings.TrimSpace(q["key"][0])
	if !strings.HasPrefix(k, licence.Prefix) || len(k) > 512 {
		return "", errors.New("not a licence key")
	}
	for _, c := range k[len(licence.Prefix):] {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return "", errors.New("not a licence key")
		}
	}
	return licence.Key(k), nil
}
