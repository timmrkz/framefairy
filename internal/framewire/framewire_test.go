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
		{Cursor: 5, Kind: End},
		{Cursor: 7, Kind: Failed, Body: []byte("no decoder")},
		{Cursor: 2, Kind: Opened, Body: []byte("file")},
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

// An open says whether it opened the file and the light of its picture,
// and a decoder from before HDR, which says only "file", is read as
// standard video.
func TestAnOpenSaysItsLight(t *testing.T) {
	for _, c := range []struct {
		file  bool
		light Light
		body  string
	}{
		{true, SDR, "file"},
		{false, SDR, ""},
		{true, HLG, "file hlg"},
		{false, PQ, " pq"},
	} {
		body := OpenedBody(c.file, c.light)
		if string(body) != c.body {
			t.Errorf("an open of file %v and light %q is %q, want %q", c.file, c.light, body, c.body)
		}
		file, light := ReadOpened(body)
		if file != c.file || light != c.light {
			t.Errorf("%q reads as file %v and light %q", body, file, light)
		}
	}
	if _, light := ReadOpened([]byte("file sunshine")); light != SDR {
		t.Errorf("a light nobody knows reads as %q, want standard video", light)
	}
	for trc, want := range map[string]Light{"smpte2084": PQ, "arib-std-b67": HLG, "bt709": SDR, "": SDR} {
		if got := LightOf(trc); got != want {
			t.Errorf("a transfer of %q is %q, want %q", trc, got, want)
		}
	}
}
