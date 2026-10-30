// Package timed holds the types every part of the engine uses for a
// moment of a video: a piece of text with its times, a span of time, a
// window of the video and a loudness reading. It imports nothing of the
// engine, so every part of it can use them, see docs/ARCHITECTURE.md.
package timed

// Cue is a timed piece of text: one word, or one caption made of words, with
// its start and end in seconds.
type Cue struct {
	Start float64
	End   float64
	Text  string
}

// Span runs from Start to End, in seconds.
type Span struct {
	Start float64
	End   float64
}

// Window is the part of the episode a run works on, in seconds.
type Window struct {
	Start float64
	End   float64
}

// Reading is one loudness measurement, in dB, at a point in time.
type Reading struct {
	At    float64
	Level float64
}

// FrameSeconds is the resolution of the loudness measurement.
const FrameSeconds = 0.01
