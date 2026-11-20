# Go answers the transcript search, over the cards it sent last

Issue #927.

## The problem

The transcript search in the clip column matched what was typed against
every card in the window, with its own rule for slips and for which
words go bold, `found` and `near` in `frontend/src/lib/match.ts`, run by
`SaidList.svelte` over every card at every letter. `docs/ARCHITECTURE.md`
says `frontend/src/lib/` never holds a rule of the engine. The issue
asks Go to answer: the keys of the cards that answer and the words to
set in bold, without making typing slower, see #894.

Measured on the code as it is, in a cloud session, a video of four
hours, 3000 cards, from `longVideo` in `engine/transcript_bench_test.go`:

- Making the cards, `engine.SaidCards`, takes 12 ms. `Sentences` makes
  them every time it is asked, which the window does only when Go's
  stamp of them, `VideoState.Cards`, changes.
- Matching one letter in Go over cards made ready once takes 0.2 to
  1.4 ms, see `BenchmarkTheTranscriptSearch`. Making them ready, the
  text in small letters and its words, takes 6 ms.

So a letter cannot make the cards again, and the cards have to be ready
before the letter comes.

## Five simulations

### 1. Who may know about whom

The window knows the cards and shows what it is told. Go knows the rule.
`SearchSaid(path, typed)` makes the cards as `Sentences` does and
matches them. Nothing new is kept anywhere. The names across the
boundary: one call and one answer type, `CardFound`. Cost: 12 ms of
cards and 6 ms of readying at every letter of a long video, where the
window took a few. Slower, so it is no design on its own, and every
other simulation starts from the cards being kept somewhere.

### 2. Where state lives

The cards are Go's, made by `SaidCards`, and the window keeps the copy
it was sent, as it keeps the clips, see the row of the clips the window
was last sent. Go keeps the cards it sent last too, made ready once, in
the app's Go side beside `Sentences`, `saidSent` in
`cmd/framefairy-app/words.go`. The window shows one video at a time, so
one video's cards are kept. Names across the boundary: the call and
`CardFound`. Cost: one small struct, one call, one pull request.

### 3. What runs together

A letter shows in the frame after it is typed, and the round trip across
the bridge is short beside a frame, but it is not nothing. So the window
keeps the answer it has until the next comes, and never empties the
list in between, and asks again at every letter. The engine's Episode
could keep the cards with the reads they came from, the way
`PlanSummary.Said` keeps a plan's captions, and the search would read
them there. But the cards depend on the transcript, on every plan and on
the look of each, and the Episode would need a key of all of that, which
`Episode.SaidStamp` already works out from the files, a probe of the
video included. Keeping the copy at the point it is sent needs no key.

### 4. What changes together

An answer names cards, and it has to name cards the window holds. The
window reads the cards again whenever the stamp changes, and asks the
search again then. If Go searched cards newer than the window's, an
answer could name a card the window lacks. Kept where `Sentences` sends
them, the cards searched are the cards sent, and a read that comes back
after a later one never takes its place, by a count of the reads
started. The window passing the stamp with each letter, for Go to make
the cards again when it differs, needs the probe the stamp needs on
every letter. Dropped.

### 5. What a test has to build

The rule is a function from cards and what is typed to an answer,
`engine.SaidSearch`, tested with `go test` alone, the cases of the
Vitest tests of `found` and `near` moved over. The copy of what was
sent is tested on the app with the caption editor's video: a word
corrected since the cards were sent is found as it was until they are
read again. The walks that type into the search stay as they are, and
reach the real Go side through the bridge.

## What converges

Every simulation but 1 keeps the cards ready before a letter comes, and
2, 3 and 4 keep them where they are sent, the copy of what the window
holds. Kept from 3: the window keeps an answer until the next one comes.
Kept from 4: the count of reads, so an old read never wins. Kept from 5:
the rule as one function of the engine, tested alone. Dropped: making
the cards at every letter, and a key of the cards in the engine.

What the window still decides is only whether anything is typed, which
is whether there is a question at all: with nothing typed the list is
every card, as it was before, with no call.

## The decision

```
interface   SaidList.svelte shows the cards Go names, bold where it says
               |  SearchSaid(path, typed) -> CardFound[]
app Go      saidSent: the cards Sentences sent last, one video, made ready
               |  NewSaidSearch(cards), Answer(typed)
engine      SaidSearch: the rule, words in small letters, slips, bold
```

Each uses only the one under it. The drawings in `docs/diagrams/` keep
their boxes: `words` is already among the files of a video in
`app-go.svg`, and the engine draws no file.

## What follows

- A letter costs 0.2 to 1.4 ms in Go on four hours of cards, and the
  first letter after the cards are read 6 ms more. The window's rows are
  built from the answer, no longer from a match of every card.
- The cards of one video are kept in memory twice, in the window and in
  Go, about 1 MB for four hours.
- It was wrong if a letter typed shows later than before, or if an
  answer names a card the window does not hold.
