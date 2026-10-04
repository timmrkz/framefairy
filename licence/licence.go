package licence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Format is the one key format there is. A later format will be a superset
// of this one, and every format ever issued stays readable.
const Format = 1

// Prefix starts every key of format 1.
const Prefix = "FF1-"

// MaxName is the longest name a key can carry, in bytes of UTF-8.
const MaxName = 64

// context is signed in front of the fields, so a licence signature can never
// be taken for anything else signed with the same key.
const context = "framefairy licence v1\n"

// The fields before the name: format, signer, id, edition, signed day and
// the name's length.
const head = 1 + 1 + 8 + 1 + 2 + 1

// The longest key there can be, in characters, so text that cannot be a key
// is refused before anything is decoded.
var maxText = len(Prefix) + base64.RawURLEncoding.EncodedLen(head+MaxName+ed25519.SignatureSize)

// Epoch is day 0 of the signed day.
var Epoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// lastDay is the last day two bytes can count to.
var lastDay = Epoch.AddDate(0, 0, 0xFFFF)

// Every way a key can be refused. Check wraps one of these, so a caller can
// tell them apart with errors.Is and show the reason.
var (
	// ErrMalformed is text that is not a key at all.
	ErrMalformed = errors.New("not a licence key")
	// ErrFormat is a key of a format this build cannot read.
	ErrFormat = errors.New("a licence key of a newer format")
	// ErrSigner is a key signed with a key this build does not trust.
	ErrSigner = errors.New("a licence key from an unknown signer")
	// ErrSignature is a key whose signature does not hold.
	ErrSignature = errors.New("a licence key that was not signed by us")
	// ErrRevoked is a key on the revocation list.
	ErrRevoked = errors.New("a revoked licence key")
	// ErrNotGenuine is a key of a leaked signer that is not on its genuine list.
	ErrNotGenuine = errors.New("a licence key that is not on its signer's genuine list")
)

// ID says which key a key is. It is drawn at random by the signer, is not
// secret and proves nothing.
type ID [8]byte

// String is the ID as people see it, 652B-757A-8DC4-F04A.
func (id ID) String() string {
	h := strings.ToUpper(hex.EncodeToString(id[:]))
	return h[0:4] + "-" + h[4:8] + "-" + h[8:12] + "-" + h[12:16]
}

// ParseID reads an ID as support might be given it: in either case, with or
// without the dashes, with space around it.
func ParseID(s string) (ID, error) {
	var id ID
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if len(s) != 2*len(id) {
		return id, fmt.Errorf("a key ID has 16 hexadecimal digits, not %d", len(s))
	}
	if _, err := hex.Decode(id[:], []byte(s)); err != nil {
		return id, errors.New("a key ID has only hexadecimal digits")
	}
	return id, nil
}

// Licence is what a key says.
type Licence struct {
	Format  uint8 // the key format, Format today
	Signer  uint8 // which signing key signed it. 0 is the test signer, which no shipped build trusts
	Edition uint8 // what it unlocks. 0 is Frame Fairy as sold today
	ID      ID
	Signed  time.Time // the day it was signed, at midnight UTC
	Name    string    // whom it is licensed to. Empty for every key from the pool
}

// TestSeed is the seed of the test signer, number 0, made from a sentence
// so anyone can make it again: its keys are for trying things out, and no
// shipped build trusts it. See docs/LICENCE.md.
func TestSeed() []byte {
	sum := sha256.Sum256([]byte("framefairy test signer, never shipped"))
	return sum[:]
}

// Key is a licence key as text: Prefix, then the fields and the signature in
// base64url without padding.
type Key string

// Fingerprint names a key in every list: the first 16 bytes of SHA-256 over
// the key's text.
type Fingerprint [16]byte

// Fingerprint is the key's fingerprint. It is the same for anything Check
// accepts, because Check accepts only one spelling of each key.
func (k Key) Fingerprint() Fingerprint {
	sum := sha256.Sum256([]byte(k))
	var f Fingerprint
	copy(f[:], sum[:])
	return f
}

// String is the fingerprint in lowercase hexadecimal, as the lists hold it.
func (f Fingerprint) String() string { return hex.EncodeToString(f[:]) }

// ParseFingerprint reads a fingerprint as the lists hold it: 32 lowercase
// hexadecimal digits and nothing else, so a list compares the same as text
// and as bytes.
func ParseFingerprint(s string) (Fingerprint, error) {
	var f Fingerprint
	if len(s) != 2*len(f) {
		return f, fmt.Errorf("a fingerprint has 32 hexadecimal digits, not %d", len(s))
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return f, errors.New("a fingerprint has only lowercase hexadecimal digits")
		}
	}
	if _, err := hex.Decode(f[:], []byte(s)); err != nil {
		return f, err
	}
	return f, nil
}

// Set is a set of fingerprints.
type Set map[Fingerprint]struct{}

// NewSet makes a set of the fingerprints given.
func NewSet(fs ...Fingerprint) Set {
	s := make(Set, len(fs))
	for _, f := range fs {
		s[f] = struct{}{}
	}
	return s
}

// Has says whether the set holds f. A nil set holds nothing.
func (s Set) Has(f Fingerprint) bool {
	_, ok := s[f]
	return ok
}

// Trust is everything Check weighs a key against.
type Trust struct {
	// Signers are the public keys accepted, by signer number. A shipped
	// build never holds signer 0, the test signer.
	Signers map[uint8]ed25519.PublicKey
	// Revoked is the revocation list from the update feed.
	Revoked Set
	// Genuine holds the genuine list of every signer whose private key
	// leaked. A key of such a signer is accepted only when its fingerprint
	// is on that list. A signer with no entry here has no such limit, but
	// a signer whose entry is an empty set accepts nothing.
	Genuine map[uint8]Set
}

// Sign makes the key for l. It refuses a licence that Check would not give
// back unchanged, and checks its own signature before it returns, so a
// fault while signing can never hand out a key that does not check.
func Sign(l Licence, priv ed25519.PrivateKey) (Key, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("a signing key has %d bytes, not %d", ed25519.PrivateKeySize, len(priv))
	}
	if l.Format != Format {
		return "", fmt.Errorf("this build signs format %d only, not %d", Format, l.Format)
	}
	if l.ID == (ID{}) {
		return "", errors.New("a key ID of all zeros means the random source failed")
	}
	if err := checkName(l.Name); err != nil {
		return "", err
	}
	day, err := dayOf(l.Signed)
	if err != nil {
		return "", err
	}

	fields := make([]byte, 0, head+len(l.Name))
	fields = append(fields, l.Format, l.Signer)
	fields = append(fields, l.ID[:]...)
	fields = append(fields, l.Edition)
	fields = binary.BigEndian.AppendUint16(fields, day)
	fields = append(fields, byte(len(l.Name)))
	fields = append(fields, l.Name...)

	sig := ed25519.Sign(priv, message(fields))
	k := Key(Prefix + base64.RawURLEncoding.EncodeToString(append(fields, sig...)))

	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return "", errors.New("the signing key has no public half")
	}
	back, err := Check(k, Trust{Signers: map[uint8]ed25519.PublicKey{l.Signer: pub}})
	if err != nil {
		return "", fmt.Errorf("a key just signed does not check: %w", err)
	}
	l.Signed = back.Signed
	if back != l {
		return "", errors.New("a key just signed reads back differently")
	}
	return k, nil
}

// Check reads k, which is untrusted text, and says what it licenses. It
// accepts exactly one spelling of each key: no space around it, no padding,
// no other alphabet, nothing after the signature. That is what makes the
// fingerprint of a key one value and no other, so a revoked key cannot slip
// past the list by being written differently. Whoever reads a pasted key
// trims the space around it before it calls Check.
func Check(k Key, t Trust) (Licence, error) {
	l, fields, sig, err := read(k)
	if err != nil {
		return Licence{}, err
	}
	pub, ok := t.Signers[l.Signer]
	if !ok || len(pub) != ed25519.PublicKeySize {
		return Licence{}, fmt.Errorf("%w: signer %d", ErrSigner, l.Signer)
	}
	if !ed25519.Verify(pub, message(fields), sig) {
		return Licence{}, ErrSignature
	}
	f := k.Fingerprint()
	if t.Revoked.Has(f) {
		return Licence{}, fmt.Errorf("%w: %s", ErrRevoked, l.ID)
	}
	if genuine, limited := t.Genuine[l.Signer]; limited && !genuine.Has(f) {
		return Licence{}, fmt.Errorf("%w: %s", ErrNotGenuine, l.ID)
	}
	return l, nil
}

// read takes k apart without weighing its signature.
func read(k Key) (l Licence, fields, sig []byte, err error) {
	s := string(k)
	if len(s) > maxText || !strings.HasPrefix(s, Prefix) {
		return l, nil, nil, ErrMalformed
	}
	s = s[len(Prefix):]
	raw, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil {
		return l, nil, nil, ErrMalformed
	}
	// Strict refuses stray bits at the end and the decoder refuses padding,
	// but it skips line breaks, so the text must also be exactly what the
	// bytes encode to.
	if base64.RawURLEncoding.EncodeToString(raw) != s {
		return l, nil, nil, ErrMalformed
	}
	if len(raw) < 1 {
		return l, nil, nil, ErrMalformed
	}
	if raw[0] != Format {
		return l, nil, nil, fmt.Errorf("%w: format %d", ErrFormat, raw[0])
	}
	if len(raw) < head+ed25519.SignatureSize {
		return l, nil, nil, ErrMalformed
	}
	n := int(raw[head-1])
	if n > MaxName || len(raw) != head+n+ed25519.SignatureSize {
		return l, nil, nil, ErrMalformed
	}
	fields, sig = raw[:head+n], raw[head+n:]

	l.Format = fields[0]
	l.Signer = fields[1]
	copy(l.ID[:], fields[2:10])
	l.Edition = fields[10]
	l.Signed = Epoch.AddDate(0, 0, int(binary.BigEndian.Uint16(fields[11:13])))
	l.Name = string(fields[head:])
	if l.ID == (ID{}) {
		return Licence{}, nil, nil, ErrMalformed
	}
	if checkName(l.Name) != nil {
		return Licence{}, nil, nil, ErrMalformed
	}
	return l, fields, sig, nil
}

func message(fields []byte) []byte {
	return append([]byte(context), fields...)
}

// dayOf counts the days from Epoch to t, by the date t has in UTC.
func dayOf(t time.Time) (uint16, error) {
	if t.IsZero() {
		return 0, errors.New("a licence needs the day it was signed")
	}
	u := t.UTC()
	d := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	if d.Before(Epoch) || d.After(lastDay) {
		return 0, fmt.Errorf("a licence is signed between %s and %s, not on %s",
			Epoch.Format(time.DateOnly), lastDay.Format(time.DateOnly), d.Format(time.DateOnly))
	}
	// Whole days, so the hours lost or won to daylight saving play no part.
	return uint16(d.Sub(Epoch) / (24 * time.Hour)), nil
}

// checkName refuses a name that could not be shown as it reads: one that is
// not UTF-8, carries control or direction characters, which can make a name
// show as another, or has space at either end.
func checkName(name string) error {
	if len(name) > MaxName {
		return fmt.Errorf("a name has at most %d bytes, not %d", MaxName, len(name))
	}
	if !utf8.ValidString(name) {
		return errors.New("a name must be UTF-8")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("a name has no space at either end")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) ||
			r == ' ' || r == ' ' || r == utf8.RuneError {
			return fmt.Errorf("a name cannot hold the character %U", r)
		}
	}
	return nil
}
