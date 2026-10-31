// Package engine turns a long-form podcast master into vertical short-form
// clips, organised so that a command-line front end and a graphical one can
// both drive it.
//
// The stages, in the order a run goes through them:
//
//  1. audio      transcribe on the machine, time every word, measure loudness
//  2. lines      group words into the numbered lines the model reads
//  3. selection  the model chooses the moments and condenses them
//  4. analysis   framing, derived from the footage
//  5. captions   captions made of the same words, written as srt and ass
//  6. render     build the ffmpeg filter graph and run it
//
// A note on arm64, which is what Apple Silicon runs. The Go compiler may fuse
// a multiplication and an addition into one instruction there, and a fused
// result can round differently from the two steps one after the other,
// which is how every file so far was computed. An explicit float64() conversion prevents the fusion, which is
// why some products in this package are wrapped in one that looks redundant.
// The numbers themselves are rounded and written by engine/num.
package engine

// Version of the engine.
const Version = "0.3.0"
