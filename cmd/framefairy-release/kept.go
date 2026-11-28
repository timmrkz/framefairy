package main

// A library whose code did not change keeps the signed bytes it had in an
// earlier build. codesign puts Apple's time stamp and the time of signing
// into every signature, so a library signed again is another file,
// though its code is the same. An update downloads every file the app
// does not have by its SHA-256, so without this every update would
// download ffmpeg, the frames decoder, llama-server and the speech
// libraries again, about 33 MB in place of about 7.
//
// So signed-before, in a step that holds no secret and no token,
// downloads the same libraries of main's build and of the channel's, and
// harden, once it has signed each library afresh, keeps an earlier copy
// in its place when the two are the same signed code: every byte before
// the signature, and every blob of the signature but the CMS one, are the
// same, and the CMS blob carries the same certificates. The code
// directory says the code, the identifier, the entitlements and the
// requirements, so those bytes the same are the same code. It does not
// say the certificate, so a library signed before a certificate was
// renewed or revoked is never kept. Then codesign checks the kept file
// against the user's team like any other, and the app is sealed around
// it. What the release holds is untrusted: a copy that is not the same in
// every one of these ways is not kept, and the library goes out as signed
// now.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"framefairy/updates"
)

// keptPerFile is how many earlier copies of one library signed-before
// downloads at most: main's and the channel's.
const keptPerFile = 2

// cmsSlot is the slot of a code signature that holds the CMS signature,
// and cmsMagic the magic of its blob.
const (
	cmsSlot  = 0x10000
	cmsMagic = 0xfade0b01
)

// signedBeforeCommand downloads earlier copies of the libraries and the
// programs beside the app, for harden -kept.
//
//	signed-before -channel C -out D
//
// It writes each copy to D, under its name in the build's zip and then
// its SHA-256. What cannot be had is said and left out, never an error:
// a library without an earlier copy is signed as it is.
func signedBeforeCommand(args []string) error {
	fs := flag.NewFlagSet("signed-before", flag.ExitOnError)
	channel := fs.String("channel", "", "the channel the build is of")
	from := fs.String("from", updates.EntriesURL, "the folder of the release the builds are read from")
	out := fs.String("out", "", "the folder the copies go in")
	_ = fs.Parse(args)
	if *out == "" {
		return errors.New("signed-before needs -out")
	}
	r := releaseFiles{from: *from, client: &http.Client{}}
	return signedBefore(r, []string{"main", *channel}, *out, os.Stdout)
}

// releaseFiles reads files of the release dev the way the app does, from
// their public addresses, with no token.
type releaseFiles struct {
	from   string
	client *http.Client
}

// get is the file of this name, refused when it is larger than limit.
func (r releaseFiles) get(name string, limit int64) ([]byte, error) {
	resp, err := r.client.Get(r.from + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", name, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, limit)
	}
	return data, nil
}

// parts are the parts of a channel's build as the release holds them,
// read against the size and checksum its entry names.
func (r releaseFiles) parts(channel string) (updates.Parts, error) {
	data, err := r.get("channel-"+channel+".json", updates.MaxList)
	if err != nil {
		return updates.Parts{}, err
	}
	var b updates.Build
	if err := json.Unmarshal(data, &b); err != nil {
		return updates.Parts{}, fmt.Errorf("its entry is not JSON: %w", err)
	}
	u, err := url.Parse(b.URL)
	if err != nil {
		return updates.Parts{}, err
	}
	zip := path.Base(u.Path)
	if !strings.HasPrefix(zip, "app-") || !strings.HasSuffix(zip, ".zip") {
		return updates.Parts{}, fmt.Errorf("its entry names %q, no build", zip)
	}
	data, err = r.get(updates.PartsName(zip), updates.MaxList)
	if err != nil {
		return updates.Parts{}, err
	}
	return updates.ParseParts(data, b.Size, b.SHA256)
}

// signedBefore writes up to keptPerFile earlier copies of every library
// and program beside the app's own that the builds of channels hold, in
// their order, to out.
func signedBefore(r releaseFiles, channels []string, out string, log io.Writer) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	taken := map[string]int{}
	seen := map[string]bool{}
	for _, channel := range slices.Compact(slices.Clone(channels)) {
		if channel == "" {
			continue
		}
		p, err := r.parts(channel)
		if err != nil {
			fmt.Fprintf(log, "no earlier copies from %s: %v\n", channel, err)
			continue
		}
		for _, part := range p.Parts {
			if part.Bytes != nil || !keepable(part.Name) || seen[part.SHA256] || taken[part.Name] >= keptPerFile {
				continue
			}
			seen[part.SHA256] = true
			packed, err := r.get(updates.PieceName(part.SHA256), part.Packed)
			var data []byte
			if err == nil {
				data, err = updates.UnpackPiece(packed, part)
			}
			if err != nil {
				fmt.Fprintf(log, "no earlier copy of %s from %.12s: %v\n", part.Name, part.SHA256, err)
				continue
			}
			dir := filepath.Join(out, filepath.FromSlash(part.Name))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, part.SHA256), data, 0o644); err != nil {
				return err
			}
			taken[part.Name]++
		}
	}
	return nil
}

// keepable says whether a file of a build's zip is one harden may keep an
// earlier copy of: a library in Contents/Frameworks, or a program in
// Contents/MacOS but the app's own, whose signature binds the
// Info.plist, and with it the version, so it changes every build.
func keepable(name string) bool {
	inside, ok := updates.InApp(name)
	if !ok || !insideApp(name) {
		return false
	}
	dir, file := path.Split(inside)
	switch dir {
	case "Contents/Frameworks/":
		return strings.HasSuffix(file, ".dylib")
	case "Contents/MacOS/":
		return file != appProgram
	}
	return false
}

// keepSigned puts an earlier copy from kept in place of each of inner,
// signed a moment ago, that is the same signed code, see sameSignedCode,
// and says which it kept. root is the folder the .app lies in, so a
// file's name below it is its name in the build's zip.
func keepSigned(root, kept string, inner []string, log io.Writer) error {
	if kept == "" {
		return nil
	}
	for _, code := range inner {
		rel, err := filepath.Rel(root, code)
		if err != nil {
			return err
		}
		copies, err := os.ReadDir(filepath.Join(kept, rel))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		fresh, err := os.ReadFile(code)
		if err != nil {
			return err
		}
		why := "no earlier copy"
		for _, c := range copies {
			earlier, err := os.ReadFile(filepath.Join(kept, rel, c.Name()))
			if err != nil {
				return err
			}
			if err := sameSignedCode(fresh, earlier); err != nil {
				why = err.Error()
				continue
			}
			info, err := os.Stat(code)
			if err != nil {
				return err
			}
			if err := os.WriteFile(code, earlier, info.Mode().Perm()); err != nil {
				return err
			}
			fmt.Fprintf(log, "kept %s as signed in an earlier build, %.12s\n", filepath.ToSlash(rel), c.Name())
			why = ""
			break
		}
		if why != "" {
			fmt.Fprintf(log, "signed %s afresh: %s\n", filepath.ToSlash(rel), why)
		}
	}
	return nil
}

// sameSignedCode is nil when earlier is the same code as fresh, signed
// with the same certificates, and otherwise says how they differ. Thin or
// fat, every architecture is held to it.
func sameSignedCode(fresh, earlier []byte) error {
	a, err := archesOf(fresh)
	if err != nil || len(a) == 0 {
		return fmt.Errorf("the new copy is no program: %v", err)
	}
	b, err := archesOf(earlier)
	if err != nil || len(b) == 0 {
		return fmt.Errorf("the earlier copy is no program: %v", err)
	}
	if len(a) != len(b) {
		return errors.New("they have other architectures")
	}
	for i := range a {
		if err := sameSignedArch(a[i], b[i]); err != nil {
			return err
		}
	}
	return nil
}

func sameSignedArch(a, b arch) error {
	offA, slotsA, err := signatureSlots(a)
	if err != nil {
		return fmt.Errorf("the new copy's signature: %w", err)
	}
	offB, slotsB, err := signatureSlots(b)
	if err != nil {
		return fmt.Errorf("the earlier copy's signature: %w", err)
	}
	if offA != offB || !bytes.Equal(a.data[:offA], b.data[:offB]) {
		return errors.New("their code differs")
	}
	if len(slotsA) != len(slotsB) {
		return errors.New("their signatures hold other blobs")
	}
	var cmsA, cmsB []byte
	for i := range slotsA {
		if slotsA[i].kind != slotsB[i].kind {
			return errors.New("their signatures hold other blobs")
		}
		if slotsA[i].kind == cmsSlot {
			cmsA, cmsB = slotsA[i].blob, slotsB[i].blob
			continue
		}
		if !bytes.Equal(slotsA[i].blob, slotsB[i].blob) {
			return fmt.Errorf("their signatures differ in slot %#x", slotsA[i].kind)
		}
	}
	if cmsA == nil {
		return errors.New("it is not signed by a developer")
	}
	certsA, err := cmsCertificates(cmsA)
	if err != nil {
		return fmt.Errorf("the new copy's certificates: %w", err)
	}
	certsB, err := cmsCertificates(cmsB)
	if err != nil {
		return fmt.Errorf("the earlier copy's certificates: %w", err)
	}
	if len(certsA) == 0 || !slices.EqualFunc(certsA, certsB, bytes.Equal) {
		return errors.New("they are signed with other certificates")
	}
	return nil
}

// slotBlob is one blob of a code signature and the slot it is in.
type slotBlob struct {
	kind uint32
	blob []byte
}

// signatureSlots is where the code signature of a starts, and its blobs
// in the order of its index, read the way signatureBlob reads one.
func signatureSlots(a arch) (uint64, []slotBlob, error) {
	const codeSignature = 0x1d
	be := binary.BigEndian
	for _, l := range a.f.Loads {
		raw := l.Raw()
		if len(raw) < 16 || a.f.ByteOrder.Uint32(raw) != codeSignature {
			continue
		}
		off, size := uint64(a.f.ByteOrder.Uint32(raw[8:])), uint64(a.f.ByteOrder.Uint32(raw[12:]))
		if off+size > uint64(len(a.data)) || size < 12 {
			return 0, nil, errors.New("it reaches past the end of the file")
		}
		sig := a.data[off : off+size]
		if be.Uint32(sig) != 0xfade0cc0 {
			return 0, nil, errors.New("it is no blob of blobs")
		}
		count := uint64(be.Uint32(sig[8:]))
		if 12+8*count > size {
			return 0, nil, errors.New("its index reaches past its end")
		}
		var slots []slotBlob
		for i := range count {
			kind, at := be.Uint32(sig[12+8*i:]), uint64(be.Uint32(sig[16+8*i:]))
			if at+8 > size {
				return 0, nil, fmt.Errorf("its slot %#x is not where its index says", kind)
			}
			end := at + uint64(be.Uint32(sig[at+4:]))
			if end > size || end < at+8 {
				return 0, nil, fmt.Errorf("its slot %#x reaches past its end", kind)
			}
			if kind == cmsSlot && be.Uint32(sig[at:]) != cmsMagic {
				return 0, nil, errors.New("its CMS slot holds no CMS blob")
			}
			slots = append(slots, slotBlob{kind, sig[at:end]})
		}
		return off, slots, nil
	}
	return 0, nil, errors.New("it is not signed")
}

// cmsCertificates are the certificates a CMS blob of a code signature
// carries, each as its own bytes, in their order. The CMS is a
// ContentInfo whose content, [0], is a SignedData, whose certificates
// are its element tagged [0]. Apple writes it in BER, with lengths left
// open, which Go's encoding/asn1 does not read, so this reads only what
// it needs, by hand.
func cmsCertificates(blob []byte) ([][]byte, error) {
	if len(blob) < 8 {
		return nil, errors.New("the CMS blob is too short")
	}
	info, _, err := berNext(blob[8:])
	if err != nil {
		return nil, err
	}
	if info.tag != 0x30 {
		return nil, errors.New("the CMS is no sequence")
	}
	parts, err := berChildren(info)
	if err != nil {
		return nil, err
	}
	if len(parts) < 2 || parts[1].tag != 0xa0 {
		return nil, errors.New("the CMS holds no content")
	}
	inner, err := berChildren(parts[1])
	if err != nil || len(inner) != 1 || inner[0].tag != 0x30 {
		return nil, errors.New("the CMS content is no SignedData")
	}
	fields, err := berChildren(inner[0])
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		if f.tag != 0xa0 {
			continue
		}
		certs, err := berChildren(f)
		if err != nil {
			return nil, err
		}
		var out [][]byte
		for _, c := range certs {
			out = append(out, c.full)
		}
		return out, nil
	}
	return nil, errors.New("the CMS carries no certificates")
}

// berElement is one element of BER: its tag, as one byte, its content,
// and the whole of it as written.
type berElement struct {
	tag      byte
	open     bool
	content  []byte
	full     []byte
	children []berElement
}

// berMaxDepth bounds how deep berNext reads elements inside elements of
// open length, which is what a crafted blob would use to go deep.
const berMaxDepth = 32

// berNext reads the element data starts with and says what follows it.
// Tags of more than one byte are refused, since a CMS has none.
func berNext(data []byte) (berElement, []byte, error) {
	return berRead(data, 0)
}

func berRead(data []byte, depth int) (berElement, []byte, error) {
	if depth > berMaxDepth {
		return berElement{}, nil, errors.New("the CMS nests too deep")
	}
	if len(data) < 2 {
		return berElement{}, nil, errors.New("the CMS ends inside an element")
	}
	e := berElement{tag: data[0]}
	if e.tag&0x1f == 0x1f {
		return berElement{}, nil, errors.New("the CMS has a tag of more than one byte")
	}
	n := int(data[1])
	at := 2
	switch {
	case n == 0x80:
		// Open: the elements inside run until two zero bytes.
		if e.tag&0x20 == 0 {
			return berElement{}, nil, errors.New("the CMS has a simple element of open length")
		}
		e.open = true
		rest := data[at:]
		for {
			if len(rest) >= 2 && rest[0] == 0 && rest[1] == 0 {
				end := len(data) - len(rest)
				e.content = data[at:end]
				e.full = data[:end+2]
				return e, rest[2:], nil
			}
			child, after, err := berRead(rest, depth+1)
			if err != nil {
				return berElement{}, nil, err
			}
			e.children = append(e.children, child)
			rest = after
		}
	case n > 0x80:
		size := n & 0x7f
		if size > 4 || len(data) < at+size {
			return berElement{}, nil, errors.New("the CMS has a length it cannot hold")
		}
		n = 0
		for _, b := range data[at : at+size] {
			n = n<<8 | int(b)
		}
		at += size
	}
	if n < 0 || len(data)-at < n {
		return berElement{}, nil, errors.New("the CMS ends inside an element")
	}
	e.content = data[at : at+n]
	e.full = data[:at+n]
	return e, data[at+n:], nil
}

// berChildren are the elements inside a constructed one.
func berChildren(e berElement) ([]berElement, error) {
	if e.open {
		return e.children, nil
	}
	if e.tag&0x20 == 0 {
		return nil, errors.New("the CMS has a simple element where it needs more")
	}
	var out []berElement
	rest := e.content
	for len(rest) > 0 {
		child, after, err := berNext(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, child)
		rest = after
	}
	return out, nil
}
