package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"framefairy/updates"
)

// This program signs what every app installs, so what it signs has to be
// what the app takes, and what it refuses has to stay refused. Nothing here
// makes a key: on a Mac that would put a private half on the clipboard.

// release is a folder shaped like the repository, as far as this program
// reads it, with a key in it, and FRAMEFAIRY_UPDATE_KEY set to its private
// half. It returns the key.
func release(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(publicKeyFile), 0o755); err != nil {
		t.Fatal(err)
	}
	pub := base64.StdEncoding.EncodeToString(public)
	if err := os.WriteFile(publicKeyFile, []byte(pub+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRAMEFAIRY_UPDATE_KEY", base64.StdEncoding.EncodeToString(private.Seed()))
	return private
}

// signed signs a zip for a channel and returns where its entry was written.
func signed(t *testing.T, channel, name string) string {
	t.Helper()
	zip := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(zip, []byte("the app, zipped, for "+channel), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), channel+".json")
	err := sign([]string{"-zip", zip, "-channel", channel, "-name", name, "-version", "0.1.0-main.abc1234",
		"-commit", "abc1234def56", "-url", "https://example.com/" + channel + ".zip", "-out", out})
	if err != nil {
		t.Fatalf("signing %s: %v", channel, err)
	}
	return out
}

func readBuild(t *testing.T, path string) updates.Build {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b updates.Build
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestASignedBuildIsOneTheAppTakes(t *testing.T) {
	private := release(t)
	b := readBuild(t, signed(t, "main", "main"))
	if err := b.Check(); err != nil {
		t.Fatalf("the entry does not pass the app's own check: %v", err)
	}
	if err := updates.Verify(private.Public().(ed25519.PublicKey), b); err != nil {
		t.Fatalf("the signature does not hold: %v", err)
	}
	sum := sha256.Sum256([]byte("the app, zipped, for main"))
	if b.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("the checksum is %s, not the zip's", b.SHA256)
	}
}

func TestALongTitleIsShortened(t *testing.T) {
	release(t)
	b := readBuild(t, signed(t, "pr-58", "#58 "+strings.Repeat("ä", 300)))
	if n := len([]rune(b.Name)); n != 100 || !strings.HasSuffix(b.Name, "…") {
		t.Errorf("the name is %d characters: %q", n, b.Name)
	}
}

func TestAKeyThatIsNotThePairIsRefused(t *testing.T) {
	release(t)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("FRAMEFAIRY_UPDATE_KEY", base64.StdEncoding.EncodeToString(other.Seed()))
	if _, _, err := keys(); err == nil {
		t.Fatal("a private key that is not the public key's half was taken")
	}
	t.Setenv("FRAMEFAIRY_UPDATE_KEY", "")
	if _, _, err := keys(); err == nil {
		t.Fatal("signing went ahead with no key at all")
	}
}

func TestAnEmptyPublicKeyIsRefused(t *testing.T) {
	release(t)
	if err := os.WriteFile(publicKeyFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := keys(); err == nil {
		t.Fatal("a build was signed for a public key no app carries")
	}
}

func TestANewKeyIsNotMadeOverAnOldOne(t *testing.T) {
	release(t)
	before, _ := os.ReadFile(publicKeyFile)
	if err := makeKey(); err == nil {
		t.Fatal("a new key was made over the one every build trusts")
	}
	if after, _ := os.ReadFile(publicKeyFile); string(after) != string(before) {
		t.Error("the public key was changed")
	}
}

func TestTheListLeavesOutWhatNoAppWouldTake(t *testing.T) {
	release(t)
	good := signed(t, "main", "main")
	pr := signed(t, "pr-58", "#58 Small things")

	// An entry signed with another key, the way a leaked or old key
	// would sign one.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	forged := readBuild(t, pr)
	forged.Channel = "pr-59"
	digest, _ := hex.DecodeString(forged.SHA256)
	forged.Signature = updates.Sign(other, digest)
	forgedPath := filepath.Join(t.TempDir(), "forged.json")
	data, _ := json.Marshal(forged)
	if err := os.WriteFile(forgedPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "channels.json")
	if err := list([]string{"-out", out, "-newest", "pr-58=0123456789ab", good, pr, forgedPath}); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	l, err := updates.Parse(written)
	if err != nil {
		t.Fatalf("the app cannot read the list: %v", err)
	}
	var channels []string
	for _, b := range l.Channels {
		channels = append(channels, b.Channel)
	}
	if strings.Join(channels, " ") != "main pr-58" {
		t.Errorf("the list holds %v, want main and pr-58 and not the forged pr-59", channels)
	}
	if b, _ := l.Find("pr-58"); b.Newest != "0123456789ab" {
		t.Errorf("pr-58's newest commit is %q", b.Newest)
	}
	// When it was written, so the app can tell the newer of the list's two
	// copies.
	if age := time.Since(l.Written); l.Written.IsZero() || age < 0 || age > time.Minute {
		t.Errorf("the list says it was written at %v", l.Written)
	}
}
