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

The cost is not the measuring, it is the ffmpeg started for it: a
process, a filter graph and a PNG pipe for a few blocks of words. libass
already runs beside the video preview, in the frames decoder of each
video, which links the libass of the ffmpeg we build and has the caption
faces loaded to draw the captions. So the decoder could measure: the
engine sends it the blocks over `internal/framewire`, it lays each out
with libass and sends back the box of its ink, with no process started
and nothing kept. That would make every measuring cheap, the first after
a start, and a corrected word too, which keeping does not.

Worked through on the code, it loses:

- The decoder is "anything but decoding" in ARCHITECTURE.md, and
  measuring would be a second job for it, with a message each way on the
  wire and a caller in the engine that knows a decoder runs.
- There is a decoder only for a video open in the video preview of the
  app. The command line, the thumbnails and a render of a video not open
  still measure with ffmpeg. Two ways to measure the same ink is two
  answers that can drift, which One drawing of the captions forbids.
- The decoder of a video starts as the video opens, the moment its first
  caption file is wanted, so the first captions would wait for the
  decoder to start instead of for ffmpeg.
- The ffmpeg way would stay for the rest, so the first caption file of a
  render after a start would still wait for it.

What comes back: the first captions after a start need an answer that
is there before anything starts, which only a kept answer gives. A
cheaper measuring can come later under it, and keeping would hide less
of it, but it is a change of its own.

Dropped too: the captions showing without their pill and gaining it
once measured, which is two drawings of one caption, and measuring ahead
while idle, which pays the whole cost again every start.

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
carries, worked out once a run, the first time ink is measured. The
ffmpeg beside the program was checked against the SHA-256 built into
the program before it ran, so that sum is its hash and the file is not
read again. Only an ffmpeg named in the environment, in a test or a
build compared by hand, is read for it. A stamp that
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

1, 3, 4 and 5 are one design seen from four sides: the ink kept on the
machine, below every caller, by a stamp of what it depends on. 1 puts it
in `~/.framefairy/captions/ink.json`, 3 lets only the programs name it,
4 stamps it by content and not by time, so an update keeps it, and 5
adds the fuzz target. 2 is the one other design, a cheap measuring in
the frames decoder, and loses because it covers only a video open in
the app and gives the ink a second way to be measured. Dropped from 1:
the work folder, which needs a video the measuring does not have.

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
  keeps, 4096, and a full cache makes room by one box, so the file
  never loses the rest.
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
