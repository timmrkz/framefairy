# The waveform keeps up with the hand

21 November 2026, issue #968. It follows
[The video preview keeps up with the hand](2026-11-20-the-video-preview-keeps-up-with-the-hand.md),
pull request #936, and partly replaces
[The window holds the loudness, and Go sends it](2026-11-18-the-window-holds-the-loudness.md):
the window still holds the whole loudness and draws every view from it,
but what is measured now reaches it through a look, not through a stamp
and a read of the whole.

## The problem

Tim clicks at random places on the range picker of a 4 h 20 min video he
has just added. The video preview shows the frame quickly. The waveform
on the clip timeline shows up 0.3 to 0.4 s later, every time. His report
of 21 November, 12.11:

- He added the video again at 12:10:13. Its waveform was measured whole
  at 12:10:45, 32 s later. Every click was in those 32 s.
- After a click the measuring went there within a few milliseconds. The
  waveform for the click's place reached the window 300 to 430 ms after
  the click.

In his words: "Whatever changes you're doing doesn't seem to move the
needle, so maybe you need to switch up your approach." And then: the
video preview's frames and the clip timeline's waveform are "basically
the same mechanism. There's a stretch of content to be displayed, whether
it's a video frame or an audio waveform. And there are those mouse
pointer events, seemingly at random, which click and drag along the
range picker or the clip timeline." What #936 does for the frames should
be used here too, "so that we are not just writing the same code all
over again."

How the two worked:

- **Video preview.** The window asks Go for the frame under the hand,
  `/frames/look`, and waits. One rule, `plan` in `engine/schedule.go`,
  says what the decoder does next, asked again after every frame. The
  moment a frame lands that is nearer the hand than the one on screen,
  Go answers the look with it. A newer look answers the one before as
  overtaken.
- **Waveform.** The window told Go where the clip timeline looks and
  waited for nothing. Go measured there first, wrote it to the disk every
  half second, and the app told the window, at most four times a second.
  The window then read the whole loudness again, 3 MB for Tim's video,
  and drew the view from it.

So between measured and on screen there was a write every half second, a
notice up to a quarter of a second late and a 3 MB read. The video
preview has none of that. That detour is most of what Tim waits for.

## What a click demands, and what the sound can do

Timed in a cloud session, 4 cores, ffmpeg on the sound of a 1 h video:

- Starting ffmpeg and seeking anywhere: 60 to 100 ms. Sound has no key
  frames, so every place is a place to start.
- Decoding: an hour in about 12 s, about 300 times as fast as it plays.
- Several ffmpeg side by side add up: each core is another ffmpeg's
  worth. Tim's Mac has 12 cores.
- What comes out is small and kept for good: a byte per 10 ms, 1.5 MB
  for 4 h. Frames are 6 MB each and kept only around the hand.

The targets:

1. A look at a view that is measured is answered at once, from memory.
2. A look at a view that is not measured gets its first readings within
   an ffmpeg start and one read, and the rest as they come.
3. The whole video is measured in about its decoding time divided by the
   number of ffmpeg, plus a start each.
4. No more ffmpeg run than the measurers allowed, and never two in the
   view.

## Five simulations

### A. Shorten the detour (keep what works)

Keep the measuring as it is. Tell the window at once rather than four
times a second, and let it read only what is new. It touches the app's
notice and the window's read, about three files. But the waveform keeps
a mechanism of its own beside the video preview's, a stamp, a notice and
a read, and the next change to how the hand is followed is made twice.
It is more of the same, which Tim asked to step away from. Dropped.

### B. One schedule for both (share the most)

Make `engine.Schedule` work for anything made, frames or loudness, with
the rule handed in. Counted on the code, most of `schedule.go` is about
video: key frames, which frames to keep when memory is short, the size
of the picture, holding the decoder while the window paints, plays, the
frames of the cards. None of it applies to sound. And sound needs what
video does not: start anywhere, several at once, keep everything. Every
one of those would be a question of which kind the schedule holds, the
pile of flags #936 set out to remove. Dropped.

### C. Share the look, each its own rule (what changes together)

Take out of `engine/schedule.go` the part that is the same for anything a
hand moves over:

- the window asks where the hand is, and waits for an answer,
- the newest look wins, and an older one is answered as overtaken,
- the answer goes out the moment the work brings what the window lacks.

That is `looks` in `engine/looks.go`, generic over what a look says and
what it is answered with. `engine.Schedule` uses it for frames,
unchanged in what it does. `engine.LoudnessSchedule` uses it for the
loudness, with a rule of its own for what sound can do: the view first,
then the rest of the video, side by side. The clip timeline asks
`/frames/levels/look` with its view and what it holds, the way the video
preview asks `/frames/look`, and is answered with the loudness of the
view. Names crossing the new boundary: the look's type, its answer, and
the `Sent` that orders looks, which `/frames/look` has already. The disk
keeps the loudness between runs and no longer carries it to the window.

### D. One hand for both (one input)

The window sends one look for the playhead and the view, and both rules
read it. But the two move apart: a swipe or a pinch moves the view and
not the playhead, and the playhead moves inside the view without moving
it. One look would change for either and wake both. Dropped. C gives
both looks the same shape instead.

### E. Nothing left to plan (make the work vanish)

Measure the whole video so fast that a look never waits: side by side,
on half the cores. A click in those first seconds still waits, so it is
not enough alone. Folded into C, as the second half of the loudness
rule.

## What converges

C, with E inside its rule. What came back in most of them: the window
must be answered from what Go holds in memory, not through the disk (A,
C, D, E), and the part that is the same for frames and loudness is the
look, not the planning (B showed the planning differs, C and D share the
look). What only one needed, B's generic planner and D's single hand, is
dropped.

## The decision

Layers, each using only the ones under it:

1. **The window**: `LoudnessLooks` in `frontend/src/lib/loudness.ts`
   sends the view on every move and lays each answer over the loudness
   it holds. It decides nothing about the readings.
2. **The app**: `/frames/levels/look` in `cmd/framefairy-app/media.go`
   hands the look to the video's measuring and writes back what it is
   answered with.
3. **The loudness schedule**: `engine.LoudnessSchedule` in
   `engine/loudnesslook.go` holds the loudness as `engine.Loudness` lays
   it out, keeps the levels for zooming out right as each reading
   comes, and answers a look with the bytes of its view, at once or the
   moment the measuring brings some.
4. **The measuring**: `MeasureLevels` in `engine/levels.go` runs half the
   cores' worth of ffmpeg, at most eight. One goes to the view, the
   others take the longest work left, and each hands every read to the
   loudness schedule.
5. **The look**: `looks` in `engine/looks.go`, which both schedules use.

## How it is proven

The record's draft promised a simulated ffmpeg on a fake clock, like
`engine/drag_test.go`. It was not built. Sound has no key frames, so its
cost is an ffmpeg start and a straight decode, and a simulation would
only restate those two numbers. The real ffmpeg on a committed fixture
runs the same paths in about a second. So:

- `TestTheWindowIsAnsweredWithTheLoudnessOfItsView` and
  `TestANewerViewOvertakesOneThatWaits` hold targets 1 and 2: a measured
  view is answered at once, an unmeasured one with its first readings,
  a newer look overtakes one that waits.
- `TestMeasurersShareTheLongestWorkLeft` and
  `TestOneMeasurerLeavesForTheView` hold target 4.
- `TestAVideoIsMeasuredInPlacesSideBySide` holds target 3 in what it
  can: three ffmpeg start where they should, every reading is measured
  once and written once, and the result is the one ffmpeg's.
- The frame schedule's own simulation, `engine/drag_test.go`, runs
  unchanged on the shared look.

Timed in a cloud session, 4 cores, so 2 measurers by the rule, on a 1 h
25 fps video. A click on a 60 s view at 40 min, after the first minute
was measured. Six runs each, the whole video three times before. Before,
the clock stops when the levels file covers the view, so it leaves out
the notice and the 3 MB read the window still had to make.

| ms | before | after | change |
| --- | ---: | ---: | --- |
| the first of the view on hand | 512 | 73 | 86 % faster, ± 1 % |
| the whole view on hand | 761 | 299 | 61 % faster, ± 33 % |
| the whole video measured | 11719 | 7390 | 37 % faster, ± 4 % |

With 4 measurers the whole video took 3342 ms, 71 % faster. With 1, as
before, 11834 ms, 1 % slower, inside the spread.

## What follows

- A click where the waveform is measured is answered from memory. One
  where it is not gets its first readings after an ffmpeg start and a
  read, and the rest as they come.
- The whole video is measured on half the cores, at most eight. The
  hearing takes every fast core while it runs, all eight of an M2 Max,
  so an ffmpeg out of the view runs at a lower priority and gives way to
  it. With the cores free, as they are once the hearing is done, the
  measuring runs as fast as ever. The review of #974 found the first
  draft of this said the hearing had the other half, which was wrong.
- What the window holds is state, and Go keeps it: which chunks of the
  loudness it sent to which window, `LoudnessSchedule.told`, the way
  `telling.sent` keeps the clips a window was sent. A version number
  from the window alone could not say it, and a view measured while the
  window looked elsewhere was told nothing was new, found by the same
  review.
- `/frames/levels` stays, for opening a video: the window reads the
  whole loudness once and the looks keep it up to date.
- It is wrong if Tim still waits for the waveform after a click on a
  part not measured, or if measuring side by side slows the hearing he
  waits on.
