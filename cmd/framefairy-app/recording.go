package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"

	"framefairy/engine"
)

// Screen recordings in Report a Problem, issue #326. A walk through a
// problem says more than a screenshot, so a report takes the recordings
// the usual screen recorders make, within the same 10 MB a picture may
// have: the videos the app takes, see engine.VideoExtensions, which the
// Mac's own recorder, QuickTime, Windows, phones and OBS make, and WebM,
// which the GNOME and KDE recorders and browsers make.
//
// What a file is, is judged by its first bytes and then by the ffmpeg the
// app ships, made to read it as that kind and nothing else, so a file
// that only claims to be a recording reaches no other reader of ffmpeg's.
// It is copied as it is, sound and picture untouched, without the place,
// the date, the device or anything else the file said about itself, the
// same way a picture is written again without its camera. The tile shows
// a still of its first frame.

// recordingEndings are the endings a recording is kept under: those of
// the videos the app takes, and WebM.
var recordingEndings = func() []string {
	var all []string
	for _, ext := range engine.VideoExtensions {
		all = append(all, strings.TrimPrefix(ext, "."))
	}
	return append(all, "webm")
}()

// recordingPattern is recordingEndings as the filter of a file box.
func recordingPattern() string {
	return engine.VideoPattern() + ";*.webm"
}

// recordingTime is how long ffprobe or ffmpeg may take over a recording
// of 10 MB, which they copy in well under a second.
const recordingTime = 30 * time.Second

// stillWidth is the widest the still on a tile is, which is shown a few
// hundred pixels wide at most.
const stillWidth = 1280

// sniffRecording says which of ffmpeg's readers may read data, by its
// first bytes, or "" for none. A HEIC or AVIF picture is in the same box
// as a MOV and is no recording.
func sniffRecording(data []byte) string {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return "matroska"
	}
	if len(data) < 12 {
		return ""
	}
	switch string(data[4:8]) {
	case "ftyp":
		switch string(data[8:12]) {
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1", "avif", "avis":
			return ""
		}
		return "mov"
	case "moov", "mdat", "wide", "free", "skip":
		return "mov"
	}
	return ""
}

// errNotKept is a file left out because it is neither a picture nor a
// recording the app can read.
var errNotKept = errors.New("is not a picture or a screen recording")

// recording copies a screen recording handed to the report without what
// else its file held, and takes a still of its first frame for the tile.
// It gives the recording's size and length, the ending it is kept under,
// the still as a PNG and the recording.
func (g handed) recording(reader string) (ReportPicture, []byte, []byte, error) {
	// On the real disk: ffprobe and ffmpeg read the recording from a file
	// of the app's own, which is gone again when this returns.
	dir, err := os.MkdirTemp("", "framefairy-recording-")
	if err != nil {
		return ReportPicture{}, nil, nil, errors.New("could not be kept")
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "handed")
	if err := os.WriteFile(in, g.data, 0o600); err != nil {
		return ReportPicture{}, nil, nil, errors.New("could not be kept")
	}
	e := engine.NewEngine(engine.NewLog(io.Discard, false, false), afero.NewOsFs())
	if e.FFmpeg == "" || e.FFprobe == "" {
		return ReportPicture{}, nil, nil, errors.New("needs ffmpeg, which the app is missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), recordingTime)
	defer cancel()
	pic, err := probeRecording(ctx, e.FFprobe, reader, in)
	if err != nil {
		return ReportPicture{}, nil, nil, err
	}
	if pic.Width*pic.Height > picturePixelsMax {
		return ReportPicture{}, nil, nil, errors.New("is too large a picture")
	}
	decoding()
	ending, film, err := copyRecording(ctx, e.FFmpeg, reader, in, filepath.Join(dir, "copy"), g.name)
	if err != nil {
		return ReportPicture{}, nil, nil, err
	}
	still, err := stillOf(ctx, e.FFmpeg, filepath.Join(dir, "copy."+ending), filepath.Join(dir, "still.png"))
	if err != nil {
		return ReportPicture{}, nil, nil, err
	}
	pic.Recording = ending
	return pic, still, film, nil
}

// probeRecording reads the size and length of a recording, as reader and
// nothing else, and turns away a file with no picture in it.
func probeRecording(ctx context.Context, ffprobe, reader, path string) (ReportPicture, error) {
	out, err := exec.CommandContext(ctx, ffprobe, "-v", "error", "-protocol_whitelist", "file",
		"-f", reader, "-select_streams", "v:0",
		"-show_entries", "stream=width,height:stream_side_data=rotation:format=duration",
		"-of", "json", path).Output()
	if err != nil {
		return ReportPicture{}, errNotKept
	}
	var data struct {
		Streams []struct {
			Width    int `json:"width"`
			Height   int `json:"height"`
			SideData []struct {
				Rotation float64 `json:"rotation"`
			} `json:"side_data_list"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(out, &data) != nil || len(data.Streams) == 0 {
		return ReportPicture{}, errNotKept
	}
	s := data.Streams[0]
	seconds, _ := strconv.ParseFloat(data.Format.Duration, 64)
	if s.Width <= 0 || s.Height <= 0 || !(seconds > 0) {
		return ReportPicture{}, errNotKept
	}
	pic := ReportPicture{Width: s.Width, Height: s.Height, Seconds: seconds}
	// A phone held upright records a picture lying on its side and says
	// how to turn it, which the copy keeps and every player follows.
	for _, side := range s.SideData {
		if r := int(side.Rotation) % 180; r == 90 || r == -90 {
			pic.Width, pic.Height = pic.Height, pic.Width
		}
	}
	return pic, nil
}

// copyRecording copies the first picture and every sound of a recording
// into a file of the same kind, without the rest: no metadata, no
// chapters, no tracks of data. It keeps the ending the file had when the
// copy can be made under it, and otherwise the ending every recording of
// that kind can be kept under. It gives the ending and the copy.
func copyRecording(ctx context.Context, ffmpeg, reader, in, out, name string) (string, []byte, error) {
	had := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	type kind struct{ ending, writer string }
	var tries []kind
	if reader == "matroska" {
		if had == "webm" {
			tries = append(tries, kind{"webm", "webm"})
		}
		tries = append(tries, kind{"mkv", "matroska"})
	} else {
		// An MPEG-4 file is kept as one, and anything else of its family
		// as a QuickTime file, which takes whatever the family holds.
		if mpeg4(had) {
			tries = append(tries, kind{had, "mp4"})
		}
		tries = append(tries, kind{"mov", "mov"})
	}
	var said []byte
	for _, k := range tries {
		path := out + "." + k.ending
		args := []string{"-nostdin", "-v", "error", "-protocol_whitelist", "file", "-f", reader, "-i", in,
			"-map", "0:v:0", "-map", "0:a?", "-c", "copy", "-map_metadata", "-1", "-map_chapters", "-1",
			"-fflags", "+bitexact"}
		if k.writer != "webm" && k.writer != "matroska" {
			args = append(args, "-movflags", "+faststart")
		}
		args = append(args, "-f", k.writer, "-y", path)
		var err error
		said, err = exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
		if err == nil {
			if film, err := os.ReadFile(path); err == nil && len(film) > 0 {
				return k.ending, film, nil
			}
		}
		_ = os.Remove(path)
	}
	theLog.line(zerolog.WarnLevel, "report").Str("name", tidyName(name)).Str("ffmpeg", cut(strings.TrimSpace(string(said)), 400)).
		Msg("a recording could not be copied")
	return "", nil, errors.New("could not be read as a screen recording")
}

// stillOf takes the first frame of a recording as a PNG, at most
// stillWidth wide, turned the way the recording asks.
func stillOf(ctx context.Context, ffmpeg, in, out string) ([]byte, error) {
	said, err := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-protocol_whitelist", "file",
		"-i", in, "-frames:v", "1", "-vf", fmt.Sprintf("scale='min(iw,%d)':-1", stillWidth),
		"-pix_fmt", "rgb24", "-f", "image2", "-c:v", "png", "-y", out).CombinedOutput()
	still, rerr := os.ReadFile(out)
	if err != nil || rerr != nil || len(still) == 0 {
		theLog.line(zerolog.WarnLevel, "report").Str("ffmpeg", cut(strings.TrimSpace(string(said)), 400)).
			Msg("the first frame of a recording could not be read")
		return nil, errors.New("could not be read as a screen recording")
	}
	return still, nil
}

// isRecordingEnding says whether a kept draft may name ending.
func isRecordingEnding(ending string) bool {
	return slices.Contains(recordingEndings, ending)
}

// mpeg4 says whether an ending is one of an MPEG-4 file.
func mpeg4(ending string) bool {
	return ending == "mp4" || ending == "m4v"
}
