package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"framefairy/engine"
	"framefairy/engine/timed"
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

// The work on a video's clips that ended transcribes the rest of its
// video, the way Transcribe video does, since it stopped the
// transcription that started when the video was added, issue #447. So a
// later search of a window transcribed already finds its clips at once.
// A video transcribed to its end, work cut off by the app closing or by
// the video's removal, work that failed as it transcribed, more work on
// the video's clips on the way, and a transcription that ended ask for
// nothing. Nor does a transcription that failed or that a person
// stopped: it waits for Transcribe video.
func TestTheWorkOnClipsThatEndedTranscribesTheRestOfItsVideo(t *testing.T) {
	t.Parallel()
	words := [][3]any{{10.1, 10.4, "eins"}, {10.5, 10.9, "zwei"}}
	// The whole video's transcription as it ended before the work on the
	// clips did.
	whole := func(state string, madeWay bool) func(svc *FrameFairy, path string) {
		return func(svc *FrameFairy, path string) {
			svc.jobs.mu.Lock()
			defer svc.jobs.mu.Unlock()
			svc.jobs.jobs = append(svc.jobs.jobs, &Job{ID: "whole", Episode: path, State: state, madeWay: madeWay,
				JobRecord: engine.JobRecord{ID: engine.TranscribeID, Kind: engine.JobTranscribe}})
		}
	}
	for _, c := range []struct {
		name        string
		end         float64
		kind        string
		state       string
		step        string
		before      func(svc *FrameFairy, path string)
		transcribes bool
	}{
		{name: "a search that is done", end: 600, kind: engine.JobSearch, state: JobDone, transcribes: true},
		{name: "a search of a video transcribed to its end", end: 30, kind: engine.JobSearch, state: JobDone},
		{name: "a search that failed", end: 600, kind: engine.JobSearch, state: JobFailed, step: engine.StepFinding, transcribes: true},
		{name: "a search that failed as it transcribed", end: 600, kind: engine.JobSearch, state: JobFailed, step: engine.StepHearing},
		{name: "a search that stopped the transcription", end: 600, kind: engine.JobSearch, state: JobDone,
			before: whole(JobStopped, true), transcribes: true},
		{name: "a search after a person stopped the transcription", end: 600, kind: engine.JobSearch, state: JobDone,
			before: whole(JobStopped, false)},
		{name: "a search after the transcription failed", end: 600, kind: engine.JobSearch, state: JobDone,
			before: whole(JobFailed, false)},
		{name: "a search stopped by hand", end: 600, kind: engine.JobSearch, state: JobStopped, transcribes: true},
		{name: "a search cut off by the app closing", end: 600, kind: engine.JobSearch, state: JobCancelled},
		{name: "a search with another of the video waiting", end: 600, kind: engine.JobSearch, state: JobDone,
			before: func(svc *FrameFairy, path string) {
				svc.jobs.add(path, engine.JobSearch, "Search", func(context.Context, *engine.Project) (string, error) { return "", nil })
			}},
		{name: "a clip made by hand", end: 600, kind: engine.JobClip, state: JobDone, transcribes: true},
		{name: "a transcription that is done", end: 600, kind: engine.JobTranscribe, state: JobDone},
	} {
		svc, eps := memoryLibrary(t, "ep.mp4")
		svc.keeping = newKeeping()
		svc.jobs = newIdleQueue(svc.store, func(JobUpdate) {}, func(string) {})
		path := eps[0]
		heardPartOf(t, svc, path, 30, c.end, words)
		if c.before != nil {
			c.before(svc, path)
		}
		svc.searched(Job{Episode: path, State: c.state, JobRecord: engine.JobRecord{Kind: c.kind, Step: c.step}})
		if got := wholeJob(svc.jobs.list()) != nil; got != c.transcribes {
			t.Errorf("%s: the rest of the video is transcribed %v, want %v", c.name, got, c.transcribes)
		}
	}
}

// The rest of a video transcribed by itself, asked for the moment Search
// or a clip made by hand is pressed, never runs beside them, the way the user's
// report of 26 October asked: the two orders the calls can land in, made
// to happen on purpose. The rest asked for after the work on the clips
// was added is dropped, without a word, and the work on the clips added
// after the rest is asked for calls it off.
func TestTheRestOfAVideoIsNeverTranscribedBesideASearchPressedAtOnce(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{engine.JobSearch, engine.JobClip} {
		for _, restFirst := range []bool{false, true} {
			svc, eps := memoryLibrary(t, "ep.mp4")
			svc.jobs = newIdleQueue(svc.store, func(JobUpdate) {}, func(string) {})
			path := eps[0]
			rest := func() Job { return svc.transcribe(path, timed.Window{}, false, true) }
			clips := func() Job {
				return svc.jobs.addSteps(path, kind, kind, true, func(j *Job) { j.JobRecord.ID = kind }, nil)
			}
			// The check before the add passes in both orders: nothing of
			// the clips is going yet.
			if !svc.jobs.makeWayForTranscribe(path) {
				t.Fatalf("%s: the rest was refused before anything was asked for", kind)
			}
			var asked Job
			if restFirst {
				asked = rest()
				clips()
			} else {
				clips()
				asked = rest()
			}
			jobs := svc.jobs.list()
			if whole := wholeJob(jobs); whole != nil {
				t.Errorf("%s, the rest first %v: the rest of the video is transcribed beside the work on its clips: %+v", kind, restFirst, *whole)
			}
			if !restFirst && asked.ID != "" {
				t.Errorf("%s: the rest asked for after it was added, not dropped: %+v", kind, asked)
			}
			for _, j := range jobs {
				if j.State == JobFailed {
					t.Errorf("%s, the rest first %v: a job was refused out loud: %+v", kind, restFirst, j)
				}
			}
		}
	}
}
