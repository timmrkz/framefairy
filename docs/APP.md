# framefairy-app, the desktop app

The app does what the command line does, on screen: it transcribes an
episode, finds the moments worth clipping, lets you check and adjust them on
a timeline and renders them as vertical shorts. It runs the same engine as
`framefairy`, so both give the same results from the same files.

Install the tools and models first, as [INSTALL.md](INSTALL.md) describes.

## Build and start

```
make run
```

That builds everything and starts the app. `make` alone builds it into
`bin/framefairy-app`. [BUILD.md](BUILD.md) covers the details and building
without make.

Building needs Node.js, because make builds the interface before the app.
The result is a plain program for now. The signed macOS app, the Windows installer and live reloading of the
interface come with the packaging work.

## Using it

### The first run

A new copy of the app on a machine with nothing on it needs two things, and
only one of them is a question. Until both are answered the setup is the
whole app: no sidebar, no workspace, nothing to press that would not work.

**Speech** is always local. No speech model ships with the app, so the app
fetches one. There is one today, which is not a choice, so the app says what
it is about to do and does it: the download starts by itself and reports how
far it has come, with the same fill every other piece of work in the app
wears, and **Cancel** stops it. The row says what the model is, what it
covers, what the download costs and what it costs on disk, before anything
starts. It is a job like any other, in the lane of hearing, so it shows in
**Activity** too, and a search that comes to hearing waits for the model
rather than failing on it.

**Finding clips** is the question. A model in the cloud, Anthropic's or
OpenAI's with a key of your own, works on any machine and
costs a few cents an episode. A model on this machine is free to run and
needs the memory to hold it. Either way only the words are read: the video
and the audio never leave the machine. The answer is saved the moment it is
given and can be changed later in the settings.

Choosing the cloud opens a choice of model, Claude Sonnet 5 or GPT-6 Sol, and a field for that company's key, which goes in the macOS keychain
and nowhere else, never in the settings file.

Choosing **On this machine** shows the models that can be installed, each
with its maker, what it costs to fetch, what it costs in memory to run and
what this machine can do with it. That last part is the point of the list.
A model runs from memory, so a machine too small for one will swap, and a
model that swaps takes minutes to answer rather than seconds. The app reads
how much memory the machine has and says, beside each model, **Best for
this machine**, **Fits this machine**, **Tight on this machine** or **Too
big for this machine**, with the machine's own figure above the list so the
judgement can be checked. The best one is the largest model the machine
can hold comfortably, largest by the model and not by the memory it
takes, because the two are not the same thing: see PACKAGING.md.

Nothing is hidden and nothing is refused: a model the app thinks is too big
can still be installed, because a machine's memory can be read wrong and it
is not the app's place to decide. A machine that will not say how much
memory it has is offered the smallest, because that is the one most likely
to run, and nothing else is promised.

A `.gguf` already in `~/.framefairy/models` is used as it is, whether the
app fetched it or not.

**Installing is work in hand, and it looks it.** The button it was started
from carries it, the way every control that starts work does: the beam
round it and the fill for how far the download has come, and it reads
**Cancel** while it runs. What has arrived stays, and **Install** carries on
from it. The row itself does not light up or move. The line under the name
that says what the model costs says instead how far it has come, how fast
and how long is left, *Fetching 1.8 GB of 5.2 GB at 42 MB a second, 0:13
left*, in the same one line, so the row keeps its height. The numbers are
counted in thousands, the way the size to fetch is, so a model said to be
5.2 GB arrives as 5.2 GB of 5.2 GB.

A model is only half of the local way. `llama-server`, from llama.cpp, is
what runs it, and a model without one is fifteen gigabytes that answer
nothing. So the app looks for it and says so while it is missing, and it
does not call itself ready on a machine that has the download and no way
to open it, which it used to. It is taken the way ffmpeg is: the one
beside the app, checked to be the file the app was built with, and no
other, see **The tools are the app's own** below.

The last button is **Finish setup**, and that is all it does. Adding an
episode has one way of being done and it is the plus in the sidebar, so a
second way here would be a second way of doing one thing, bought for a
click that is only ever saved once. While the speech model is still coming
the button says so and waits, because there is nothing to be done with the
app until it is there.

A key found in the environment is not an answer to the question. The app
never decides on somebody's behalf, so an app that has never been asked asks.

### Episodes

The sidebar lists your episodes and how far each one is. **Add episode**, at
the bottom of it, opens a file dialog. Any mp4, mov, m4v or mkv works.
Transcription starts right away in the background.

The episodes are listed in the order they were added, the newest at the
foot, where one just added is brought into view. Once there are two, the
sort mark at the right of the head of the sidebar, an arrow up beside an
arrow down as Reminders, Notes and Files have it, opens a list of
**Added**, with a clock, and **Name**, with A over Z. A tick marks the one
that is on. The sidebar stays open while
that list is, and after it closes until the pointer moves away. Escape
closes one thing at a time: the list, then the sidebar hovering opened. By name, Folge 2 comes before Folge 10. The choice is kept by
the app, like whether the sidebar stays open. A library from before this
kept no order, so its episodes stay in the order they were in, sorted by
where they are on disk, and the ones added from now on go at the foot.

The sidebar is a narrow rail until the pointer reaches the left edge, and it
opens over the workspace while it is there. The mark at the top keeps it
open, and clicking it while it is open closes it at once, even with the
pointer still on it. Hovering opens it again once the pointer has left and
come back. Closed, it leaves room for the settings column of the workspace,
which is exactly what it covers when it opens. **Add episode**, **Activity**,
**Settings** and **Updates** sit at the bottom of it and stay reachable as
marks on the rail. Updates says which build is running, and a still dot on
its mark says a newer one is ready. Its page has the build and its commit,
the channel it follows, where things stand, and Check or Relaunch. See
[UPDATES.md](UPDATES.md). Every mark is in the same place on the rail as it is in the open
sidebar, to the pixel, so opening the sidebar never moves the mark out from
under the pointer that came for it. The Activity mark has no dot of its
own: work in hand is the lamp of the episode it is for, and a second dot
said the same thing twice.

The episodes stay on the rail too, as their lamps. **An episode's row is
one line and the rail's own 36 pixel box**, so shut it is a square around
its lamp, exactly like every other mark on the rail, and open it is the
same square with the name beside it. It keeps that height and its lamp
keeps its column whether the sidebar is shut or open, so the rail is one
column of marks from the top to the bottom and nothing in it moves as the
sidebar goes over. The name is what waits for the room, along with the two
marks on the row, which need a pointer on the row anyway. What the episode
has, its transcript and its clip sets, is in the row's own title: it is
one line about state on a rail of lamps, and it made the row two lines
tall, which left the lamp adrift in a box half again its size.

The marks on an episode's row, on the right of it, show it in the file
manager and remove it. **Remove** asks in a box over the workspace whether to
keep the episode's files or delete them. The box names the folder they are
in, `<episode>.framefairy` beside the video. Keeping it means adding the episode
again picks up the transcript, the clip sets and the rendered clips where
this left off, which is why it is the highlighted answer. Deleting it takes
the whole folder, so nothing of the episode is left behind. Whatever is
running on that episode is stopped either way, so a removed episode never
goes on transcribing and holding up the episodes added after it. From the
moment removing begins, nothing new starts on it until it is added again,
not even the transcription a stopped search would otherwise carry on.
Deleting also waits for the work to stop, and if something will not stop,
nothing is deleted and the episode stays where it is with a line saying
so. The answer clicked says Removing at once and the box takes no second
click while the work stops, which is a moment when a search runs. The
workspace of the episode closes before the list is read again, so it asks
nothing more of an episode that is gone. A folder deleted under a running transcription comes back,
half written, for an episode that is no longer in the library. The video file
itself always stays, and so do the training records, which live in one
folder of their own and are thrown away in the settings and nowhere else.

The list of episodes is the app's own, in its config folder, not in those
work folders. So an episode stays in the list when its folder is deleted by
hand, and what it lost is simply gone: opening it then is the same as adding
it, and the transcription starts by itself. With no folder beside it there is
nothing to keep or to throw away, so the box says so and offers one
**Remove**.

Anything that cannot be taken back asks in that same box, and nothing else
does. Its buttons read left to right from the safest answer to the one that
cannot be taken back: **Cancel**, then what is recommended, then what is
destructive. The box opens on **Cancel**, the arrow keys and Tab move
between its buttons, Enter takes the one that is highlighted, and Escape
leaves everything as it was.

What is destructive is never the highlighted answer and never the one the
keyboard starts on. It is marked as what it does instead, in the colour of
a warning, so it is read before it is clicked rather than after. Making it
the default would mean that Enter, or a second Return still held down from
somewhere else, throws work away, and the whole reason this box exists is
that a click which throws work away has to be a click you meant.

### The bar

A bar runs across the top of the app. It holds the close, minimise and zoom
buttons on macOS, it is what the app is dragged by, and it says what is on
screen: the name of the episode, or **Activity**, **Settings** or **Acknowledgements**. No screen
writes its own name below it, and the sidebar opens under it, so the name is
always there to read.

On macOS the bar **is** the title bar. macOS lays that out and centres its
three buttons in it, and nothing an app can set moves them by a pixel: the
height the app asks macOS to leave empty only says how far down a drag
still moves the app. So the app takes the height it was given rather than
asking for one, and then the buttons are on the bar's middle because the
bar is what they were centred in. Everywhere else the system draws its own
title bar above the app and this one is an ordinary header.

The Go side measures and the stylesheet lays out. `FrameFairy.Chrome` answers
four numbers in whole pixels, the title bar's height and the left edge,
right edge and middle of those buttons, and the interface is told again
on the `chrome` event whenever the answer changes: a resize, either way
through fullscreen, another display, another scale, light or dark, back
from the Dock. Nothing is ever moved, so there is nothing that can snap
back, and nothing is measured on the page. The four numbers land on the
shell as `--bar-h`, `--lights-l`, `--lights-r` and `--lights-y`, and the
bar lays itself out from them. `--bar-h` is in `app.css` as well, which is
what the first frame uses and what every other system keeps. In native
fullscreen there is no title bar and no buttons, so the height stays as it
was and the name moves to the left edge rather than the bar collapsing.

The name of what is on screen sits on the buttons' line. Its box is
trimmed to the capitals and the baseline, so it is the letters that are
centred and not the space a line of text reserves around them, and then it
is padded back out so a name with a tail below the baseline, `YouTube.mp4`,
keeps it. A name too long for the bar ends in an ellipsis.

The colour of the app's window itself is the bar's colour, `--ink-1`. It
is only ever seen where the page does not paint, which on macOS 26 is the
sliver between its rounded corner and the webview's, and at the top that
sliver is inside the bar.

### The workspace

Selecting an episode opens its workspace. It is three columns, the settings
on the left, the video preview in the middle and the clips on the right, with
the clip up close under all three. It fits the app without scrolling.

The video preview plays the episode from its own queue of frames, see
`frontend/src/lib/frames/`. The app reads the samples of the picture and
the sound straight from the episode file, a range at a time, decodes them
with the system's decoders a few frames ahead of the playhead, draws each
frame onto one canvas and hands the sound to the sound card, which keeps
the time. Each frame is drawn when the sound heard reaches where that
frame begins, through the clip's pieces, whatever moment the play began
at. A cut is frames never queued: the last frame before a cut is followed
straight by the first frame after it, both always drawn, and the sound
meets at the sample, faded over 15 ms either side the way the render
fades it. So a clip plays in the video preview the way it plays in the
short.

Paused, the picture is the frame that holds the playhead, exactly, wherever
the playhead was put: a click, a drag, a step of an arrow key or a word.
The play from there is already prepared, its first frames and its first
sound decoded, so the space bar starts it at once rather than decoding the
same frames again. The picture moves about 80 ms after the space bar in
the harness, most of it the sound card's own latency and the first
frame's own time on screen, since the picture waits for the sound to be
heard. While it plays, the playhead is the sound being heard,
on every animation frame, so it moves smoothly, jumps a cut with the sound
and stands in the frame on screen. It never goes back while playing. A
click while it plays puts the playhead where it landed in that frame and
goes on from there, the clip's or the episode's by where that is, a click
across the clip's edge as well. A clip's pieces changing while it plays, a cut
made, moved or put back, or loop switched on or off, goes on from the
playhead on what the clip is now.

A clip played from a playhead that stands in one of its cuts plays from
where that cut ends, see `playFrom` in `lib/playhead.ts`: the queue is
given the clip's pieces and a seek to the end of the cut before it plays,
so not one frame or sample of the cut is queued. A double-click that cuts
a part out leaves the playhead in it, and the space bar then played the
part just cut, which the render does not have. The playback walk found it
while 2.121 fixed it, and holds it fixed: it records every frame the queue
draws while a clip plays and fails on one from inside a cut.

The canvas is as many device pixels as the stylesheet makes it, and the
frame is drawn into it in its own shape, so a 4K episode is never kept at
its full size. Opening another episode closes the queue of the one before,
its decoders and its sound card with it.

The queue reads MP4 and MOV, H.264, HEVC and VP9 for the picture, AAC,
HE-AAC, Opus and plain sound for the sound, as far as the system decodes
them. A file it cannot play, a codec the system cannot decode, a
fragmented MP4, a file that is not an MP4 or a MOV at all, says so in one
sentence where the picture would be, with the reason, and nothing else in
the workspace changes. A sound it cannot decode plays the picture without
it and says so at the foot of the picture. The playback copy of the
episode for such files is plan row 1.5b.

A system can also say it decodes a file and then fail on it: WebKit says
yes to HEVC with 10-bit colour and its decoder then fails on the first
frame, "Decoder failure". For such a file, and for one the system says no
to, the Go side decodes the picture instead, plan row 2.126. The queue
asks its picture decoder the same things either way: `AppPictures` in
`lib/frames/app.ts` answers the calls of a `VideoDecoder`, like
`PlainSound` stands in for the sound decoder, so drawing, the clock, cuts
and the walks are the same. Only the frames the queue will draw are asked
for, the file's picture is not read by the page at all, and like a
decoder it puts them out in the order they are shown, not the order they
were fed: fed in the order they decode, a run's B-frames came too early
and its first frames too late, and 72 frames of a play of three seconds
were dropped as late until it did.

Where the system has a decoder of its own for the picture, VideoToolbox on
the Mac for H.264 and HEVC, the queue uses it, `NativePictures` in
`lib/frames/native.ts` and `engine.Pictures`: the page reads the file as
it does for its own decoder and sends the samples, a batch for whatever it
fed in a moment, to `POST /frames/decode`, and the decoder, one for each
of the queue's two, stays open in the app's own process for as long as
the episode does. So a jump starts no program, reads no index again and
makes no session again: it costs only decoding from the key frame before.
VideoToolbox makes the frame the size of the canvas and 8-bit itself, on
the graphics chip, and the page gets it in NV12 as it comes out. A
decoder that will not open, or fails on a frame, or puts out a frame at
another size than asked for, hands over to ffmpeg's streams below.

    POST /frames/native?codec=&cw=&ch=&w=&h=   the avcC or hvcC box   {"id": ...}
    POST /frames/decode?id=                    samples, the frames kept come back

Elsewhere, or where it will not take the file, the frames come from
ffmpeg in streams the page opens and pulls from:

    /frames/open?path=&from=&w=&h=   {"id": ...}
    /frames/read?id=&n=&skip=        up to n frames
    /frames/close?id=

A stream runs the ffmpeg the app ships, on the system's own decoder where
there is one, from the key frame before `from`, and hands over every
frame from `from` on, scaled to `w` by `h` in 8-bit I420, each after the
moment it starts at as a float64 in 8 bytes. ffmpeg decodes a frame ahead
of what was pulled and waits: WebKit takes whatever a response writes
without pushing back, so one long response would have let it decode the
whole episode into memory while the page stood paused. A stream nobody
pulls from for 20 seconds is closed, and at most six are open. Like
`/media/`, it only reads a file of an episode in the library.

What makes it quick, since a new stream costs ffmpeg starting, reading the
file's index and decoding from the key frame before:

- A stream that will reach a frame within a second and a half is waited
  for rather than a new one started, and the frames it passes on the way
  are dropped on the Go side, `skip`, not sent. So the arrow keys stepping
  on, and a paused play going on, cost a frame each.
- The frames shown last are kept, 96 MB of them, so stepping back or a
  still close by costs nothing.
- A frame is decoded at the size of the canvas and no larger than the
  file's own.
- The queue's second decoder asks for the piece after a cut while the
  first plays, so its stream is open before it is needed.
- On the Mac, ffmpeg makes the frame smaller and 8-bit on the graphics
  chip, `scale_vt`, before it is copied out of VideoToolbox, a fraction of
  the 6 MB a 1080p frame of 10-bit colour is. A chain that fails before
  its first frame is made again on the processor, and every one after it.
- A pull is read in a Worker, `lib/frames/pull.worker.ts`, each frame
  straight into a buffer of its own as it arrives and handed to its
  `VideoFrame` without another copy, so nothing on the page waits for it.
  Where a Worker cannot reach the Go side or hand a frame back, the same
  code runs on the page.

Measured on the cloud machine, four cores and no system decoder, with the
bridge's HEVC 10-bit episode at 320 by 180 in Chromium: a click on the
clip timeline shows its frame 65 to 115 ms later, a frame from a stream
already open comes 1 to 5 ms after it is asked for, a play of three
seconds draws every frame with none late, and the arrow keys show the next
frame and the one before within a few milliseconds. The prototype,
measured before the queue used it, with an HEVC 10-bit episode at 1920 by
1080 read as fast as it came and drawn on a canvas:

| Size | First frame after a jump | Frames a second after it | MB a second |
| ---: | ---: | ---: | ---: |
|  640 by 360 | 266 to 277 ms | 122 to 141 |  27 to 35 |
| 1280 by 720 | 145 to 337 ms |  48 to 105 |  63 to 109 |
| 1920 by 1080 | 174 to 288 ms |  66 to 89 | 187 to 223 |

A podcast needs 25 or 30 a second.

It used to be a `<video>` element, and everything about it was a
workaround. The element answers a seek before its frame is on screen,
answers a paused seek with the start of the frame it shows, runs its clock
ahead of the picture as playing starts and then stands, drops a seek while
the machine is busy, and has no clock at all before it has read the file.
A still read by the engine stood in for the picture while it caught up,
and a cut was a seek in the middle of playing, which held the last frame
before the cut for as long as the seek took. The frame queue has none of
that to work around, because the app decides every frame it draws.

**The space bar plays the clip or the video, and the playhead says which.**
The playhead is either on the chosen clip or on the video, and the gesture
that put it there decides, never a clock. Picking a clip puts it
on the clip, at its start. So does a click on the clip's start edge, a
trim, a click on a caption, and a click, a drag or a step into any frame
of the clip, cuts included, from the frame that holds its first moment.
A click on the end edge, a landing in the frame that holds the clip's
last moment, and the clip playing to its end, leave it on the clip at its
end. Any frame before or after the clip is on the video, and so is having
no clip chosen. It goes by frames and not by seconds because what is on
screen is a frame: a clip that starts inside a frame shows its own first
frame from that frame's start, which by the second is before the clip.

On the clip, the space bar plays the clip from the playhead: its cuts are
jumped and it stops at its end, or with loop on goes back to its start.
From inside a cut it plays from the end of that cut, and from the clip's
end it starts the clip over, the way QuickTime starts over at the end of a
video. On the video, the space bar plays the episode straight on from the
playhead, through the chosen clip and its cuts, with no jump and no stop,
so any part of the episode can be heard with a clip chosen. Playing through
the clip leaves the playhead on the video, and loop changes nothing there.
While the playhead is on the video the chosen clip is dimmed as one thing,
wherever it is drawn: the crop frame in the video preview, its mark on the
range picker, and on the clip timeline everything drawn for it, its frame,
its pieces, its cuts and their edges, its trim edges, its caption blocks and
their edges and its thumbnails. So it shows that the clip's rules are not in
play. It is one rule, `.chosen.dim` with `.frame.dim` and
`.clipmark.selected.dim` in `app.css`, at the brightness of a button that
cannot be pressed, `--dimmed`, and it changes in the frame the playhead
does. With the hand on the clip somewhere else, its card or its mark, it is
lit whole.

It used to be measured: the clip played when the playhead was within half
a frame of its start. The paused video element on the Mac answered with
where the frame it showed began, up to a frame before the moment it was
sent to, so a clip just picked measured as before its own start, and the
space bar played the episode straight through its cuts and past its end.
See the rules in `frontend/src/lib/playhead.ts`.

Opening an episode that already has clips opens on one of them: the clip it
was last worked on, or the first one where it has never been opened. The
playhead goes to the start of that clip, because that is what choosing a
clip does. Where the playhead stood when the app was closed is not kept, no
editor keeps that, and the start of a clip is a place that means something.
Which clip it was lives in the episode's own folder, so it goes when the
folder goes.

Putting the playhead somewhere on the range picker is asking to look there,
so the clip timeline goes there too. With a clip chosen that counts as moving
the view by hand, and the view stays where it was put. **The playhead is
dragged the same way on both**: a press anywhere on the track or on the
playhead's head takes hold of it, and the video preview follows the hand,
one seek a frame, until it lets go. The range picker only went there once
the hand let go, and Tim asked for it to behave like the clip timeline. It
is one function, `scrub` in `frontend/src/lib/scrub.ts`, used by both. The
playhead is taken hold of by its line as well as its head, a few pixels
either side, and on both tracks it shows the arrows left and right under
the pointer, the way a clip's edges do. The empty track shows the hand.
The two differed: the clip timeline seemed to show the arrows on the
playhead only because the playhead so often stands on the chosen clip's
edge.

The **crosshair** in the row under the timeline goes to the playhead, always,
clip or no clip, and puts it in the middle of the view, because the reason to
ask is to see where it is. Going back to the clip is what clicking the clip in
the list does, the one already chosen included, so the crosshair does not do it
as well: a control that went to the clip with one chosen and to the playhead
without could not be relied on for either.

The video preview is as big as the room allows and keeps the shape of the
episode, so it grows until either the height or the width runs out. The
middle column is exactly as wide as the picture, and the settings and the
clip list share everything left over. A wider app makes those two wider
rather than leaving a strip of nothing beside the picture, and a taller
app makes the picture bigger. Once the picture is as wide as it may be,
the height left over goes to the two tracks: the clip timeline grows and the
range picker stays exactly half of it, so nothing is left empty at the foot
of the app.

The two tracks are one episode seen from two distances, so they lie on one
floor, `--well`, a step darker than the workspace around them and never
black, the way the editors people know draw a timeline. What is chosen on
them, the window on the range picker and the clip on the clip timeline, is
marked by its frame and the wash in it, and nothing around it is darkened.
The range picker used to darken everything outside the window while a
search ran, so it changed colour as searches came and went and never
matched the clip timeline. The video preview still darkens what lies
outside the crop while the playhead is in a clip, because that part is cut
from the short. Anywhere else, in a cut or between clips, nothing is laid
over the video preview at all. The crop used to stay there as a grey
dashed frame with the shade round it, which laid a short over a part of
the episode that is in none.

All of that is one expression in the stylesheet, worked out from the size
of the app itself and the tokens in `app.css`. Two things it cannot know
come in as custom properties, the shape of the episode and the height of an
error line above the workspace, and neither changes because the app was
resized. So dragging the edge of the app costs no JavaScript at all and the
workspace keeps up with the edge instead of arriving a frame behind it. The waveform is the
one thing still told its size in pixels, because a canvas has to be, and
nothing is laid out from the answer.

Two things are called what they are, here and everywhere else. The slim strip
under the video preview, the whole episode at a glance, is the **range
picker**. The waveform under the whole workspace, the episode up close, is the
**clip timeline**.

Nothing explains itself in a line of text that is always on screen. Every
control says what it is for when the pointer rests on it, and the small info
marks open a bubble that says more, on hover or on a click. A bubble is a few
sentences, never an essay, and it hangs from the page rather than from the
area it belongs to, so nothing clips it and nothing lies over it. An area
with an info mark carries no tooltip of its own: one explanation, in one
place.

**Every word of a bubble can be read, always.** It hangs under its mark,
over it, or beside it, the first of those that holds the whole of it
inside the app, and a long text that fits nowhere at the narrow width is
made wider, which makes it shorter. It never covers its own mark. Only an
app too small for it at any width gets a bubble as tall as the app that
scrolls. The rule is `placeBubble` in `frontend/src/lib/bubble.ts`, with
tests that put a mark everywhere in apps of several sizes. The clip
timeline's bubble used to fit neither under nor over its mark, went under
it anyway and ran off the foot of the app. Moving into the bubble keeps it
open: an eight pixel bridge on the side facing the mark carries the
pointer across the gap. It used to sit inside a bubble that clipped its
overflow, so over the mark the bridge never worked, and the range picker's
bubble scrolled by those eight pixels.

- **The transcription is the first step of a search, and nothing else.**
  A search is one job on the Go side, see [JOBS.md](JOBS.md): it hears the
  episode from where the transcript ends to the end of its window, stops
  exactly on its edge, and then finds. Adding a video asks for its first
  search, of its first window, so the Go side starts it and the
  workspace only shows it. Nothing carries the transcription on through
  the rest of the episode afterwards. So there is nothing to control about
  it but the search: **New** starts it and **Cancel** stops it, and what
  was heard stays, so the next search goes on from there. While another
  episode's search finds its clips, a search that hears waits and carries
  on after, because the two models want the same memory.
- **What is not there yet says so by waiting.** The part of the clip
  timeline the transcript has not reached and the rows the clip list will
  have are places waiting to be filled: the shimmer passes over them, the
  same light the window on the range picker shows while clips are being
  found for it. On the clip timeline the waveform is there for the whole
  episode within seconds of adding it, see below, and nothing is drawn
  over it for the part not heard yet: a grey band across the middle,
  where the captions stand, looked like a hole in the sound rather than
  words still to come, and said nothing the clip cards do not. The
  range picker carries none of the reading either. It once did, the part
  not transcribed darker and the part being heard filling, but how far
  the work has come is said on the clip cards, the transcribing and the
  finding alike, and the range picker said only the first half of it.
  Tim had it taken away: while clips are found the window breathes and
  the motes of work in hand rise through it, and nothing else on the
  range picker moves.

  **The width it is measured at is the width it grows from.** Which way a
  list grows is the placement's to decide, and it decides from the width
  it measures, before the stylesheet has run its rules. A list told to be
  the trigger's width and then widened to fit its longest name is measured
  first and widened after, so the placement hangs the trigger's width on
  the right edge and every pixel the list then gains goes out the other
  way, over the edge every field in that column ends on. `max-content` is
  the width before anything is measured, so the placement measures what it
  will get.

  A name longer than the app has room for is the one case a list cannot
  grow into, and the whole of a name is in the row's title either way. The row under the pointer and the row the
  keyboard is on are the same row, in `--ink-3`, and what is chosen is in
  the accent with a tick that keeps its place whether or not it is there.
  The trigger keeps room for the longest thing the list can say, so a row
  never changes width as it is used. It is built on
  [bits-ui](https://bits-ui.com), the headless half of shadcn-svelte, which
  brings the keyboard, the roles, the focus, the typeahead and the floating
  placement, and no look at all.
- **Correcting a word:** click it in the caption box, in the picture, where
  a short will show it. A word under the pointer is framed in the accent and
  the pointer becomes a caret, so what can be corrected says so without a
  word of instruction. The caret lands where the hand clicked, letters are
  typed and taken out from there, Enter saves and Escape leaves the word as
  it was. Clicking a word stops the picture, because a caption that moved on
  under the caret would leave the hand correcting a word that is no longer
  there. Playing the other way round ends the correction: a click on the
  video preview or anything else that plays saves the word the way Enter
  does, takes the caret and any highlighted letters out of it, and lets the
  captions follow the picture again. A click on the video preview lands on
  the crop frame, which refuses the pointer so it can be dragged, so the
  word used to keep the caret while the video played. The frame and the caret belong to the interface and not to the
  render: nothing of them is ever burned into a short, and **the frame is
  all the interface adds**. A word being corrected takes no background of
  its own. One would read as a second highlight pill, in the app's colour,
  on a word nobody is speaking, and on the word that is being spoken it
  would take away the colour the render really burns in. The picture goes
  on showing what the render will show while a word in it is being typed.
  - **From the keyboard:** Shift and the arrows walk the playhead from
    word to word, and the word walked to wears the same frame the pointer
    puts on a word. Enter opens that word with the whole of it selected,
    the way Enter renames what is chosen in the Finder, so typing replaces
    it. Enter again saves it, the caret goes, and the frame stays on the
    word. Escape in an open word leaves it as it was and goes back to the
    frame. Enter opens the word in the frame, wherever the playhead is: a
    word clicked and let go of keeps the frame and leaves the playhead
    where it was, and Enter used to open the word at the playhead, a word
    with no frame on it. The frame is the caption holding the keyboard: anything else
    takes it away, a press of the pointer wherever it lands, a field
    getting the keyboard, Escape, any other key, the arrows without Shift
    among them, and the clip playing. It is the same with the highlight
    off, and that is what it is for there: with no pill nothing else says
    which word is spoken.
    The caret went on blinking in a word already saved, because WebKit
    keeps the selection in a field the keyboard has left, so a word let go
    of takes its selection with it.
    A walk past the last word of a clip lands on the first word of the
    next, and past the first word on the last of the one before, and that
    word wears the frame too. The walk could not catch that landing by
    itself, because the clip changes first with no words in it, and the
    playhead only reaches the word once the clip's captions have arrived,
    so the frame was lost on the way. The workspace now tells the video
    preview where it landed, see `keyAt` in `Player.svelte`.
  - **A colour picked with the pointer lets go of the keyboard**, so Shift
    and the arrows walk the words again straight after. Left with Escape,
    the keyboard goes back to the colour, the way a list gives it back.
  - **A button pressed with the pointer keeps no keyboard**, the way a
    button on the Mac does not, see `letButtonGo` in `App.svelte`. In
    Chromium, on Windows and in the preview, a clicked button kept it, and
    Enter then pressed the button again instead of opening a word: a click
    on Highlight and then Enter switched the highlight back on.
  - The correction applies to every clip with that word, because it belongs
    to the episode and not to the clip. It is kept in
    `<episode>.framefairy/logs/corrections.json` and applied every time the
    transcript is read.
  - **A correction may hold more than one word.** Where the recogniser heard
    one word and two were said, writing both splits the word it measured
    between them, so the captions break and highlight them one by one.
    Writing one word again makes it one word again. Each of the words is a
    field of its own in the caption box: "weil" made "weil es", a click on
    "es" opens "es", and delete on it removes "es" and leaves "weil". The
    engine says which of the words each is, `part` in the captions. Before
    it did, the field went on reading "weil es" with "es" drawn after it as
    well, a click on either opened both, and removing either removed both.
    A word the captions hyphenate is drawn in two halves, and clicking
    either one hands back the whole of it to correct, with the other half
    out of the way while it is being typed.
  - **Removing a word:** delete, the key marked delete on the Mac and the
    forward delete key alike, removes the word in the frame, and a word
    typed empty and saved is removed the same way. The word leaves the
    caption box with the key, before the engine has answered, and comes
    back if the engine refuses. A removal is a correction to nothing,
    kept in `corrections.json` like any other, so the word is gone from
    the transcript, the captions and the render, and Cmd+Z brings it
    back. A word removed then corrected again comes back with the new
    text. Delete in a word open for typing is the field's own and takes
    out letters. The caption the word stood in stays as it was, with the
    word gone from it: the captions are laid out as if the word were still
    there, reading what it read when it was removed, and then it is left
    out, see `Captions` in `engine/lines.go`. What it read is kept with the
    removal in `corrections.json`, behind a mark no typed word can have. The
    time it held is no pause, the room it took on its line stays taken, so
    no word of the next caption is pulled up into it, and a caption whose
    first word it was appears when that word is said. A caption whose
    words are all removed goes, and the one before it stays up through
    its time. Before, the gap it left
    read as a pause, so the caption ended at the word before it, nothing
    was on screen for a moment, and the words after it went on to a
    caption of their own. Removing a word lost its way when the words
    became one list: the engine refused an empty word and the caption box
    put the old one back.
  - **Putting a word back:** Cmd+Z, or type it into the word beside it,
    "das" made "das ein". Both end the same way: the word is back where
    it was heard, with its own time, lit while it is said, and no
    correction is left on either word. The engine sees a word typed
    before or after what its neighbour read, beside a removed word, as
    that word coming back, `putBack` in `engine/corrections.go`. Each
    removed word there takes one, the nearest first, and a word that
    reads as the recogniser heard it keeps no correction. Typed back
    otherwise, "das eine", it comes back corrected. It used to take its
    time from the neighbour it was typed into, so it was lit while the
    neighbour was said and not while it was.
  - **The captions run on the clip's clock and a correction belongs to the
    episode.** A clip's clock has its cuts taken out of it, so the two
    clocks run apart by however much the cuts hold. The middle of the
    caption word is read back through the clip's pieces to find the word of
    the episode it stands for, and the middle rather than the edge because
    both halves of a split word have to point at the one word they came
    from. `inEpisode` and `saidWord` in `frontend/src/lib/flow.ts` are the
    way back, with tests.
- **Moving the captions:** drag the caption box up or down by its handle,
  a ring around the box that draws itself in the accent the moment the
  pointer comes near and is invisible the rest of the time. The whole of
  the box that is not a word takes hold of it, the ring reaches a little
  past its edges so there is somewhere to take hold even where a word runs
  to the end of a line, and the words themselves stay one click from being
  corrected. Its pointer is the up and down one, not the hand: the crop
  frame it sits inside is moved sideways and wears the hand, and two
  things that move in different directions should not say the same thing
  about themselves. The handle is there because the box stopped being
  grabbable the day the words in it became fields: what was left was the
  padding and the spaces between words, a few pixels of a preview, with
  nothing to say where they were. It lands on a
  step of a grid that appears while it moves, and it stays inside the frame.
  **There is one place for every clip of every episode**, because a place
  that suits one video suits the next one, so the drag saves it as a setting
  rather than as an edit of that clip. It stands as **Height** with the other
  caption settings on the left, where it can be typed instead, and **Put the
  captions back** appears under it once it is not the standard 300. A click
  on the box without dragging plays or pauses.
- **Moving the crop:** drag the frame sideways when the automatic crop is
  off. The frame is there while the playhead is in the clip, and only
  then. The new place applies to every piece of the clip filmed from the same
  camera angle. **Automatic crop** appears once a crop was moved and brings
  back the automatic placement for that angle, and **Put the crop back**
  takes that click back again. A click on the frame without dragging plays
  or pauses.
- **Clip list:** beside the player, exactly as tall as the video preview with
  the range picker under it, every clip found, in time order, with its start
  and length. A green dot marks a rendered clip. Click one to select it,
  which also puts the clip timeline back on it.
    - Put back on the clip, the clip timeline centres it under the video
      preview and the range picker, not under the middle of the app. The
      clip list is wider than the settings once the app is narrow, so the
      middle column stands a little left of the middle, and a clip centred
      on the whole track stood up to 38 px right of the picture above it.
      Where the middle column lies is read when the clip is fitted, from
      `.middle`, never kept as a size.
    - The clips lie on a surface of their own, the grey of every row in
      the app, parted from each other by lighter lines than the ones that
      part one area of the workspace from another.
    - **A new episode finds its first clips by itself.** Adding a video is
      all it takes: the Go side asks for its first search, of the first
      of the equal windows the episode is cut into, and no longer than the
      model can read at once, and the range picker goes to that window.
      **The windows grow with the episode**, as the square root of its
      length, so a short episode shows its first clips soon and a long one
      is not cut into dozens: half an hour makes three windows of 10
      minutes, an hour four of 15, two hours six of 20 and four hours
      eight of 30. None is shorter than 10 minutes, so a video of 15
      minutes is one window, and the windows are equal, so no scrap is
      left at the end. When the window moves on by itself, after a search,
      it takes the next one. See `engine/suggest.go`.
      The search hears the episode to the end of the window and then
      finds. Until it finds, the first row of the clip list says Waiting
      for the transcript, and the window is locked for as long as the
      search runs. **Cancel** calls it off, and what was heard stays. The
      local model is loaded while the episode is heard, so the search
      starts finding with it in memory. **While clips are found, no search
      hears**, on this episode or any other, so the model has the machine
      to itself: a search that hears when another comes to finding saves
      what it heard, waits its turn, and carries on after. **Hearing stops
      exactly at the end of the window**: the piece of audio the speech
      model hears is cut on the window's edge, so it hears nothing past
      it, and the search finds at once. A word that runs across the edge
      is not in the window, and is heard whole by the next search that
      needs it. The first search happens only for an episode nobody has
      ever searched, which is one with no plan and no `jobs/` folder, so
      removing every clip again does not bring a search of its own back:
      a search is the machine's time, and nobody asked for it. Deleting
      the work folder makes the episode new, and then it starts over as a
      new one does. The rule is `firstSearch` in
      `cmd/framefairy-app/search.go`, and the path tests follow it.
    - **The target follows the window.** Until it is changed, Target
      holds how many clips the next window suggests, as a number like any
      other: 6 for half an hour of clips of 20 to 30 seconds,
      one for every twelve clip lengths, and other windows in proportion
      to the square root of their length, 3 for six or ten minutes, 8 for
      an hour. A number typed there is for the window it was typed for, and
      kept with that window's length. A window of another length, drawn
      anew or the first window of the next episode, follows its own
      suggestion again, and so does the first search the app starts by
      itself when a video is added. Three typed for the six minutes of one
      episode went on asking for three in the half hour of the next.
      Cleared, the field follows the window again and shows its
      suggestion. The suggestion used to be a grey placeholder in an empty
      field, and a browser steps an empty number field from zero, so the
      down arrow on a suggested 6 landed on 1. Twelve
      because the model, asked for 8 in half an hour, gave 6: it gives
      fewer when fewer moments are strong enough, and asking for more only
      asks it to fill places with weaker ones. The square root because in
      proportion to the length, six minutes were asked for one clip, and a
      short window is still worth a choice. The local model thinks in
      proportion to the window, see [ENGINE.md](ENGINE.md), so a smaller
      window is also found sooner.
    - **What a search is doing is in the row its next clip appears in.**
      The first of the rows still to come wears the beam and the fill and
      says two things: Waiting for the transcript with the window it is
      waiting to cover, then Finding clips, then how many are found, with
      about how long is left under it. Loading the model, reading and
      thinking are the machine's steps and are not spelled out. A headline
      stays at least two and a half seconds and a count of clips at least
      one, so it can be read, while the fill and the time left move at
      once. The time left moves in steps of five seconds. Only the search
      reports on that row: what ffmpeg says while it frames a clip goes to
      the log. **New** becomes **Cancel** the moment a first search is on
      its way, while it still waits for the transcript, and there it calls
      the search off before it starts. While a search runs, Cancel stops
      it. A render is not a search, so it leaves this head alone:
      **New** stays New and cannot be pressed until the render is done,
      because one lane does the work. The render shows in the **Render**
      button it was started from, which fills from the moment it is
      pressed and says **Cancel**, and a click on it stops the render.
    - **How far a search is, is measured.** The fill is the share of the
      search that is done, measured against how long the same parts took
      the last time on this machine. Inside a part, what the model counts
      itself beats the clock: the transcript it has read, what it has
      thought against its budget, the clips it has written. Asking a local
      model again about the clips well off the length is a part of its
      own, so the fill is not full while the cards still say *Fitting to
      the length*. A local model
      this machine has never timed is measured against a search timed on
      an M2 Max until its own first search has finished. A model in the
      cloud the app knows by name is measured against a first guess the
      same way, and one written in by hand shows the beam without a fill
      until it has been timed. Anthropic's models that think by
      themselves are asked for a summary of their thought as it goes, so
      the fill moves on while they think rather than waiting in silence.
      The engine keeps the timings, see [ENGINE.md](ENGINE.md).
    - **Clips arrive one at a time.** The engine writes each clip to the
      plan the moment it is framed, while the model is still writing the
      next, so the list fills in a row at a time and the rows still to come
      shrink as it does. The first clip found is put on screen as soon as
      it lands, unless another clip was picked after the search began: a
      clip taken away from the hand that is working on it is worse than
      one shown a little later. When the search is over, the earliest of
      the clips it found, the first of them in the list, is the one chosen,
      with the playhead at its start, as long as the clip chosen is still
      the one the search chose and the video is paused. The model names its
      clips strongest first, so the first to land is often not the first
      in the list. A clip that has landed can be played,
      trimmed and corrected while the rest are still coming. Stopping a
      search keeps the clips it had found.
    - **A clip on its way is a card in its place.** A clip the model has
      named is in the list, among the clips of the episode where it lies,
      from the moment it is named until it is written: the card of a clip
      with the beam round it, its title and what is being done to it,
      "Placing the crop" or "Fitting to the length". It hands
      over to the clip itself in one step, holding its place until the
      list has read the clip, so there is never a gap and never two cards.
      A clip made with I or O comes in the same way, see below, because
      every job says which clips it has on the way the same way, see
      [JOBS.md](JOBS.md).
    - **The search's fill goes on to the end.** The row still to come
      wears how far the search has come and how long it has left. Once
      every clip is named, no row is left to wear it, so one of the
      search's cards takes it on, with the fill and the time left, until
      the search is done. The fill went with the row before, at sixty per
      cent, and the search went on with nothing to say how far it was.
      Then every card of the search wore it, the same fill and time left
      three times over.
    - **A search is one fill, and it moves down its cards.** Every card
      on its way wears the beam, because each is still being worked on,
      but a search is one piece of work, so only one of its cards wears
      the fill: the first of its cards in the list. It keeps it until its
      own clip is written, even when a card is named above it, and then
      the first card left takes it on, so the fill starts at the first
      card of the batch and ends at the last. While no card of the search
      is on its way, the row still to come wears it, and once one is, that
      row wears the beam alone. The last card in the list wore it before,
      so the fill stood at the foot of the batch while the cards above it
      landed.
    - **The rows still to come stand where the window is.** Everything a
      search finds lies in its window, so its rows stand in the list where
      the window ends: after the clips before the window and before the
      clips after it. A window drawn between the second clip and the third
      opens its rows between the two, and each card that comes takes a
      row's place, so the clips after the window move down once, when the
      search starts, and not again.
    - **A card stays where it came in.** A search stands as many rows as
      it was asked for, from the start: the first says what it is doing,
      the others breathe. Each clip it names comes into the row that says
      so, with the fill it was wearing, and that row's words move on into
      the next row waiting. A card once there stays in its row until the
      search is over, the list keeps its height the whole time, and no row
      slides. Then the clips go to the order they are spoken in, the order
      of the range picker and the clip timeline, in one step. Each card
      used to come in at its moment in the episode, above the rows waiting
      or between cards already there, so every row under it slid down one
      and the last row waiting closed, each time a clip was named. Tim saw
      cards being replaced rather than filled in, and a stack that never
      stood still.
    - **A search starts in sight.** The rows still to come are often after
      the clips there are, so in a long list a search began out of sight and
      all anyone saw was New turning into Cancel. The row the next clip
      will appear in, the one wearing the work, is brought to the top of
      the column the moment it is there, and back into view every time a
      clip lands, wherever the list has been scrolled to in between. It
      comes back with a third of the row after it showing, so it is plain
      there is more to come, and the clip that just landed stays in view
      above it. Only when a clip lands: following every report would fight
      a hand that is scrolling. The last clip a search finds leaves no row
      after it, so that clip itself is brought into view, to the foot of
      the list when it lands below. The model writes its clips in an order
      of its own, and the list is in the order they were spoken, so the
      last clip found is not always the last in the list: it is the one
      that comes into view, wherever it lands, and the rows still to come
      stay after the last of them. Once the search is over the list is in
      the order the clips were spoken, which is the order of the range
      picker and the clip timeline.
    - **What a search was asked for is held while it runs.** Target,
      Shortest and Longest go to the model with the prompt, so they are
      locked from New until the search has run, and dimmed. Changing Target
      used to change the rows still to come while the model looked for the
      number it had been given.
    - **An empty list is never empty.** With nothing in the list and no
      search on its way, the rows New will fill stand there as many as
      Target says, following it as it changes, and still, because nothing
      is filling them yet. When every clip there was has been removed, the
      first of them says so, **All clips removed**, **New finds others**,
      so the empty rows read as something that happened. It gives no
      number, because the plan keeps every clip ever removed, and a count
      of them was more than the list had just shown. The empty rows open
      only once the last removed row has closed, so they never push it
      down the column as it goes. An episode whose first search was stopped, by
      quitting among other things, had a bare column there before.
    - **A search that stopped before it was done says so** in the row its
      next clip would have appeared in, in the colour of a warning, and
      **New** becomes **Continue**. *Interrupted. Click Continue* is a
      search the app was closed or fell over in, with the window it was
      about under it, or with how far the transcript came when it still
      waited for the transcript, which is the first half of every search on
      an episode read only part way. *Failed. Click Continue* has its
      reason under it. The whole of it is in the row's title. **Continue**
      takes the window back to the one the search was about and asks for
      it again: a search cut off while it heard hears on from where it
      stopped and then finds. Nothing starts by itself. It stays until it
      is acted on, after a restart too, because the search keeps its record
      in `jobs/search.json` in the work folder, see [JOBS.md](JOBS.md), and
      the app reads the records as it starts: one in a running step is a
      search that was cut off, a failed one says why. **Cancel** says
      *Stopping* in that row the moment it is pressed, standing still, and
      then leaves the same row, *Stopped. Click Continue*, with how far the episode was
      transcribed, because what it did stays and can be carried on. **New**
      takes its place. Clips it wrote before it stopped stay, with the row
      after them. The episode's dot in the sidebar says it too: the colour
      of a warning for a search stopped or cut off, of an error for one
      that failed.
    - **New**, above the list, finds clips in the window, as long as the
      episode's windows are, and says so while it looks. The clips it
      finds join the ones already there. After a search the window walks
      on from where the search ended, as long as it was left, so a window
      made a minute long stays a minute long, and at the end of the
      episode it starts over at the start. The last window may be cut
      short by the end of the episode, and the one after it, at the
      start, is as long as the window was made again.
      The window is kept with the episode the moment it changes, beside
      the clip last worked on in `chosen.json`, so the workspace opens on
      it again, after a restart too. An episode that has none opens on the
      window where the fewest searches have been, earliest first, as long
      as the episode's windows are, and a double-click on the window's
      marks puts it back there. A short
      episode is one window and is searched whole each time. A window searched again keeps every
      clip there is: the model is told which lines are clips already and
      asked for other moments, and a search that brings none ends with
      nothing added. New is never off for want of room, and its title
      says whether it looks somewhere new or again.
    - The trash can on a row removes that clip. Its mark leaves the track at
      once, and its row stays in place for ten seconds, in red with a trash
      can, saying **Removed** and offering **Put it back**. The time left is
      the app's one fill, in red, running down to nothing over those ten
      seconds, with the light over it running the same way, right to
      left, see `Busy.svelte`, and it stands still while the pointer or
      the keyboard is on the row, so the time to change your mind is seen
      and nobody is rushed. Nothing above or
      below it moves while it is there, and when it goes the list closes
      over it. Every clip removed has its own row and its own ten seconds:
      removing another, or putting one back, leaves the others as they
      are. There was one for the whole list, and a second clip removed took
      the first one's way back with it. The ten seconds are the episode's,
      not the screen's: they run on while another episode or page is open,
      and coming back within them finds the row again with the time it has
      left. Time the pointer held it still is kept too. A list read again,
      after a search, a render or an undo, leaves the rows as they are, and
      an undo that puts the clip back makes it a clip again. Leaving the
      episode, or any of those, used to take every way back with it. The
      clip stays in the plan with everything done to it, and a render of
      the whole plan leaves it out.
- **Range picker:** one slim strip for the whole episode, under the video
  preview and exactly as wide as it. It shows where the clips are and
  where the playhead stands, and while clips are being found, the part
  being searched.
    - **At rest the window is marked at its corners, not drawn.** A bar
      runs just outside the top border and one just outside the bottom
      border, over the window's width, and a triangle hangs from each end
      of them with its tip reaching into the track. Nothing else of it
      lies over the track, so it hides no clip and does not read as a
      window. It says where New looks next, which the app decides, see
      New above. The four triangles are the handles: the left two drag
      where the window starts and the right two where it ends. A bar
      drags the whole window and keeps its length, and a double-click on
      any of it puts it back where the app would have it. An edge lands
      on a round step, the smallest one still about eight pixels wide,
      five minutes on a four hour episode and five seconds on a six
      minute one, and the ends of the episode win over the step. Every
      window the app places itself lands on the same step, its start and
      its length, so the app's own half hour of a four hour episode is
      30:00 rather than an even eighth of it, 30:02, which drifted a few
      seconds off the ruler's lines with every search. The last window of
      the episode takes in a scrap shorter than a step rather than leaving
      it behind. A handle
      stops where the window would hold too few clips, room for the
      clips typed into Target at the Shortest length or for one, or more
      than the model reads at once, and the marks flash twice in the
      colour of a warning as it runs in, so a hand that keeps pulling
      knows it is the limit and not the app that stopped. The triangles'
      titles say both limits in words. Under the pointer the whole mark lights up, a bar shows
      the hand that grabs and a triangle the arrows left and right. The
      triangles lie over the playhead, which a search leaves on the
      window's start, and the bars under it. The title of New and the
      suggested Target follow the hand. While its clips are found, or
      while its search stands stopped with Continue, the mark turns into
      the window, drawn over the track, and back into the mark when the
      search is done, on the window after it. Only one of the two is ever
      drawn. Tim found a window at rest a leftover that got in the way:
      after a search of the whole of a short episode it lay over all of
      it and hid every clip just found, and an outline with handles read
      as the same window. A clip is removed from its row in the list.
    - While clips are being found for it, the window breathes and the
      motes of work in hand, the ones every control sheds, rise through
      it, which is the track saying work is in hand. Nothing else on the
      range picker moves: how far the search has come, the transcribing
      and the finding, is on its cards in the clip list. A press on the
      window is a press on the track.
    - A mark for every clip runs across the middle of the track, green once
      rendered, and the chosen clip is a frame round the wash, the way the
      clip timeline draws it. Click a mark to select that clip.
    - A press anywhere else takes hold of the playhead, see above.
- **Nothing sits under the range picker.** The line that parts the workspace
  from the clip up close runs right below it, and the workspace is exactly as
  tall as the video preview and the range picker need. If the transcript has
  not heard all of the window yet, a search has what is missing heard first
  and then finds its clips by itself.
- **One mark explains one thing, where that thing is.** The video preview,
  the range picker and the clip timeline each carry their own info mark in
  their top right corner, and a mark only appears while the pointer is on
  the area it belongs to. The same goes for the marks beside **New clips**,
  **Captions** and the head of the clip list. No mark explains two areas,
  and nothing floats at the end of a row of its own.
- **One row under the clip up close**, so the range picker and the waveform
  stand together with nothing between them: play, loop and the crosshair
  that goes to the playhead, as the three marks anyone knows, then the
  title of the selected clip, why it was chosen and its numbers, and on the
  right what a reset threw away,
  **Render** and, once rendered, **Show in folder**. The time is not written
  anywhere else: the playhead says it on both timelines. Above that row, the
  clip up close:
    - The arrow keys step the playhead one frame of the episode, and one
      word with shift, wherever the keyboard is as long as it is not in a
      field or on an edge of the clip. **A word, because a word is what the
      picture is showing:** the caption lights up the word being spoken, so
      shift and an arrow walk that light one word on. A second was what
      they took before, and a second is nothing in particular. It lands in
      the middle of a word as often as not, it walks four words at a time
      where someone speaks quickly and none at all across a pause.
      - **It walks the words the caption lights up, not the words the
        transcript holds.** They are not the same list. A correction that
        reads as two words is two words in the caption and one in the
        transcript, so a word added by hand stood in no list the timeline
        had and the keys stepped straight past it. A word a cut takes out
        is the other way round, in the transcript and never in the caption,
        and landing on one lit nothing. And a plan keeps the moments it was
        made with, so a transcript read again since can put the same word a
        quarter of a second elsewhere. Reading the caption makes all three
        right at once, and keeps them right, because whatever lights up is
        what these keys walk.
      - **Which word to go to is decided by the word the playhead is in,
        never by how far it is from one.** The playhead is not where it was
        put: the picture answers with the frame it is showing, which can be
        later than where the playhead was sent. Measuring a distance then
        found the word the playhead was already on and sent it to the same
        place again, so the key did nothing at all and nothing but the
        mouse got out of it.
      - The playhead lands a frame inside a word, never on its edge. A
        boundary is exactly where the question "is this word being spoken"
        has no steady answer: the caption runs on the clip's clock and the
        playhead on the episode's, and the frame the picture settles on is
        a third answer again. On the edge, two words in every five lit
        nothing at all.
      - **Past the last word of a clip they carry on into the one beside
        it**, the left arrow from the first word of a clip landing on the
        last word of the one before it and the right arrow the other way
        round. A clip's words arrive with its captions, so the clip is
        chosen first and the landing waits for them.
      - Outside a clip there is no caption, so the words that were heard
        are the only ones there are. Where nothing has been heard yet
        shift takes a second.
      - **Which list is walked is decided a whole frame either side of the
        clip**, for the same reason the crop frame is. Read exactly, a
        playhead just put at a clip's start is outside it, so the keys
        walked the transcript instead and the first press after picking a
        clip landed wherever the word before the clip happened to be.
    - Drag an edge to trim. The edge lands on the frame, the same as the
      edge of a cut, and shift puts it on the nearest word instead, the way
      the render cuts a clip the engine proposes. Shift and not alt,
      because it is the key Tim reached for first. It used to snap to words
      always, which left no way to take a breath off the end of a clip or
      keep the first frame of a gesture before the first word.
    - **A double-click on an edge puts it back where the clip was found**,
      the way a double-click on a caption's edge puts that back where its
      words put it. Tim asked for the two to be the same. The clip keeps
      where its edges were in the plan, `found`, written by `editPieces` in
      `engine/edit.go` the first time anything changes its pieces and never
      after, so a clip never changed needs none and is where it was found.
      A clip changed before this was written keeps the edges it had at its
      first change after it. The edge's title says when it was moved by
      hand, and the double-click is a trim to the edge it was found with, so
      Undo takes it back like any other trim. It lands there exactly, not on
      the frame nearest: a search finds a clip on the episode's clock, so
      its edges are seldom on a frame, and the end went back to 24.80 where
      the clip had ended at 24.72. A trim within half a frame of where the
      clip was found lands there, see `onFound` in `engine/shape.go`. A
      shot the trim had taken away comes back with its own crop, from the
      camera switch, the same as a cut put back, see
      [ENGINE.md](ENGINE.md).
    - **The playhead goes with the edge.** While either edge of a clip is
      dragged, the playhead stands on it and the video preview shows that
      frame, the clip as it is being dragged, with its captions. On frames
      it is the clip's first frame, or its last, a frame before the end.
      With shift it is inside the word the edge snapped to, a frame into
      the first word or a frame before the end of the last, so that word is
      always the one lit. The edge itself stands a pause away from the
      word, and the playhead used to go to the clip's start after a trim,
      so the word was lit only when the pause happened to be none, which
      looked random. After letting go the playhead stays where it was.
    - **Shift stops at the words that light up.** An edge dragged with
      shift lands on the words the way the clip's captions split them, so
      the halves of a hyphenated word, or a correction that reads as two
      words, are two stops, the same words the arrow keys walk.
    - **Shift stops where a caption goes, too.** A caption stays up a
      little after its last word, and its block on the clip timeline is
      that long. The end of a clip and the left edge of a cut dragged with
      shift stop at the end of the block before they go on to the word,
      when it lies in the pause after it. They used to know only the word,
      so the edge went past the end of the block into it and the block
      shrank under it, and the block looked longer than the words it
      stood for.
    - **The engine answers for every drag.** The timeline sends what the
      hand is doing, `Shape`, and draws what comes back: the pieces, the
      captions and where the playhead goes. Letting go saves the same
      gesture, `Reshape`, so what was drawn is what is saved. The timeline
      keeps no rules about where an edge lands, see
      [WORDS.md](WORDS.md).
    - **The clip edge is over the caption handles.** The first caption is
      on screen from the clip's first frame, so its handle stood on the
      clip's start edge, and a hand that reached for the clip in the band
      of the captions moved the caption instead.
    - **The captions follow the drag.** While an edge of the clip or of a
      cut is dragged, the caption blocks are drawn for the clip as the hand
      has it: a word the edge reaches gets its caption under the hand, and
      a word it leaves loses it. The engine makes them with the answer to
      the gesture, and saves nothing until the hand lets go. They used to wait for the hand to let go, so a drag showed
      the old captions over the new clip. After letting go they stay until
      the saved clip's captions come back, which are the same, so nothing
      jumps.
    - Click an edge to put the playhead exactly on it, which is how a clip
      is started over.
    - **Nothing on the playhead but the playhead.** It carried a magnifier
      that opened a pill of the words around it, which was where a word was
      corrected. Words are corrected in the caption box over the picture
      now, where a short will show them, and a magnifier that could only
      read was a second place to look at the same words. So it came out,
      and the track is the waveform and the playhead and nothing else.
    - **The clip is one thing, holes and all.** One frame in the accent,
      with its corners just rounded, runs round it from its first piece to
      its last, whatever is cut out in between, so a clip with a cut in it
      reads as one clip and not as two standing in a row. It is drawn over
      the time lines, the cuts and the captions, so nothing crosses it. An
      edge of it widens under the pointer and while it is dragged. The accent wash inside the rules says
      which parts are kept. A part the clip leaves out, usually dead
      air the engine found, is the track's own background with an accent
      line at each end, the way an editor marks the place two shots were
      joined: what is not in the clip looks like everything else that is
      not in the clip. Playing jumps it, and the render does too.
    - **The cuts can be changed.** Each one carries a handle on either edge,
      in the accent's lighter shade so it is not taken for the clip's own
      edge. Dragging a handle moves that edge of the cut, and a double-click
      on the block or on either handle removes the cut, so the part plays
      again.
    - **A double-click in the clip cuts a part out** where it lands, forty
      pixels wide, which is wide enough to see and to take hold of by either
      edge and drag to size. Outside the clip a double-click does nothing,
      and nothing else on the track makes a cut. Shift and a drag used to
      draw one, and a double-click in the place a cut had just been removed
      took it out again: Tim found neither, and the second made a double-
      click in the clip cut something only sometimes. Shift means words and
      clips everywhere else in the app, so a drag with it held moves the
      playhead like any other drag.
      Forty pixels and not a quarter of a second, because what has to stay
      the same is what the hand sees: the timeline goes from a whole four
      hour episode down to a second across, and a width in seconds is
      wrong at both ends of a range that wide. Measured at the zoom the
      timeline opens at and again with a second across the track, a quarter
      of a second came out as 9 pixels and then as 187, narrower than one
      of the two 12 pixel edge handles and then a quarter of the track.
      Forty pixels comes out as 40 and 47, the 47 being the frame rounding:
      a cut is a whole number of frames wide, rounded up, so it is never
      too short for the engine to take. The view never moves: a gesture
      that cuts and zooms at the same time is a gesture nobody can aim.
    - **Moving one edge never moves the other**, of a cut or of the clip.
      A drag of a cut's edge says which edge it moves, `Edge` "from" or
      "to" in a `move` gesture, and the engine puts only that edge on a
      word with shift, and stops it at the other rather than pushing the
      other along. Shift on the right edge of a cut used to put the left
      one on a word as well. A click on an edge of a cut that does not
      drag puts the playhead on it, the way a click on an edge of the clip
      does.
    - **A caption that a cut takes the first word of** appears where the
      cut ends, when that word ran straight on into the rest of it. It
      used to wait for its next word, so a cut's right edge moved a frame
      further into a short first word made the caption jump past the
      pause after it, though it had been on screen there a moment before.
    - **A caption is drawn only where the clip is.** Its block lies over
      the parts the clip keeps and never over a cut, in as many pieces as
      it takes. Where a caption goes at a cut, its end is the end of the
      piece before, `endInEpisode` in `frontend/src/lib/flow.ts`: a moment
      at a cut is both the end of the piece before and the start of the
      one after, and an end taken as the start of the next piece drew the
      caption across the whole cut. Tim saw it moving the left edge of a
      cut into a caption, and with shift on a cut's edge, where the next
      caption begins right where the cut ends. A block has no padding, so
      one a few pixels long is a few pixels long and does not reach into
      a cut.
    - **A double-click is told from the two presses**, not from the
      browser's dblclick. The first press moves the playhead under the
      pointer, so the second lands on the playhead's line or on a cut's
      handle, and a dblclick goes to whatever the second landed on. Over
      the line it went nowhere, which is why a double-click in the clip did
      nothing while one on a cut's handle did. Two presses within half a
      second and six pixels of each other are a double-click wherever the
      second lands.
    - **A cut lands on the frame.** The edges stay where the hand put them,
      rounded to a whole frame of the episode and no further, because a
      double-click and a drag both say where exactly and moving the edges
      somewhere else is not what was asked. A cut made this way may stop
      inside a word, which is the point of it.
    - **Shift lands on whole words instead.** Holding shift while dragging a
      handle of a cut puts the edges where the render would cut them, so a
      cut dragged over a pause takes the whole pause and a cut dragged over
      speech takes whole words, and it can never stop half way through a
      word. That is the right thing when a whole phrase is to go and the
      wrong thing when a breath is: a drag of a few pixels in a silence came
      out as the whole silence, which is why this is the modifier and not
      the default. The key is read while the hand moves rather than when it
      goes down, so letting go of it part way through goes back to frames
      and the block says so before the drag ends.
    - Nothing is written over the waveform. The captions are in the video
      preview as they are spoken, and that is the one place they are
      written out, so the waveform has the whole track to itself.
    - The waveform is drawn the way an editor draws one: one column of the
      screen per column of the picture, each a whole pixel wide. Nothing is
      ever drawn between two pixels, so it keeps the same weight at every
      zoom instead of brightening and dimming as it is pinched.
    - **Zoomed out a column is the loudest reading in it**, because a peak
      that was averaged away is a peak nobody can see. **Zoomed in past the
      measurement the outline runs between the readings.** Loudness is
      read every hundredth of a second and no finer, so a second of it on
      a retina screen is a hundred readings across eight hundred pixels:
      squared off, that is eight pixels of one height, a cliff, and eight
      more. The engine says how fine its measurement was by never
      answering with more parts than it read, and the timeline joins them
      rather than squaring them off. It invents no detail, it stops
      pretending each hundredth of a second was flat.
    - The times sit at the top of both tracks, in the same quiet grey, a
      step above the lines around them and below the waveform inside them.
    - Drag the playhead anywhere on the track and the video preview follows.
    - Time labels along the top say where in the episode the timeline is,
      which is what a swipe needs in order to mean anything. They are at the
      top and in the same colour as the ones on the range picker: two tracks
      that both say where you are say it in the same place. A ruler is two
      layers and has to be: the line goes behind what is drawn on the
      track, so a minute falling on the edge of the window on the range
      picker never paints over its border, and the time goes in front of
      everything, because a time nothing can read is worse than no time at
      all. The time is not written inside its line, because an element with
      a z-index makes a stacking context and would hold its own time down
      there with it.
    - **Two fingers move along the episode**, the way an editing timeline
      works: a swipe left or right travels through the episode, and a pinch
      zooms around the pointer, down to two seconds across the track and out
      to the whole episode. **Both ends are walls.** A pinch that can go no
      closer does nothing at all, rather than carrying on and sliding the
      view sideways. A view moved by hand stays where it was put,
      wherever the playhead goes. The crosshair in the row under the track
      goes to the playhead and puts it in the middle. Clicking a clip in
      the list, the one that is already selected included, puts that clip
      back in view. A double-click on the track never did: one was meant
      to, and it went nowhere for the reason above. Its info bubble says
      only what the track itself does, a line each, and leaves the row
      under it to its own titles. The words and the waveform of the whole episode
      are read once and kept, so swiping does not wait for a file to be
      read again.
    - **Every clip is on it, not only the chosen one.** The other clips
      are the same marks the range picker draws, across the middle of the
      track where the captions run, in the app's colour and green once
      rendered, one style in `app.css` for both tracks. Each is the same
      share of its track's height, 16%, so the mark on the range picker is
      half as tall, as the range picker is, in solid colour with a soft
      shadow on all four sides. The waveform's grey and the app's colour are
      about as bright as each other, and such an edge seems to shimmer. The
      shadow gives it a step in brightness. A click on one chooses it. The
      chosen clip has no mark, because its frame already shows it. Clips
      may lie over each other, and on the clip timeline another clip's
      mark gives way inside the chosen clip's frame, where its captions
      are worked on. Drawn over them it hid them, and the caption being
      shown, in the app's colour, could not be told from a mark in the
      same colour. On the
      range picker the chosen clip is drawn the way it is up close, a
      frame round the wash, nearly three times as tall as the rest. Zoomed out, the
      track is the episode with all its clips, the way an editor's
      timeline shows every clip on it.
    - **Detail comes in as there is room for it.** The caption blocks are
      drawn only while the caption in the middle of the clip is at least
      as wide as a block is tall, 16 pixels. Narrower, a block's line has
      no room inside its padding and the blocks run into one smear over the
      waveform, so zoomed out there are the clips and the waveform and no
      captions, and zoomed in to the clip they come back.
- **The waveform comes first.** The moment an episode is added, the app
  measures its loudness on its own, without the speech model: the audio
  decoded and a reading every 10 ms, the same readings the transcription
  takes. Audio decodes at a few hundred times real time, so an hour is
  measured in seconds, and the waveform fills in as it goes, first where
  the clip timeline looks. Every time the clip timeline asks for the
  waveform it says what it shows, and the measuring goes there next: a
  playhead put near the end of a four hour episode has its waveform
  within a second rather than after everything before it. The clip
  timeline reads its view again while what it read had a gap in it and
  more has been measured. An episode added before this was there is measured the first time
  it is opened. It is not a job: nobody starts it or waits for it, so it
  has no row in Activity. At most two episodes are measured at a time.
- **Before the first transcription** there are no words. That is where
  every episode starts, so the captions band simply waits. The words
  appear as the transcript grows past them, without anything being
  clicked. Nothing about this is an error, and nothing about it is logged
  as one.
- **The timeline is always there.** With no clip selected it shows the
  minute around the playhead and follows it as the episode plays, so there
  is always something saying where you are. Trimming needs a clip, so it
  appears once one is selected, and so does the caption box a word is
  corrected in.

Every change is saved at once. There is no save button. A changed clip gets
new captions on its next render.

Two files that would share a work folder cannot both be in the library.
Everything about an episode lives in a folder named after it without its
extension, so `ep.mp4` and `ep.mov` side by side would share a transcript,
clip sets and rendered names. The second one is left out and the reason
says which two.

### The colour picker

The colours of the captions, **Text**, **Box** and **Highlight** in the
settings column, and the accent's colour wheel in the settings open one
colour picker, `Colour.svelte`. It is the app's own, drawn on bits-ui the
way `Pick.svelte` is, because the colour field it replaces handed the click
to macOS: the pop-up was the system's, looked like another program, could
not be styled, and never said which of its colours was the one in use.

In it, from the quickest pick to the most exact:

- **The colours a caption is most often made in**, round, the way the Mac
  shows a row of colours: white, black, yellow, orange, red, pink, the
  app's purple, blue and green, from `captionColours` in
  `frontend/src/lib/colour.ts`. The one in use wears a ring.
- **A square for the shade and a strip for the hue**, for any other colour.
  The video preview and the clip timeline follow the hand as it drags, and
  the colour is saved when it lets go. The arrows move them too, Shift in
  bigger steps.
- **The pipette**, beside the hex. It closes the pop-up and makes the video
  preview a place to point at: a loupe sits on the pointer the way the
  Mac's own colour sampler does, the pixels under it drawn ten times over
  and the one in its middle marked, with its hex under it. The captions are
  drawn in that colour as the pointer moves, a click takes it, and Escape
  or a click anywhere else leaves the colour as it was. What is read is the
  episode's own frame, not the screen, so the crop, its shade and the
  captions lying over the picture never colour what is taken. The colour
  being picked keeps its ring in the meantime, so it is clear which one
  the picture is giving. See `sampleColour` in `Player.svelte`. WebKit has
  no eyedropper of its own for a page, and it is not needed: the picture
  is the app's to read.
- **The hex**, for a colour known by its number, a brand colour above all,
  with or without the #. It is drawn as it is typed and saved with Enter or
  when it is left.

The opacity stays the number beside each colour, so the row keeps its
width.

While a colour was drawn on the way and the last one was still being
saved, the next colour drawn was thrown away the moment it was drawn: the
drawing was held against the captions from before the save, and nothing
watched the save end. The colour field drew again with every movement and
hid it. The pipette draws once per colour, and showed nothing until the
saving was watched.

### Thumbnails

A clip can have any number of thumbnails, or none. A thumbnail is a frame
of the short exactly as the short shows it, captions included, and
**Render** writes each one beside the short as `<name>-1.jpg`, `<name>-2.jpg`
and on. There is no way to render the pictures alone: a change to them is a
change to the clip, and the next render writes the whole of it again. The
reasoning is in [THUMBNAILS.md](THUMBNAILS.md).

- **I and O make a clip at the playhead**, the way In and Out mark a clip in
  every video editor, for a moment the model did not pick. **I** starts
  the clip with the sentence the playhead stands in, from where that
  sentence begins, and grows it forward. **O** ends it with that sentence,
  for a moment noticed only once it has passed, and grows it back. Either
  grows a line at a time until it is as long as **Shortest**, never past
  **Longest**, and then its far edge goes onto a sentence the way every
  clip's does. The two letters are buttons in the row under the clip
  timeline too. No model is asked: the pauses are cut and the crop placed
  exactly as for a clip a search found, and after that it is an ordinary
  clip.
  - **Everything shows in the frame the key is pressed in.** The **I** or
    **O** button wears the beam for as long as its clip is on its way.
    The clip's card is in the list at once, in its place in the episode,
    wearing the beam, and it is the chosen card, so it is brought into
    view wherever the list was scrolled to. Where the transcript does not
    reach far enough yet it says Transcribing and fills as the part
    around the playhead is heard, then Placing the crop, and fills again
    as the crop is placed, with the time left. It wore the beam alone
    while its crop was placed, which is most of its wait. Any number can
    be on their way at once, beside a search too, and none waits for a
    search to finish finding.
  - **Its frame is on the clip timeline at once**, fitted, first as long
    as **Shortest** from the playhead, or up to it for **O**, then on its
    sentences as soon as they are known, with its title over it. While
    its crop is placed its pieces are drawn, and its caption blocks come
    in one after another, laid out as the render will draw them. It
    cannot be edited until it is written.
  - **It is followed while it is made.** Its card is brought all the way
    into view once it has slid open and again whenever it moves in the
    list, and with the video paused the playhead goes to where its frame
    starts, then to where its sentences start.
  - **The card becomes the clip** when it lands, still chosen, unless
    another clip has been chosen since. While the video plays it stays
    where it is playing.
  - **New stays New.** It is the search's button, so a clip made by hand
    does not turn it into Cancel. Cancel while a search runs stops all
    the work on the clips, the search and every clip on its way, and
    Continue carries all of it on.
  - **It searched nothing**: the range picker marks nothing, and giving a
    searched part back leaves it where it is. It goes the way every clip
    goes, with its trash can.
  - **Cut off by the app closing, or failed**, its card stays where the
    clip would have appeared, still, and a click carries it on.
- **The thumbnail button**, in the row under the clip timeline, makes the
  frame under the playhead a thumbnail. **T** does the same. It is not a
  mode, so it never looks pressed. Its icon says what a click does: a
  picture with a plus adds one, and a picture with a minus, shown while
  the playhead stands on a thumbnail, removes it. It can only be pressed
  with the playhead inside the clip, because a frame that was cut is not in
  the short.
- **Each thumbnail is a mark** along the foot of the clip timeline, the
  same picture icon in the app's colour, so a click on the button is seen
  leaving its picture on the timeline. It sits on a chip of the app's
  colour while the playhead is on its frame.
- **With the playhead on a thumbnail**, the button or **T** removes it.
  What one click adds, one click takes away.
- **A click on a mark** puts the playhead on it, so the video preview shows
  the picture.
- **A mark is dragged** to another frame. The playhead goes with it, so the
  video preview shows the frame under the hand, and the mark stays inside
  the pieces the clip keeps. It is saved when the hand lets go.
- **Undo** takes back adding, moving and removing one.
- A trim or a cut that leaves a thumbnail outside the clip hides it. It
  comes back with the part it was in.

### Caption timing

A caption appears when its first word is said and goes when the next one
appears, or a moment after its last word when a pause follows. Where the
timing of the words is a little off from what is heard, a caption can be
moved by hand, on the clip timeline.

- **The captions run across the middle of the clip timeline**, each a
  block from where it appears to where it goes, on the same clock as
  everything else on it. A click on a block puts the playhead on that
  caption's first word.
- **Either edge is dragged.** Where two captions meet, the line between
  them has two sides: the left side is where the one before goes, the
  right side where the one after appears. Dragging the left side leaves a
  gap and moves nothing else. Dragging the right side moves when the next
  caption appears, and the one before goes with it, so nothing is left bare
  that was not bare before. Nothing overlaps, because the picture shows one
  caption at a time. An edge lands on a whole frame.
- **The caption box in the video preview follows the drag**, so what is
  seen while dragging is what is saved.
- **A block is its caption in the short's colours**: the box colour, as
  see-through as the box is set to be, over the waveform the way the box
  lies over the picture, with a bar in the colour of the words, so every
  colour and every opacity picked in the captions column is seen here too,
  while it is picked. The blocks lie over everything else on the clip
  timeline, the clip frame, the time lines and the playhead included. The
  colours never change with the state, because a dimmed colour is another
  colour. At rest the bar is a hairline and under the pointer it is
  thicker. The caption the video preview is showing wears the highlight
  colour over its box, with its own opacity, the pill the spoken word
  wears, and pops with the same animation that pill makes in the video
  preview, on every word as the pill does, so walking the words with shift
  and the arrow keys pops the pill and the block together. A block takes no focus, so no focus ring is left
  round one when the keys walk the words on. A click puts the
  playhead a frame into the caption's first word, so the video preview
  shows that word lit, the first caption of a clip included, which is on
  screen from the clip's first frame, before its first word.
- **A word is lit from its start until the next word starts**, and the
  last word of a caption until the caption goes, in the video preview, on
  the clip timeline and in the render alike: `litWord` in
  `frontend/src/lib/flow.ts` for the two in the app and the highlight in
  `engine/highlight.go` for the short. The video preview used to light a
  word only until its own end, so the highlight went out while a word was
  still being said and between every two words, where the short and the
  clip timeline went on showing it. The edge
  under the pointer, or the one being dragged, is a white line with a dark
  edge that goes again when the hand lets go. Two captions that meet have
  a gap of two pixels between them.
- **A click on an edge puts the playhead on it**, the way a clip edge does,
  which is how a caption is heard from where it appears.

A moved caption is kept in the clip's plan against the word it begins or
ends on, by when that word starts in the episode, so it survives the
clip's pieces moving, and a size or face that breaks the captions in other
places simply leaves it unused. The engine side is `SetCaptionTime` in
`engine/edit.go` and `moveCaptions` in `engine/lines.go`.

### Undo and redo

**Everything done to an episode's clips can be taken back**, with **Undo**
and **Redo** in the Edit menu, Cmd-Z and Cmd-Y, Ctrl-Z and Ctrl-Y on Windows and Linux. That is a trim, a
cut made, moved or put back, the crop frame, the caption box moved, a word
corrected, added or removed, the caption face, size and colours, a clip removed,
a search removed, and the window on the range picker moved, made longer or
shorter, or put back with a double-click. Each is one step. The window a
search moves on by itself is no step, and undoing a step of the window
puts it back where the hand had it whatever a search did with it since. Taking one back chooses the clip it
changed, so nothing changes where nobody is looking. A new edit after an
undo starts again from there, and what was undone is gone, as in every
editor.

- **What is in it.** What a person does to the clips, and nothing the app
  does by itself. Transcribing, searching and rendering make things rather
  than change them. Moving the playhead, choosing a clip and zooming are
  not edits. The settings are not in it, apart from the caption height,
  which is moved in the workspace like everything else here.
- **The playhead goes back with the edge it moved.** Dragging an edge or a
  cut on the clip timeline takes the playhead along, so the hand moved
  both, and taking the edge back puts the playhead back where it stood
  when the hand took hold. Redo puts it where the drag left it. Moving the
  playhead alone is still no step.
- **A field keeps its own undo.** While a word in the caption box or a
  number beside the clip is being typed in, Cmd-Z takes back the typing,
  the way it does in any text field. Once it is saved, the key goes to the
  episode.
- **One history per episode, while the app is open.** Everything saves the
  moment it is done, so closing loses nothing, and there is no history to
  come back to. It goes 200 steps back.
- **What landed since stays.** A search writes clips into a plan while its
  earlier clips are edited, so an undo puts back only what the edit
  changed, clip by clip, and never the whole plan. A clip that lands at
  the very moment an edit is saved is not part of that edit either, since
  an edit never makes a clip. A clip that has been
  changed again since by something the history does not know about, a
  search over the same part above all, is not written over: the undo
  says it cannot be taken back, and the history of the episode starts
  again from there.

The engine side is `engine/undo.go`, the app side
`cmd/framefairy-app/history.go`, and the menu `cmd/framefairy-app/menu.go`.
The menu has to be the app's own: on macOS the stock Undo takes Cmd-Z
before the page sees it and hands it to the web view, whose undo only
knows about text being typed.

### Work in hand

Everything that runs says so the same way, wherever it runs. There are four
things and no others, and each one means one thing.

- **The beam.** Light runs clockwise round the edge of the control the work
  was started from, for as long as the work runs. A round is five turns and
  no two of them alike: the comet gets away, eases right off through one
  long side, comes back hard through the next, sits down again and finishes
  quickly, so the slow part falls somewhere else on the edge each turn, and
  a broad faint wash goes round three times in the same time, so the two
  are never in the same place twice. It never stands still and it never
  goes back, because a light that hesitates on a border reads as broken and
  one that backs up reads as a stutter. Sampled out of the browser over a
  round: five turns, no step backwards, and between 0.59 and 1.63 of the
  even pace.
- **The motes.** Specks of that light drift up through the control, behind
  its own words. They are the beam's, not a thing of their own, and they
  are why a control with nothing to report is still alive to look at.
- **The fill.** How far the work has come, when that is known. Inside the
  control it is a wash of the app's colour, 30 percent at the start and 50
  at the head, with a bright line at the front, so what is done can be told
  from what is left with daylight on the screen, and a light passes over
  what is done, so a fill is never a flat wash. The fill slides by a transform rather than growing, so it
  moves with whatever moves beside it. The range picker draws the reading
  of the episode with `Busy.svelte` itself, fill, head, glow and motes,
  without the rim, and the row of the clip list that waits for the
  transcript fills by the same line, so the two never say different
  things.
- **Paused work is still, not gone.** How far it got stays true, so the
  fill stays where it is, and everything that says the work is running
  stops: no beam, no motes, no light over the fill, and the head keeps its
  line without the glow ahead of it. Running and paused are told apart by
  movement. The range picker shows a transcription stopped at a window,
  or waiting while a search has the machine, this way. In
  Activity, where a job has no control of its own, the same fill lies in a
  track of its own. It is `Busy.svelte` with no rim and no motes, not a
  bar drawn apart, so it cannot drift from the fill in New and Render.
  With no words over it, the track sets the wash to the app's colour
  itself, a brighter light over it and a rounded head, and nothing else
  about it is its own. Work that cannot say how far it has come shuttles
  across that track instead of standing at a number it does not have,
  which is Busy's `shuttle`.
- **The shimmer.** A place that is not filled yet: the rows the clip list
  will have, which are already rows of the list and brighten under the
  pointer the way a clip's row does while they go on breathing, the part of the clip timeline the transcript has not reached,
  the window on the range picker while clips are being found for it. The
  place itself dims and comes back, two seconds, in and out. It is
  `animate-pulse`, which is what
  [shadcn/ui](https://www.shadcn-svelte.com/docs/components/skeleton) and
  Tailwind ship and what most of the web wears, and it is the whole of it:
  no band, no sweep, no light crossing anything. In a list each row starts
  a step after the row above it, so the breath runs down the column rather
  than every row rising and falling together. Four brighter ways of doing
  this were tried and looked at side by side, and this is the one that was
  chosen. They all stay in `make motion` to be compared again, where they
  cost the app nothing.
- **The pulse.** Work running somewhere else. The dot beside an episode in
  the sidebar and the dot on **Activity** on the rail keep their size and
  their place, and a ring widens out of them and fades.

The colours are the app's own throughout, mixed from the accent, so
changing it in the settings moves the beam, the fill and the shimmer with
everything else. On a control that is already the app's colour the light is
white instead, because the app's colour cannot be seen on itself. With
**Reduce motion** on in the system nothing moves: the beam is a steady rim,
the motes are not drawn, and the shimmer and the fill stand still.

All five on one page, without starting five jobs in the app and catching
each at the right moment:

```
make motion
```

It opens a page in the browser with every one of them running, at every
share and in every kind of control, and a colour picker at the top so the
whole set can be seen in another accent. The shimmer stands there five
ways: react-loading-skeleton's sweep, MUI's wave, the breath that
shadcn/ui and Tailwind ship and that the app wears, the breath and the
wave together, and a foil of our own. It is preview material in
`frontend/preview/motion/` and never goes into the app: the product only
serves making shorts.

All of it is the stylesheet's. Nothing here runs JavaScript, nothing asks
for a frame and nothing measures anything: every moving part animates a
transform or an opacity, which the compositor carries without painting
again, and the one curve that is not a plain ease is a CSS `linear()`.
Measured in headless Chromium: 60 frames a second with 114 of them running
at once on the bench, 60 with a search running in the workspace, and no
animation at all at rest.

The one exception is time running out, the fill of a removed clip. Its
animation is still the compositor's, but it is started, held and let go
from `Busy.svelte`, and a clock there decides when the time is over. The
clock is the time. Held, the fill stands where the clock says, a plain
transform with no animation. Let go, it is a new animation from there to
nothing over the time left. An animation is never paused and played
again: WebKit, which draws the app on a Mac, plays a paused animation on
from its timeline's last frame rather than from now, so the fill jumped
ahead as the pointer left the row and back as it came in, and read as
time given back. A play state flipped by `:hover` in the stylesheet did
the same and really gave the time back.

`frontend/src/components/Busy.svelte` is the whole of the beam, the motes
and the fill, in a control and in the track. The track itself, the shimmer
and the pulse are in `frontend/src/app.css`, because they are worn by
things that are not controls. The track is `.progress` there and not
`.bar`, because the bar is the one across the top of the app, and while
the two shared a name the bar was picking up the track's rounded corners.
A way of showing work that looks like one of these is one of these, given
what differs as a prop or a colour its host sets, never a copy.

### What the interface may ask for

Every call that names a file is checked against the library before anything
is read, written or started, and a path is judged by where it really leads.
The interface only ever names files it was given, so this is the last line
rather than the first, but it is the line that holds when something else
asks.

A plan is checked against its episode too, not only against the library.
It has to be in that episode's own logs folder, so a plan of one episode
given with the path of another is refused, rather than edited against the
wrong video, the wrong words and the wrong history.

### Activity

Everything that runs in the background, with progress, a log per job and
**Cancel**, which wears the beam while the job winds down so the click is
seen at once. One job is one
row, parted from the next by a line across the page, and clicking a finished
row opens its log. Transcription runs in its own lane, so finding and
rendering clips never wait for it.

The list is kept current by the news the Go side sends about every job.
Each piece of news carries a number that grows with every change, so a
piece that arrives late never puts a job back to where it was. While
anything runs, the list is also read again every five seconds, so a piece
of news that was lost costs a few seconds and never a job that looks as if
it runs for ever. Quitting the app stops the model and every job before
the app goes.

**Quitting.** Cmd+Q dims the app at once and says what happens next.
The first press only asks, Press ⌘Q again to quit, for three seconds, the
way Chrome does. It does so on every page and whether work runs or not:
it asked only while work ran, so the workspace, where something nearly
always runs, took two presses and the settings page quit on the first. A
click or Escape takes the question away, and the next press asks again:
the press behind a question taken away still counted once, so ⌘Q,
Escape and ⌘Q on the next page quit at once. The second press says Quitting
while the app stops everything it started, and then it goes. Closing the
app's window quits without asking, and so does **Relaunch** on the
Updates page, which was asked for with a click. The stopping happens away
from the main thread, see `cmd/framefairy-app/quit.go`: done on it, it
froze the app under the spinning wheel for five seconds.

### Settings

The settings are made the way the **Updates** page is: a column of cards,
each card one subject, each row a mark, what it is in a few words with a
line under them, and at its end the one thing to do about it. The card and
its rows are `.card` in `app.css`, shared by both pages. Everything saves
as it is changed, the way System Settings does on the Mac, so there is no
**Save** button and nothing to forget to press. A field saves a moment
after the typing stops.

**What is in the way is said where it is put right**, not in a list of
its own. A missing API key is said on the row where the key goes, a
missing llama-server under the choice of model, a missing ffmpeg under
**Shorts**, a missing speech model on its row, which offers **Install**.
A problem the page has no row for, which none of today's checks is, is
said at the top so nothing goes unsaid. The first version had one card at
the top listing every problem, and a person had to go from the top of the
page to the bottom to find where to fix what it said.

**Finding clips is one choice**, **Find clips with**, in one list: under
**In the cloud** a model from each company the app knows, Claude Sonnet 5
from Anthropic and GPT-6 Sol from OpenAI, with the company beside each,
and under **On this machine** every model that runs here, the way apps that
offer models list them by where they run. The rows that depend on the
choice are under it in the same card, the way a pop-up in the Mac's own
settings changes the rows beneath it: the key of the company whose model it
is, and with a local model what it needs and, when it is missing,
llama-server. The first version had a card of two ways and a second card of
models under it, one deciding the other, and the two read as things that
had nothing to do with each other. It also had room for one company only.
Nobody should need an account with a particular company to use the app,
so every company is a row of the same list, and a third is an entry in
`engine/provider.go` and nothing more.

**Each company's key is its own.** The key row names the company of the
model chosen, **OpenAI API key** or **Anthropic API key**, and where to get
one. Keys are kept apart in the keychain, so trying one company never costs
the other's key, and going back finds it still there. Without its key the
choice wears the warning like a model not downloaded: the triangle, the
frame round the list and the line under the key's name. A model written in
by hand under **Advanced**, **Model in the cloud**, belongs to the company
its name says, `claude-` or `gpt-`, and is in the list too, so the list
never names nothing.

The decisions behind the row, which company a model belongs to, what the
list offers, and what the line says and how it stands in every state, are
in `frontend/src/lib/finding.ts`, and `finding.test.ts` walks every state
somebody can click their way into, run by `make interface` and CI.

The line under the choice says what it costs: a few cents an episode and
which company, or who made the model, the memory it needs and whether it fits.
Beside each model in the list is what it would cost to fetch, or that it is
the best here, or that it is too big for this machine. The list holds any
number of models in the room of one control.

**Choosing a model chooses it**, whether it is on this machine or not.
The settings name it, and the list ticks it. One that is not here yet
cannot find clips until it is downloaded, and everything about the row
says so in the colour of a warning: the triangle with the exclamation mark
before it, the frame round the list, and the line under it, **Not
downloaded yet** and its size. **Download** beside the list fetches it: a
download of gigabytes starts when it is pressed, never as a side effect of
looking through a list. While it runs the line says how far it has come and
how long is left, **Cancel** carries the beam and the fill, and the list
waits. When it is there the row turns to the check, with nothing more to
do, because the model was already the one chosen. A download cancelled
leaves the choice as it was, and **Download** carries on from what arrived.
Keeping the model before it in use until the download was done was tried
first, and the list then ticked one model while the line said the clips
came from another.

**The line under a name is one line.** It is short enough to fit beside
the list and a button, and anything longer is cut short with the whole of
it in the line's title, because a line that wraps when a button arrives
beside it moves the whole card. What the choice costs, who made a model
and how much it asks of the machine, is on the line. Everything else is
for the info mark. After a model is chosen the page reads the models again,
because the list says which one is in use from them: reading only the
settings once left the list on the model before, so a choice looked as if
it had done nothing.

Its info mark, at the right end of the heading, says what
each way costs and, with a local model chosen, how much memory this
machine has and what that decides. The key goes in the keychain the
moment **Save key** is pressed, not with the rest of the settings, because
it never lands in the settings file. An app opened from Finder has no shell
environment, so this is the only way to give it a key. Before it is kept,
the key is shown to its company, which asks for the list of models and
costs nothing, and while that runs the button says Checking. A key the
company refuses is said in the key's own row, in red, and not kept, so a
wrong key is found where it was typed rather than when the first search
fails. A company that cannot be reached says nothing about the key, so it
is kept. A refusal is one short line: **Anthropic did not accept this
key**, or, for something else copied from the same page, like the key's ID,
**Not an Anthropic key. Those start with sk-ant-**. The field takes it the
way the Mac's password fields take a wrong password: it shakes, keeps what
was typed, selected, with the keyboard in it, so the next paste replaces
it, and Save waits until the field holds something else, since the same
text would be refused again. Without a key the row says **None yet. Get one
at platform.claude.com**, or platform.openai.com, and the address is a link
that opens the page where keys are made in the browser. The app only ever
opens the two addresses written in `engine/provider.go`. A saved key is shown in
short, the way the companies list keys, `sk-ant-api03...MwAA`, its first
twelve characters and its last four. The short form is kept as the
keychain item's comment, which macOS lets the app read without the key,
so showing it never asks for anything. The field is narrow, since what is
typed shows as dots. The trash can beside it removes the saved key, and
asks first, because the app keeps no other copy. Without a saved key it
keeps its room, so the field does not move. A key in `ANTHROPIC_API_KEY`
or `OPENAI_API_KEY`, which an app started from a terminal has, is shown in
short too, with where it comes from in the title, until a key is saved,
and a saved key is the one used. It cannot be removed here.

A key refused in the middle of a search says so in words on the search's
row, and says where the key came from: the saved key, to be replaced in
the settings, or the variable it was read from. The company's own answer
is kept in the episode's `logs` folder, as every refused request is.

**The settings are not left while clips cannot be found.** A model
chosen and not downloaded, a model in the cloud without its key, nothing
chosen, or llama-server missing: going on from any of them is going on to
an app whose every search fails, far from the one place it can be put
right. So a move to another page, from the sidebar or the menu, is refused,
and the card comes into view and shakes, the way the Mac's own password
field does when it will not let somebody in, with the keyboard on the
list, where choosing a model that is here or the one with a key puts it
right. A download on its way holds the app too, because a model is no use
until the last of it has arrived. Only a page still reading what it has
to show does not. The
settings hold the app through `nav.hold` in `lib/state.svelte.ts`, which
every move between pages asks first, and when they hold it is decided in
`lib/finding.ts`, with its tests.

**Downloaded models** is the last row of the card, folded, with how many
there are and the room they take. It is only about that room: opened, it
lists them with their size on disk and a trash can each, quiet until the
pointer reaches it. Removing one asks first in the box over the app,
because fetching it again is gigabytes: **Cancel**, where the keyboard
starts, and **Remove**. While a model is being installed the trash cans
wait, dimmed. A model is not removed while it is being installed or while
the work that reads it runs, and the refusal says which. Removing the model
in use lets go of it in the settings too, and the page reads the settings
again rather than saving over that.

Two other ways of choosing a model were tried before this one and left: a
card of rows with a round mark each, which took a row per model and grew
with every model added, and a row of tiles by size, which does not hold
four models and put names like Largest on models that are only larger than
each other.

A second model arriving by itself never takes the first one's place. When
the settings name none, the one model on the machine is in use by being the
only one, and a download that finishes names it in the settings. A machine
that already has two and none named, from before this was so, says so
under the choice, and choosing one settles it.

**Speech** is one model today, so it is a row rather than a choice: a
check before it once it is there, what it covers and the room it takes.
Removing the speech model means the app asks for one again the next time it
starts, because nothing can be transcribed without it.

**Shorts** says where rendered shorts go, next to each episode unless a
folder is chosen with **Choose…**, which
opens the system's own folder dialog. **Next to episode** goes back to the
default.

**Appearance** holds the **accent colour**, what the app picks things out
in: the chosen clip, the window on the range picker, a button that matters.
It is the Mac's own row of round colours, with the app's purple, `#942192`,
first, and a colour wheel at the end for any other, which opens the app's
colour picker, see **The colour picker** under the workspace, without
presets of its own, because the row before it is its presets. The app's colour is the
only one the interface has: the lighter shade under the pointer and the
wash behind a chosen clip are mixed from it, so changing it moves all
three, and it moves as the colour is picked. The colours burned into a
short, the words, their box and the pill behind the word being spoken, are
set together in the captions column of the workspace, where the video
preview and the clip timeline show them as they are picked. A taste in one
is not a taste in the other. A highlight colour chosen here before is still
the one a search gets until its own is picked in the captions column.

**The tools are the app's own.** The app runs the ffmpeg, ffprobe and
llama-server beside it and no others: not one on the search path, and not
one named in the settings, which it used to run whatever they named. Each
is checked before it runs to be the file the app was built with: `make`
takes the SHA-256 of each tool it puts beside the programs and builds it
into them, and a tool that does not match, or one no sum was built in
for, is not run. A tool that is missing or does not match is said in the
check under **Advanced**, in words, rather than replaced by another.

**The check says which tools these are.** Each of ffmpeg, ffprobe and
llama-server has its row, with the version it says it is beside its name,
the file, its SHA-256, and whether that is the sum built into the app.
**Check again** reads every file again rather than trusting what it read
before, so a tool changed since it last ran is caught by looking, not
only by the next run that refuses it. llama-server says its build, b and
the number, because the build passes the pinned build number to
llama.cpp: a clone of one commit had every server saying it was build 1.

**The tools come with the app.** They are inside it, so an update brings
the ones its build was made with, and there is nothing of them to update
on their own: a version raised in a build script is a new app, and the
next update is the new tools. A customer never runs older tools than the
app they have. The
environment can still name one, `FRAMEFAIRY_FFMPEG`, `FRAMEFAIRY_FFPROBE`
and `FRAMEFAIRY_LLAMA_SERVER`, which is how the tests take theirs. See
`FindTool` in `engine/tools.go`.

**Training data** says how many records there are and has the one trash
can that throws them away, which asks first.

**Advanced** is closed until it is opened, like a disclosure in Finder, and
holds the machinery: the path to the language model file, or the Claude
model, the speech model folder and the training data folder, each a name
on the left and a field on the right. There is no field for ffmpeg or
llama-server any more, see **The tools are the app's own**. Empty paths use
the same defaults as the command line. Under the paths is everything the
check looked at, found or not, with the check's own words and **Check
again**, though the check runs again by itself whenever a setting changes:
which ffmpeg,
which of the system's own video decoders it can use, VideoToolbox on a Mac
or decoding on the processor, the caption faces the app carries inside
itself, and which model files it found. Which decoder a file really went
through is in the log of its search.

What belongs to the episode being worked on is not here: the caption face,
the size and the height, and how many clips a search looks for and how
long they may be, all sit in the workspace next to the clip, and what is
set there is kept for the next episode.

### Acknowledgements

In the Help menu, which is where apps keep them, rather than on the rail
beside the things making shorts needs. Every piece of other people's work
the app is made of or brings with it,
grouped by where it is: the app, its interface, speech recognition, ffmpeg,
llama-server, the caption fonts, and the models the app fetches from their
makers. A row is the name, the version and the licence, and it opens to what
the licence asks to be said and to the licence's own text, the way a finished
job opens to its log in **Activity**. The list is built into the app from
`notices/`, and how it is made and kept complete is in
[THIRD_PARTY.md](THIRD_PARTY.md).

## Where things are kept

- **Models:** in `~/.framefairy/models`. A speech model is a folder, a
  language model is one `.gguf` file. Neither is part of the app, so the
  app fetches them: the speech model by itself on the first run, a language
  model when somebody picks one. See [PACKAGING.md](PACKAGING.md).
- **The API keys:** in the macOS keychain, one item for each company,
  **Frame Fairy: Anthropic API key** and **Frame Fairy: OpenAI API key**,
  under the account `framefairy`. Never in a file. They are kept through
  the Security framework, so a key never passes through a command line,
  where any other program could see it while the command ran, and the
  item's access list names Frame Fairy: another program asking for the key
  gets a box from macOS first. Whether a key is there is asked without
  reading it, so opening the settings never asks for anything, and saving
  one never asks either, because an app may always write an item of its
  own. Until the app is signed with a Developer ID, every new build is a
  new app to the keychain, so a key saved by one build and read by a later
  one asks once whether the later one may use it, which **Always Allow**
  answers for that build. A key saved and used by the same build never
  asks. The first keys were kept with the `security` command, under
  `anthropic-api-key` and `openai-api-key`, where anything could read them.
  A key found there is moved into the app's own item the first time it is
  used, and the old item removed. `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`
  in the environment stands in when no key is saved. The command line
  reads the environment first, which is how it is given a key, and the app
  reads the saved key first: an app started from a terminal has the
  terminal's environment, and an old key in it would otherwise win over
  the one just saved, without a word.
- **Settings and the episode list:** plain JSON files in
  `~/Library/Application Support/Frame Fairy` on macOS, `%AppData%\Frame Fairy` on
  Windows and `~/.config/Frame Fairy` on Linux.
- **Everything about an episode:** next to it in `<episode>.framefairy/`, the
  same folder the command line uses. See [CLI.md](CLI.md#run).
- **Word corrections:** in `<episode>.framefairy/logs/corrections.json`. They
  change what captions show, never the stored transcript.
- **Training records:** in `<episode>.framefairy/training/`. The app writes them
  and shows nothing about them. See [TRAINING.md](TRAINING.md).

The app only shows files that belong to an episode in its list. It finds
Homebrew's ffmpeg and llama-server even when it is started from Finder.

## How the app is built

| Part | Where | What |
| --- | --- | --- |
| Go side | `cmd/framefairy-app/` | the app's window, the calls the interface makes, the job queue, settings |
| Interface | `frontend/` | Svelte 5 and TypeScript, no Go |
| Built interface | `cmd/framefairy-app/dist/app/` | made by make from `frontend/`, embedded into the program, not in git |

The Go side uses Wails v3, pinned to v3.0.0-beta.23. The interface calls the
methods of the `FrameFairy` type by name, through the typed wrappers in
`frontend/src/lib/api.ts`. The type is declared in
`cmd/framefairy-app/main.go`, and its methods sit in files by what they
are for, named like their tests: `library.go`, `media.go`, `views.go`,
`words.go`, `edits.go`, `chosen.go`, `training.go`, `settings.go`,
`setup.go`, `jobs.go`, `search.go` and `updates.go`.

The built interface lives next to the Go code because Go can only embed files
from its own folder. It is generated, so never edit it by hand.

### Changing the interface

```
make run
```

make notices the change, installs the interface's packages if they are
missing and builds the interface before the app. `make test` also checks the
interface's types.

| Folder | What |
| --- | --- |
| `frontend/src/screens/` | the workspace, activity and settings |
| `frontend/src/components/` | player, timelines, clip list, work in hand |
| `frontend/src/lib/` | calls into Go and the shared state |
| `frontend/src/app.css` | colours, sizes and the base styles |

## Tests

```
make test
```

They cover the job queue, waiting for the transcript and the media route.
The one that transcribes is skipped when ffmpeg is missing.
