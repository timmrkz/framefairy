// The parts of the app whose sequences still walk in Chromium, #380. The
// walks run in WebKit, the engine the app runs in on a Mac and on Linux,
// and the nightly walks in Chromium, which it runs in on Windows, see Two
// browsers in docs/TESTING.md. A part is the folder its sequences are kept
// in. A part moves to WebKit in a pull request of its own, which takes its
// name out of this list and fixes what WebKit finds there, so each stays
// a size to review, see the count in the decision record. Until the last
// name is out, the random walks walk in Chromium too. The pull request
// that takes it out takes this file out. sequences.mjs --names fails on a name here that is
// no part, so a name cannot outlive its folder.
export const inChromium = ["captions", "timeline", "playback", "clips", "search", "videos"];
