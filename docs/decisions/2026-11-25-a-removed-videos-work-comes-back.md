# A removed video's transcript and clips come back when it is added again

A spec, written before any code. The pull requests that build it name
this record.

## In 30 words

When Remove deletes a video's folder, the transcript and the clip sets
are set aside under a fingerprint of the video's content. Adding the
same file again, from anywhere, puts them back.

## The problem

The user, on 24 November 2026: "when I remove it and re-add it again
later, the information about the clips that I have already generated
and that I have already rendered are gone ... I then have to go through
the entire video again." On 25 November they set the scope: "The
fingerprinting and then the restoring of the metadata of those three
things", the transcript and how far transcription got, the clips with
their rendered marks, and every word fixed in the captions. "Nothing
else should change." The shorts stay where the settings put them, and
the person can still delete everything else.

What happens today, read on main at ba5b224 and in the logs of the
user's own reports:

- Everything about a video lives in a folder beside it,
  `Folge 12.framefairy/` beside `Folge 12.mp4`, found by the video's
  file name, `engine.WorkDir`.
- **Remove** asks Cancel, Keep or a red Remove. The red Remove deletes
  the whole folder, `Machine.DeleteWork` in `engine/frames.go`.
- In the user's logs from 7 to 21 November there are 14 removals, every
  one with the folder deleted, `work_deleted: true`. 8 of them were
  followed by adding the same video again within 20 seconds. The other
  6 were not added again within the logs there are. The user removes
  videos while testing, or for any other reason, and wants a video
  added again to come back with the progress made on it.
- The three things to keep are small files in the folder's `logs`:
  - the transcript, `words.json`, with the parts transcribed so far, and its
    loudness, `words.frames`
  - the clip sets, `clips*.json`. Each clip holds its edges, cuts,
    crop, the words fixed in its captions, `corrections`, and the words
    removed from them, `removed_words`. Each clip set notes under
    `shorts`, by clip id, where each short went, which is what makes a
    clip green
  - nothing else: a fixed word lives in its clip, never in the
    transcript, see docs/WORDS.md
- The transcript is held to its video by the video's name, size and
  time, `sourceStamp` in `engine/transcript.go`. A video renamed, or
  downloaded again with a new time, finds its transcript out of date
  and is transcribed again. Each clip set names its video's path,
  `source`, which nothing reads.
- A fingerprint of a video's content is already in the code,
  `EpisodeKey` in `engine/training.go`: the file's size and its first
  and last megabyte, hashed, sixteen letters and digits like
  `3f9a1c7e04b2d8a1`. The training records use it. It is the same for
  the same file under any name in any folder.

## What a person sees once it is built

1. **Remove a video with the red Remove.** Its folder is deleted as
   today, shorts in its `out` folder included. Only the transcript and
   the clip sets are kept, in the app's own folder.
2. **Add the same file again,** under the same name or another, from
   the same folder or another. The transcript is there and transcription goes
   on from where it stopped. The clip cards are there, the rendered
   ones green on the range picker, with every word fixed in their
   captions. A short that was deleted with the folder no longer has
   Show in folder, and its clip still says it was rendered, as today.
3. **Keep** in the remove box and everything else work as today.

## Five simulations

### 1. Where state lives: set aside when the folder goes

The principle: one copy at a time. The three kinds of file stay in the
folder beside the video while it is in the list. When the red Remove
deletes the folder, they are moved first to
`~/.framefairy/kept/<key>/`, where the engine already keeps the models
and the training records. When the video is added again they are moved
back, and the set-aside copy goes.

What crosses: the key, worked out when the video is added and written
into the mark the folder already carries, `added.json`, so it is known
at Remove even if the video file is gone by then. Cost: one function
that sets aside, one that puts back, both in the engine. A folder
deleted in Finder is not covered, because nothing sees it go in time.

### 2. What changes together: a mirror kept up to date

The principle: whatever deletes the folder, a copy survives. Every
write of the transcript or a clip set also writes it under
`~/.framefairy/kept/<key>/`. It covers a folder deleted in Finder. But
it is a second copy of every file, written on every edge dragged, and
ARCHITECTURE.md says why that is a bug waiting for the day the two
disagree. The command line writes the folder without the mirror.

### 3. Who may know about whom: read the training records

The principle: use what is already kept. The training records hold
every clip proposed and every action on a clip, by the same key. But
they are for training, the app must not read them, see CLAUDE.md, they
hold no transcript and no fixed words, and they can be thrown away in
the settings.

### 4. What runs together: Remove leaves the small files beside the video

The principle: never delete what is cheap to keep. The red Remove
deletes everything in the folder but the three kinds of file. Nothing
new to learn and no key. But a renamed or moved video finds nothing,
and a folder stays beside the video after the person asked to remove
it, which the user does not want.

### 5. What a test has to build: the key inside the folder, found by looking around

The principle: no new folder. Each work folder records its key, and
adding a video with no folder of its own looks at the other
`.framefairy` folders in the same folder for its key. It covers a
rename in one folder, and nothing deleted or moved elsewhere.

## What converges

- **The key.** Simulations 1, 2, 3 and 5 know the video by its content.
  The fingerprint is in the code already. Reading two megabytes takes a
  few milliseconds on an SSD, reasoned, not measured.
- **A folder the engine owns, outside the video's folder.** Simulations
  1, 2 and 3. Only there does a removed video's work survive.
- **One copy at a time.** Simulation 1 moves, simulation 2 copies. A
  move cannot drift.
- **Dropped:** the mirror (2) for its second copy, the training records
  (3) by rule, leaving files behind (4) and looking around (5), which
  cover too little.

## The decision

Simulation 1. Drawn as layers, each using only the ones under it:

```
the app: Remove, and adding a video, as today
        │
        ▼
the engine: SetAside(video, key), at the red Remove before the folder
            goes, PutBack(video, key), when a video is added, before
            the app decides whether it is new
        │
        ▼
fileio over afero: the disk, or memory in a test
```

On the disk, for `Folge 12.mp4` after the red Remove:

```
~/.framefairy/kept/3f9a1c7e04b2d8a1/
  words.json, words.frames      the transcript and how far it got
  clips-0-1800.json, ...        the clip sets, fixed words and renders in them
```

A few megabytes for hours of video, most of it the transcript,
reasoned, not measured.

### The rules

- **The app works the key out and hands it to the engine.** At the red
  Remove it reads the fingerprint from the video file. That covers the
  videos already in the library when this is built, which have no key
  saved anywhere. It also keeps the key in `added.json` when a video is
  added, for the one case the file is gone by the time it is removed.
  A video that is gone and was added before this was built has no key,
  so nothing is kept for it, as today. The engine never reads
  `added.json`, which is the app's.
- **The kept folder is named on the `Machine`,** through `UseSystem`,
  the way the caption ink file is, so a test keeps it in memory.
- **Setting aside never loses anything.** The files are copied into a
  new folder beside the kept one and then renamed into place, so a quit
  halfway or a full disk leaves either the old set or the new one, never
  a mix. Only regular files are taken, never through a link. When it
  fails, the folder is not deleted, the video stays in the list, and a
  toast says why. When a set for the same key is already there, the
  newer one takes its place.
- **Putting back fits the transcript to the video it comes back to.**
  The fingerprint has proved it is the same content, so the
  transcript's stamp is written anew for the file's name, size and
  time. Without that, a renamed or downloaded-again video would be
  transcribed again from the start. The clip sets go back byte for
  byte.
- **Putting back comes first.** The app decides whether an added video
  is new, and so whether a first search starts, by whether it has a
  folder, `fresh` in `library.go`. A video put back is not new, so it
  is put back before that is decided, and no search starts on top of
  its clips. The build pull request times adding a video before and
  after.
- **Only into an empty place.** A video added again with a transcript
  or clips of its own, after Keep, keeps them, and what was set aside
  stays set aside.
- **A key is checked before it is a path.** An empty fingerprint, which
  `EpisodeKey` gives for a file it cannot open, sets nothing aside. A
  key is taken only as sixteen lowercase hex characters, and the folder
  is reached through `SafeChild`.
- **What comes back is read like any file from outside,** through
  `ReadPlan`, `LoadClips` and the transcript's own reader. A file that
  fails is not put back.
- **The command line** removes nothing, so it sets nothing aside. A
  video put back by the app is the same folder to it as before.

### How it is built

1. **The key, setting aside at the red Remove, putting back when a
   video is added.** One pull request, in the engine and in
   `removeEpisode` and `AddEpisodes`, with tests for a rename, a move
   and a file downloaded again.
2. **A walk** that removes a video with a rendered clip and a fixed
   word, adds it again under another name, and finds the green clip on
   the range picker and the word as it was fixed.

It adds one row to Where state lives in `docs/ARCHITECTURE.md`: the
transcript and clip sets of a removed video, in
`~/.framefairy/kept/<key>/`, written by the engine at Remove and taken
back when the video is added.

## What follows

What it buys: a video removed and added again comes back as it was,
rendered clips green and words fixed, whatever it is called now and
wherever it is.

What it costs: two megabytes read when a video is added, and a few
megabytes kept for each removed video that is never added again. They
are not cleaned up. If they ever add up, a way to throw them away comes
then.

What it does not cover: a folder deleted in Finder, and a video
exported again as a new file, which is a different fingerprint.
