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
		{Cursor: 4, Kind: Hung},
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

// An open says how it got to its place, whether it opened the file, sought
// or went on, and the light of its picture. A decoder from before HDR,
// which says only "file", is read as standard video, and a word nobody
// writes as a seek.
func TestAnOpenSaysHowItGotThereAndItsLight(t *testing.T) {
	for _, c := range []struct {
		how   Reached
		light Light
		body  string
	}{
		{OpenedFile, SDR, "file"},
		{Sought, SDR, ""},
		{WentOn, SDR, "on"},
		{OpenedFile, HLG, "file hlg"},
		{Sought, PQ, " pq"},
		{WentOn, HLG, "on hlg"},
	} {
		body := OpenedBody(c.how, c.light)
		if string(body) != c.body {
			t.Errorf("an open that reached its place by %q with light %q is %q, want %q", c.how, c.light, body, c.body)
		}
		how, light := ReadOpened(body)
		if how != c.how || light != c.light {
			t.Errorf("%q reads as %q and light %q", body, how, light)
		}
	}
	if _, light := ReadOpened([]byte("file sunshine")); light != SDR {
		t.Errorf("a light nobody knows reads as %q, want standard video", light)
	}
	if how, _ := ReadOpened([]byte("flew pq")); how != Sought {
		t.Errorf("a way nobody knows reads as %q, want a seek", how)
	}
	for trc, want := range map[string]Light{"smpte2084": PQ, "arib-std-b67": HLG, "bt709": SDR, "": SDR} {
		if got := LightOf(trc); got != want {
			t.Errorf("a transfer of %q is %q, want %q", trc, got, want)
		}
	}
}

// A display matrix's rotation is read as a quarter turn to the left, the
// way ffprobe gives it, with a degree of slack, and anything else as none.
func TestQuarterReadsATurnToTheLeft(t *testing.T) {
	for left, want := range map[float64]int{
		0: 0, 90: 90, -90: 270, 180: 180, -180: 180, 270: 270, 89.6: 90, 450: 90, 45: 0, 92: 0,
	} {
		if got := Quarter(left); got != want {
			t.Errorf("a turn of %v is read as %d, not %d", left, got, want)
		}
	}
}
