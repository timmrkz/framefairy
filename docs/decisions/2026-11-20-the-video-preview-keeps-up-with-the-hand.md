# The video preview keeps up with the hand

20 November 2026, issue #924, pull request #936. It follows
[Go decides which frames the video preview gets](2026-11-19-go-schedules-the-video-previews-frames.md),
which moved the choice of the next frame from the window to Go. This
record is about how Go makes that choice while a hand drags.

## The problem

Tim's reports of 19 November, 21.39, and 20 November, 15.02, both have
drags where the hand moved and the picture stood still. The second, on a
build with the fix for the first, still had stand-stills of up to 558
ms, 1010 of 1723 looks overtaken before a frame came, and one run that
decoded 216 frames, about 800 ms of work, and showed none of them.

Tim asked to step back: how should this work, for fast and slow drags
in any direction, when nobody can tell where the hand goes next.

The code and the logs showed three flaws:

1. Only the frame asked for was shown. The frames a run decoded on its
   way, many of them nearer the hand than the picture, were thrown
   away.
2. A target that keeps moving was never reached. In a slow drag the
   next look came before the run reached the last one, and the run was
   started again from the next key frame, every move.
3. No one rule said what the decoder does next. The choice was spread
   over about ten flags, `passing`, `dragged`, `ahead`, `behind`,
   `held` and more, and each fix had added one.

## What a drag demands, and what the decoder can do

The costs are fitted from the decoder's own lines in the two reports, a
25 fps video on Tim's M2 Max: 10 ms to open at a key frame, then 3.8 ms
for each frame, one at a time. Making a picture of a frame at the size of
the video preview costs more on top, which the reports do not tell
apart, guessed at 1 ms.

- Forward the decoder reads about 260 frames a second, ten times the
  speed the video plays.
- Backward every new frame means a seek to a key frame, and decoding
  from there forward. A frame kept costs nothing.
- A hand that moves further at a step than the decoder makes frames in
  a frame of the screen, three or four frames, leaves frames on its way
  that nobody can read.
- The hand speeds up, stops, turns round and jumps. Nothing may depend
  on guessing where it goes next.

The targets, each a check in the simulation:

1. The decoder never rests while the picture is not the frame under
   the hand.
2. The picture moves toward the hand at least every 50 ms, and 95 % of
   the time within 25 ms, counted from when the frame that moves it
   could be there at the soonest.
3. After letting go, the exact frame comes within the time an open and
   a whole key frame span of frames take, 135 ms at 25 frames.
4. The picture is never further behind the hand than one key frame
   span and how far the hand moved in the last 50 ms.
5. While the picture is more than a key frame span from the hand, it
   moves at least every 50 ms, with no allowance.

The second and third are not quite what the proposal to Tim said. The
proposal held every stand-still to 50 ms. A hand that turns round into a
stretch nobody read waits for the key frame before it on any schedule:
the decoder reads only forward, and at a key frame every 25 frames that
can be 90 ms. So a stand-still counts from the moment a decoder that set
out when the hand last turned, pressed or moved too fast to follow could
have had the frame. A drag the hand makes steadily gets no such
allowance: there the frames could have been read before it got there,
and the schedule is held to 50 ms. The third was 120 ms before the cost
of making the pictures was counted.

The fifth came from the review. A fast drag, the usual drag on the range
picker, where a pixel is about 100 frames of an hour, moves further at
every step than the decoder can follow, so every step counted as a turn
and the second target's allowance always lay ahead. A schedule broken on
purpose so that it never caught up passed the second with a longest
stand-still of 5 ms while the picture was 205 frames behind. The fifth
has no allowance, and catches it.

## Five designs

Each starts from a principle of its own.

### A. Patch the schedule there is

Change as little as possible. It meets the cases seen so far and no
others: each fix added a flag, and each report found the next case.
Dropped.

### B. One planner, asked after every frame

The decoder always works toward the hand, and every frame it makes that
is nearer the hand than the frame on screen goes up at once. After each
frame, each look and each change of what has the decoder, one rule says
what is wanted most and whether the run under way does it the quickest
way. It assumes nothing about where the hand goes. Recommended, and
built.

### C. Predict where the hand goes

Decode where the hand will be, from its speed. It fails each time the
hand turns or jumps, which Tim said it does, and then has the decoder
far from the hand. Dropped.

### D. Two decoders, one forward and one backward

Work on both sides at once. Dropped, first because "the decoder makes
one frame at a time, so two streams take turns". That is not so: each
cursor of `framefairy-frames` runs on a goroutine of its own, and a play
already runs the next stream beside the one it shows. In the cloud, on
the processor with four cores, two streams of 40 frames at once took
1.18 to 1.49 times as long as one, in three runs, so two streams at once
make more frames than one. On Tim's Mac the frames are decoded on the
graphics chip, whose one decoder the streams share, and that was not
measured. D stays dropped for this change, on two grounds that hold:
the planner of B already keeps the decoder busy on the frames wanted
most, and D needs a second run in every rule of the schedule, which B
has none of. Whether two streams at once on Tim's Mac make more frames
than one decides whether D is worth simulating, see What follows.

### E. Only key frames while the hand moves

The cheapest picture first. Good for a fast drag, poor for a slow one,
which jumps a second at a time. Folded into B: a fast drag's run hands
the key frame first, and the frames nearer the hand after it.

## What converges

B, with E for fast drags. One rule, `plan` in `engine/schedule.go`, asked
after every frame. It wants, the most wanted first:

1. the frame under the hand. A drag's run starts at the furthest of the
   frames nearer the hand than the picture, so each one it makes moves
   the picture. A frame before the key frame before the hand is not put
   up on the way: it is of another stretch of the video, which the
   picture would leave again as soon as the key frame comes. A drag to the left reads the whole key frame span it
   drags into, since it wants those frames one after another and a run
   that read only the nearest would seek back to the same key frame for
   the rest.
2. the frames on the side the hand drags to: eight ahead of a drag to
   the right, a span behind a drag to the left.
3. a few frames on either side, four after and 15 before, whatever the
   hand does, since nobody knows where it goes next and a decoder with
   nothing to do costs the hand as much as one stopped between two
   frames for its next place. Not on a drag faster than eight frames a
   step, where the decoder is kept for the frame under the hand.

What is read around the hand fits in the frames the schedule keeps,
192 MB, with a quarter to spare, and each run around a place is read
once until the hand moves. At a video preview of 2560 by 1440 and more,
the review found, fewer frames fit than were wanted, each run let go of
what the one before read, and the decoder never rested.

Where the key frames are is learned from the decoder, which hands the
key frame first for a drag: the key frame it starts from for a frame
says no key frame lies between the two, and the shortest gap between two
key frames seen is the span once it is less than twice the longest
stretch seen. Until then a span is a second. Encoders differ: two
seconds is what streaming guides ask for, and x264 puts one every 250
frames unless told otherwise, and one at each cut besides.

The run under way goes on when it reads what is wanted by reading on,
with no key frame worth seeking to on the way, or when it is a run for
the hand that has yet to reach it. Otherwise it is stopped between two
frames and a new one starts.

The window tells Go where the hand is on every move, also over frames
it keeps itself, so Go reads around a place before the hand gets
further. With the look that tells it, a hand at rest on a frame the
window holds names the frames around it the window holds, and those are
not read again. The window's list is never kept in Go beyond the place
it came with: a copy kept from what was read ahead went stale over
presses that read nothing ahead. Each drag is written to the log as
one line once the hand lets go, with every place it went to, so a drag
that lagged on someone's Mac can be played in the simulation. Tim's two
reports could not be: their lines name no frame for a look.

While the window draws the frame of a click, the decoder reads nothing
else, so it takes no processor from the drawing. Go holds from the
moment it answers a look of a hand at rest, and the window asks Near
once the frame is painted, the one signal from the window that a frame
is on screen. The run under way ends at the frame it was for while Go
holds, and the next reads on from there. Four other ways were tried:

- The window holds back what it sends until the frame is painted, its
  tells, its words to read ahead and Near. That is a fetch policy in the
  window, and the window had to copy the shape of a read ahead, four
  frames after a word, to keep the right frames. The hold is Go's, and
  the answer of a read ahead names the frames the window keeps.
- Go holds once the window asks whether to ask Near. That came after
  the answer on another goroutine, so the click's run had read on before
  it, and a hold that never ended a run let one decode to the end of the
  video.
- The run under way waits between two frames while Go holds, and goes
  on once the window asks, in one stream. The simulated presses and
  clicks decoded as many frames as with a run that ends and a next that
  reads on, and a run that waits keeps a stream open on a window that
  may never ask, one hidden before it painted.
- A look that tells Go where the hand is on a frame the window holds
  holds too, and the window asks Near after it as well. The window was
  sent the frames before every word it stepped to, let go of others for
  them, and the simulated presses decoded more frames than without it.

## How it is proven

`engine/drag_test.go` drags the playhead over a simulated decoder with
the costs above, on the fake clock of a `testing/synctest` bubble: 11
drags written out, slow and fast, both ways, turning round, a click far
off, and 12 made at random from fixed seeds, which speed up, stop, turn
and jump. The simulated decoder hands frames the way `framefairy-frames`
does: with the key frame first, the key frame and then the frames from
the one asked for, and a stream stopped finishes the frame it is making.
Each drag is checked against the five targets, and a drag that misses
one prints what the decoder, the window and the hand did around the
miss.

The drags run over a video with a key frame every second, held to all
five targets, and over videos with one every two seconds, every ten,
and at gaps of 12 to 75 frames, held to the first. A video preview of
1012 by 570 up to 3840 by 2160 has the decoder rest once the hand does.

The schedule as it stood before this record misses a target in 19 of
the 23 drags over the video with a key frame a second. Its Before column
was made in a worktree of its own, with the schedule of that commit and
one line added: its runs behind and ahead had no context to stop them
by, and stopping one panicked under this simulation. The planner meets
all five targets in all 23. Measured in the simulation, which repeats
exactly, so each number is one run with no spread:

| Over the 23 drags, a key frame a second   | Before | After | Change      |
| ----------------------------------------- | -----: | ----: | ----------- |
| Mean longest stand-still, ms              |   69.0 |  14.4 | 79 % faster |
| Worst stand-still, ms                     |  133.0 |  49.0 | 63 % faster |
| Mean 95th percentile stand-still, ms      |   55.1 |  12.2 | 78 % faster |
| Mean exact frame after letting go, ms     |   86.6 |  67.0 | 23 % faster |
| Worst exact frame after letting go, ms    |  339.0 | 131.0 | 61 % faster |
| Mean furthest behind the hand, frames     |   41.8 |  10.7 | 74 % fewer  |

The exact frame after letting go comes later in 6 of the 23 drags, by
up to 18 ms, 131 ms at worst. Four are wandering drags that let go
inside a span the run reads whole, making a picture of each frame on
the way, by 4 to 18 ms. Read with the key frame first and the rest
unscaled, they came sooner, and the stand-stills of a slow drag to the
left came back. The other two are the fast drags, by 2 ms, 16 to 18 ms.
The difference is the guessed cost of making the pictures, so it is as
uncertain as that guess.

Over the other videos the planner guesses where the key frames are
until the decoder has started from a few, and each wrong guess costs
the frames between where the decoder starts and where it was asked to.
The same simulation, After only, since the schedule before took a key
frame every second for every video:

| Over the 23 drags, After                  | 2 s   | 10 s  | 12 to 75 frames |
| ----------------------------------------- | ----: | ----: | --------------: |
| Mean longest stand-still, ms              |  45.4 | 127.7 |           108.5 |
| Worst stand-still, ms                     | 105.0 | 549.0 |           301.0 |
| Mean exact frame after letting go, ms     | 105.8 | 295.6 |           151.6 |
| Worst exact frame after letting go, ms    | 254.0 | 566.0 |           302.0 |

A key frame every ten seconds costs up to 250 frames, about 1.2 s of
decoding, for a frame decoded from it, on any schedule.

## What follows

- The next report of a drag that lags has its places in the log, and
  becomes a drag of its own in the simulation.
- The cost of making a picture is a guess. A measurement of it on Tim's
  Mac settles the six slower drags either way.
- The key frames of a video could be read from its file once, the way
  the engine already asks ffprobe for the key frame before a cut, so
  the planner knows them rather than learns them.
- Two streams at once on Tim's Mac, from the decoder's lines in a
  report, say whether D is worth simulating.
