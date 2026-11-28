package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// clipListOf24 is a plan of 24 clips of 20 s over 20000 words heard,
// about two hours, the first word of every sixth clip corrected, read the
// way the clip list reads it, and its transcript.
func clipListOf24(b *testing.B) (PlanSummary, *Transcript) {
	b.Helper()
	tr := fromStored(longHeard(20000), nil, 0, 0, nil)
	var clips []string
	for i := range 24 {
		start := float64(i) * 250
		corrected := ""
		if i%6 == 0 {
			corrected = fmt.Sprintf(`, "corrections": {%q: "Wort"}`, wordKey(tr.HeardWords[i*750].Start))
		}
		clips = append(clips, fmt.Sprintf(`{"id": "%02d", "slug": "clip-%d", "title": "Clip %d", "segments": [{"start": %g, "end": %g}]%s}`,
			i+1, i, i, start, start+20, corrected))
	}
	fsys := afero.NewMemMapFs()
	path := "/Folge.framefairy/logs/clips.json"
	if err := afero.WriteFile(fsys, path, []byte(`{"clips": [`+strings.Join(clips, ", ")+`]}`), 0o644); err != nil {
		b.Fatal(err)
	}
	summary, err := summaryOf(fsys, path)
	if err != nil {
		b.Fatal(err)
	}
	return summary, tr
}

// The clip list is made on every read of a video's state, four times a
// second while work runs, each clip called by the words it shows now.
func BenchmarkTheClipListOf24Clips(b *testing.B) {
	summary, tr := clipListOf24(b)
	for b.Loop() {
		summary.NamedView(tr)
	}
}

// An edit reads the plan again, and the clips are named again from the
// names the transcript read keeps, see Transcript.Named.
func BenchmarkTheClipListOf24ClipsAfterAnEdit(b *testing.B) {
	summary, tr := clipListOf24(b)
	for b.Loop() {
		summary.titled = &keptTitles{}
		summary.NamedView(tr)
	}
}

// A transcript read again names every clip again, which is when a clip
// with a correction makes the words of the whole video again, see
// Transcript.Named.
func BenchmarkTheClipListOf24ClipsAfterTheTranscriptIsReadAgain(b *testing.B) {
	summary, tr := clipListOf24(b)
	l := tr.lookups
	for b.Loop() {
		summary.titled = &keptTitles{}
		tr.lookups = &heardLookups{raw: l.raw, snapped: l.snapped, words: l.words}
		summary.NamedView(tr)
	}
}
