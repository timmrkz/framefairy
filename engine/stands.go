package engine

// How a video stands in the sidebar: its dot and the line under its name.
// The sidebar once worked both out for itself from the status and the
// search, and a search set aside still marked the video, #436. Now they
// are said here and the sidebar only draws them, #928.

// The ways a video stands, EpisodeStatus.Stands. A video with none of
// these stands as nothing worth a mark.
const (
	// StandsFailed is a video whose file is gone, or whose search failed.
	StandsFailed = "failed"
	// StandsLook is a video that needs a look: its search stopped or was
	// cut off, or its file changed since it was transcribed.
	StandsLook = "look"
	// StandsMade is a video with clips found or made in it.
	StandsMade = "made"
)

// How far a video is transcribed, EpisodeStatus.Transcript.
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

// The search of a video as the app's queue knows it, for Standing: none
// that has anything to say, one that failed, or one that stopped or was
// cut off.
const (
	SearchQuiet = iota
	SearchFailed
	SearchStopped
)

// Standing is how the video whose status s is stands in the sidebar,
// with search how its newest search stands, SearchQuiet and the rest. A
// file gone and a failed search come first, then what needs a look, then
// clips made. A search set aside says nothing, so the app passes it as
// SearchQuiet.
func (s EpisodeStatus) Standing(search int) string {
	switch {
	case s.Missing || search == SearchFailed:
		return StandsFailed
	case search == SearchStopped || s.TranscriptStale:
		return StandsLook
	case len(s.Plans) > 0:
		return StandsMade
	}
	return ""
}

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
