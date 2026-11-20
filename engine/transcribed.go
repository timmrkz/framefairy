package engine

// How far a video is transcribed, said from its files alone, so the
// interface draws it with no rule of its own about which parts heard
// count as what, #928.

// The ways a video is transcribed, EpisodeStatus.Transcript.
const (
	// TranscriptWhole is a video transcribed whole, as its file is now.
	TranscriptWhole = "whole"
	// TranscriptChanged is a video whose file changed since it was
	// transcribed.
	TranscriptChanged = "changed"
	// TranscriptTo is a video heard from its start to Covered, in one
	// part.
	TranscriptTo = "to"
	// TranscriptParts is a video heard in parts.
	TranscriptParts = "parts"
	// TranscriptNone is a video not heard at all.
	TranscriptNone = "none"
)

// transcript is EpisodeStatus.Transcript for the status s.
func (s EpisodeStatus) transcript() string {
	switch {
	case s.Transcribed:
		return TranscriptWhole
	case s.TranscriptStale:
		return TranscriptChanged
	case len(s.Heard) == 1 && s.Covered > 0:
		return TranscriptTo
	case len(s.Heard) > 0:
		return TranscriptParts
	}
	return TranscriptNone
}
