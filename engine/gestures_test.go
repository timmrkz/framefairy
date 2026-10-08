package engine

// The edits the tests make, each one gesture through Reshape, the one way
// a clip's pieces are changed. They keep the names the tests were written
// with, so what the tests prove about trimming and cutting holds for the
// gestures the app sends.

type Snap bool

const (
	ToWords  Snap = true
	ToFrames Snap = false
)

func TrimClip(planPath, clipID string, start, end float64, t *Transcript, keepPause float64, snap Snap) error {
	return Reshape(planPath, clipID, Gesture{Kind: "trim", Edge: "both", From: start, To: end,
		ToWords: bool(snap)}, t, keepPause)
}

func CutClip(planPath, clipID string, from, to float64, t *Transcript, keepPause float64, snap Snap) error {
	return Reshape(planPath, clipID, Gesture{Kind: "cut", From: from, To: to, ToWords: bool(snap)},
		t, keepPause)
}

func MoveCut(planPath, clipID string, index int, from, to float64, t *Transcript, keepPause float64,
	snap Snap) error {
	return Reshape(planPath, clipID, Gesture{Kind: "move", Index: index, From: from, To: to,
		ToWords: bool(snap)}, t, keepPause)
}

func JoinCut(planPath, clipID string, at float64, t *Transcript) error {
	return Reshape(planPath, clipID, Gesture{Kind: "join", From: at}, t, 0.1)
}

// The frames of an episode at 25 and at 5 frames a second, from the start
// of the file, for a gesture that puts its edges on frames.
var (
	at25 = SourceInfo{FPSNum: 25, FPSDen: 1}
	at5  = SourceInfo{FPSNum: 5, FPSDen: 1}
)
