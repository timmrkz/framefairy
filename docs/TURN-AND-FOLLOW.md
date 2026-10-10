# Turning a part of a clip, and a crop that follows

A proposal, not yet built. Plan rows 3.25 to 3.28 in
[GUI-PLAN.md](GUI-PLAN.md).

## What Tim wants to fix

A phone records a video in the shape it had when recording began. Tim
started in portrait, turned the phone to landscape while recording, and the
phone kept writing portrait. So the video has a part where the person
lies on their side, a quarter turn off, and the moments of the turn itself,
where the whole picture spins. He wants to mark that part with cuts, turn
it upright, and place the crop on a few frames by hand with the movement
between them worked out for him. No numbers, no curves, no keyframe editor,
and nothing changes for a video that is fine as it is.

## What the app has today

- A clip is a list of pieces, `Segment` in `engine/clips.go`. Each piece
  has a start, an end and one crop, `CropX`, the left edge in pixels of
  the video. The crop is always the full height of the video and only
  moves sideways.
- The crop belongs to the camera angle, not the piece: `ClipSegments` in
  `engine/analysis.go` gives every piece of the same shot the same crop,
  measured once from where the faces are, and `SetCrop` in
  `engine/edit.go` moves every piece of that angle together. One crop per
  angle, standing still for the whole clip.
- A double-click in the clip on the clip timeline cuts a part out, and its
  edges are dragged to size. The render plays straight over it.
- `BuildFilterGraph` in `engine/render.go` builds one ffmpeg chain per
  piece: cut, crop, scale.

So most of what Tim needs is there. The cuts are the cuts that exist, the
crop is the crop that exists. Two things are missing: a piece cannot be
turned, and a crop cannot move while a piece plays.

## The design

### 1. The spin is cut out, with the cut that exists

The seconds where the phone turns are no use to anybody: the picture spins
and no turn of ours makes it stand still. So the first step is what Tim
already does to a pause: a double-click on the spin cuts it out, and the
cut's edges are dragged onto the first and last frame of the spin. When
the phone is turned back later, a second cut. That is the "two cuts" Tim
described, and it needs nothing new. What lies between two cuts, or
between a cut and an edge of the clip, is a **part** of the clip below.

### 2. Turn: one key, one button, a quarter turn each

- **R**, and a **Turn** button in the row under the clip timeline beside
  the thumbnail button, turns the part the playhead stands in a quarter
  turn to the left. Press again for the next quarter, four presses bring
  it back. It is the rotate button of Preview and Photos, with their
  icon, the arrow turning left, and their rule of a quarter at a time.
  The single letter follows I, O, L and T.
- The video preview turns the picture at once, in the same frame as the
  press, and the crop frame stands on the turned picture.
- **Undo** takes it back like any other edit, and so do three more
  presses.
- A turned part shows a small turn mark at its start on the clip
  timeline, so a clip that holds a turn says so without playing it.
- Nothing about it is a setting, and nothing is on screen until a part
  is turned. A video that was never turned looks and works as it does
  today.

What happens to the shape: Tim's video is portrait, 1080 by 1920. The
turned part is landscape, 1920 by 1080, and the short is portrait, so the
crop is a 608 by 1080 window that moves sideways in it, exactly as the
crop of a landscape podcast does today. It is scaled up to the short,
which costs sharpness and nothing can avoid. The other way round, a
landscape video with a part turned to portrait, the turned part is already
the shape of the short and its crop is the whole picture, nothing to
place.

In the video preview a turned part is drawn turned, fitted into the same
viewer, with black where it is narrower. The viewer keeps the shape of the
video, so nothing moves or jumps as the playhead crosses a cut into a
turned part.

### 3. A crop that follows: places on the clip, the glide worked out

Today a drag of the crop frame moves the crop for the whole camera angle.
That stays exactly as it is. What is new:

- **Every placement is kept at the moment it was made.** A crop placed
  once stands still for the whole angle, which is today's behaviour.
- **A second place makes it move.** With the playhead on another frame,
  the crop is placed again (see the question below for how), and the crop
  now glides from the first place to the second. Before the first place
  it stands where the first one is, after the last it stands where the
  last one is.
- **The glide is ours, not the person's.** Between two places the crop
  eases out and eases in, the way a camera operator pans: slow from rest,
  smooth in the middle, slow into rest. There is no curve to choose and
  no number to type. To hold still for a while and then move, place the
  crop twice in the same spot.
- **The places show on the clip timeline** as small dots on the piece,
  in the accent's lighter shade, the colour of a cut's handles. A click on
  one puts the playhead there, the way a click on a cut's edge does, and
  a drag of the crop frame then moves that place. A double-click removes
  it, the way a double-click on a cut removes the cut. **Automatic crop**
  removes them all and brings back the placement the engine found.
- **What is happening is shown while it happens.** While a clip plays,
  the crop frame in the video preview follows the glide frame by frame,
  so what plays is what renders.

Turning and following are two separate things. The phone video needs
both, a handheld video that was never turned needs only the second, and a
podcast needs neither.

### The one question

How a second place is made, so a podcast that is only nudged twice does
not start to pan:

- **A: Option-drag makes another place (recommended).** A plain drag
  moves the crop of the whole angle, today's behaviour, untouched. Holding
  Option while dragging adds a place at the playhead, the way Option-drag
  in Finder makes another copy instead of moving. Every existing clip and
  every habit stays as it is.
- **B: Every drag at a new moment is a new place.** One rule, nothing to
  hold, but re-placing the crop of a podcast at a later moment makes it
  glide from the old place to the new one, and the old dot has to be
  removed to stop it.

### 4. What the engine can do by itself, later

The app should not need Tim to do even this much, so two rows follow the
two above:

- **It finds a turned part.** The face finder already samples every shot
  for faces. A shot where it finds none upright but finds them turned a
  quarter is a turned part, and the spin before it is where the shot
  detector fires many times in a second. The engine then proposes the cut
  over the spin and the turn, and R or Undo takes them back.
- **It follows a subject that moves.** Where the faces in a shot do not
  settle on one place, the engine places the crop at a few moments
  itself instead of one, and the glide does the rest. Places made by
  hand override its places.

## How it is built

- **The plan.** A turn is kept for a part of the clip as its start and end
  in seconds of the video and a quarter count, the way caption times are
  kept against the video, so trimming a clip or moving a cut's edge does
  not lose it. The places are kept with the camera angle as pairs of a
  moment in the video and a left edge. `editPlan` writes both, `LoadClips`
  checks both, and an old plan without them reads as no turn and one
  still crop. The fuzz targets that read a plan read the new fields.
- **The engine.** `CropWindow` takes the turned shape of a part.
  `BuildFilterGraph` puts `transpose` before the crop of a turned piece
  and gives the crop a left edge that changes with the frame, an ffmpeg
  expression in `t` built from the places, worked out on whole frames and
  even pixels, so the render and the video preview agree to the pixel.
  The face finder samples turned frames for a turned part, so its
  automatic crop is found on the upright picture.
- **The app.** The video preview turns the frame it draws, with a
  transform on the canvas, and the crop frame reads its left edge from
  the same glide the engine uses, in `lib/`, with a test that compares
  both on the same places. The clip timeline draws the dots and the turn
  mark. `Turn`, `SetCrop` with a place, and removing a place go through
  `Shape` and the undo history like every other edit of a clip.
- **Nothing for training.** Framing and turning are not a judgement of a
  clip, so like the crop today nothing is recorded.
- **Proof.** A test video in `engine/testdata` with a part turned a
  quarter: a render test that reads the short back and finds the part
  upright, a test that the crop of a render with two places is where the
  glide says on chosen frames, and walks that press R and drag the crop
  with Option, checking the canvas and the plan, so Tim does not have to
  try it by hand to know it holds.
