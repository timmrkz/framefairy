package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

	// A name typed without the ending is given it, and a file already
	// there under that name was never asked about, so it stays.
	chosen = "/Users/tim/Desktop/notes"
	if err := afero.WriteFile(svc.store.files, chosen+".txt", []byte("my notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if saved, err := svc.ExportTranscript(mine, "text"); saved != "" || err == nil {
		t.Errorf("saved over a file the box never asked about, at %q: %v", saved, err)
	}
	if body, _ := afero.ReadFile(svc.store.files, chosen+".txt"); string(body) != "my notes" {
		t.Errorf("the file the box never asked about reads %q", body)
	}
	// One the box asked about is replaced whole.
	chosen += ".txt"
	if saved, err := svc.ExportTranscript(mine, "text"); saved != chosen || err != nil {
		t.Errorf("replacing a file the box asked about saved at %q: %v", saved, err)
	}
	if body, _ := afero.ReadFile(svc.store.files, chosen); string(body) != "0:10\neins zwo drei.\n" {
		t.Errorf("the replaced file reads %q", body)
	}
	if info, err := svc.store.files.Stat(chosen); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("the file is saved as %v: %v", info.Mode(), err)
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

// A file the save box asked to replace that is a link is replaced itself,
// and what it leads to, the video here, is left as it was. Found by the
// review of #458: the export wrote through the link.
//
// On the real disk: a link and where it leads are what the test is about.
func TestTheExportReplacesALinkAndNotTheVideo(t *testing.T) {
	svc, mine, home := library(t)
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}})
	link := filepath.Join(home, "ep.srt")
	if err := os.Symlink(mine, link); err != nil {
		t.Fatal(err)
	}
	svc.saveBox = func(string) (string, error) { return link, nil }
	if saved, err := svc.ExportTranscript(mine, "srt"); saved != link || err != nil {
		t.Fatalf("saved at %q: %v", saved, err)
	}
	if body, err := os.ReadFile(mine); err != nil || string(body) != "video" {
		t.Errorf("the video reads %q: %v", body, err)
	}
	if info, err := os.Lstat(link); err != nil || !info.Mode().IsRegular() {
		t.Errorf("the link was not replaced by the subtitles: %v", err)
	}
}

// A drive that takes no modes, like exFAT or one on the network.
type noModes struct{ afero.Fs }

func (noModes) Chmod(string, os.FileMode) error { return errors.New("modes are not supported") }

// The file is saved before its mode is set, so a drive that takes no
// modes still gets the export, and it is not said to have failed. Found
// by the second review of #458.
func TestAnExportToADriveWithoutModesIsSaved(t *testing.T) {
	svc, mine, _ := anEpisodeWithAPlan(t)
	heardEpisode(t, svc, mine, 30, [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei."}})
	svc.store.files = noModes{svc.store.files}
	svc.saveBox = func(string) (string, error) { return "/Volumes/Stick/ep.txt", nil }
	saved, err := svc.ExportTranscript(mine, "text")
	if err != nil || saved != "/Volumes/Stick/ep.txt" {
		t.Fatalf("saved at %q: %v", saved, err)
	}
	if body, err := afero.ReadFile(svc.store.files, saved); err != nil || string(body) != "0:10\neins zwei.\n" {
		t.Errorf("the text reads %q: %v", body, err)
	}
}
