# The video preview on one engine

A spec, not yet built. It decides how the video preview gets its picture
and its sound from here on, and what is removed on the way. Decided with
Tim on 8 October 2026, after the history below. Plan row 2.156.

Three parts of the app are named here. **The interface** is the
TypeScript and Svelte in `frontend/`, which runs in the webview and draws
the video preview. **The Go side** is the app's own Go program in
`cmd/framefairy-app/`, which serves the interface. **The engine** is
`engine/`, which the Go side and the command line share.

## In one paragraph

ffmpeg decodes everything the video preview shows and plays, for every
file on every system, the way it already decodes everything the short is
made of. A decoder for the episode is always running and waiting, so a
jump is a request to it, not a program starting. The interface keeps only
what nobody else can do for it: the clock, the cuts and drawing the frames
it is handed. WebKit's decoders, the Mac's decoder in our own cgo, our
reader of MP4 files and every rule about colour in the interface are
removed. Nothing is prepared before the first play.

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
  The interface read the file itself, `mp4.ts`, and decoded with WebKit,
  and where WebKit failed, with the Mac's own decoder through cgo, and
  where that failed, with ffmpeg. Every media bug since was two engines
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
| Two `<video>` elements swapped at each cut | Each keeps its own picture and sound together, but the swap is made by script on the interface's next frame, and no element can be told to start at an exact moment of the sound card. Every cut lands somewhere within a frame or two |
| Media Source Extensions, the way YouTube plays | A piece can only start on a key frame, and an export has one every second or more. Exact cuts would need a copy of the episode made of key frames only |
| A playback copy made when a video is added | Minutes of encoding per hour of episode before the first play, and gigabytes on the disk. Adding a video has to be enough |
| WebCodecs with our own reader, as now | Two engines that disagree, see above, and a different path on each system |
| ffmpeg's libraries inside the app, go-astiav | A crash in C takes the whole app down, where a crash of a program of its own fails only that program |

## The rules

1. **Adding a video is enough.** Nothing is copied, converted or prepared
   before the first play, and the first frame shows as soon as the file
   can be read.
2. **One engine.** ffmpeg decodes every frame the video preview shows and
   every sample it plays, with the graphics chip where the system has
   one, VideoToolbox on the Mac. There is no second decoder and no
   fallback by system or by file. A file whose picture ffmpeg cannot
   decode cannot be rendered either, so it is refused once, when it is
   added, with the reason.
3. **A decoder is always waiting.** While an episode is open, its decoder
   runs with the file open, its index read and the decoder ready. A jump,
   a click, a drag or the space bar is a request to it, answered at once,
   never a program that has to start first. If it stops, the Go side
   starts it again, and the video preview keeps the frame it shows until
   then. See Speed.
4. **The interface never reads the episode file.** Every time the
   interface knows, where the picture starts, how long a frame is, comes
   from the engine's probe, the same numbers the render cuts by.
5. **The video preview shows what the short will be.** The frames of a
   clip are the frames the render takes, chosen by the same code, the
   sound is the sound the render takes, and the colours are what ffmpeg
   makes of the file's own tags. How those colours are shown on the
   screen is decided once, by Tim, after the test in Colour. An HDR
   episode makes an HDR short, and the video preview shows its HDR as far
   as the webview lets a canvas show it, see HDR.
6. **A running play is never changed in place.** This is the rule from
   #155. A play runs one program, the clip's pieces or the episode
   straight on, from the moment it starts. A gesture that changes the
   program stops the play and starts it again from where the hand put it,
   or it is off while it would matter, the way I and O are off inside a
   clip. #152 did the same in 366 lines by splicing the new program into
   the running play, and was closed for #155, which made the case
   impossible in about 40. The one exception is loop, 2.150, which only
   changes what follows the clip's end. No other splice is built.
7. **A rule before machinery.** When a case would need new machinery in
   the frame queue, the first question is whether a rule of the product
   makes the case not happen, as in rule 6. Machinery is built only where
   no rule does.

## What the parts do

### The episode's decoder

One program per open episode, started by the Go side when the episode
opens and stopped when it closes.

- It is built on ffmpeg's own libraries, the code the ffmpeg program
  runs, so it decodes exactly as the render does. It is a program of its
  own, not part of the app, so a crash ends only it. Written in Go with
  go-astiav, linked to ffmpeg's libraries as shared libraries shipped
  beside it, so LGPL is met the way it is for ffmpeg itself.
- It keeps the file open, its index read and two picture decoders and a
  sound decoder ready, with the graphics chip where there is one.
- It takes requests from the Go side: frames from a moment on at a size,
  sound from a moment on, stop. A request for a new moment drops what the
  last one was doing.
- It keeps the frames around the playhead it has already decoded, so a
  step back, a step on and a drag over them cost nothing, and while
  paused it decodes the frames on either side of the playhead before they
  are asked for.
- The routes the interface reads from, `/frames/open`, `/frames/sound`,
  `/frames/read` and `/frames/close`, stay as they are. The interface
  cannot tell whether a frame came from it or from the ffmpeg program.

### The Go side

- Starts and stops the episode's decoder, and passes its frames and sound
  on through the routes above.
- **The frames of a clip are the render's frames.** A request for a piece
  is made by the same function that cuts the piece in the render, `cutOf`
  and `byTimes` in `engine/render.go`, so a file with uneven frames shows
  the frames the short will have, on the short's even rate. That ends
  2.155 by construction.
- **Colour** is converted by ffmpeg from the file's tags, range, matrix
  and transfer, with ffmpeg's defaults where a tag is missing, and the
  frames carry pixels ready to draw. Nothing in the interface decides
  colour.

### The interface

- **The program**, `plan.ts`: which pieces play in which order, and where
  each starts and ends, counted on the frames the engine names.
- **The clock**, the sound card's, as now. The playhead is the sound
  heard.
- **Two picture streams and a sound stream**, as the two decoders are
  now: the second stream is asked for the next piece while the first
  plays, so the frame after a cut is already waiting.
- **Drawing**: each frame onto the one canvas when the clock reaches it.

## Speed

A jump today costs three things: starting the ffmpeg program, opening
the file and reading its index, and decoding from the key frame before
the moment. The prototype of 2.141 took 145 to 337 ms from a click to the
first frame, in the cloud with no graphics chip. The ffmpeg program is
told where to start when it starts and cannot be sent anywhere else
afterwards, so a waiting copy of it would save only the first of the
three. The episode's decoder saves the first two, and with the graphics
chip and the frames around the playhead kept, most of the third.

What counts as fast enough, measured on Tim's Mac with his own files, by
his eye and by the walks in CI:

| What | Fast enough |
| --- | --- |
| A click on the clip timeline while paused | its frame on screen within 100 ms |
| A drag along the clip timeline | a new frame at least every 50 ms while the hand moves, and the frame under the hand within 100 ms of it stopping |
| The space bar | picture and sound start within 100 ms |
| Playing at 1080p, up to 60 frames a second | no frame late, and a cut takes one frame's time, as now |
| The machine while playing at 1080p | Frame Fairy and its decoder together under 100% in Activity Monitor, one core's worth |

The numbers are a proposal for Tim to change. Main is measured the same
way first, so each step is judged against today as well as against the
marks.

## Colour

**What Tim saw** was a pale, foggy picture: black shown as a grey of 17
where QuickTime shows 1. That was the range, video range taken for full
range, and #141 fixed it. Black is 0 now.

**What is left** is smaller and only in the dark tones. Black stays
black and white stays white. In between, QuickTime on the Mac lifts the
shadows of standard video: a dark grey the file stores as 26 is shown by
the standard curve as 26 and by QuickTime as about 35. So against
QuickTime the video preview shows a little more contrast in the shadows.
It is not paler, and it is not the other extreme either.

**Which look is right is decided by a test, not here.** In the colour
step, the same frame of `start.mp4`, of an export in H.264 and of a phone
file is shown side by side:

- in the video preview, with the standard curve
- in QuickTime
- the rendered short in Safari and in Chrome on the Mac
- the rendered short on Tim's iPhone, in Photos and as uploaded to
  Instagram and YouTube

Tim picks the look the video preview should have. If it is QuickTime's,
the lift is one fixed curve in ffmpeg's conversion for the video
preview, the same on every system. The short itself is never changed by
this choice: its numbers are the file's, and each phone shows them its
own way.

## HDR

**What it is.** Standard video, SDR, is made for a screen of about 100
nits. HDR stores much brighter highlights and more colours, BT.2020, with
a different curve, HLG or PQ. An iPhone films in HDR by default, as HEVC
with 10-bit colour, HLG and Dolby Vision on top. A file says it is HDR in
its tags: a transfer of `arib-std-b67` is HLG, `smpte2084` is PQ. The
engine's probe already reads these tags.

**What happens today.** Nothing is decided. The render copies the file's
colour tags onto a short encoded in H.264 with 8-bit colour, so a short
made from iPhone footage comes out as HDR-tagged video with too few bits
for HDR, and players show it in different ways. The video preview shows
whatever the decoder in use makes of it.

### The short: HDR in, HDR out

Decided by Tim. A short keeps the quality of its episode, and an HDR
episode makes an HDR short. That is the rule of no colour changes.

- The short is HEVC with 10-bit colour and the episode's own transfer,
  primaries and matrix, HLG stays HLG and PQ stays PQ. Instagram and
  YouTube take HDR from an iPhone in this form.
- The captions are drawn at the reference white for graphics in HDR,
  BT.2408, 203 nits, so the caption colour looks on an HDR screen the way
  it was chosen, and not glaring.
- What the encoder for 10-bit HEVC is on Windows and Linux, where our
  ffmpeg has no x265 because x265 is GPL, is a question for
  [PACKAGING.md](PACKAGING.md). On the Mac it is VideoToolbox.
- An SDR episode makes an SDR short, as now.

### The video preview: HDR as far as the screen is given to us

Everything up to the screen can carry HDR, and nothing in our own code
stops it:

- **ffmpeg** decodes the whole picture, every bit of it.
- **The frames** are only bytes on their way from the decoder to the
  interface. They can be 10-bit, or half floats, as easily as 8-bit. It
  costs more bytes, see What it costs, not a limit.
- **The screen**, an XDR display or an HDR monitor, can show brighter
  than white, and the Mac, Windows and Linux all have ways to ask for it.

The one link we do not own is the last: a canvas in the webview. The
interface can only put pixels on screen through what the webview offers,
and the webview decides whether a canvas may be brighter than white.
WebKit makes that surface for its own `<video>` and for HDR images, and
not yet for a canvas, as far as can be found:

- **A 2D canvas** ends at white everywhere. HDR for it is a proposal.
- **A WebGPU canvas** has an extended mode for exactly this,
  `toneMapping: "extended"` with half floats, where 1.0 is white and
  brighter values are brighter light. Chrome shipped it, so the webview on
  Windows has it. On the Mac, Safari took the setting and still showed
  normal brightness in August 2025, and WebKit has since marked the work
  as done. Whether the webview on Tim's macOS shows it is not known.
  Linux's webview has nothing of the kind yet.

So the video preview draws on a WebGPU canvas, for every file, and asks
for the extended mode:

- ffmpeg converts each frame to half floats in which 1.0 is the reference
  white of BT.2408 and the highlights go above it, and the interface
  hands them to the canvas as they come. The colour is still ffmpeg's.
  The conversion of HLG and PQ needs zimg in our build of ffmpeg, which
  has a permissive licence.
- Where the webview gives the extended mode and the screen is HDR, the
  video preview shows the HDR short as it is.
- Where it does not, the system squeezes everything above white, and the
  video preview shows the HDR short as an SDR screen would show it.
  Nothing in our code changes for that. It improves by itself the day a
  webview gives more.
- The first thing the colour step does is a test on Tim's Mac: one frame
  of an iPhone file on the extended canvas, its sky or a lamp beside the
  white of the interface. If it shines brighter, WebKit gives us HDR
  today.

A layer of our own beside the webview, drawn with Metal on the Mac, would
show HDR whatever WebKit does. It is the one way left if the test fails
and HDR in the video preview matters enough, and it would be a path for
the Mac alone, against rule 2. It is not planned.

## What is removed

| What | Where | Lines today |
| --- | --- | ---: |
| Our reader of MP4 and MOV | `frontend/src/lib/frames/mp4.ts` and its tests | 851 and 457 |
| The Mac's decoder through cgo | `engine/pictures*.go`, `frontend/src/lib/frames/native.ts`, `/frames/native` and `/frames/decode` | about 600, and 148 of tests |
| WebKit's decoders | `WebPictures`, `PlainSound` and the reader of byte ranges in `queue.ts` | about 200 |
| The choice between decoders | `fromGoSide`, `pictureFailed`, `nativeFailed`, `replaceDecoders` in `queue.ts` | about 100 |
| Colour in the interface and the full range forced for WebKit | `queue.ts`, `app.ts`, `engine/preview.go` | a few dozen |

About 1,800 lines of code and 600 of tests, of about 6,400 in all, and
three of the four ways a picture can reach the canvas. The episode's
decoder is new code in their place, a few hundred lines that do one
thing.

## The steps

Each step is its own pull request, tested by Tim on his Mac before the
next one starts.

1. **Measure first, remove nothing.** Every file plays through the ffmpeg
   program's streams that already exist, on every system. A change of a
   few lines in `queue.ts`. Tim compares it with main, by updating
   between the two from the Updates page, on `start.mp4`, an export in
   H.264, a file from a phone and a 4K file, against the marks in Speed.
   The walks measure where a jump's time goes: starting the program,
   opening the file, decoding from the key frame.
2. **The episode's decoder.** The waiting program of rule 3, for picture
   and sound, behind the same routes. Measured against step 1 and the
   marks.
3. **One engine.** Remove WebKit's decoders, the Mac's decoder in cgo and
   the choice between them. Refuse at Add a file whose picture ffmpeg
   cannot decode.
4. **Colour and HDR.** The frames carry pixels converted by ffmpeg from
   the file's tags, and the interface draws them as they come. Proved
   against ffmpeg's own conversion of the same frame, value by value, on
   files tagged BT.601, BT.709 and BT.2020, in video and full range, 8 and
   10 bit, and on files with no tags. Then the side-by-side test, and Tim
   picks the look. HDR in both: the test of the extended canvas on Tim's
   Mac first, then the video preview on the WebGPU canvas and the HDR
   short with its captions at the reference white.
5. **The interface stops reading the file.** The program counts on the
   frames the engine names, the picture's start and the short's rate, and
   a piece is asked for by the render's own code. `mp4.ts` is removed.
   Ends 2.155.

## What it costs

- **More work for the machine.** Every frame is copied from the decoder
  to the Go side and on to the interface, where WebKit's decoder handed
  it over inside the webview. At the size of the video preview that is
  about 40 MB a second while playing. Finished pixels in half floats, for
  HDR in step 4, are about 220 MB a second at 1280 by 720 and 30 frames a
  second. Copying memory runs at several GB a second, so this is not a
  limit on an M2 Max, and a 4K episode costs no more, because frames are
  made at the size of the video preview. Activity Monitor and the marks
  say whether it matters on smaller machines.
- **A program of our own on ffmpeg's libraries.** The episode's decoder
  is code we keep, and make has to build ffmpeg's libraries as shared
  libraries as well as the program. It is still ffmpeg's decoding, only
  kept running.
- **Possibly a little more contrast in the shadows than QuickTime**, if
  Tim picks the standard look in the colour test.

## What this replaces

- **The playback copy of the episode**, plan row 1.5b, made for files the
  video preview could not play. With one engine the video preview plays
  whatever the render can read, and no copy is made.
- **The survey of media libraries in #151**, which proposed a colour
  shader of our own and mediabunny in place of `mp4.ts`. With one engine
  neither is needed, and #151 was closed for this spec.
