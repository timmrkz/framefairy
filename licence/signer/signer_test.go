package signer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"framefairy/licence"
)

func testKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("framefairy test signer, never shipped"))
	return ed25519.NewKeyFromSeed(seed[:])
}

var day = time.Date(2026, time.October, 28, 9, 30, 0, 0, time.UTC)

func clock() time.Time { return day }

// counter is a random source that never repeats, the same in every run.
type counter struct {
	mu sync.Mutex
	n  uint64
}

func (c *counter) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range p {
		if i%32 == 0 {
			c.n++
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], c.n)
			sum := sha256.Sum256(b[:])
			copy(p[i:], sum[:])
		}
	}
	return len(p), nil
}

// script is a random source that gives these IDs in turn, then fails.
type script struct{ ids [][8]byte }

func (s *script) Read(p []byte) (int, error) {
	if len(s.ids) == 0 {
		return 0, errors.New("the script ran out")
	}
	n := copy(p, s.ids[0][:])
	s.ids = s.ids[1:]
	return n, nil
}

func open(t *testing.T, path string) *Record {
	t.Helper()
	r, err := OpenRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func newSigner(t *testing.T, number uint8, r *Record, opts ...Option) *Signer {
	t.Helper()
	opts = append([]Option{WithClock(clock), WithRandom(&counter{})}, opts...)
	s, err := New(number, testKey(), r, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func trust(s *Signer) licence.Trust {
	return licence.Trust{Signers: map[uint8]ed25519.PublicKey{s.Number(): s.Public()}}
}

func recordPath(t *testing.T) string { return filepath.Join(t.TempDir(), "record.jsonl") }

func TestPool(t *testing.T) {
	path := recordPath(t)
	s := newSigner(t, 3, open(t, path))
	keys, err := s.Pool(250, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 250 {
		t.Fatalf("%d keys", len(keys))
	}
	lines := s.record.Lines()
	if len(lines) != 250 {
		t.Fatalf("%d lines in the record", len(lines))
	}
	ids := map[licence.ID]bool{}
	for i, k := range keys {
		l, err := licence.Check(k, trust(s))
		if err != nil {
			t.Fatal(err)
		}
		if l.Signer != 3 || l.Edition != 0 || l.Name != "" || !l.Signed.Equal(time.Date(2026, time.October, 28, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("key %d reads %+v", i, l)
		}
		if ids[l.ID] {
			t.Fatalf("ID %s twice", l.ID)
		}
		ids[l.ID] = true
		want := Line{Day: "2026-10-28", Signer: 3, ID: l.ID.String(), Fingerprint: k.Fingerprint().String(), Batch: 1, Index: i + 1, Of: 250, Kind: KindPool}
		if lines[i] != want {
			t.Fatalf("line %d is %+v, want %+v", i, lines[i], want)
		}
	}
	// No key is in the file, only what says which keys there are.
	text, _ := os.ReadFile(path)
	for _, k := range keys {
		if bytes.Contains(text, []byte(k[len(licence.Prefix):len(licence.Prefix)+20])) {
			t.Fatal("the record holds a key")
		}
	}
}

func TestBatchesAreNumberedAndSurviveReopening(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	s := newSigner(t, 1, r)
	for range 3 {
		if _, err := s.Pool(10, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Partner("Bundle Hunt", 5, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Named("Lena Fischer", "review copy", 0); err != nil {
		t.Fatal(err)
	}
	before := r.Lines()
	r.Close()

	r = open(t, path)
	after := r.Lines()
	if len(after) != 36 || len(before) != 36 {
		t.Fatalf("%d lines before, %d after", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("line %d changed on reopening", i)
		}
	}
	if after[35].Batch != 5 || after[30].Batch != 4 || after[30].Index != 1 || after[34].Index != 5 {
		t.Fatalf("batches %+v", after[30:])
	}

	// A signer on the reopened record goes on from batch 6 and never draws
	// an ID it drew before, even from the same random source.
	s = newSigner(t, 1, r)
	keys, err := s.Pool(36, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Lines()[36].Batch; got != 6 {
		t.Fatalf("batch %d after reopening, want 6", got)
	}
	seen := map[string]bool{}
	for _, l := range r.Lines() {
		if seen[l.ID] {
			t.Fatalf("ID %s twice", l.ID)
		}
		seen[l.ID] = true
	}
	if len(keys) != 36 {
		t.Fatal(len(keys))
	}
}

func TestRepeatedIDsAreDrawnAgain(t *testing.T) {
	r := open(t, recordPath(t))
	a := [8]byte{1, 1, 1, 1, 1, 1, 1, 1}
	b := [8]byte{2, 2, 2, 2, 2, 2, 2, 2}
	c := [8]byte{3, 3, 3, 3, 3, 3, 3, 3}
	zero := [8]byte{}
	s := newSigner(t, 1, r, WithRandom(&script{ids: [][8]byte{a, zero, a, b}}))
	if _, err := s.Pool(2, 0); err != nil {
		t.Fatal(err)
	}
	s = newSigner(t, 1, r, WithRandom(&script{ids: [][8]byte{b, a, zero, c}}))
	keys, err := s.Pool(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	l, _ := licence.Check(keys[0], trust(s))
	if l.ID != licence.ID(c) {
		t.Fatalf("drew %s, want %s", l.ID, licence.ID(c))
	}
}

func TestBrokenRandomWritesNothing(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	a := [8]byte{9, 9, 9, 9, 9, 9, 9, 9}
	same := make([][8]byte, 1000)
	for i := range same {
		same[i] = a
	}
	s := newSigner(t, 1, r, WithRandom(&script{ids: same}))
	if _, err := s.Pool(2, 0); err == nil || !strings.Contains(err.Error(), "random source is broken") {
		t.Fatalf("got %v", err)
	}
	s = newSigner(t, 1, r, WithRandom(&script{ids: [][8]byte{a, {8}}}))
	if _, err := s.Pool(3, 0); err == nil {
		t.Fatal("signed with a random source that ran out")
	}
	if n := len(r.Lines()); n != 0 {
		t.Fatalf("%d lines written for batches that failed", n)
	}
	if text, _ := os.ReadFile(path); len(text) != 0 {
		t.Fatalf("the file holds %q", text)
	}
}

func TestNamed(t *testing.T) {
	path := recordPath(t)
	s := newSigner(t, 1, open(t, path))
	k, err := s.Named("Lena Fischer", "review copy for her podcast", 0)
	if err != nil {
		t.Fatal(err)
	}
	l, err := licence.Check(k, trust(s))
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Lena Fischer" {
		t.Fatalf("name %q", l.Name)
	}
	line := s.record.Lines()[0]
	if line.Kind != KindNamed || line.Note != "review copy for her podcast" {
		t.Fatalf("line %+v", line)
	}
	text, _ := os.ReadFile(path)
	if bytes.Contains(text, []byte("Lena")) {
		t.Fatal("the record holds the name")
	}
}

func TestPartner(t *testing.T) {
	s := newSigner(t, 1, open(t, recordPath(t)))
	keys, err := s.Partner("Bundle Hunt", 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range s.record.Lines() {
		if line.Kind != KindPartner || line.Note != "Bundle Hunt" || line.Index != i+1 || line.Edition != 1 {
			t.Fatalf("line %+v", line)
		}
		if line.Fingerprint != keys[i].Fingerprint().String() {
			t.Fatal("the lines are not in the order of the keys")
		}
	}
}

func TestRefuses(t *testing.T) {
	r := open(t, recordPath(t))
	s := newSigner(t, 1, r)
	cases := map[string]func() error{
		"no keys":                 func() error { _, err := s.Pool(0, 0); return err },
		"fewer than no keys":      func() error { _, err := s.Pool(-1, 0); return err },
		"too many keys":           func() error { _, err := s.Pool(MaxBatch+1, 0); return err },
		"partner without a name":  func() error { _, err := s.Partner("", 1, 0); return err },
		"partner with a newline":  func() error { _, err := s.Partner("Bundle\nHunt", 1, 0); return err },
		"partner with space":      func() error { _, err := s.Partner(" Bundle", 1, 0); return err },
		"partner turned around":   func() error { _, err := s.Partner("‮Bundle", 1, 0); return err },
		"partner too long":        func() error { _, err := s.Partner(strings.Repeat("x", MaxNote+1), 1, 0); return err },
		"partner not UTF-8":       func() error { _, err := s.Partner("Bundle\xff", 1, 0); return err },
		"named without a name":    func() error { _, err := s.Named("", "review", 0); return err },
		"named without a purpose": func() error { _, err := s.Named("Lena", "", 0); return err },
		"named, bad name":         func() error { _, err := s.Named("Lena\n", "review", 0); return err },
		"named, name too long":    func() error { _, err := s.Named(strings.Repeat("x", licence.MaxName+1), "review", 0); return err },
	}
	for name, try := range cases {
		if try() == nil {
			t.Errorf("%s: signed", name)
		}
	}
	if n := len(r.Lines()); n != 0 {
		t.Fatalf("%d lines written for refused requests", n)
	}
	if _, err := New(1, testKey()[:10], r); err == nil {
		t.Fatal("a signer with half a key")
	}
	if _, err := New(1, testKey(), nil); err == nil {
		t.Fatal("a signer with no record")
	}
}

func TestIssued(t *testing.T) {
	r := open(t, recordPath(t))
	one := newSigner(t, 1, r)
	two := newSigner(t, 2, r, WithRandom(&counter{n: 1 << 40}))
	if _, err := one.Pool(5, 0); err != nil {
		t.Fatal(err)
	}
	p, err := one.Partner("Bundle Hunt", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	n, err := one.Named("Lena Fischer", "review", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := two.Named("Max", "giveaway", 0); err != nil {
		t.Fatal(err)
	}
	want := []licence.Fingerprint{p[0].Fingerprint(), p[1].Fingerprint(), n.Fingerprint()}
	got := r.Issued(1)
	if len(got) != len(want) {
		t.Fatalf("%d issued, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("issued %d is %s, want %s", i, got[i], want[i])
		}
	}
	if len(r.Issued(2)) != 1 || len(r.Issued(3)) != 0 {
		t.Fatal("issued mixes up signers")
	}
}

// Many requests at once on one signer: every ID once, every batch whole and
// numbered in turn, and the file reads back as it was written.
func TestSignFromManyGoroutines(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	s := newSigner(t, 1, r)
	var wg sync.WaitGroup
	for g := range 12 {
		wg.Go(func() {
			for i := range 5 {
				var err error
				switch (g + i) % 3 {
				case 0:
					_, err = s.Pool(20, 0)
				case 1:
					_, err = s.Partner("Bundle Hunt", 3, 0)
				default:
					_, err = s.Named("Lena Fischer", "review", 0)
				}
				if err != nil {
					t.Error(err)
				}
				_ = r.Lines()
				_ = r.Issued(1)
			}
		})
	}
	wg.Wait()
	lines := r.Lines()
	r.Close()
	again := open(t, path).Lines()
	if len(again) != len(lines) {
		t.Fatalf("%d lines written, %d read back", len(lines), len(again))
	}
	if again[len(again)-1].Batch != 60 {
		t.Fatalf("last batch %d, want 60", again[len(again)-1].Batch)
	}
}

func TestOneSignerAtATime(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	if _, err := OpenRecord(path); err == nil || !strings.Contains(err.Error(), "another signer") {
		t.Fatalf("a second open: %v", err)
	}
	r.Close()
	r2, err := OpenRecord(path)
	if err != nil {
		t.Fatalf("after close: %v", err)
	}
	r2.Close()
	if err := r2.Close(); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
}

func TestClosedRecordRefuses(t *testing.T) {
	r := open(t, recordPath(t))
	s := newSigner(t, 1, r)
	r.Close()
	if _, err := s.Pool(1, 0); err == nil {
		t.Fatal("signed into a closed record")
	}
}

// A write that fails hands out no key, and a record whose file cannot even
// be cut back refuses every write after it.
func TestFailedWriteHandsOutNothing(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	s := newSigner(t, 1, r)
	if _, err := s.Pool(2, 0); err != nil {
		t.Fatal(err)
	}
	// The file can still be looked at but no longer written or cut back,
	// as when the disk goes read-only under the signer.
	r.f.Close()
	ro, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r.f = ro
	keys, err := s.Pool(2, 0)
	if err == nil || keys != nil {
		t.Fatalf("got %d keys and %v", len(keys), err)
	}
	if r.broken == nil {
		t.Fatal("a record that could not be cut back is not marked broken")
	}
	if _, err := s.Pool(1, 0); err == nil || !strings.Contains(err.Error(), "opening again") {
		t.Fatalf("a broken record: %v", err)
	}
	if n := len(r.Lines()); n != 2 {
		t.Fatalf("%d lines, want the 2 written before", n)
	}
	r.Close()
	if n := len(open(t, path).Lines()); n != 2 {
		t.Fatalf("%d lines after opening again, want 2", n)
	}
}

// A batch cut short on a line boundary, as a crash can leave it, is
// dropped whole when the record is opened, and the batches before it stay.
func TestBatchCutShortIsDropped(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	s := newSigner(t, 1, r)
	if _, err := s.Pool(2, 0); err != nil {
		t.Fatal(err)
	}
	r.Close()
	before, _ := os.ReadFile(path)
	r = open(t, path)
	s = newSigner(t, 1, r)
	if _, err := s.Pool(5, 0); err != nil {
		t.Fatal(err)
	}
	r.Close()
	full, _ := os.ReadFile(path)
	lines := strings.SplitAfter(string(full), "\n")
	cut := strings.Join(lines[:4], "") // batch 1 whole, two of batch 2's five
	if err := os.WriteFile(path, []byte(cut), 0o600); err != nil {
		t.Fatal(err)
	}
	r = open(t, path)
	if n := len(r.Lines()); n != 2 {
		t.Fatalf("%d lines, want batch 1's 2", n)
	}
	if r.Dropped != len(cut)-len(before) {
		t.Fatalf("dropped %d bytes, want %d", r.Dropped, len(cut)-len(before))
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, before) {
		t.Fatal("the file was not cut back to batch 1")
	}
	// And the next batch is numbered 2 again: the dropped one never left.
	if _, err := newSigner(t, 1, r).Pool(1, 0); err != nil {
		t.Fatal(err)
	}
	if b := r.Lines()[2].Batch; b != 2 {
		t.Fatalf("batch %d, want 2", b)
	}
}

func TestAppendTakesOnlyTheNextWholeBatch(t *testing.T) {
	r := open(t, recordPath(t))
	line := Line{Day: "2026-10-28", Signer: 1, ID: "0000-0000-0000-0001", Fingerprint: strings.Repeat("ab", 16), Batch: 1, Index: 1, Of: 2, Kind: KindPool}
	second := line
	second.ID, second.Fingerprint, second.Index = "0000-0000-0000-0002", strings.Repeat("cd", 16), 2
	for name, lines := range map[string][]Line{
		"nothing":       nil,
		"half a batch":  {line},
		"batch 2 first": {func() Line { l := line; l.Batch = 2; return l }(), func() Line { l := second; l.Batch = 2; return l }()},
		"the same key":  {line, func() Line { l := line; l.Index = 2; return l }()},
	} {
		if err := r.append(lines); err == nil {
			t.Fatalf("%s: written", name)
		}
	}
	if err := r.append([]Line{line, second}); err != nil {
		t.Fatal(err)
	}
	if err := r.append([]Line{line, second}); err == nil {
		t.Fatal("the same batch written twice")
	}
}

func TestHalfWrittenLastLineIsDropped(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	if _, err := newSigner(t, 1, r).Pool(3, 0); err != nil {
		t.Fatal(err)
	}
	r.Close()
	good, _ := os.ReadFile(path)
	torn := append(append([]byte(nil), good...), `{"day":"2026-10-28","signer":1,"id":"AB`...)
	if err := os.WriteFile(path, torn, 0o600); err != nil {
		t.Fatal(err)
	}
	r = open(t, path)
	if r.Dropped != len(torn)-len(good) {
		t.Fatalf("dropped %d bytes, want %d", r.Dropped, len(torn)-len(good))
	}
	if n := len(r.Lines()); n != 3 {
		t.Fatalf("%d lines, want 3", n)
	}
	if now, _ := os.ReadFile(path); !bytes.Equal(now, good) {
		t.Fatal("the file was not cut back to its whole lines")
	}
}

func TestRecordChangedByHandIsRefused(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	s := newSigner(t, 1, r)
	if _, err := s.Pool(2, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Named("Lena Fischer", "review", 0); err != nil {
		t.Fatal(err)
	}
	r.Close()
	good, _ := os.ReadFile(path)
	lines := strings.SplitAfter(string(good), "\n")[:3]

	edit := func(i int, from, to string) string {
		out := append([]string(nil), lines...)
		out[i] = strings.Replace(out[i], from, to, 1)
		if out[i] == lines[i] {
			t.Fatalf("%q is not in line %d", from, i)
		}
		return strings.Join(out, "")
	}
	l0 := s.record.Lines()[0]
	cases := map[string]string{
		"an ID in lowercase":        edit(0, l0.ID, strings.ToLower(l0.ID)),
		"an ID without dashes":      edit(0, l0.ID, strings.ReplaceAll(l0.ID, "-", "")),
		"an ID of zeros":            edit(0, l0.ID, "0000-0000-0000-0000"),
		"a short fingerprint":       edit(0, l0.Fingerprint, l0.Fingerprint[:30]),
		"a fingerprint in capitals": edit(0, l0.Fingerprint, strings.ToUpper(l0.Fingerprint)),
		"an unknown kind":           edit(0, `"kind":"pool"`, `"kind":"gift"`),
		"a pool key with a note":    edit(0, `"kind":"pool"`, `"kind":"pool","note":"x"`),
		"a named key with no note":  edit(2, `,"note":"review"`, ``),
		"an unknown field":          edit(0, `"kind":"pool"`, `"kind":"pool","name":"Lena"`),
		"a day that is no day":      edit(0, "2026-10-28", "2026-13-28"),
		"space added":               edit(0, `"signer":1`, `"signer": 1`),
		"a batch skipped":           edit(2, `"batch":2`, `"batch":3`),
		"a batch numbered back":     edit(2, `"batch":2`, `"batch":1`),
		"a place skipped":           edit(1, `"index":2`, `"index":3`),
		"a first batch at 0":        strings.ReplaceAll(strings.Join(lines, ""), `"batch":1`, `"batch":0`),
		"a line twice":              lines[0] + lines[0] + lines[1] + lines[2],
		"a line removed":            lines[0] + lines[2],
		"the lines swapped":         lines[1] + lines[0] + lines[2],
		"two lines on one":          strings.TrimSuffix(lines[0], "\n") + lines[1] + lines[2],
		"an empty line":             lines[0] + "\n" + lines[1] + lines[2],
		"not JSON":                  lines[0] + "hello\n",
		"a second named in batch":   strings.Join(lines, "") + strings.Replace(strings.Replace(lines[2], `"index":1`, `"index":2`, 1), l0.ID[:4], "FFFF", 1),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			if r, err := OpenRecord(path); err == nil {
				r.Close()
				t.Fatalf("opened:\n%s", text)
			}
		})
	}
}

func TestEmptyRecordAndItsFile(t *testing.T) {
	path := recordPath(t)
	r := open(t, path)
	if len(r.Lines()) != 0 || r.Dropped != 0 {
		t.Fatal("a new record is not empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("the record is %v, only its owner may read it", info.Mode().Perm())
	}
}
