# Go answers the transcript search, over cards its Episode keeps ready

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
it was sent, as it keeps the clips. Go keeps the cards it sent last too,
made ready once, in the app's Go side beside `Sentences`, a struct of its
own with a count of the reads so an old read never wins. Names across
the boundary: the call and `CardFound`. But Where does it go in
`docs/ARCHITECTURE.md` says what a video's files said is kept by its
`engine.Episode`, by the files' stamp, never in a cache of the app's
own, and the review of this change found this first version doing just
that. Dropped for the next.

### 3. What runs together, the Episode keeping them

A letter shows in the frame after it is typed, and the round trip across
the bridge is short beside a frame, but it is not nothing. So the window
keeps the answer it has until the next comes, and never empties the
list in between, and asks again at every letter. The cards are worked
out of the transcript's read and the plans' reads, and the Episode is
where what is worked out of a read is kept, the way `PlanSummary.Said`
keeps a plan's captions. Their key is `Episode.SaidStamp`, which the
video's state already works out up to four times a second, `VideoState.Cards`.
`Episode.SaidSearch(stamp, cards)` gives the cards of that stamp made
ready, and makes them again only when the stamp changed. `Sentences`
keeps the cards it just made with the stamp it took before making them,
`Episode.KeepCards`, so the first letter after a read makes nothing.
Measured on the caption editor's video in the app's tests, memory disk:
the stamp 0.02 ms, a letter's whole answer 0.01 ms more. Names across
the boundary: the call and `CardFound`, and on the Episode two methods.

### 4. What changes together

An answer names cards, and it has to name cards the window holds. The
window reads the cards again whenever the stamp in the video's state
changes, and asks the search again then. Searched by the stamp of now,
an answer can name a card a moment newer than the window's, which the
window leaves out until its read of the new cards comes and it asks
again, so it heals in one round trip, the same way it heals a new card
the window has before the answer. Kept where `Sentences` sends them,
version 2, the same holds, and a search before any read makes cards the
window never got. Neither is exact, and the stamp is the simpler of the
two.

### 5. What a test has to build

The rule is a function from cards and what is typed to an answer,
`engine.SaidSearch`, tested with `go test` alone, the cases of the
Vitest tests of `found` and `near` moved over. The copy of what was
sent is tested on the app with the caption editor's video: a word
corrected since the cards were sent is found as it was until they are
read again. The walks that type into the search stay as they are, and
reach the real Go side through the bridge.

## What converges

Every simulation but 1 keeps the cards ready before a letter comes.
Kept from 3: the Episode keeps them by `SaidStamp`, the home of what is
worked out of a read, and the window keeps an answer until the next one
comes. Kept from 4: an answer over cards a moment newer heals when the
window reads them. Kept from 5: the rule as one function of the engine,
tested alone. Dropped: making the cards at every letter, and a cache of
the app's own, version 2, which this pull request first built.

What the window still decides is only whether anything is typed, which
is whether there is a question at all: with nothing typed the list is
every card, as it was before, with no call.

## The decision

```
interface   SaidList.svelte shows the cards Go names, bold where it says
               |  SearchSaid(path, typed) -> CardFound[]
app Go      SearchSaid: the stamp of the cards, Sentences to make them
               |  Episode.SaidSearch(stamp, cards), KeepCards, Answer(typed)
engine      Episode keeps them by stamp. SaidSearch is the rule
```

Each uses only the one under it. The drawings in `docs/diagrams/` keep
their boxes: `words` is already among the files of a video in
`app-go.svg`, and the engine draws no file.

## What follows

- A letter costs 0.2 to 1.4 ms in Go on four hours of cards, and the
  stamp of the cards, and the first letter after the cards changed 6 ms
  more. The window's rows are
  built from the answer, no longer from a match of every card.
- The cards of each video searched are kept in memory twice, in the
  window and in its Episode, about 1 MB for four hours, until the video
  leaves the library.
- It was wrong if a letter typed shows later than before, or if an
  answer names a card the window does not hold.
