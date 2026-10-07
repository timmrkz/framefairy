package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"framefairy/licence"
)

func signerDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "signer")
	t.Setenv("FRAMEFAIRY_SIGNER_DIR", dir)
	return dir
}

func call(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := run(args, &out)
	return out.String(), err
}

func must(t *testing.T, args ...string) string {
	t.Helper()
	out, err := call(t, args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out
}

func trustOf(t *testing.T) licence.Trust {
	t.Helper()
	line := strings.Fields(must(t, "public"))
	pub, err := hex.DecodeString(line[2])
	if err != nil {
		t.Fatal(err)
	}
	trust := licence.Trust{Signers: map[uint8]ed25519.PublicKey{}}
	switch line[1] {
	case "1":
		trust.Signers[1] = pub
	default:
		t.Fatalf("signer %s", line[1])
	}
	return trust
}

func testTrust() licence.Trust {
	return licence.Trust{Signers: map[uint8]ed25519.PublicKey{0: ed25519.NewKeyFromSeed(licence.TestSeed()).Public().(ed25519.PublicKey)}}
}

func TestKey(t *testing.T) {
	dir := signerDir(t)
	out := must(t, "key", "-signer", "1")
	if !strings.Contains(out, "public half") {
		t.Fatalf("said %q", out)
	}
	info, err := os.Stat(filepath.Join(dir, "key.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key.json is %v", info.Mode().Perm())
	}
	before, _ := os.ReadFile(filepath.Join(dir, "key.json"))
	if _, err := call(t, "key", "-signer", "2"); err == nil {
		t.Fatal("a second key overwrote the first")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "key.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("key.json changed")
	}
	// The public half printed by key is the one public prints.
	pub := strings.Fields(must(t, "public"))[2]
	if !strings.Contains(out, pub) {
		t.Fatalf("key printed %q, public %q", out, pub)
	}
}

func TestKeyRefuses(t *testing.T) {
	signerDir(t)
	for _, args := range [][]string{
		{"key"},
		{"key", "-signer", "0"},
		{"key", "-signer", "256"},
		{"key", "-signer", "1", "-test"},
		{"key", "-signer", "1", "extra"},
		{"nonsense"},
		{},
	} {
		if _, err := call(t, args...); err == nil {
			t.Fatalf("%v: no error", args)
		}
	}
}

func TestKeyFileReadableByOthersIsRefused(t *testing.T) {
	dir := signerDir(t)
	must(t, "key", "-signer", "1")
	if err := os.Chmod(filepath.Join(dir, "key.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, "named", "-name", "Lena", "-purpose", "review"); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("got %v", err)
	}
}

func TestKeyFileThatIsNoKey(t *testing.T) {
	dir := signerDir(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		``,
		`{}`,
		`{"signer":1,"seed":"abcd"}`,
		`{"signer":0,"seed":"` + strings.Repeat("ab", 32) + `"}`,
		`{"signer":1,"seed":"` + strings.Repeat("ab", 32) + `","extra":1}`,
		`{"signer":256,"seed":"` + strings.Repeat("ab", 32) + `"}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, "key.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := call(t, "public"); err == nil {
			t.Fatalf("%s read as a key", body)
		}
	}
}

func TestNamed(t *testing.T) {
	signerDir(t)
	must(t, "key", "-signer", "1")
	out := must(t, "named", "-name", "Lena Fischer", "-purpose", "review copy")
	l, err := licence.Check(licence.Key(strings.TrimSpace(out)), trustOf(t))
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Lena Fischer" || l.Signer != 1 {
		t.Fatalf("%+v", l)
	}
	issued := strings.Fields(must(t, "issued"))
	if len(issued) != 1 || issued[0] != licence.Key(strings.TrimSpace(out)).Fingerprint().String() {
		t.Fatalf("issued %v", issued)
	}
}

func TestPartner(t *testing.T) {
	signerDir(t)
	must(t, "key", "-signer", "1")
	file := filepath.Join(t.TempDir(), "bundle.txt")
	must(t, "partner", "-partner", "Bundle Hunt", "-n", "25", "-out", file)
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if len(lines) != 25 {
		t.Fatalf("%d keys in the file", len(lines))
	}
	trust := trustOf(t)
	issued := strings.Fields(must(t, "issued"))
	for i, k := range lines {
		if _, err := licence.Check(licence.Key(k), trust); err != nil {
			t.Fatal(err)
		}
		if issued[i] != licence.Key(k).Fingerprint().String() {
			t.Fatalf("issued %d is not the key in line %d", i, i)
		}
	}
	// A file that is there already is never written over, and nothing is
	// signed for it.
	if _, err := call(t, "partner", "-partner", "Bundle Hunt", "-n", "5", "-out", file); err == nil {
		t.Fatal("wrote over a file of keys")
	}
	if again, _ := os.ReadFile(file); !bytes.Equal(again, text) {
		t.Fatal("the file changed")
	}
	if n := len(strings.Fields(must(t, "issued"))); n != 25 {
		t.Fatalf("%d issued after a refused batch, want 25", n)
	}
	// A refused batch leaves no empty file behind.
	other := filepath.Join(t.TempDir(), "other.txt")
	if _, err := call(t, "partner", "-partner", "", "-n", "5", "-out", other); err == nil {
		t.Fatal("a partner without a name")
	}
	if _, err := os.Stat(other); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused batch left its file")
	}
}

// The test signer keeps its record apart from the real one, so a sandbox
// key is never on a real signer's genuine list.
func TestTestSignerIsKeptApart(t *testing.T) {
	dir := signerDir(t)
	out := must(t, "named", "-test", "-name", "Sandbox", "-purpose", "end to end")
	if _, err := licence.Check(licence.Key(strings.TrimSpace(out)), testTrust()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "record.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the test signer wrote into the real record")
	}
	if _, err := os.Stat(filepath.Join(dir, "test", "record.jsonl")); err != nil {
		t.Fatal(err)
	}
	must(t, "key", "-signer", "1")
	if n := len(strings.Fields(must(t, "issued"))); n != 0 {
		t.Fatalf("the real signer lists %d test keys", n)
	}
	if n := len(strings.Fields(must(t, "issued", "-test"))); n != 1 {
		t.Fatalf("the test signer lists %d keys", n)
	}
}

func TestEditionFlag(t *testing.T) {
	signerDir(t)
	if _, err := call(t, "named", "-test", "-name", "Lena", "-purpose", "review", "-edition", "256"); err == nil {
		t.Fatal("edition 256")
	}
	out := must(t, "named", "-test", "-name", "Lena", "-purpose", "review", "-edition", "2")
	l, err := licence.Check(licence.Key(strings.TrimSpace(out)), testTrust())
	if err != nil || l.Edition != 2 {
		t.Fatalf("%+v %v", l, err)
	}
}

func TestKeyFileFormat(t *testing.T) {
	dir := signerDir(t)
	must(t, "key", "-signer", "7")
	var k keyFile
	body, _ := os.ReadFile(filepath.Join(dir, "key.json"))
	if err := json.Unmarshal(body, &k); err != nil || k.Signer != 7 || len(k.Seed) != 64 {
		t.Fatalf("%s", body)
	}
}
