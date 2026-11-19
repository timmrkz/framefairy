# A video's probe is kept in its work folder

Issue #891.

## The problem

The issue, from the speed hunt of 16 November: "What the app learns
about a video at a cost, its probe and the ink of its captions, is kept
in memory only, so every start pays for it again."

Read on main, 19 November:

- `Engine.Probe` in `engine/ffmpeg.go` asks ffprobe what a video is: one
  run for the streams, and four more side by side, `uneven`, for frames
  off the grid of the rate. `askOnce` in `engine/probed.go` keeps the
  answer on the `Machine`, by the stamps of the file and of ffprobe, so
  a program asks once while it runs. It dies with the program.
- As the app starts, the window asks `State` and `Source` of every open
  video at once, and each waits for its video's probe. The start of the
  app with ten videos, timed in the cloud session by the benchmark of
  the speed hunt, 3 runs of 2 rounds: 50 runs of ffprobe and 1114 to
  1517 ms for all ten to answer.
- Every place that probes goes through `Engine.Probe`: eight callers, in
  the app's `media.go` and in the engine's jobs, render, audio, project,
  run and a clip made by hand.
- The measured ink of the caption blocks is kept the same way, in
  `inkCache` on the `Machine`, by the face, size, width and text of the
  block. It costs the first play of each clip in each run, not the start.

## Five simulations

### 1. Where state lives

What ffprobe says of a video is a fact about the video's file, the way
its transcript and its loudness are, and those live in its work folder,
`logs/words.json` and `logs/levels.json`, each with the stamp of the
video it was made from. So the probe does too: `logs/probe.json`, with
the video's stamp, `sourceStamp`, and the stamp of the ffprobe that
answered. A probe whose stamps do not match is not read, and the next
probe writes it again. The work folder goes when the video is removed,
so nothing is left behind. Home: one file per video. Writer: the
engine. Crossing a boundary: the files of a work folder, which an older
and a newer build both read, one new file in it, versioned.

The ink is not a fact about one video. The same block in the same face
measures the same in every video. Its home would be `~/.framefairy/`,
which holds what the machine knows, beside the models. A second home.

### 2. What runs together

Nothing needs to be kept if nothing waits for it. The start could show
each video without its probe and probe it after, so the window never
waits. But the window cannot draw a video preview or a range picker
without the video's length and its grid of frames, which are the probe.
Every start would draw an empty workspace that fills in a second later,
which is the cost moved, not removed. Dropped. What comes back: the
answer has to be there before the window asks, which only a kept answer
gives.

### 3. Who may know about whom

The app could keep the probe of each video in its library, in the
config folder, since the app is the one that starts. But the command
line probes the same files and would not find it, and `Engine.Probe` is
under every caller. A rule about a video belongs as low as it can go.
So the engine keeps it, inside `Engine.Probe`, and no caller knows
whether an answer came from ffprobe, from memory or from the disk. What
comes back: the kept file is read and written in one place, the one
`askOnce` already wraps.

### 4. What changes together

The probe changes when the video changes or when ffprobe does, an
update of the app. Both are stamps already, in `keyFor`. The file keeps
both, so a new build reads none of the old answers and asks again, once.
The ink changes with the face and with libass, so its file would need
the stamp of ffmpeg and of the face for every block, and the faces the
machine has come and go as fonts are installed. Of the two, only the
probe changes with one file.

### 5. What a test has to build

A test of the kept probe needs a video ffprobe can read, which is a
committed fixture on the real disk, and a second `Machine`, which is a
second start of the app. It counts the runs of ffprobe with the stand-in
runner. The reader of the file is outside bytes, like a plan, so it has
a fuzz target that holds every answer it takes to what a probe can say:
a picture with a size, a rate above nought, a length that is a number,
a quarter turn, and colour tags of plain words, since they are handed to
ffmpeg as arguments.

## What converges

Every simulation but 2 keeps the probe beside the video, below every
caller, by the stamps of the video and of ffprobe. 1, 3 and 4 put it in
the work folder, 3 inside `Engine.Probe`. 5 adds the fuzz target and the
check of every field read. Dropped from 2: probing after the start,
which moves the wait instead of removing it.

The ink came back different in 1 and 4: it belongs to the machine, not
to a video, and depends on files that come and go. It is left out of
this decision and stays in memory. It costs the first play of a clip,
not the start, and needs its own five simulations for a home in
`~/.framefairy/`.

## The decision

```
engine.Probe(path)
  memory, askOnce on the Machine, by the stamps of the file and ffprobe
  the work folder, logs/probe.json, by the same stamps     <- new
  ffprobe, five runs, and the answer written to both
```

- `logs/probe.json` holds a version, the video's `sourceStamp`, the
  stamp of the ffprobe that answered, and the answer. It is written only
  into a logs folder that is there, so a short or a file the command
  line was handed gets none, and no folder is made for it, which could
  bring back a work folder a removal had just deleted. It is replaced in
  one step.
- It is read only when every stamp matches and every field is one a
  probe can give. Anything else is as if it were not there.
- `docs/ARCHITECTURE.md` gains the row in Where state lives.

## What follows

- A start of the app with videos it has probed before runs no ffprobe
  for them.
- An app update, a new ffprobe, probes every video once again.
- What would show it wrong: a probe read from the file that differs
  from what ffprobe says of the same file and the same ffprobe. The
  test compares the two field by field.
