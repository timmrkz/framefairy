# The video preview on one engine

A spec, being built. It decides how the video preview gets its picture
and its sound from here on, and what is removed on the way. Decided with
Tim on 8 October 2026, after the history below. Plan row 2.156. Steps 1
and 2 are built, every file's picture through ffmpeg and the episode's
decoder for it, in one pull request, #157, as Tim asked. Step 3, one
engine, is built in #162 and tried on Tim's Mac. Step 4 is built in
#168. Step 5 is built in #195, found needed by a video in Matroska. What
is not built yet says so where it is described.

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
   impossible in about 40. The one exception is loop, 2.150 and 2.183,
   which changes only where the play ends and nothing that plays. No
   other splice is built. Only
   the space bar and a click on the clip timeline change a play. An edge
   of the clip or of a cut, dragged or clicked while it plays, moves no
   playhead and changes nothing that plays: the edit is saved and drawn
   at once, and the queue keeps the new pieces for the next play, seek
   or click. Tim's rule, #277: that holds when the clip ends up shorter
   than where the playhead already is. An end dragged back past it lets
   the play run on to where the clip ended when it started, and the
   playhead stays there once it stops. Paused, the playhead goes with
   the edge dragged, a cut's edges too, see the clip timeline in
   [APP.md](APP.md). An undo while it plays moves no playhead either. Tim heard
   the play stutter with the hand on a cut's edge, because every step of
   the drag started the play again from the playhead.
7. **A rule before machinery.** When a case would need new machinery in
   the frame queue, the first question is whether a rule of the product
   makes the case not happen, as in rule 6. Machinery is built only where
   no rule does.

## What the parts do

### The episode's decoder

One program per open episode, started by the Go side when the episode
opens in the video preview and stopped when it closes. The frame queue
holds it for as long as it has the episode, and a held decoder is never
closed for standing unused.

- It is built on ffmpeg's own libraries, the code the ffmpeg program
  runs, so it decodes exactly as the render does. It is a program of its
  own, not part of the app, so a crash ends only it. It is
  `cmd/framefairy-frames`, written in Go with go-astiav, built by make
  against the ffmpeg make builds and linked to its libraries statically,
  as the ffmpeg program is. LGPL asks that a library linked into a
  program can be replaced, so the release has to carry what relinks it,
  or link the libraries as shared ones. That is for
  [PACKAGING.md](PACKAGING.md) before the first release. It speaks
  `internal/framewire` with the Go side, `engine.EpisodeFrames`.
- It keeps cursors, each the file open with its index read and a picture
  decoder ready, with the graphics chip where there is one. The chip only
  decodes: the frame is brought out as it is, and one chain on the
  processor scales it to the size of the canvas, the same chain on every
  system and for every file. Scaling on the chip as well was a second
  chain with a fallback, and the chip on Tim's Mac refused it for an HEVC
  file with 10-bit colour. Why the chip refused it is still to be found
  out. A file the chip does not decode at all, AV1 on an M2 or MPEG-4, is
  decoded by ffmpeg on the processor, with as many threads as it takes:
  that is the file's codec, not a second path of ours. When the chip
  fails a frame, in taking a packet, giving a frame or bringing one out,
  which VideoToolbox does now and then on a virtual Mac, the cursor opens
  its decoder again on the processor and goes on from the frame it is
  at. The render decodes on the processor, so that frame is the frame
  the short has. The cursor stays on the processor for the rest of that
  open and tries the chip again at the next, a jump or a new play,
  because ffmpeg starts a lost VideoToolbox session again by itself, as
  after the Mac slept. The log says it as a warning, with all ffmpeg
  said about why. Before, the cursor failed and the video preview stood
  still, one CI run in four, plan row 2.215. The engine's framing falls
  back for good after one failure, see `softDecode`, because it runs the
  ffmpeg program on a whole window and has no frame to go on from. mpv
  waits for three failed frames in a row before it falls back, and
  ffmpeg's program skips the frame, neither of which keeps the frames
  of the short. `FRAMEFAIRY_FRAMES_FAIL_AFTER=n` makes the decoder fail
  on the n-th packet of every open, for the tests, on any system. When
  the chip hangs instead, and one call into the decoder, a packet handed
  in, a frame taken out or brought off the chip, has taken 10 s, where
  it takes milliseconds, the decoder gives that cursor up and decodes on the processor from
  then on, every cursor at its next open too. ffmpeg's calls cannot be
  called off, so the work is left where it hangs. Reading the file is
  not timed, so a slow disk or a file still coming down from iCloud is
  no hang, and neither is a long seek, which is many short calls. The
  stream hears that
  the cursor hung and goes on from a new cursor at the frame after the
  last one it had. Before, the video preview stood still for good, as
  the chip did on CI's virtual Mac in the middle of a stream, issue
  #553. `FRAMEFAIRY_FRAMES_HANG_AFTER=n` makes the decoder hang once on
  the n-th packet a cursor of picture sends, and say so at once, so a
  test waits for no 10 s of the clock. A stream the
  Go side opens takes a cursor that a stream before it has finished with
  and only seeks it, so a jump starts no program and reads no index. Up
  to three are kept, and they are opened on the file as the decoder
  starts, before any frame is asked for. Cursors of sound, two kept, read
  the sound by the same rule the ffmpeg program reads it by for the render
  and the transcript, and give the same samples, step 3.
- A seek lands on a key frame shown at the moment asked for or before it.
  ffmpeg finds a key frame by when it is decoded, and in an open GOP, the
  way HEVC encoders and DaVinci Resolve write it, a key frame is decoded
  before the frames shown just ahead of it, which need the GOP before and
  are dropped when decoding starts at that key frame. So a seek to one of
  them landed a key frame late, and the frame asked for never came. The
  decoder reads the key frame a seek lands on and seeks to the one before
  while it is shown too late. Plan row 2.186.
- A jump forward that no key frame stands in front of is not a seek. The
  Go side hands a stream the kept cursor that stands closest before where
  it starts, on the same grid at the same size and no more than 5 s back,
  and waits for one whose stream has just stopped to finish the few
  frames of its batch, because the decoder has to make those first in
  any case. The decoder then reads on from the frame it is at, by its
  packets: when a key frame is found shown at the moment asked for or
  before it, it seeks after all, because decoding from that key frame is
  less work. A play after a pause and a jump a few frames on are then a
  frame or two of work, where they were all the frames from the key
  frame before. About one seek in six in Tim's report of two days was
  such a jump. When a cursor is let go of and three are kept already,
  the one kept longest is closed, so the cursor that stands where the
  playhead was is the one that stays.
- On the way to the first frame asked for, a cursor of picture does not
  decode the frames that no other frame is made from, the B-frames of
  most files, the way ffmpeg's `skip_frame` does with `nonref`. They are
  never shown, so nothing is lost. A jump 48 frames past a key frame of
  an MPEG-4 file with two B-frames in three decodes 18 frames where it
  decoded every one. The log says how many frames a jump decoded on the
  line of its first frame. Plan row 2.243.
- It takes requests from the Go side: frames from a moment on at a size,
  sound from a moment on at a rate in so many channels, and stop, see
  `internal/framewire`. A request for a new moment drops what the last one
  was doing.
- Not built yet: keeping the frames around the playhead it has already
  decoded, so a step back, a step on and a drag over them cost nothing,
  and while paused decoding the frames on either side of the playhead
  before they are asked for. The interface keeps the frames it was given,
  which covers a step back, so this waits until the marks say it is
  needed, with plan row 2.157. The interface asks for the frames a step
  goes to next, plan row 2.269: paused with nothing under the hand, the
  first frame of the word after the playhead and of the word before, the
  way Shift and an arrow walk, once the frame the hand asked for has
  come, see `AppFrames.ahead`, and the layers of their captions, which a
  step waits for before its frame is put up. After them come the first frames of
  the clips on the cards, the card under the pointer first and then the
  cards nearest the one chosen, with their captions, so a card clicked
  puts up its clip from what was read, issue #383. They are read once the
  workspace is on screen, a clip a search finds or Mark In makes as it
  comes, and kept for as long as the card is there, apart from the frames
  a play or a step keeps, as many frames as 400 MB holds at the size
  frames are read at, counted again when that size changes, `CARDS_BYTES`
  in `lib/frames/app.ts`. They are read only while paused, never on a
  second decoder beside a play, and read again when a workspace is woken,
  whose video preview opens with frames of its own. A workspace asleep
  reads none. Their captions wait while a word stepped onto is lifted
  out and bounces, which comes first. Everything read ahead goes one at a time, so a click
  that comes before it is in waits behind one read and not three. A
  jump forward in an open stream brings
  the frame before it too, for the arrow to the left. The decoder is told
  the moment before which nothing is wanted, `next` with a skip, so the
  frames a jump passes are decoded but never scaled. A frame asked of
  the stream while such a read is on its way, somewhere in what it
  skips, never comes from it, so it is asked of another stream. It was
  handed the frame before it, and the video preview showed that frame
  and said it showed the one asked for, issue #358. While the playhead
  is dragged, each place it passes shows its frame alone and cues no
  play, `FrameQueue.grip`, and the play is cued where the hand lets go.
  A place the hand moves to that needs a stream of its own opens it with
  `key`, and the decoder hands over the key frame its seek lands on
  before it decodes on to the frame asked for, issue #281. The place a
  press lands on does not, so a click shows the frame clicked and nothing
  before it. The queue puts that key frame up only when it is nearer the
  playhead than the frame on screen, and the exact frame still follows
  and takes its place. The frame asked for is the next frame the decoder
  hands over after the key frame: the frames between are decoded and let
  go unscaled, the way a jump lets them go. So a hand that outruns the decoder sees a frame
  near it at the cost of the key frame and the few frames stored next to
  it, not the frame of a place it left long ago, and a place it has
  already left costs its key frame alone, since the queue lets go of that
  stream sooner. Where the hand rests, and once it lets go, the frame on
  screen is the exact one, as before. The app's log says how each drag
  went once the hand lets go: how far the frame on screen was from the
  playhead as the hand moved, the key frames put up, and how long the
  frame under the hand took after letting go. The decoder's own lines say
  when an open asked for the key frame first and when it came. An open
  that finds a cursor already standing between the key frame and the
  frame asked for reads on from there instead, which is cheaper, and then
  no key frame comes: the open says it went on, and the page counts it
  apart from the key frames that came, `keyWentOn` and `keyCame` in
  `AppFrames.stats`, with the frames it read on to in `wentOnRanks`,
  issue #511.
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
- **The size of a frame** is the canvas's, no larger than the file's
  own picture. No frame is asked for before the stylesheet has laid the
  canvas out and told it its size, one frame after the workspace is on
  screen. A paused frame that came at another size than the canvas has
  now is asked for again whenever the app is resized, so
  a picture that stands still is always as sharp as the canvas lets it
  be. Plan row 2.187.
- **A paused frame** is the frame that holds the playhead. Paused on a
  piece, the play from the playhead is cued, its first frame drawn and
  the frames after it read ahead, the next piece's too. Whether the cue's
  first frame holds the playhead is asked of the cue's own plan, never
  worked out apart: a playhead a hair before a piece's end is in the cut
  for the plan, and is then a still of its own. Under a dragging hand, a
  frame that comes after the hand has moved on is put up while the next
  one is on its way, but only a frame that held the playhead, never one
  read ahead of it. Both kept the frames after a cut off the screen while
  a cut's left edge was dragged, which flickered between the two, plan
  row 2.270.
- **One moment at a time.** The video preview shows a frame with the crop
  frame, its shade and the captions that belong to that frame, never one
  of them ahead of the rest. A frame a gesture puts the playhead on, paused
  or as a play starts, is put up once its captions are there, asked for as
  it is decoded, see [CAPTIONS.md](CAPTIONS.md). A seek to another clip
  holds the crop frame and the captions over the frame on screen until the
  new clip's frame goes up, and all of it changes in that one frame,
  `held` in `Player.svelte`. Tim saw a clip card clicked move the crop
  frame first, the picture a few frames later and the captions a few after
  that, issue #383. Nothing waits under a dragging hand, which follows the
  hand.

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

**How even a play was is in the log.** Each play ends with a line, at a
pause or at the end, that says how many frames were late, which is a
frame not decoded in time, and how even the play was, `lib/frames/even.ts`.
Nearly every play on Tim's Mac said 0 frames late while he still saw it
not quite even, issue #251, so the line also says:

- the screen's refresh, from the time between two animation frames
- the animation frames the page missed, and the longest gap
- how many frames stood on the screen for how many refreshes, and how
  many stood a whole refresh longer or shorter than they last. A frame of
  25 a second on a screen of 60 Hz lasts 2.4 refreshes, so 2 and 3 are
  as even as the screen allows
- how far the clock, the sound heard as worked out on each animation
  frame, strayed from a straight line through the play, and where it
  came from: the output timestamp, the output timestamp held back to the
  sound card's own time, or that time less its latency. A frame goes up
  when the clock reaches it, so a clock that moves in steps puts frames
  up a refresh early or late, which the tests show for a clock that
  steps by 1024 samples

The first report with the line, three plays of start.mp4 on Tim's Mac,
said every frame was on time, no refresh was missed and the clock
strayed 1.5 ms typically. But the screen was at 60 Hz, so each frame
of 25 a second stood 2 and 3 refreshes by turns, 33 and 50 ms, and that
is what Tim saw. His Mac's own screen refreshes 120 times a second.
WebKit holds a page to about 60 updates a second, to save power on
websites, where QuickTime uses the whole 120. So the app lifts that hold
for its window as the page
comes up, `freeRefresh` in `cmd/framefairy-app/refresh.go`, and the log
says whether it took and what the screen can do. A page asks for an
update only while something moves, so a still app costs nothing more.
The beam, the motes, the shimmer and the pulse of work in hand move
only transform and opacity, which WebKit hands to Core Animation to run
at the screen's own rate with or without the hold, so a search costs no
more either. That is how WebKit draws them, not a measurement.

The second report, three plays with the hold lifted, said the screen can
show 120 frames a second and the page updated that often. Of 1576
frames, 317 stood 4 refreshes, 1254 stood 5 and 5 stood 6, and Tim found
it smoother. How long a frame stood, from the two reports, three plays
of start.mp4 each:

| Frame stood           | Before (ms) | After (ms) | Change      |
| --------------------- | ----------: | ---------: | ----------- |
| Shortest              |          33 |         33 | 0 %         |
| Longest, all but 5    |          50 |         42 | 17 % less   |

The 5 that stood 6 refreshes each stood over a refresh the page missed,
27 in about 63 s of play. WebKit
gives the page its times to the whole millisecond, so the refresh is
the mean of the times between animation frames, not the most common
one, which read 8 ms and a screen of 125 Hz.

To find what the page was doing when it missed a refresh, the line also
says where each animation frame's time went: how late its callback
began, putting the frame up, telling the app, feeding the decoders, all
of the callback, and the page's own work after it, typical, most and
just before a missed refresh. `frontend/preview/walks/smooth.mjs` plays
a clip of a 1080p video against the real Go side and prints the line,
see [TESTING.md](TESTING.md). A trace of such a play in Chromium showed
the page cleaning up memory for about 1.3 s of every 12 s: every frame
came in a new buffer of 7 MB, 170 MB a second. So the buffers are now
kept and handed back to the next pull once no frame holds them,
`lib/frames/pool.ts`, and a play makes no new memory for its frames.
Refreshes the page missed on Tim's Mac, from the second and the third
report, three plays each:

| Refreshes missed         | Before | After | Change     |
| ------------------------ | -----: | ----: | ---------- |
| A minute, buffers kept   |   25.6 |   9.1 | 64 % fewer |

At 120 Hz a frame of 25 a second still stands 4 and 5 refreshes by
turns, 33 and 42 ms, and so it does in QuickTime and Final Cut on the
same screen: macOS never changes a screen's rate for a video by itself.
Apple's advice is to set the screen by hand to a rate that divides evenly
by the video's, and the screen of a MacBook Pro offers 47.95, 48, 50,
59.94 and 60 Hz besides its own 120. At 50 Hz every frame of 25 a second
stands exactly 40 ms. The app could switch the screen to 50 Hz while a
video plays, and Tim chose on 21 October 2026 to leave the rhythm as
QuickTime has it. A video of 24, 30 or 60 frames a second is even at 120.

The line of a play is counted as the play goes, in sums and in samples
of a fixed size, so it costs the same and keeps the same memory however
long the play. Kept per animation frame, it took 90 ms at the pause of a
play of 10 minutes, and threw on one of half an hour, so the space bar
could not pause it.

The line of a play also says how long the picture waited for the sound
to start, about 45 ms, which every play does on purpose, see
`START_LEAD` in `lib/frames/queue.ts`. That wait once read as the clock
off its line by 23 to 47 ms each play, so the line is now fitted from the
moment the sound starts.

Tim's third report on #430 still missed about 9 refreshes a minute, and
the time went to the page's own work after each animation frame, not to
the frame queue's. While it played, the playhead moved on every
animation frame, and everything in the workspace that depends on it was
worked out again and laid out each time, 120 times a second on Tim's
screen for a video of 25 frames a second. Everything that follows the
playhead is drawn by its frame, so the playhead now moves only when the
sound heard is in another frame, `playMoves` in `lib/playhead.ts`, issue
#446. A trace of plays in the cloud, headless Chromium at 60 Hz, with
`smooth.mjs`'s video of a minute at 1080p, two runs of three plays each,
101.3 s of play a run, the work of a second of play averaged over the
two:

| Work a second of play       | Before (ms) | After (ms) | Change    |
| --------------------------- | ----------: | ---------: | --------- |
| The app's own JavaScript    |        33.4 |       20.8 | 38 % less |
| Working out styles          |        22.8 |       13.5 | 41 % less |
| Layout                      |        17.1 |       14.6 | 15 % less |
| Paint                       |        22.3 |       19.3 | 13 % less |

The playhead moved 3434 times in the first play of 60 s before and 1497
times after. The refreshes missed in the cloud are not in the table: the
cloud draws each frame on the processor, which takes most of the time
there, and they moved by more between two runs of the same code than
between before and after.

A playhead dragged faster than the frames under it can be decoded shows
the key frame on the way, issue #281, see The interface. `speed.mjs` in
the cloud, on its camera videos of 25 frames a second, five runs a side,
each five drags over a clip and back, with the key frame first turned
off for Before by an edit of `AppFrames.frame` and on for After, on the
same branch, off and on taking turns. The 1080p H.264 video stands in
for Tim's youtube.mp4. Each number is the median of the five runs, with
the lowest and highest run beside it:

| Frames behind while dragging | Before (frames) | Before, runs | After (frames) | After, runs | Change                        |
| ---------------------------- | --------------: | -----------: | -------------: | ----------: | ----------------------------- |
| 4K HEVC, median              |              66 |     61 to 71 |             48 |    45 to 55 | 27 % fewer                    |
| 4K HEVC, 90th percentile     |             142 |   122 to 154 |            112 |  106 to 121 | 21 % fewer                    |
| 4K HEVC, most                |             231 |   183 to 305 |            195 |  160 to 259 | 16 % fewer, inside the spread |
| 1080p H.264, median          |              10 |     10 to 13 |              8 |     7 to 10 | 20 % fewer, inside the spread |
| 1080p H.264, 90th percentile |              21 |     20 to 21 |             20 |    20 to 21 | 5 % fewer, inside the spread  |

At 4K the median and the 90th percentile moved further than the runs of
either side lie apart. Letting go of a drag, a far click, and the steps
by word and by frame came out inside the spread of the runs at both
sizes, faster or slower by a few per cent in most rows: the numbers are
in pull request #502. A click and a step never ask for a key frame first,
so they take the path they took before.

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

**Tim picked QuickTime's look**, on 9 October 2026, from start.mp4 at
the same frame in the video preview and in QuickTime, side by side: he
found QuickTime's blacks deeper and the video preview a little pale.
His screenshot of the two says what the difference is. Black is the
same in both and so is white, and QuickTime shows the middle tones
brighter: a grey the video preview showed as 22 QuickTime showed as 25,
46 as 55 and 81 as 93. The brighter middle against the same black is
what reads as deeper. The curve fitted to it is a value v of 0 to 1
shown as v to the power of 0.98 - 0.31 v, never below 0.891, which is
1.961 over 2.2, the Mac's curve for video over the screen's. It meets
the measured points within 2 and keeps black and white where they are.
It is `lift` in `frontend/src/lib/frames/light.ts`, put on in floats on
the GPU and in the same numbers on the processor for a 2D canvas. It is
the same on every system, so
on Windows and Linux the video preview looks as QuickTime does on a Mac,
a little brighter in the middle than most players there show the same
file. Above the brightest grey of start.mp4, 93, the curve is not
measured.

**Every frame comes in 10 bits**, standard video too. Tim saw banding
in the dark of start.mp4 with the look first made in ffmpeg, in 8 bits:
the curtain in stripes and a block on the black shirt, where QuickTime
showed smooth gradients. Video range spreads 220 values over 256, and the
look lifts the shadows further, so in 8 bits a value of the file became
a step of 3 in the dark, with the values between left out. In 10 bits
from `framewire.Picture` every value of the file keeps a colour of its
own, and the look on the GPU in floats makes each a step of about 0.27
of 255 there. The frames are no bigger, 4 bytes a pixel either way.

## HDR

**What it is.** Standard video, SDR, is made for a screen of about 100
nits. HDR stores much brighter highlights and more colours, BT.2020, with
a different curve, HLG or PQ. An iPhone films in HDR by default, as HEVC
with 10-bit colour, HLG and Dolby Vision on top. A file says it is HDR in
its tags: a transfer of `arib-std-b67` is HLG, `smpte2084` is PQ. The
engine's probe already reads these tags.

**What happened before.** The render copied the file's colour tags onto
a short encoded in H.264 with 8-bit colour, so a short made from iPhone
footage came out as HDR-tagged video with too few bits for HDR, and
players showed it in different ways. With our own ffmpeg it lost the tags
as well: a newer ffmpeg takes an encoder's tags from its frames and drops
`-color_trc` and the like.

### The short: HDR in, HDR out

Decided by Tim. A short keeps the quality of its episode, and an HDR
episode makes an HDR short. That is the rule of no colour changes.

- The short is HEVC with 10-bit colour and the episode's own transfer,
  primaries and matrix, HLG stays HLG and PQ stays PQ. Instagram and
  YouTube take HDR from an iPhone in this form. **This is done**:
  `hevc_videotoolbox` on the Mac and `libx265` where an ffmpeg has it,
  tagged `hvc1`, `HDREncoder` in `engine/encode.go`. The episode's colour
  tags are put on every frame of every short, standard video too, with
  `setparams`, so the encoder takes them, `TestAnHDREpisodeMakesAnHDRShort`.
- The captions are drawn at the reference white for graphics in HDR,
  BT.2408, 203 nits, so the caption colour looks on an HDR screen the way
  it was chosen, and not glaring. **This is done**: each colour of the
  caption style is taken as sRGB, turned into light with white at 203
  nits, into BT.2020's colours and into the short's own curve, which puts
  white at 75 percent of HLG's signal and 58 percent of PQ's,
  `engine/hdrcolour.go`. The caption file says `None` for its matrix, so
  libass draws in the frame's own range and matrix, BT.2020's, and a
  white caption stays white. A standard short keeps `TV.709`.
  `TestTheCaptionsOfAnHDRShortAreAtTheReferenceWhite` and
  `TestACaptionIsBurnedIntoAnHDRShortAtTheReferenceWhite`, which reads the
  burned white back from the frame.
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
for the extended mode for HDR. Standard video has nothing above white,
and its canvas is in the standard mode, the one the rest of the app is
drawn in. In the extended mode it turned the Mac's HDR on for every
video, which greys all ordinary white, and a screenshot then took the
canvas darker than the rest of the window by however much the screen had
to spare. On #201 the white of the captions came out 188 and 209 of 255
in two of Tim's pictures, beside the box a word is typed in at 255. The
walk "the GPU draws standard video with QuickTime's look, a step for
every value, and HDR as BT.2100's light" checks the mode. For HDR:

- ffmpeg turns each frame of HDR into red, green and blue with 10 bits
  each, from the file's own range and matrix, and leaves it in the file's
  curve, PQ or HLG, with BT.2020's colours. That is 4 bytes a pixel, as
  much as standard video, and 10 bits is what an HDR file has. The GPU
  turns it into light by BT.2100's formulas, in which 1.0 is the
  reference white of BT.2408, 203 nits, the white of the interface, and
  the highlights go above it, in BT.709's colours, `lib/frames/light.ts`.
  This was to be half floats made by ffmpeg with zimg, and is not: our
  ffmpeg's scaler makes no half floats, and the formulas are short and
  fixed. So the build needs no zimg. HDR is known by the file's transfer
  tag, `framewire.LightOf`, and every pull of its frames says so,
  `X-Frames-Light`.
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
| Our reader of MP4 and MOV | `frontend/src/lib/frames/mp4.ts` and its tests | 851 and 457, removed in step 5 |
| The Mac's decoder through cgo | `engine/pictures*.go`, `frontend/src/lib/frames/native.ts`, `/frames/native` and `/frames/decode` | about 600, and 148 of tests, removed in step 3 |
| WebKit's decoders | `WebPictures`, `PlainSound` and the reader of byte ranges in `queue.ts` | about 200, removed in step 3 |
| The choice between decoders | `fromGoSide`, `pictureFailed`, `nativeFailed`, `replaceDecoders` in `queue.ts` | about 100, removed in step 3 |
| Colour in the interface and the full range forced for WebKit | `queue.ts`, `app.ts`, `engine/preview.go` | a few dozen |

About 1,800 lines of code and 600 of tests, of about 6,400 in all, and
three of the four ways a picture can reach the canvas. The episode's
decoder is new code in their place, a few hundred lines that do one
thing.

## The steps

Each step is its own pull request, tested by Tim on his Mac before the
next one starts. Steps 1 and 2 went into one, #157, at Tim's wish.

1. **Measure first, remove nothing.** Every file plays through the ffmpeg
   program's streams that already exist, on every system. A change of a
   few lines in `queue.ts`. Tim compares it with main, by updating
   between the two from the Updates page, on `start.mp4`, an export in
   H.264, a file from a phone and a 4K file, against the marks in Speed.
   The walks measure where a jump's time goes: starting the program,
   opening the file, decoding from the key frame.
2. **The episode's decoder.** The waiting program of rule 3, for picture
   and sound, behind the same routes. The picture is built, the sound
   moves in with step 3. Measured against step 1 and the
   marks. The picture first: 20 paused clicks on the clip timeline of the
   bridge's episode came to their frame in 29 ms at the median, H.264 and
   HEVC with 10-bit colour alike, where the ffmpeg program took 78 and 94
   ms and main, decoding in the webview, 30 ms. The decoder hands out the
   first frame 4 ms after it is asked.
3. **One engine.** Remove WebKit's decoders, the Mac's decoder in cgo and
   the choice between them. Refuse at Add a file whose picture ffmpeg
   cannot decode. The sound through the episode's decoder as well. Built
   in #162 and tried on Tim's Mac.
4. **Colour and HDR.** The frames carry pixels converted by ffmpeg from
   the file's tags, and the interface draws them as they come. Proved
   against ffmpeg's own conversion of the same frame, value by value, on
   files tagged BT.601, BT.709 and BT.2020, in video and full range, 8 and
   10 bit, and on files with no tags. Then the side-by-side test, and Tim
   picks the look. HDR in both: the test of the extended canvas on Tim's
   Mac first, then the video preview on the WebGPU canvas and the HDR
   short with its captions at the reference white.
   The frames are colours now: the episode's decoder scales each frame
   and turns it into red, green and blue from the file's range and
   matrix, first in 8 bits and now in 10, see Colour, and the interface
   draws them with no colour space of its own. `TestTheEpisodesDecoderMakesFFmpegsColours`
   holds every value to within 2 of the ffmpeg program's own conversion
   of the same frame, on files tagged BT.601, BT.709 and BT.2020, in
   video and full range, in 8 and 10 bits, and on a file with no tags,
   and checks that the same numbers tagged otherwise come out otherwise.
   A frame is 4 bytes a pixel where I420 was 1.5, about 110 MB a second
   at 1280 by 720 and 30 frames a second.
   The test of the extended canvas was Colour Test in the Help menu, in
   the pull request of this step only, and it was removed before the
   merge: patches of 1, 2, 4 and 8 times
   white on a WebGPU canvas in the extended mode, beside the app's own
   white, with whether the webview has WebGPU, whether it kept the
   extended mode and whether it says the screen is HDR. In the cloud's
   Chromium it has WebGPU and keeps the mode. On Tim's M2 Max, on 9 October
   2026, it passed: the webview has WebGPU, keeps the extended mode and
   says the screen is HDR. The app's white and 1 × white looked the same,
   both greyer than usual, because macOS holds ordinary white back once
   HDR is on the screen. 2 × was brighter, and 4 × and 8 × were the
   brightest the screen could give at half brightness. With the app in
   the background the headroom shrank and 2, 4 and 8 looked alike. A
   screenshot cannot hold HDR and shows none of this. So WebKit gives a
   canvas HDR today, and the video preview takes the WebGPU canvas.
   HDR is drawn as HDR now, see The video preview under HDR. The walk
   `the GPU draws standard video with QuickTime's look, a step for every value, and HDR as BT.2100's light`
   holds what the GPU gives the canvas to `light.ts` worked out on the
   processor, and to the anchors: HDR's reference white is the app's
   white, PQ at 1000 nits is 1.99 and a green of BT.2020 lies outside
   BT.709. `TestTheEpisodesDecoderMakesFFmpegsColours` holds files tagged
   HLG and PQ to the ffmpeg program's conversion like the rest, in 10
   bits. Where there is no WebGPU, and for a colour taken from the
   picture, HDR is cut at white, as an SDR screen shows it.
   The video preview draws on that canvas now, `lib/frames/screen.ts`,
   for every file, where the webview has WebGPU, and on a 2D canvas where
   it has none. A frame is the app's own, `lib/frames/picture.ts`, its
   bytes as they came from the Go side, and no longer a WebCodecs
   `VideoFrame`: a VideoFrame holds nothing brighter than white, and
   handing one to WebGPU took Chromium's GPU process down in the cloud,
   where the bytes written as they are draw on every system. Headless
   Chromium gives nothing back from a WebGPU canvas, so the walks, which
   read the frame on screen from the canvas, draw the video preview on a
   2D canvas and check what the GPU draws on its own.
5. **The interface stops reading the file.** The program counts on the
   frames the engine names, the picture's start and the short's rate, and
   a piece is asked for by the render's own code. `mp4.ts` is removed.
   Ends 2.155. Built in #195, after Tim added a video OBS recorded in
   Matroska and the video preview said it had no moov box.
   `SourceInfo.Grid` in `engine/render.go` names the frames of the
   short: the short's rate, from the first frame of it inside the file to
   the file's end. The app hands it to the interface with the source, with
   the sound's rate and channels, and `lib/frames/movie.ts` makes what the
   queue plays from it: every frame of the grid, each one a decoder can
   start at, and the sound in packets of 1024 moments one after another.
   An open asks the episode's decoder for a frame of the grid, and the
   decoder makes each frame of it from the file's frames by
   `framewire.Grid`: the frame that holds its moment, the last to begin
   at or before it, give or take a millisecond, and the file's first
   before the picture begins, counted in whole microseconds the way the
   render's `fps` filter counts in `byTimes`. A frame of the file that
   stands for no frame of the grid is let go unscaled, and one that stands
   for two is scaled once. Times count from the start of the file, as the
   engine and the sound count them, where the picture had counted by the
   file's own stamps, off by as much as a file starts late. Where the file
   starts on its own clock is the grid's origin, the engine's reading of
   it, so the decoder never works it out a second time: two builds of
   ffmpeg read the start of a Matroska file with AAC 21 ms apart. The page
   hands the grid back with every stream it opens, and the Go side makes
   the frames of that grid, so the two cannot count on different grids,
   not even for a file written over while the workspace slept. A file
   with no index of its key frames, MPEG-TS, lands a seek between two key
   frames, and the decoder read on to the next one and showed it for
   every frame before it. A seek now finds the key frame shown at or
   before its moment, and the decoder seeks again if its first frame
   begins after it. A seek after the last key frame of such a file found
   no key before the end and showed nothing, so a jump into the last
   stretch of an MPEG-TS video stood empty. It now goes back to the last
   key frame too, issue #287. The route that served a file's bytes to the page,
   `/media/`, is removed with the reader that used it.
   `TestTheVideoPreviewShowsTheFramesOfTheShort` holds the decoder to the
   render's rule on a phone's uneven frames, a screen recorder's left-out
   frames, left-out frames the probe does not see, a picture that starts
   late in MOV and in Matroska, even frames in MP4 and in Matroska, what
   OBS records, H.264 with B-frames and AAC in Matroska with frames left
   out, MPEG-TS, and a Matroska recording cut off by a crash, which
   says nothing of how long it is, and the walk `a video in Matroska
   shows in the video preview the frames its short has` holds the video
   preview to the short.

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
  is code we keep, about 700 lines, and its Go side, `engine.EpisodeFrames`
  and the protocol, about 450 more. It is still ffmpeg's decoding, only
  kept running.
- **A video preview a little brighter in the middle tones than most
  players on Windows and Linux**, because Tim picked QuickTime's look.

## What established players do

Researched on 8 October 2026 in the source of Chromium, WebKit, mpv,
ffplay, MLT and Shotcut, Olive, libopenshot and VLC, for the steps above.
What applies here:

- **Scrubbing.** Olive drops queued requests for frames the hand has left
  but lets a decode already running finish, which is what 2.156 does
  with one stream on its way at a time. MLT decodes forward rather than
  seeking when the target is less than 64 frames ahead, and libopenshot
  keeps decoded frames around the playhead, ahead in the direction of
  travel. For the episode's decoder: decode forward inside the group of
  frames, keep a ring of frames around the playhead, and while the hand
  outruns the decoder show the nearest key frame first.
- **Seeking.** mpv and MLT tell the decoder to skip frames nothing refers
  to on the way to the target, `AVDISCARD_NONREF`, and convert or scale
  nothing before it. ffmpeg's VideoToolbox decodes one frame at a time,
  so a seek costs the frames from the key frame to the target.
- **Playing evenly.** Chromium picks the frame for the display interval
  the draw will land in, and locks a cadence, two refreshes a frame for
  30 frames a second on 60 Hz, when that glitches less than once in 8
  seconds. It holds about four frames and grows that after an underflow.
  The frame queue picks the frame for the moment of the animation frame,
  with no cadence, and jitter there turns 2-2-2 into 1-3: the likely
  cause of the unevenness Tim sees, not yet proved.
- **Frames into the webview.** WebKit on the Mac copies an I420 frame
  into NV12 on the processor before it draws it on a canvas. Chromium
  converts on the graphics chip. Frames that ffmpeg has already turned
  into colours, uploaded to a WebGPU canvas with `writeTexture`, take one
  upload and no conversion in the webview, on every system, which is
  step 4.
- **Reliability.** ffplay numbers every request and drops answers to old
  ones. MLT draws a frame after a few dropped in a row so the picture
  never freezes. Olive asks again after 250 ms with nothing. mpv falls
  back to software decoding after three errors of the graphics chip,
  still ffmpeg.

## What this replaces

- **The playback copy of the episode**, plan row 1.5b, made for files the
  video preview could not play. With one engine the video preview plays
  whatever the render can read, and no copy is made.
- **The survey of media libraries in #151**, which proposed a colour
  shader of our own and mediabunny in place of `mp4.ts`. With one engine
  neither is needed, and #151 was closed for this spec.
