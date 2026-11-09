# Rotating a part of a clip, and keyframes

Plan rows 3.25 to 3.28 and 2.190 in [GUI-PLAN.md](GUI-PLAN.md). Rotate,
3.25, is built, see Rotate in [APP.md](APP.md), and so are keyframes,
3.26, see Keyframes there.

## What Tim wants to fix

A phone records a video in the shape it had when recording began. Tim
started in portrait, turned the phone to landscape while recording, and the
phone kept writing portrait. So the video has a part where the person
lies on their side, a quarter turn off, and the moments of the turn itself,
where the whole picture spins. He wants to mark that part with cuts, rotate
it upright, and place the crop on a few frames by hand with the movement
between them worked out for him. No numbers, no curves, no curve editor,
and nothing changes for a video that is fine as it is.

Later, on #750, Tim found a glide between keyframes hard to work with for
a camera switch, and asked for a keyframe as a turning point instead: the
crop switches on it and holds until the next. He then wanted the glide
back as a choice, and picked how from clickable mockups: two kinds of
keyframe, a diamond the crop switches on and a circle, with the crop
gliding from one circle into the next, each with its key and its button. That is what keyframes do now, see
section 4.

## What the app has today

- A clip is a list of pieces, `Segment` in `engine/clips.go`. Each piece
  has a start, an end and one crop, `CropX`, the left edge in pixels of
  the video. The crop is always the full height of the video and only
  moves sideways.
- The crop belongs to the clip, not the piece: `ClipSegments` in
  `engine/analysis.go` gives every piece the same crop, measured once from
  where the faces are, and `SetCrop` in `engine/edit.go` moves every piece
  together. One crop per clip, standing still for the whole clip. It once
  was one crop per camera angle, which #396 took out.
- A double-click in the clip on the clip timeline cuts a part out, and its
  edges are dragged to size. The render plays straight over it.
- `BuildFilterGraph` in `engine/render.go` builds one ffmpeg chain per
  piece: cut, crop, scale.

So most of what Tim needs is there. The cuts are the cuts that exist, the
crop is the crop that exists. Two things are missing: a piece cannot be
rotated, and a crop cannot move while a piece plays.

## The design

### 1. The spin is cut out, with the cut that exists

The seconds where the phone turns are no use to anybody: the picture spins
and no rotation of ours makes it stand still. So the first step is what Tim
already does to a pause: a double-click on the spin cuts it out, and the
cut's edges are dragged onto the first and last frame of the spin. When
the phone is turned back later, a second cut. That is the "two cuts" Tim
described, and it needs nothing new. What lies between two cuts, or
between a cut and an edge of the clip, is a **part** of the clip below.

### 2. Rotate: one key, one button, a quarter turn each

- **R**, and a **Rotate** button in the row under the clip timeline beside
  the thumbnail button, rotates the part the playhead stands in a quarter
  turn to the left. Press again for the next quarter, four presses bring
  it back. It is the rotate button of Preview and Photos, with their
  name, their icon, the arrow turning left, and their rule of a quarter
  at a time.
  The single letter follows I, O, L and T.
- The video preview rotates the picture at once, in the same frame as the
  press, and the crop frame stands on the rotated picture.
- **Undo** takes it back like any other edit, and so do three more
  presses.
- A rotated part has the rotate icon on the clip timeline, large, in its
  middle, in the accent at half strength with a dark glow round it, clear of the keyframes along the top
  rule and the thumbnails along the foot, issue #348, so a clip that
  holds a rotation says so without playing it.
- Nothing about it is a setting, and nothing is on screen until a part
  is rotated. A video that was never rotated looks and works as it does
  today.

What happens to the shape: Tim's video is portrait, 1080 by 1920. The
rotated part is landscape, 1920 by 1080, and the short is portrait, so the
crop is a 608 by 1080 window that moves sideways in it, exactly as the
crop of a landscape podcast does today. It is scaled up to the short,
which costs sharpness and nothing can avoid. The other way round, a
landscape video with a part rotated to portrait, the rotated part is already
the shape of the short and its crop is the whole picture, nothing to
place.

In the video preview a rotated part is drawn rotated, fitted into the same
viewer, with black where it is narrower. The viewer keeps the shape of the
video, so nothing moves or jumps as the playhead crosses a cut into a
rotated part.

### 3. A crop that follows: keyframes, made like thumbnails

Tim decided this on #188: a keyframe is made the way a thumbnail is,
with a button and a key, and dragging the crop frame only places the
crop.

- **K and G, and two keyframe buttons** in the row under the clip
  timeline beside the thumbnail button, put a keyframe on the frame under
  the playhead, with the crop where it stands there now: K a diamond, where
  the crop switches, and G a circle, between which the crop glides. With the
  playhead on a keyframe of its kind, the button or its key removes it,
  the way the thumbnail button does, and on one of the other kind it
  turns it into its own. Their icons are a diamond and a circle with a
  plus, and with a minus while the playhead stands on one of their kind.
  The shapes are Tim's: the diamond is pointed, an instant change, and the
  circle round and smooth, a glide.
- **A keyframe is a turning point**, Tim's word on #750. From its frame
  on, the crop is the keyframe's until the next keyframe. Before the
  first keyframe it is the clip's own crop. One diamond where the camera
  switches is a camera switch.
- **The crop frame is dragged only with the playhead on a keyframe**, Tim
  on #750, and moves that keyframe's crop. The keyframe is lit then, so
  what moves is seen. Anywhere else the crop frame shows the crop as the
  short has it, held or gliding, and a drag moves nothing. A clip with no
  keyframes has one crop, today's drag anywhere in it.
- **What is happening is shown while it happens.** While a clip plays,
  the crop frame in the video preview switches on the keyframe's own
  frame, so what plays is what renders.

### 4. One rule: a keyframe holds until the next

Each keyframe says where the crop is from its frame on, and the next
keyframe takes over from its own frame. Its kind says how the crop gets
there:

- **Into a diamond the switch is instant.** A camera switch is cut, not
  panned, so the crop switches on the keyframe's frame, with nothing in
  between.
- **From a circle the crop glides into the next circle**, over the whole
  time between them, easing out and in the way a camera operator pans,
  and reaches the second circle's crop on its frame. A glide runs only
  between circles, Tim's rule: a diamond is an instant switch with
  nothing before or after it, so into a circle after a diamond, or into
  the first, the crop switches. So a person who walks across the picture
  takes a circle where the walk starts and a circle where it ends.
- **A drag places one keyframe.** Dragging a keyframe along the clip
  timeline moves where its switch or the end of its glide is.
- **A keyframe holds across a cut, by the time the short plays.** A cut
  takes time out of the clip, so a keyframe before a cut keeps its crop
  after the cut until the next keyframe. A rotated part has a crop and
  keyframes of its own, so the rotate case needs nothing at all: the
  rotated part begins at the cut over the spin, and its crop is there from
  its first frame.

The first design glided the crop from one keyframe to the next over the
whole time between them, easing out and in, so the tempo was where the
keyframes were. One keyframe then stood still, and a camera switch took
two keyframes a frame apart. Tim found that odd on #750 and asked for the
keyframe as a turning point, with the glide kept between circles.
Mockups of five ways to offer it, a glide key, a menu on the keyframe, a
switch with a length, a pan across a part and a movement recorded while
playing, are kept in the project's files, and Tim chose the two keys.

### 5. Where the keyframes are on the clip timeline

The clip timeline already has the waveform, the caption blocks across its
middle and the thumbnail marks along its foot. Keyframes add no row of
their own:

- **A keyframe is the same mark as a thumbnail**, one component with
  another icon. A click puts the playhead on it, a drag moves it to
  another frame, a double-click removes it, and it lights under the hand
  and while the playhead is on its frame.
- **They stand at either end of the playhead's line.** The thumbnail is
  centred on the clip's bottom rule and the keyframe on its top rule, a
  diamond or a circle. So on the same frame both are on the frame's line,
  and each takes its own click, drag and double-click.
- **Two circles in a row are joined** by a line on the top rule, as thick
  as the rule, from the well's black brightening into the light colour at
  the second circle, so the two read as one glide over the clip's wash.
  Three in a row have two lines, and a diamond has none. Tim asked for the
  join and picked this look, number 4 of a sheet of nine drawn for him.
- **Automatic crop** removes every keyframe of the clip and brings back the
  placement the engine found.

### 6. What the engine can do by itself, later

The app should not need Tim to do even this much, so two rows follow the
two above:

- **It finds a rotated part.** The face finder already samples every clip
  for faces. A stretch where it finds none upright but finds them rotated a
  quarter is a rotated part, and the spin before it is where the picture
  changes all over many times in a second. The engine then proposes the cut
  over the spin and the rotation, and R or Undo takes them back.
- **It follows a subject that moves.** Where the faces in a clip do not
  settle on one place, the engine puts keyframes at a few moments
  itself instead of one. Marks made by
  hand override its keyframes.

## How it is built

- **The plan.** A rotation is kept for a part of the clip as its start and end
  in seconds of the video and a quarter count, the way caption times are
  kept against the video, so trimming a clip or moving a cut's edge does
  not lose it. The keyframes are kept with the clip as pairs of a
  moment in the video and a left edge. `editPlan` writes both, `LoadClips`
  checks both, and an old plan without them reads as no rotation and one
  still crop. The fuzz targets that read a plan read the new fields.
- **The engine.** `CropWindow` takes the rotated shape of a part.
  `BuildFilterGraph` puts `transpose` before the crop of a rotated piece
  and gives the crop a left edge that changes with the frame, an ffmpeg
  expression in the frame built from the keyframes, worked out on whole
  frames, see `holdExpr` in docs/ENGINE.md, so the render and the video
  preview agree on every frame.
  The face finder samples rotated frames for a rotated part, so its
  automatic crop is found on the upright picture.
- **The app.** The video preview rotates the frame it draws, with a
  transform on the canvas, and the crop frame reads its left edge from
  the steps the engine works out and sends with the clip, `CropSteps`, with a test that compares
  both on the same keyframes. The clip timeline draws a keyframe with the thumbnail's mark, one
  component with an icon of its own, and the rotate icon on a
  rotated part and its glow, through masks. `Rotate`,
  `SetCrop`, adding and removing a keyframe go through
  `Shape` and the undo history like every other edit of a clip.
- **Nothing for training.** Framing and rotating are not a judgement of a
  clip, so like the crop today nothing is recorded.
- **Proof.** A test video in `engine/testdata` with a part rotated a
  quarter: a render test that reads the short back and finds the part
  upright, a test that the crop of a render with keyframes is where they
  hold it on every frame, and walks that press R and K and drag the
  crop, checking the canvas and the plan, so Tim does not have to
  try it by hand to know it holds.
