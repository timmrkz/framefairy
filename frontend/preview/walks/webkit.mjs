// The parts of the app whose sequences do not walk in WebKit yet, #380.
// Every walk and sequence is to run in Chromium, which the app runs in on
// Windows, and in WebKit, which it runs in on a Mac and on Linux, see Two
// browsers in docs/TESTING.md. A part is the folder its sequences are
// kept in. Until a part has walked in WebKit, and what that found is
// fixed, it is named here, and make walks WALKBROWSER=webkit leaves its
// sequences out. The pull request that walks a part in WebKit takes its
// name out, and the one that takes the last name out takes this file and
// the random walks' wait in TestWalks out too. sequences.mjs --names
// fails on a name here that is no part, so a name cannot outlive its
// folder.
export const notYet = ["captions", "timeline", "playback", "clips", "search", "videos", "app"];
