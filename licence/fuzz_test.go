package licence

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

// FuzzCheck feeds Check any text at all. It must never panic, must refuse
// with one of the package's errors, and whatever it accepts must be the one
// spelling of that key: signing what it read gives the same text back, so
// the fingerprint the lists hold is the only one the key has.
func FuzzCheck(f *testing.F) {
	for _, ex := range examples {
		f.Add(string(ex.key))
		raw, _ := base64.RawURLEncoding.DecodeString(string(ex.key[len(Prefix):]))
		f.Add(Prefix + base64.StdEncoding.EncodeToString(raw))
	}
	f.Add("")
	f.Add(Prefix)
	f.Add("FF1-AQ")
	f.Add("FF1-AgBlK3V6jcTwSgABLAA")

	trust := testTrust()
	known := []error{ErrMalformed, ErrFormat, ErrSigner, ErrSignature, ErrRevoked, ErrNotGenuine}
	f.Fuzz(func(t *testing.T, s string) {
		l, err := Check(Key(s), trust)
		if err != nil {
			for _, e := range known {
				if errors.Is(err, e) {
					return
				}
			}
			t.Fatalf("refused with an error of no kind: %v", err)
		}
		again, err := Sign(l, testSigner())
		if err != nil {
			t.Fatalf("accepted %q but cannot sign what it read: %v", s, err)
		}
		if string(again) != s {
			t.Fatalf("accepted\n%q\nwhich signs as\n%q", s, again)
		}
	})
}

// FuzzSign signs licences of every shape. Sign either refuses or makes a
// key that Check reads back as exactly the licence given.
func FuzzSign(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint64(0x652b757a8dc4f04a), uint16(300), "")
	f.Add(uint8(0), uint8(0), uint64(0xf6259a14e8059d0d), uint16(315), "Lena Fischer")
	f.Add(uint8(255), uint8(255), uint64(1), uint16(0xffff), "王小明")
	f.Add(uint8(1), uint8(0), uint64(0), uint16(0), "‮")

	trust := Trust{Signers: map[uint8]ed25519.PublicKey{}}
	pub := testSigner().Public().(ed25519.PublicKey)
	for s := 0; s < 256; s++ {
		trust.Signers[uint8(s)] = pub
	}
	f.Fuzz(func(t *testing.T, signer, edition uint8, id uint64, day uint16, name string) {
		want := Licence{Format: 1, Signer: signer, Edition: edition, Signed: Epoch.AddDate(0, 0, int(day)), Name: name}
		for i := range want.ID {
			want.ID[i] = byte(id >> (56 - 8*i))
		}
		k, err := Sign(want, testSigner())
		if err != nil {
			if checkName(name) == nil && want.ID != (ID{}) {
				t.Fatalf("refused a licence it should sign: %v", err)
			}
			return
		}
		got, err := Check(k, trust)
		if err != nil {
			t.Fatalf("signed %q, which does not check: %v", k, err)
		}
		if got != want {
			t.Fatalf("signed %+v, read back %+v", want, got)
		}
		if got.Signed.Location() != time.UTC {
			t.Fatal("the day reads back outside UTC")
		}
	})
}

// FuzzFields signs any bytes at all as the fields of a key, past Sign and
// its rules, the way a broken or stolen signer could. Check must refuse
// them as malformed or of another format, or read a licence that Sign
// turns back into the very same key: no field bytes are accepted that
// Sign would not have written.
func FuzzFields(f *testing.F) {
	for _, ex := range examples {
		raw, _ := base64.RawURLEncoding.DecodeString(string(ex.key[len(Prefix):]))
		f.Add(raw[:len(raw)-ed25519.SignatureSize])
	}
	f.Add([]byte{})
	f.Add([]byte{1})
	f.Add([]byte{1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 1, 64})
	f.Add([]byte{1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0, 1, 1, 0x80})

	trust := testTrust()
	priv := testSigner()
	f.Fuzz(func(t *testing.T, fields []byte) {
		if len(fields) > 1 {
			fields[1] = 0 // signer 0, so the signature is the test signer's
		}
		sig := ed25519.Sign(priv, message(fields))
		k := Key(Prefix + base64.RawURLEncoding.EncodeToString(append(fields, sig...)))
		l, err := Check(k, trust)
		if err != nil {
			if !errors.Is(err, ErrMalformed) && !errors.Is(err, ErrFormat) {
				t.Fatalf("%x refused as %v", fields, err)
			}
			return
		}
		again, err := Sign(l, priv)
		if err != nil {
			t.Fatalf("accepted %x as %+v, which Sign refuses: %v", fields, l, err)
		}
		if again != k {
			t.Fatalf("accepted %x, which Sign writes as %x", fields, again)
		}
	})
}
