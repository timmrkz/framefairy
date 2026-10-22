package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// Transcribe whole video and Export transcript, the two items about the
// words of a video in its menu in the sidebar, issue #432.

// Transcribe hears the whole video to its end, with no search after:
// Transcribe whole video in its menu. It is a job like a search, with a
// record, so an app closed while it hears says so and Continue carries it
// on, and its hearing gives way to every search and clip made by hand,
// see lanes.go. A video has one at a time, so asking again while one runs
// gives the one that runs.
func (s *FrameFairy) Transcribe(path string) Job {
	label := jobLabel(engine.JobTranscribe, false)
	if !s.store.Known(path) {
		return s.jobs.refuse(path, engine.JobTranscribe, label, notInLibrary)
	}
	// One that stopped is taken up by this one, which writes the record
	// again in its place.
	s.jobs.settle(path, engine.JobTranscribe, "")
	return s.jobs.addSteps(path, engine.JobTranscribe, label, true, func(j *Job) {
		j.Record = engine.TranscribeID
	}, func(ctx context.Context, p *engine.Project, turn engine.Turn) (string, error) {
		return "", p.TranscribeJob(ctx, turn)
	})
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
// video or anything else of the person's that it was pointed at.
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
	if _, done := engine.Coverage(s.store.files, path, s.store.Settings().ASRModel); !done {
		return "", errNotWhole
	}
	t, err := s.words(path)
	if err != nil {
		return "", err
	}
	// The plans oldest first, so of two clips that correct the same word
	// differently, the one in the plan changed last is read.
	plans := plansOf(s.store.files, path)
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
	if !strings.EqualFold(filepath.Ext(dest), as.ext) {
		dest += as.ext
	}
	if err := afero.WriteFile(s.store.files, dest, []byte(text), 0o644); err != nil {
		return "", err
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
