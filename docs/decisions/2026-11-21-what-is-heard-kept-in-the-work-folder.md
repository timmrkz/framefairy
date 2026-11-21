# What a video's transcript has heard is kept in its work folder

Issue #953.

## The problem

The issue, from the review of the back end's algorithms on 20 November:
"At every start, the app reads and parses the whole transcript of every
video in its list, to learn only what is heard and where speech starts
and ends. The cost grows with the list."

Read on main, 21 November, at dcc4d0e:

- As the app starts, the library asks `Episode.Status` of every video.
  `Status` asks `Episode.heardSoFar` in `engine/held.go` for three
  things: whether the transcript is done, the parts heard, and from the
  start of the first line to the end of the last, `said`.
- `heardSoFar` keeps its answer on the video's `Episode`, by the
  transcript's stamp, `transcriptStamp`: the video, the speech model,
  the floor asked for, and the size and time of `words.json` and
  `words.frames`. It dies with the program. So each start reads both
  files whole, decodes every word, snaps every word to the sound and
  builds the lines, `readSaved` and `BuildLines`, for those few numbers.
- Timed in the cloud session, `BenchmarkAStartOfTheApp`, a new
  `Machine` each run, 6 runs: 9.0 ms and 7.5 MB for a video of 1 hour,
  17.0 ms and 14.4 MB for 2 hours, 30.7 ms and 29.3 MB for 4 hours. A
  library of ten 2 hour videos reads 170 ms and 144 MB before it can
  list them. The profile has no one place to make cheaper: decoding the
  words is a quarter, snapping them a fifth, the loudness a tenth.
- Done and the parts heard are in the head of `words.json` already,
  `Parts` and `End`. `said` is not. It comes from the words as they are
  snapped, and the snapping runs on every read, on purpose, so a better
  rule never needs a new transcription. It moves with the floor, which
  the settings choose, and with the build, whose rules snap.

## Five simulations

### 1. Where state lives

What a transcript has heard is a fact about the transcript's files, the
way the probe is a fact about the video's file, and both live in the
work folder. So it does too: `logs/heard.json`, beside `words.json`,
with the stamp it was read at. One file per video, written by the
engine, read by an older or a newer build alike, versioned. The issue
asked for the head of `words.json` instead. Done and the parts are
there, but `said` written by `addHeard` would be said by the build that
heard the words and at the floor of that moment, and the next build, or
another floor, would read a `said` its own rules never made. And a
head is still at the front of a file of megabytes: reading only it
needs a reader of its own that stops before the words. Home: one small
file per video. Writer: the engine.

### 2. What runs together

Nothing needs to be kept if nothing waits for it. The library could
list each video without what it heard and read the transcripts after,
one by one. But the sidebar's row and the range picker show what is
heard, and a video still being heard says how far it got, so every
start would show each video as not heard and fill it in later, which is
the cost moved and a row that changes under the hand. Dropped. What
comes back: the answer has to be there when the library asks.

### 3. Who may know about whom

The app could keep what each video heard in its library, in the config
folder, since the app is the one that starts. But the library would
then hold a second copy of a fact the work folder holds, which Where
state lives forbids, and a transcript written by the command line or
copied in by hand would leave it wrong until a rule said when to look
again. `heardSoFar` already decides when the transcript has changed, by
its stamp, and nobody above it knows where its answer came from. So the
kept file is read and written there, inside the read `held` makes once
for each stamp, and `Status` and `Heard` change nothing.

### 4. What changes together

What is heard changes when the transcript's files change, when the
floor asked for changes, and when the rules that snap the words and
build the lines change. The first two are the transcript's stamp
already. The rules change with the build. So the file keeps the stamp
and the build's revision, `buildStamp`, the one the kept probe keeps,
and a new build reads every transcript once again. A version number
raised by hand when the rules change would let the first start after an
update skip that read, but a rule changed without raising it would keep
a wrong `said` for good, and nothing would show it. The file is written
by the read that had to read the transcript itself: the first start, a
transcript heard further and then read, a new build. A read taken from
the transcript a workspace holds writes nothing, so hearing, which
saves every few seconds, writes no second file each time.

### 5. What a test has to build

A test needs a heard transcript in memory, which `longVideo` in the
benchmarks makes, and a second `Machine`, which is a second start. That
the second start reads no word is shown by a disk on which the files of
the transcript cannot be opened, only found, so their stamps still
match. It compares what the two starts say. A kept file from another
stamp, or another floor, is read past. The file is outside bytes,
anyone can write a work folder, so its reader has a fuzz target that
holds every answer it takes to what a transcript can say: parts in
order and apart, times inside the longest video there can be, lines
that end after they start.

## What converges

Every simulation but 2 keeps the answer beside the transcript, below
`Status`, by the transcript's stamp. 1 and 4 put it in its own file in
the work folder, not the head of `words.json`, because `said` depends
on the build and the floor and the words file on neither. 3 puts the
reading and the writing inside `heardSoFar`. 4 adds the build to the
stamp and writes only after a read of the transcript. 5 adds the fuzz
target. Dropped from 2: listing the library before it is known, which
moves the wait. Dropped from 1: the issue's head of `words.json`.

## The decision

```
Episode.Status
  Episode.heardSoFar, held on the Episode by the transcript's stamp
    the transcript a workspace holds, when it is held for that stamp
    logs/heard.json, by that stamp and the build                 <- new
    readSaved, the whole transcript, and the answer kept in the file
```

- `logs/heard.json` holds a version, the stamp, done, the parts heard
  and `said`. It is written only into the logs folder the transcript
  was read from, in one step, and not flushed: one lost costs one read
  of the transcript at the next start.
- It is read only when the version and the stamp match, and the parts
  and `said` are ones a transcript can have. Anything else is as if it
  were not there.
- `docs/ARCHITECTURE.md` gains the row in Where state lives.

## What follows

- A start of the app reads a few hundred bytes of each video it read
  before, about 0.02 ms, however long the video. Timed in the cloud
  session, see the pull request.
- An app update, a new build, reads every transcript once again, so the
  first start after each update is as slow as before, and writes each
  file again.
- A write into `logs` is heard by the app's watch on the disk, so the
  first start looks at each video once more. That look finds the answer
  held by the same stamp and reads nothing.
- What would show it wrong: a start that says what a read of the whole
  transcript would not, the parts, done or `said`. The test compares
  the two.
