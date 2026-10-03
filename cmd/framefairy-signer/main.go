// Command framefairy-signer makes licence keys. It runs on a machine of
// ours that is mostly off, holds the private signing key and the record of
// every key it signed, and accepts no connections. It is not shipped. See
// docs/LICENCE.md.
//
//	framefairy-signer key -signer N
//	    make a signing key with number N. Its public half is printed, to be
//	    built into a release before the first key is signed with it
//	framefairy-signer public
//	    print the signer number and the public half
//	framefairy-signer named -name NAME -purpose WHY [-edition E]
//	    sign one key with a name on it, for press and giveaways
//	framefairy-signer partner -partner NAME -n N -out FILE [-edition E]
//	    sign N keys for a partner who sells keys made in advance, into FILE
//	framefairy-signer issued [-signer N]
//	    list the fingerprints of every key signed for a partner or by hand,
//	    the signer's half of its genuine list
//
// Every command takes -dir, the signer's folder, which holds key.json and
// record.jsonl. It is FRAMEFAIRY_SIGNER_DIR, or ~/.framefairy-signer. With
// -test, the commands sign with the test signer, number 0, which no shipped
// build trusts, and keep their record in the folder's test/ beside the real
// one, never in it.
package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"framefairy/licence/signer"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "framefairy-signer:", err)
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

var errUsage = errors.New("usage: framefairy-signer key | public | named | partner | issued, see the top of cmd/framefairy-signer/main.go")

func run(args []string, out io.Writer) error {
	if len(args) < 1 {
		return errUsage
	}
	switch args[0] {
	case "key":
		return makeKey(args[1:], out)
	case "public":
		return public(args[1:], out)
	case "named":
		return named(args[1:], out)
	case "partner":
		return partner(args[1:], out)
	case "issued":
		return issued(args[1:], out)
	}
	return errUsage
}

// options are the flags every command has.
type options struct {
	dir  string
	test bool
}

func flags(name string) (*flag.FlagSet, *options) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	o := &options{}
	dir := os.Getenv("FRAMEFAIRY_SIGNER_DIR")
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".framefairy-signer")
		}
	}
	fs.StringVar(&o.dir, "dir", dir, "the signer's folder")
	fs.BoolVar(&o.test, "test", false, "sign with the test signer, which no shipped build trusts")
	return fs, o
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return errUsage
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w: %q is not a flag", errUsage, fs.Arg(0))
	}
	return nil
}

// keyFile is what key.json holds. The signer number is kept with the key,
// so a key is never used under another number by a mistyped flag.
type keyFile struct {
	Signer uint8  `json:"signer"`
	Seed   string `json:"seed"` // 32 bytes, hexadecimal
}

func makeKey(args []string, out io.Writer) error {
	fs, o := flags("key")
	number := fs.Uint("signer", 0, "the signer number, from 1, one more than the last")
	if err := parse(fs, args); err != nil {
		return err
	}
	if o.test {
		return errors.New("the test signer is made from a sentence, not drawn")
	}
	if *number < 1 || *number > 255 {
		return errors.New("-signer is 1 to 255. 0 is the test signer")
	}
	if err := os.MkdirAll(o.dir, 0o700); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	body, err := json.Marshal(keyFile{Signer: uint8(*number), Seed: hex.EncodeToString(priv.Seed())})
	if err != nil {
		return err
	}
	path := filepath.Join(o.dir, "key.json")
	// O_EXCL: a signing key in use is never overwritten. Replacing one is
	// a rotation, which is planned, not typed.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already holds a signing key. Moving to the next one is a rotation, see docs/LICENCE.md", path)
		}
		return err
	}
	if _, err := f.Write(append(body, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(out, "Signer %d made, its private half is in %s.\n", *number, path)
	fmt.Fprintf(out, "Put a copy of that file in the password manager, and nowhere else.\n\n")
	fmt.Fprintf(out, "The public half, to build into a release before the first key is signed:\n\n  %s\n", hex.EncodeToString(pub))
	return nil
}

// open reads the signing key and opens the record. The caller closes the
// record.
func open(o *options) (*signer.Signer, *signer.Record, error) {
	if o.dir == "" {
		return nil, nil, errors.New("no folder for the signer: set -dir or FRAMEFAIRY_SIGNER_DIR")
	}
	number, seed, err := readKey(o)
	if err != nil {
		return nil, nil, err
	}
	dir := o.dir
	if o.test {
		dir = filepath.Join(o.dir, "test")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	record, err := signer.OpenRecord(filepath.Join(dir, "record.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	if record.Dropped > 0 {
		fmt.Fprintf(os.Stderr, "framefairy-signer: removed %d bytes of a batch that was cut short while it was written. None of its keys ever left.\n", record.Dropped)
	}
	s, err := signer.New(number, ed25519.NewKeyFromSeed(seed), record)
	if err != nil {
		record.Close()
		return nil, nil, err
	}
	return s, record, nil
}

func readKey(o *options) (uint8, []byte, error) {
	if o.test {
		return 0, signer.TestSeed(), nil
	}
	path := filepath.Join(o.dir, "key.json")
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return 0, nil, fmt.Errorf("%s can be read by others (%v). Make it the owner's alone: chmod 600", path, info.Mode().Perm())
	}
	var k keyFile
	dec := json.NewDecoder(io.LimitReader(f, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&k); err != nil {
		return 0, nil, fmt.Errorf("%s: %w", path, err)
	}
	seed, err := hex.DecodeString(k.Seed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return 0, nil, fmt.Errorf("%s does not hold a signing key", path)
	}
	if k.Signer == 0 {
		return 0, nil, fmt.Errorf("%s says signer 0, which is the test signer's number", path)
	}
	return k.Signer, seed, nil
}

func public(args []string, out io.Writer) error {
	fs, o := flags("public")
	if err := parse(fs, args); err != nil {
		return err
	}
	number, seed, err := readKey(o)
	if err != nil {
		return err
	}
	pub := ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey)
	fmt.Fprintf(out, "signer %d %s\n", number, hex.EncodeToString(pub))
	return nil
}

func edition(fs *flag.FlagSet) *uint {
	return fs.Uint("edition", 0, "what the key unlocks, 0 for Frame Fairy as sold today")
}

func checkEdition(e uint) error {
	if e > 255 {
		return fmt.Errorf("-edition is 0 to 255, not %d", e)
	}
	return nil
}

func named(args []string, out io.Writer) error {
	fs, o := flags("named")
	name := fs.String("name", "", "whom the key is licensed to, shown in the app")
	purpose := fs.String("purpose", "", "why it was given, kept in the record instead of the name")
	ed := edition(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := checkEdition(*ed); err != nil {
		return err
	}
	s, record, err := open(o)
	if err != nil {
		return err
	}
	defer record.Close()
	k, err := s.Named(*name, *purpose, uint8(*ed))
	if err != nil {
		return err
	}
	fmt.Fprintln(out, k)
	return nil
}

func partner(args []string, out io.Writer) error {
	fs, o := flags("partner")
	name := fs.String("partner", "", "the partner's name, kept in the record")
	n := fs.Int("n", 0, "how many keys")
	path := fs.String("out", "", "the file the keys go to, one a line. It must not exist yet")
	ed := edition(fs)
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := checkEdition(*ed); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("-out is the file the keys go to")
	}
	s, record, err := open(o)
	if err != nil {
		return err
	}
	defer record.Close()
	// The file is made before anything is signed, so a name that is taken
	// or a folder that is not there costs no keys.
	f, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	keys, err := s.Partner(*name, *n, uint8(*ed))
	if err != nil {
		f.Close()
		os.Remove(*path)
		return err
	}
	w := bufio.NewWriter(f)
	for _, k := range keys {
		w.WriteString(string(k))
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return fmt.Errorf("the keys are signed and recorded, but writing %s failed: %w", *path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(out, "%d keys for %s in %s\n", len(keys), *name, *path)
	return nil
}

func issued(args []string, out io.Writer) error {
	fs, o := flags("issued")
	which := fs.Int("signer", -1, "whose keys, this signer's when not given")
	if err := parse(fs, args); err != nil {
		return err
	}
	s, record, err := open(o)
	if err != nil {
		return err
	}
	defer record.Close()
	number := s.Number()
	if *which >= 0 {
		if *which > 255 {
			return fmt.Errorf("-signer is 0 to 255, not %d", *which)
		}
		number = uint8(*which)
	}
	var b strings.Builder
	for _, f := range record.Issued(number) {
		b.WriteString(f.String())
		b.WriteByte('\n')
	}
	_, err = io.WriteString(out, b.String())
	return err
}
