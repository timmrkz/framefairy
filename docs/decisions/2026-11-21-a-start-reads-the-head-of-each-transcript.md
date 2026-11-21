# A start reads the head of each transcript

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
  start of the first line to the end of the last, `said`, from which
  `Speechless` follows.
- `heardSoFar` keeps its answer on the video's `Episode`, by the
  transcript's stamp, and the answer dies with the program. So each
  start reads `words.json` and `words.frames` whole, decodes every word,
  snaps every word to the sound and builds the lines, for those few
  numbers.
- Timed in the cloud session, `BenchmarkAStartOfTheApp`, a new
  `Machine` each run, 6 runs: 9.0 ms and 7.5 MB for a video of 1 hour,
  17.0 ms and 14.4 MB for 2 hours, 30.7 ms and 29.3 MB for 4 hours. The
  profile has no one place to make cheaper: decoding the words is a
  quarter, snapping them a fifth, the loudness a tenth.
- Done and the parts heard are in the head of `words.json`, `Parts` and
  `End`, or `From`, `To`, `Partial` and `Resume` in older versions,
  before the words, which `encodeTranscript` always writes last. `said`
  is not. It comes from the words as they are snapped, and the snapping
  runs on every read, on purpose, so a better rule never needs a new
  transcription. It moves with the floor and with the build.
- Who reads `said` and `Speechless`: only `State` in the app, for the
  video the workspace opens, `MarkingOf` in `videostate.go` and the clip
  list's note in `Episode.svelte`. `State` reads the whole transcript
  before it asks `Status`, `heardIn`, and holds it. The sidebar and the
  range picker read done and the parts heard.

## Five simulations

### 1. Where state lives

What a transcript has heard is a fact about the transcript's files, and
the files already hold most of it. A kept summary, `logs/heard.json`
beside `words.json`, by the transcript's stamp and the build, the way
`probe.json` is kept, would answer a start in a few hundred bytes. But
it is a second copy of what the files say, and two things keep it true:
the stamp, and the build, whose rules snap the words. The build is not
there to read in a build that ships, `make` builds with
`-buildvcs=false`, so the copy would never expire on an update. And
writing it into the work folder wakes the app's watch on the disk, which
then reads the state of every video, each transcript whole. Home: a new
file per video. A second copy, which Where state lives warns of.

### 2. What runs together

What a start waits for is the sidebar and the range picker, and they
need done and the parts heard, which the head of the file says. `said`
and `Speechless` are needed only once a video is opened, and opening it
reads the whole transcript anyway, `State`. So the start reads only the
heads, and the lines are worked out where the transcript is read whole.
Nothing new is kept anywhere. What it costs: `Status` says `said` only
of a transcript the episode holds, and the answer `heardSoFar` keeps for
a stamp must not stand in for one with `said` once the transcript is
held, so the held transcript is asked first.

### 3. Who may know about whom

The app could keep what each video heard in its library, in the config
folder. But that is a second copy of a fact the work folder holds, and a
transcript written by the command line or copied in by hand would leave
it wrong. `heardSoFar` already decides when the transcript changed, by
its stamp, and nobody above it knows where its answer came from. So
whatever reads less is read there, and `Status` and `Heard` change
nothing about how they are asked.

### 4. What changes together

Done and the parts heard change when the transcript's files change, and
by the rules that read them, `heard`, `done` and the cut to what the
loudness reaches, which run on every read. Reading the head by the same
rules as the whole file keeps the two from drifting: both go through one
`transcriptFile.fit`. `said` changes with the snapping and the floor,
so it stays where the words are snapped, never stored. The issue asked
for `said` in the head of `words.json`, written by `addHeard`: that
would be said by the build that heard the words, at the floor of that
moment, and the next build would read a `said` its rules never made.

### 5. What a test has to build

A test needs a heard transcript in memory, which `longVideo` makes, and
a disk that counts the bytes read of each file, to show a start reads a
few hundred bytes of the words and none of the loudness. It compares
what the start says with a whole read, and checks that an opened video
says where its lines are after the library asked first. The head of the
file is outside bytes, read by a reader of its own, so it has a fuzz
target: whatever the bytes, no panic, and a file as the program writes
it says through its head what it says read whole.

## What converges

2, 3, 4 and 5 read only what a start needs from the files that hold it,
inside `heardSoFar`, by the rules a whole read uses, and work `said` out
only where the words are snapped. 1 is the kept summary, which the first
version of this pull request built: it costs a new home for state, a
build stamp that shipped builds do not have, and a read of every
transcript after each write of it, all for a start no faster than the
head. Dropped. Dropped from 4: `said` in the file, as the issue asked.

## The decision

```
Episode.Status
  Episode.heardSoFar
    the transcript the episode holds, all of it, said too     <- asked first
    held on the Episode by the transcript's stamp
      readTranscriptHead: words.json up to its words,
        the length of words.frames, transcriptFile.fit         <- new
      readSaved, the whole transcript, when the head is not one written here
```

- `Status` says `Said` and `Speechless` only of a transcript the
  episode holds. The library, which shows neither, reads only the heads.
- `readTranscriptFile` and `readTranscriptHead` share
  `transcriptFile.fit`, the checks of the version, the video, the model,
  the parts and the reach of the loudness.
- No new file, no new home for state, nothing in `ARCHITECTURE.md`
  changes.

## What follows

- A start of the app reads at most 4 KB of each video's words file and
  none of its loudness, about 0.02 ms, however long the video. Timed in
  the cloud session, see the pull request.
- An update reads no transcript again. There is nothing kept to expire.
- What would show it wrong: a start that says done or parts other than
  a whole read of the same files says, or an opened video with no lines
  where its transcript has words. The tests compare both.
