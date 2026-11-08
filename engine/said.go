package engine

import (
	"log/slog"
	"strings"
)

// ProgramSaid hears a line a program the engine starts wrote for the log:
// which program, from, its process, the video it works on, empty for one
// that serves every video, and the line at its level. Every program the
// engine keeps running says its lines through it, the video's decoder as
// "decoder" and the local model's llama-server as "llama-server", so the
// app's log holds what each said, the way syslog tags a program. ffmpeg
// and ffprobe run with their errors only, and a job that fails says them
// in its own error.
type ProgramSaid func(from, video string, pid int, level slog.Level, line string)

// say hands a line to said, when there is one.
func (said ProgramSaid) say(from, video string, pid int, level slog.Level, line string) {
	if said != nil {
		said(from, video, pid, level, line)
	}
}

// llamaLine is a line llama-server writes, as the log takes it: its level
// from llama.cpp's own mark, E for an error and W for a warning and the
// rest for the log alone, and the line without the mark and the time
// since the server started, which the log's own time stands for. A line
// with neither, like the second line of a message, is kept whole, for
// the log alone.
func llamaLine(line string) (slog.Level, string) {
	stamp, rest, ok := strings.Cut(line, " ")
	if _, isStamp := serverStamp(stamp); !ok || !isStamp {
		return slog.LevelDebug, line
	}
	mark, text, _ := strings.Cut(rest, " ")
	switch mark {
	case "E":
		return slog.LevelError, text
	case "W":
		return slog.LevelWarn, text
	case "I", "D", "T":
		return slog.LevelDebug, text
	}
	return slog.LevelDebug, rest
}

// describesTheModel is whether a line of llama-server is part of its
// description of the model file, which is the same on every load of the
// file and says nothing about how the load went.
func describesTheModel(text string) bool {
	return strings.HasPrefix(text, "print_info:") || strings.HasPrefix(text, "llama_model_loader:")
}
