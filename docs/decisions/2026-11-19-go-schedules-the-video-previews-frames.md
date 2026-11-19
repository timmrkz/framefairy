# Go decides which frames the video preview gets

Issue #924, after #920.

## The problem

Tim, report of 18 November, 14.36: "the video preview sometimes lags to
render the frame at which I put the play head ... we must make sure that
the frame fairy app is instant." The first fix, #921, taught the window
one more rule. Tim, the same evening: "all the changes that you did were
just in the front end part ... if the front end is truly just a dumb
rendering layer, then there shouldn't be any performance issues in the
front end ... the fix of this pull request is very likely just a
workaround ... there is just so much that we can push on a string."
#921 was closed for this record.

Read from Tim's own logs, 10 to 18 November, on his M2 Max:

- 4815 jumps of the decoder on youtube.mp4, 1080p at 50 frames a
  second, took 48 ms from the open to the first frame on the median and
  110 ms at the 90th percentile, for 10 and 17 frames decoded. A line
  through them gives 25 ms to start and 3.2 ms for each frame decoded
  from the key frame. ffmpeg hands VideoToolbox one frame and waits for
  it before the next.
- Of 262 drags, 112 waited for the frame under the hand after letting
  go, 61 ms on the median, 148 ms at the 90th percentile and 999 ms at
  most. In 113 of the 230 drags of five moves or more, the picture stood
  over 100 ms at least once, against the mark of 50 ms in
  [VIDEO-PREVIEW.md](../VIDEO-PREVIEW.md#speed).
- In the 999 ms case Go had the frame out 53 ms after it was asked. The
  rest the page spent with the place parked behind a stream for a place
  the hand had left.

Read from the code on main:

- The window decides what Go decodes. `frontend/src/lib/frames/app.ts`
  lines 35 to 950, `AppFrames`, `Stream` and `Puller`, open up to three
  streams, `STREAMS`, guess whether a stream will reach a frame within
  1.5 s, `REACH`, start 15 frames back on a slow drag to the left,
  `BACK` and `SLOW`, ask for the key frame first while dragging, read
  ahead the first frames of the next and the last word and of 400 MB of
  clip cards, keep 40 frames, and park a new place while a stream for a
  place the hand left is still on its way, `stale` and `unpark`.
- Go holds the other half of the same schedule: `take` in
  `engine/episodeframes.go` gives a stream the cursor standing nearest
  before it, waits for one that is finishing, and the decoder in
  `cmd/framefairy-frames` decides between a seek and reading on, `goOn`.
- Neither half can stop the other. Closing a stream cancels its
  context, and `finish` then waits for the decoder to end the batch it
  was asked for. An open's first batch decodes from the key frame up to
  the frame asked for, up to 40 frames on youtube.mp4, and nothing ends
  it early. That is why the window parks: a new stream beside a stale one
  would decode beside it, not instead of it.
- Under all of it lies the shape of the frame queue of #110, which fed
  WebCodecs. `queue.ts` still feeds a decoder with samples, and
  `AppPictures` in `app.ts` answers it the way a `VideoDecoder` would,
  `decode`, `flush`, `decodeQueueSize`, from frames `AppFrames` fetches.

So the window keeps a decoder's schedule for a decoder it cannot see and
cannot stop. Each report since #281 tuned it: #502, #511, #596, #921.
Every table in VIDEO-PREVIEW.md shows a rule that helped one drag and
hurt another, because the cost it guesses at, where the key frames are
and what each cursor is doing, is Go's to know.

## Five simulations

### 1. Where state lives

What a choice of the next frame needs is where the key frames are, where
each cursor stands, what it is decoding now and what is decoded already.
All of it is in Go and in the decoder. So the schedule lives there, as one
type in the engine beside `EpisodeFrames`, and the window keeps what is
its own: where the hand is, the clock of the sound card and the frames it
draws. Layers: the window, a route, the schedule in the engine, the
decoder. What moves: `Stream`, `REACH`, `BACK`, `SLOW`, `stale`, the
parked places and the reads ahead out of `app.ts`. What crosses the
boundary: one call, the hand's place and how the hand is, answered by a
frame. Cost: an engine type, a route, the window's `AppFrames` mostly
removed, and a stop in the decoder, without which the schedule cannot
give up work either.

### 2. What runs together

The hand moves every 8 ms on Tim's screen, and the decoder takes 25 ms
and more for a jump. So the decoder must always work on the newest
place, and work for an old place must end the moment a newer one comes,
not at the end of its batch. That gives three needs. The decoder stops
between two frames when told, `stop` in `internal/framewire`, read by
the decoder at once rather than queued behind the work. The newest place
for a video replaces the one before it in Go, at once, without waiting
for an answer to come back to the window. And the window never waits on
an answer for a place it has left: an old answer that comes late is
dropped.

Weighed here: a channel open for as long as the video preview, the
window sending places on it and Go sending frames as they are made. It
saves a request per frame, about a millisecond, and needs a reply body
that streams, through Wails on WebKit, WebKitGTK and WebView2, and the
walks' bridge. Each request already brings one frame, and the request is
not where the time goes. Dropped for now, kept as the next step should
the request show up in the measurements.

### 3. Who may know about whom

The window should know nothing of key frames, cursors, streams and
batches, the words of a decoder. What it knows is what the hand does: it
moves over a place, it rests on a place, it plays from a place. So the
call says that and no more, and Go answers with a frame and whether it is
the exact frame or one near it, the key frame a drag shows on the way.
Showing a key frame while the hand moves and the exact frame when it
rests becomes Go's rule. The same holds for reading ahead: the first
frames of the next word and of the last are a rule about words, which Go
holds, and the cards on screen are what the window shows, which it says.
`AppPictures`, the `VideoDecoder` the window still pretends to talk to,
has no place in this either, once the play goes the same way.

### 4. What changes together

A play and a drag use the same decoder, the same cursors and the same
reading on, and the program of a play, which pieces in which order, is
cut by Go's own `cutOf` and `byTimes` and copied into the window in
`plan.ts`. They change together, so in the end both are Go's: the window
says play from here, Go sends the program's frames in order, and the
window draws each when the sound card's clock reaches it. That is the
largest step, with `plan.ts`, the two picture decoders and `AppPictures`
leaving the window. It is kept as the last step of this record, because
a paused video preview is where Tim waits and where the schedule's
guesses go wrong, and a play keeps up today.

### 5. What a test has to build

A rule of the window's schedule is proven today by a walk through the
whole app, and #921's first version made two walks hang. As a type in
the engine, the schedule is a function of the places it is told and of
what the decoder answers, tested in Go with the real decoder on the
committed fixtures, counted in frames decoded and frames handed over, as
`TestTheEpisodesDecoderDecodesAJumpNoFurtherThanItMust` does already. The
window's side is tested by what it draws for the frames it is sent. The
walks keep only what a person sees: the frame under the hand after a
drag and a click.

## What converges

All five put the choice of the next frame in Go, and none keeps it in
the window. Kept from 2: the decoder stops between two frames, and the
newest place replaces the one before it in Go. Kept from 3: the window
says what the hand does, not what to decode, and Go says whether a frame
is exact. Kept from 5: the schedule is tested in Go by frames counted,
not by walks. Kept from 4 as the last step: the play. Dropped from 2:
a channel that streams, until the request is shown to cost time.

## The decision

```
the window     queue.ts, AppFrames   says where the hand is and how: moving,
                                     resting, playing. Draws the frame it
                                     is sent, drops one for a place left
                    |  GET /frames/look   the place and how, answered by a frame,
                    |                     exact or near, or by nothing when a
                    |                     newer look took its place
the app's Go   frames.go             the route, one schedule per video
the engine     the schedule          the newest place wins, which cursor reads
                                     on and which seeks, the key frame while
                                     the hand moves, what to read ahead
               EpisodeFrames         the cursors, kept and handed on
                    |  internal/framewire   open, next, stop, close
the decoder    cmd/framefairy-frames decodes, and stops between two frames
```

The steps, each a pull request, each measured before and after:

1. The decoder stops between two frames when told, and a stream that
   is closed tells it, so its cursor is free at once instead of after its
   batch. With this record.
2. Go decides for a paused video preview: a click, a drag, a step. The
   route, the schedule, and `Stream`, `REACH`, `BACK`, `SLOW`, `stale` and
   the parked places out of `app.ts`.
3. Go keeps the frames it decoded around the playhead, so a place the
   hand has been costs nothing, and the window's 40 kept frames go.
4. Go reads ahead, the next and the last word and the cards on screen,
   into what it keeps, and the window's reads ahead go.
5. The decoder gets quicker: kept ready across a jump, several frames in
   the graphics chip at once. Measured by the decoder's own first frame
   line on Tim's Mac, since the cloud has no VideoToolbox.
6. The play: Go sends the program's frames, and `plan.ts`, the two
   picture decoders and `AppPictures` leave the window. It may need a
   record of its own when it comes, which then names this one.

## What follows

It costs a request in the decoder's wire, a route, a type in the engine,
and most of `app.ts`, about 900 lines, over five or six pull requests.
It buys an end to the waits that come from the window's guessing, up to
950 ms in Tim's report, and it makes the decoder's own speed worth
working on, since what it works on is then chosen by what knows the cost.
Estimated from the parts and Tim's numbers, not measured: a median jump
from about 61 ms to 15 to 20 ms once steps 2 to 5 are in, and no stand
over about 30 ms while dragging.

It would show it was wrong if the frames under a drag came no sooner
with Go deciding than with the window, measured the way
`speed.mjs DRAG=1` measures drags in the cloud and the let-go line on
Tim's Mac, or if the request per frame turns out to cost more than the
decoding, which would bring back the channel of simulation 2.
