package engine

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

// updateLoudnessCases writes the bytes of frontend/src/lib/loudness.cases.json
// again from what the engine makes, with go test ./engine -run
// TestTheWindowReadsThePiecesTheEngineMakes -update-loudness-cases.
var updateLoudnessCases = flag.Bool("update-loudness-cases", false, "write the bytes of frontend/src/lib/loudness.cases.json from the engine")

// loudnessCases is a loudness, readings measured over it, and the bytes a
// look at a view is answered with, in frontend/src/lib/loudness.cases.json.
// The window's own tests lay the same bytes over the same loudness, so the
// layout the engine writes and the one the window reads cannot drift apart
// unseen: each side's tests would stay green with a copy of its own.
// Found by the review of #974.
type loudnessCases struct {
	Count int `json:"count"`
	// Runs of readings, from, to and the byte each reading is, two to a
	// decibel above -90 dB: what was measured before, and what the
	// measuring brings while the window looks.
	Before [][3]int `json:"before"`
	Put    [][3]int `json:"put"`
	// The view in seconds.
	View [2]float64 `json:"view"`
	// The piece the look is answered with, once put is measured, and the
	// loudness the engine holds then, as Loudness lays it out, in hex.
	Piece string `json:"piece"`
	After string `json:"after"`
}

const loudnessCasesFile = "../frontend/src/lib/loudness.cases.json"

// putRuns hands a schedule runs of readings, each byte as the dB it stands
// for.
func putRuns(s *LoudnessSchedule, runs [][3]int) {
	for _, r := range runs {
		frames := make([]float32, r[1]-r[0])
		for i := range frames {
			frames[i] = float32(loudnessFloor + float64(r[2])/loudnessSteps)
		}
		s.Put(r[0], frames)
	}
}

// On the real disk: what is read is the repository's testdata, shared with
// the window's tests.
func TestTheWindowReadsThePiecesTheEngineMakes(t *testing.T) {
	data, err := os.ReadFile(loudnessCasesFile)
	if err != nil {
		t.Fatal(err)
	}
	var c loudnessCases
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	before := NewLoudnessSchedule()
	before.Start(nil, c.Count)
	putRuns(before, c.Before)
	s := NewLoudnessSchedule()
	s.Start(append([]byte(nil), before.held...), c.Count)
	window := Sent{Window: "w", N: 1}
	first := <-lookOn(s, View{From: c.View[0], To: c.View[1], Count: c.Count, Sent: window})
	if first.err != nil {
		t.Fatal(first.err)
	}
	version, _, _, _, _ := pieceOf(t, first.got)
	putRuns(s, c.Put)
	window.N++
	got := <-lookOn(s, View{From: c.View[0], To: c.View[1], Count: c.Count, Has: version, Sent: window})
	if got.err != nil {
		t.Fatal(got.err)
	}
	s.mu.Lock()
	after := hex.EncodeToString(s.held)
	s.mu.Unlock()
	piece := hex.EncodeToString(got.got)
	if *updateLoudnessCases {
		c.Piece, c.After = piece, after
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(loudnessCasesFile, append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if piece != c.Piece {
		t.Errorf("the engine answers the look with other bytes than %s holds, which the window's tests read: write them again with -update-loudness-cases and run make interface", loudnessCasesFile)
	}
	if after != c.After {
		t.Errorf("the engine holds another loudness than %s says: write it again with -update-loudness-cases and run make interface", loudnessCasesFile)
	}
}
