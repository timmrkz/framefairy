package framewire

import (
	"bytes"
	"testing"
)

// Records come back as they were written, one after another, and a
// header that claims more than any frame is refused before anything is
// allocated for it.
func TestRecordsComeBackAsWritten(t *testing.T) {
	var b bytes.Buffer
	in := []Record{
		{Cursor: 3, Kind: Frame, At: 12.04, Body: []byte{1, 2, 3, 4, 5, 6}},
		{Cursor: 3, Kind: Done},
		{Cursor: 7, Kind: Failed, Body: []byte("no decoder")},
	}
	for _, r := range in {
		if err := Write(&b, r); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range in {
		got, err := Read(&b)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cursor != want.Cursor || got.Kind != want.Kind || got.At != want.At || !bytes.Equal(got.Body, want.Body) {
			t.Errorf("record %d came back as %+v, want %+v", i, got, want)
		}
	}
	huge := make([]byte, HeaderSize)
	huge[13], huge[14], huge[15], huge[16] = 0xff, 0xff, 0xff, 0x7f
	if _, err := Read(bytes.NewReader(huge)); err == nil {
		t.Error("a record of 2 GB was read")
	}
}
