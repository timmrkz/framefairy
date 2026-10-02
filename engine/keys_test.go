package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func (m *memKeys) hint(item string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.items[item]; ok {
		return abbreviate(v)
	}
	return ""
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

// The app uses the key saved in it before one in the environment. An app
// started from a terminal has the terminal's environment, and an old key in
// it won over the one just saved in the settings, until the provider
// refused it. The command line still reads the environment first.
func TestTheAppUsesTheSavedKeyFirst(t *testing.T) {
	p, _ := ProviderNamed("anthropic")
	keychains(t, map[string]string{p.Item: "sk-ant-saved"}, nil)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-old-in-the-shell")
	t.Cleanup(func() { savedFirst.Store(false) })

	if key, _ := ReadAPIKey(context.Background(), p); key != "sk-ant-old-in-the-shell" {
		t.Errorf("the command line read %q", key)
	}
	if got := KeySource(p); got != KeyInEnvironment {
		t.Errorf("the command line's key is in the %q", got)
	}

	PreferSavedKeys()
	if key, _ := ReadAPIKey(context.Background(), p); key != "sk-ant-saved" {
		t.Errorf("the app read %q", key)
	}
	if got := KeySource(p); got != KeyInKeychain {
		t.Errorf("the app's key is in the %q", got)
	}

	// With nothing saved, the environment still stands in.
	keychains(t, nil, nil)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-in-the-shell")
	if key, _ := ReadAPIKey(context.Background(), p); key != "sk-ant-in-the-shell" {
		t.Errorf("with nothing saved the app read %q", key)
	}
	if got := KeySource(p); got != KeyInEnvironment {
		t.Errorf("with nothing saved the key is in the %q", got)
	}
}

// A key is shown to its company before it is kept, the way that company
// asks for one, and a key it refuses is refused. A company that cannot be
// reached says nothing about the key, so it is kept.
func TestVerifyAPIKeyAsksTheCompany(t *testing.T) {
	var mu sync.Mutex
	var asked []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.Clone(context.Background()))
		mu.Unlock()
		if strings.Contains(r.Header.Get("x-api-key")+r.Header.Get("authorization"), "bad") {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()
	anthropic, _ := ProviderNamed("anthropic")
	openai, _ := ProviderNamed("openai")
	anthropic.Models, openai.Models = server.URL+"/anthropic", server.URL+"/openai"

	if err := VerifyAPIKey(context.Background(), anthropic, "sk-ant-good\n"); err != nil {
		t.Errorf("a good key: %v", err)
	}
	err := VerifyAPIKey(context.Background(), anthropic, "sk-ant-bad")
	if err == nil || !strings.Contains(err.Error(), "Anthropic did not accept this key") ||
		strings.Contains(err.Error(), "{") {
		t.Errorf("a refused key: %v", err)
	}
	if err := VerifyAPIKey(context.Background(), openai, "sk-proj-bad"); err == nil ||
		err.Error() != "OpenAI did not accept this key." {
		t.Errorf("a refused OpenAI key: %v", err)
	}

	mu.Lock()
	first, third := asked[0], asked[2]
	mu.Unlock()
	if first.Method != http.MethodGet || first.URL.Path != "/anthropic" ||
		first.Header.Get("x-api-key") != "sk-ant-good" || first.Header.Get("anthropic-version") == "" {
		t.Errorf("anthropic was asked %s %s with %v", first.Method, first.URL.Path, first.Header)
	}
	if third.URL.Path != "/openai" || third.Header.Get("authorization") != "Bearer sk-proj-bad" ||
		third.Header.Get("x-api-key") != "" {
		t.Errorf("openai was asked at %s with %v", third.URL.Path, third.Header)
	}

	// Something else copied from the same page, like the ID the Console
	// lists a key by, is refused as no key of theirs, in a short line.
	err = VerifyAPIKey(context.Background(), anthropic, "apikey_01Jc8z2Rkr225X8W3UCSCFSDbad")
	if err == nil || err.Error() != "not an Anthropic key. Those start with sk-ant-." {
		t.Errorf("a key's ID: %v", err)
	}

	// Nobody there: the key passes, since nothing is known against it.
	server.Close()
	if err := VerifyAPIKey(context.Background(), anthropic, "sk-ant-bad"); err != nil {
		t.Errorf("with the company out of reach: %v", err)
	}
}

// A key refused in the middle of a search is said in words, names where
// the key came from, and keeps the company's JSON for the log.
func TestARefusedKeyIsSaidInWords(t *testing.T) {
	refuse := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_1"}`)
	}
	_, e := cloud(t, refuse, refuse)
	_, err := e.CallAPI(context.Background(), "the transcript", "claude-sonnet-5", 4000, "", "plan", "", nil)
	if err == nil || strings.Contains(err.Error(), "{") ||
		!strings.Contains(err.Error(), "Anthropic did not accept the API key in ANTHROPIC_API_KEY") {
		t.Errorf("from the environment: %v", err)
	}

	p, _ := ProviderNamed("anthropic")
	keychains(t, map[string]string{p.Item: "sk-ant-saved"}, nil)
	_, err = e.CallAPI(context.Background(), "the transcript", "claude-sonnet-5", 4000, "", "plan", "", nil)
	if err == nil || !strings.Contains(err.Error(), "did not accept the saved API key. Replace it in the settings") {
		t.Errorf("from the keychain: %v", err)
	}
}

func TestTestsNeverShowAKeyToACompany(t *testing.T) {
	for _, p := range Providers() {
		if p.Models != "" {
			t.Errorf("a test binary would show %s a key at %s", p.Title, p.Models)
		}
	}
}

// A saved key is shown in short, the way the companies list keys, without
// reading the secret, and one too short to cut shows nothing of itself.
func TestAKeyIsShownInShort(t *testing.T) {
	p, _ := ProviderNamed("anthropic")
	const key = "sk-ant-api03-Ah3xxxxxxxxxxxxxxxxxxxxxxxxxxMwAA"
	now, _ := keychains(t, nil, nil)
	if err := StoreAPIKey(p, key); err != nil {
		t.Fatal(err)
	}
	reads := now.reads
	if got := KeyHint(p); got != "sk-ant-api03...MwAA" {
		t.Errorf("in short: %q", got)
	}
	if now.reads != reads {
		t.Error("the secret was read to show it in short")
	}
	if got := abbreviate("sk-short"); got != "" {
		t.Errorf("a short key showed %q", got)
	}
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-fromtheenvironment0000WXYZ")
	if got := KeyHint(p); got != "sk-ant-api03...WXYZ" {
		t.Errorf("from the environment: %q", got)
	}
	if _, err := ReadAPIKey(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := StoreAPIKey(p, ""); err != nil || KeyHint(p) != "sk-ant-api03...WXYZ" || now.has(p.Item) {
		t.Errorf("removed: %v, hint %q", err, KeyHint(p))
	}
}

// A key being checked goes to the provider's address and nowhere else: a
// redirect to another host is not followed, so the key never reaches it.
func TestAKeyBeingCheckedFollowsNoRedirect(t *testing.T) {
	var leaked []string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = append(leaked, r.Header.Get("x-api-key")+r.Header.Get("authorization"))
	}))
	defer elsewhere.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/v1/models", http.StatusTemporaryRedirect)
	}))
	defer provider.Close()
	for _, p := range []Provider{
		{Name: "anthropic", Models: provider.URL, KeyPrefix: "sk-ant-"},
		{Name: "openai", Models: provider.URL, KeyPrefix: "sk-"},
	} {
		_ = VerifyAPIKey(context.Background(), p, p.KeyPrefix+"api03-abcdefghijklmnopqrstuvwxyz0123456789")
	}
	if len(leaked) > 0 {
		t.Errorf("a redirect took the key to another host: %q", leaked)
	}
}
