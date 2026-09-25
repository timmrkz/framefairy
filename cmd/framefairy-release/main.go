// Command framefairy-release makes the update key, and signs and lists the
// builds the app updates itself to. It is not shipped. Tim runs it once, to
// make the key, and the build workflow runs the rest. See docs/UPDATES.md.
//
//	framefairy-release key
//	    make an update key: the public half into the repository, the
//	    private half to the clipboard, for GitHub's secrets and the
//	    password manager
//	framefairy-release sign -zip F -channel C -name N -version V -commit X -url U -out F
//	    sign a build with the private half, read from FRAMEFAIRY_UPDATE_KEY,
//	    and write its entry for the channel list
//	framefairy-release list -out F ENTRY...
//	    write the channel list from the entries of every channel
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"framefairy/updates"
)

// publicKeyFile is where the public half lives, beside the program that
// builds it in.
const publicKeyFile = "cmd/framefairy-app/update-key.txt"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "key":
		err = makeKey()
	case "sign":
		err = sign(os.Args[2:])
	case "list":
		err = list(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "framefairy-release:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: framefairy-release key | sign ... | list -out FILE ENTRY...")
	os.Exit(2)
}

// makeKey makes the pair. The private half never touches a file: it goes to
// the clipboard, from where it is pasted into GitHub's secrets and the
// password manager, and the clipboard is emptied by the next copy.
func makeKey() error {
	if text, _ := os.ReadFile(publicKeyFile); strings.TrimSpace(string(text)) != "" {
		return fmt.Errorf("%s already holds a key. Every build made with it trusts only that key, so a new one is a decision, not a command: empty the file first if it really is meant", publicKeyFile)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	pub := base64.StdEncoding.EncodeToString(public)
	priv := base64.StdEncoding.EncodeToString(private.Seed())
	if err := os.WriteFile(publicKeyFile, []byte(pub+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("The public half is in %s:\n\n  %s\n\n", publicKeyFile, pub)
	copy := exec.Command("pbcopy")
	copy.Stdin = strings.NewReader(priv)
	if copy.Run() == nil {
		fmt.Println("The private half is on the clipboard. Paste it into")
		fmt.Println("  1. GitHub, the repository's Settings, Secrets and variables, Actions,")
		fmt.Println("     New repository secret, named FRAMEFAIRY_UPDATE_KEY")
		fmt.Println("  2. your password manager")
		fmt.Println("and then copy something else, so it leaves the clipboard.")
		return nil
	}
	fmt.Println("There is no clipboard here, so this is the private half. Put it in")
	fmt.Println("GitHub's secrets as FRAMEFAIRY_UPDATE_KEY and in your password manager,")
	fmt.Println("and nowhere else:")
	fmt.Printf("\n  %s\n", priv)
	return nil
}

// keys reads both halves and refuses a pair that does not belong together:
// a build signed with a key the app does not carry is a build nobody can
// install, and it is better found here than on the Mac.
func keys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	secret := os.Getenv("FRAMEFAIRY_UPDATE_KEY")
	if secret == "" {
		return nil, nil, errors.New("FRAMEFAIRY_UPDATE_KEY is not set")
	}
	private, err := updates.PrivateKey(secret)
	if err != nil {
		return nil, nil, err
	}
	text, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, nil, err
	}
	public, err := updates.PublicKey(string(text))
	if err != nil {
		return nil, nil, err
	}
	if public == nil {
		return nil, nil, fmt.Errorf("%s is empty, so no app would trust the build", publicKeyFile)
	}
	if !public.Equal(private.Public()) {
		return nil, nil, fmt.Errorf("FRAMEFAIRY_UPDATE_KEY is not the private half of %s", publicKeyFile)
	}
	return private, public, nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip holding the app")
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	name := fs.String("name", "", "what the app shows for the channel")
	version := fs.String("version", "", "the build's version")
	commit := fs.String("commit", "", "the commit it was built from")
	where := fs.String("url", "", "where the zip will be downloaded from")
	out := fs.String("out", "", "where the entry goes")
	_ = fs.Parse(args)
	private, public, err := keys()
	if err != nil {
		return err
	}
	f, err := os.Open(*zipPath)
	if err != nil {
		return err
	}
	digest, size, err := updates.Digest(f)
	f.Close()
	if err != nil {
		return err
	}
	b := updates.Build{
		Channel: *channel, Name: *name, Version: *version, Commit: *commit, URL: *where,
		Size: size, SHA256: hex.EncodeToString(digest), Signature: updates.Sign(private, digest),
		Published: time.Now().UTC().Truncate(time.Second),
	}
	if err := b.Check(); err != nil {
		return err
	}
	if err := updates.Verify(public, b); err != nil {
		return err
	}
	return writeJSON(*out, b)
}

// list writes the channel list from every channel's entry. Each entry is
// read with the same rules as the app reads the list, and checked against
// the public key, so the list never promises a build no app will take.
func list(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	out := fs.String("out", "", "where the channel list goes")
	_ = fs.Parse(args)
	text, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return err
	}
	public, err := updates.PublicKey(string(text))
	if err != nil || public == nil {
		return fmt.Errorf("no public key in %s", publicKeyFile)
	}
	var l updates.List
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var b updates.Build
		if err := json.Unmarshal(data, &b); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := b.Check(); err != nil {
			fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
			continue
		}
		if err := updates.Verify(public, b); err != nil {
			fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
			continue
		}
		l.Channels = append(l.Channels, b)
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	// The same reading the app does, so what is written is what it keeps.
	parsed, err := updates.Parse(data)
	if err != nil {
		return err
	}
	return writeJSON(*out, parsed)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
