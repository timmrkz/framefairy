package engine

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
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
	// get reads the secret.
	get(ctx context.Context, item string) (string, bool)
	set(item, secret string) error
	remove(item string)
}

// errNoKeychain is a machine with nowhere safe to keep a key.
var errNoKeychain = errors.New("no keychain")

// noKeys is a machine without a keychain.
type noKeys struct{}

func (noKeys) has(string) bool                            { return false }
func (noKeys) get(context.Context, string) (string, bool) { return "", false }
func (noKeys) set(string, string) error                   { return errNoKeychain }
func (noKeys) remove(string)                              {}

// keys is where keys are kept now, and oldKeys where the first ones were.
// A test never reaches the machine's own keychain: macOS puts a box on
// screen when it is locked, a box nobody answers is a test that hangs, and
// a test that writes a key writes it into somebody's keychain. Tests that
// are about keeping keys put a keyStore of their own here.
var keys, oldKeys keyStore = systemKeys(), legacyKeys()

func init() {
	if testing.Testing() {
		keys, oldKeys = noKeys{}, noKeys{}
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

// ReadAPIKey takes a provider's key from the environment first, then the
// keychain. Never from a file on disk. A key still kept the old way is
// moved into an item only this app may read, and the old one is removed.
func ReadAPIKey(ctx context.Context, p Provider) (string, error) {
	key, source := envKey(p), p.Env
	if key == "" {
		if found, ok := keys.get(ctx, p.Item); ok {
			key, source = strings.TrimSpace(found), "the keychain"
		} else if found, ok := oldKeys.get(ctx, p.Keychain); ok {
			key, source = strings.TrimSpace(found), "the keychain"
			// Only once the key is safe in the new item does the old one
			// go, so a keychain that refuses the new one loses nothing.
			if keyIsSane(key) && keys.set(p.Item, key) == nil {
				oldKeys.remove(p.Keychain)
			}
		}
	}
	if key == "" {
		return "", missingKey(p)
	}
	if !keyIsSane(key) {
		return "", renderErr("the API key from %s contains characters that cannot go in "+
			"an HTTP header. It was probably stored with a stray newline.", source)
	}
	return key, nil
}

// CheckAPIKey says whether a provider's key can be found, without reading
// it, and why not when it cannot. The settings ask this every time they are
// opened, and a secret read each time would be a box on screen each time.
func CheckAPIKey(p Provider) error {
	if key := envKey(p); key != "" {
		if !keyIsSane(key) {
			return renderErr("the API key in %s contains characters that cannot go in "+
				"an HTTP header.", p.Env)
		}
		return nil
	}
	if keys.has(p.Item) || oldKeys.has(p.Keychain) {
		return nil
	}
	return missingKey(p)
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
	if err := keys.set(p.Item, key); err != nil {
		if errors.Is(err, errNoKeychain) {
			return renderErr("this machine has no keychain to put a key in. Set %s instead.", p.Env)
		}
		return renderErr("the keychain did not take the key: %s", err)
	}
	oldKeys.remove(p.Keychain)
	return nil
}
