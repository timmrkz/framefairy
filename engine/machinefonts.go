package engine

// The fonts installed on the machine are offered beside the three the
// program carries.
//
// A face found on the machine is treated exactly like a bundled one: its
// file is read by the engine, measured by the same code, served to the
// interface for the word being typed, and copied into the fonts folder
// beside a caption file before libass draws it. So the video preview, the
// thumbnails and the short are drawn from one file, as they are for a
// bundled face.
//
// The copy is a face of its own under a name nobody else has. libass on
// the Mac also sees every font installed there, and when a caption asks
// for a name that is a family, it picks the face of that family closest
// to the bold the caption asks for, which for Arial at bold is Arial Bold
// and not the Arial that was measured. For a face with PostScript outlines
// it matches no full name at all, only the PostScript name. A copy whose
// only names are its own is the one face that name can find. One face of
// a collection, a .ttc that holds several weights in one file, is copied
// out on its own, so the fonts folder holds the face that was chosen and
// not its whole family.
//
// Font files are bytes from outside, so everything here reads them with
// the same bounds checks as metrics.go, and FuzzMachineFace feeds it
// whatever it likes.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/spf13/afero"
)

// machineFace is one face found in the font folders of the machine.
type machineFace struct {
	font CaptionFont
	// path is the font file it is in, and offset where its table directory
	// starts there, which is past zero for a face of a collection.
	path   string
	offset int64
	// alias is the only name the copy beside a caption file answers to,
	// see the top of this file. It changes when the file changes, so a
	// copy of an older version is never taken for the new one.
	alias string
}

// fontShelf is what the machine has, read from its font folders. It holds
// nothing until a program says where to look, see UseMachineFonts, so a
// test sees only the bundled faces unless it puts some on the shelf.
type fontShelf struct {
	// reading is held while the folders are read, so a second caller
	// waits for the one reading rather than reading them all again.
	reading sync.Mutex
	mu      sync.Mutex
	fsys    afero.Fs
	dirs    []string
	faces   []machineFace
	byName  map[string]int
	scanned time.Time
}

var shelf = &fontShelf{}

// UseMachineFonts offers the fonts installed on this machine beside the
// bundled ones. The programs call it as they start. The folders are read
// the first time a face is looked for, and again by RefreshFonts.
func UseMachineFonts() { UseFontFolders(disk, FontFolders()) }

// UseFontFolders puts the faces in dirs of fsys on the shelf, in place of
// whatever was there. A test that calls it changes the faces every test of
// its package sees, so it does not run beside the tests that draw captions.
func UseFontFolders(fsys afero.Fs, dirs []string) {
	shelf.mu.Lock()
	shelf.fsys, shelf.dirs = fsys, dirs
	shelf.faces, shelf.byName, shelf.scanned = nil, nil, time.Time{}
	shelf.mu.Unlock()
	forgetMetrics()
}

// FontFolders are the folders a system keeps its fonts in, the user's own
// first, because that is the one the system takes when two hold a face of
// the same name.
func FontFolders() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	add := func(dir string) {
		if dir != "" && filepath.IsAbs(dir) {
			dirs = append(dirs, dir)
		}
	}
	switch runtime.GOOS {
	case "darwin":
		if home != "" {
			add(filepath.Join(home, "Library", "Fonts"))
		}
		add("/Library/Fonts")
		add("/System/Library/Fonts")
		// The fonts Font Book downloads when one is asked for, which the
		// system keeps among its assets, one folder for each version of
		// the catalogue.
		assets, _ := filepath.Glob("/System/Library/AssetsV2/com_apple_MobileAsset_Font*")
		for _, dir := range assets {
			add(dir)
		}
	case "windows":
		add(filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"))
		add(filepath.Join(os.Getenv("WINDIR"), "Fonts"))
	default:
		if home != "" {
			add(filepath.Join(home, ".local", "share", "fonts"))
			add(filepath.Join(home, ".fonts"))
		}
		add("/usr/local/share/fonts")
		add("/usr/share/fonts")
	}
	return dirs
}

// RefreshFonts reads the font folders again, so a font installed while the
// app runs is offered the next time the list is opened. It reads them at
// most once a second, however often it is asked.
func RefreshFonts() {
	shelf.mu.Lock()
	fresh := time.Since(shelf.scanned) < time.Second
	shelf.mu.Unlock()
	if !fresh {
		shelf.scan()
	}
}

// scan reads every font folder and puts what it found on the shelf.
func (s *fontShelf) scan() {
	s.reading.Lock()
	defer s.reading.Unlock()
	s.mu.Lock()
	fsys, dirs := s.fsys, s.dirs
	s.mu.Unlock()
	var faces []machineFace
	if fsys != nil {
		faces = findFaces(fsys, dirs)
	}
	byName := make(map[string]int, len(faces))
	for i, face := range faces {
		byName[face.font.Name] = i
	}
	s.mu.Lock()
	s.faces, s.byName, s.scanned = faces, byName, time.Now()
	s.mu.Unlock()
	forgetMetrics()
}

// all is every face on the shelf, read the first time it is asked for.
func (s *fontShelf) all() []machineFace {
	s.mu.Lock()
	ready := !s.scanned.IsZero()
	s.mu.Unlock()
	if !ready {
		s.reading.Lock()
		s.mu.Lock()
		ready = !s.scanned.IsZero()
		s.mu.Unlock()
		s.reading.Unlock()
		if !ready {
			s.scan()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.faces
}

// find is the face of that name on the shelf.
func (s *fontShelf) find(name string) (machineFace, bool) {
	s.all()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i, ok := s.byName[name]; ok {
		return s.faces[i], true
	}
	return machineFace{}, false
}

// Limits on what is read, so a folder or a file made to be endless costs a
// moment and not the app.
const (
	fontFolderDepth = 6
	fontFilesMost   = 10000
	facesInFileMost = 64
	tablesMost      = 200
	headTableSize   = 54
	nameTableMost   = 1 << 20
	cmapTableMost   = 8 << 20
	fontFileMost    = 512 << 20
)

// findFaces reads the faces in the font files under dirs. A name found
// twice is the first one's, in the order of dirs, and a name a bundled
// face has stays the bundled face's.
func findFaces(fsys afero.Fs, dirs []string) []machineFace {
	var faces []machineFace
	seen := map[string]bool{}
	for _, font := range captionFonts {
		seen[strings.ToLower(font.Name)] = true
	}
	files := 0
	for _, dir := range dirs {
		root := filepath.Clean(dir)
		_ = afero.Walk(fsys, root, func(path string, info fs.FileInfo, err error) error {
			if err != nil || files >= fontFilesMost {
				return nil
			}
			if info.IsDir() {
				if path != root && strings.Count(strings.TrimPrefix(path, root), string(filepath.Separator)) > fontFolderDepth {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ttf", ".otf", ".ttc", ".otc":
			default:
				return nil
			}
			if !info.Mode().IsRegular() || info.Size() > fontFileMost {
				return nil
			}
			files++
			file, err := fsys.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()
			for _, face := range readSafely(file, info.Size()) {
				key := strings.ToLower(face.name)
				if seen[key] {
					continue
				}
				seen[key] = true
				sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d\x00%d\x00%d\x00%s",
					path, face.offset, info.Size(), info.ModTime().UnixNano(), face.name))
				id := hex.EncodeToString(sum[:6])
				faces = append(faces, machineFace{
					font: CaptionFont{Name: face.name, Family: face.family, Style: face.style,
						PostScript: face.postscript, Weight: face.weight, Width: face.width,
						Italic: face.italic, File: "FrameFairy-" + id + face.ext, Machine: true},
					path: path, offset: face.offset, alias: "Frame Fairy " + id,
				})
			}
			return nil
		})
	}
	slices.SortStableFunc(faces, func(a, b machineFace) int {
		return strings.Compare(strings.ToLower(a.font.Name), strings.ToLower(b.font.Name))
	})
	return faces
}

// readSafely reads the faces of a font file, and none of one whose reading
// panics. Every font on the machine is read as the app starts, on a
// goroutine nothing recovers, so one font the reader trips on would end
// the app at every start, before Report a Problem could be reached, issue
// #604. A font left out is a font the Font list does not offer, the way a
// file that cannot be read is.
func readSafely(r io.ReaderAt, size int64) (faces []sfntFace) {
	defer func() {
		if recover() != nil {
			faces = nil
		}
	}()
	return readSfntFaces(r, size)
}

// sfntFace is what one face of a font file says about itself.
type sfntFace struct {
	name       string
	family     string
	style      string
	postscript string
	weight     int
	width      int
	italic     bool
	ext        string
	offset     int64
}

// readSfntFaces reads the faces of a font file that captions can be written
// in: a face with outlines, a name a person can read and a style line can
// hold, and the letters and digits of a caption.
func readSfntFaces(r io.ReaderAt, size int64) []sfntFace {
	head, ok := readAt(r, size, 0, 12)
	if !ok {
		return nil
	}
	offsets := []int64{0}
	if string(head[:4]) == "ttcf" {
		count := binary.BigEndian.Uint32(head[8:])
		if count == 0 || count > facesInFileMost {
			return nil
		}
		list, ok := readAt(r, size, 12, int64(count)*4)
		if !ok {
			return nil
		}
		offsets = offsets[:0]
		for i := range int(count) {
			offsets = append(offsets, int64(binary.BigEndian.Uint32(list[4*i:])))
		}
	}
	var faces []sfntFace
	for _, offset := range offsets {
		if face, ok := readSfntFace(r, size, offset); ok {
			faces = append(faces, face)
		}
	}
	return faces
}

// tableEntry is one line of a face's table directory.
type tableEntry struct {
	tag            string
	offset, length int64
}

// tableDirectory reads the table directory of the face that starts at
// offset, and tells whether its outlines are PostScript ones.
func tableDirectory(r io.ReaderAt, size, offset int64) (map[string]tableEntry, bool, bool) {
	head, ok := readAt(r, size, offset, 12)
	if !ok {
		return nil, false, false
	}
	version := string(head[:4])
	switch version {
	case "\x00\x01\x00\x00", "true", "OTTO":
	default:
		return nil, false, false
	}
	count := int64(binary.BigEndian.Uint16(head[4:]))
	if count == 0 || count > tablesMost {
		return nil, false, false
	}
	list, ok := readAt(r, size, offset+12, count*16)
	if !ok {
		return nil, false, false
	}
	tables := make(map[string]tableEntry, count)
	for i := range count {
		at := list[16*i:]
		entry := tableEntry{tag: string(at[:4]),
			offset: int64(binary.BigEndian.Uint32(at[8:])),
			length: int64(binary.BigEndian.Uint32(at[12:]))}
		if entry.offset+entry.length > size {
			return nil, false, false
		}
		tables[entry.tag] = entry
	}
	return tables, version == "OTTO", true
}

func readSfntFace(r io.ReaderAt, size, offset int64) (sfntFace, bool) {
	tables, postscript, ok := tableDirectory(r, size, offset)
	if !ok {
		return sfntFace{}, false
	}
	for _, tag := range []string{"head", "hhea", "hmtx", "cmap", "name"} {
		if _, ok := tables[tag]; !ok {
			return sfntFace{}, false
		}
	}
	// Every head table is 54 bytes. A shorter one is no face, and the copy
	// of it a caption needs could not be made.
	if tables["head"].length < headTableSize {
		return sfntFace{}, false
	}
	_, glyf := tables["glyf"]
	_, cff := tables["CFF "]
	_, cff2 := tables["CFF2"]
	if !glyf && !cff && !cff2 {
		return sfntFace{}, false
	}
	read := func(tag string, most int64) []byte {
		entry := tables[tag]
		if entry.length > most {
			return nil
		}
		data, _ := readAt(r, size, entry.offset, entry.length)
		return data
	}
	names := read("name", nameTableMost)
	name := faceName(names, 4)
	if !captionFontName(name) {
		return sfntFace{}, false
	}
	cmap := bestCmap(read("cmap", cmapTableMost))
	if cmap == nil {
		return sfntFace{}, false
	}
	letters := &faceMetrics{cmap: cmap}
	for _, r := range "AZaz09" {
		if letters.glyph(r) == 0 {
			return sfntFace{}, false
		}
	}
	ext := ".ttf"
	if postscript {
		ext = ".otf"
	}
	os2, headTable := read("OS/2", 1024), read("head", 1024)
	family, style := faceFamily(names, name)
	psName := faceName(names, 6)
	if !captionFontName(psName) {
		psName = ""
	}
	return sfntFace{name: name, family: family, style: style, postscript: psName,
		weight: faceWeight(os2, headTable), width: faceWidth(os2),
		italic: faceItalic(os2, headTable), ext: ext, offset: offset}, true
}

// readAt reads n bytes at offset, when the file has them.
func readAt(r io.ReaderAt, size, offset, n int64) ([]byte, bool) {
	if offset < 0 || n <= 0 || offset+n > size {
		return nil, false
	}
	data := make([]byte, n)
	if _, err := r.ReadAt(data, offset); err != nil && !(errors.Is(err, io.EOF) && offset+n == size) {
		return nil, false
	}
	return data, true
}

// faceName is one of the names a face has by its number in the name
// table, 4 for the full name Font Book shows: an English one from the
// Windows table where there is one, then any from it, then the Mac's own.
func faceName(name []byte, want uint16) string {
	count, ok1 := u16(name, 2)
	strings0, ok2 := u16(name, 4)
	if !ok1 || !ok2 {
		return ""
	}
	best, bestRank := "", -1
	for i := range int(count) {
		at := 6 + 12*i
		platform, ok1 := u16(name, at)
		encoding, ok2 := u16(name, at+2)
		language, ok3 := u16(name, at+4)
		id, ok4 := u16(name, at+6)
		length, ok5 := u16(name, at+8)
		offset, ok6 := u16(name, at+10)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || id != want {
			continue
		}
		start := int(strings0) + int(offset)
		if start+int(length) > len(name) {
			continue
		}
		raw := name[start : start+int(length)]
		rank, text := -1, ""
		switch {
		case platform == 3 && (encoding == 1 || encoding == 10):
			text = utf16BE(raw)
			rank = 2
			if language == 0x0409 {
				rank = 4
			} else if language&0xFF == 0x09 {
				rank = 3
			}
		case platform == 0:
			text, rank = utf16BE(raw), 1
		case platform == 1 && encoding == 0 && language == 0:
			text, rank = macRoman(raw), 0
		}
		if text != "" && rank > bestRank {
			best, bestRank = text, rank
		}
	}
	return strings.TrimSpace(best)
}

func utf16BE(raw []byte) string {
	if len(raw)%2 != 0 {
		return ""
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.BigEndian.Uint16(raw[2*i:])
	}
	return string(utf16.Decode(units))
}

// macRoman reads the Mac's own names, of which only plain ASCII is taken:
// a name with anything else in it has a Windows one as well.
func macRoman(raw []byte) string {
	for _, b := range raw {
		if b >= 0x80 {
			return ""
		}
	}
	return string(raw)
}

// captionFontName tells whether a face can be offered under its name. A
// name that starts with a dot is one of the system's own, which macOS
// keeps for its interface. A style line holds no comma, colon or brace,
// see cleanField, and a name that cleaning would change would ask libass
// for another one.
func captionFontName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || len([]rune(name)) > 64 {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return cleanField(name) == name
}

// faceFamily is the family a face belongs to and its style in it, the two
// lists the Mac's own apps offer a font in, Font and Typeface in Pages. The
// typographic names come first, so American Typewriter Condensed Bold is
// the Condensed Bold of American Typewriter rather than a family of its
// own, then the plain ones. A face with neither is a family of one.
func faceFamily(names []byte, full string) (family, style string) {
	family = faceName(names, 16)
	style = faceName(names, 17)
	if family == "" {
		family, style = faceName(names, 1), faceName(names, 2)
	}
	if !familyName(family) {
		return full, "Regular"
	}
	if !familyName(style) {
		style = strings.TrimSpace(strings.TrimPrefix(full, family))
		if !familyName(style) {
			style = "Regular"
		}
	}
	return family, style
}

// familyName tells whether a family or a style name can be shown: it is
// only ever shown, never written into a caption file, so it only has to
// be a short line a person can read.
func familyName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || len([]rune(name)) > 64 {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// faceWidth is how wide a face says it is, the width class of OS/2, from 1
// for the narrowest to 9 for the widest and 5 for a face of usual width.
func faceWidth(os2 []byte) int {
	class, ok := u16(os2, 6)
	if !ok || class < 1 || class > 9 {
		return 5
	}
	return int(class)
}

// faceItalic tells whether a face leans, by the italic or the oblique bit
// of OS/2 or the italic bit of head.
func faceItalic(os2, head []byte) bool {
	selection, _ := u16(os2, 62)
	style, _ := u16(head, 44)
	return selection&0x201 != 0 || style&0x2 != 0
}

// faceWeight is the weight libass gives a face, ass_face_get_weight in
// ass_font.c: the weight class of OS/2, read the old way when it is a
// single digit, and 400 or 700 by the bold flag when there is none.
func faceWeight(os2, head []byte) int {
	class, _ := u16(os2, 4)
	switch {
	case class == 0:
		selection, _ := u16(os2, 62)
		style, _ := u16(head, 44)
		if selection&0x20 != 0 || style&0x1 != 0 {
			return 700
		}
		return 400
	case class < 10:
		return []int{0, 100, 200, 300, 350, 400, 600, 700, 800, 900}[class]
	}
	return int(class)
}

// machineFontBytes is the face copied out of its file under its alias, a
// font file of one face.
func machineFontBytes(face machineFace) ([]byte, error) {
	shelf.mu.Lock()
	fsys := shelf.fsys
	shelf.mu.Unlock()
	if fsys == nil {
		return nil, fs.ErrNotExist
	}
	file, err := fsys.Open(face.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return copyFace(file, info.Size(), face.offset, aliasNames(face.alias))
}

// NamedFace is the single face of a font file under another name, the way
// a test or the bridge makes a font the machine has out of one of ours.
func NamedFace(data []byte, name string) ([]byte, error) {
	return copyFace(bytes.NewReader(data), int64(len(data)), 0, aliasNames(name))
}

// StyledFace is the single face of a font file as the style of a family,
// named the way Apple names its own faces, so a test or the bridge can make
// a family the machine has out of the faces of ours.
func StyledFace(data []byte, family, style string) ([]byte, error) {
	full := family + " " + style
	return copyFace(bytes.NewReader(data), int64(len(data)), 0, nameTable(map[uint16]string{
		1: family, 2: style, 4: full, 6: strings.ReplaceAll(full, " ", "-"), 16: family, 17: style}))
}

// copyFace writes the face at offset as a font file of its own, with the
// name table given, which for a copy libass draws from says its alias and
// nothing else. The tables keep their
// bytes, in a fresh directory with fresh checksums. A signature would not
// fit the copy, so it is left out.
func copyFace(r io.ReaderAt, size, offset int64, names []byte) ([]byte, error) {
	tables, postscript, ok := tableDirectory(r, size, offset)
	if !ok {
		return nil, errors.New("not a font file")
	}
	type table struct {
		tag  string
		data []byte
	}
	var out []table
	for tag, entry := range tables {
		if tag == "DSIG" || tag == "name" {
			continue
		}
		data, ok := readAt(r, size, entry.offset, entry.length)
		if !ok && entry.length > 0 {
			return nil, errors.New("a table runs past the end of the file")
		}
		if tag == "head" {
			if len(data) < 12 {
				return nil, errors.New("the head table is too short")
			}
			// Worked out again once the file is put together.
			binary.BigEndian.PutUint32(data[8:], 0)
		}
		out = append(out, table{tag, data})
	}
	out = append(out, table{"name", names})
	slices.SortFunc(out, func(a, b table) int { return strings.Compare(a.tag, b.tag) })

	count := len(out)
	power := 1
	for power*2 <= count {
		power *= 2
	}
	level := 0
	for 1<<(level+1) <= power {
		level++
	}
	file := make([]byte, 12+16*count)
	if postscript {
		copy(file, "OTTO")
	} else {
		binary.BigEndian.PutUint32(file, 0x00010000)
	}
	binary.BigEndian.PutUint16(file[4:], uint16(count))
	binary.BigEndian.PutUint16(file[6:], uint16(power*16))
	binary.BigEndian.PutUint16(file[8:], uint16(level))
	binary.BigEndian.PutUint16(file[10:], uint16(count*16-power*16))
	headAt := -1
	for i, t := range out {
		at := 12 + 16*i
		copy(file[at:], t.tag)
		binary.BigEndian.PutUint32(file[at+4:], tableSum(t.data))
		binary.BigEndian.PutUint32(file[at+8:], uint32(len(file)))
		binary.BigEndian.PutUint32(file[at+12:], uint32(len(t.data)))
		if t.tag == "head" {
			headAt = len(file)
		}
		file = append(file, t.data...)
		for len(file)%4 != 0 {
			file = append(file, 0)
		}
	}
	if headAt >= 0 {
		binary.BigEndian.PutUint32(file[headAt+8:], 0xB1B0AFBA-tableSum(file))
	}
	return file, nil
}

// tableSum is the checksum of a table, its bytes added up as 32 bit words.
func tableSum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		var word [4]byte
		copy(word[:], data[i:])
		sum += binary.BigEndian.Uint32(word[:])
	}
	return sum
}

// aliasNames is a name table in which the family, the full name and the
// PostScript name are all alias, in the Windows table libass reads.
func aliasNames(alias string) []byte {
	return nameTable(map[uint16]string{1: alias, 2: "Regular", 4: alias,
		6: strings.ReplaceAll(alias, " ", "-")})
}

// nameTable is a name table that holds the names given by their number,
// in the Windows table, in English.
func nameTable(names map[uint16]string) []byte {
	type record struct {
		id   uint16
		text string
	}
	var records []record
	for id, text := range names {
		records = append(records, record{id, text})
	}
	slices.SortFunc(records, func(a, b record) int { return int(a.id) - int(b.id) })
	var stringsData []byte
	table := make([]byte, 6+12*len(records))
	binary.BigEndian.PutUint16(table[2:], uint16(len(records)))
	binary.BigEndian.PutUint16(table[4:], uint16(len(table)))
	for i, rec := range records {
		var text []byte
		for _, unit := range utf16.Encode([]rune(rec.text)) {
			text = binary.BigEndian.AppendUint16(text, unit)
		}
		at := 6 + 12*i
		binary.BigEndian.PutUint16(table[at:], 3)
		binary.BigEndian.PutUint16(table[at+2:], 1)
		binary.BigEndian.PutUint16(table[at+4:], 0x0409)
		binary.BigEndian.PutUint16(table[at+6:], rec.id)
		binary.BigEndian.PutUint16(table[at+8:], uint16(len(text)))
		binary.BigEndian.PutUint16(table[at+10:], uint16(len(stringsData)))
		stringsData = append(stringsData, text...)
	}
	return append(table, stringsData...)
}
