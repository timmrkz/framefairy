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

## The bounce after a word step

The short burns its pop into its frames, and a frame paused on shows one
moment of it. A step from frame to frame, an arrow alone, a click or a
drag, shows exactly that: the frame as the short shows it, with nothing
played over it. A step from word to word, Shift and an arrow, bounces the
word it lands on, which is how the video preview shows the step and is
nothing the short shows. Tim saw the pop the short burns in played again
over every frame he stepped to, the arrow keys included, two animations
mixed into one, and wanted them apart. The word that bounces is the one
on the frame the step lands on, counted from the frame on screen whether
its layer has come or not, so two quick steps bounce the second word and
not the one passed over.

The two are separate paths that share only the shape of the curve. The
pop in the short is libass's, on the short's frames, `popScales` in
`engine/highlight.go`, and the video preview shows it as it shows any
frame. The bounce after a word step is the app's own animation of what
libass drew. libass draws the word with its pill apart from the rest of
the caption, both once and at rest: `CaptionDrawing.Lifted` marks every
caption `LiftWord` or `LiftRest`, and `/frames/captions` gives either
with `part=word` or `part=rest`, the word with the curve in
`X-Caption-Bounce`, "<ms> <rise> <peak>", from `BounceCurve`. The screen
lays the rest over the frame and the word over that, scaled round the
middle of its box, on every frame of the screen, 60 or 120 a second, by
the clock of that frame. That is how the Mac animates a layer, drawn once
and moved by the GPU, see `liftBox` and `laidPart` in
`lib/frames/captions.ts` and the shader in `screen.ts`. Nothing is
drawn in the browser: the word is libass's, only scaled. A frame drawn
again over the same frame and the same captions writes none of its
pixels to the GPU again, so a bounce costs one small uniform a frame.

Before, the bounce was the short's own frames of the pop, drawn on the Go
side one by one and shown one each frame of the short, 25 a second on
Tim's episode and stepped by a timer, and Tim found it laggy.

Every word bounces the same, from rest through 115 % and back in 220
milliseconds, a word the short lights for one frame too, which the short
shows at its peak: Tim wanted the bounce the same for every word, without
exception, even where that is not the frame the short shows. At rest the
word stays lifted over the frame until the playhead moves or the clip
plays. The frame's own layer stays while the two parts are on their way,
or the layer before the step where the frame shows its word grown. At its
peak the word's pill lies over the letters of the words beside it, as it
does in the short. Nothing bounces while the clip plays, where the frames pop by
themselves, or when the Mac is set to reduce motion. `Lift` and
`CaptionLayers.sought` in `lib/frames/captions.ts`.

## Words too short for a frame of their own

A word with no frame of the short starting in its time is lit on the
frame showing when it begins, together with the words lit there, in one
pill round them that pops from its middle, a part of it on each line they
stand on. That frame is the only one they share: the word before is lit
alone on its frames before it. A word with no frame before the first
word of its caption that has one is lit on that word's first frame, with
it. `litRuns` in `engine/wordframes.go` says which words are lit
together and when, and the caption file, `ShowingAt`, `WordsLit` and
`PopScale` all read it. `X-Caption-Showing` says the first and the last
of them, "<caption> <word> <last>", and the caption box lights every
word from the one to the other, each frame growing in the pop round the
middle of the whole pill, as libass grows it, see `litMiddle` in
`Player.svelte`. The walk "a word too short for a frame of its own is
lit with the word on the frame it shares" types " Q Schulhofgeschichten"
into "war", steps frame by frame onto "Q", and sees it lit with the
words beside it on one pill libass drew. Several words typed into a word a frame or two long were
lit on no frame before, so they were never seen lit, #280. Tim asked
that how many words go into so little time stays the person's choice,
so their times are not stretched, the words share the frame.

## The frame round a word

The frame round a word in the caption box stands across the edge of its
pill, a pixel of the screen out and one in, `--hair` in `app.css`. libass
draws the edge where it falls, between two pixels as often as not, and
the row it half covers comes out half dark, while the browser puts a
frame on whole pixels. As an outline round the box, that row showed
between the frame and the pill or just inside it, and Tim saw the pill a
pixel short at the top or the bottom. An outline cannot be moved by half
a point, so the frame is a shadow.

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
`trackCache.once`. It is made in a context of its own, with a limit of
its own, so an ask called off leaves the others waiting their answer, and
each ask waits only as long as it is wanted. It is written whole under a
name of its own and then
renamed into place, so no ask reads half a file. A file that could not be
made is not remembered, so the next ask tries again. The folder keeps the
256 used last and removes the rest, those of earlier runs first, and what
an earlier run left half written. A file half written a moment ago is
another ask's, made beside it, and stays. The caption file is never what makes the work folder: a folder without its mark gets no caption file, and one gone by the moment the file is written gets none either, `ErrNoWorkFolder`. The folders the file and its face go in are made one at a time inside the work folder, `foldersIn`, and never with everything above them, so a deletion that crosses the write leaves the folder gone, which CI's run of that walk caught. The video preview went on asking for captions after Tim's folder was deleted in Finder, and wrote the folder back before the app let the video go, which main's walk for 2.247 found on #201.

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
`CaptionLayers`, so playing keeps up. A frame a gesture puts the playhead
on, a click, a step, a clip picked, a play starting, waits for its own
layer before it is put up, `ready` in `CaptionLayers` and `dress` in
`queue.ts`, so the picture and its captions come in one frame, issue
#383. The layer is asked for the moment the frame is, ahead of the frames
read ahead, so the two take the time the slower takes. A clip just picked
waits for its captions from the Go side first, `captionsDue`, unless
they were read before: the captions of every clip on the cards are asked
for once the workspace is on screen, one at a time, `readAhead` in
`Episode.svelte`, and the layers of their first frames drawn, `next` in
`CaptionLayers`, one at a time and only while nothing else is on its
way. Those layers are kept for as long as their cards are there, so a
card clicked has its captions at once. Nothing
waits under a dragging hand, and nothing waits longer than 600 ms,
`DRESS_WAIT`: past that the frame goes up without its captions and the
log says so. A frame shown before its layer has
come keeps the last one up and is drawn again when its own arrives, so the
captions never blink off. That holds over a new track of the same clip
too, which comes with every saved edit and every step of a caption edge
being dragged. A layer is laid by the size it was drawn at, `size` on the
layer, so while the app is resized the last one is stretched into its
place until the new size's own comes, and the captions do not jump toward
the crop's corner. A layer that does not come is not kept: the
frame keeps the last one up and asks once more a moment later. A change
of clip drops the last layer, `clip` on `CaptionAsk`, so the old clip's
captions never stand over the new one, and what a frame waits for is asked for with the ask it was
drawn with, never the next one's.

At most three layers are on their way at once. An ask no longer wanted
is called off, so its place goes to the one wanted now: every ask on its
way when a draft, the track or the clip changes, the frames that were
coming when the playhead is put elsewhere while playing, all but the
frame on screen, and the two parts of a lifted word once it is put back.
Before, they kept their places until they came, a colour dragged across
the wheel waited for three caption files nobody would see, and every one
of them was made on the Go side. A request called off is no line in the
app's log.

While a clip plays, a frame that asks ahead while three are on their
way waits its turn and is asked for as soon as one comes, after the frame
on screen if that is still waiting for its own. Before, it was dropped
and asked for only once it was drawn, so on a busy machine a playing clip
showed a frame with the captions of the frame before, issue #288.
Paused, nothing waits: the frames read ahead are the ones a step may
come to, the step asks for its own, and a bounce after a word step needs
the Go side to itself.

## The caption box

The caption box over the picture stays: it is what the hand takes hold of
to click a word, correct it and drag the captions up or down. It draws
nothing while the captions are only shown. While a word is typed, libass
draws the caption without that word, in its place, `typing` on
`/frames/captions` and `CaptionDrawing.Typing`, and the caption box draws
only the letters being typed, on that word's box and in the caption's
colour, since they are in no caption file yet. Nothing else of the
caption moves. Before, the layer stepped aside and the browser drew the
whole caption in its own font, larger than libass draws it, and Tim saw
the caption grow as a word was opened and shrink as it was left. Only
before libass has said where the words stand does the browser still draw
the whole caption while a word is typed.

The word typed sits on a pill in the highlight colour, whether it is the
word lit or not, and wears no frame. A word not lit was typed on the dark
box of the caption, and the frame round a lit word, in the app's lighter
purple, read as a second shade of the pill, so Tim saw three colours where
there is one. The caret says the word is open, and the word wears no
focus ring either: the ring every other control wears stood a pixel off
the pill, and the caption's box showed through between the two as a dark
line. With the highlight off the
word typed wears the frame and no pill, since there is no pill to match.
The walk "a word typed sits on the pill libass draws, lit or not" holds
the colour behind the letters typed to libass's pill and finds no ring
and no focus ring.

While the word typed is the word lit, libass leaves out its pill too, and
the caption box draws the pill behind the letters typed. It is libass's
pill as long as it holds the word: its height, its corners, and its width,
which is the margin left round the browser's letters, read once as the
word is opened. It grows round its middle by exactly what is typed, so a
word made longer, or made two, stays inside it. libass drew the pill at
the word's old size, and Tim saw a word typed longer run out of it. On a
frame of the pop the word opens at that frame's scale and keeps it while
it is typed, since the frame stays where it was, so opening a word moves
nothing. The rest of the line stays put while a word is typed, and the
grown pill lies over the words beside it, as the pop does. Enter saves
the word, and libass lays the line out again with it, which moves the
words beside it apart and can break the line in another place. The walk
"a word typed longer grows its pill round its middle".

The letters typed are drawn in the face as libass draws it. Each face is
declared to the browser with its own weight, `CaptionFont.Weight`, read
from its OS/2 table, and the caption box draws it at that weight and
never fakes a bold, `font-synthesis: none`. libass thickens a face only
when bold, 700, asks for more than 150 above its weight, by growing the
outline of every letter by an em over 64 and leaving the advances alone,
`ass_glyph_embolden` in libass. The caption box does the same with a
stroke round each letter in its own colour, as wide as the share of the
em `Thickening` in `engine/fonts.go` says, which the captions view hands
over as `thicken`. With no weight declared every face counted as 400, so
on the Mac Inter Black was thickened in the caption box and not in the
short, and Tim saw the word typed larger than the word in the frame
behind it. Then Tim saw a word typed in Archivo Black, a face of weight
400 that libass thickens for bold captions, lighter and narrower than
libass draws it, since the Mac faked the bold its own way. The walk "a
word typed in a face libass thickens is as heavy as libass draws it"
holds its letters to libass's in size and reads how they are drawn: the
face's own weight, no bold faked and the stroke libass's. The pixels of
letters are no measure of weight across machines: CI draws Inter Black,
which nothing thickens, a tenth heavier than a cloud session does. The pill and the
letters typed wear the look of the frame they lie over, `shownColour` in
`frontend/src/lib/frames/light.ts`: libass draws the captions into the
frame and the frame is then given its look, so a colour drawn by the
browser beside it needs the same look or it stands out. Tim saw the pill
of the word typed in another purple than the pill round it. The caret is
the colour of the letters, as in a text field on the Mac, and the
selection is the Mac's own, `Highlight`, kept just short of opaque so
WebKit does not thin it with white into the colour of the pill. Nothing
of the word is drawn outside its pill, `overflow: clip`, since a
selection is as tall as the face's line and stood over the pill, the
walk "the letters selected in a word typed stay in its pill". The walk
"a word typed longer grows its pill round its middle" holds the letters'
size and colour and the pill's colour to libass's.

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
stand off its word on #201. They are not asked for while the hand still
shapes the clip or a caption on the clip timeline, only for the captions
it leaves, see `wordFrames` in `Episode.svelte`. Asked on every step of
a drag, they held a connection to the Go side beside the video
preview's layers, and on a busy machine the drag's own answers waited
behind them: the walk "an edge taken again while the drag before it
saves goes with the hand" saw the edge stand still.

The letters typed stand on the baseline libass set the word on. Each
word frame carries it, `base`, the bottom of the ink of a capital H on
the word's line, measured with the rest, `baseLines` in
`engine/highlight.go`. The word being typed is trimmed to the top of a
capital and its baseline, `text-box: trim-both cap alphabetic`, and its
top padding puts the baseline on `base`. The browser put the line in the
middle of the pill's height before, where the face's own ascent and
descent decide where the letters stand. On the Mac that was a few pixels
lower than libass, and Tim saw a word drop as he opened it on #201. The
walk "a word opened in the caption box stays where libass draws it"
holds the rows of the letters typed to the rows libass drew them on. In
the cloud's Chromium the two agreed before too, so the walk holds the
place, and the Mac is where the drop was.

With the highlight on, the box round the caption is measured from the
top of its first line's reach to the bottom of its last, the rows the
pills of those lines stand round, and the ink across, `wordsLaid.extent`
in `engine/wordframes.go`. Round the ink alone it hugged a caption with no
tall or deep letter while the pill reached down for a tail the word did
not have, and Tim saw the pill of "Und" touch the bottom of its box with
room over it on #201. Now every caption of as many lines has a box of
one height, and it leaves as much room over its pills as under them,
`TestTheBoxStandsEvenlyRoundItsPills`.

The frame round the lit word grows with its pill. Every layer says how
large libass draws the pill of its word on that frame, `X-Caption-Scale`
from `CaptionDrawing.PopScale`, and the caption box scales the word's
frame by it round the pill's middle, so the frame stays on the pill's
edge through the pop, as it did when the browser drew both. Where no
pill is drawn, with the highlight off or when the words could not be
measured and the caption file is plain, every layer says 1, and the frame
stays still over a word that does not move. With the highlight off, the
box round the caption is measured with the word being typed in it, so it
keeps its size while a word is typed.

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
reads the same times the caption file is written with, `litRuns`. The
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
- `TestTheWordAndItsPillPopTogether` in `engine/bounce_test.go` reads
  the pop back from the frames the render's own graph draws, before
  they are encoded: on every frame the word and its
  pill are as large as `popScales` says, in width and in height, around
  one centre. `TestEveryWordOfAShortPopsOnItsFrames` does the same for
  every word of a 25 fps short, with words lit for 1 to 10 frames. `TestThePillLeavesRoomRoundItsWord` in the same file holds
  the pill to at least a tenth of the caption size round its word on its
  first frame and at rest, and `WordFrames` to where the pill rests,
  within 2 pixels. `TestThePillAtRestNeverReachesAnotherLine` reads a
  caption of two lines back from a short: the pill at rest keeps 0.04 of
  the size from the letters of the other line, with the lines where
  libass puts them, see ENGINE.md.
- `TestTheWordLiftedOutAndTheRestAreTheCaptionAtRest` in
  `engine/captionlayer_test.go`: the rest with the word laid over it is
  the caption libass draws whole, at rest. "the bounce after a word
  step" in `captions.test.ts`: a word step lifts its word and bounces it
  once on each frame of the screen, up to its peak and back to rest, a
  word of one frame too, and any other step shows the frame alone, see
  above. The walk "a step onto a word pops it over the paused frame"
  steps six words on and back with Shift and the arrows, each bouncing to
  115 % and back over at least five frames of the screen, then forty
  frames over four words with the right arrow, over which nothing is
  played. "screen light" holds the GPU's word, grown over a box, to
  `laidPart`'s.
  `TestTheWordFramesRouteSaysWhereLibassPutTheWords` in
  `cmd/framefairy-app/captions_test.go` asks the route for them.
- The walk "the frame round a word in the caption box is the pill libass
  draws" puts the playhead in five words and reads the pill off the
  canvas: the frame round the lit word stands on it within 1.5 pixels,
  once its bounce has settled, and inside it in its bounce, which grows
  more on one side where another line is near. The browser's own layout
  stood 5.5 pixels off.
- The walk "the frame round a word stays on the pill while the clip
  plays" plays three seconds over the end of a caption and reads the pill
  off the canvas on every frame the browser paints: where a pill is drawn
  a word is lit and its frame stands on the pill, within 3 pixels across
  and inside it down, and where none is drawn no word is lit. The caption
  box that followed the playhead put the frame on the next caption's
  first word, 64 pixels off the pill. The caption box follows the layer,
  so a layer that comes late takes the box and the pill behind the sound
  together, and the walk counts that too: at most one paint in twenty
  with the layer of a frame before, and never for two frames on end. The
  app counts the layers that take longer than a frame to draw and says
  them in one line once the frames stop, see Captions in
  [LOGGING.md](LOGGING.md).
- The walk "the keyboard's frame goes from word to word with the pill"
  makes every caption layer come 120 ms late and presses Shift and the
  right arrow eight times. On every frame painted after each press, the
  keyboard's frame stands on the pill libass drew, within 3 pixels, and it
  is there once the layer has come. Before, it stood on "war" 38 pixels
  from the pill, which was still on "Ich". `TestWordsLitIsWhatEachFrameShows`
  holds `WordsLit` to the layer of every frame of a clip whose pieces start
  off the grid of the frames. `TestWordsTooShortForAFrameAreLitTogether`
  holds words with no frame of their own to the frame they share, and
  `TestWordsLitIsWhatTheShortLights` reads the pill of a rendered short
  move onto "Ich so" on the one frame they share.
- `TestShowingAtIsWhatTheCaptionFileShows` in `engine/wordframes_test.go`
  holds `ShowingAt` to the hundredths the caption file is written in, and
  `TestPillCornersAreQuartersOfACircle` holds each corner of a pill to a
  circle of its radius, within half a pixel. The corners before ran 49.5
  pixels from their centre for a radius of 40.
- The walk "the video preview shows the short's captions, frame for
  frame" in `frontend/preview/walks/clips/`, against the real Go
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
