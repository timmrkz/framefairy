# The window holds the loudness, and Go sends it

Issue #909.

## The problem

Tim, report of 17 November, 22.30: "I was scrolling back and forth in the
clip timeline and there was a moment where the waveform of the audio was
just lagging to catch up and be rendered ... The waveform should always
be there instantaneously." Issue #909.

The first fix, #912, taught the window to read the waveform ahead of the
swipe. Tim, 18 November: "If we assume that the front end shouldn't have
any logic other than displaying information ... why would a dumb UI
layer have any kind of performance problems? ... I'm wondering what's
actually broken by design."

Read on main:

- The window holds no waveform. `ClipTimeline.svelte` asks Go for one,
  `Waveform(path, from, to, buckets)` in `cmd/framefairy-app/media.go`,
  for the view and one view either side, every time the view settles.
  Each answer is up to 4000 numbers as JSON across the bridge.
- To hide that round trip, the window keeps a fetch policy of its own:
  how far either side to read, a 150 ms rest before it reads, a counter
  that drops answers that come back out of order, and readings asked
  again as the transcript and the measuring grow. A swipe past one view
  runs into empty track until the hand rests and the answer lands. In
  the preview harness, with answers 40 ms late, a swipe of three views at
  five views a second showed empty track in 24 of 36 frames, and filled
  in 200 ms after the hand stopped.
- #912 made the policy bigger: a reading every quarter view, half a view
  more ahead in the swipe's direction, several readings out at once. It
  removed the lag on the swipes measured and left the design as it was:
  the window still decides when to ask Go for something that exists.
- The loudness is small. It is measured every 10 ms, so a two hour video
  is 720,000 readings. Go keeps it as `float32` in `logs/levels.frames`,
  2.9 MB for two hours, and `engine.Levels` holds it in memory.
- The view also tells Go where to measure first: `Waveform` calls
  `measuring.look`, and the measuring goes where the clip timeline looks.

## Five simulations

### 1. Where state lives

The loudness is the video's, and the window needs all of it to draw any
view at once. So the window keeps it, the way it keeps the clips Go
sends. Go sends it as bytes, once when the video opens and again while
it is being measured, on a route beside the frames,
`/frames/levels?path=`, one byte a reading. The window keeps a
`Uint8Array` per video and draws every view from it. A swipe or a pinch
asks nothing. Crossing the boundary: one route and its byte format. What
moves: `Peaks` stops being called for the window, the fetch policy in
`ClipTimeline.svelte` goes. Cost: one route, one encoder, one small
module in `frontend/src/lib/`, one pull request.

Zoomed out to a four hour video, a column of the clip timeline covers
thousands of readings, and finding the loudest of them for every column
on every frame of a pinch is 1.4 million comparisons a frame.

### 2. What runs together

What a person sees while the hand moves has to be ready before the next
frame, and no round trip across the bridge is. So nothing on the path of
a frame may wait on Go. That is simulation 1 seen from time: whatever the
frame draws is in the window's memory before the hand moves. It adds
that zooming out must also be ready in memory, so the loudest reading
over 2, 4, 8 and more readings is made beforehand, as levels, each half
as long as the one under it. Together they are about as long again as
the readings. A column then reads about forty of them at most at any
zoom, rather than the thousands of readings it covers. Who makes the levels: Go, since it makes the readings. They
are part of what the route sends.

### 3. Who may know about whom

The window draws and Go decides. Go could draw the waveform itself, as
pictures the way it sends the frames of the video preview, in tiles for
each zoom step. The window would show pictures and know nothing of
loudness. But a swipe still needs the tile ahead before it is there, so
the window would keep the same fetch policy, now for tiles, and a pinch
changes the zoom continuously, so every step of it is new tiles. It moves
the round trip, it does not remove it. Dropped.

### 4. What changes together

The loudness grows while it is measured, and the window learns of every
change through the video's state, `VideoState`, which already carries
`measuredParts`. So the loudness could ride in the state. But the state
is sent up to four times a second and is kept small on purpose, see
[2026-11-13-a-state-sends-only-the-clips-that-changed.md](2026-11-13-a-state-sends-only-the-clips-that-changed.md),
and 1.4 MB of JSON in it would undo that. What comes back: the state
says what is measured, and that is the window's signal to read the
loudness again. No new event, no poll.

### 5. What a test has to build

Today a test of the waveform needs the whole window, the bridge and a
held call, because the policy lives in a component. With the loudness as
bytes, Go's side is a function from `engine.Levels` to bytes, tested
alone with `go test`, and the window's side is a function from bytes and
a view to the height of each column, tested alone with Vitest. The walk
only checks what a person sees: every column painted after every step of
a swipe, and no call to Go while it swipes.

## What converges

Every simulation but 3 ends with the loudness in the window's memory
before the hand moves, sent by Go, and nothing on a frame's path asking
Go. Kept from 2: the levels for zooming out, made by Go. Kept from 4: the
window reads the loudness again when the state says more is measured,
and only then. Kept from 5: two small functions, each tested alone.
Dropped: tiles drawn by Go, which keep the round trip.

What stays a question for the window is only where the view is, and that
is the hand's, not a rule. Go still wants to know where the clip
timeline looks, to measure there first, so the window tells it the view
when it moves, a call that waits for nothing.

## The decision

```
the window     ClipTimeline.svelte  draws every view from the loudness it holds
               lib/loudness.ts      the bytes and a view to a level per column
                    |  Loudness(path)          (when the video opens, and
                    |                          when its state says more is
                    |                          measured or heard)
                    |  Look(path, from, to)    (the view at rest, for the
                    |                          measuring, waits for nothing)
the app's Go   media.go             Loudness and Look
               levels.go            the measuring, told where the view is
the engine     engine/levels.go     Loudness: the readings and the levels
                                    for zooming out, as bytes
```

The bytes: the number of readings as four bytes, little end first, then
a reading of -90 to 0 dB as 0 to 180, half a decibel a step, from the
start of the video, then the levels for zooming out one after another,
each the loudest of two of the one under it, down to a level of one. A
part not measured yet is -90, as `Levels.Over` already gives it, and
past the furthest part measured there is no reading, so the clip
timeline draws nothing there.

How it crosses, changed from simulation 1 while building it: a binding,
`Loudness`, rather than a route beside the frames. A binding is checked
by name and arguments like every call, in `bindings_test.go`, is said in
the log when it is slow, and is answered by the preview harness's
stand-in for Go, which keeps the measuring that grows while a probe
looks. Wails sends a `[]byte` as base64, a third more: two hours are
1.9 MB of text, which the window turns back into bytes in about 5 ms,
timed in Node in a cloud session, once when the video opens and about
twice a second while it is measured. Go makes the bytes in about 5 ms
for two hours, `BenchmarkLoudnessTwoHours`.

## What follows

It costs a call, an encoder and a module, and the `Waveform` binding,
`Transcript.Peaks`, its JSON answers and the fetch policy in
`ClipTimeline.svelte` go. A
video opened costs one read of about 1.4 MB for two hours from the local
disk, and again about twice a second while it is measured.

It buys a swipe and a pinch that never wait, at any speed and however
slow Go is, and a window that asks nothing while the hand moves.

It was wrong if the bytes take long enough to arrive that a video opens
with no waveform for longer than it does today, or if drawing a frame
from memory costs more than the frames budget in WebKit on the Mac.
