package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// Export transcript saves the words of the whole video with the word
// corrected in a clip's caption box, as text and as subtitles, in a file
// that always ends in its format's own ending. It is not offered before
// the video is transcribed to its end, and a cancelled save box saves
// nothing. Issue #432.
func TestTheTranscriptIsExportedWithItsCorrections(t *testing.T) {
	svc, mine, plan := anEpisodeWithAPlan(t)
	var asked []string
	chosen := "/Users/tim/Desktop/ep transcript"
	svc.saveBox = func(name string) (string, error) {
		asked = append(asked, name)
		return chosen, nil
	}
	if _, err := svc.ExportTranscript(mine, "text"); !errors.Is(err, errNotWhole) {
		t.Errorf("exported before the video was transcribed: %v", err)
	}
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}, {11.0, 11.4, "drei."}})
	// The answer describes the clip for the interface, which needs the
	// video read, and this one is not a video. The word is corrected
	// either way.
	_, _ = svc.SetWord(context.Background(), mine, plan, "01", 10.5, "zwo")

	saved, err := svc.ExportTranscript(mine, "text")
	if err != nil || saved != chosen+".txt" {
		t.Fatalf("saved at %q: %v", saved, err)
	}
	body, err := afero.ReadFile(svc.store.files, saved)
	if err != nil || string(body) != "0:10\neins zwo drei.\n" {
		t.Errorf("the text reads %q: %v", body, err)
	}

	chosen = "/Users/tim/Desktop/ep.srt"
	saved, err = svc.ExportTranscript(mine, "srt")
	if err != nil || saved != chosen {
		t.Fatalf("saved at %q: %v", saved, err)
	}
	body, _ = afero.ReadFile(svc.store.files, saved)
	if !strings.HasPrefix(string(body), "1\n00:00:10,100 --> 00:00:11,400\neins zwo drei.\n") {
		t.Errorf("the subtitles read %q", body)
	}
	if len(asked) != 2 || asked[0] != "ep transcript.txt" || asked[1] != "ep transcript.srt" {
		t.Errorf("the save box was opened on %q", asked)
	}

	// Pointed at the video itself, it saves beside it, never over it.
	chosen = mine
	saved, err = svc.ExportTranscript(mine, "text")
	if err != nil || saved != mine+".txt" {
		t.Errorf("pointed at the video, saved at %q: %v", saved, err)
	}

	chosen = ""
	if saved, err := svc.ExportTranscript(mine, "text"); saved != "" || err != nil {
		t.Errorf("a cancelled save box saved %q: %v", saved, err)
	}
	if _, err := svc.ExportTranscript(mine, "docx"); err == nil {
		t.Error("exported in a format there is not")
	}
	if _, err := svc.ExportTranscript("/elsewhere/other.mp4", "text"); !errors.Is(err, errNotInLibrary) {
		t.Errorf("exported a video that is not in the library: %v", err)
	}
}
