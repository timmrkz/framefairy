package main

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"

	"framefairy/engine"
)

// SourceView is what the player needs to know about an episode.
type SourceView struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	CropWidth  int     `json:"cropWidth"`
	CropHeight int     `json:"cropHeight"`
	// TurnedCropWidth is how wide the crop is on the picture rotated a
	// quarter turn, where its width and height have swapped: the crop of a
	// piece a person rotated, see engine.Segment.Rotate.
	TurnedCropWidth int `json:"turnedCropWidth"`
	// FPS is the episode's frame rate, which is what one step of the arrow
	// keys on the clip timeline is worth.
	FPS float64 `json:"fps"`
	// VideoStart is where the picture's first frame begins in the file,
	// and with it the grid its frames are on, so the app counts frames from
	// where the render does. Nought for most files.
	VideoStart float64 `json:"videoStart"`
	// Grid is the frames the video preview shows, the frames of the short,
	// see engine.SourceInfo.Grid: the frame queue counts on them and reads
	// nothing of the file itself.
	Grid GridView `json:"grid"`
	// The sound's samples a second and its channels, both nought for a
	// video with no sound.
	SoundRate     int `json:"soundRate"`
	SoundChannels int `json:"soundChannels"`
}

// GridView is framewire.Grid as the interface reads it.
type GridView struct {
	Origin float64 `json:"origin"`
	Start  float64 `json:"start"`
	Num    int     `json:"num"`
	Den    int     `json:"den"`
	Count  int     `json:"count"`
}

// probed keeps what each video was probed as last, by path alone, for a
// video whose file has gone since. What ffprobe said of a file that is
// there is kept by the app's machine, while the file and ffprobe stay as
// they are, see engine.Engine.Probe, so the app, the video preview and
// every job ask ffprobe once between them. FrameFairy keeps it. Its zero
// value is ready.
type probed struct {
	mu   sync.Mutex
	last map[string]engine.SourceInfo
}

// lastOf is what the episode at path was probed as last, if it ever was
// while the app ran.
func (p *probed) lastOf(path string) engine.SourceInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last[path]
}

// probe is what the video at path is, its frames among it, asked of
// probeVideo when a test handed one in and of ffprobe on the app's
// machine otherwise.
func (s *FrameFairy) probe(ctx context.Context, path string) (engine.SourceInfo, error) {
	ask := s.probeVideo
	if ask == nil {
		ask = s.probeOnDisk
	}
	info, err := ask(ctx, path)
	if err == nil {
		s.probes.mu.Lock()
		if s.probes.last == nil {
			s.probes.last = map[string]engine.SourceInfo{}
		}
		s.probes.last[path] = info
		s.probes.mu.Unlock()
	}
	return info, err
}

// probeOnDisk asks ffprobe on the app's machine, which keeps the answer.
func (s *FrameFairy) probeOnDisk(ctx context.Context, path string) (engine.SourceInfo, error) {
	return engine.NewEngine(engineLog("files", path), s.store.machine).Probe(ctx, path)
}

// Source probes an episode.
func (s *FrameFairy) Source(ctx context.Context, path string) (SourceView, error) {
	if !s.store.Known(path) {
		return SourceView{}, errNotInLibrary
	}
	info, err := s.probe(ctx, path)
	if err != nil {
		return SourceView{}, err
	}
	o := s.store.Settings().options()
	cw, ch := engine.CropWindow(info, o.Width, o.Height)
	tw, _ := engine.CropWindow(info.Rotated(90), o.Width, o.Height)
	grid := info.Grid()
	return SourceView{Duration: info.Duration, Width: info.Width, Height: info.Height,
		CropWidth: cw, CropHeight: ch, TurnedCropWidth: tw, FPS: info.FPS(), VideoStart: info.VideoStart,
		Grid:      GridView{Origin: grid.Origin, Start: grid.Start, Num: grid.Num, Den: grid.Den, Count: grid.Count},
		SoundRate: info.SoundRate, SoundChannels: info.SoundChannels}, nil
}

// frames are an episode's frames, the grid a gesture puts its edges on,
// see engine.Gesture. None when the episode cannot be probed, and the
// edges are then left where they are put.
func (s *FrameFairy) frames(ctx context.Context, path string) engine.SourceInfo {
	info, err := s.probe(ctx, path)
	if err != nil {
		return engine.SourceInfo{}
	}
	return info
}

// loudness is the loudness of an episode as far as it is measured, for
// the clip timeline to keep and draw every view from, see engine.Loudness
// and /frames/levels. It is the loudness measured on its own laid over
// the transcript's, which are the same frames from the same samples, so
// the waveform has whatever either has measured: the measuring runs ahead
// of the transcription, and a transcript made before it existed is there
// before it has run. An episode not measured yet has no readings, which
// is not a failure.
func (s *FrameFairy) loudness(path string) ([]byte, error) {
	// An episode shown is an episode measured: one added before the
	// loudness had a job of its own is measured the first time it is
	// opened.
	s.levels.start(path)
	t, err := s.transcript(path)
	if err != nil && !errors.Is(err, engine.ErrNoTranscript) {
		return nil, err
	}
	var levels engine.Levels
	if s.levels != nil {
		levels = s.levels.read(path)
	} else {
		levels = engine.ReadLevels(s.store.files, path)
	}
	return engine.Loudness(levels.Over(t)), nil
}

// serveLoudness answers /frames/levels?path=<episode> with the bytes of
// its loudness, plain, beside the frames and the sound it is drawn with.
// A call would carry them as base64 in JSON, a third more, which the
// window then turns back into bytes on its main thread. The window asks
// when the episode opens and whenever its state's Loudness changes, never
// while the view moves.
func (s *FrameFairy) serveLoudness(w http.ResponseWriter, r *http.Request) {
	// Known rather than episodeFile: the loudness is read from the work
	// folder, through the store's files, never from the video itself.
	path := r.URL.Query().Get("path")
	if !s.store.Known(path) {
		http.NotFound(w, r)
		return
	}
	body, err := s.loudness(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// serveLoudnessLook answers where the clip timeline looks with the
// loudness of its view the window does not hold, the way serveLook answers
// where the hand is with a frame: at once when it is measured, or the
// moment the measuring brings some of it. Nothing new, or a newer look
// that took its place, is answered with no content. See
// engine.LoudnessSchedule.
func (s *FrameFairy) serveLoudnessLook(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if !s.store.Known(path) {
		http.NotFound(w, r)
		return
	}
	from, errFrom := strconv.ParseFloat(q.Get("from"), 64)
	to, errTo := strconv.ParseFloat(q.Get("to"), 64)
	has, errHas := strconv.ParseUint(q.Get("has"), 10, 32)
	count, errCount := strconv.Atoi(q.Get("count"))
	if errors.Join(errFrom, errTo, errHas, errCount) != nil || math.IsNaN(from) || math.IsNaN(to) || count < 0 {
		http.Error(w, "the view is not one", http.StatusBadRequest)
		return
	}
	body, err := s.levels.lookLoudness(r.Context(), path, engine.View{From: from, To: to, Has: uint32(has), Count: count, Sent: sentOf(q)})
	switch {
	case err != nil:
		// Nothing new, a newer look in its place, or called off as the
		// window went.
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	}
}
