# A removed video's transcript and clips come back when it is added again

Written as a spec on 24 and 25 November 2026, and built in the same
pull request once the user chose when the copy is made.

## In 30 words

A copy of each video's transcript and clip sets is kept as they are
written, under a fingerprint of the video's content. However the folder
goes, adding the same file again puts them back.

## The problem

The user, on 24 November 2026: "when I remove it and re-add it again
later, the information about the clips that I have already generated
and that I have already rendered are gone ... I then have to go through
the entire video again." On 25 November they set the scope: "The
fingerprinting and then the restoring of the metadata of those three
things", the transcript and how far transcription got, the clips with
their rendered marks, and every word fixed in the captions. "Nothing
else should change." The shorts stay where the settings put them, and
the person can still delete everything else. Then, reading that a folder
deleted in Finder would not be covered by a copy made at Remove: "maybe
instead of doing a copy once on delete, is to actually maintain the
data when it is being created in the first place ... it doesn't matter
which method is used in order to clean up, because the recovery then
works all the time."

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

## What a person sees

1. **Remove a video with the red Remove, delete its folder in Finder,
   or delete the folder the video is in.** The folder goes as today,
   shorts in its `out` folder included. The copy of the transcript and
   the clip sets, kept as they were written, stays in
   `~/.framefairy/kept/`.
2. **Add the same file again,** under the same name or another, from
   the same folder or another. The transcript is there and
   transcription goes on from where it stopped. The clip cards are
   there, the rendered ones green on the range picker, with every word
   fixed in their captions. A short that was deleted with the folder no
   longer has Show in folder, and its clip still says it was rendered,
   as today.
3. **Keep** in the remove box and everything else work as today. The
   remove box says that deleting the folder takes the rendered clips
   and that the transcript and the clips come back.

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
`~/.framefairy/kept/<key>/`. It covers a folder deleted in Finder, and
the folder the video is in deleted whole. Its cost is a second copy of
every file, which ARCHITECTURE.md warns of: the day the two disagree,
which is right? It holds if the copy is never read while the work
folder has files of its own, and only one thing writes it, from the
work folder.

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
  The fingerprint is in the code already.
- **A folder the engine owns, outside the video's folder.** Simulations
  1, 2 and 3. Only there does a removed video's work survive.
- **When the copy is made.** Simulation 1 moves the files once, at
  Remove, and cannot drift, but misses every other way a folder goes.
  Simulation 2 copies as the files are written and misses none. The
  user chose simulation 2 for that.
- **Dropped:** the training records (3) by rule, leaving files behind
  (4) and looking around (5), which cover too little.

## The decision

Simulation 2, with the key. Drawn as layers, each using only the ones
under it:

```
the app: the watch sees a transcript or a clip set written, Remove,
         the start, adding a video
the command line: after a run
        │
        ▼
the engine: Machine.Keep(video), the copy brought up to date,
            Machine.PutBack(video), when a video is added
        │
        ▼
fileio over afero: the disk, or memory in a test
```

On the disk, for `Folge 12.mp4`:

```
~/.framefairy/kept/3f9a1c7e04b2d8a1/
  words.frames, words.json     the transcript and how far it got
  clips-0-1800.json, ...       the clip sets, fixed words and renders in them
```

A few megabytes for hours of video, most of it the transcript.

### The rules

- **One writer, from the work folder only.** `Machine.Keep` copies each
  of `words.json`, `words.frames` and `clips*.json` whose size or time
  differs from its copy, and gives the copy the time of what it copied,
  so the next Keep with nothing new reads only the folder.
- **Never deletes.** A work folder with nothing to keep, gone or not
  written yet, leaves the copy as it is. The app deletes a clip set file
  in one place only, an undo of the search that made it. The copy keeps
  that one, because a folder deleted file by file looks the same to
  Keep while it goes, and a copy that followed it would lose what it is
  for.
- **When it runs.** The app calls it when the watch sees a transcript
  or a clip set written, `reconcile`, which is every save of an edit,
  every clip set a search writes and every 8 seconds of transcription,
  `checkpointDefault`. Before the red Remove deletes a folder, and if
  it fails nothing is deleted. Once as the app starts, after the window
  has its answers, for what the command line wrote while the app was
  closed. The command line calls it after a run.
- **Why not the writers themselves.** Simulation 2 has every write
  copy itself. `writeTranscript` and `replacePlan`, the two writers,
  hold only an `afero.Fs`, not the `Machine` that knows the kept folder
  and the keys, and eighteen callers reach them from the app, the
  command line and the engine's own jobs. Passing the `Machine` to all
  of them is a change of its own, and transcription's goroutine would
  then pay the 20 ms copy of every save. The watch already sees every
  one of those writes, on a goroutine of its own, so the copy follows it
  there, and the four places that call Keep all call the one function
  in the engine. What that costs is the start, which has to catch up on
  what was written while the app was closed, and the gaps named under
  What follows.
- **The words are read before their loudness,** which only ever grows,
  and the loudness is written first, the way `writeTranscript` writes
  them, so the copy never has words that go further than their
  loudness.
- **Read only by `PutBack`, only into an empty place.** A video added
  with a transcript or a clip set of its own, after Keep, keeps them.
  So the work folder stays the truth while it is there.
- **Putting back comes first, and inside the add.** The app decides
  whether an added video is new, and so whether a first search starts,
  by whether it had a folder. A video put back is not new. The put back
  runs inside the store's add, `store.addEpisodes`, after the checks
  that refuse a video sharing a folder with another, so a video refused
  writes nothing, and under the lock the delete of what was written
  back into the folder of a video let go also takes, so that delete
  never falls between the put back and the add.
- **Putting back fits the transcript to the video it comes back to.**
  The fingerprint has proved it is the same content, so the
  transcript's stamp is written anew for the file's name, size and
  time. Without that, a renamed or downloaded-again video would be
  transcribed again from the start. The clip sets go back byte for
  byte, each read as a plan first, `writePlanFile`. A file that does
  not read is not put back.
- **A key is checked before it is a path.** The key of each video is
  worked out once while its size and time stay the same, and noted in
  `keys.json` beside the copies, so a start does not read every video
  again. It is worked out outside the lock Keep and PutBack take. An empty one,
  which `EpisodeKey` gives for a file it cannot open, keeps nothing. A
  key is taken only as sixteen lowercase hex digits, and the folder is
  reached through `SafeChildIn`. Only regular files are copied, never
  a link.
- **The kept folder is named on the `Machine`,** by `UseSystem`, the way
  the caption ink file is, so a test keeps it in memory and a machine
  with none keeps nothing.

## What follows

What it buys: a video comes back as it was, rendered clips green and
words fixed, whatever it is called now and wherever it is, however its
folder went.

What it costs, measured with `BenchmarkKeep` in a cloud session, on a
four hour transcript, five runs each: a Keep with nothing new takes
0.02 ms, one after a save of the transcript 20 ms, which is once every
8 seconds while a long video is transcribed, on the watch's goroutine.
Remove waits for one Keep. Adding a video reads two megabytes of it for
its key, once, 3 ms, and putting a four hour video's work back takes
57 ms, which the add waits for. The first start after the update reads
each video for its key, 3 ms each, and copies what each has. Later
starts read the keys from `keys.json`, 0.03 ms a video. A few megabytes stay for each video ever added. They are not
cleaned up. If they ever add up, a way to throw them away comes then.

What it does not cover: a video exported again as a new file, which is
a different fingerprint. A search undone whole comes back with the
video, as a clip set, when the video is added again after its folder
was deleted. Writes made while the app was closed by
anything but the command line, and a folder deleted before the next
start, which is rare.
