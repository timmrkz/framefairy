package engine

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// memKeys is a keychain in memory. It counts every time a secret is read,
// which is what puts a box on screen on a real Mac, and every time one is
// written.
type memKeys struct {
	mu     sync.Mutex
	items  map[string]string
	reads  int
	writes int
	refuse bool
}

func newMemKeys(items map[string]string) *memKeys {
	if items == nil {
		items = map[string]string{}
	}
	return &memKeys{items: items}
}

func (m *memKeys) has(item string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.items[item]
	return ok
}

func (m *memKeys) get(_ context.Context, item string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	v, ok := m.items[item]
	return v, ok
}

func (m *memKeys) set(item, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.refuse {
		return errNoKeychain
	}
	m.writes++
	m.items[item] = secret
	return nil
}

func (m *memKeys) remove(item string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, item)
}

// keychains puts two keychains in memory in place of the machine's, the
// one the app keeps keys in now and the one the first keys were kept in.
func keychains(t *testing.T, now, old map[string]string) (*memKeys, *memKeys) {
	t.Helper()
	savedKeys, savedOld := keys, oldKeys
	k, o := newMemKeys(now), newMemKeys(old)
	keys, oldKeys = k, o
	t.Cleanup(func() { keys, oldKeys = savedKeys, savedOld })
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	return k, o
}

func TestTestsNeverReachTheMachinesKeychain(t *testing.T) {
	if _, ok := keys.(noKeys); !ok {
		t.Fatalf("a test binary keeps keys in %T, which may be the machine's own keychain", keys)
	}
	if _, ok := oldKeys.(noKeys); !ok {
		t.Fatalf("a test binary reads old keys from %T", oldKeys)
	}
}

// A key goes into an item of the app's own, and a key kept the old way is
// removed once it is, so no copy is left that anything on the machine can
// read.
func TestStoreAPIKeyKeepsItInTheAppsOwnItem(t *testing.T) {
	openai, _ := ProviderNamed("openai")
	anthropic, _ := ProviderNamed("anthropic")
	now, old := keychains(t, nil, map[string]string{openai.Keychain: "sk-old"})

	if err := StoreAPIKey(openai, "  sk-proj-new\n"); err != nil {
		t.Fatal(err)
	}
	if now.items[openai.Item] != "sk-proj-new" {
		t.Errorf("kept %q in the app's item", now.items[openai.Item])
	}
	if old.has(openai.Keychain) {
		t.Error("the old item anything could read is still there")
	}
	if old.writes != 0 {
		t.Error("something was written the old way")
	}
	// Each company's key is its own.
	if now.has(anthropic.Item) {
		t.Error("an OpenAI key landed in Anthropic's item")
	}

	// An empty key takes it off the machine, both ways.
	old.items[openai.Keychain] = "sk-old"
	if err := StoreAPIKey(openai, ""); err != nil {
		t.Fatal(err)
	}
	if now.has(openai.Item) || old.has(openai.Keychain) {
		t.Error("an empty key left a key behind")
	}

	// A key that cannot go in a header is refused before it is kept.
	if err := StoreAPIKey(openai, "sk-a\nb"); err == nil || now.has(openai.Item) {
		t.Errorf("a key with a newline: %v", err)
	}

	// With no keychain the person is told what works instead.
	now.refuse = true
	if err := StoreAPIKey(openai, "sk-proj-x"); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("no keychain: %v", err)
	}
}

// A key kept the old way is still found, and moved into the app's own item
// the first time it is read, and only then is the old one removed.
func TestReadAPIKeyMovesAnOldKey(t *testing.T) {
	p, _ := ProviderNamed("anthropic")
	now, old := keychains(t, nil, map[string]string{p.Keychain: "sk-ant-old"})
	key, err := ReadAPIKey(context.Background(), p)
	if err != nil || key != "sk-ant-old" {
		t.Fatalf("read %q, %v", key, err)
	}
	if now.items[p.Item] != "sk-ant-old" || old.has(p.Keychain) {
		t.Errorf("not moved: new %v, old %v", now.items, old.items)
	}

	// A keychain that will not take the new item keeps the old one, so the
	// key is never lost on the way.
	now, old = keychains(t, nil, map[string]string{p.Keychain: "sk-ant-old"})
	now.refuse = true
	if key, err := ReadAPIKey(context.Background(), p); err != nil || key != "sk-ant-old" {
		t.Fatalf("read with a refusing keychain: %q, %v", key, err)
	}
	if !old.has(p.Keychain) {
		t.Error("the old key was removed although the new item was never made")
	}
}

// The environment comes first, then the app's item, and a missing key
// says where a key goes.
func TestReadAPIKeyOrder(t *testing.T) {
	p, _ := ProviderNamed("openai")
	now, _ := keychains(t, map[string]string{p.Item: "sk-from-keychain"}, nil)
	if key, _ := ReadAPIKey(context.Background(), p); key != "sk-from-keychain" {
		t.Errorf("from the keychain: %q", key)
	}
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	reads := now.reads
	if key, _ := ReadAPIKey(context.Background(), p); key != "sk-from-env" {
		t.Errorf("from the environment: %q", key)
	}
	if now.reads != reads {
		t.Error("the keychain was read although the environment had the key")
	}
	t.Setenv("OPENAI_API_KEY", "")
	now.remove(p.Item)
	_, err := ReadAPIKey(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "settings") || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Errorf("no key: %v", err)
	}
}

// Whether a key is there is asked without reading it, every time the
// settings open, so nothing is put on screen for it.
func TestCheckAPIKeyNeverReadsTheSecret(t *testing.T) {
	anthropic, _ := ProviderNamed("anthropic")
	openai, _ := ProviderNamed("openai")
	now, old := keychains(t, map[string]string{anthropic.Item: "sk-ant"}, map[string]string{openai.Keychain: "sk-old"})
	if err := CheckAPIKey(anthropic); err != nil {
		t.Errorf("anthropic: %v", err)
	}
	// A key still kept the old way counts, and is moved when it is used.
	if err := CheckAPIKey(openai); err != nil {
		t.Errorf("openai, kept the old way: %v", err)
	}
	if now.reads+old.reads != 0 {
		t.Errorf("a check read %d secrets", now.reads+old.reads)
	}
	old.remove(openai.Keychain)
	if err := CheckAPIKey(openai); err == nil {
		t.Error("no key, and the check found one")
	}
	t.Setenv("OPENAI_API_KEY", "sk-a\nb")
	if err := CheckAPIKey(openai); err == nil {
		t.Error("a key with a newline in the environment passed the check")
	}
}
