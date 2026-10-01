package signer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// FuzzOpenRecord opens any file at all as the record. It must either refuse
// it or keep exactly the whole batches in it: what is left in the file is
// what the record holds, written as the signer writes it, it is what was
// there minus what was dropped from the end, and it opens again the same.
func FuzzOpenRecord(f *testing.F) {
	pool := `{"day":"2026-10-28","signer":1,"id":"652B-757A-8DC4-F04A","fingerprint":"62fe5cd6c5086b5e1a4d9e0d2ad392f3","edition":0,"batch":1,"index":1,"of":1,"kind":"pool"}` + "\n"
	named := `{"day":"2026-11-12","signer":1,"id":"F625-9A14-E805-9D0D","fingerprint":"0123456789abcdef0123456789abcdef","edition":0,"batch":2,"index":1,"of":1,"kind":"named","note":"review"}` + "\n"
	f.Add([]byte(""))
	f.Add([]byte(pool))
	f.Add([]byte(pool + named))
	f.Add([]byte(pool + named[:40]))
	f.Add([]byte(pool + pool))
	f.Add([]byte("\n"))

	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, text []byte) {
		path := filepath.Join(dir, "record.jsonl")
		if err := os.WriteFile(path, text, 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := OpenRecord(path)
		if err != nil {
			return
		}
		lines := r.Lines()
		dropped := r.Dropped
		r.Close()
		left, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var want bytes.Buffer
		for _, l := range lines {
			b, _ := json.Marshal(l)
			want.Write(b)
			want.WriteByte('\n')
		}
		if !bytes.Equal(left, want.Bytes()) {
			t.Fatalf("the file holds\n%q\nthe record\n%q", left, want.Bytes())
		}
		if !bytes.Equal(text[:len(text)-dropped], left) {
			t.Fatalf("kept %q of %q, which is not what was there minus its end", left, text)
		}
		again, err := OpenRecord(path)
		if err != nil {
			t.Fatalf("opened once, refused the second time: %v", err)
		}
		defer again.Close()
		if again.Dropped != 0 || len(again.Lines()) != len(lines) {
			t.Fatal("opened differently the second time")
		}
	})
}
