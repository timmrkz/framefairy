// The walks and sequences that record a Playwright trace on every run,
// each with the issue it fails now and then in. A trace makes a walk
// slower, about a fifth in CI with its film of the screen, so it is
// recorded only where a failure is still waiting to be explained.
//
// A walk or a sequence that fails at random gets an issue and a line
// here, and its next failure in CI comes with its trace. The pull request
// that fixes it takes its line out again. A sequence is named by its
// name, a walk by its file, for every seed, or by its file and seed as
// it prints them, for that one. sequences.mjs --names fails on a name
// here that is no walk or sequence, so a line cannot outlive what it
// names. TRACE=all traces every walk for one run, TRACE=none none. See
// Finding why a walk failed in docs/TESTING.md.
export const traced = {
  "a cut's left edge dragged shows the frames before the cut, never one after it": 556,
  "a keyframe goes on a clip's first and last frame with C, and comes off with C": 559,
  "keyframes put with C glide the crop from one to the next in the video preview, frame by frame": 534,
  "playback.mjs SEED=1 HEVC=1": 532,
  "a cut changed before the caption under the playhead pops nothing": 514,
  "a playhead dragged far shows the key frame on the way, and the frame under the hand once it rests": 511,
  "Continue fills the transcription from nothing over what is left to hear": 487,
  "the frame round a word stays on the pill while the clip plays": 338,
  "a step onto a word pops it over the paused frame": 338,
};
