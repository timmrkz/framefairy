// Package signer is the half of the licence system that makes keys. It
// holds the private signing key and a record of every key it ever signed,
// and nothing else of value. It never accepts a connection: what it signs
// for the pool it hands to the dispenser by calling out. See
// docs/LICENCE.md.
package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"framefairy/licence"
)

// MaxBatch is the most keys one batch can hold. The pool is refilled a
// thousand at a time, and a partner batch larger than this is a mistake
// typed on the command line.
const MaxBatch = 10000

// MaxNote is the longest a partner's name or the purpose of a named key
// can be, in bytes.
const MaxNote = 100

// What a key was signed for.
const (
	KindPool    = "pool"    // for the dispenser to sell
	KindPartner = "partner" // in a file for a partner who sells keys made in advance
	KindNamed   = "named"   // by hand, with a name on it, for press and giveaways
)

// A random source that keeps repeating itself is broken, and the signer
// stops rather than drawing for ever.
const maxDraws = 64

// TestSeed is the seed of the test signer, number 0, made from a sentence
// so anyone can make it again: its keys are for trying things out, and no
// shipped build trusts it. See docs/LICENCE.md.
func TestSeed() []byte {
	sum := sha256.Sum256([]byte("framefairy test signer, never shipped"))
	return sum[:]
}

// Signer signs keys with one signing key and records every one of them.
type Signer struct {
	mu     sync.Mutex
	number uint8
	key    ed25519.PrivateKey
	record *Record
	now    func() time.Time
	random io.Reader
}

// Option changes how a Signer works, for tests.
type Option func(*Signer)

// WithClock makes the signer read the day from now instead of the clock.
func WithClock(now func() time.Time) Option { return func(s *Signer) { s.now = now } }

// WithRandom makes the signer draw key IDs from r instead of crypto/rand.
func WithRandom(r io.Reader) Option { return func(s *Signer) { s.random = r } }

// New makes a signer that signs as signer number with key and writes what
// it signs to record. The record is the caller's to close.
func New(number uint8, key ed25519.PrivateKey, record *Record, opts ...Option) (*Signer, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("a signing key has %d bytes, not %d", ed25519.PrivateKeySize, len(key))
	}
	if record == nil {
		return nil, errors.New("a signer needs its record")
	}
	s := &Signer{number: number, key: key, record: record, now: time.Now, random: rand.Reader}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// Public is the public half of the signing key, the one the app is built
// with.
func (s *Signer) Public() ed25519.PublicKey {
	return s.key.Public().(ed25519.PublicKey)
}

// Number is the signer number every key it signs carries.
func (s *Signer) Number() uint8 { return s.number }

// Pool signs n keys without a name for the dispenser's pool.
func (s *Signer) Pool(n int, edition uint8) ([]licence.Key, error) {
	return s.sign(n, edition, KindPool, "", "")
}

// Partner signs n keys without a name for a partner who sells keys made in
// advance. The partner's name and each key's place in the batch go in the
// record.
func (s *Signer) Partner(partner string, n int, edition uint8) ([]licence.Key, error) {
	if err := checkNote("a partner's name", partner); err != nil {
		return nil, err
	}
	return s.sign(n, edition, KindPartner, partner, "")
}

// Named signs one key with a name on it, for press and giveaways. The name
// is on the key and nowhere else: the record holds the purpose, not whom
// it was for.
func (s *Signer) Named(name, purpose string, edition uint8) (licence.Key, error) {
	if name == "" {
		return "", errors.New("a named key needs a name")
	}
	if err := checkNote("the purpose", purpose); err != nil {
		return "", err
	}
	keys, err := s.sign(1, edition, KindNamed, purpose, name)
	if err != nil {
		return "", err
	}
	return keys[0], nil
}

// sign makes n keys and writes them to the record before it returns any.
// A key that is not in the record does not leave the signer, so no key is
// ever out there that the record cannot account for, and no ID is ever
// drawn twice.
func (s *Signer) sign(n int, edition uint8, kind, note, name string) ([]licence.Key, error) {
	if n < 1 || n > MaxBatch {
		return nil, fmt.Errorf("a batch holds 1 to %d keys, not %d", MaxBatch, n)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	day := s.now().UTC()
	batch := s.record.nextBatch()
	drawn := make(map[licence.ID]bool, n)
	keys := make([]licence.Key, 0, n)
	lines := make([]Line, 0, n)
	for i := range n {
		id, err := s.draw(drawn)
		if err != nil {
			return nil, err
		}
		l := licence.Licence{Format: licence.Format, Signer: s.number, Edition: edition, ID: id, Signed: day, Name: name}
		k, err := licence.Sign(l, s.key)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		lines = append(lines, Line{
			Day:         day.Format(time.DateOnly),
			Signer:      s.number,
			ID:          id.String(),
			Fingerprint: k.Fingerprint().String(),
			Edition:     edition,
			Batch:       batch,
			Index:       i + 1,
			Of:          n,
			Kind:        kind,
			Note:        note,
		})
	}
	if err := s.record.append(lines); err != nil {
		return nil, err
	}
	return keys, nil
}

// draw draws an ID that no key in the record and none drawn so far in this
// batch has.
func (s *Signer) draw(drawn map[licence.ID]bool) (licence.ID, error) {
	for range maxDraws {
		var id licence.ID
		if _, err := io.ReadFull(s.random, id[:]); err != nil {
			return id, fmt.Errorf("drawing a key ID: %w", err)
		}
		if id == (licence.ID{}) || drawn[id] || s.record.has(id) {
			continue
		}
		drawn[id] = true
		return id, nil
	}
	return licence.ID{}, fmt.Errorf("drew %d key IDs and every one was taken, so the random source is broken", maxDraws)
}

// checkNote refuses a partner's name or a purpose that would not read as
// one plain line in the record.
func checkNote(what, s string) error {
	if s == "" {
		return fmt.Errorf("%s cannot be empty", what)
	}
	if len(s) > MaxNote {
		return fmt.Errorf("%s has at most %d bytes, not %d", what, MaxNote, len(s))
	}
	if !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return fmt.Errorf("%s must be UTF-8 with no space at either end", what)
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || r == ' ' || r == ' ' {
			return fmt.Errorf("%s cannot hold the character %U", what, r)
		}
	}
	return nil
}
