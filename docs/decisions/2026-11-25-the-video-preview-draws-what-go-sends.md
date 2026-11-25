# The video preview draws what Go sends

The next step after [Go plays the video preview](2026-11-19-go-plays-the-video-preview.md)
and [Go decides which frames the video preview gets](2026-11-19-go-schedules-the-video-previews-frames.md).
Those moved the decoding, the frames and the cuts of a play to Go. This
one moves what is drawn over a frame, the steering of a play and the
place of the playhead.

## The problem

The user, 25 November: "trim down the front end so that it becomes as dumb
and simple as possible, so that only data is rendered there, while all
of the data and behavior is basically served by the backend. So if
there is some fancy algorithm or some weird functionality implemented
in the front end, then it probably belongs into the backend." And: make
it "simpler, better, and faster at the same time".

Read on main at f20be21. The video preview's code in the window is
about 9,300 lines without its tests, `Player.svelte` 2,632 of them,
`frames/queue.ts` 1,933 and `frames/captions.ts` 1,010. Three groups in
it are rules and steering, not drawing:

- **A frame's dress is put together in the window.** A frame arrives
  bare. For the frame on screen the window then works out which piece
  holds it, `pieceHolding`, which way it is turned, `rotationAt`, a copy
  of `engine.RotationAt`, where its crop stands, `heldLeft` over the
  steps Go sent in `ClipEntry.cropSteps`, which keyframe stands on it,
  `markOn` and `frameOf`, a copy of the engine's `nearestFrame`, how wide
  the crop is on a turned picture, `pictureOf`, and whether the frame is
  inside the clip, `insideClip`. All of them are named under Not to copy
  in ARCHITECTURE.md. The captions over it come by a second road,
  `/frames/captions`, asked for each frame and up to three ahead by
  `CaptionLayers`, with a cache of its own. The crop and the captions
  then have to be held in step with the frame: `held`, `holdOverlay`
  and `holdLeftBehind` in `Player.svelte`, and `dress` and `dressed` in
  `queue.ts`, which wait for a frame's captions and, after `DRESS_WAIT`,
  put the frame up without them. Three bugs of the past came from this
  one cause: a card clicked moved the crop first, the picture a few
  frames later and the captions after that. Captions blinked off for a
  frame while a clip's start was dragged. A changed clip had its new
  pieces before its new captions and lit the wrong word, which
  `captionPieces` works around.
- **The play is steered from the window.** The window decides what a
  play plays, the clip's pieces or the video straight on, and when a
  clip changes while it plays it opens a play to take over a moment
  ahead of the sound, tries again when the sound got there first, and
  turns the play into one of the video when the playhead is left
  outside the clip: `setProgram`, `changed`, `takeOver`, `relooped`,
  `playOn` and `want` in `queue.ts`, about 400 lines, and `programOf`
  with the effect for a dragged edge in `Player.svelte`. The change
  itself is made in Go, by a gesture or an edit, so Go knows of it
  first, sends it to the window, and the window asks Go for a new play.
- **The playhead's place is decided in the window.** `lib/playhead.ts`
  says whether the playhead is on the clip, at its end or on the video,
  what the space bar plays from there and whether a play moved it, and
  `Player.svelte` works out the clip's own clock, `inClipTime`, and the
  word the keyboard walked to, `keyOf` and `wordAt`.

The frames the window keeps, the last 64 and those of the cards, are a
keeping policy too, but one for speed: a frame crosses into the page at
4 bytes a pixel. They stay until a measurement says otherwise. Drawing
on the canvas, light, the sound card, the clock and the buffers are the
window's work and stay.

## Five simulations

### 1. Who may know about whom

The window knows frames, moments and what the hand did. It knows no
rule about a clip. Go's frame side, `engine.Schedule` and the decoder,
knows episodes and frames and no clips. The app knows clips. So the
dress of a frame is answered by the app, from what it keeps of each
clip it sent the window, for a frame of the grid. The app already keeps
every caption drawing it hands the window by its track, `tracks` in
`captions.go`, and each drawing holds its clip. A clip with no words
has no drawing, so the app keeps each clip's `Framing` too, which it
already works out for a saved clip and for a clip under the hand alike,
by a name the framing carries. Layers: the window, the route, the app's
keep, the engine's rules. What crosses: one route, `/frames/dress`, in
place of `/frames/captions`, asked by the framing's name, the track when
there are captions, the frame and the size, answered by one header with
the dress and the layer as its body. What leaves the window: the
copies above, and `cropSteps`, which only the window's lookup read.
Next change: a new thing drawn over a frame is a field of the dress.
Cost: one route, one engine function, the window's lookups out.

### 2. What changes together

A frame's pixels never change. Its dress changes with every edit of the
clip, every step of a drag and every draft of a colour. So the two are
not one answer: a frame the window keeps is drawn again with a new
dress, and a new dress must not cost a new frame. The crop, the turn and
the captions do change together, by the same edit, and today come by
two roads, the clip's state and the caption layer, which is why they
can disagree on screen. Joined in one dress they cannot. Layers and
names as in the first. What it adds: the dress keyed by the frame and
the framing's name, so an edit gives a new name and a new dress, and a
frame kept in the window needs only the dress. Next change: an edit
that moves nothing over the frame keeps its name and asks nothing.
Cost: as the first.

### 3. Where state lives

The clip lives in its plan, and the app holds what it last sent. The
window holds a copy of the crop's steps over every frame of every glide,
sent in every state, only to look up one of them for the frame on
screen. The play lives in Go, `engine.Play`, but which play is the one
drawn lives in the window. The playhead's place lives in the window.
Moved to where the state is: the dress is worked out where the clip is,
the play is chosen and changed where the clip is changed, and the place
of the playhead is kept by the play that moves it. Layers: the window
sends what the hand did, the app keeps the clips it sent and the play of
each video, the engine works out dress, program and place. What
crosses: the dress route, a play asked for a clip by its framing's name
or for the video, the clock the sound card has reached with each pull,
and with a read, the place on the play's clock from which what follows
replaces what was sent. What leaves the window: the lookups, the
program and the take over, and in the end `lib/playhead.ts`. Cost: three
steps, one pull request.

### 4. What runs together

The frame drawn, its crop, its captions and the word lit are one moment
on screen. Today they run apart and are joined late. A play is a run of
moments, each frame stamped with where on the clock it goes, and each
stamp could carry its dress, so a play needs no second ask a frame. A
paused look is one frame, whose dress can come beside it. While a hand
drags an edge the play goes on and takes the change in, which today runs
Go, window, Go. If the app hands the change to the play the moment the
gesture makes it, it runs in one place. Layers as in the third. What it
adds: the dress inside a play's stream, which needs the play to know its
clip, so it follows the second step. Next change: anything drawn in step
with a play comes in its stamps. Cost: the dress route first, then a
play of a clip, then the dress in its stamps.

### 5. What a test has to build

A rule in the window is tested by a walk, or by a TypeScript test with a
copy of what Go would send. The dress as an engine function of a clip,
the source and a frame is a table test in Go with no decoder, beside
the render's own crop and turn, which it must equal: the same frame
dressed the way the render cuts it. The caption layer keeps the tests it
has. A play that takes a change in is a test of `engine.Play` with a
clock the test moves, as the play's tests already are, and the window's
part shrinks to dropping what it holds past a place, a unit test of the
queue with its clock and sound card handed in. What it removes: the
TypeScript tests of `rotate.ts`, `keyframes.ts`, `playhead.ts` and the
take over in `queue.test.ts`, and the walks that only checked the window
held the crop and the captions in step. Cost: tests move to Go, the
walks that test what a person sees stay.

## What converges

All five move the dress to Go and leave drawing, sound and the clock in
the window. Four of five want one answer for everything drawn over a
frame, keyed by the frame and by what the clip is now, and three of five
want the play to know its clip and take changes in itself. Two want the
dress inside a play's stamps, which only works once a play knows its
clip, so it comes after that step. Only the second keeps the dress
apart from the pixels on purpose, and that holds for a frame the window
keeps: kept frames are redressed, never sent again.

Dropped: the dress drawn into the frame's pixels by Go, which would send
a frame again for every edit and every colour dragged, and the window's
keep of 64 frames moved to Go, which waits for a measurement.

## The decision

Three steps, in one pull request, a commit each.

1. **Frames come dressed.** `engine` works out a frame's dress from a
   clip and the source: the piece that holds it, its turn, the crop's
   left edge and width on the picture as the render crops it, the
   keyframe on it and whether a drag may place the crop there, and
   whether it is inside the clip. The app keeps each `Framing` it sends
   by its name beside the caption drawings, and answers `/frames/dress`
   with the dress in a header and the caption layer as the body, or no
   body where the layer is the one the window has. `/frames/captions`
   goes. The window draws the frame with its dress, and keeps no rule
   about either.
2. **Go runs the play.** A play is asked for a clip by its framing's
   name, or for the video. A clip changed while it plays is handed to
   that play by the name of its new framing, with where the window's
   sound reached, `/frames/play/take`. The play takes it in a little
   past there, from the moment it stands at, and answers where, and
   whether the playhead was left outside the clip, so it goes on as a
   play of the video. Its frames and its sound say how many changes
   they were made after. The window drops what it holds of the clip as
   it was past that place and plays on. It opens no second play, tries
   nothing again and decides nothing about the pieces.

   Planned first, Go took the change in by itself, as a gesture or an
   edit made the framing, and said so with its next frames. A pull of
   frames comes every four frames, so that word came up to 160 ms
   late, past the 150 ms the place lies ahead of the sound. The window
   hands the play the framing it was sent instead, and has the answer
   in one call, the way its own take over had it.
3. **The playhead comes from Go.** A look and a play are asked for with
   what the gesture was about, the clip or the video, and Go answers
   with the place of the playhead, the moment on the clip's clock and
   the word there. The window shows them.

```
window      draws, plays sound, keeps the clock, sends what the hand did
   |
routes      /frames/look, /frames/play, /frames/read, /frames/dress
   |
app         keeps the clips and drawings it sent, the play of each video
   |
engine      dress, program, place, schedule, play, captions
   |
decoder     frames, sound, libass
```

## What follows

What it costs: about 2,000 lines leave the window, a route and an
engine function come in, and the play gains a way to take a change in.
What it buys: a frame can no longer show without its captions, the crop
and the captions cannot disagree, and the rules named under Not to copy
for the video preview go. What would show it was wrong: a dress ask
slower than a caption ask was, so frames wait longer for their dress,
or a play that stutters where the window's take over did not. Both are
measured on the bridge before and after each step.
