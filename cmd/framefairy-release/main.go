// Command framefairy-release decides, signs and lists the builds the app
// updates itself to, makes the update key, and tells of a scheduled run
// that failed on main. It is not shipped. Tim
// runs it once, to make the key, and the workflows run the rest, one
// command a step: every decision about a build is made here, in Go, and
// none in the workflows' shell. See docs/UPDATES.md and
// docs/decisions/2026-11-14-update-decisions-in-go.md.
//
//	framefairy-release key
//	    make an update key: the public half into the repository, the
//	    private half to the clipboard, for GitHub's secrets and the
//	    password manager
//	framefairy-release build-plan -event E -branch B -commit X
//	    for builds.yml: what a push is a build of, and whether it needs one
//	framefairy-release publish-plan -run R -branch B -commit X
//	    for publish.yml: what a finished build is, and whether to publish
//	    it, only for a commit its channel's history holds
//	framefairy-release check -zip F [-refusal F]
//	    refuse a build that could read input meant for other apps, watch
//	    the screen or the camera, or ask for administrator rights, see
//	    inputs.go. The workflow runs it before sign, in a step that does
//	    not hold the key
//	framefairy-release pack -zip F -out D
//	    write the build again as the zip it is published as, its files
//	    stored, with its parts, a piece for every file and the zip packed
//	    with zstd, see pack.go
//	framefairy-release sign -zip F -channel C -name N -version V -commit X -url U -out F [-parts F] [-refusal F]
//	    sign a build that check passed with the private half, read from
//	    FRAMEFAIRY_UPDATE_KEY, and write its entry for the channel list
//	framefairy-release publish -zip F -as NAME -entry F -channel C -commit X [-packed D]
//	    upload the build and its entry, and what pack wrote beside it,
//	    and mark a pull request's commit Ready to update to
//	framefairy-release not-published -commit X -run URL [-refusal F]
//	    mark a commit whose build was not published, and why
//	framefairy-release channels
//	    write the channel list again from every entry, and take away what
//	    nobody can reach any more
//	framefairy-release unstick
//	    cancel a publish that has waited half an hour for its environment
//	framefairy-release failed -workflow W -run URL -commit X
//	    open the issue that says a scheduled run of W failed on main, or
//	    add the run to the one that is open, see failed.go
//	framefairy-release behind -status N -table F
//	    keep the issue of the tools the app ships that are behind their
//	    newest release, from how scripts/upstream.sh --behind ended and
//	    what it wrote, see behind.go
//
// Every command but key, check and sign asks GitHub, with GH_TOKEN and
// GH_REPO, through the interface in github.go.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

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
	args := os.Args[2:]
	switch os.Args[1] {
	case "key":
		err = makeKey()
	case "build-plan", "publish-plan":
		err = planCommand(os.Args[1], args, gitHubFromEnv())
	case "pack":
		err = packCommand(args)
	case "check":
		err = check(args)
	case "sign":
		err = sign(args)
	case "publish":
		err = publishCommand(args, gitHubFromEnv())
	case "not-published":
		err = notPublishedCommand(args, gitHubFromEnv())
	case "channels":
		err = channelsCommand(gitHubFromEnv())
	case "unstick":
		err = unstick(gitHubFromEnv(), time.Now(), os.Stdout)
	case "failed":
		err = failedCommand(args, gitHubFromEnv(), time.Now(), os.Stdout)
	case "behind":
		err = behindCommand(args, gitHubFromEnv(), time.Now(), os.Stdout)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "framefairy-release:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: framefairy-release key | build-plan | publish-plan | pack | check | sign | publish | not-published | channels | unstick | failed | behind, see the head of main.go")
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
		fmt.Println("  1. GitHub, the repository's Settings, Environments, updates,")
		fmt.Println("     Add environment secret, named FRAMEFAIRY_UPDATE_KEY")
		fmt.Println("  2. your password manager")
		fmt.Println("and then copy something else, so it leaves the clipboard.")
		return nil
	}
	fmt.Println("There is no clipboard here, so this is the private half. Put it in")
	fmt.Println("the secrets of GitHub's environment updates as FRAMEFAIRY_UPDATE_KEY and")
	fmt.Println("in your password manager,")
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

// check reads a build before it is signed. It is a command of its own,
// and not the first thing sign does, so the workflow can run it in a step
// that does not hold the key: the build is a branch's code, and its bytes
// are read before anybody has reviewed them.
func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip holding the app")
	refusal := fs.String("refusal", "", "where to keep why the build was refused, for not-published")
	_ = fs.Parse(args)
	return refused(*refusal, checkInputs(*zipPath), os.Stderr)
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	refusal := fs.String("refusal", "", "where to keep why the build was refused, for not-published")
	zipPath := fs.String("zip", "", "the zip holding the app")
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	name := fs.String("name", "", "what the app shows for the channel")
	version := fs.String("version", "", "the build's version")
	commit := fs.String("commit", "", "the commit it was built from")
	where := fs.String("url", "", "where the zip will be downloaded from")
	out := fs.String("out", "", "where the entry goes")
	parts := fs.String("parts", "", "the parts pack wrote, beside the stored zip and the zip packed, if it was packed")
	_ = fs.Parse(args)
	return refused(*refusal, signBuild(*zipPath, *channel, *name, *version, *commit, *where, *parts, *out), os.Stderr)
}

// unpacksTo says whether a zip packed with zstd unpacks to the zip with
// this SHA-256.
func unpacksTo(packed, digest []byte) error {
	dec, err := zstd.NewReader(bytes.NewReader(packed))
	if err != nil {
		return err
	}
	defer dec.Close()
	h := sha256.New()
	if _, err := io.Copy(h, dec); err != nil {
		return fmt.Errorf("the packed zip does not unpack: %w", err)
	}
	if !bytes.Equal(h.Sum(nil), digest) {
		return errors.New("the packed zip is of another zip")
	}
	return nil
}

// signBuild signs the zip and writes its entry. With parts, it signs the
// stored zip beside them too, once they are read as its parts and the zip
// packed beside them unpacks to it, and the entry names it. An app that
// knows parts installs that zip, and an app from before installs the zip.
func signBuild(zipPath, channel, name, version, commit, where, parts, out string) error {
	private, public, err := keys()
	if err != nil {
		return err
	}
	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	digest, size, err := updates.Digest(f)
	f.Close()
	if err != nil {
		return err
	}
	// A pull request's title can be long, and the app shows it in a list.
	shown := []rune(strings.TrimSpace(name))
	if len(shown) > 100 {
		shown = append(shown[:99], '…')
	}
	b := updates.Build{
		Channel: channel, Name: string(shown), Version: version, Commit: commit, URL: where,
		Size: size, SHA256: hex.EncodeToString(digest), Signature: updates.Sign(private, digest),
		Published: time.Now().UTC().Truncate(time.Second),
	}
	if parts != "" {
		if err := sameApp(zipPath, filepath.Join(filepath.Dir(parts), storedZip)); err != nil {
			return err
		}
		st, err := signStored(private, b, filepath.Dir(parts), parts)
		if err != nil {
			return err
		}
		b.Stored = &st
	}
	b.Claim = updates.SignClaim(private, b)
	if err := b.Check(); err != nil {
		return err
	}
	if err := updates.Verify(public, b); err != nil {
		return err
	}
	if err := updates.VerifyClaim(public, b); err != nil {
		return err
	}
	if st, ok := b.AsStored(); ok {
		if err := updates.Verify(public, st); err != nil {
			return err
		}
		if err := updates.VerifyClaim(public, st); err != nil {
			return err
		}
	}
	return writeJSON(out, b)
}

// sameApp says whether two zips hold the same files, by name, size, mode
// and the SHA-256 of what each holds, so the stored zip signed beside a
// zip is the app check passed, byte for byte.
func sameApp(a, b string) error {
	za, err := zip.OpenReader(a)
	if err != nil {
		return err
	}
	defer za.Close()
	zb, err := zip.OpenReader(b)
	if err != nil {
		return err
	}
	defer zb.Close()
	if len(za.File) != len(zb.File) {
		return errors.New("the stored zip holds other files than the zip")
	}
	for i, f := range za.File {
		g := zb.File[i]
		if f.Name != g.Name || f.UncompressedSize64 != g.UncompressedSize64 || f.Mode() != g.Mode() {
			return fmt.Errorf("the stored zip holds %s otherwise than the zip", f.Name)
		}
		fs, err := fileSum(f)
		if err != nil {
			return err
		}
		gs, err := fileSum(g)
		if err != nil {
			return err
		}
		if !bytes.Equal(fs, gs) {
			return fmt.Errorf("the stored zip holds other bytes in %s than the zip", f.Name)
		}
	}
	return nil
}

// fileSum is the SHA-256 of what a file of a zip holds.
func fileSum(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// signStored signs the stored zip pack left in packed as the build b is,
// and checks that the parts and the zip packed beside it make that zip.
func signStored(private ed25519.PrivateKey, b updates.Build, packed, parts string) (updates.Stored, error) {
	f, err := os.Open(filepath.Join(packed, storedZip))
	if err != nil {
		return updates.Stored{}, err
	}
	digest, size, err := updates.Digest(f)
	f.Close()
	if err != nil {
		return updates.Stored{}, err
	}
	sum := hex.EncodeToString(digest)
	data, err := os.ReadFile(parts)
	if err != nil {
		return updates.Stored{}, err
	}
	if _, err := updates.ParseParts(data, size, sum); err != nil {
		return updates.Stored{}, err
	}
	whole, err := os.ReadFile(filepath.Join(packed, packedWhole))
	if err != nil {
		return updates.Stored{}, err
	}
	if err := unpacksTo(whole, digest); err != nil {
		return updates.Stored{}, err
	}
	b.Size, b.SHA256 = size, sum
	return updates.Stored{
		Size: size, SHA256: sum, Signature: updates.Sign(private, digest),
		Claim: updates.SignClaim(private, b), PackedSize: int64(len(whole)),
	}, nil
}

func publishCommand(args []string, gh gitHub) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the signed zip")
	as := fs.String("as", "", "the name it is published under")
	entry := fs.String("entry", "", "its entry for the channel list")
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	commit := fs.String("commit", "", "the whole commit it was built from")
	packed := fs.String("packed", "", "the folder pack wrote, whose parts and pieces go up too")
	_ = fs.Parse(args)
	return publish(gh, *zipPath, *as, *packed, *entry, *channel, *commit)
}

func notPublishedCommand(args []string, gh gitHub) error {
	fs := flag.NewFlagSet("not-published", flag.ExitOnError)
	commit := fs.String("commit", "", "the whole commit that was built")
	run := fs.String("run", "", "the address of the run, which the mark links to")
	refusal := fs.String("refusal", "", "where check or sign kept why they refused, if they did")
	_ = fs.Parse(args)
	text, _ := os.ReadFile(*refusal)
	return notPublished(gh, *commit, text, *run)
}

// channelsCommand writes the list with the public half in the checkout of
// main it runs in. Until the key is made there is no list to write, since
// a list without it cannot be checked.
func channelsCommand(gh gitHub) error {
	text, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return err
	}
	public, err := updates.PublicKey(string(text))
	if err != nil {
		return err
	}
	if public == nil {
		fmt.Println("::notice::No update key yet, so there is no list to write.")
		return nil
	}
	return writeChannels(gh, gitTrees{gh}, public, time.Now(), os.Stdout)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
