// The sequences about searching: the clip list's rows of a search, the
// window, the language model and the speech model. They are cases found by
// hand, kept so they stay fixed. Their steps are said in steps.mjs beside
// this file, and in common/ for the steps of every folder. See
// ../sequences.mjs.
export default [
  {
    // Tim asked for a search through what was said, in the clip column
    // (#366): a card for each sentence, the clip it is in named on it,
    // and the parts not heard yet drawn where the search cannot look.
    name: "a search of what was said finds the sentences, names their clips and puts the playhead on one",
    // A second clip, made with I, so choosing one on the range picker has
    // somewhere to take the list.
    steps: [["trim", "start", 300], ["make while paused", "i"], ["search said", "Jahre"]],
  },
  {
    // Found by Tim: a word corrected in the caption box read corrected in
    // the video preview and as it was heard in the search of what was
    // said. A clip's cards are its captions, so they read what it shows.
    name: "a word corrected in a clip reads corrected in the search of what was said",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["search said", "Schulhoff"],
    ],
  },
  {
    // Found in the review of #367: the cards that could not be read left
    // the column empty, with nothing said. A clip trimmed so they are read
    // again as the search opens.
    name: "cards of what was said that cannot be read say so in the list",
    steps: [["said fails", "on"], ["trim", "start", 300], ["said fails", "check"]],
  },
  {
    // The same on a video heard only as far as its first search reached,
    // so part of it is not heard yet and says so, in the list and on the
    // range picker.
    name: "a search of what was said shows the parts not transcribed yet",
    steps: [
      // Held, and let through a piece at a time until some of it is
      // heard, so part of it is still to come while the search looks.
      ["speech", "hold"],
      ["add", 600],
      ["heard some", 12],
      ["search said", ["*", 1]],
      ["speech", "go"],
    ],
  },
  {
    // Found by Tim on 20 October: Shift and the down arrow stopped at the
    // card before a part not transcribed yet, and the up arrow went past
    // it. Then a click on that card lit the part. The speech model is
    // held, so the part stays where it is.
    name: "Shift and the arrows walk onto a part not transcribed yet and off it",
    steps: [["speech", "hold"], ["add", 600], ["heard some"], ["walk over a part"], ["speech", "go"]],
  },
  {
    // Found by Tim on 20 October: a card under the hand was lit a step
    // less than the card the keys were on, and a part not transcribed yet
    // walked to with the keys lit nothing on the range picker, where the
    // hand on its card lit it there. Whatever is lit is lit one way.
    name: "the hand and the keys light a card of the transcript and a part not transcribed yet the same",
    steps: [["speech", "hold"], ["add", 600], ["heard some", 3], ["one light", "said"], ["speech", "go"]],
  },
  {
    // Found by Tim on 19 October: while the work heard a window, the parts
    // not transcribed yet gave way in thirds of it, each time the
    // transcript was saved. They give way as the work hears.
    name: "the parts not transcribed yet give way as the work hears",
    steps: [["speech", "hold"], ["add", 600], ["heard some"], ["parts give way"], ["speech", "go"]],
  },
  {
    // Tim felt the app lag while it worked. A running search reads the
    // clip list again every two seconds, and every read made every clip a
    // new one, the clip on screen too, so its captions were asked for
    // again and the clip timeline and the caption box drew them again,
    // with nothing changed. Plan row 2.242.
    name: "a running search leaves the clip on screen alone",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["asks none", "Captions", 7],
    ],
  },
  {
    name: "Cancel stops a search, and Continue carries it on to its clips",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "Search"],
      ["cards", 1],
    ],
  },
  {
    // Tim cancelled a search to look somewhere else, and the window stood
    // locked on the range picker until the search was carried on. Moved
    // off it, the search is set aside: Search, and a fresh search where the
    // window is. Undo puts it back on the search, and Continue with it.
    name: "after Cancel the window moves, and Search looks where it was moved",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["move window", "to", -0.4],
      ["wait for", "Search"],
      ["no row", "Stopped. Click Continue"],
      ["undo"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["redo"],
      ["wait for", "Search"],
      ["restart"],
      ["wait for", "Search"],
      ["no row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "Search"],
      ["searched moved"],
    ],
  },
  {
    // Plan row 2.131. Cancel is pressed before the Go side has heard of
    // the search that Search asked for, which is held on its way there, so the
    // Cancel reaches the Go side first. The search never starts, and says
    // Stopped with Continue where its clips would have been.
    name: "Cancel pressed at once after Search stops the search before it starts",
    steps: [
      ["hold", "Search"],
      ["search and cancel"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
    ],
  },
  {
    name: "a search the app was closed on says Interrupted, after it opens again",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Interrupted. Click Continue"],
    ],
  },
  {
    name: "a search that failed says so, and Continue tries again",
    steps: [
      ["model", "fails"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Failed. Click Continue"],
      ["failure toast"],
      ["model", "answers"],
      ["head"],
      ["wait for", "Search"],
    ],
  },
  {
    // Issue #431: Tim saw a failed search red in the sidebar and in its
    // toast but yellow in its row, and could not put the row or the red
    // dot away without searching again.
    name: "a search that failed is red everywhere, and its row can be removed, put back first",
    steps: [
      ["model", "fails"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Failed. Click Continue"],
      ["failed red"],
      ["remove search", "put back"],
      ["dot", "err"],
      ["remove search", "for good"],
      ["wait for", "Search"],
      ["dot", ""],
      ["restart"],
      ["wait for", "Search"],
      ["no row", "Failed. Click Continue"],
      ["dot", ""],
    ],
  },
  {
    // Issue #431: the same for a search called off with Cancel. Tim saw
    // Removed slide in again each time he came back to the video while it
    // ran down. A video added first gives it another video to go to, and
    // its first search is the one Cancel stops.
    name: "a search stopped with Cancel can be removed, and stays removed",
    steps: [
      ["model", "holds"],
      ["add", 60],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["dot", "warn"],
      ["remove search", "away and back"],
      ["wait for", "Search"],
      ["dot", ""],
      ["restart"],
      ["wait for", "Search"],
      ["no row", "Stopped. Click Continue"],
      ["model", "answers"],
    ],
  },
  {
    // Issue #257: a clip made by hand whose work stopped could only be
    // carried on, never removed, since Cancel on the Activity page went.
    name: "a clip made by hand that stopped can be removed, and put back first",
    steps: [
      ["speech", "hold"],
      ["add", 120],
      ["hand stopped"],
      ["remove stopped", "put back"],
      ["remove stopped", "for good"],
      ["speech", "go"],
      ["restart"],
      ["no hand card"],
    ],
  },
  {
    // Issue #444: Tim searched windows heard in part already, and the fill
    // of the transcription started well past nothing. The speech is held,
    // so a search is stopped part way through its window and carried on.
    name: "Continue fills the transcription from nothing over what is left to hear",
    steps: [
      // Held from the start, so every piece of audio heard is one the walk
      // let through, and nothing depends on how fast the speech is.
      ["speech", "hold"],
      ["add", 120],
      ["wait for", "Cancel"],
      ["heard part", [0.2, 120]],
      ["cancel held"],
      ["fills from nothing", 120],
      ["speech", "go"],
    ],
  },
  {
    // Issue #450: the card of a clip made by hand that failed said why in
    // its second line, cut to a line. A video with no speech has nothing
    // to make a clip of, which the clip finds once it has heard it.
    name: "a clip made by hand that failed says where it lies, and why in a toast",
    steps: [["speech", "wordless"], ["speech", "hold"], ["add", 120], ["wait for", "Cancel"], ["hand failed"]],
  },
  {
    // Tim on #456: I at the end of the video made a card that could only
    // fail. Nothing is said after the last word, so I is not offered there.
    name: "I after the last word is not offered, and says why",
    steps: [["add", 120], ["wait for", "Search"], ["no mark after the end", 120]],
  },
];
