# Captions, drawn once

The captions of a clip are seen in three places: in the video preview while
the clip is made, in the thumbnails, and in the short. They are one drawing
in all three, pixel for pixel, the bounce of the word being spoken and the
padding of its pill included. Not three drawings made to look alike, which
drift the day one of them changes: Tim saw the video preview show the pill
settled on a frame where the short showed it on its way up, and the
thumbnail darker than the short, with less room round its word. This file
says how the one drawing is kept one. The batch is 2.211 in
[GUI-PLAN.md](GUI-PLAN.md).

## Who draws

**libass, and nothing else.** The captions are a caption file, made by
`Engine.CaptionTrack` in `engine/ass.go` from the words a clip shows, see
[WORDS.md](WORDS.md), its style and the light of the episode, and laid out
at the size of the short. libass, through ffmpeg's `ass` filter, draws that
file:

- **The short.** The render burns the file in, `BuildFilterGraph` in
  `engine/render.go`, handing libass each frame's time in whole
  milliseconds, see [ENGINE.md](ENGINE.md#captions-on-the-frames-they-belong-to).
- **The thumbnails.** A thumbnail is a frame cut out of the finished short,
  the frame that holds the marked moment, in the short's colours, see
  [THUMBNAILS.md](THUMBNAILS.md). It draws nothing of its own.
- **The video preview.** The episode's decoder, `framefairy-frames`, has
  the same libass, built with the ffmpeg we ship. For each frame shown it
  draws the clip's caption file into a layer at the moment the short reads
  on that frame, and the screen lays the layer over the frame.

## The moment

The short does not run on the episode's clock. Its frames are the frames
of the clip's pieces one after another, from nought, and libass reads each
frame's start in milliseconds the way the render hands it over.
`SourceInfo.CaptionMoment(clip, at)` is that millisecond for the frame of
the episode that holds `at`: the frame of the short that shows it,
`shortFrame`, a frame in a cut taken as the first one after it. So the
video preview asks libass for the same moment the render does, and the
pill is at the same stage of its bounce on the same frame.

## The size

There is one rule for the size of the short, `Options.RenderSettings()`,
with `RenderSettings.FrameSize` for the size of its frames in a piece. The
render takes both from there, and so does the video preview, through
`jobOptions` in `cmd/framefairy-app/jobs.go`, the options a render job runs
with. The caption file is laid out at the size asked for, 1080 by 1920 by
default. The video preview shows the short smaller, so the decoder tells
libass the size of the short's frames, `original_size`, and libass draws
the short's captions made smaller instead of laying them out again for the
smaller picture. Laid out again, a line broke and a pill grew differently,
a pixel here and there. Before that the app made the preview's caption
file at the size in the settings while a render job ran with others, and
the two disagreed by far more.

## The layer

`/frames/captions` in `cmd/framefairy-app/captions.go` answers the video
preview with the layer for one frame of the episode at the size the crop is
shown, see `engine.Engine.PreviewCaptions` in `engine/captionlayer.go`. The
caption view a clip's captions came with names them by its `track`, and
the colours and the height being dragged on the way come along as drafts,
checked the way the edit that saves them checks them. The caption file is
written once, under a name made from what it is drawn from, in
`<episode>.framefairy/captions/preview/`, with its fonts beside it. Many
frames ask for the same file at the same time while a clip plays, so it
is made once however many ask, and the others wait for it, see
`trackCache.once`. It is written whole under a name of its own and then
renamed into place, so no ask reads half a file. A file that could not be
made is not remembered, so the next ask tries again. The folder keeps the
256 used last and removes the rest, those of earlier runs first.

The decoder draws the file twice, over black and over white, and takes
colour and coverage from the two, `framewire.Layer`: red, green and blue
with the alpha multiplied in, in full range, and only the box that has any
ink. Only the rows and columns where the two drawings differ are worked
out pixel by pixel, `inked` in `cmd/framefairy-frames/ink.go`: elsewhere
nothing is drawn, and a caption is a strip of the frame. Full range, because the ass filter draws a file of standard video in
video range on a picture of red, green and blue too, see `videoRange` in
`cmd/framefairy-frames/captions.go`, and the frames of the video preview
are read the same way.

## Laid over the frame

The screen lays the layer over the frame the way the render burns it in,
colour plus frame times one less alpha, before the frame is given its
look, the QuickTime lift or HDR, see [VIDEO-PREVIEW.md](VIDEO-PREVIEW.md).
So the captions take the look with the picture, as they do in the short
on the player. On WebGPU the shader in `frontend/src/lib/frames/screen.ts`
samples the layer at its own pixels. Without it the 2D canvas does the
same in `laidPart`, `frontend/src/lib/frames/captions.ts`. Both lay it at
the canvas's own pixels, so the captions are as sharp as the screen.

A frame on its way to the screen asks for its layer ahead, `coming` in
`CaptionLayers`, so playing keeps up. A frame shown before its layer has
come keeps the last one up and is drawn again when its own arrives, so the
captions never blink off. A layer that does not come is not kept: the
frame keeps the last one up and asks once more a moment later. A change
of clip drops the last layer, so the old clip's captions never stand over
the new one, and what a frame waits for is asked for with the ask it was
drawn with, never the next one's.

## The caption box

The caption box over the picture stays: it is what the hand takes hold of
to click a word, correct it and drag the captions up or down. It draws
nothing while the captions are only shown. While a word is typed it shows
the words itself, since the letters being typed are not in any caption
file yet, and the layer steps aside until the word is saved.

Its words stand where libass put them. `/frames/captions/words` in
`cmd/framefairy-app/captions.go` answers with where each word of a view
stands in the short, `engine.WordFrame`, the box its pill takes at rest:
the word's ink across and its line's reach down, with the pill's room
round both, measured by libass the way the highlight measures them,
see `layWords` and `WordFrames` in `engine/wordframes.go`. The caption
box places each word on that box, from the line the captions stand on,
so the frame round a word under the hand is the edge of its pill and the
word stands in its middle, wherever the captions are dragged. The
browser laid the words out in its own font before, and Tim saw the frame
stand off its word on #201. While a word is typed the browser lays the
words out again, since it draws them itself then.

Each word frame also carries the radius of the pill's corners, `r`, from
`pillRadius` in `engine/wordframes.go`, the one rule the highlight draws
the pill with, and the caption box rounds the frame round a word by it.
The pill's corners are quarters of a circle, the way a browser rounds a
corner, see `roundedRect` in `engine/ass.go`. Its curves had both handles
on the corner before, which drew corners much sharper than the radius,
and Tim saw the frame and the pill with different corners on #201.

The caption box shows the caption and the lit word of the layer on
screen, not of the playhead. The playhead is the sound, and the frame on
the canvas trails it while a clip plays, and libass reads its caption
file to the hundredth of a second at the moment `CaptionMoment` names on
the short's frames. So `/frames/captions` says with every layer which
caption and which word of it libass drew, in the header
`X-Caption-Showing`, from `ShowingAt` in `engine/wordframes.go`, which
reads the same times the caption file is written with, `wordBegins`. The
video preview hands the caption box what the layer it paints says. Tim saw
the frame round a word go on to the next caption a few frames before the
pill did on #201, when the caption box worked the word out from the
playhead.

The keyboard goes by the frames too. Each word frame also says when the
picture lights the word, `from` and `to` on the episode's clock, from
`WordsLit` in `engine/wordframes.go`: it reads every frame of the clip at
the moment `CaptionMoment` names, with `ShowingAt`. Shift and an arrow walk
to where the picture lights the next word, see `lit` in
`frontend/src/screens/Episode.svelte`, and the word walked to is the
keyboard's while the picture lights it. Its frame shows on a frame only
when the layer on screen lights that word too, so a layer still on its way
holds it back with the pill. A caption's times are on the clip's clock and
a piece of the short starts on the frame nearest its start, so a word can
light up to half a frame before or after its own time. Walking by the
words' own times landed now and then on a frame that still lit the word
before, with the keyboard's frame already round the next, which Tim saw on
#201.

## The tests

- `TestTheVideoPreviewDrawsTheShortsCaptions` in
  `engine/captionlayer_test.go`: every frame of a short of two pieces,
  made by the render's own graph, is its frame without captions with the
  decoder's layer for that frame laid over it, within a tenth of a step on
  average. The frame before is told apart, so a layer a frame off in the
  bounce does not pass.
- `TestTheVideoPreviewDrawsTheCaptionsSmallerAsTheShortWouldShowThem` in
  the same file: the layer drawn at a third of the short is the short's
  layer made a third as big, closer than a frame of the bounce apart.
- `TestOneCaptionFileIsMadeOnceForManyAtOnce` and
  `TestACaptionTrackIsNamedByWhatItIs` in the same file, and
  `TestTheCaptionsRouteAnswersManyAtOnce` in
  `cmd/framefairy-app/captions_test.go`: many asks at once, at two sizes,
  with drafts and with asks called off, each get the layer they get alone,
  under the race detector.
- `TestTheLayerOfTheBoxIsTheLayerOfTheFrame` in
  `cmd/framefairy-frames/ink_test.go`: the layer worked out in its box is
  the layer of the whole frame, pixel for pixel.
- `TestTheBounceKeepsTheSpaceBetweenWords` in `engine/bounce_test.go`
  reads the pill of every frame back from a rendered short: it leaves at
  least a tenth of the caption size round its word on every frame, and
  `WordFrames` puts each word where its pill rests, within 2 pixels.
  `TestThePillNeverReachesAnotherLine` in the same file reads a caption
  of two lines back from a short: the pill keeps 0.06 of the size from
  the letters of the other line on every frame, and its bounce grows it
  by at least 0.25 of the size, see ENGINE.md.
  `TestTheWordFramesRouteSaysWhereLibassPutTheWords` in
  `cmd/framefairy-app/captions_test.go` asks the route for them.
- The walk "the frame round a word in the caption box is the pill libass
  draws" puts the playhead in five words and reads the pill off the
  canvas: the frame round the lit word stands on it within 1.5 pixels.
  The browser's own layout stood 5.5 pixels off.
- The walk "the frame round a word stays on the pill while the clip
  plays" plays three seconds over the end of a caption and reads the pill
  off the canvas on every frame the browser paints: where a pill is drawn
  a word is lit and its frame stands on the pill within 3 pixels, and
  where none is drawn no word is lit. The caption box that followed the
  playhead put the frame on the next caption's first word, 64 pixels off
  the pill.
- The walk "the keyboard's frame goes from word to word with the pill"
  makes every caption layer come 120 ms late and presses Shift and the
  right arrow eight times. On every frame painted after each press, the
  keyboard's frame stands on the pill libass drew, within 3 pixels, and it
  is there once the layer has come. Before, it stood on "war" 38 pixels
  from the pill, which was still on "Ich". `TestWordsLitIsWhatEachFrameShows`
  holds `WordsLit` to the layer of every frame of a clip whose pieces start
  off the grid of the frames.
- `TestShowingAtIsWhatTheCaptionFileShows` in `engine/wordframes_test.go`
  holds `ShowingAt` to the hundredths the caption file is written in, and
  `TestPillCornersAreQuartersOfACircle` holds each corner of a pill to a
  circle of its radius, within half a pixel. The corners before ran 49.5
  pixels from their centre for a radius of 40.
- The walk "the video preview shows the short's captions, frame for
  frame" in `frontend/preview/walks/sequences.mjs`, against the real Go
  side: it renders a short, makes the video preview as large as the short,
  steps to the start and the middle of a dozen words, and holds what the
  canvas shows to that frame of the short. The right frame differs by
  about 3 in brightness over the rows of the captions, the frame before or
  after with another word lit or another stage of the bounce by 4 to 12,
  and a video preview whose captions were drawn by the browser by far
  more.
- The pixel check of the screen, "screen light" in the same file, holds
  the GPU's laying over to the 2D canvas's for a highlight over standard
  video, a box at half and a white caption over PQ.
