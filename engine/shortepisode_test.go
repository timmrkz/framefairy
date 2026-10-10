package engine

import (
	"bytes"
	"context"
	"testing"
)

// An episode shorter than a clip at its shortest is the clip, all of it,
// and the model is not asked. Tim added a fifteen second video with
// Shortest at 20 seconds, and its first search failed with "1 clips of at
// least 20s need 0:00:20".
func TestAShortEpisodeIsItsOwnClip(t *testing.T) {
	source := testEpisode(t, "15")
	ownTrainingDir(t)
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()

	e := NewEngine(NewLog(&bytes.Buffer{}, false, false))
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	ctx := context.Background()

	// Three clips of twenty seconds asked for, more than the episode has.
	plan, err := p.Search(ctx, PlanRequest{Count: 3, Min: 20, Max: 30}, nil)
	if err != nil {
		t.Fatalf("search: %v %s", err, p.LastError())
	}
	if asked != 0 {
		t.Errorf("the model was asked %d times about a clip there is only one of", asked)
	}
	_, clips, err := LoadClips(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 {
		t.Fatalf("%d clips, want the one", len(clips))
	}
	segs := clips[0].Segments
	from, to := float64(segs[0].Start), float64(segs[len(segs)-1].End)
	// The fake recogniser hears its first word at 0.1 and its last before
	// 14.65, so the clip is all that is said.
	if from > 0.5 || to < 14 {
		t.Errorf("the clip runs %.2f to %.2f, want the whole episode", from, to)
	}
}
