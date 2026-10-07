package engine

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"

	"framefairy/licence"
)

// The licence key the app is unlocked with. It is kept where the API keys
// are, the keychain on the Mac, with a description beside it that the
// settings read without reading the key. See docs/LICENCE.md.
const licenceItem = "Frame Fairy: licence key"

// shippedSigners are the public keys of the signers every build trusts, by
// signer number. None yet: the first is made on Tim's Mac with
// framefairy-signer key -signer 1, and its public half goes here before
// any key of it is sold.
var shippedSigners = map[uint8]ed25519.PublicKey{}

// testSigner is signer 0, whose seed anyone can make again. Only a build
// made from the code on this machine trusts it, so the keys of the
// dispenser on this machine, make dispenser, unlock it. A build from the
// build workflow never does: anyone could sign keys for it.
var testSigner = ed25519.NewKeyFromSeed(licence.TestSeed()).Public().(ed25519.PublicKey)

// LicenceTrust is what a build accepts. testKeys is true for a build made
// from the code on this machine and for nothing else.
func LicenceTrust(testKeys bool) licence.Trust {
	signers := make(map[uint8]ed25519.PublicKey, len(shippedSigners)+1)
	for n, pub := range shippedSigners {
		signers[n] = pub
	}
	if testKeys {
		signers[0] = testSigner
	}
	return licence.Trust{Signers: signers}
}

// CheckLicence reads a key as it was pasted or came in a link, and says in
// words why it is refused. The words start small, to sit in a sentence.
func CheckLicence(text string, testKeys bool) (licence.Key, licence.Licence, error) {
	k := licence.Key(strings.TrimSpace(text))
	l, err := licence.Check(k, LicenceTrust(testKeys))
	if err == nil {
		return k, l, nil
	}
	switch {
	case errors.Is(err, licence.ErrSigner):
		if _, tested := licence.Check(k, licence.Trust{Signers: map[uint8]ed25519.PublicKey{0: testSigner}}); tested == nil {
			return "", licence.Licence{}, errors.New("this is a test key, from the local dispenser. Only an app built on this Mac with make takes test keys, never one brought in through Updates")
		}
		return "", licence.Licence{}, errors.New("this key comes from a signer this version does not know yet. An update may")
	case errors.Is(err, licence.ErrSignature):
		return "", licence.Licence{}, errors.New("this key was not signed by Frame Fairy")
	case errors.Is(err, licence.ErrRevoked):
		return "", licence.Licence{}, errors.New("this key has been revoked")
	case errors.Is(err, licence.ErrNotGenuine):
		return "", licence.Licence{}, errors.New("this key is not one Frame Fairy sold")
	}
	return "", licence.Licence{}, errors.New("that is not a Frame Fairy licence key")
}

// DescribeLicence is a licence in a few words, the way the settings show
// it: its ID, whom it is for, and whether it is a test key.
func DescribeLicence(l licence.Licence) string {
	s := "Key " + l.ID.String()
	if l.Name != "" {
		s += ", licensed to " + l.Name
	}
	if l.Signer == 0 {
		s += ", a test key"
	}
	return s
}

// SaveLicence checks a key and keeps it, and gives back how it is
// described. An empty one takes the kept key off the machine.
func SaveLicence(text string, testKeys bool) (string, error) {
	if strings.TrimSpace(text) == "" {
		keys.remove(licenceItem)
		return "", nil
	}
	k, l, err := CheckLicence(text, testKeys)
	if err != nil {
		return "", err
	}
	about := DescribeLicence(l)
	if err := keys.set(licenceItem, string(k), about); err != nil {
		return "", fmt.Errorf("the key could not be kept in the keychain: %w", err)
	}
	return about, nil
}

// SavedLicence says whether a key is kept, and how it was described when
// it was, without reading the key, so opening the settings never puts the
// keychain's box on screen.
func SavedLicence() (about string, saved bool) {
	if !keys.has(licenceItem) {
		return "", false
	}
	return keys.hint(licenceItem), true
}
