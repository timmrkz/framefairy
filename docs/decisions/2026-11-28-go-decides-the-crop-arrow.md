# Go decides the arrow in the crop frame's corner

The arrow in the crop frame's top right corner brings the crop back to
where the app places it, and then puts it back where the hand had it.
This record says which side decides what about it, and where the way
back is kept.

## The problem

The user, 28 November, after three versions of the arrow:

> I want to know how good or bad the software design and the
> architecture is for this change, because the fact that you're just
> tinkering across multiple tries and your explanations on how this
> actually works makes me think that the implementation is subpar and
> will probably make maintenance worse, because the solution is maybe
> brittle or put in the wrong place, like rules added to the front end
> again, where actually the back end should have the only authority
> about what has to happen when and how. Let's make sure that the front
> end is just a very dumb representation layer that's only rendering.

And before that, on what the arrow is for:

> Putting the crop frame back into its automatic position should not do
> more than just moving the crop frame into a different place, which is
> the default place that was assigned by the engine, but that should
> also only happen if the crop frame is actually out of this default
> position. The feature here is not allowed to touch key frames or
> anything in the clips themselves.

Read on the branch at ae3ab79, main c7fb725. The independent review of
that commit rated the architecture worse.

**Go sends facts, the window decides.** Each frame's dress,
`engine.FrameDress`, says where the app places the crop, `Auto`, whether
the crop stands elsewhere, `Moved`, and whether a drag may place it,
`Movable`. `Player.svelte` then decides which arrow shows,
`moved && movable` or the way back, what a click sends, `SetCrop` at
`cropMoment()` with `dress.auto`, what it keeps to put back, `undoCrop`,
and when it forgets it, only after a drag or a put back.

**The way back lives in the window.** `undoCrop` holds the clip, the
turn, the moment, the crop before and the clip's framing before. The
window names that framing to Go in each ask for a dress, `back=`, so Go
can say whether putting back would move the frame, `sentDress.Back`.
The framing is looked up in a cache of 512, and the answer is about that
framing, not about what the click will do. So the arrow showed where its
click was refused, after S was pressed elsewhere, a second click on
Automatic crop kept the crop after the reset as the way back, and a
refused call still changed what the window kept.

**The reset moved into the window.** It was `engine.ResetCrop` and the
app's `ResetCrop` call. The pull request removed both, because the reset
removed every keyframe, and the window now puts the reset together out
of a `SetCrop`.

## Five simulations

### 1. Who may know about whom: the window knows nothing about crops

The window knows a dress and two buttons. Go adds one field to the
dress, `Corner`, `"auto"`, `"back"` or nothing, and two calls,
`ResetCrop(path, plan, clip, at)` and `PutCropBack(path, plan, clip)`.
`ResetCrop` reads the clip's framing, works out the dress at `at`, does
nothing where the corner offers no reset there, and otherwise places the
crop where the app does, with `engine.SetCrop` on the moment that holds
the crop, and keeps the way back. `PutCropBack` places the crop kept.
The way back is kept by the app's Go side, in memory, beside the video's
undo steps: the clip, the moment, the crop before, the framing before
and the name of the framing after. It holds while the clip's framing is
still the one the reset left, which every dress ask names already, so
any other change to the clip's framing, an undo of the reset included,
ends it, and a redo of the reset brings it back.

Crossing the boundary: one field of the dress and two calls, where the
window today sends `back=`, keeps `undoCrop` and reads `auto`, `moved`
and `back`. Next change, the same arrow for the caption box: one more
corner value and one more kept way back, in Go. Cost: one pull request,
this one.

### 2. Where state lives: the way back in the plan

The work folder is the truth about a video, so the way back is kept in
the plan, beside `crop_x_auto`: `crop_x_hand` on the pieces and `hand`
on a keyframe. The engine then decides the corner from the plan alone,
and the way back outlives a restart.

What moves: a new key read by `LoadClips` and checked as untrusted
input, a field on `CropMark` carried through `carry`, `opened`,
`realise`, `settled` and `setCropMarks`, about ten functions of
`engine/cropmarks.go`, and the command line sees a key it has no use
for. Every edit of a crop has to decide whether it ends the way back.
Next change, the caption box: its height is a setting of the app, not a
plan's, so this design does not carry over. Cost: the largest of the
five, for a way back nobody asked to keep across a restart.

### 3. What changes together: Put the crop back is an undo

Put the crop back is the undo of the reset's step. The corner offers it
while the reset is the newest step of the video's history, and its
click is `Undo`. Nothing new is kept.

What moves: a step learns it was a reset, the corner reads the history.
But an edit of another clip, or the window moved on the range picker,
both steps of the same history, take the way back away from a clip they
did not touch, and Cmd+Z and the arrow then do the same thing two ways.
Cost: small, and a coupling the user would feel.

### 4. What runs together: the corner worked out with the frame

The corner is worked out where the rest of the frame's dress is, in
`engine.FrameDress`, once a frame. That needs the way back as an input
of the engine, so it has to be kept somewhere first, by 1 or by 2. It
comes to the same as 1, with the engine handed the framing kept. The
part that stands on its own: the engine already says per frame whether
a reset would move the crop, `Moved` and `Movable`, and how a framing
dresses a frame, `DressAlone`. The app's Go side only puts those
together with what it keeps.

### 5. What a test has to build

A test of today's design needs the window, the walk, to prove that the
arrow and the click agree, because the window decides both. With the
decisions in Go, a Go test builds a library in memory, asks the dress
handler for the corner, calls `ResetCrop` and `PutCropBack`, and checks
the corner again: after an edit elsewhere, after a second click, after a
refused call, after an undo and a redo. The walk only shows that the
window draws the corner and forwards the click. This favours 1 and 2
over today, and 1 over 2, whose tests have to write plans with the new
keys.

## What converges

- Go decides whether the corner shows, which arrow, and what a click
  does, in all five. The window draws one field and forwards a click
  with the frame's moment, in all five.
- The reset and the put back are calls of the app's Go side, made of
  `engine.SetCrop`, and never put or remove a keyframe, in all five.
- The way back is kept by the app's Go side, in memory, in 1, 4 and 5.
  Kept in the plan, 2, costs a plan key for a way back nobody asked to
  keep, and does not carry over to the caption box. Kept as an undo
  step, 3, ties the arrow to every other step of the video.
- The way back is valid while the clip's framing is the one the reset
  left, by its name, which the dress ask carries already. So it needs no
  rule for each kind of edit that ends it.

## The decision

Simulation 1, with the corner worked out in the app's Go side from the
engine's facts, 4.

```
window       Player.svelte draws dress.corner, and a click sends
             ResetCrop or PutCropBack with the clip and the frame's moment
                 |
app Go side  ResetCrop, PutCropBack, the way back kept per video
             (cropBacks in history.go), the corner of each dress
             (dressFrame in dress.go)
                 |
engine       FrameDress: Auto, Moved, Movable, the moment that holds the
             crop, SetCrop, DressAlone
```

The window sends no `back=` and keeps no `undoCrop`. The dress it is
sent carries `corner` and no longer `auto` or `moved`, which only Go
reads.

## What follows

It costs a row in Where state lives, the way back in memory beside the
undo steps, and the two calls. It buys a window that cannot offer an
arrow Go would refuse, and tests of every case in Go. It removes the
window's `undoCrop`, its `back=` ask and the rules of `cropUndo`.

It would show itself wrong if the corner and the click ever disagreed
again, or if the same kind of way back, for the caption box, could not
be kept the same way.
