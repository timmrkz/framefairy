package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"framefairy/engine/fileio"

	"github.com/spf13/afero"
)

// A copy of what a person made of a video is kept outside its work
// folder, so a video added again after its folder was deleted, by Remove
// or in Finder, comes back with its transcript, how far transcription
// got, its clips, which of them were rendered and every word fixed in
// their captions. The copy is kept by a fingerprint of the video's
// content, EpisodeKey, so it is found again under any name in any
// folder. See docs/decisions/2026-11-25-a-removed-videos-work-comes-back.md.
//
// The work folder stays the truth while it is there. The copy is written
// only by Keep, from the work folder, and read only by PutBack, into a
// work folder that has no transcript and no clip set of its own. Nothing
// is ever deleted from it, so a folder that is gone, or half written
// back, never takes the copy with it.

// KeptDir is where the copies are kept, ~/.framefairy/kept, beside the
// models and the training records. Empty when there is no home folder.
func KeptDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".framefairy", "kept")
}

// keptPlace is where a machine keeps the copies, and the key of each
// video it worked out, by the video's size and time. The keys are kept
// too, in keysName beside the copies, so a video is read for its key once
// while it stays as it is, and not again at every start: that is two
// megabytes of each video in the library, read from a drive that may
// have to wake up for it.
type keptPlace struct {
	mu     sync.Mutex
	dir    string
	keys   map[string]keyedVideo
	loaded bool
}

// keysName is the file beside the copies that holds the key of every
// video worked out so far, by its path.
const keysName = "keys.json"

// keyedVideo is the key of a video and the size and time it was worked
// out for.
type keyedVideo struct {
	Size int64     `json:"size"`
	Mod  time.Time `json:"modified"`
	Key  string    `json:"key"`
}

// KeepIn names the folder the copies are kept in. UseSystem names
// KeptDir, a test a folder in memory. A machine with none keeps nothing.
func (m *Machine) KeepIn(dir string) {
	m.kept.mu.Lock()
	defer m.kept.mu.Unlock()
	m.kept.dir = dir
	m.kept.keys = nil
	m.kept.loaded = false
}

// keptFor is the folder of a video's copy, and false when it has none:
// the machine keeps no copies, or the video cannot be read for its key.
// It takes m.kept.mu only to look the key up and to note it, so a video
// read for its key holds up no Keep or PutBack of another video.
func (m *Machine) keptFor(source string) (string, bool) {
	info, err := m.Files.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	m.kept.mu.Lock()
	dir := m.kept.dir
	known, ok := m.knownKey(source)
	m.kept.mu.Unlock()
	if dir == "" {
		return "", false
	}
	if !ok || known.Size != info.Size() || !known.Mod.Equal(info.ModTime()) {
		known = keyedVideo{Size: info.Size(), Mod: info.ModTime(), Key: EpisodeKey(m.Files, source)}
		// The key becomes a folder name, and a mistake there would reach
		// every copy, so only a key of the one shape EpisodeKey makes is
		// taken: an empty one, from a file that could not be opened, is
		// none, and is not noted.
		if !validKey(known.Key) {
			return "", false
		}
		m.kept.mu.Lock()
		if m.kept.dir == dir {
			m.noteKey(source, known)
		}
		m.kept.mu.Unlock()
	}
	if !validKey(known.Key) {
		return "", false
	}
	child, err := fileio.SafeChildIn(m.Files, dir, known.Key)
	return child, err == nil
}

// knownKey is the key noted for a video, read from keysName the first
// time. A file that does not read is taken as no keys at all, so each
// video is read for its key again. The caller holds m.kept.mu.
func (m *Machine) knownKey(source string) (keyedVideo, bool) {
	if !m.kept.loaded && m.kept.dir != "" {
		m.kept.loaded = true
		if body, err := afero.ReadFile(m.Files, filepath.Join(m.kept.dir, keysName)); err == nil {
			_ = json.Unmarshal(body, &m.kept.keys)
		}
	}
	known, ok := m.kept.keys[source]
	return known, ok
}

// noteKey notes the key of a video and writes the keys to keysName. One
// that cannot be written costs only a read of the video at the next
// start. The caller holds m.kept.mu.
func (m *Machine) noteKey(source string, known keyedVideo) {
	if m.kept.keys == nil {
		m.kept.keys = map[string]keyedVideo{}
	}
	m.kept.keys[source] = known
	body, err := json.Marshal(m.kept.keys)
	if err != nil || m.Files.MkdirAll(m.kept.dir, 0o755) != nil {
		return
	}
	_ = fileio.WriteAtomic(m.Files, filepath.Join(m.kept.dir, keysName), body)
}

// validKey says whether key is what EpisodeKey makes: sixteen lowercase
// hex digits.
func validKey(key string) bool {
	if len(key) != 16 {
		return false
	}
	for _, c := range key {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// keptNames are the files of dir that the copy holds, in the order they
// are written: the transcript's loudness before its words, the way
// writeTranscript writes them, so a reader never finds words without
// their loudness, and the clip sets between. Only regular files, never
// a link, and never a file half written, whose name starts with a dot.
func keptNames(fsys afero.Fs, dir string) []string {
	entries, err := afero.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	words := TranscriptName(nil)
	frames := framesPath(words)
	var clips []string
	var hasWords, hasFrames bool
	for _, entry := range entries {
		name := entry.Name()
		info, err := fileio.LstatIn(fsys, filepath.Join(dir, name))
		if err != nil || !info.Mode().IsRegular() || strings.HasPrefix(name, ".") {
			continue
		}
		switch {
		case name == words:
			hasWords = true
		case name == frames:
			hasFrames = true
		default:
			if ok, _ := filepath.Match("clips*.json", name); ok {
				clips = append(clips, name)
			}
		}
	}
	slices.Sort(clips)
	var names []string
	if hasFrames {
		names = append(names, frames)
	}
	names = append(names, clips...)
	if hasWords {
		names = append(names, words)
	}
	return names
}

// Keep brings the copy of a video's work up to date with its work
// folder: each of the transcript and the clip sets that changed since it
// was last copied is copied again. A file is taken as unchanged while its
// size and time are those of its copy, so a Keep with nothing new reads
// nothing but the folder. A work folder with nothing to keep, gone or
// not written yet, leaves the copy as it is.
func (m *Machine) Keep(source string) error {
	dir, ok := m.keptFor(source)
	if !ok {
		return nil
	}
	m.kept.mu.Lock()
	defer m.kept.mu.Unlock()
	logs := LogsDir(source)
	names := keptNames(m.Files, logs)
	if len(names) == 0 {
		return nil
	}
	// The words are read before their loudness, which only ever grows, so
	// the copy never has words that go further than its loudness, however
	// transcription writes in between.
	type read struct {
		name string
		body []byte
		mod  time.Time
	}
	var reads []read
	for _, name := range slices.Backward(names) {
		from := filepath.Join(logs, name)
		info, err := m.Files.Stat(from)
		if err != nil {
			continue
		}
		if have, err := m.Files.Stat(filepath.Join(dir, name)); err == nil &&
			have.Size() == info.Size() && have.ModTime().Equal(info.ModTime()) {
			continue
		}
		body, err := afero.ReadFile(m.Files, from)
		if err != nil {
			continue
		}
		reads = append(reads, read{name: name, body: body, mod: info.ModTime()})
	}
	if len(reads) == 0 {
		return nil
	}
	if err := m.Files.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, r := range slices.Backward(reads) {
		to := filepath.Join(dir, r.name)
		if err := fileio.WriteAtomic(m.Files, to, r.body); err != nil {
			return err
		}
		// The copy wears the time of what it copies, which is how the
		// next Keep tells it is still the same.
		if err := m.Files.Chtimes(to, r.mod, r.mod); err != nil {
			return err
		}
	}
	return nil
}

// PutBack puts the kept copy of a video's work into its work folder, and
// says whether it did. It does only into a work folder with no
// transcript and no clip set of its own, so a video added again with its
// folder kept keeps what it has. Each file goes back through the same
// checks a file from outside gets: a clip set is read as a plan before
// it lands, and the transcript's loudness as loudness. The transcript is
// fitted to the file it comes back to, its name, size and time, since
// the key has shown it is the same video: without that, a video renamed
// or downloaded again would be transcribed again from the start. The
// copy stays where it is, so the video can come back again.
func (m *Machine) PutBack(source string) (bool, error) {
	dir, ok := m.keptFor(source)
	if !ok {
		return false, nil
	}
	m.kept.mu.Lock()
	defer m.kept.mu.Unlock()
	names := keptNames(m.Files, dir)
	logs := LogsDir(source)
	if len(names) == 0 || len(keptNames(m.Files, logs)) > 0 {
		return false, nil
	}
	stamp, err := stampOf(m.Files, source)
	if err != nil {
		return false, err
	}
	if err := MakeDir(m.Files, logs); err != nil {
		return false, err
	}
	words := TranscriptName(nil)
	put, framesBack := false, false
	for _, name := range names {
		body, err := afero.ReadFile(m.Files, filepath.Join(dir, name))
		if err != nil {
			continue
		}
		to := filepath.Join(logs, name)
		switch {
		case name == framesPath(words):
			if _, ok := readLoudness(body); !ok {
				continue
			}
			if err := fileio.WriteAtomic(m.Files, to, body); err != nil {
				return put, err
			}
			framesBack = true
		case name == words:
			var file transcriptFile
			if !framesBack || decodeTranscript(body, &file) != nil {
				continue
			}
			file.Source = stamp
			fitted, err := encodeTranscript(file)
			if err != nil {
				continue
			}
			if err := fileio.WriteAtomic(m.Files, to, fitted); err != nil {
				return put, err
			}
		default:
			// A clip set goes back byte for byte, read as a plan first.
			if err := writePlanFile(m.Files, to, body); err != nil {
				continue
			}
		}
		put = true
	}
	return put, nil
}
