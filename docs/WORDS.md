# Words

What an episode says, and how every part of the program gets at it. One
model, one place to look, no copies.

## Why

Before this, the words of an episode lived in six places, and each kept
its own version of the rules:

| where | what it kept |
| --- | --- |
| the transcript as it is read | the recogniser's words, moved onto the sound, with the corrections laid over them, but only in the app: the command line read them uncorrected |
| every clip in a plan | a copy of its words, taken when the clip was found and refreshed on an edit, so a rule that changed reached a clip only once it was edited |
| the captions | a correction that reads as two words split in two, and a word too wide for a line hyphenated, neither of which the rest of the program knew about |
| the caption files next to a short | a file that, once written, won over everything else, from before corrections were made in the caption box |
| the interface | its own copy of the engine's snapping rules, so an edge could be drawn where the engine would save it |
| the preview harness | another copy of those rules |

Every bug of the day this was written was two of these disagreeing: a
word the clip said with no caption, a shift drag that skipped the second
half of a hyphenated word, a clip whose words had to be edited once
before a fix reached them.

## The model

There are two levels, and each is worked out in one function.

**What is said**, `Transcript.Words`. The recogniser's words, moved onto
the sound (`SnapWords`), with the corrections applied, and a correction
that reads as two words split into two words that share the time the
recogniser measured. Everything that reads the episode reads these: the
prompt, the room a window has, the clip timeline, the edits. The stored
transcript keeps the recogniser's own words, and the corrections stay in
`corrections.json`, so nothing the recogniser heard is lost.

**What is shown**, `Shown`. The words said, the way a clip's captions show
them: a word too wide for a line is its two hyphenated halves, each with
its share of the word's time. It depends on the caption style, so it is
worked out for a clip, from the clip's style and the episode's language.
The captions, the highlight, the caption blocks, the snapping of an edge
or a cut and the words the arrow keys walk are all made from it.

**A clip's words** are not stored. A clip is its pieces, and its words are
the words said that its pieces hold, `Said`: a word is in the clip
while the clip holds more than a frame of its sound. Plans no longer carry
a `words` list, and one that does is not read.

## Who does what

- The engine works out every word and every edge. A gesture is what a
  hand does to a clip on the clip timeline: an edge trimmed, a part taken
  out, a cut moved, put back or taken out again. `ShapeClip` in
  `engine/shape.go` answers while the hand moves: where the edges land, on
  a frame or on the words shown, how far they may go, where the playhead
  stands and which pieces the clip is left with, and `ShapeClipView` adds
  the captions. `Reshape` saves the same gesture through the same
  functions, so what is drawn is what is saved.
- What a gesture puts back, a cut or an edge, comes back framed by the
  shots it shows, from the pieces the clip was found with, so a camera
  switch in it stays where it was and each shot keeps its crop, see
  [ENGINE.md](ENGINE.md).
- A gesture is held inside what the clip can be rather than refused on the
  way: a cut is never narrower than the least a cut may be, a moved cut
  stops short of swallowing the piece beside it, a clip never gets shorter
  than a second. The timeline used to hold the hand itself, with its own
  copy of these rules, and the engine refused what got past it.
- The interface sends where the hand is and draws what comes back, through
  the app's `Shape` and `Reshape`. It keeps no rules about where an edge
  lands. What it still does on its own is walk the playhead from word to
  word with shift and the arrow keys, over the words the captions light,
  which is navigation and not an edit.
- Captions are always made from the words, by `ClipCaptions`, for the app
  and the render alike. The caption files written next to a short are
  output, never read back. A word is corrected in the caption box, and each
  word in a caption says which word of the episode it stands for, so the
  box needs no rule of its own to find it.
- The harness answers the same calls with a small stand-in, in
  `frontend/preview/wails-stub.ts`, and says there that it is one. What
  the gestures really do is proved by the Go tests.

## What went away

- In plans: the `words` list of every clip. One that is still there is not
  read.
- In the engine: `Clip.Words`, `refreshWords`, `SplitCorrected` and
  `ApplyCorrections` outside the transcript, `WordStops`,
  `DraftCaptionsView`, `TrimClip`, `CutClip`, `MoveCut` and `JoinCut`,
  reading caption files back with `LoadCaptions`, `AlignWords`, the words
  file and the srt reader, and setting caption files aside when a plan
  changes.
- In the interface: `snapStart`, `snapEnd`, `snapCut`, `cutAt`,
  `saidWord` and the tests that kept them in step with the engine by hand.
- On the command line: `--refresh-captions` does nothing and is kept so a
  script that passes it still runs.

No migration. Plans and training records from before are not carried
over: a plan's `words` list is ignored, and training starts afresh.
