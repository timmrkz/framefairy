package signer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"framefairy/licence"
)

// Line is one key in the record. It says everything about the key except
// the key itself and, for a named key, the name: the key cannot be made
// again from the record, and nobody's name is kept.
type Line struct {
	Day         string `json:"day"` // the day it was signed, 2026-10-28
	Signer      uint8  `json:"signer"`
	ID          string `json:"id"`          // 652B-757A-8DC4-F04A
	Fingerprint string `json:"fingerprint"` // 32 lowercase hexadecimal digits
	Edition     uint8  `json:"edition"`
	Batch       int    `json:"batch"`          // counted from 1, one per Pool, Partner or Named
	Index       int    `json:"index"`          // its place in the batch, from 1
	Of          int    `json:"of"`             // how many keys the batch has
	Kind        string `json:"kind"`           // KindPool, KindPartner or KindNamed
	Note        string `json:"note,omitempty"` // the partner, or the purpose of a named key
}

// Record is the signer's file of every key it ever signed, one line each,
// only ever added to. It is how the signer never repeats an ID and how the
// genuine list of a signer is made after its key leaks.
type Record struct {
	mu     sync.Mutex
	f      *os.File
	unlock func()
	lines  []Line
	ids    map[licence.ID]bool
	prints map[licence.Fingerprint]bool
	broken error // set when the file may hold a half-written batch

	// Dropped is how many bytes of a half-written last batch were removed
	// when the record was opened. A batch is written to the record before
	// any of its keys leave the signer, so a signer that stopped in the
	// middle of writing one never handed it out, and nothing is lost.
	Dropped int
}

// OpenRecord opens the record at path, making it when there is none, and
// holds it so no second signer can write to it at the same time. A record
// that has been changed into something the signer would not have written
// is refused, with the line that is wrong.
func OpenRecord(path string) (*Record, error) {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	unlock, err := lock(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	r := &Record{f: f, unlock: unlock}
	r.rebuild(nil)
	if err := r.load(); err != nil {
		r.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if errors.Is(statErr, os.ErrNotExist) {
		// A new file is only there for good once its folder says so.
		if err := syncDir(filepath.Dir(path)); err != nil {
			r.Close()
			return nil, err
		}
	}
	return r, nil
}

// Close lets go of the record.
func (r *Record) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	r.unlock()
	err := r.f.Close()
	r.f = nil
	return err
}

func (r *Record) load() error {
	text, err := os.ReadFile(r.f.Name())
	if err != nil {
		return err
	}
	keep := len(text)
	if keep > 0 && text[keep-1] != '\n' {
		keep = bytes.LastIndexByte(text, '\n') + 1
	}
	var lines []Line
	var ends []int // where each line ends in text
	at := 0
	for i, raw := range bytes.SplitAfter(text[:keep], []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		at += len(raw)
		var l Line
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&l); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		if again, _ := json.Marshal(l); !bytes.Equal(append(again, '\n'), raw) {
			return fmt.Errorf("line %d is not written the way the signer writes it", i+1)
		}
		if err := r.admit([]Line{l}, true); err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		r.add([]Line{l})
		lines = append(lines, l)
		ends = append(ends, at)
	}
	// A last batch with fewer lines than it says it has was cut short while
	// it was written, so it goes, like a half-written line.
	if n := len(lines); n > 0 && lines[n-1].Index != lines[n-1].Of {
		first := n - lines[n-1].Index
		keep = 0
		if first > 0 {
			keep = ends[first-1]
		}
		r.rebuild(lines[:first])
	}
	if keep < len(text) {
		if err := r.f.Truncate(int64(keep)); err != nil {
			return fmt.Errorf("removing a half-written last batch: %w", err)
		}
		if err := r.f.Sync(); err != nil {
			return err
		}
		r.Dropped = len(text) - keep
	}
	return nil
}

// admit says whether lines can follow the record as it is: each key new,
// each batch numbered one on from the last and whole, each place in a
// batch one on from the place before. A batch may be left open at the end
// only while the record is read, where load decides what to do with it.
func (r *Record) admit(lines []Line, open bool) error {
	var prev Line
	if n := len(r.lines); n > 0 {
		prev = r.lines[n-1]
	}
	ids := map[licence.ID]bool{}
	prints := map[licence.Fingerprint]bool{}
	for _, l := range lines {
		if _, err := time.Parse(time.DateOnly, l.Day); err != nil {
			return fmt.Errorf("day %q: %w", l.Day, err)
		}
		id, err := licence.ParseID(l.ID)
		if err != nil || id.String() != l.ID || id == (licence.ID{}) {
			return fmt.Errorf("key ID %q is not written as the signer writes one", l.ID)
		}
		f, err := licence.ParseFingerprint(l.Fingerprint)
		if err != nil {
			return err
		}
		switch l.Kind {
		case KindPool:
			if l.Note != "" {
				return errors.New("a pool key has no note")
			}
		case KindPartner, KindNamed:
			if err := checkNote("the note", l.Note); err != nil {
				return err
			}
		default:
			return fmt.Errorf("a key of kind %q", l.Kind)
		}
		if l.Of < 1 || l.Of > MaxBatch || (l.Kind == KindNamed && l.Of != 1) {
			return fmt.Errorf("a batch of %d %s keys", l.Of, l.Kind)
		}
		switch {
		case l.Batch == prev.Batch+1 && l.Index == 1 && prev.Index == prev.Of:
		case l.Batch == prev.Batch && l.Index == prev.Index+1 && l.Index <= l.Of &&
			l.Of == prev.Of && l.Kind == prev.Kind && l.Note == prev.Note && l.Signer == prev.Signer &&
			l.Edition == prev.Edition && l.Day == prev.Day:
		default:
			return fmt.Errorf("batch %d place %d of %d after batch %d place %d of %d",
				l.Batch, l.Index, l.Of, prev.Batch, prev.Index, prev.Of)
		}
		if r.ids[id] || ids[id] {
			return fmt.Errorf("key ID %s twice", l.ID)
		}
		if r.prints[f] || prints[f] {
			return fmt.Errorf("fingerprint %s twice", l.Fingerprint)
		}
		ids[id] = true
		prints[f] = true
		prev = l
	}
	if !open && prev.Index != prev.Of {
		return fmt.Errorf("batch %d has %d of its %d keys", prev.Batch, prev.Index, prev.Of)
	}
	return nil
}

func (r *Record) add(lines []Line) {
	for _, l := range lines {
		id, _ := licence.ParseID(l.ID)
		f, _ := licence.ParseFingerprint(l.Fingerprint)
		r.ids[id] = true
		r.prints[f] = true
		r.lines = append(r.lines, l)
	}
}

// rebuild sets what the record knows to lines and nothing else.
func (r *Record) rebuild(lines []Line) {
	r.lines = nil
	r.ids = map[licence.ID]bool{}
	r.prints = map[licence.Fingerprint]bool{}
	r.add(lines)
}

// append writes one whole batch and makes sure it is on the disk before it
// returns. A batch is written whole or not at all: if the write fails, the
// file is cut back to where it was, and if even that fails, the record
// refuses every write after it until it is opened again, which removes the
// half-written batch.
func (r *Record) append(lines []Line) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return errors.New("the record is closed")
	}
	if r.broken != nil {
		return fmt.Errorf("the record needs opening again: %w", r.broken)
	}
	if len(lines) == 0 || lines[0].Batch != r.nextBatchLocked() || len(lines) != lines[0].Of {
		return errors.New("one write is one whole batch, the next one")
	}
	if err := r.admit(lines, false); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	info, err := r.f.Stat()
	if err != nil {
		return err
	}
	if _, err := r.f.Write(buf.Bytes()); err != nil {
		r.cutBack(info.Size(), err)
		return err
	}
	if err := r.f.Sync(); err != nil {
		r.cutBack(info.Size(), err)
		return err
	}
	r.add(lines)
	return nil
}

func (r *Record) cutBack(size int64, cause error) {
	if err := r.f.Truncate(size); err != nil {
		r.broken = errors.Join(cause, err)
		return
	}
	if err := r.f.Sync(); err != nil {
		r.broken = errors.Join(cause, err)
	}
}

func (r *Record) nextBatch() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nextBatchLocked()
}

func (r *Record) nextBatchLocked() int {
	if n := len(r.lines); n > 0 {
		return r.lines[n-1].Batch + 1
	}
	return 1
}

func (r *Record) has(id licence.ID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ids[id]
}

// Lines is a copy of every line in the record, oldest first.
func (r *Record) Lines() []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.lines)
}

// Issued is every key signer number signed that was not for the pool:
// partner batches and named keys. With the pool keys the dispenser sold,
// it is that signer's genuine list.
func (r *Record) Issued(signer uint8) []licence.Fingerprint {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []licence.Fingerprint
	for _, l := range r.lines {
		if l.Signer == signer && l.Kind != KindPool {
			f, _ := licence.ParseFingerprint(l.Fingerprint)
			out = append(out, f)
		}
	}
	return out
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
