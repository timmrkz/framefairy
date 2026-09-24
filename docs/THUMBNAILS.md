# Thumbnails

A spec. Nothing here is built yet. The batches are 1.5 in
[GUI-PLAN.md](GUI-PLAN.md).

Every short gets a thumbnail: one picture that stands for it. The engine
proposes it, the clip card shows it, a person can pick another in a second,
and **Render** writes it beside the video, in the same folder, under the
same name.

## What a thumbnail is for

- **YouTube.** Since 24 July 2026, channels in the YouTube Partner Program
  can upload their own thumbnail for a Short, in YouTube Studio on the
  desktop. The rollout is gradual. Channels without it pick one of three
  suggested frames, or a frame of the Short on the phone. A Short's
  thumbnail is what shows everywhere outside the swipe-up player: the
  channel grid, the Shorts shelf, search, subscriptions and the homepage.
- **Instagram.** A Reel's cover is uploaded or picked from the video, and
  can be changed after it is live. The Reel plays at 9:16, but the profile
  grid crops the cover to 3:4, which takes 240 pixels off the top and 240
  off the bottom of a 1080 by 1920 picture.

So one picture serves both: 1080 by 1920, the size of the short, with the
face inside the middle 3:4 of it. For a channel that cannot upload yet, the
same choice still helps: it names the frame to pick.

## What a thumbnail is

**One moment of the clip.** It is kept in the clip's plan as a time on the
episode, like the edges of the clip, in a field `thumbnail`. The picture is
the frame at that moment with the clip's crop: the same pixels the short
shows at that moment, without the captions. A caption on a cover is caught
mid-sentence and reads as a mistake.

The moment is untrusted like everything else in a plan. `LoadClips` accepts
it only as a number inside a kept piece of the clip. Anything else is read
as no choice, and the engine's own proposal stands. It moves with the clip:
a trim or a cut that leaves the moment outside what is kept puts the
thumbnail back on the engine's first proposal, the way a moved caption falls
back when its word is gone.

## What the engine proposes

Three moments per clip, worked out when the clip is framed, from frames the
framing already decodes. A moment scores for:

- **a face towards the lens**, found by the face finder the crop already
  uses. It only finds faces turned towards the camera, which is what a
  cover wants anyway. Larger is better.
- **the face inside the middle 3:4** of the crop, so the Instagram grid does
  not cut it.
- **sharpness**, so a head in motion loses to one at rest.
- **distance from a cut or a change of camera**, at least a fifth of a
  second, so no picture is half of two shots.

The three are at least three seconds apart, so they are three different
pictures rather than one picture three times. The best is the thumbnail
until someone picks another. The same clip always gets the same three.

## In the app

**The clip card shows the thumbnail**, on its left, at the full height of
the card and in the shape of a short. The title and the numbers move to its
right. It is the picture the render will write, crop and all. While the
clip is still being framed, the place for it wears the shimmer, the fourth
of the five ways work in hand is shown.

**The thumbnail is chosen on the clip timeline, where every other moment of
a clip is chosen.** Choosing it in the card was considered and left out:
three pictures the size of a stamp in a narrow list are a poor way to judge
a face, and the video preview is right there, as big as the app allows.

- **The thumbnail is a mark** at its moment on the clip timeline, in the
  accent colour. The other two proposals are quieter marks of the same
  shape.
- **A click on a proposal makes it the thumbnail.** The card changes at
  once.
- **The mark is dragged**, and the playhead goes with it, so the video
  preview shows the frame under the hand, crop frame included, and the card
  follows on the way. It is saved when the hand lets go. It lands on a whole
  frame and stays inside the kept pieces, because a moment that was cut is
  not in the short.
- **T puts the thumbnail at the playhead**, for someone who found the
  moment while playing. The mark says so in its `title`.
- **Undo takes a choice back**, like any other edit of the clip. The
  proposals stay as marks, so going back to the engine's pick is one click
  too.

Nothing else is added: no button, no dialog, no strip of pictures.

## On render

**Render** writes `<name>.jpg` beside `<name>.mp4`, in the same folder,
so **Show in folder** shows both and they are uploaded together. It is the
size of the short, 1080 by 1920 unless `--width` and `--height` say
otherwise, a JPEG under 2 MB, which is YouTube's limit for a thumbnail. It
is taken from the original episode, never from a playback copy.

A thumbnail chosen after the render rewrites only the picture. That takes a
moment, where a new render of the short would take minutes, and the short
stays rendered.

The command line writes the picture too, from the plan. The moment can be
set there by editing `thumbnail` in the plan file.

## Not in the first version

- **The title written on the picture.** Covers with a line of text on them
  are common. It is a design of its own, with a face, a size and a place, so
  it waits until the plain picture is in use.
- **A training signal.** Which frame someone picks says nothing about which
  clip is worth making, so nothing is recorded.

## Batches

| # | Batch |
| --- | --- |
| 1 | `thumbnail` in the plan, checked by `LoadClips` and covered by its fuzz target, set through `editPlan`, and the picture of a moment with the crop applied |
| 2 | The three proposals, worked out while a clip is framed |
| 3 | Render writes the picture, and a new choice after a render rewrites only the picture. The command line too |
| 4 | The picture in the clip card, with the shimmer while it is made |
| 5 | The marks on the clip timeline: click, drag with the playhead, T, undo |
