// The sequences about the caption box and the captions drawn over the video
// preview: words, blocks, pills, fonts and styles. They are cases found by
// hand, kept so they stay fixed. Their steps are said in steps.mjs beside
// this file, and in common/ for the steps of every folder. See
// ../sequences.mjs.
export default [
  {
    name: "delete removes the framed word and nothing else",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["box", "Ich war sechs Jahre alt,"],
      ["spans", "start"],
    ],
  },
  {
    // Found by Tim on #201: a caption whose first word the picture lights
    // for a single frame showed nothing when its block was clicked, and
    // its block did not pop, while shift and an arrow went to that frame.
    // The clip's start dragged near the end of the first word lands it on
    // the frame before "war", which starts in the middle of the clip's
    // first frame, so the first frame shows its caption with no word lit
    // and a click a frame into "war" by its own time landed there.
    name: "every caption block clicked lights a word of its caption",
    steps: [["trim into", 0.04], ["every block"]],
  },
  {
    name: "delete removes the caption block clicked on the clip timeline",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["blocks"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["blocks"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    // Tim removed a caption block, removed its clip with the bin, and
    // while its row still offered it back pressed I where the playhead
    // was. The clip came back, and the caption he had removed was still
    // gone from it: a removed word was kept for the whole video, so every
    // clip over it lost it. Now it is the clip's, and goes with it, and the
    // clip put back has it removed as it was. Plan row 2.249.
    name: "a caption removed, then its clip removed and made again with I, has the caption again",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["blocks"],
      ["remove clip"],
      ["make here", "i"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    name: "a caption removed, then its clip removed and put back, is still removed",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["mark", "removed"],
      ["remove clip"],
      ["put back"],
      ["same", "removed"],
      ["gone", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // A word removed in the caption box is the clip's too, and goes with
    // it.
    name: "a word removed, then its clip removed and made again with I, has the word again",
    steps: [
      ["frame", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["remove clip"],
      ["make here", "i"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // Plan row 2.249, Tim chose: a spelling fix is the clip's, and goes
    // with it.
    name: "a word corrected, then its clip removed and made again with I, reads as it was heard",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["remove clip"],
      ["make here", "i"],
      ["gone", "Schulhoff"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // A drag sets when a caption is shown: the word is left out while the
    // edge stands past it, and is still there, #598.
    name: "a caption's right edge dragged in over its last word leaves the word out, and back out puts it back",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["gone last", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["undo"],
      ["same", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge out", ["end", "Schulhof"]],
      ["fewer", "start", 0],
      ["others", ["start", "Schulhof"]],
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
      ["mark", "start"],
      ["edge to word", ["start", "Schulhof"]],
      ["drawn", "Schulhof"],
      ["others", ["start", "Schulhof"]],
      ["edge to word", ["end", "Schulhof"]],
      ["drawn", "Schulhof"],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "a caption's left edge dragged in over its first word leaves the word out and the caption before it alone",
    steps: [
      ["mark", "start"],
      ["edge in", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["gone first", ["start", "Schulhof"]],
      ["edge out", ["start", "Schulhof"]],
      ["fewer", "start", 0],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    // A word removed in the caption box is fixed, like a typo, and a drag
    // only sets when the caption is shown. Tim removed a word, dragged the
    // start of the caption after it out to where he heard its next word
    // begin, and the word came back, #598.
    name: "a caption's first word removed stays removed when its left edge is dragged out over it",
    steps: [
      ["mark", "start"],
      ["remove edge word", ["first", "Schulhof"]],
      ["fewer", "start", 1],
      ["edge out", ["start", "Schulhof"]],
      ["fewer", "start", 1],
      ["meets", ["start", "Schulhof", "before"]],
    ],
  },
  {
    name: "a caption's right edge dragged in and double-clicked, twice, is back where it was with its words",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge put back", ["end", "Schulhof"]],
      ["same", "start"],
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
    name: "a caption's edges moved by hand leave the captions beside them alone",
    steps: [
      ["mark", "start"],
      ["edge nudge", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["edge nudge", ["end", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    // On the caption that ends a sentence, so a pause comes after its last
    // word. Where the speech runs on, the caption stays up until the next
    // appears, see the walk below on the last word, #597.
    name: "removing a caption's last word draws it in like dragging its right edge over the word",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "geschubst"]],
      ["mark", "dragged"],
      ["undo"],
      ["same", "start"],
      ["remove edge word", ["last", "geschubst"]],
      ["drawn in", ["dragged", "geschubst", "last"]],
      ["others", ["start", "geschubst"]],
    ],
  },
  {
    // The speech runs on into "als", so the caption before stays up
    // until this one appears, #597, which the edge dragged in leaves
    // alone.
    name: "removing a caption's first word draws it in like dragging its left edge over the word",
    steps: [
      ["mark", "start"],
      ["edge in", ["start", "Schulhof"]],
      ["mark", "dragged"],
      ["undo"],
      ["same", "start"],
      ["remove edge word", ["first", "Schulhof"]],
      ["drawn in", ["dragged", "Schulhof", "first"]],
      ["others", ["start", "Schulhof", "held"]],
    ],
  },
  {
    // Tim removed a word and saw the caption blocks come apart with the
    // sound running on under the gap, #597.
    name: "removing a caption's first word leaves no gap to the caption before while the speech runs on",
    steps: [
      ["mark", "start"],
      ["meets", ["start", "Schulhof", "before"]],
      ["remove edge word", ["first", "Schulhof"]],
      ["meets", ["start", "Schulhof", "before"]],
    ],
  },
  {
    name: "removing a caption's last word leaves no gap to the caption after while the speech runs on",
    steps: [
      ["mark", "start"],
      ["meets", ["start", "Schulhof", "after"]],
      ["remove edge word", ["last", "Schulhof"]],
      ["meets", ["start", "Schulhof", "after"]],
    ],
  },
  {
    name: "a caption block clicked, then a word walked to, delete removes only the word",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Shift+ArrowRight"],
      ["press", "Backspace"],
      ["fewer", "start", 1],
      ["blocks"],
    ],
  },
  {
    name: "a caption block clicked, then the playhead moved a frame, delete removes nothing",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "ArrowRight"],
      ["press", "Backspace"],
      ["same", "start"],
      ["blocks"],
    ],
  },
  {
    name: "a caption block clicked and then let go of is not removed by delete",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Escape"],
      ["press", "Delete"],
      ["same", "start"],
      ["blocks"],
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
    // Found by Tim on #201: a word typed longer ran out of its pill,
    // because libass drew the pill at the word's old size, and opening a
    // word on a frame of its pop is to move nothing.
    name: "a word typed longer grows its pill round its middle",
    steps: [["frame", "Jahre"], ["grow", "Jahre"]],
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
    // looks like.
    name: "the font list hangs on the field's edge with rows as tall as it, each in its own face",
    steps: [["font list"]],
  },
  {
    // Found by Tim on #349: a search sorted what it found by how well it
    // answered, so Archivo Black stood under the head of the installed
    // fonts.
    name: "a search of the fonts keeps the order and the heads of the list",
    steps: [["search order", "b"]],
  },
  {
    // Tim, 19 October: the font list open and the transcript button
    // pressed left both open, the keyboard in the font list's field, and
    // Escape closed the font list first, though it was opened first.
    name: "the transcript button pressed with the font list open closes the list and takes the keyboard",
    steps: [["font then transcript"]],
  },
  {
    // Found by Tim on #349: the Style list of one style lay over the Size
    // field two pixels short of its bottom, so its border showed twice.
    name: "a list of one style covers the field under it",
    steps: [["style list"]],
  },
  {
    // Found by Tim on #349: Nothing matches stood where the names start,
    // after the room of a tick that is never there, and widened the list.
    name: "a search that finds nothing says so in the middle of a narrow list",
    steps: [["nothing found", "zzqx"]],
  },
  {
    // Found by Tim on #349: a family is picked first and then its style,
    // the way Pages does, so the list is as wide as a family's name. A
    // family picked comes in the style nearest the face it replaces.
    name: "a family picked keeps the look of the face it replaces",
    steps: [
      ["font", "Bridge Sans"],
      ["style is", "Heavy"],
      ["style", "Regular"],
    ],
  },
  {
    // Found by Tim on #201: a word opened while another was lit was typed
    // on the dark box, and the lit one on its pill inside a lighter ring.
    // Every word typed sits on the one pill libass draws, lit or not.
    name: "a word typed sits on the pill libass draws, lit or not",
    steps: [["one pill", "Jahre"]],
  },
  {
    // Found by Tim: the selection's blue reached over the pill, since a
    // selection is as tall as the face's line and the pill is not.
    name: "the letters selected in a word typed stay in its pill",
    steps: [["click", "Jahre"], ["selection in the pill"]],
  },
  {
    // Found by Tim: a word opened with Enter and left for Report a problem
    // kept its caret, and keys went on typing into it.
    name: "a word left for a button lets go of its caret",
    steps: [["click", "Jahre"], ["button", "Report a problem"], ["no caret"]],
  },
  {
    name: "a word saved empty is removed",
    steps: [
      ["click", "Jahre"],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs alt,"],
    ],
  },
  {
    name: "undo puts a removed word back and redo takes it again",
    steps: [
      ["mark", "start"],
      ["frame", "sechs"],
      ["press", "Delete"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    name: "a removed word typed back after the word before it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "war"],
      ["press", "End"],
      ["type", " vielleicht"],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a removed word typed back before the word after it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "sechs"],
      ["press", "Home"],
      ["type", "vielleicht "],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a word typed in beside another is a word of its own, removed alone",
    steps: [
      ["mark", "start"],
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
      ["same", "start"],
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
      ["holds", "drei"],
      ["click", "war"],
      ["press", "End"],
      ["type", " Q Schulhofgeschichten"],
      ["press", "Enter"],
      ["captions drawn"],
      ["holds", "Q"],
      ["holds", "Schulhofgeschichten"],
      ["holds", "drei"],
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
      ["holds", "neun"],
      ["holds", "Q"],
      ["holds", "drei"],
    ],
  },
  {
    name: "Enter opens the framed word, wherever the playhead is",
    steps: [
      ["click", "Jahre"],
      ["press", "Escape"],
      ["press", "Enter"],
      ["open", "Jahre"],
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
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["step onto words and see them pop", 6],
      ["step frame by frame and see nothing pop", 40],
    ],
  },
  {
    name: "removing a word in the middle of a caption leaves the caption whole",
    steps: [
      ["mark", "start"],
      ["frame", "auf"],
      ["press", "Backspace"],
      ["box", "als mich dem Schulhof irgendein"],
      ["spans", "start"],
    ],
  },
];
