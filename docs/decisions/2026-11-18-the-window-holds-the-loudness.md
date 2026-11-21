# The window holds the loudness, and Go sends it

Partly replaced by [The waveform keeps up with the hand](2026-11-21-the-waveform-keeps-up-with-the-hand.md).

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
window reads the loudness again when the state says it changed, and only
then. Kept from 5: two small functions, each tested alone. Dropped:
tiles drawn by Go, which keep the round trip.

What stays a question for the window is only where the view is, and that
is the hand's, not a rule. Go still wants to know where the clip
timeline looks, to measure there first, so the window tells it the view
when it comes to rest, a call that waits for nothing.

## Weighed after the review

The review of the first version found that simulations 1, 2, 4 and 5 are
one design seen four ways, and that three choices were made without
being weighed. They are weighed here.

**Who says when to read again.** The first version read the loudness
again when `measured` grew, or `heard` grew while not all was measured.
That is `Levels.Over`'s rule, which loudness lies under which, copied
into the window. Go now says it: the status carries `loudness`, a stamp
that changes whenever what `engine.Loudness` gives would, and the window
reads again when the stamp changes, `HeldLoudness` in
`frontend/src/lib/loudness.ts`.

**Only what grew.** Go could send the parts measured since the last
reading, and the window lay them into what it holds. That is fewer bytes
while a video is measured, but the window would then keep the parts,
merge them, and make the levels for zooming out again, which is the
engine's work done in the window, and two copies of what is measured to
keep in step. The whole is at most 2.9 MB for four hours, read about
twice a second for the seconds a video is measured. Dropped.

**A binding or a route.** The first version sent the bytes through a
binding, which Wails carries as base64 in JSON, a third more, turned
back into bytes on the window's main thread, about 11 ms for four hours.
A route beside the frames, `/frames/levels`, carries them plain, and the
window takes the answer as it is, with no copy. Taken. The preview
harness's stand-in for Go answers the route itself, since the loudness
it measures lives there.

**How the two sides keep one format.** The head of the bytes says how
long a reading is, the floor and how many steps make a decibel, so the
window keeps no copy of any of them. What is left is the layout, and the
window takes bytes that do not add up to exactly it as no loudness at
all. A walk draws what the real Go side sends and fails on a waveform
that is not there, so the two cannot drift apart unseen. A shared file
of cases for both sides was not taken: it is a second copy to keep in
step, not a check.

## The decision

```
the window     ClipTimeline.svelte  draws every view from the loudness it holds
               lib/loudness.ts      the bytes and a view to a level per
                                    column, and HeldLoudness, which reads
                                    them when Go's stamp changes
                    |  GET /frames/levels?path=  (when the video opens, and
                    |                            when the status's loudness
                    |                            stamp changes)
                    |  Look(path, from, to)      (the view at rest, for the
                    |                            measuring, waits for nothing)
the app's Go   media.go             /frames/levels and Look
               levels.go            the measuring, told where the view is
the engine     engine/levels.go     Loudness: the readings and the levels
                                    for zooming out, as bytes
               engine/episode.go    the stamp of the loudness in the status
```

The bytes: a head of eight, the number of readings as four bytes and how
long one is in microseconds as two, little end first, the floor in dB as
a signed byte and the steps in a decibel as one. Then a reading of -90
to 0 dB as 0 to 180, half a decibel a step, from the start of the video,
then the levels for zooming out one after another, each the loudest of
two of the one under it, down to a level of one. A part not measured yet
is -90, as `Levels.Over` already gives it, and past the furthest part
measured there is no reading, so the clip timeline draws nothing there.

## What follows

It costs a route, an encoder, a stamp and a module, and the `Waveform`
binding, `Transcript.Peaks`, its JSON answers and the fetch policy in
`ClipTimeline.svelte` go.

Opening a video costs more than it did. It read the waveform of three
views, 3000 numbers as 33 KB of JSON, and now reads the whole loudness,
1.4 MB for two hours and 2.9 MB for four. Measured in a cloud session,
5 runs of a Go benchmark and 10 fetches in Chromium over the loopback:
Go made the answer in 0.16 ms and now in 2.0 ms for two hours and 4.2 ms
for four, and Chromium fetched it in 2.2 ms and now in 8.1 ms and 14.4
ms. While a video is measured that read comes again about twice a
second, off the window's main thread. How long WebKit on a Mac takes to
carry 2.9 MB through Wails was not measured.

It buys a swipe and a pinch that never wait, at any speed and however
slow Go is, and a window that asks nothing while the hand moves.

It was wrong if the bytes take long enough to arrive that a video opens
with no waveform for noticeably longer than it does today, or if drawing
a frame from memory costs more than the frame's budget in WebKit on the
Mac.
