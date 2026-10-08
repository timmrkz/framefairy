# The video preview on one engine

A spec, not yet built. It decides how the video preview gets its picture
and its sound from here on, and what is removed on the way. Decided with
Tim on 8 October 2026, after the history below. Plan row 2.156.

## In one paragraph

ffmpeg decodes everything the video preview shows and plays, for every
file on every system, the way it already decodes everything the short is
made of. The page keeps only what nobody else can do for it: the clock,
the cuts and drawing the frames it is handed. WebKit's decoders, the
Mac's decoder in our own cgo, our reader of MP4 files and every rule about
colour in the page are removed. Nothing is prepared before the first play.

## How we got here

- **Until 5 October** the video preview was a `<video>` element. WebKit
  read the file, decoded it, kept the sound in step and drew the colours.
  It could not play a clip with cuts: a cut was a seek in the middle of
  playing, so the last frame before it stood for up to 200 ms and the
  sound stopped. About ten pull requests patched the element's clock
  around that (#5, #6, #49, #97, #98, #100, #102, #105).
- **#110** replaced it with our own frame queue: two decoders, the second
  fed from the key frame before the next piece while the first plays, the
  frames drawn on the sound card's clock. Cuts became seamless and exact.
  That part was right, and it stays.
- **What went wrong** is that the queue also took over the media work.
  The page read the file itself, `mp4.ts`, and decoded with WebKit, and
  where WebKit failed, with the Mac's own decoder through cgo, and where
  that failed, with ffmpeg. Every media bug since was two engines
  disagreeing:
  - our file reader and ffmpeg counting time from different places, 2.139
  - WebKit saying it would decode HEVC with 10-bit colour, then failing, 2.141
  - the Mac's sound decoder and ffmpeg disagreeing where the sound starts, 2.153
  - WebKit and ffmpeg disagreeing on the colour range, the pale picture
- **The render is always ffmpeg.** Whenever the video preview used
  something else, the two could differ, and each difference was a bug to
  find.

### What was weighed and why not

| Way | Why not |
| --- | --- |
| One `<video>` | A cut is a seek, and a seek decodes from the key frame before it, so the picture holds and the sound stops at every cut |
| Two `<video>` elements swapped at each cut | Each keeps its own picture and sound together, but the swap is made by script on the page's next frame, and no element can be told to start at an exact moment of the sound card. Every cut lands somewhere within a frame or two |
| Media Source Extensions, the way YouTube plays | A piece can only start on a key frame, and an export has one every second or more. Exact cuts would need a copy of the episode made of key frames only |
| A playback copy made when a video is added | Minutes of encoding per hour of episode before the first play, and gigabytes on the disk. Adding a video has to be enough |
| WebCodecs with our own reader, as now | Two engines that disagree, see above, and a different path on each system |
| ffmpeg's libraries inside the app, go-astiav | A crash in C takes the whole app down, where a crash of the ffmpeg program fails only its own job |

## The rules

1. **Adding a video is enough.** Nothing is copied, converted or prepared
   before the first play, and the first frame shows as soon as the file
   can be read.
2. **One engine.** ffmpeg, the program the app ships, decodes every frame
   the video preview shows and every sample it plays, with the graphics
   chip where the system has one, VideoToolbox on the Mac. There is no
   second decoder and no fallback by system or by file. A file whose
   picture ffmpeg cannot decode cannot be rendered either, so it is
   refused once, when it is added, with the reason.
3. **The page never reads the episode file.** Every time the page knows,
   where the picture starts, how long a frame is, comes from the engine's
   probe, the same numbers the render cuts by.
4. **The video preview shows what the short will be.** The frames of a
   clip are the frames the render takes, chosen by the same code, the
   sound is the sound the render takes, and the colours are what ffmpeg
   makes of the file's own tags. The reference is the short as YouTube and
   Instagram show it, not QuickTime, which lifts the shadows on the Mac.
5. **A running play is never changed in place.** This is the rule from
   #155. A play runs one program, the clip's pieces or the episode
   straight on, from the moment it starts. A gesture that changes the
   program stops the play and starts it again from where the hand put it,
   or it is off while it would matter, the way I and O are off inside a
   clip. #152 did the same in 366 lines by splicing the new
   program into the running play, and was closed for #155, which made the
   case impossible in about 40.
   The one exception is loop, 2.150, which only changes what follows the
   clip's end. No other splice is built.
6. **A rule before machinery.** When a case would need new machinery in
   the frame queue, the first question is whether a rule of the product
   makes the case not happen, as in rule 5. Machinery is built only where
   no rule does.

## What the parts do

### The Go side

- **Picture streams**, `engine.PreviewFrames` and `/frames/open`, as now:
  from a moment on, scaled on the graphics chip to the size of the
  canvas, each frame with the moment it belongs to. ffmpeg finds the key
  frame before and decodes from it, which is its own work, not the page's.
- **Sound streams**, `engine.PreviewSound` and `/frames/sound`, as now.
- **The frames of a clip are the render's frames.** A stream for a piece
  is built by the same function that cuts the piece in the render,
  `cutOf` and `byTimes` in `engine/render.go`, so a file with uneven
  frames shows the frames the short will have, on the short's even rate.
  That ends 2.155 by construction.
- **Colour** is converted by ffmpeg from the file's tags, range, matrix
  and transfer, with ffmpeg's defaults where a tag is missing, and the
  stream carries pixels ready to draw. Nothing in the page decides colour.

### The page

- **The program**, `plan.ts`: which pieces play in which order, and where
  each starts and ends, counted on the frames the engine names.
- **The clock**, the sound card's, as now. The playhead is the sound
  heard.
- **Two picture streams and a sound stream**, as the two decoders are
  now: the second stream is opened at the next piece while the first
  plays, so the frame after a cut is already waiting.
- **Drawing**: each frame onto the one canvas when the clock reaches it.

## What is removed

| What | Where | Lines today |
| --- | --- | ---: |
| Our reader of MP4 and MOV | `frontend/src/lib/frames/mp4.ts` and its tests | 851 and 457 |
| The Mac's decoder through cgo | `engine/pictures*.go`, `frontend/src/lib/frames/native.ts`, `/frames/native` and `/frames/decode` | about 600, and 148 of tests |
| WebKit's decoders | `WebPictures`, `PlainSound` and the reader of byte ranges in `queue.ts` | about 200 |
| The choice between decoders | `fromGoSide`, `pictureFailed`, `nativeFailed`, `replaceDecoders` in `queue.ts` | about 100 |
| Colour in the page and the full range forced for WebKit | `queue.ts`, `app.ts`, `engine/preview.go` | a few dozen |

About 1,800 lines of code and 600 of tests, of about 6,400 in all, and
three of the four ways a picture can reach the canvas. What stays is the program, the clock, the
streams and the drawing.

## The steps

Each step is its own pull request, tested by Tim on his Mac before the
next one starts.

1. **Measure first, remove nothing.** Every file plays through the ffmpeg
   streams that already exist, on every system. A change of a few lines
   in `queue.ts`. Tim compares it with main, by updating between the two
   from the Updates page, on `start.mp4`, an export in H.264, a file from
   a phone and a 4K file:
   - playing straight on and through a clip's cuts is smooth
   - a click on the clip timeline while paused shows its frame without a
     wait he notices
   - the arrow keys step and a drag on the clip timeline follows the hand
   - Activity Monitor shows Frame Fairy and ffmpeg together at a share of
     the processor he can live with while playing

   The walks measure the same on Linux in CI: frames drawn late while
   playing, and the time from a click to its frame.
2. **One engine.** Remove WebKit's decoders, the Mac's decoder in cgo and
   the choice between them. Refuse at Add a file whose picture ffmpeg
   cannot decode.
3. **Colour from ffmpeg.** The streams carry pixels converted from the
   file's tags, and the page draws them as they come. Proved against
   ffmpeg's own conversion of the same frame, value by value, on files
   tagged BT.601, BT.709 and BT.2020, in video and full range, 8 and 10
   bit, and on files with no tags.
4. **The page stops reading the file.** The program counts on the frames
   the engine names, the picture's start and the short's rate, and a
   piece's stream is cut by the render's own code. `mp4.ts` is removed.
   Ends 2.155.

## What it costs

- **More work for the machine.** Every frame is copied from ffmpeg to the
  Go side and on to the page, where WebKit's decoder handed it over
  inside the webview. Step 1 measures how much.
- **A jump starts a new ffmpeg read.** A click away from what is decoded
  starts ffmpeg, which reads the file's index and decodes from the key
  frame before. The prototype of 2.141 measured 145 to 337 ms to the
  first frame in the cloud, with no graphics chip. The Mac's decoder in
  cgo was faster there, because it stays open. The streams already keep
  the frames shown last and wait for a stream that is close rather than
  start another, and that is where any further speed comes from. If step
  1 shows jumps too slow, the answer is in how the Go side keeps its
  streams, never a second engine.
- **One difference from other apps on the Mac.** QuickTime and Safari
  lift the shadows of BT.709, so the video preview looks a little darker
  there than in QuickTime. It looks like the short.

## What this replaces

The survey of media libraries in #151 proposed a colour shader of our own
and mediabunny in place of `mp4.ts`. With one engine neither is needed,
and #151 was closed for this spec. The ffmpeg program stays, as it
proposed.
