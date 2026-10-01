package licence

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test signer of docs/LICENCE.md: its seed is SHA-256 of a sentence,
// so anyone can make it, which is why no shipped build trusts it.
func testSigner() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("framefairy test signer, never shipped"))
	return ed25519.NewKeyFromSeed(seed[:])
}

// A second key pair, for keys signed by someone else.
func otherSigner() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("framefairy licence tests, a stranger"))
	return ed25519.NewKeyFromSeed(seed[:])
}

func testTrust() Trust {
	return Trust{Signers: map[uint8]ed25519.PublicKey{0: testSigner().Public().(ed25519.PublicKey)}}
}

func id(t *testing.T, s string) ID {
	t.Helper()
	i, err := ParseID(s)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// The three keys in docs/LICENCE.md, exactly as the spec prints them.
var examples = []struct {
	name        string
	key         Key
	id          string
	signed      time.Time
	licensee    string
	fingerprint string
}{
	{
		name:        "from the pool",
		key:         "FF1-AQBlK3V6jcTwSgABLABmp524aGzqP1MSVRahBH_9Q8Z57h2_HByz9RmWNvbzEge1J5szJ20G0ejY1lZw01BgFbfKP2SMd7PwcgEVB-IB",
		id:          "652B-757A-8DC4-F04A",
		signed:      day(2026, time.October, 28),
		fingerprint: "62fe5cd6c5086b5e1a4d9e0d2ad392f3",
	},
	{
		name:   "the next key in the pool",
		key:    "FF1-AQDLYWhUTAVFygABLADLKfTX5WVZqY2iCW2mWutlsHNLjU5DfDahvtX0-aTomrqgfF2oSLmNQnYXuYPw9SNJ4vUnkxOb0NAjXe1ATvoH",
		id:     "CB61-6854-4C05-45CA",
		signed: day(2026, time.October, 28),
	},
	{
		name:     "issued by hand",
		key:      "FF1-AQD2JZoU6AWdDQABOwxMZW5hIEZpc2NoZXJt4rt_NPo-K8Cm1V_l-TjIzQbpWRc4y2dYCrOPOAEKcTTOWFqCTR6UCV50bvX8hVQL5CkNEaHEcsrMkHnZOTIN",
		id:       "F625-9A14-E805-9D0D",
		signed:   day(2026, time.November, 12),
		licensee: "Lena Fischer",
	},
}

func TestTestSignerIsTheOneInTheSpec(t *testing.T) {
	got := hex.EncodeToString(testSigner().Public().(ed25519.PublicKey))
	if want := "b959ea225fd22150952415eceb782b933bb8651d2df926492b93d8c205e32ee1"; got != want {
		t.Fatalf("test signer public key %s, the spec says %s", got, want)
	}
}

func TestExamplesInTheSpecCheck(t *testing.T) {
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			l, err := Check(ex.key, testTrust())
			if err != nil {
				t.Fatal(err)
			}
			want := Licence{Format: 1, Signer: 0, Edition: 0, ID: id(t, ex.id), Signed: ex.signed, Name: ex.licensee}
			if l != want {
				t.Fatalf("got %+v, want %+v", l, want)
			}
			if l.ID.String() != ex.id {
				t.Fatalf("ID shows as %s, want %s", l.ID, ex.id)
			}
			if ex.fingerprint != "" && ex.key.Fingerprint().String() != ex.fingerprint {
				t.Fatalf("fingerprint %s, want %s", ex.key.Fingerprint(), ex.fingerprint)
			}
		})
	}
}

// Ed25519 is deterministic, so signing the same licence again must give the
// very key the spec prints. That pins the whole format: field order, byte
// order, the context line and the encoding.
func TestSignMakesTheExamplesInTheSpec(t *testing.T) {
	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			l := Licence{Format: 1, ID: id(t, ex.id), Signed: ex.signed, Name: ex.licensee}
			k, err := Sign(l, testSigner())
			if err != nil {
				t.Fatal(err)
			}
			if k != ex.key {
				t.Fatalf("signed\n%s\nthe spec has\n%s", k, ex.key)
			}
		})
	}
}

func TestKeyLength(t *testing.T) {
	if n := len(examples[0].key); n != 108 {
		t.Fatalf("a key without a name has %d characters, the spec says 108", n)
	}
	l := Licence{Format: 1, ID: ID{1}, Signed: Epoch, Name: strings.Repeat("x", MaxName)}
	k, err := Sign(l, testSigner())
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != maxText {
		t.Fatalf("the longest key has %d characters, maxText is %d", len(k), maxText)
	}
}

// Every bit of every key flipped is refused, and every value of every
// field byte. Nothing about a key can be altered and still check.
func TestEveryChangedByteIsRefused(t *testing.T) {
	t.Parallel()
	trust := testTrust()
	for _, ex := range examples {
		raw, err := base64.RawURLEncoding.DecodeString(string(ex.key[len(Prefix):]))
		if err != nil {
			t.Fatal(err)
		}
		fields := len(raw) - ed25519.SignatureSize
		for i := range raw {
			var values []byte
			if i < fields {
				for v := range 256 {
					values = append(values, byte(v))
				}
			} else {
				for b := range 8 {
					values = append(values, raw[i]^1<<b)
				}
			}
			for _, v := range values {
				if v == raw[i] {
					continue
				}
				changed := append([]byte(nil), raw...)
				changed[i] = v
				k := Key(Prefix + base64.RawURLEncoding.EncodeToString(changed))
				if _, err := Check(k, trust); err == nil {
					t.Fatalf("%s: byte %d set to %#x still checks", ex.name, i, v)
				}
			}
		}
	}
}

// Every character of the text changed to anything that is not base64url is
// refused, and so is every character of the alphabet in the last two
// places, where the stray bits are. The byte test covers the rest.
func TestEveryChangedCharacterIsRefused(t *testing.T) {
	t.Parallel()
	trust := testTrust()
	const url = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	const other = "+/= \n\r\t.\x00\x7f\xc3\xff"
	for _, k := range append(spellings(t), examples[0].key) {
		for i := 0; i < len(k); i++ {
			try := other
			if i >= len(k)-2 {
				try += url
			}
			for j := 0; j < len(try); j++ {
				c := try[j]
				if c == k[i] {
					continue
				}
				bad := k[:i] + Key(c) + k[i+1:]
				if _, err := Check(bad, trust); err == nil {
					t.Fatalf("%s: character %d set to %q still checks", k, i, c)
				}
			}
		}
	}
}

// spellings are keys whose bytes do not fill the last character, a name
// of one byte and of two, so they have stray bits and padding that a key
// of a whole number of three bytes, like the examples, does not.
func spellings(t *testing.T) []Key {
	t.Helper()
	var keys []Key
	for _, name := range []string{"x", "xy"} {
		k, err := Sign(Licence{Format: 1, ID: ID{5}, Signed: Epoch, Name: name}, testSigner())
		if err != nil {
			t.Fatal(err)
		}
		if (len(k)-len(Prefix))%4 == 0 {
			t.Fatalf("name %q makes a key with no stray bits", name)
		}
		keys = append(keys, k)
	}
	return keys
}

func TestOneSpellingPerKey(t *testing.T) {
	trust := testTrust()
	for _, k := range append(spellings(t), examples[0].key, examples[2].key) {
		if _, err := Check(k, trust); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		raw, err := base64.RawURLEncoding.DecodeString(string(k[len(Prefix):]))
		if err != nil {
			t.Fatal(err)
		}
		last := k[len(k)-1]
		cases := map[string]Key{
			"padding":                k + "=",
			"two paddings":           k + "==",
			"space before":           " " + k,
			"space after":            k + " ",
			"line break after":       k + "\n",
			"line break inside":      k[:40] + "\n" + k[40:],
			"carriage return inside": k[:40] + "\r" + k[40:],
			"lowercase prefix":       "ff1-" + k[len(Prefix):],
			"no prefix":              k[len(Prefix):],
			"another prefix":         "FF2-" + k[len(Prefix):],
			"a character more":       k + "A",
			"a character less":       k[:len(k)-1],
			"the key twice":          k + k,
			"empty":                  "",
			"only the prefix":        Prefix,
			"prefix and one byte":    Prefix + "AQ",
		}
		if len(raw)%3 != 0 {
			cases["url with padding"] = Prefix + Key(base64.URLEncoding.EncodeToString(raw))
			// The last character carries bits past the end of the bytes.
			// Setting one of them spells the same bytes another way.
			for _, c := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_" {
				other := k[:len(k)-1] + Key(c)
				back, err := base64.RawURLEncoding.DecodeString(string(other[len(Prefix):]))
				if c != rune(last) && err == nil && string(back) == string(raw) {
					cases["stray bits set to "+string(c)] = other
				}
			}
		}
		for _, enc := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding} {
			if other := Prefix + Key(enc.EncodeToString(raw)); other != k {
				cases["standard alphabet "+string(other[len(other)-1])] = other
			}
		}
		for name, bad := range cases {
			if _, err := Check(bad, trust); !errors.Is(err, ErrMalformed) {
				t.Fatalf("%s, %s: got %v, want ErrMalformed", k, name, err)
			}
		}
		if len(raw)%3 != 0 {
			stray := 0
			for name := range cases {
				if strings.HasPrefix(name, "stray bits") {
					stray++
				}
			}
			if stray == 0 {
				t.Fatalf("%s: no other spelling with stray bits was tried", k)
			}
		}
	}
}

func TestNothingButASignature(t *testing.T) {
	raw, _ := base64.RawURLEncoding.DecodeString(string(examples[0].key[len(Prefix):]))
	k := Prefix + Key(base64.RawURLEncoding.EncodeToString(raw[len(raw)-64:]))
	if _, err := Check(k, testTrust()); err == nil {
		t.Fatal("a signature alone checks")
	}
}

func TestTooLongIsRefusedBeforeDecoding(t *testing.T) {
	k := Prefix + Key(strings.Repeat("A", 1<<20))
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("got %v, want ErrMalformed", err)
	}
	trust := testTrust()
	allocs := testing.AllocsPerRun(10, func() { _, _ = Check(k, trust) })
	if allocs > 2 {
		t.Fatalf("refusing a megabyte of text took %v allocations, it should take none of its own", allocs)
	}
}

// A key whose fields were signed without the context line, as if the
// signature had been made for something else, is refused.
func TestSignatureWithoutContextIsRefused(t *testing.T) {
	good := examples[0].key
	raw, _ := base64.RawURLEncoding.DecodeString(string(good[len(Prefix):]))
	fields := raw[:head]
	sig := ed25519.Sign(testSigner(), fields)
	k := Key(Prefix + base64.RawURLEncoding.EncodeToString(append(append([]byte(nil), fields...), sig...)))
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrSignature) {
		t.Fatalf("got %v, want ErrSignature", err)
	}
}

func TestSignedBySomeoneElse(t *testing.T) {
	l := Licence{Format: 1, ID: ID{1, 2, 3}, Signed: day(2026, time.March, 3)}
	k, err := Sign(l, otherSigner())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrSignature) {
		t.Fatalf("got %v, want ErrSignature", err)
	}
}

func TestUnknownSigner(t *testing.T) {
	l := Licence{Format: 1, Signer: 7, ID: ID{1}, Signed: Epoch}
	k, err := Sign(l, testSigner())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrSigner) {
		t.Fatalf("got %v, want ErrSigner", err)
	}
	if _, err := Check(k, Trust{}); !errors.Is(err, ErrSigner) {
		t.Fatalf("with no trust at all: got %v, want ErrSigner", err)
	}
	broken := Trust{Signers: map[uint8]ed25519.PublicKey{7: {1, 2, 3}}}
	if _, err := Check(k, broken); !errors.Is(err, ErrSigner) {
		t.Fatalf("with a public key of the wrong size: got %v, want ErrSigner", err)
	}
}

// Signer numbers are signed, so a key of signer 0 cannot be passed off as
// one of signer 1 even when both public keys are trusted.
func TestSignerNumberIsSigned(t *testing.T) {
	trust := Trust{Signers: map[uint8]ed25519.PublicKey{
		0: testSigner().Public().(ed25519.PublicKey),
		1: otherSigner().Public().(ed25519.PublicKey),
	}}
	raw, _ := base64.RawURLEncoding.DecodeString(string(examples[0].key[len(Prefix):]))
	raw[1] = 1
	k := Key(Prefix + base64.RawURLEncoding.EncodeToString(raw))
	if _, err := Check(k, trust); !errors.Is(err, ErrSignature) {
		t.Fatalf("got %v, want ErrSignature", err)
	}
}

func TestNewerFormat(t *testing.T) {
	raw, _ := base64.RawURLEncoding.DecodeString(string(examples[0].key[len(Prefix):]))
	raw[0] = 2
	k := Key(Prefix + base64.RawURLEncoding.EncodeToString(raw))
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrFormat) {
		t.Fatalf("got %v, want ErrFormat", err)
	}
}

func TestRevoked(t *testing.T) {
	k := examples[0].key
	trust := testTrust()
	trust.Revoked = NewSet(examples[1].key.Fingerprint())
	if _, err := Check(k, trust); err != nil {
		t.Fatalf("another key on the list refused this one: %v", err)
	}
	trust.Revoked = NewSet(examples[1].key.Fingerprint(), k.Fingerprint())
	if _, err := Check(k, trust); !errors.Is(err, ErrRevoked) {
		t.Fatalf("got %v, want ErrRevoked", err)
	}
	// The spec's example list, read as the app would read it from the feed.
	f, err := ParseFingerprint("62fe5cd6c5086b5e1a4d9e0d2ad392f3")
	if err != nil {
		t.Fatal(err)
	}
	trust.Revoked = NewSet(f)
	if _, err := Check(k, trust); !errors.Is(err, ErrRevoked) {
		t.Fatalf("the spec's list: got %v, want ErrRevoked", err)
	}
}

func TestGenuine(t *testing.T) {
	sold, unsold := examples[0].key, examples[1].key
	trust := testTrust()

	trust.Genuine = map[uint8]Set{0: NewSet(sold.Fingerprint())}
	if _, err := Check(sold, trust); err != nil {
		t.Fatalf("a key on the genuine list: %v", err)
	}
	if _, err := Check(unsold, trust); !errors.Is(err, ErrNotGenuine) {
		t.Fatalf("a key not on the genuine list: got %v, want ErrNotGenuine", err)
	}

	trust.Genuine = map[uint8]Set{0: {}}
	if _, err := Check(sold, trust); !errors.Is(err, ErrNotGenuine) {
		t.Fatalf("an empty genuine list: got %v, want ErrNotGenuine", err)
	}
	trust.Genuine = map[uint8]Set{0: nil}
	if _, err := Check(sold, trust); !errors.Is(err, ErrNotGenuine) {
		t.Fatalf("a nil genuine list: got %v, want ErrNotGenuine", err)
	}

	trust.Genuine = map[uint8]Set{1: {}}
	if _, err := Check(sold, trust); err != nil {
		t.Fatalf("another signer's genuine list: %v", err)
	}

	// Revoked wins over genuine.
	trust.Genuine = map[uint8]Set{0: NewSet(sold.Fingerprint())}
	trust.Revoked = NewSet(sold.Fingerprint())
	if _, err := Check(sold, trust); !errors.Is(err, ErrRevoked) {
		t.Fatalf("genuine and revoked: got %v, want ErrRevoked", err)
	}
}

func TestSignRefuses(t *testing.T) {
	good := Licence{Format: 1, ID: ID{1}, Signed: Epoch}
	cases := map[string]func(*Licence){
		"format 0":                 func(l *Licence) { l.Format = 0 },
		"format 2":                 func(l *Licence) { l.Format = 2 },
		"zero ID":                  func(l *Licence) { l.ID = ID{} },
		"no day":                   func(l *Licence) { l.Signed = time.Time{} },
		"before the epoch":         func(l *Licence) { l.Signed = Epoch.Add(-time.Second) },
		"after the last day":       func(l *Licence) { l.Signed = lastDay.AddDate(0, 0, 1) },
		"name too long":            func(l *Licence) { l.Name = strings.Repeat("x", MaxName+1) },
		"name too long in bytes":   func(l *Licence) { l.Name = strings.Repeat("ü", MaxName/2+1) },
		"name not UTF-8":           func(l *Licence) { l.Name = "Lena \xff" },
		"name with a line break":   func(l *Licence) { l.Name = "Lena\nFischer" },
		"name with a tab":          func(l *Licence) { l.Name = "Lena\tFischer" },
		"name with a NUL":          func(l *Licence) { l.Name = "Lena\x00" },
		"name with C1 control":     func(l *Licence) { l.Name = "Lena\u0085" },
		"name turned around":       func(l *Licence) { l.Name = "Lena ‮Fischer" },
		"name with isolate":        func(l *Licence) { l.Name = "Lena ⁦Fischer" },
		"name with line separator": func(l *Licence) { l.Name = "Lena Fischer" },
		"name with space before":   func(l *Licence) { l.Name = " Lena" },
		"name with space after":    func(l *Licence) { l.Name = "Lena " },
		"name with only space":     func(l *Licence) { l.Name = " " },
		"name with replacement":    func(l *Licence) { l.Name = "Lena �" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			l := good
			change(&l)
			if k, err := Sign(l, testSigner()); err == nil {
				t.Fatalf("signed %s", k)
			}
		})
	}
	if _, err := Sign(good, testSigner()); err != nil {
		t.Fatalf("the good licence: %v", err)
	}
	if _, err := Sign(good, testSigner()[:32]); err == nil {
		t.Fatal("signed with half a private key")
	}
	if _, err := Sign(good, nil); err == nil {
		t.Fatal("signed with no private key")
	}
}

// A name Sign refuses is refused by Check too, even with a valid signature,
// so a key signed by a broken signer is never shown.
func TestCheckRefusesNamesSignWould(t *testing.T) {
	fields := []byte{1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 1}
	for _, name := range []string{"Lena\n", "‮Lena", "Lena\xff", " Lena"} {
		f := append(append(append([]byte(nil), fields...), byte(len(name))), name...)
		sig := ed25519.Sign(testSigner(), message(f))
		k := Key(Prefix + base64.RawURLEncoding.EncodeToString(append(f, sig...)))
		if _, err := Check(k, testTrust()); !errors.Is(err, ErrMalformed) {
			t.Fatalf("name %q: got %v, want ErrMalformed", name, err)
		}
	}
	// The same for an ID of all zeros.
	f := []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0}
	sig := ed25519.Sign(testSigner(), message(f))
	k := Key(Prefix + base64.RawURLEncoding.EncodeToString(append(f, sig...)))
	if _, err := Check(k, testTrust()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("zero ID: got %v, want ErrMalformed", err)
	}
}

// The day is the date in UTC, whatever the hour and the zone of the time
// given, and the first and last day both round-trip.
func TestSignedDay(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no time zone database:", err)
	}
	cases := []struct {
		in   time.Time
		want time.Time
	}{
		{Epoch, Epoch},
		{Epoch.Add(23*time.Hour + 59*time.Minute), Epoch},
		{lastDay, lastDay},
		{lastDay.Add(23 * time.Hour), lastDay},
		// 00:30 in Berlin on 2 March is 1 March in UTC.
		{time.Date(2026, time.March, 2, 0, 30, 0, 0, berlin), day(2026, time.March, 1)},
		// Across the change to summer time.
		{time.Date(2026, time.March, 29, 12, 0, 0, 0, berlin), day(2026, time.March, 29)},
		{time.Date(2026, time.October, 28, 9, 0, 0, 0, berlin), day(2026, time.October, 28)},
	}
	for _, c := range cases {
		k, err := Sign(Licence{Format: 1, ID: ID{9}, Signed: c.in}, testSigner())
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		l, err := Check(k, testTrust())
		if err != nil {
			t.Fatal(err)
		}
		if !l.Signed.Equal(c.want) || l.Signed.Location() != time.UTC {
			t.Fatalf("%s reads back as %s, want %s", c.in, l.Signed, c.want)
		}
	}
	if _, err := Sign(Licence{Format: 1, ID: ID{9}, Signed: time.Date(2025, time.December, 31, 23, 30, 0, 0, time.UTC)}, testSigner()); err == nil {
		t.Fatal("signed on the last day of 2025")
	}
	// 00:30 in Berlin on 1 January 2026 is still 2025 in UTC.
	if _, err := Sign(Licence{Format: 1, ID: ID{9}, Signed: time.Date(2026, time.January, 1, 0, 30, 0, 0, berlin)}, testSigner()); err == nil {
		t.Fatal("signed on a day that is still 2025 in UTC")
	}
}

func TestNames(t *testing.T) {
	for _, name := range []string{
		"Lena Fischer",
		"Jürgen Müller-Lüdenscheidt",
		"Zoë O'Brien",
		"王小明",
		"محمد",
		"Ἀριστοτέλης",
		"🎙️ Podcast Crew",
		strings.Repeat("ü", MaxName/2),
		"x",
	} {
		k, err := Sign(Licence{Format: 1, ID: ID{4}, Signed: Epoch, Name: name}, testSigner())
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		l, err := Check(k, testTrust())
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if l.Name != name {
			t.Fatalf("name %q reads back as %q", name, l.Name)
		}
	}
}

func TestEditionsAndSigners(t *testing.T) {
	trust := Trust{Signers: map[uint8]ed25519.PublicKey{}}
	for _, s := range []uint8{0, 1, 255} {
		trust.Signers[s] = testSigner().Public().(ed25519.PublicKey)
	}
	for _, s := range []uint8{0, 1, 255} {
		for _, e := range []uint8{0, 1, 255} {
			want := Licence{Format: 1, Signer: s, Edition: e, ID: ID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, Signed: Epoch}
			k, err := Sign(want, testSigner())
			if err != nil {
				t.Fatal(err)
			}
			got, err := Check(k, trust)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		}
	}
}

func TestParseID(t *testing.T) {
	want := ID{0x65, 0x2b, 0x75, 0x7a, 0x8d, 0xc4, 0xf0, 0x4a}
	for _, s := range []string{"652B-757A-8DC4-F04A", "652b-757a-8dc4-f04a", "652B757A8DC4F04A", "  652B-757A-8DC4-F04A\n"} {
		got, err := ParseID(s)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if got != want {
			t.Fatalf("%q reads as %s", s, got)
		}
	}
	for _, s := range []string{"", "652B-757A-8DC4", "652B-757A-8DC4-F04G", "652B-757A-8DC4-F04A-00", "652B 757A 8DC4 F04A"} {
		if _, err := ParseID(s); err == nil {
			t.Fatalf("%q was read as an ID", s)
		}
	}
}

func TestParseFingerprint(t *testing.T) {
	f := examples[0].key.Fingerprint()
	got, err := ParseFingerprint(f.String())
	if err != nil || got != f {
		t.Fatalf("round trip: %s, %v", got, err)
	}
	for _, s := range []string{
		"",
		"62FE5CD6C5086B5E1A4D9E0D2AD392F3",
		"62fe5cd6c5086b5e1a4d9e0d2ad392f",
		"62fe5cd6c5086b5e1a4d9e0d2ad392f3a",
		"62fe5cd6c5086b5e1a4d9e0d2ad392g3",
		" 62fe5cd6c5086b5e1a4d9e0d2ad392f3",
		"62fe5cd6c5086b5e1a4d9e0d2ad392f3\n",
	} {
		if _, err := ParseFingerprint(s); err == nil {
			t.Fatalf("%q was read as a fingerprint", s)
		}
	}
}

func TestNilSetHoldsNothing(t *testing.T) {
	var s Set
	if s.Has(Fingerprint{}) {
		t.Fatal("a nil set holds something")
	}
}

// Check is called from every request of the dispenser at once, with one
// Trust shared between them.
func TestCheckFromManyGoroutines(t *testing.T) {
	trust := testTrust()
	trust.Revoked = NewSet(examples[1].key.Fingerprint())
	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Go(func() {
			for i := 0; i < 50; i++ {
				if _, err := Check(examples[0].key, trust); err != nil {
					t.Error(err)
					return
				}
				if _, err := Check(examples[1].key, trust); !errors.Is(err, ErrRevoked) {
					t.Errorf("got %v, want ErrRevoked", err)
					return
				}
				if _, err := Sign(Licence{Format: 1, ID: ID{byte(g) + 1, byte(i)}, Signed: Epoch}, testSigner()); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}
