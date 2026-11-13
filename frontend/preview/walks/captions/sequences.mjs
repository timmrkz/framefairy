// The sequences about the caption box and the captions drawn over the video
// preview: words, blocks, pills, fonts and styles. They are cases found by
// hand, kept so they stay fixed. Their steps are said in steps.mjs beside
// this file, and in common/ for the steps of every folder. See
// ../sequences.mjs.
export default [
  {
    // The heights the captions may sit at, and where they start out, come
    // from the Go side, see engine.CaptionStart. The interface kept a copy
    // of both, which could drift from the engine's, issue #610.
    name: "the captions dragged land on a height the Go side offers, and put back sit where they start out",
    steps: [
      ["drag captions", 120],
      ["captions put back"],
    ],
  },
  {
    // Which words a removal leaves, and when every caption shows, is the
    // Go side's, see cmd/framefairy-app/captionedits_test.go. The walk
    // holds the keys that remove a word and the caption box showing it
    // gone, and back with Undo.
    name: "a word removed with delete or saved empty leaves the caption box, and undo and redo show it",
    steps: [
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["box", "Ich war sechs Jahre alt,"],
      ["undo"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["redo"],
      ["box", "Ich war sechs Jahre alt,"],
      ["click", "Jahre"],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["press", "Enter"],
      ["box", "Ich war sechs alt,"],
      ["frame", "sechs"],
      ["press", "Delete"],
      ["box", "Ich war alt,"],
    ],
  },
  {
    // Found by Tim on #201: a caption whose first word the picture lights
    // for a single frame showed nothing when its block was clicked, and
    // its block did not pop, while shift and an arrow went to that frame.
    // The clip's start dragged near the end of the first word lands it on
    // the frame before "war", which starts in the middle of the clip's
    // first frame, so the first frame shows its caption with no word lit
    // and a click a frame into "war" by its own time landed there. Only
    // the canvas draws the captions, the caption box over it none.
    name: "every caption block clicked lights a word of its caption",
    steps: [["trim into", 0.04], ["every block"], ["drawn once"]],
  },
  {
    name: "delete removes the caption block clicked on the clip timeline",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["blocks", 8],
      ["box", ""],
      ["undo"],
      ["blocks", 9],
      ["box", "als mich auf dem Schulhof irgendein"],
      ["redo"],
      ["blocks", 8],
      ["box", ""],
    ],
  },
  {
    // Tim removed a caption block, removed its clip with the bin, and
    // while its row still offered it back pressed I where the playhead
    // was. The clip came back, and the caption he had removed was still
    // gone from it: a removed word was kept for the whole video, so every
    // clip over it lost it. Now it is the clip's, and goes with it, and the
    // clip put back has it removed as it was. Plan row 2.249. Which words
    // a clip made again has is the Go side's, see
    // TestAClipMadeAgainHasTheCaptionsItsRemovedOneLost and the tests
    // beside it in engine/clipstate_test.go.
    name: "a caption removed stays removed when its clip is put back, and is there again when the clip is made again with I",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["blocks", 8],
      ["remove clip"],
      ["put back"],
      ["blocks", 8],
      ["remove clip"],
      ["make here", "i"],
      ["blocks", 9],
    ],
  },
  {
    // A drag sets when a caption is shown: the word is left out while the
    // edge stands past it, and is still there, #598. Which words an edge
    // leaves out, and that it moves no other caption, is the Go side's,
    // see TestDraggingACaptionEdgeOverAWordRemovesIt and
    // cmd/framefairy-app/captionedits_test.go. Each drag here checks the
    // edge stays where it was let go.
    name: "a caption's edges dragged in over a word and back out stay where they are let go",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["differs from", "start"],
      ["undo"],
      ["same", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge out", ["end", "Schulhof"]],
      ["edge in", ["start", "Schulhof"]],
      ["edge out", ["start", "Schulhof"]],
    ],
  },
  {
    // A click on the edge where a caption appears put the playhead on the
    // frame that holds its first word's start, which begins before the
    // word, where the caption is not up yet. Tim saw it on #294.
    // Its caption starts at 10.15 s, between two frames of the test video.
    name: "a click on either edge of a caption shows the caption",
    steps: [
      ["caption edge", ["start", "jedenfalls"]],
      ["drawn", "jedenfalls"],
      ["caption edge", ["end", "jedenfalls"]],
      ["drawn", "jedenfalls"],
    ],
  },
  {
    name: "a caption's edges dragged with shift land on words",
    steps: [
      ["edge to word", ["start", "Schulhof"]],
      ["drawn", "Schulhof"],
      ["edge to word", ["end", "Schulhof"]],
      ["drawn", "Schulhof"],
    ],
  },
  {
    // A word removed in the caption box is fixed, like a typo, and a drag
    // only sets when the caption is shown. Tim removed a word, dragged the
    // start of the caption after it out to where he heard its next word
    // begin, and the word came back, #598.
    // The word staying removed is the Go side's, see
    // TestNoEdgeBringsBackAWordRemovedInTheCaptionBox. The walk holds the
    // blocks on the clip timeline meeting after the drag.
    name: "a caption's first word removed stays removed when its left edge is dragged out over it",
    steps: [
      ["mark", "start"],
      ["remove edge word", ["first", "Schulhof"]],
      ["edge out", ["start", "Schulhof"]],
      ["meets", ["start", "Schulhof", "before"]],
    ],
  },
  {
    // However often it was dragged is the Go side's, see
    // TestAnEdgePutBackShowsItsWords. The walk holds the double-click.
    name: "a caption's right edge dragged in and double-clicked is back where it was with its words",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge put back", ["end", "Schulhof"]],
      ["same", "start"],
    ],
  },
  {
    // An edge pressed takes the playhead with it, so the edge's move is
    // taken back with Undo while the playhead stands on the caption, the
    // same change with no hand on the edge.
    name: "a caption block pops for neither edge dragged in, with the playhead on it",
    steps: [
      ["edge in", ["start", "Schulhof"]],
      ["block", "Schulhof"],
      ["count pops"],
      ["undo"],
      ["pops", 0],
      ["edge in", ["end", "Schulhof"]],
      ["block", "Schulhof"],
      ["count pops"],
      ["undo"],
      ["pops", 0],
    ],
  },
  {
    // Tim removed a word and saw the caption blocks come apart with the
    // sound running on under the gap, #597. When each caption shows is
    // the Go side's, see TestARemovedWordLeavesNoGapWhileTheSpeechRunsOn.
    // The walk holds the blocks meeting on the clip timeline.
    name: "removing a caption's first or last word leaves no gap to the caption beside while the speech runs on",
    steps: [
      ["mark", "start"],
      ["remove edge word", ["first", "Schulhof"]],
      ["meets", ["start", "Schulhof", "before"]],
      ["remove edge word", ["last", "Schulhof"]],
      ["meets", ["start", "Schulhof", "after"]],
    ],
  },
  {
    // A caption block clicked is picked, and delete removes it. Let go of
    // with Escape, or with the playhead moved a frame, delete removes
    // nothing, and with a word walked to, only the word. Each round clicks
    // a block of its own: two clicks on one block within the Mac's
    // double-click time are a double-click, which cuts a part out there.
    name: "a caption block clicked and let go of, or left for a word, is not removed by delete",
    steps: [
      ["block", "Schulhof"],
      ["press", "Escape"],
      ["press", "Delete"],
      ["blocks", 9],
      ["box", "als mich auf dem Schulhof irgendein"],
      ["block", "Typ"],
      ["press", "ArrowRight"],
      ["press", "Backspace"],
      ["blocks", 9],
      ["box", "Typ geschubst hat."],
      ["block", "Ich"],
      ["press", "Shift+ArrowRight"],
      ["press", "Backspace"],
      ["blocks", 9],
      ["box", "Ich vielleicht sechs Jahre alt,"],
    ],
  },
  {
    // Found by Tim on #201: a word opened in the caption box made the
    // whole caption grow, drawn by the browser, and shrink again when it
    // was left, and leaving it popped the word lit.
    name: "a word opened in the caption box stays where libass draws it",
    steps: [["open in place", "Jahre"]],
  },
  {
    // Found by Tim: in Archivo Black, a face of weight 400 that libass
    // thickens for bold captions, the word typed was narrower and lighter
    // than libass draws it.
    name: "a word typed in a face libass thickens is as heavy as libass draws it",
    steps: [["font", "Archivo Black"], ["frame", "Jahre"], ["grow", "Jahre"], ["thickened as libass", "Jahre"]],
  },
  {
    // Tim asked for the fonts installed on his Mac (#347). The bridge's
    // computer has Anton installed as Bridge Narrow, which the Font list
    // offers under Installed, and libass and the word typed are both drawn
    // from its copy.
    name: "a font installed on the computer draws the captions and the word typed",
    steps: [
      ["listed under", "Installed|Bridge Narrow"],
      ["search font", "brnar|Bridge Narrow"],
      ["frame", "Jahre"],
      ["grow", "Jahre"],
      ["thickened as libass", "Jahre"],
    ],
  },
  {
    // Found by Tim on #349: the Font list lined up with neither edge of
    // its field, its rows were squeezed to the height of their text, and
    // every name was in the app's face, so nothing showed what a font
    // looks like. A search sorted what it found by how well it answered,
    // so Archivo Black stood under the head of the installed fonts. The
    // Style list of one style lay over the Size field two pixels short of
    // its bottom, so its border showed twice. Nothing matches stood where
    // the names start, after the room of a tick that is never there, and
    // widened the list. A family is picked first and then its style, the
    // way Pages does, so the list is as wide as a family's name, and a
    // family picked comes in the style nearest the face it replaces.
    // Tim, 19 October: the font list open and the transcript button
    // pressed left both open, the keyboard in the font list's field, and
    // Escape closed the font list first, though it was opened first.
    name: "the font list hangs on the field's edge in each font's own face, searches in order, and a family picked keeps its look",
    steps: [
      ["font list"],
      ["search order", "b"],
      ["nothing found", "zzqx"],
      ["style list"],
      ["font then transcript"],
      ["font", "Bridge Sans"],
      ["style is", "Heavy"],
      ["style", "Regular"],
    ],
  },
  {
    // Found by Tim on #201: a word opened while another was lit was typed
    // on the dark box, and the lit one on its pill inside a lighter ring.
    // Every word typed sits on the one pill libass draws, lit or not. The
    // selection's blue reached over the pill, since a selection is as tall
    // as the face's line and the pill is not. Enter opens the framed word
    // wherever the playhead is. A word opened with Enter and left for
    // Report a problem kept its caret, and keys went on typing into it.
    name: "a word typed sits on the pill libass draws, its selection in it, and lets go of its caret for a button",
    steps: [
      ["one pill", "Jahre"],
      ["click", "Jahre"],
      ["selection in the pill"],
      ["press", "Escape"],
      ["press", "Enter"],
      ["open", "Jahre"],
      ["button", "Report a problem"],
      ["no caret"],
    ],
  },
  {
    // That it is the word put back, and not a word typed in, is the Go
    // side's, see TestAWordTypedBackGoesWhereItWasHeard.
    name: "a removed word typed back beside the word before it or after it is the word put back",
    steps: [
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "war"],
      ["press", "End"],
      ["type", " vielleicht"],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "sechs"],
      ["press", "Home"],
      ["type", "vielleicht "],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
    ],
  },
  {
    name: "a word typed in beside another is a word of its own, removed alone",
    steps: [
      ["click", "war"],
      ["press", "End"],
      ["type", " noch"],
      ["press", "Enter"],
      // A word more takes room, so the last word goes on to the next
      // caption. Adding may move words, removing may not.
      ["box", "Ich war noch vielleicht sechs Jahre"],
      ["click", "noch"],
      ["open", "noch"],
      ["press", "Escape"],
      ["press", "Backspace"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
    ],
  },
  {
    // #280: words typed into a word a frame or two long were lit on no
    // frame, so they were never seen lit. "Q" gets 12 ms of "war", so it
    // has no frame of its own, and it is lit on the frame it shares with
    // the word lit there, in one pill round both.
    name: "a word too short for a frame of its own is lit with the word on the frame it shares",
    steps: [
      ["click", "war"],
      ["press", "End"],
      ["type", " Q Schulhofgeschichten"],
      ["press", "Enter"],
      ["captions drawn"],
      ["lit together", "Q"],
      // A step back from the frame they share goes on to the word
      // before, never to the same frame again.
      ["steps back", 1],
    ],
  },
  {
    // #280: the walks' video says "so", "sich" and "wir" faster than a
    // frame, see quick in bridge_test.go, so every caption walk meets
    // words that share a frame. Which words share a frame, at the start,
    // in the middle and at the end of a caption, is the Go side's, see
    // TestWordsTooShortForAFrameAreLitTogether. This holds one on screen:
    // it is lit with the word beside it, and a step back goes on from
    // there.
    name: "the walks' video has words that share a frame",
    steps: [
      ["frame", "dabei"],
      ["lit together", "so"],
      ["steps back", 1],
    ],
  },
  {
    // Found by Tim on #201: several words typed into one and saved with
    // Enter left the picture with no captions, and corrections made
    // before were gone when he went back. A word typed into a short word
    // gets less than a frame of its time, and the engine left such a
    // word out of the captions. "Q" gets 12 ms of "war".
    name: "several words typed into one keep the captions in the picture and the corrections before",
    steps: [
      ["click", "sechs"],
      ["press", "End"],
      ["type", " eins zwei drei"],
      ["press", "Enter"],
      ["captions drawn"],
      ["click", "war"],
      ["press", "End"],
      ["type", " Q Schulhofgeschichten"],
      ["press", "Enter"],
      ["captions drawn"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Enter"],
      ["type", "sieben acht neun"],
      ["press", "Enter"],
      ["captions drawn"],
      ["press", "Shift+ArrowRight"],
      ["captions drawn"],
      ["press", "Shift+ArrowLeft"],
      ["press", "Shift+ArrowLeft"],
      ["captions drawn"],
    ],
  },
  {
    // Plan row 2.236, found by Tim: with the playhead on a caption after a
    // cut, the cut's edges moved made the caption block and the pill in
    // the video preview pop again and again. A word was known by when it
    // starts on the clip's clock, which the cut moves. Undo takes the cut
    // back with the playhead standing on the caption, which is the same
    // change with no hand on an edge.
    name: "a cut changed before the caption under the playhead pops nothing",
    steps: [
      ["cut at", 0.4],
      ["block after", 1],
      ["drag edge", "cut from", -40],
      ["block after", 1],
      ["count pops"],
      ["undo"],
      ["pops", 0],
      ["pill pops", 0],
    ],
  },
  {
    // Found in a review: a caption block kept the press to itself, so a
    // double-click on one, across the middle of the clip, cut nothing.
    name: "a double-click on a caption cuts a part out there",
    steps: [
      ["cut on caption", 1],
      ["cuts", 1],
    ],
  },
  {
    // Found by Tim on #201: the frame round a word in the caption box
    // stood off the word, since the browser laid the words out in its own
    // font over the words libass drew.
    name: "the frame round a word in the caption box is the pill libass draws",
    steps: [["frames on the pills", 5]],
  },
  {
    // Found by Tim on #201: while a clip played, the frame round a word
    // went on to the first word of the next caption a few frames before
    // the pill did, since the caption box followed the playhead and the
    // pill the frame drawn.
    name: "the frame round a word stays on the pill while the clip plays",
    steps: [["frame on the pill while it plays", 3]],
  },
  {
    // Found by Tim on #201: Shift and an arrow put the keyboard's frame
    // on the next word a frame before the pill came, while the picture's
    // captions were on their way.
    name: "the keyboard's frame goes from word to word with the pill",
    steps: [
      ["layers late", 120],
      ["walk the words with the frame on the pill", 8],
    ],
  },
  {
    // Found by Tim on #201: a step onto a word left it standing still,
    // where the browser's captions before had popped it, and the word and
    // its pill did not grow together.
    // Filmed at 29.97 fps, as a camera records: at the 5 fps of the
    // other walks' videos the pop is over before the next frame.
    // At 25 fps, the rate of Tim's videos, forward and back again to the
    // clip's first word: Tim saw some words pop fully, some barely, some
    // rise and snap back, and the first word not at all.
    // Then Tim saw the pop the short burns in played again over every
    // frame he stepped to with an arrow alone, two animations mixed into
    // one: a step from frame to frame shows the short's frames and nothing
    // over them, and only a step from word to word bounces.
    name: "a step onto a word pops it over the paused frame",
    steps: [
      ["added", 40, "25"],
      ["step onto words and see them pop", 3],
      ["step frame by frame and see nothing pop", 40],
    ],
  },
];
