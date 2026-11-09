package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"framefairy/engine"
	"framefairy/engine/fileio"
	"framefairy/engine/timed"
)

// Transcribe whole video and Export transcript, the two items about the
// words of a video in its menu in the sidebar, issue #432, and the part of
// a clip not heard yet, heard by itself, issue #745.

// Transcribe hears the whole video to its end, with no search after:
// Transcribe video in its menu. It is a job like a search, with a record,
// so it carries on from what it heard when it is asked for again, and its
// hearing gives way to the searches and clips made by hand of other
// videos, see lanes.go. A video has one at a time, so asking again while
// one runs gives the one that runs.
//
// It and the work on the video's clips go one at a time: it is refused
// while a search or a clip made by hand of the video runs, and either of
// those stops it, see stopTranscribing. Tim's report of 26 October, 12.27:
// carried on behind a search by itself, the two needed a menu nobody could
// read.
func (s *FrameFairy) Transcribe(path string) Job {
	label := jobLabel(engine.JobRecord{Kind: engine.JobTranscribe})
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobTranscribe, label, notInLibrary)
	}
	for _, j := range s.jobs.list() {
		if j.Episode == path && makesClips(j.Kind) && j.going() {
			return s.jobs.refuse(path, engine.JobTranscribe, label, errClipsFirst)
		}
	}
	return s.transcribe(path, timed.Window{}, false)
}

// transcribe queues the video's transcription of a part, the whole video
// to its end for a part with no end, see engine.Project.TranscribeJob.
// around says the part is the stretch kept heard around a clip, see
// partToHear. The whole video, a part of a clip and the stretch around a
// clip each have their record, and run side by side: lanes.go says which
// hears first.
func (s *FrameFairy) transcribe(path string, part timed.Window, around bool) Job {
	rec := engine.TranscribeRecord(part, around)
	// One that stopped is taken up by this one, which writes the record
	// again in its place.
	s.jobs.settle(path, engine.JobTranscribe, rec.ID)
	return s.jobs.addSteps(path, engine.JobTranscribe, jobLabel(rec), true, func(j *Job) {
		j.JobRecord.ID, j.From, j.To = rec.ID, rec.From, rec.To
	}, func(ctx context.Context, p *engine.Project, host engine.Host) (string, error) {
		return "", p.TranscribeJob(ctx, part, around, host)
	})
}

// The stretch kept heard on each side of a clip made by hand, so an edge
// dragged out finds its words there already, and how short a side gets
// before it is heard again, in seconds, see engine.ClipAround. Tim on
// #745: nobody using the app should wait for the transcription. Each time
// is 30 s of audio or more, which the speech model hears in a second or
// two on his Mac, while an edge is dragged through the 30 s still heard.
const (
	aroundClip  = 60.0
	aroundLeast = 30.0
)

// hearChosen has the clip chosen heard, by itself, the way a video is
// heard as it is added, issue #745: whatever of the clip is not heard
// yet, which an edge dragged out, a cut put back, an undo or a redo can
// bring in, and, for a clip made by hand, a stretch around it. st is the
// state of the video just read, see State, duration the video's length,
// and partToHear says what is heard.
func (s *FrameFairy) hearChosen(path string, st VideoState, duration float64) {
	if s.jobs == nil {
		return
	}
	for _, around := range []bool{false, true} {
		part, ok := partToHear(st, duration, around)
		if !ok {
			continue
		}
		msg := "the part of the clip chosen not heard yet is heard"
		if around {
			msg = "the stretch around the clip chosen is heard"
		}
		theLog.line(zerolog.InfoLevel, "app").Str("video", filepath.Base(path)).Str("clip", st.Chosen).
			Float64("from", part.Start).Float64("to", part.End).Msg(msg)
		s.transcribe(path, part, around)
	}
}

// partToHear is what to hear now for the clip chosen in st: what of the
// clip is not heard yet, or with around the stretch around a clip made by
// hand, see engine.ClipAround. Somebody waits on the first, so it goes
// ahead of the searches and of the stretch, see lanes.go. The stretch is
// heard once the clip is heard whole. A clip a search found lies in the
// window the search heard. A video never heard is the first search's to
// hear.
//
// While one is heard, the next of its kind waits for the state read once
// it ends. A part that failed is not asked for again, or every state
// would ask and fail again, and neither is one stopped by hand. One the
// app was closed on carries on.
func partToHear(st VideoState, duration float64, around bool) (timed.Window, bool) {
	if st.Status.Missing || len(st.Status.Heard) == 0 || st.Chosen == "" {
		return timed.Window{}, false
	}
	var part timed.Window
	found := false
	for _, c := range st.Clips {
		if c.Key != st.Chosen {
			continue
		}
		part, found = engine.ClipUnheard(c.ClipView, st.Status.Heard)
		if around {
			if found || !strings.HasPrefix(c.Key, engine.HandPlanName+"/") {
				return timed.Window{}, false
			}
			part, found = engine.ClipAround(c.ClipView, st.Status.Heard, duration, aroundClip, aroundLeast)
		}
	}
	if !found {
		return timed.Window{}, false
	}
	want := engine.HearsPart
	if around {
		want = engine.HearsAround
	}
	var last *Job
	for i, j := range st.Jobs {
		hears := j.Hears()
		// The stretch waits for the clip's own part too.
		if j.going() && (hears == want || (around && hears == engine.HearsPart)) {
			return timed.Window{}, false
		}
		if hears == want && (last == nil || j.Seq > last.Seq) {
			last = &st.Jobs[i]
		}
	}
	if last != nil && (last.State == JobFailed || last.State == JobStopped) &&
		last.From <= part.Start && last.To >= part.End {
		return timed.Window{}, false
	}
	return part, true
}

// errClipsFirst is Transcribe video asked for while the work on the
// video's clips runs, which the menu does not offer.
const errClipsFirst = "the video's clips are being found, transcribe it once that is done"

// stopTranscribing stops the whole video's transcription of a video the
// way Stop transcribing does, for a search or a clip made by hand of the
// video asked for, see Transcribe. What it heard stays.
func (s *FrameFairy) stopTranscribing(path, why string) {
	for _, j := range s.jobs.list() {
		// A part of a clip and the stretch around one are heard in turn
		// with the search's window, not stopped by it, or the clip would
		// wait for words for good.
		if j.Episode == path && j.Hears() == engine.HearsWhole && j.going() {
			theLog.jobLine(zerolog.InfoLevel, &j).Str("why", why).Msg("the whole video's transcription is stopped for the work on its clips")
			s.CancelJob(j.ID)
		}
	}
}

// errNotWhole is Export transcript asked for before the video is
// transcribed to its end, which the menu does not offer.
var errNotWhole = errors.New("the video is not transcribed to its end yet")

// The formats a transcript is exported in: text to read and paste, in
// paragraphs with their times, and subtitles, an srt file, which every
// subtitle tool and video site takes.
var transcriptFormats = map[string]struct {
	ext, kind string
	write     func(*engine.Transcript) string
}{
	"text": {".txt", "Text", engine.TranscriptText},
	"srt":  {".srt", "Subtitles", engine.TranscriptSRT},
}

// ExportTranscript saves the transcript of a whole video in a format,
// "text" or "srt", where the person says in the system's save box: Export
// transcript in its menu. The words read the way they were corrected in
// the caption box of every clip, see engine.CorrectedTranscript. It gives
// where the file was saved, or nothing when the box was cancelled.
//
// The file always ends in the format's own ending, whatever name was
// typed, so the box can only ever replace a file of that kind, never the
// video or anything else of the person's that it was pointed at. It is
// the one file the app writes outside the library, where the person says
// in the system's box, see docs/APP.md.
func (s *FrameFairy) ExportTranscript(path, format string) (saved string, err error) {
	if !s.store.Known(path) {
		return "", errNotInLibrary
	}
	as, ok := transcriptFormats[format]
	if !ok {
		return "", errors.New("there is no such format")
	}
	began := time.Now()
	words, bytes := 0, 0
	// How many words and how big, never a word of them, and where only
	// by whether it was saved.
	defer func() {
		level := zerolog.InfoLevel
		if err != nil {
			level = zerolog.WarnLevel
		}
		theLog.line(level, "files").Str("video", filepath.Base(path)).Str("format", format).Bool("saved", saved != "").
			Int("words", words).Int("bytes", bytes).Int64("ms", time.Since(began).Milliseconds()).Err(err).
			Msg("the transcript was exported")
	}()
	// Whole or not is asked of the same read as the words exported.
	h, err := s.heardIn(path)
	if errors.Is(err, engine.ErrNoTranscript) || (err == nil && !h.Done) {
		return "", errNotWhole
	}
	if err != nil {
		return "", err
	}
	t := h.Transcript
	// The plans oldest first, so of two clips that correct the same word
	// differently, the one in the plan changed last is read.
	plans := s.plansOf(path)
	slices.Reverse(plans)
	var clips []engine.Clip
	for _, summary := range plans {
		clips = append(clips, summary.AllClips()...)
	}
	corrected := engine.CorrectedTranscript(t, clips)
	text := as.write(corrected)
	words = len(corrected.Words)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + " transcript" + as.ext
	dest, err := s.saveAs(name, as.kind, as.ext)
	if err != nil || dest == "" {
		return "", err
	}
	// The ending is added to a name typed without it, which is a file the
	// box never asked to replace, so one already there is left as it is,
	// found by the review of #458.
	if !strings.EqualFold(filepath.Ext(dest), as.ext) {
		dest += as.ext
		if _, err := fileio.LstatIn(s.store.files, dest); err == nil {
			return "", fmt.Errorf("%s is already there. Export again and pick another name, or that file to replace it", filepath.Base(dest))
		}
	}
	// Written beside it and moved in place, so a file it replaces is
	// whole or untouched, and a link is replaced and not what it leads to.
	if err := fileio.WriteAtomic(s.store.files, dest, []byte(text)); err != nil {
		return "", err
	}
	// A file made to share, so it is readable the way a saved file is. It
	// is saved by now, so a drive that takes no modes, like exFAT or one on
	// the network, is no failure of the export, found by the second
	// review of #458.
	if err := s.store.files.Chmod(dest, 0o644); err != nil {
		theLog.line(zerolog.WarnLevel, "files").Str("video", filepath.Base(path)).Err(err).
			Msg("the exported transcript is saved, and its mode could not be set")
	}
	bytes = len(text)
	return dest, nil
}

// saveAs asks where to save a file of a kind, with the ending ext, in the
// system's save box, with name filled in, and gives the path chosen, or
// nothing when it was cancelled. Tests put their own in its place.
func (s *FrameFairy) saveAs(name, kind, ext string) (string, error) {
	if s.saveBox != nil {
		return s.saveBox(name)
	}
	if s.app == nil {
		return "", nil
	}
	return s.app.Dialog.SaveFile().
		SetFilename(name).
		SetButtonText("Export").
		CanCreateDirectories(true).
		AddFilter(kind, "*"+ext).
		PromptForSingleSelection()
}
