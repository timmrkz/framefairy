// Package updates is how the app finds, checks and fetches a newer build of
// itself. The swap is Wails' updater's, pkg/updater: this package is the
// source it reads from, the channel list, and the signing the build
// workflow does on the other end. See docs/UPDATES.md.
//
// A channel is main or one pull request. The build workflow publishes each
// channel's newest build of the app as a zip, and one small public file,
// the channel list, says for every channel which build that is, where it
// is, and its checksum and signature. The app fetches that one file. It
// never asks GitHub's API and has no idea what a pull request is.
//
// The channel list is untrusted: anybody on the way could change it. What
// makes a build safe to install is the signature, made with a key only the
// build workflow holds, over the SHA-256 of the zip. The public half is
// built into the app and Wails' updater checks it before anything is
// unpacked, so a list that points somewhere else can only point at a file
// that fails.
package updates

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ListURL is where every development build looks for the channel list: a
// file of the release called dev in this repository, which the build
// workflow writes again whenever a channel gets a new build.
const ListURL = "https://github.com/timmrkz/framefairy/releases/download/dev/channels.json"

// MaxList is the most a channel list may weigh. A few hundred bytes a
// channel, so this is room for thousands.
const MaxList = 1 << 20

// MaxBuild is the most a build may weigh. The app with ffmpeg, llama-server
// and the speech libraries in it is a few hundred megabytes.
const MaxBuild = 2 << 30

// Build is one channel's newest build, as the channel list has it.
type Build struct {
	// Channel is main or pr- and the number of the pull request.
	Channel string `json:"channel"`
	// Name is what the app shows for the channel: main, or the pull
	// request's number and title.
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	// URL is where the zip is, which holds the app and nothing else.
	URL  string `json:"url"`
	Size int64  `json:"size"`
	// SHA256 is the checksum of the zip, in hex.
	SHA256 string `json:"sha256"`
	// Signature is Ed25519 over the SHA-256 of the zip, the raw 32 bytes,
	// in base64. That is what Wails' updater checks. Sparkle signs the
	// file itself, so the two cannot share a signature.
	Signature string    `json:"signature"`
	Published time.Time `json:"published"`
}

// List is the channel list.
type List struct {
	Channels []Build `json:"channels"`
}

var (
	channelPattern = regexp.MustCompile(`^(main|pr-[1-9][0-9]{0,5})$`)
	versionPattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+-]{0,63}$`)
	commitPattern  = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidChannel says whether a name is one a channel can have.
func ValidChannel(channel string) bool { return channelPattern.MatchString(channel) }

// Check says what is wrong with a build as the list describes it, or nil.
// It says nothing about whether the zip is really ours: that is the
// signature's job, and it is checked against the file, not the list.
func (b Build) Check() error {
	if !channelPattern.MatchString(b.Channel) {
		return fmt.Errorf("the channel %q is not main or a pull request", b.Channel)
	}
	if b.Name == "" || len(b.Name) > 200 || strings.ContainsAny(b.Name, "\x00\n\r") {
		return fmt.Errorf("channel %s has no name that can be shown", b.Channel)
	}
	if !versionPattern.MatchString(b.Version) {
		return fmt.Errorf("channel %s has no version", b.Channel)
	}
	if !commitPattern.MatchString(b.Commit) {
		return fmt.Errorf("channel %s has no commit", b.Channel)
	}
	u, err := url.Parse(b.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || !strings.HasSuffix(u.Path, ".zip") {
		return fmt.Errorf("channel %s is not a zip on https", b.Channel)
	}
	if b.Size <= 0 || b.Size > MaxBuild {
		return fmt.Errorf("channel %s has a size of %d bytes", b.Channel, b.Size)
	}
	if !sha256Pattern.MatchString(b.SHA256) {
		return fmt.Errorf("channel %s has no checksum", b.Channel)
	}
	if sig, err := base64.StdEncoding.DecodeString(b.Signature); err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("channel %s has no signature", b.Channel)
	}
	return nil
}

// Parse reads a channel list. A build the list describes badly is left
// out, so one bad entry does not take every other channel with it. Two
// entries for one channel keep the newer.
func Parse(data []byte) (List, error) {
	if len(data) > MaxList {
		return List{}, errors.New("the channel list is too large")
	}
	var raw List
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return List{}, fmt.Errorf("the channel list is not readable: %w", err)
	}
	seen := map[string]int{}
	var out List
	for _, b := range raw.Channels {
		if b.Check() != nil {
			continue
		}
		if i, ok := seen[b.Channel]; ok {
			if b.Published.After(out.Channels[i].Published) {
				out.Channels[i] = b
			}
			continue
		}
		seen[b.Channel] = len(out.Channels)
		out.Channels = append(out.Channels, b)
	}
	out.Sort()
	return out, nil
}

// Sort puts main first and the pull requests after it, newest first.
func (l *List) Sort() {
	sort.SliceStable(l.Channels, func(i, j int) bool {
		a, b := l.Channels[i].Channel, l.Channels[j].Channel
		if a == "main" || b == "main" {
			return a == "main" && b != "main"
		}
		return prNumber(a) > prNumber(b)
	})
}

func prNumber(channel string) int {
	n := 0
	for _, c := range strings.TrimPrefix(channel, "pr-") {
		n = n*10 + int(c-'0')
	}
	return n
}

// Find returns the build of a channel, if the list has it.
func (l List) Find(channel string) (Build, bool) {
	for _, b := range l.Channels {
		if b.Channel == channel {
			return b, true
		}
	}
	return Build{}, false
}

// Follow decides which channel a build follows: the one picked, while the
// list still has it, and main once it does not, which is what happens to a
// pull request when it is merged or closed. Nothing picked follows the
// channel the running build came from.
func (l List) Follow(picked, own string) (Build, bool) {
	for _, c := range []string{picked, own, "main"} {
		if c == "" {
			continue
		}
		if b, ok := l.Find(c); ok {
			return b, true
		}
	}
	return Build{}, false
}

// Digest is the SHA-256 of what r reads, and how many bytes that was.
func Digest(r io.Reader) ([]byte, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return nil, 0, err
	}
	return h.Sum(nil), n, nil
}

// Sign signs a digest the way Wails' updater checks it.
func Sign(key ed25519.PrivateKey, digest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest))
}

// Verify checks a build's signature against a public key. The app leaves
// this to Wails' updater, which checks the file as it arrives. The
// workflow checks every build it publishes with it, so a key that does
// not match the app's is found before anybody downloads anything.
func Verify(public ed25519.PublicKey, b Build) error {
	digest, err := hex.DecodeString(b.SHA256)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(public, digest, sig) {
		return fmt.Errorf("the build of %s is not signed with this key", b.Channel)
	}
	return nil
}

// PublicKey reads a public key as it is kept in the repository: base64 of
// the 32 bytes, on one line. Empty text is no key, which is what a
// repository has before the key is made.
func PublicKey(text string) (ed25519.PublicKey, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("the update key is not 32 bytes of base64")
	}
	return ed25519.PublicKey(raw), nil
}

// PrivateKey reads a private key as it is kept in the workflow's secret:
// base64 of the 32 byte seed.
func PrivateKey(text string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("the private update key is not 32 bytes of base64")
	}
	return ed25519.NewKeyFromSeed(raw), nil
}
