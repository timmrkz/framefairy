package engine

import (
	"crypto/ed25519"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"framefairy/licence"
)

func testKey(t *testing.T, signer uint8, seed string, name string) licence.Key {
	t.Helper()
	priv := ed25519.NewKeyFromSeed(licence.TestSeed())
	if signer != 0 {
		s := sha256.Sum256([]byte(seed))
		priv = ed25519.NewKeyFromSeed(s[:])
	}
	k, err := licence.Sign(licence.Licence{Format: licence.Format, Signer: signer, ID: licence.ID{1, 2, 3, 4, 5, 6, 7, 8}, Signed: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), Name: name}, priv)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestOnlyABuildFromTheCodeTakesTestKeys(t *testing.T) {
	k := testKey(t, 0, "", "")
	if _, _, err := CheckLicence(string(k), true); err != nil {
		t.Fatalf("a build from the code refused a test key: %v", err)
	}
	_, _, err := CheckLicence(string(k), false)
	if err == nil || !strings.Contains(err.Error(), "test key") {
		t.Fatalf("a build from the workflow took a test key, or said %v", err)
	}
	if _, ok := LicenceTrust(false).Signers[0]; ok {
		t.Fatal("a build from the workflow trusts signer 0")
	}
}

func TestRefusedKeysSayWhy(t *testing.T) {
	good := string(testKey(t, 0, "", ""))
	cases := map[string]string{
		"":                        "not a Frame Fairy licence key",
		"FF1-nonsense":            "not a Frame Fairy licence key",
		"hello":                   "not a Frame Fairy licence key",
		good[:len(good)-2] + "AA": "not signed by Frame Fairy",
		string(testKey(t, 7, "someone else", "")): "signer this version does not know",
	}
	for text, want := range cases {
		_, _, err := CheckLicence(text, true)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%.20q: %v, want %q", text, err, want)
		}
	}
	if _, _, err := CheckLicence("  "+good+"\n", true); err != nil {
		t.Errorf("a key with space around it: %v", err)
	}
}

func TestALicenceIsKeptAndDescribedWithoutReadingIt(t *testing.T) {
	k, _ := keychains(t, nil, nil)
	if _, saved := SavedLicence(); saved {
		t.Fatal("a licence before any was saved")
	}
	key := testKey(t, 0, "", "Lena Fischer")
	about, err := SaveLicence(string(key), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(about, "Lena Fischer") || !strings.Contains(about, "test key") || !strings.Contains(about, "0102-0304-0506-0708") {
		t.Fatalf("described as %q", about)
	}
	reads := k.reads
	got, saved := SavedLicence()
	if !saved || got != about || k.reads != reads {
		t.Fatalf("saved %v as %q after %d reads of the secret", saved, got, k.reads-reads)
	}
	if k.items[licenceItem] != string(key) {
		t.Fatal("the key itself is not what was kept")
	}
	if _, err := SaveLicence("not a key", true); err == nil {
		t.Fatal("a refused key was kept")
	}
	if k.items[licenceItem] != string(key) {
		t.Fatal("a refused key replaced the kept one")
	}
	if _, err := SaveLicence("", true); err != nil {
		t.Fatal(err)
	}
	if _, saved := SavedLicence(); saved {
		t.Fatal("still kept after it was removed")
	}
}

func TestADescriptionFitsWhereItIsKept(t *testing.T) {
	about := DescribeLicence(licence.Licence{Signer: 0, Name: strings.Repeat("é", licence.MaxName/2)})
	if len(about) >= 256 {
		t.Fatalf("a description of %d bytes does not fit the 256 the keychain hint is read back with", len(about))
	}
}
