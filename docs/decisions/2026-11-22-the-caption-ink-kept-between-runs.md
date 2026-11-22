# The caption ink kept between runs

Issue #891, the part the decision record
[2026-11-19-a-videos-probe-kept-in-its-work-folder.md](2026-11-19-a-videos-probe-kept-in-its-work-folder.md)
left open.

## The problem

The issue: "What the app learns about a video at a cost, its probe and
the ink of its captions, is kept in memory only, so every start pays for
it again." The probe is kept since #941. The ink is not.

Read on main, 22 November:

- The pill under each word and the box under each caption are drawn from
  where libass puts the ink of each block of words. `measureMany` in
  `engine/ass.go` has ffmpeg draw the blocks into tall frames and reads
  the ink off the pixels. It keeps each box in `inkCache` on the
  `Machine`, by the face, size, weight, width and text of the block,
  `measureKey`. It dies with the program.
- So the first caption file of each clip after a start waits for ffmpeg.
  In the cloud session a caption file of 18 captions took 433 ms with
  the ink measured and 3.6 ms with it known, 3 runs each. On Tim's Mac,
  report of 22 November, the first caption files of a run took 100 to
  270 ms, later ones of other clips 60 to 100 ms, and 6 frames of the
  video preview were put up without their captions because they did
  not come in time.
- The ink depends on the block, the face file, the size, the weight and
  the width, and on the libass and FreeType inside ffmpeg. A face the
  program carries is in the program, `fontBundle`. A face of the
  machine's is copied under a name that changes with its file,
  `AssName`. ffmpeg is a file beside the program, copied anew by every
  update even when it did not change.

## Five simulations

### 1. Where state lives

Ink is not about one video. The same words in the same face measure the
same in every video, and the command line's renders measure the same
blocks as the app. So its home is the machine's: `~/.framefairy/`, which
already holds the models, the notes about running servers and the speed
records. In a folder of its own, `captions/ink.json`, since the folder
over the models is watched, and a file written in it would wake the
app's look at the library at every write. One file, one writer at a
time per program, replaced in one step.

The video's work folder was weighed: it is removed with the video, and
it is where the caption files of the video preview are kept. But
`measureMany` knows no video, and every caller between the render and
the video preview would carry a path for it. The same block in two
videos would be measured twice, and its logs folder is watched too.

### 2. What runs together

Nothing is kept if nothing waits: the captions could show without their
pill and gain it once measured. Two drawings of the same caption in a
row break One drawing of the captions, and Tim would see the pill jump
in. Measuring ahead while idle, every clip of every open video after a
start, moves the cost onto the machine at the moment a search or the
hearing wants it, and pays it again every start. Dropped. What comes
back: the ink must be there before the clip is clicked, which only a
kept answer gives.

### 3. Who may know about whom

The engine keeps the ink, inside `measureMany`, under every caller, the
way `Engine.Probe` keeps the probe. No caller knows whether a box came
from ffmpeg, from memory or from the disk. Where the file is is the
program's to say, in `Machine.UseSystem`, which hands a machine the real
system: a test's machine keeps the ink in memory only, and never writes
into the home folder of the computer it runs on.

### 4. What changes together

The ink changes when ffmpeg or a face changes. A face of the machine's
is already in the key by the name of its copy. The faces the program
carries change only with the program, and ffmpeg only when it is built
again, but its file's time changes with every update. So the file is
stamped with a hash of ffmpeg's bytes and of the faces the program
carries, read once a run, the first time ink is measured. A stamp that
does not match reads nothing, and the next measuring writes the file
again under the new stamp. An update that changes neither keeps the ink.

### 5. What a test has to build

A test needs two machines on one memory disk, the second the next start,
a stand-in ffmpeg that counts its runs, and the stamp. The second
machine draws the same captions without running ffmpeg. The reader of
the file takes bytes from the disk, so it has a fuzz target that holds
every box to what libass can draw: whole numbers inside a frame's
reach, left of right, top over bottom, and no more entries than memory
keeps.

## What converges

1, 3 and 4 keep the ink on the machine, below every caller, by a stamp
of what it depends on. 1 puts it in `~/.framefairy/captions/ink.json`,
3 lets only the programs name it, 4 stamps it by content and not by
time, so an update keeps it. 5 adds the fuzz target. Dropped from 1:
the work folder, which needs a video the measuring does not have. From
2: captions drawn twice, and measuring ahead.

## The decision

```
measureMany(blocks)
  memory, inkCache on the Machine, by measureKey
  ~/.framefairy/captions/ink.json, read once a run, by the stamp   <- new
  ffmpeg measures, and the boxes are written to both
```

- `ink.json` holds a version, the stamp, a hash of the ffmpeg that
  measured and of the faces the program carries, and the boxes by
  `measureKey`. It is read once a run, before the first measuring, and
  only when the stamp matches and every box is one libass can draw.
- After a measuring that added a box, the whole cache is written, in
  one step, into a folder it makes. At most as many boxes as memory
  keeps, 4096.
- `Machine.UseSystem` names the file. A machine without it keeps the
  ink in memory only.
- `docs/ARCHITECTURE.md` gains the row in Where state lives.

## What follows

- The first captions of a clip after a start come without ffmpeg, once
  any run before measured the same words in the same face and size.
- A new ffmpeg, or a new face the program carries, measures again once.
- The system's ffmpeg on Linux links libass from the system, which can
  change without ffmpeg's file changing. The stamp does not see that.
  The builds we ship link it into ffmpeg.
- What would show it wrong: a pill that does not fit its word after an
  update. The stamp would then miss something the ink depends on.
