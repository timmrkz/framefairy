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
the words shown that its pieces hold, `ClipWords`: a word is in the clip
while the clip holds more than a frame of its sound. Plans no longer carry
a `words` list, and one that does is not read.

## Who does what

- The engine works out every word and every edge. Snapping an edge or a
  cut to words, where the playhead goes while an edge is dragged, the
  captions of a clip as it is being dragged: one call, `Shape`, answers
  all of it for a drag, and the edit that saves it uses the same
  functions, so what is drawn is what is saved.
- The interface sends where the hand is and draws what comes back. It
  keeps no rules about words.
- Captions are always made from the words. The caption files written next
  to a short are output, never read back. A word is corrected in the
  caption box.
- The harness answers the same calls with a small stand-in, and says
  where it is one.

## Batches

1. This plan.
2. The words said: corrections applied and split in `Transcript.Words`,
   for every reader.
3. A clip's words worked out, never stored, and the caption files no
   longer read back.
4. The words shown, and snapping on them in the engine, with `Shape`.
5. The interface draws what `Shape` answers and keeps no snapping rules.
6. Docs and the last of what went away.

No migration. Plans and training records from before are not carried
over: a plan's `words` list is ignored, and training starts afresh.
