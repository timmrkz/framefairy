package engine

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Where the keys are kept
//
// A key to a company's API is money, so it is kept the way the operating
// system keeps secrets, never in a file, and only this app may read it.
// On the Mac that is the keychain, through the Security framework itself
// in keys_darwin.go: the item's access list names the app that made it,
// so another program that asks for the key gets a box from macOS asking
// the person first, and the key never passes through a command line,
// where every other program on the machine could have read it for as long
// as the command ran.
//
// The first keys were kept with the security command, which did both of
// those things wrong: the key went in as an argument, and the item's
// access list named the security command, so anything that ran it could
// read the key without a word. Those items are moved into ones of the
// app's own the first time they are read, and then removed.
//
// Machines without a keychain read the key from the environment only.
// ---------------------------------------------------------------------------

// keyStore is a place keys are kept, one item per provider.
type keyStore interface {
	// has says whether an item is there without reading the secret in it,
	// so asking never puts a box on screen.
	has(item string) bool
	// hint is the key in short, as abbreviate writes it, kept beside the
	// secret where it can be read without it. Empty when there is none.
	hint(item string) string
	// get reads the secret.
	get(ctx context.Context, item string) (string, bool)
	// set keeps the secret with hint beside it, where hint can read it
	// back without the secret.
	set(item, secret, hint string) error
	remove(item string)
}

// errNoKeychain is a machine with nowhere safe to keep a key.
var errNoKeychain = errors.New("no keychain")

// noKeys is a machine without a keychain.
type noKeys struct{}

func (noKeys) has(string) bool                            { return false }
func (noKeys) hint(string) string                         { return "" }
func (noKeys) get(context.Context, string) (string, bool) { return "", false }
func (noKeys) set(string, string, string) error           { return errNoKeychain }
func (noKeys) remove(string)                              {}

// keys is where keys are kept now, and oldKeys where the first ones were.
// A test never reaches the machine's own keychain: macOS puts a box on
// screen when it is locked, a box nobody answers is a test that hangs, and
// a test that writes a key writes it into somebody's keychain. Tests that
// are about keeping keys put a keyStore of their own here.
//
// Nor does a test show a key to a company: a key saved in a test is never
// sent anywhere, and a test that means to ask a company names a server of
// its own.
var keys, oldKeys keyStore = systemKeys(), legacyKeys()

func init() {
	if testing.Testing() {
		keys, oldKeys = noKeys{}, noKeys{}
		for i := range providers {
			providers[i].Models = ""
		}
	}
}

// keyAccount is the account every item is kept under.
const keyAccount = "framefairy"

// envKey is the key in the environment, which comes first, because that is
// how the command line is given one.
func envKey(p Provider) string {
	return strings.TrimSpace(os.Getenv(p.Env))
}

func missingKey(p Provider) error {
	return renderErr("no %s API key found. Save one in the app's settings, or set %s.",
		p.Title, p.Env)
}

// savedFirst puts the key saved in the app before the one in the
// environment. The command line reads the environment first, which is how
// it is given a key. The app is given one in its settings, and a key saved
// there has to be the one used: an app started from a terminal has the
// terminal's environment, and an old key in it would otherwise win over
// the one just saved, without a word.
var savedFirst atomic.Bool

// PreferSavedKeys makes the key saved in the app the one used, and one in
// the environment a stand-in for when none is saved. The app calls it once,
// as it starts.
func PreferSavedKeys() { savedFirst.Store(true) }

// Where a key was found, as KeySource says it.
const (
	KeyInKeychain    = "keychain"
	KeyInEnvironment = "environment"
)

// KeySource says where a provider's key would be read from, without reading
// it: the keychain, the environment, or nowhere. The settings ask this every
// time they are opened, and a secret read each time would be a box on screen
// each time.
func KeySource(p Provider) string {
	saved := keys.has(p.Item) || oldKeys.has(p.Keychain)
	env := envKey(p) != ""
	switch {
	case saved && (savedFirst.Load() || !env):
		return KeyInKeychain
	case env:
		return KeyInEnvironment
	}
	return ""
}

// abbreviate is a key in short, the way the companies list keys: its first
// twelve characters, which on Anthropic's are the kind of key and not the
// secret, and its last four. It says which key is saved without saying
// the key. A key too short to cut that way is not shown at all.
func abbreviate(key string) string {
	if len(key) < 24 {
		return ""
	}
	return key[:12] + "..." + key[len(key)-4:]
}

// KeyHint is the provider's key in short, from wherever KeySource says it
// is, read without the secret. Empty when it cannot be said.
func KeyHint(p Provider) string {
	switch KeySource(p) {
	case KeyInEnvironment:
		return abbreviate(envKey(p))
	case KeyInKeychain:
		return keys.hint(p.Item)
	}
	return ""
}

// ReadAPIKey takes a provider's key from the environment and the keychain,
// in the order KeySource says. Never from a file on disk. A key still kept
// the old way is moved into an item only this app may read, and the old one
// is removed.
func ReadAPIKey(ctx context.Context, p Provider) (string, error) {
	key, _, err := readAPIKey(ctx, p)
	return key, err
}

// readAPIKey is ReadAPIKey, and says where the key came from, so a key the
// provider refuses can be named.
func readAPIKey(ctx context.Context, p Provider) (key, source string, err error) {
	fromKeychain := func() {
		if found, ok := keys.get(ctx, p.Item); ok {
			key, source = strings.TrimSpace(found), "the keychain"
		} else if found, ok := oldKeys.get(ctx, p.Keychain); ok {
			key, source = strings.TrimSpace(found), "the keychain"
			// Only once the key is safe in the new item does the old one
			// go, so a keychain that refuses the new one loses nothing.
			if keyIsSane(key) && keys.set(p.Item, key, abbreviate(key)) == nil {
				oldKeys.remove(p.Keychain)
			}
		}
	}
	fromEnv := func() {
		if env := envKey(p); env != "" {
			key, source = env, p.Env
		}
	}
	if savedFirst.Load() {
		fromKeychain()
		if key == "" {
			fromEnv()
		}
	} else {
		fromEnv()
		if key == "" {
			fromKeychain()
		}
	}
	if key == "" {
		return "", "", missingKey(p)
	}
	if !keyIsSane(key) {
		return "", "", renderErr("the API key from %s contains characters that cannot go in "+
			"an HTTP header. It was probably stored with a stray newline.", source)
	}
	return key, source, nil
}

// refusedKey is what a search says when the provider does not take the key,
// in words rather than the provider's JSON, and naming where the key came
// from, since a key in the environment and one in the keychain are fixed in
// different places.
func refusedKey(p Provider, source string) error {
	if source == "the keychain" {
		return renderErr("%s did not accept the saved API key. Replace it in the settings, "+
			"or check it at %s.", p.Title, p.KeysAt)
	}
	return renderErr("%s did not accept the API key in %s. Check it at %s.",
		p.Title, source, p.KeysAt)
}

// CheckAPIKey says whether a provider's key can be found, without reading
// it, and why not when it cannot.
func CheckAPIKey(p Provider) error {
	switch KeySource(p) {
	case KeyInKeychain:
		return nil
	case KeyInEnvironment:
		if !keyIsSane(envKey(p)) {
			return renderErr("the API key in %s contains characters that cannot go in "+
				"an HTTP header.", p.Env)
		}
		return nil
	}
	return missingKey(p)
}

// VerifyAPIKey asks the provider whether it takes a key, before the key is
// kept. It asks for the list of models, which costs nothing and needs a key
// the provider knows. A key it refuses is refused here, where it was typed,
// rather than an hour later, when an episode has been transcribed and the
// first search fails. When the provider cannot be reached, nothing can be
// said about the key, so it passes: somebody saving a key on a train keeps
// it.
func VerifyAPIKey(ctx context.Context, p Provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" || !keyIsSane(key) || p.Models == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Models, nil)
	if err != nil {
		return nil
	}
	setKey(request, p, key)
	// Never followed anywhere, like every other request that carries a key,
	// see httpClient. Go keeps a header it does not know on a redirect to
	// another host, and the key is one: a redirect would have handed it to
	// whoever the provider's address sent it on to.
	response, err := httpClient.Do(request)
	if err != nil {
		return nil
	}
	response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		// Something else copied from the same page, like the key's ID,
		// is said to be no key of theirs. The line is short, since it
		// stands in one line of the settings.
		if !strings.HasPrefix(key, p.KeyPrefix) {
			return renderErr("not an %s key. Those start with %s.", p.Title, p.KeyPrefix)
		}
		return renderErr("%s did not accept this key.", p.Title)
	}
	return nil
}

// StoreAPIKey puts a provider's key in the keychain, which is the only place
// the app keeps one. Never a file on disk, and never the settings, which
// are plain JSON in the config folder and get copied about. Each
// provider's key is kept apart, so trying one never costs the other. A key
// kept the old way is removed once the new one is in.
//
// An empty key removes the stored one. That is how somebody takes their key
// off a machine, so it has to be an ordinary thing to do rather than an
// error.
//
// The key is checked before it is stored rather than only when it is used.
// A key pasted with a newline on the end is the common case, and finding
// that out at the moment it is typed beats finding out when an episode has
// already been transcribed.
func StoreAPIKey(p Provider, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		keys.remove(p.Item)
		oldKeys.remove(p.Keychain)
		return nil
	}
	if !keyIsSane(key) {
		return renderErr("that key has characters in it that cannot go in an HTTP header.")
	}
	if err := keys.set(p.Item, key, abbreviate(key)); err != nil {
		if errors.Is(err, errNoKeychain) {
			return renderErr("this machine has no keychain to put a key in. Set %s instead.", p.Env)
		}
		return renderErr("the keychain did not take the key: %s", err)
	}
	oldKeys.remove(p.Keychain)
	return nil
}
