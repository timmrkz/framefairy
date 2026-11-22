# Go plays the video preview

The issue of the lagging video preview, the last step of
[Go decides which frames the video preview gets](2026-11-19-go-schedules-the-video-previews-frames.md),
which said the play may need a record of its own.

## The problem

The user, 18 November: "if the front end is truly just a dumb rendering layer,
then there shouldn't be any performance issues in the front end". And on
19 November, of the steps of that record: "I would have hoped you just
made all of those changes in one pull request."

Since step 4, Go decides every frame of a paused video preview. A play
still runs on the window's rules. Read from the code:

- `frontend/src/lib/frames/plan.ts`, 844 lines, works out what a play
  plays: the program of the clip's pieces, once or in a loop, and a clip
  changed while it plays, `Program` and its splice. On it, `VideoPlan`
  works out the grid of moments each frame is drawn at, the runs of
  frames one decoder is fed in one go, and which frame is due at a
  moment. `AudioPlan` works out the packets of sound for each piece, and
  the fade at every cut, `FADE`, a copy of `Fade` in `engine/render.go`.
- `frontend/src/lib/frames/queue.ts`, 2467 lines, runs them. It feeds two
  picture decoders, `Slot`, the ranks of a run, `pump` and `feed`, keeps
  the decoded frames until they are due, `Keeper`, and draws the frame
  due on every tick of the sound card's clock, `tick`. 44 of its lines
  name `vplan` or `aplan`.
- The picture decoders are `AppPictures` in `app.ts`, which answers the
  calls of a WebCodecs `VideoDecoder`, `decode`, `flush`,
  `decodeQueueSize`, from frames `AppFrames` reads through streams of its
  own, `/frames/open` and `/frames/read`. The sound decoder is `GoSound`
  in `queue.ts`, a `/frames/sound` stream cut into the packets the plan
  asks for.
- So the window still needs the episode's table of frames, `Samples`, to
  count ranks, and a play and a paused look use the same decoder through
  two schedules that do not know of each other: the window's streams for
  a play, `engine.Schedule` for a look.

The rules of a cut are Go's: `cutOf` and `byTimes` in the engine say
which frames a piece has, and the render fades every cut by `Fade`. The
window holds a second copy of both.

## Five simulations

### 1. Where state lives

What a play needs to know is the clip's pieces, the loop, where the play
began, the episode's frames and where the decoder stands. All but the
sound card's clock is Go's already: the pieces are in the plan file, the
frames in the decoder. The clock is the window's, since the sound card is
in the webview. So Go works out the program and its frames, and the
window keeps the clock and what it draws. Layers: the window, a route,
`engine.Play` beside `engine.Schedule`, the decoder. What moves:
`Program`, `VideoPlan`, `AudioPlan`, `Keeper`, the two `Slot`s and
`AppPictures` out of the window. What crosses: a play asked for with the
pieces, the loop and where it starts, answered by frames stamped with
where on the program each is drawn, and sound stamped the same way, with
its fades made. Cost: an engine type, a route, most of `plan.ts` and a
third of `queue.ts` out.

### 2. What runs together

A play draws 25 to 60 frames a second, and a frame the window has not
got when it is due is a frame late. A request for each frame, the way a
look works, cost about 30 ms for a 2.3 MB frame in the cloud, more than a
frame. So a play is a stream, the way the sound is today: Go decodes the
program's frames in order and the window pulls them a batch at a time,
which keeps Go no more than a few frames ahead. The sound is a second
stream on the same program, from the same start. The clock stays the
window's: the sound card says what has been heard, and the window draws
the frame whose moment has come. Two names cross: open a play, read its
frames or its sound. A clip changed while it plays is a new play from
where the old one stands, on the program as it now is, with no gap
because Go has the frames before the change kept.

### 3. Who may know about whom

The window should know nothing of ranks, key frames, runs, decoders or
fades. What it knows is the clip it shows: its pieces, whether it loops,
where the playhead is. So it says play these pieces from here, and Go
answers with pictures stamped in program seconds and sound stamped the
same way. `Samples` leaves the window, and with it `rankAt`, `ticks` and
the frame grid. What is left in the window is what a person sees and
hears: draw this picture at this moment, play this sound now.

### 4. What changes together

The cut of a piece and its fade change together with the render, and a
play and a look change together with the decoder. Today the cut and the
fade are in two languages, `Fade` copied into `FADE`, and a change to
one has to be made in the other by hand. With the play in Go, the
preview plays the program by the same `cutOf` and `Fade` as the short,
and the play and the look share one owner of the decoder,
`engine.Schedule`, which stops a play the moment the hand takes the
playhead, and lets a look start a play from the frame it holds.

### 5. What a test has to build

`plan.test.ts`, 532 lines, and much of `queue.test.ts`, 830 lines, prove
the plan in TypeScript with made-up tables of frames. In Go, a play is a
function of the pieces and of what the decoder hands over: tested with
the held decoder of `engine/schedule_test.go`, frame by frame, and with
the committed fixtures for the sound's fades, against the render's own
cut. The window's side is a clock and a list of stamped pictures, tested
with a fake clock in Vitest. The walks keep what a person sees and hears:
a play moves the playhead once a frame, its sound is heard, a cut is
heard and seen where the short has it.

## What converges

All five put the program, its frames and its fades in Go, and the clock
and the drawing in the window. Kept from 2: a play is a stream pulled in
batches, not a request per frame, and the clock stays the sound card's.
Kept from 3: the window says pieces, loop and start, and gets pictures
and sound in program seconds. Kept from 4: one schedule owns the decoder,
so a play and a look never work against each other, and the preview cuts
and fades by the render's own rules. Kept from 5: the play is tested in
Go frame by frame. Nothing came up in only one.

## Three more designs, weighed after the review

The five above looked at one design from five sides. The review of the
pull request named three designs they did not weigh. Each was dropped,
by reasoning, none was built:

- **One push channel per play,** Go sending frames as it decodes them
  rather than the window pulling a batch at a time. A pull is one
  request for a batch, not one for each frame, and keeps Go no more
  than a few frames ahead, which a push would have to get back with a
  flow control of its own. Every other frame of the video preview comes
  through the asset routes, and Wails' events, the app's one other way
  from Go to the window, carry JSON, not 2.3 MB pictures.
- **Go owns the clock and plays the sound itself,** through the
  system's sound on each platform, and the window draws the frame Go
  says is due. It would take the sound buffers and the take over's lead
  out of `queue.ts`. But the picture is drawn in the webview, at
  moments Go does not see, so the window would need Go's clock across
  the bridge to draw by, late by a round trip each way, and picture and
  sound would drift by that. It would also bring a sound library for
  three platforms. The clock stays where the picture is drawn.
- **Go decides where a take over happens,** from what it handed over.
  The place has to be ahead of the sound heard, which only the sound
  card in the window knows. So the window names the place and Go says
  whether the new pieces hold it. What the review found wrong there,
  the old play stopped before the window had the new one, is fixed by
  the new play coming as a cue, see below.

## The decision

```
the window     queue.ts             plays the sound it is sent on the sound
                                    card and draws the picture whose moment
                                    the sound card has reached
                    |  /frames/play       pieces, loop, start: a play's id
                    |  /frames/read       its pictures in order, stamped in
                    |                     program seconds, a batch at a time
                    |  /frames/sound      its sound, faded at every cut
the app's Go   frames.go            the routes, one schedule per video
the engine     engine.Schedule      a look, a play and what is read ahead,
                                    one at a time on the decoder
               engine.Play          the program: which frames, in what
                                    order, with what sound, by cutOf and Fade
               EpisodeFrames        the cursors
the decoder    cmd/framefairy-frames
```

The steps, each a commit of the same pull request:

1. `engine.Play`: the program's frames and sound in order, tested in Go
   against the render's cut and fade.
2. The routes, and the schedule giving the decoder to a play.
3. The window draws a play from the stream: `VideoPlan`, `Keeper`, the
   slots and `AppPictures` go.
4. The sound from the stream: `AudioPlan` and the fades go, and
   `Samples` with them.

## What follows

It costs an engine type and its tests, a route, and about 1500 lines of
the window, most of `plan.ts`, a third of `queue.ts` and `AppPictures`.
It buys one owner of the decoder, the preview cut and faded by the
render's own rules, and a window that knows pictures and moments and no
longer the episode's frames.

It would show it was wrong if a play measured in the cloud with
`playback` and `speed.mjs` drew fewer of its frames on time than on
main, or started later, or if a clip changed while it plays was heard
to stop at the change.

## How the steps went

The steps went in as planned, with these changes, each found while
building it:

- **The sound is the play's second stream**, read with `/frames/read`
  like its frames, not a route of its own: the answer of `/frames/play`
  names both. Each chunk says the sample of the play's clock it starts
  at and how many moments it holds.
- **A cue is a play that waits.** A paused playhead cues a play with
  `cue=1`: it hands over the frames the look kept and its first sound
  at once, and takes the decoder only at `/frames/play/go`, which the
  space bar sends. Looks and the reading ahead go on until then. The
  window keeps a cue only where its first frame is the frame on screen.
- **Each frame says whether it is the first or the last of its
  piece**, which the window draws however short a time it is due for,
  since the frames either side of a cut say where the cut is.
- **A clip changed while it plays is a play that takes over**,
  `after` and `place` on `/frames/play`: Go works out the moment the
  play under way stands at that place on its clock, and the new play
  starts from it, counted on the old play's clock. It answers 409 where
  the new pieces do not hold that moment, and the play goes on as it
  was. The loop switched off is `/frames/play/loop` on the play under
  way, which ends it at the end of the time through the sound card has
  played into.
- **Steps 3 and 4 are one commit.** The picture and the sound of a play
  share the clock, the start and the takeover, and the window's old
  plan could not keep one of them while the other went.
- **`Samples` stays.** A look still names its frame by the grid, and
  the window turns the playhead into that frame and back, so the
  window keeps the grid's times. It no longer counts a play's frames.
- **What only the old play used went too:** the window's streams of
  frames, `Stream` in `app.ts`, the routes `/frames/open` and
  `/frames/sound`, the `skip` of a pull, and the times a stream said
  of its first frame, `engine.PreviewTimes` and `X-Frames-Times`, which
  no route sends any more.
- **A play the Go side closed starts again or is dropped.** The Go side
  closes a stream nobody has pulled from for 20 seconds, and a play
  paused or cued and left standing is one. Playing, it starts again from
  the playhead, and paused or cued it is dropped, so the space bar starts
  a play of its own.
- **A paused play gives the decoder back.** Kept, a play paused in its
  middle held the decoder, so nothing was read ahead while it stood.
  Closed and cued again from the playhead once its sound had faded, the
  space bar after a pause started a play of its own, 55 ms later than
  going on, measured in the cloud. So a paused play stays, holds its
  frames and its sound in the window, and gives only the decoder back,
  `engine.Play.Hold` and `/frames/play/hold`. Go takes it again, and each
  run goes on from the frame after the last it handed over, which the
  frames the window holds cover while the decoder gets there.
- **A play that takes over is a cue.** Taken at once, the Go side
  stopped the play under way while the window still pulled from it, and
  the window, told it was closed, started the play again. The new play
  now waits, and the window lets it go once it is the one drawn. Where
  the sound passed the place while Go answered, the place moves on.
- **A sleeping video keeps no frames on the Go side.** Its decoder
  stays a while for a video that comes back, two beside the one awake,
  and each kept up to 192 MB of frames with it.
