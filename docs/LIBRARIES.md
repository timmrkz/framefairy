# Media libraries: what we wrote and what exists

Tim asked for the lay of the land before more media code is written: which
parts of the media machinery are our own, which established libraries
could carry them instead, and in what order to move. This is that survey,
as of October 2026. It decides nothing by itself. Each move is its own
pull request, with its own proof.

The rule it follows: what a standard defines and an established library
implements, we take from the library. What is particular to making
shorts, we keep, small and tested.

## What we use today

- **ffmpeg and ffprobe, the programs**, in an LGPL build of our own, for
  probing, decoding, cutting, scaling, encoding and burning in captions.
  libass draws the captions inside ffmpeg.
- **WebCodecs and WebAudio** in the webview, for the video preview.
- **VideoToolbox** on the Mac, through about 250 lines of cgo, for
  pictures WebKit cannot decode.
- **sherpa-onnx** for speech, **speedata/hyphenation** for breaking
  caption words, **whatlanggo** for the language.

No Go module and no npm package of ours is a media library. Everything
else in the media path is our own code.

## What is our own

| Area | Where | Lines | What a library would normally do |
| --- | --- | ---: | --- |
| Reading MP4 and MOV in the page | `frontend/src/lib/frames/mp4.ts` | 851 | sample tables, edit lists, timestamps, codec strings, colour tags, AAC and Opus headers, plain sound |
| Playing pieces in step | `frames/plan.ts`, `frames/queue.ts` | 644, 1779 | the program of a clip, decoding runs from key frames, sound placed to the sample, the clock |
| Frames from the Go side | `frames/app.ts`, `native.ts`, `pull.ts`, `cmd/framefairy-app/frames.go` | 435, 198, 104, 456 | streams of decoded frames and sound with a framing of our own |
| VideoToolbox decoder | `engine/pictures_darwin.go` | 247 | decoding H.264 and HEVC on the Mac |
| Probing | `engine/ffmpeg.go` | 724 | ffprobe's JSON read, uneven frames found from packet times |
| Cutting and rendering | `engine/render.go`, `encode.go` | 518, 192 | ffmpeg filter graphs built to cut on whole frames and to the sample |
| Colour | spread over the above | | range, matrix and transfer of the picture, see below |
| Loudness for the clip timeline and word timing | `engine/levels.go`, `audio.go` | 531, 603 | RMS in dB every 10 ms, read from ffmpeg |
| Captions | `engine/ass.go`, `highlight.go`, `lines.go` | 689, 324, 707 | ASS written for libass, the rounded box measured by rendering it |
| Faces for the crop | `engine/faces.go` | 283 | a detector adapted from pigo |

## What exists, and whether it fits

### Reading the file in the page: mediabunny

[mediabunny](https://github.com/Vanilagy/mediabunny), MPL-2.0, about 7300
stars, released every few weeks and sponsored by Remotion and Mux among
others. Pure TypeScript, made to feed WebCodecs. It reads MP4, MOV, MKV,
WebM and more, the sample tables with B-frames, the colour tags from
`colr` and from the stream itself, and finds the sample for a moment.

- Fit: it would replace most of `mp4.ts` and give us MKV and WebM, which
  the video preview cannot play today.
- Gap: it honours an empty edit and one edit after it, and uses only the
  first of several. DaVinci Resolve writes two edits on the picture, an
  empty one and the media, which it handles. Our own files are the test.
- Licence: MPL-2.0 is copyleft per file, so it is fine in a closed app as
  long as its files are used unchanged, or changes to them are published.
- What stays ours: `plan.ts` and the clock in `queue.ts`. There is no
  established player that cuts on whole frames and places sound to the
  sample across cuts. mediabunny's authors leave that to the app as well.

mp4box.js from GPAC is the older choice, BSD, lower level: it hands over
the samples but leaves the edit list to us, which is what we would be
replacing. web-demuxer and libav.js put ffmpeg itself in WebAssembly,
exact but heavy, with LGPL on the WebAssembly file.

### Decoding and probing on the Go side: keep the ffmpeg program

[go-astiav](https://github.com/asticode/go-astiav), MIT, binds ffmpeg's
libraries through cgo. It would put decoding in the app's own process.

- Against it: a crash in C takes the whole app down, where a crash of the
  ffmpeg program fails only its job, which is a rule of ours. It is pinned
  to one ffmpeg release at a time. LGPL then asks for the libraries as
  replaceable shared libraries on three systems.
- For the program: it is isolated, LGPL is met by shipping it beside the
  app, and it is the code ffmpeg tests itself.

So the program stays. ffprobe's JSON already gives everything the Go side
needs, so no MP4 library is needed in Go. Should one ever be,
[abema/go-mp4](https://github.com/abema/go-mp4) and
[Eyevinn/mp4ff](https://github.com/Eyevinn/mp4ff), both MIT and active,
are the established ones.

### Colour: the standard, applied by us in one place

There is no library that turns a decoded frame into the right colours the
same way on every system. The standards are clear, and the engines differ:

- The tags of a file say its range, its matrix, BT.601, BT.709 or BT.2020,
  and its transfer, ITU-T H.273. A file without them is, by the defaults
  ffmpeg and mpv use, video range and BT.709 when 720 lines high or more,
  BT.601 below.
- Chromium and WebView2 show BT.709 with the sRGB curve, deliberately, a
  comment in their own source says so. Most players and the browsers that
  show Shorts and Reels do the same.
- WebKit on the Mac goes through ColorSync, which lifts BT.709's dark
  tones, the QuickTime gamma shift. WebKit also drew the frames the Go side
  made as full range whatever they said, which is what made `start.mp4`
  pale.
- A reference monitor in a dark room, BT.1886, is darker again.

So the way to show a file the same on every system is not to let the
engine convert it at all: take the decoded planes as they are,
`VideoFrame.copyTo`, and turn them into colours in one WebGL2 shader of
ours, from the file's own tags and the H.273 defaults, with BT.709 shown on
the sRGB curve. That shader is a few dozen lines written from the public
formulas, and its proof is the same frame converted by ffmpeg's zscale,
the reference converter, compared value by value. HDR, PQ and HLG, needs
tone mapping on top, BT.2390, later.

- It replaces: relying on WebKit's and Chromium's conversion, the full
  range forced on the Go side's frames, and the colour guess in `mp4.ts`
  once mediabunny reads the tags.
- The render is unchanged: it keeps the picture's numbers and copies its
  tags, so the short says what the episode said.
- QuickTime's lift stays out, so a Mac shows the video preview a little
  darker in the shadows than QuickTime. That is Apple's choice of display,
  not the file.

libplacebo is the reference for all of this on a graphics card, but it has
no port for the web, and copying its shaders would bring LGPL with them.

### Loudness: nothing to replace

What `levels.go` and `audio.go` measure is not loudness for normalising,
EBU R128, but how loud each 10 ms is, for the clip timeline and to put word
edges on the sound. That is plain RMS on samples ffmpeg decodes, a few
lines, and no library does it better. Should the app ever level its
shorts, ffmpeg's `ebur128` and `loudnorm` filters are the established way.

### Captions: libass already, and possibly in the preview too

The render's captions are drawn by libass inside ffmpeg. The video preview
draws its own in HTML, so the two can differ by a pixel or a line break.
[jassub](https://github.com/ThaUnknown/jassub), active, is libass in
WebAssembly: the preview would draw the same ASS the render burns in. It
brings FriBidi under LGPL, so its WebAssembly file ships beside the app.
Worth a look once the colours are done.

### Faces: not surveyed

`faces.go` is adapted from pigo. Whether an established detector fits
better is a question of its own, for later.

## The order proposed

1. **Colour**, now: one shader for every frame, whichever decoder made it,
   proved against zscale on files of every kind: 8 and 10 bit, video and
   full range, BT.601, BT.709 and BT.2020, tagged and untagged, the Go
   side's frames and the webview's. The pale `start.mp4` is its first case.
2. **mediabunny** in place of `mp4.ts`, behind the same tests the frame
   queue has now, and checked on Tim's own files, Resolve's above all.
3. **jassub** for the captions in the video preview, if the preview and the
   short differ visibly.

The ffmpeg program, the clock of the frame queue and the cutting of the
render stay as they are.
