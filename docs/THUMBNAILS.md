# Thumbnails

A thumbnail is a picture that stands for a short, for the upload. A person
picks any number of them for a clip, none, one, three or sixteen, as marks on
the clip timeline, and **Render** writes each one as a JPEG beside the short,
in the same folder. The batches are 1.5 in [GUI-PLAN.md](GUI-PLAN.md), and how it is used is
in [APP.md](APP.md#thumbnails).

## What a thumbnail is for

- **YouTube.** Since 24 July 2026, channels in the YouTube Partner Program
  can upload their own thumbnail for a Short, in YouTube Studio on the
  desktop. The rollout is gradual. Channels without it pick a frame of the
  Short. A Short's thumbnail is what shows everywhere outside the swipe-up
  player: the channel grid, the Shorts shelf, search, subscriptions and the
  homepage.
- **Instagram.** A Reel's cover is uploaded or picked from the video, and
  can be changed after it is live. The profile grid crops it to 3:4, which
  takes 240 pixels off the top and 240 off the bottom of a 1080 by 1920
  picture.

## What a thumbnail is

**A frame of the short, exactly as the short shows it**: the crop, the
captions and the highlight, if there is one. It is the screenshot of a
frame that was liked in the finished video, which is how thumbnails were
made by hand before, and it needs nothing to be decided about how a picture
should look apart from how the short looks.

Whoever wants the captions without the bouncing highlight turns the
highlight off in the caption settings, and the short and its thumbnails
lose it together. See [APP.md](APP.md#the-workspace).

A picture without captions is left out on purpose. It is a second look to
choose between, and it can come later if it is missed.

## Where it is kept

In the clip's plan, as the moments of the episode the pictures are taken
at, in a list `thumbnails`, in seconds. The list is untrusted like the rest
of a plan: `LoadClips` keeps a moment only if it is a number inside a piece
the clip keeps, at most 50 of them, each once, in time order. A trim or a
cut that leaves a moment outside the clip takes that thumbnail out of what
is shown and rendered. It stays in the file, so putting the cut back brings
it back.

## In the app

- **The thumbnail button**, in the row under the clip timeline between loop and
  the crosshair, makes the frame under the playhead a thumbnail.
  **T** does the same.
- **Every thumbnail is a mark on the clip timeline**, at its moment, in the
  app's colour.
- **A click on a mark** puts the playhead on it, so the video preview shows
  the frame.
- **A mark is dragged** to another frame, and the playhead goes with it, so
  the video preview shows the frame under the hand. It is saved when the
  hand lets go, it lands on a whole frame and it stays inside the pieces the
  clip keeps.
- **With the playhead on a thumbnail**, the button is lit and a click on it,
  or **T**, removes that thumbnail. What one click adds, one click takes
  away.
- **Undo** takes back adding, moving and removing, like any other edit of
  the clip.

## On render

**Render** writes the short and then its thumbnails, as
`<name>-1.jpg`, `<name>-2.jpg` and on, in time order, beside `<name>.mp4`.
Each is taken from the short that was just written, so it is exactly a frame
of it, 1080 by 1920 unless `--width` and `--height` say otherwise. Thumbnails
of an earlier render that are no longer wanted are removed at the same
time, so the folder always holds what the plan asks for.

There is no way to render the pictures alone. A change to the thumbnails is
a change to the clip, and the next render writes the whole of it again.

The command line does the same from the plan. There the moments are set by
editing `thumbnails` in the plan file.

## Not in the first version

- **The thumbnail in the clip card.** The card would show the first one. It
  waits until the pictures are in use.
- **Proposals from the engine.** Frames with a face towards the lens and no
  cut nearby could be suggested. Picking by hand comes first.
- **A training signal.** Which frame someone picks says nothing about which
  clip is worth making, so nothing is recorded.

## Batches

| # | Batch |
| --- | --- |
| 1 | `thumbnails` in the plan, checked by `LoadClips` and covered by its fuzz target, set through `editPlan` |
| 2 | Render writes the pictures from the short, and removes those no longer wanted. The command line too |
| 3 | The thumbnail button, T, and the marks on the clip timeline: click, drag with the playhead, undo |
