package tool

import (
	"bytes"
	"testing"
)

// What a program writes reaches its reader until the writer is cut, and
// after that it is taken and dropped, so a program left behind neither
// fails on a closed pipe nor writes into a buffer somebody already reads.
func TestACutWriterDropsWhatComesAfterTheCut(t *testing.T) {
	var got bytes.Buffer
	w := NewCutWriter(&got)
	if _, err := w.Write([]byte("before ")); err != nil {
		t.Fatal(err)
	}
	w.Cut()
	n, err := w.Write([]byte("after"))
	if n != len("after") || err != nil {
		t.Errorf("a write after the cut gave %d, %v, want it taken whole", n, err)
	}
	if got.String() != "before " {
		t.Errorf("the reader got %q, want only what came before the cut", got.String())
	}
}
