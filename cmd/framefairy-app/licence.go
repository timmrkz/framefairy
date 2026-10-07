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
	// Waiting says a key came in a framefairy:// link and waits to be
	// unlocked with, without handing it over.
	Waiting bool `json:"waiting"`
}

// Licence is the Licence row as it stands.
func (s *FrameFairy) Licence() LicenceState {
	about, saved := engine.SavedLicence()
	s.mu.Lock()
	waiting := s.linkKey != ""
	s.mu.Unlock()
	return LicenceState{Saved: saved, About: about, Waiting: waiting}
}

// SaveLicence checks a key and keeps it. An empty key takes the kept one
// off the machine. The errors start small, to sit inside a sentence.
func (s *FrameFairy) SaveLicence(key string) (LicenceState, error) {
	if _, err := engine.SaveLicence(key, testKeys()); err != nil {
		return s.Licence(), err
	}
	return s.Licence(), nil
}

// TakeLicenceLink hands over the key a link brought, once. The settings
// put it in the field, and nothing is unlocked until Unlock is pressed:
// any page and any app on the Mac can open a framefairy:// link.
func (s *FrameFairy) TakeLicenceLink() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.linkKey
	s.linkKey = ""
	return k
}

// openedWith is a framefairy:// link macOS handed the app, from a browser,
// a mail or any other app. Only an unlock link with one key in it is
// taken, and the key waits in the settings to be unlocked with.
func (s *FrameFairy) openedWith(raw string) {
	k, err := keyFromLink(raw)
	if err != nil {
		log.Printf("a link the app does not take: %v", err)
		return
	}
	s.mu.Lock()
	s.linkKey = string(k)
	s.mu.Unlock()
	if s.app != nil {
		s.app.Event.Emit("licence-link", nil)
	}
	if s.window != nil {
		s.window.Show()
		s.window.Focus()
	}
}

// The longest link taken. A key is at most a little over 200 characters.
const maxLink = 1024

// keyFromLink reads framefairy://unlock?key=FF1-..., and nothing else: one
// key, no other parameters, no path. The key is only checked when Unlock
// is pressed, the way a pasted one is.
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
