package engine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sync"

	"framefairy/engine/fileio"
)

// planIndex is where each plan record starts in plans.jsonl, by its plan
// id, its reply key and its example key, so finding one record reads that
// one line rather than every record ever written. Every search adds a
// record of about 100 KB, and every trim or cut let go looked its plan up
// by reading them all, so a trim grew slower with every search made.
//
// The file is read once, and then only what was appended since. It is
// read again from the start when it is not the file it was, by
// fileio.Grew: shorter than what was read, or another file put in its
// place, and where the system cannot tell one file from another,
// starting with other bytes. A file in another folder is another file. The app
// removes it to clear the training, and framefairy-train reads it but
// never writes it. A record is checked against what was looked for once
// it is read, so a lookup never trusts the index over the file.
type planIndex struct {
	mu   sync.Mutex
	path string
	// size is how much of the file is read, up to the end of its last
	// whole line, and head its first bytes.
	size int64
	head []byte
	// seen is the file as it was when it was last read.
	seen fs.FileInfo
	// Where the records are, oldest first, by each key.
	byID, byReply, byExample map[string][]lineSpan
}

// lineSpan is where a line of the file starts and ends, its newline
// included.
type lineSpan struct{ at, end int64 }

// planKeys are the keys a plan record is looked up by.
type planKeys struct {
	PlanID     string `json:"plan_id"`
	ReplyKey   string `json:"reply_key"`
	ExampleKey string `json:"example_key"`
}

// headSize is how many of the file's first bytes tell it from another.
// The first record starts with its plan id, which holds when it was made.
const headSize = 256

// newest is the newest plan record of r whose key, by of, is key and
// that matches, read from the file at path.
func (x *planIndex) newest(r *Records, of func(*planIndex) map[string][]lineSpan, key string,
	matches func(PlanRecord) bool) (PlanRecord, bool) {
	path := filepath.Join(r.TrainingDir(), "plans.jsonl")
	x.mu.Lock()
	defer x.mu.Unlock()
	f, err := r.Files.Open(path)
	if err != nil {
		x.reset("")
		return PlanRecord{}, false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		x.reset("")
		return PlanRecord{}, false
	}
	if !x.same(f, path, info) {
		x.reset(path)
	}
	if err := x.catchUp(f, info); err != nil {
		return PlanRecord{}, false
	}
	x.seen = info
	at := of(x)[key]
	for i := len(at) - 1; i >= 0; i-- {
		var rec PlanRecord
		line, err := lineAt(f, at[i])
		if err == nil && json.Unmarshal(line, &rec) == nil && matches(rec) {
			return rec, true
		}
	}
	return PlanRecord{}, false
}

// reset forgets everything read, for the file at path.
func (x *planIndex) reset(path string) {
	x.path, x.size, x.head, x.seen = path, 0, nil, nil
	x.byID, x.byReply, x.byExample = map[string][]lineSpan{}, map[string][]lineSpan{}, map[string][]lineSpan{}
}

// same says whether f, at path and now as info says, is the file the
// index has read: the same file grown, by fileio.Grew, and starting with
// the same bytes, which tells a file put in its place where the system
// does not say which file is which.
func (x *planIndex) same(f io.ReaderAt, path string, info fs.FileInfo) bool {
	if x.path != path || x.byID == nil {
		return false
	}
	if x.seen != nil && !fileio.Grew(x.seen, info) {
		return false
	}
	if x.size == 0 {
		return true
	}
	head := make([]byte, len(x.head))
	n, err := f.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return n == len(x.head) && bytes.Equal(head, x.head)
}

// catchUp reads the whole lines appended to f, now as info says, since it
// was last read. A last line not ended yet is left for the next read, the
// way a record being appended is.
func (x *planIndex) catchUp(f io.ReaderAt, info fs.FileInfo) error {
	if info.Size() < x.size {
		x.reset(x.path)
	}
	if info.Size() == x.size {
		return nil
	}
	if len(x.head) < headSize {
		x.head = make([]byte, min(headSize, info.Size()))
		if _, err := f.ReadAt(x.head, 0); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}
	in := bufio.NewReaderSize(io.NewSectionReader(f, x.size, info.Size()-x.size), 1<<16)
	for {
		line, err := in.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		at := lineSpan{at: x.size, end: x.size + int64(len(line))}
		x.size = at.end
		keys, ok := keysOf(line)
		if !ok {
			continue
		}
		x.byID[keys.PlanID] = append(x.byID[keys.PlanID], at)
		x.byReply[keys.ReplyKey] = append(x.byReply[keys.ReplyKey], at)
		x.byExample[keys.ExampleKey] = append(x.byExample[keys.ExampleKey], at)
	}
}

// keysOf are the keys of the plan record on line. Reading the whole
// record took as long as reading every record did before there was an
// index, so they are found in its bytes: a record is written as JSON
// with each key once, and a quote inside a string is always escaped, so
// the first "plan_id":" is the record's own. A key whose value holds an
// escape, or a line that holds none of them, is read as JSON. A broken
// line, from a crash in the middle of writing it, is found when its
// record is read, and passed over then.
func keysOf(line []byte) (planKeys, bool) {
	var keys planKeys
	found := 0
	for _, k := range []struct {
		name string
		to   *string
	}{{`"plan_id":"`, &keys.PlanID}, {`"reply_key":"`, &keys.ReplyKey}, {`"example_key":"`, &keys.ExampleKey}} {
		i := bytes.Index(line, []byte(k.name))
		if i < 0 {
			continue
		}
		value := line[i+len(k.name):]
		end := bytes.IndexByte(value, '"')
		if end < 0 || bytes.IndexByte(value[:end], '\\') >= 0 {
			found = -1
			break
		}
		*k.to = string(value[:end])
		found++
	}
	if found > 0 {
		return keys, true
	}
	keys = planKeys{}
	return keys, json.Unmarshal(line, &keys) == nil
}

// lineAt is the line of f at span, read on its own.
func lineAt(f io.ReaderAt, span lineSpan) ([]byte, error) {
	line := make([]byte, span.end-span.at)
	if _, err := f.ReadAt(line, span.at); err != nil {
		return nil, err
	}
	return line, nil
}

// planRecordWith gives the newest plan record with the plan id.
func (x *planIndex) planRecordWith(r *Records, planID string) (PlanRecord, bool) {
	return x.newest(r, func(x *planIndex) map[string][]lineSpan { return x.byID }, planID,
		func(rec PlanRecord) bool { return rec.PlanID == planID })
}

// planRecordReplying gives the newest plan record with the reply key.
func (x *planIndex) planRecordReplying(r *Records, replyKey string) (PlanRecord, bool) {
	return x.newest(r, func(x *planIndex) map[string][]lineSpan { return x.byReply }, replyKey,
		func(rec PlanRecord) bool { return rec.ReplyKey == replyKey })
}

// planRecordAsking gives the newest plan record with the example key that
// matches.
func (x *planIndex) planRecordAsking(r *Records, exampleKey string, matches func(PlanRecord) bool) (PlanRecord,
	bool) {
	return x.newest(r, func(x *planIndex) map[string][]lineSpan { return x.byExample }, exampleKey,
		func(rec PlanRecord) bool { return rec.ExampleKey == exampleKey && matches(rec) })
}
