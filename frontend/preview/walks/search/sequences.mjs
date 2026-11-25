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
    // Shift and a click pick cards and Copy copies their words, Tim's ask.
    // Then, in a short app so most cards are out of view, a card walked
    // to lands where it should, though typing lays out only the cards in
    // view. "a" after "ar" brings cards in again not laid out in that
    // search. Last, the answer to an earlier letter coming back after
    // the answer to a later one.
    steps: [
      ["trim", "start", 300],
      ["make while paused", "i"],
      ["search said", ["Jahre", 0, "copy"]],
      ["app size", 1100, 520],
      ["said lands", ["e", "ar", "a", ""]],
      ["said answers in order", ["a", "ar"]],
    ],
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
    // A video heard only part of the way, held and let through a piece at
    // a time, so part of it is still to come while the hand and the keys
    // work on it. Found by Tim on 20 October: a card under the hand was
    // lit a step less than the card the keys were on, and a part not
    // transcribed yet walked to with the keys lit nothing on the range
    // picker, where the hand on its card lit it there. Whatever is lit is
    // lit one way. Shift and the down arrow stopped at the card before a
    // part not transcribed yet, and the up arrow went past it. Then a
    // click on that card lit the part. Found by Tim on 19 October: while
    // the work heard a window, the parts not transcribed yet gave way in
    // thirds of it, each time the transcript was saved. They give way as
    // the work hears. And a search of what was said shows the parts not
    // transcribed yet, in the list and on the range picker.
    name: "the parts not transcribed yet light one way, the arrows walk onto and off them, they give way as the work hears, and a search of what was said shows them",
    steps: [
      ["speech", "hold"],
      ["add", 600],
      // First, since it opens the transcript itself.
      ["heard some"],
      ["parts give way"],
      ["heard some", 3],
      ["one light", "said"],
      ["walk over a part"],
      ["heard some", 12],
      // Escape gives the head and the clips back, where a search starts.
      ["press", "Escape"],
      ["search said", ["*", 1]],
      ["speech", "go"],
    ],
  },
  {
    // Tim felt the app lag while it worked. A running search reads the
    // clip list again every two seconds, and every read made every clip a
    // new one, the clip on screen too, so its captions were asked for
    // again and the clip timeline and the caption box drew them again,
    // with nothing changed. Plan row 2.242. And a card clicked puts the
    // playhead in the middle of the clip timeline, Tim's report of 14
    // November, issue #852.
    name: "a running search leaves the clip on screen alone, and a card clicked centres the playhead",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["asks none", "Captions", 7],
      ["card centres", "outside"],
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
    // Found in the review of #682: once Go said how many clips a window
    // suggests, the Target field kept the old number all through a drag
    // and caught up only when the hand let go. It follows the hand. A
    // number typed into it and Search clicked at once searched for the
    // number from before on a busy Mac, issue #877.
    name: "the clip count follows the window as its length is dragged",
    steps: [
      ["wait for", "Search"],
      ["count follows drag", "to", -0.3],
      ["type and search", "fits"],
      ["wait for", "Search"],
      ["type and search", "grows"],
      ["wait for", "Search"],
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
    // its search is the one Cancel stops.
    name: "a search stopped with Cancel can be removed, and stays removed",
    steps: [
      ["model", "holds"],
      ["add", 60],
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      // The transcription carries on after the search, and the dot
      // pulses while it runs, the way it does for all work.
      ["transcribed"],
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
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
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
      ["wait for", "Search"],
      ["head"],
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
    // to make a clip of, which the clip finds once it has heard it. Its
    // search is done, not failed, and says so in its row, issue #831. A
    // video with no sound at all is the same, and its clip timeline says
    // No sound.
    name: "a clip made by hand that failed says where it lies, and why in a toast",
    steps: [["speech", "wordless"], ["speech", "hold"], ["add", 120], ["wait for", "Search"], ["head"], ["wait for", "Cancel"], ["hand failed"], ["no speech"], ["no sound", 20]],
  },
  {
    // Tim on #456: I at the end of the video made a card that could only
    // fail. Nothing is said after the last word, so I is not offered there.
    // And Tim's report of 14 November, issue #852: the search of a video
    // just added ended while he picked cards in the transcript, and its
    // first clip took the playhead, the cards and the clip timeline away
    // from him. None of its clips is chosen then.
    name: "I after the last word is not offered, and says why, and a search leaves the cards a hand works in alone",
    steps: [
      ["model", "holds"],
      ["add", 120],
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
      ["work in the cards"],
      ["model", "answers"],
      ["view stays"],
      ["no mark after the end", 120],
    ],
  },
  {
    // Tim's report of 24 October, 15.02. A search on a window at the very
    // end of a video, partly transcribed, ended in one state after another:
    // the window's marks put in place and moved a frame later, then the
    // first clip chosen and the playhead jumping to it. It ends in one.
    name: "a search at the end of a video ends in one frame",
    steps: [
      ["speech", "hold"],
      ["add", 1200],
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["speech", "go"],
      ["wait for", "Continue"],
      ["speech", "hold"],
      ["move window", "bar", 1],
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
      ["ends in one frame"],
    ],
  },
  {
    // The fourth review of #458. A search that found its clips moved the
    // window on one answer after its first clip was picked, a step of its
    // own at the end of a search, and the window still drawn could be
    // written back over the one the Go side moved on. Tim's report of 25
    // October, 21.40: the window moved on to where the search had
    // transcribed to, and the dots after it began a few pixels right of
    // its edge, because they were laid from the end of the video and the
    // room left over fell at the window.
    name: "a search that found its clips moves the window on in the frame they show, and the dots after it begin at that window, with a dashed edge",
    steps: [
      // The model holds its answer and the speech stays held after the
      // window, so the transcription that carries on after the search
      // stands at the window's end, where the dots begin.
      ["speech", "hold"],
      ["model", "holds"],
      ["add", 1200],
      ["wait for", "Search"],
      ["head"],
      ["wait for", "Cancel"],
      ["window transcribed"],
      ["walks on with the clips", "model"],
      ["dots begin at the window"],
    ],
  },
  {
    // Tim's report of 24 October, 21.42. The first search of a video just
    // added was once the Go side's, with its window an even share of the
    // video, here 751.5 s, off the range picker's step. The window placed
    // on the step took its place, so Cancel while it heard left a search
    // on a window that was not the one drawn: set aside, it said Search,
    // with the sidebar's dot yellow. And the frame over the window dropped
    // the moment the search ended, where it now fades from the press.
    name: "Cancel on the search of a video just added says Stopped with Continue, and the window's frame fades",
    steps: [
      ["speech", "hold"],
      ["add and search", 1503],
      ["cancel fades"],
      ["speech", "go"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
    ],
  },
];
