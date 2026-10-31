package engine

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"framefairy/engine/runlog"
)

// An episode shorter than a clip at its shortest is the clip, all of it,
// and the model is not asked. Tim added a fifteen second video with
// Shortest at 20 seconds, and its first search failed with "1 clips of at
// least 20s need 0:00:20".
//
// On the real disk: ffmpeg reads the episode the search hears.
func TestAShortEpisodeIsItsOwnClip(t *testing.T) {
	source := testEpisode(t, "15")
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()

	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	ownRecords(e)
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
	_, clips, err := LoadClips(disk, plan)
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

// The clip of a short episode taken out of the list does not stop the
// episode from having one. Tim removed the one clip of a fifteen second
// video and searched again, and every search failed with "there is
// nothing to make a clip of in this video": the removed clip still
// counted as the moment taken, and it was the only moment there is. A
// search with the clip still in the list finds nothing new, and that is
// not a failure either.
//
// On the real disk: ffmpeg reads the episode the search hears.
func TestAShortEpisodeGetsItsClipAgainOnceRemoved(t *testing.T) {
	source := testEpisode(t, "15")
	var heard, asked int32
	server := fakeModel(t, &asked)
	defer server.Close()

	e := NewEngine(runlog.New(&bytes.Buffer{}, false, false), disk)
	e.OpenRecognizer = func(string) (Recognizer, error) { return fakeRecognizer{&heard}, nil }
	ownRecords(e)
	base := DefaultOptions()
	base.LLMURL = server.URL
	base.ASRModel = t.TempDir()
	base.Width, base.Height = 360, 640
	p := NewProject(e, source, base)
	ctx := context.Background()
	req := PlanRequest{Count: 1, Min: 20, Max: 30}

	first, err := p.Search(ctx, req, nil)
	if err != nil {
		t.Fatalf("first search: %v %s", err, p.LastError())
	}

	// Searched again with the clip in the list: nothing new, no failure.
	again, err := p.Search(ctx, req, nil)
	if err != nil {
		t.Fatalf("a search with the clip still there failed: %v %s", err, p.LastError())
	}
	if _, clips, _ := LoadClips(disk, again); len(clips) != 0 {
		t.Errorf("a search with the clip still there made %d clips more", len(clips))
	}

	_, clips, err := LoadClips(disk, first)
	if err != nil || len(clips) != 1 {
		t.Fatalf("first plan: %d clips, %v", len(clips), err)
	}
	removed := clips[0].ID
	before := TakeSnapshot(disk, nil, p.LogsDir())
	if err := SetRejected(disk, nil, first, removed, true); err != nil {
		t.Fatal(err)
	}
	removal := ChangeBetween(before, TakeSnapshot(disk, nil, p.LogsDir()))

	plan, err := p.Search(ctx, req, nil)
	if err != nil {
		t.Fatalf("search after the clip was removed: %v %s", err, p.LastError())
	}
	_, clips, err = LoadClips(disk, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 1 {
		t.Fatalf("%d clips after the removed one, want the episode again", len(clips))
	}
	segs := clips[0].Segments
	if from, to := float64(segs[0].Start), float64(segs[len(segs)-1].End); from > 0.5 || to < 14 {
		t.Errorf("the clip runs %.2f to %.2f, want the whole episode", from, to)
	}
	if asked != 0 {
		t.Errorf("the model was asked %d times about a clip there is only one of", asked)
	}

	// The clip made again is the clip that was removed, so the removed one
	// is gone: putting it back or undoing its removal showed the episode
	// twice.
	if _, clips, _ := LoadClips(disk, first); len(clips) != 0 {
		t.Errorf("the removed clip is still in its plan beside the one made again")
	}
	if err := SetRejected(disk, nil, first, removed, false); err == nil {
		t.Errorf("the removed clip could be put back beside the one made again")
	}
	if _, err := removal.Undo(); !errors.Is(err, ErrChangedSince) {
		t.Errorf("undoing the removal: %v, want it refused as changed since", err)
	}
	listed := 0
	for _, path := range p.Plans() {
		_, clips, _ := LoadClips(disk, path)
		for _, c := range clips {
			if !c.Rejected {
				listed++
			}
		}
	}
	if listed != 1 {
		t.Errorf("%d clips in the list, want the one", listed)
	}
}
