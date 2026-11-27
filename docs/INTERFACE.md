# Changing the interface of this app

The interface is TypeScript and Svelte in `frontend/`, run in a webview by
Wails: WebKit on a Mac and on Linux, Chromium on Windows. A cloud session
cannot open the app, so only the user can look at it. This page is where
the [interface skill](../.claude/skills/interface/SKILL.md) meets this
repository: the harness, the test video, the bench, and what went wrong
here, each a round of the user's testing. How the tests of the interface
run is in [TESTING.md](TESTING.md).

## The interface skill in this project

| The skill says | Here it is |
| --- | --- |
| a harness that renders the real interface | the preview, `frontend/preview/`, see [The harness](#the-harness) |
| the real back end, where it can be reached | the bridge, `frontend/preview/bridge.config.ts` and `cmd/framefairy-app/bridge_test.go`, see [Two ways to open it in a browser](TESTING.md#two-ways-to-open-it-in-a-browser) |
| the stub and its modes | `frontend/preview/wails-stub.ts`, see [The modes](#the-modes) |
| a settled signal | `settled(page)`, `preview/settled.ts` and `preview/standing.ts` |
| test data that says what it is | [The test video](#the-test-video) |
| a bench page for a shared component | `make motion`, see [The motion bench](#the-motion-bench) |
| a test of the boundary typed on one side | `cmd/framefairy-app/bindings_test.go` |
| the end-to-end tests | the walks in `frontend/preview/walks/`, see [Walks](TESTING.md#walks) |
| the shipped engine | WebKit, `WALKBROWSER=webkit`, see [Two browsers](TESTING.md#two-browsers) and [WebKit](#webkit) |
| a switch to a 2D canvas | `window.__flatScreen` |
| one component for work in hand | `Busy.svelte` and `app.css` |
| one rule for one kind of thing | `.frame` in `app.css`, for the crop, the window and the clip |
| one list to pick from | `Pick.svelte` |
| the type check and the unit tests | `make interface` |
| the rules of the layout | Interface rules in `CLAUDE.md` |

## The harness

`frontend/preview/` builds the interface against a fake Go side and opens
it in a headless browser.

```
npx vite build --config frontend/preview/vite.config.ts
node <your probe>.mjs
```

```js
import { workspace, boxes, rows } from "./frontend/preview/open.mjs";
const { page, stop } = await workspace({ scale: 2 });
// ... measure the one thing this is about ...
await stop();
```

`workspace()` opens a video, puts the sidebar away, picks a clip and steps
the playhead onto its first word, because a clip starts a little before
its first word and a playhead on its start has no caption box to measure.
It fails when the video preview shows no picture or there is no clip to
pick. A query whose search has not landed clips yet, like `?found` or
`?growing`, passes `clip: false`. `screen()` is the same window with
nothing clicked, for the first run, the settings, the empty window.

Steps wait with `settled(page)` until nothing is on its way and the picture
stands, or with `pageUntil` for what shows a moment later, never with
`waitForTimeout`, which ESLint refuses. On the frame queue's page,
`queueUntil` waits on the queue's reports. A probe that hangs: run it with
`DEBUG=pw:api`, and the last wait Playwright prints is the one that hung.

`frontend/preview/dist/` is build output, never committed. Write one-off
probes in the scratchpad.

### The modes

`query` tells the fake Go side what to pretend, `frontend/preview/wails-stub.ts`:

| mode | what it pretends |
| --- | --- |
| *(none)* | a finished video with clips and two searched windows |
| `?busy` | a search finding, with progress |
| `?unknown` | the same, with progress it cannot put a number on |
| `?transcribing` | a search hearing, ahead of the saved transcript, stuck there |
| `?growing` | a new video: a search transcribes from 10 minutes to its window's end, then finds, job events every 250 ms |
| `?lagging` | with `?growing`, the saved transcript trails what is heard, saved every 8 s of work like the engine |
| `?hear=100` | hearing at 100 seconds of audio a second, not 600, slow enough to watch |
| `?paused` | clips found, the video read part way, nothing reading on. `?short` reads only to 2500 |
| `?found` | Search starts a search that really finishes, clips and all |
| `?interrupted` | a search recorded as cut off while finding. `?waiting`: cut off while hearing, at 15 minutes. Cancel leaves any running search stopped, like the Go side |
| `?failed` | a search recorded as failed, with its reason |
| `?rendering` | a render running on the first clip, with progress |
| `?measuring` | the waveform measured from page open, four hours in 12 s, a levels event every 0.5 s, in the Go side's order: where the clip timeline last looked, on, then from the start. `?unmeasured`: never measured, the waveform is the transcript's alone |
| `?slowhand` | a clip made with I or O takes 4 s to place its crop and 4 more to be heard past the transcript, so its card shows on the way, not a second |
| `?slowgo=80` | every Go call answers 0.5 to 1.5 times 80 ms late, per method, like the real bridge |
| `?lagclips` | every clip list comes back 300 ms late, so a card on its way must hold its place until the list has its clip |
| `?asked=4` | with `?growing`, the search asks for 4 clips, not the workspace's number now, which the rows and target must follow |
| `?crossclips` | one clip list read comes back 400 ms late and the next at once, so the later read answers first. A card on its way and its clip must never both show |
| `?setup` | a bare machine: the first run, then an empty library. Download really runs both model installs, each on its own clock. One language model fits the machine, one does not |
| `?refuse` | the engine refuses edits: correcting a word and picking a caption face fail, to show a control's no |
| `?closing` | the Go side refuses every search, as while closing or removing the video: Search returns an already failed job, with its event |
| `?slowread=200` | every range of the video answered 200 ms late by `open.mjs`'s server, in the workspace and on the frame queue's page, to show the queue reading ahead so a cut never waits on the file |

Every search in the stub is one fake search taking the Go side's steps from
the same kind of record, hearing, then finding, clips landing one at a
time. What the interface asked for is on `window.__searches`. A mode
starts its search in the state it needs, never a made-up job.

The job list is read once at startup, then changed only by `onJob`. Two
probes passed against a broken cancel in a mode that sent no events.
`?growing` once sent a fraction but no `covered`, so the transcript's edge
never moved.

Made-up answers hid three bugs. Hand-written caption cues could not be
clicked back to the word they came from. Cues are built from the words the
clip's pieces hold now, on the clip's clock, a split word drawn as two.
Words made from wherever a call started differed between a call about ten
minutes and one about a clip, which showed only when stepping by words
crossed from one to the other. And cues overlapped where the engine clamps
each to the start of the next, so a word of the next cue lit nothing.

A new argument of a Go call goes in `api.ts`, in whatever passes it down,
and in `wails-stub.ts`, or the harness cannot reach it.
`main.FrameFairy.CutClip expects 7 arguments, got 6` was found by hand with
every test green, and `cmd/framefairy-app/bindings_test.go` now parses both
sides and walks the boundary.

## The test video

The harness passes every call under `/frames/`, the video preview's picture
and sound, to the Go side's own routes: `open.mjs` builds the app's test
program once and starts `TestHarnessFrames` in
`cmd/framefairy-app/harness_test.go` on the first call, on the decoder
`make frames` builds or the one `FRAMEFAIRY_FRAMES` names. Without the
decoder the video preview shows no picture, and a probe of the picture says
so rather than pass. Headless Chromium reads nothing back from a WebGPU
canvas, so such a probe asks for the 2D canvas, `window.__flatScreen`, as
the walks do.

The test episode is made by ffmpeg on the first run and kept in the temp
folder: four hours, 25 frames a second, 96 by 54, VP9 and Opus in an MP4,
about 45 MB, about four minutes to make. Without ffmpeg there is none, the
video preview says it cannot read the file, and a probe that needs it says
so rather than pass.

- **VP9 and Opus, not H.264 and AAC,** as Playwright's Chromium has no
  proprietary codecs.
- **The picture says which frame it is.** The top half is the frame number
  in 24 bars of four pixels, highest bit left, light for one. Read
  `.screen canvas` with `getImageData` along a row through the middle of
  the top half, at each bar's middle, since scaling blends the edges. The
  bottom half is a grey climbing from the first second to the last, as
  black and no picture look alike: around 28 is a picture.
- **The sound says where it is.** A tone climbing in pitch over noise that
  never repeats, so what the sound card got is matched sample by sample to
  the video's sound decoded by ffmpeg. Tap the app's own `AudioContext`
  from an init script: a subclass that adds an `AudioWorklet` recorder, and
  `AudioNode.prototype.connect` patched to send whatever goes to the
  destination to the recorder too. Find where the program starts in the
  recording by fitting, a few thousand samples either way.

`open.mjs` exports `episode()` and `episodeAt`, and for the frame queue's
page `framesEpisode()` and `framesAt`.

Attach `page.on("pageerror", ...)` in any probe that plays, or edits while
playing. The old player's frame loop threw `Cannot read properties of
undefined (reading 'end')` when a cut was put back while playing past it,
which ended the loop while all else looked fine.

## The motion bench

`make motion` shows the five ways the app shows work in hand on one page,
the beam and its motes, the fill in a control and as a track, the shimmer
and the pulse, at every share, in every kind of control, with a colour
picker for other accents. It is built from `frontend/preview/motion/` by
`frontend/preview/motion.config.ts` with the app's own `app.css` and
`Busy.svelte`. For a probe:

```
npx vite build --config frontend/preview/motion.config.ts
```

Anything added to the five goes on that page in the same change.

## What happened here

Each of these cost at least one round of the user's testing. They are
grouped by the section of the skill they taught.

### Measuring

- Info marks that stepped as they faded in kept a still
  `getBoundingClientRect`, and the bug was wrongly called unreproducible.
- `rows()` in `open.mjs` reads a screenshot's pixels back, averaged per
  row. It showed the waveform at 117 of 255 rather than 227, every bar
  straddling two pixels. A vertical edge needs the columns instead.
  `boxes()` gives the layout rects.
- Three names reported the same right edge to the hundredth of a pixel,
  and the one too long for its box ended four pixels short on screen.
- The transcript's edge seemed to move after a pause while its style held
  142px.
- `page.mouse.click` missed the range picker's mark every time while the
  transcript grew.
- A clip handle is twelve pixels wide and pulled six back, so a probe
  pressing at `getBoundingClientRect().left` cut a sixth of a second off.

### Proving

- The raster shift that made the info marks step never reproduced in
  headless Chromium, and only the cause being gone could be proved.
- Twice in one session a fix looked proved, and reverting showed the
  harness had never produced the state.
- The mark stepped beside the captions, the clip list and the range
  picker, not beside New clips, the clip timeline or the video preview,
  and the ones that stepped sat on a fractional pixel row.
- A pause passed with the Go wait called directly, but the interface never
  reached it. A probe of what the app does by itself starts where the
  person does, adding the video, and measures the edge passing the window
  to the search being asked for, with `?growing&lagging`, which saves
  every 8 s as the engine does.
- The `<video>` video preview drew its last still whenever a seek looked
  stuck, so a frame from anywhere flashed up. The probe passed against
  broken code until it loaded a still the way a busy machine does.
- A pause once hid the range picker's fill. Stopped work keeps its fill,
  `Busy still`, and loses what moves.

### Same kind, same look

- The head of a settings group and the head of the clip list share size
  and weight.
- The range picker's own copy of the fill lacked the glow and the motes,
  and the user saw it at once. A place that shows work puts `Busy` in it,
  `rim={false}` where there is no edge to run round. The same for the
  shimmer and the pulse.
- The crop, the window and the clip had three 2 pixel borders with three
  corners, now `.frame` in `app.css`.
- A card on its way and the clip it became were two rows of a keyed list,
  the card held by an effect a render late, and the whole list blinked.
  Counting rows every frame passed. Logging `scrollHeight` on every forced
  layout, through a patched `getBoundingClientRect`, caught the list a row
  short inside one update, `animate:flip` having set the leaving row
  absolute.
- A search's cards came in between rows at their moments in the video, and
  `animate:flip` slid the rows under them down. A running search keeps its
  rows by place, `places` in `ClipList.svelte`. The proof followed each
  row's element every frame, a `WeakMap` from element to number: the old
  code moved rows on 112 frames of one search of six, the new one on none.
- The clip list's placeholders brighten under the pointer like a clip's
  row while they breathe.
- The row waiting for the transcript read the saved transcript, the range
  picker's line what was heard, seconds ahead, so the line passed the
  window with the row at 95 percent. And the search waited on the written
  transcript while the line showed the heard one.
- ffmpeg framing a clip flicked the search's row to "finding camera
  switches" twice a second. Work that owns a line holds it,
  `Log.HoldProgress`.
- A new headline stays 2.5 s, a count going up 1 s, a time left moves in
  steps of five seconds. Loading the model, reading, thinking, are not
  headlines.
- A caption block marked in the app's colour vanished on a purple caption.
  A state is a pill in the highlight colour where the short uses one, or
  brightness or size.
- A fill of 7 to 22 percent of the accent read as none by a window. A fill
  is 30 to 50 percent.
- A caption block with `tabindex` took focus on a click, and after the
  first arrow key a `:focus-visible` ring stayed round a caption the video
  preview had left. Probe by clicking, pressing a key, and listing what
  matches `:focus-visible`.
- Between the rail and the workspace is `--gap`, and only the side on the
  app's own border is `--edge`. A 24 pixel edge where a 12 pixel gap
  belonged cost the settings column its room.

### Against a real window

The title bar looked wrong for round after round of tuning, and was fixed
in one change once it was put next to Terminal. A traffic light 28 pixels
wide means a 2x shot:

| | down from the window top | in from its left |
| --- | --- | --- |
| ours | 26.0 pt | 26.0 pt |
| VS Code | 17.0 pt | 18.0 pt |
| Terminal | 16.0 pt | 16.0 pt |
| Chrome | 20.0 pt | 20.0 pt |

Six to ten points out both ways, on every window: the whole bug,
invisible until written down.

`App.svelte` said nothing the app sets moves the three buttons. False:
Electron apps and VS Code move them, and `chrome_darwin.go` already reads
the frame of `standardWindowButton:`, so setting it is the same call the
other way round. Measuring, centring and rounding grew on that premise.
The cause was `UseToolbar: true` inside `MacTitleBarHiddenInset`: a toolbar
makes the bar taller, and macOS insets the buttons to centre them in it.
The user asked how VS Code does it, and that ended it.

### WebKit

The app on a Mac is WebKit. A removed row's fill grew back under the
pointer for two rounds, as WebKit plays a paused animation on from its
timeline's last frame and Chromium from now.

Walk a sequence in Playwright's WebKit with `WALKBROWSER=webkit` in front
of `node frontend/preview/walks/sequences.mjs`, and a probe of `open.mjs`
the same way. A cloud session installs Playwright's WebKit in the
background as it starts, see `/tmp/framefairy-webkit.log`.

For a probe outside the walks there is also the system's WebKitGTK 6.0,
with Python bindings for `/usr/bin/python3.12` (not the default `python3`),
`xvfb-run` for a screen and `libXtst` for a real pointer. Serve
`frontend/preview/dist` over http, open it in a `WebKit.WebView` in a
`Gtk.ApplicationWindow` with `set_decorated(False)` under
`GDK_BACKEND=x11`, move the pointer with `XTestFakeMotionEvent` from
`ctypes`, and read the page with `evaluate_javascript`. It plays the page
too with `gstreamer1.0-plugins-bad` and `gstreamer1.0-libav`: VP9 needs
`vp9parse`, H.264 needs `h264parse` and `avdec_h264`. There an MP4 of
H.264 with B-frames and AAC played every frame in order, cuts one frame's
time, but that is GStreamer. The Mac's WebKit decodes with VideoToolbox and
AudioToolbox, and only the user's Mac shows how those order output, stamp
sound and report latency.

A playing `<video>`'s clock in WebKit was taken for one that steps back,
and a forward-only rule froze the playhead. It is an estimate that never
goes back and runs ahead of the picture, `TimeProgressEstimator` in
`MediaPlayerPrivateRemote.cpp`, four lines.

### What a hand feels

- Continue left the Stopped row empty for six frames before Transcribing,
  and Cancel said Cancelling at once while the row said Transcribing for
  the second the search took to save. The rule covers the control, the
  row, the fill and the range picker. The frame-by-frame probe reads
  `.ghost.next` after clicking `button.search`, and passed against broken
  code until the fake search waited 800 ms to stop, as the engine does.
- Dragging a clip edge moved the edge and its wash, while the caption
  blocks waited for the hand to let go. `Shape` is the engine asked on the
  way. A line once followed a dragged edge while the marks it ran between
  stayed put, so `Framing` answers with pieces, marks, line and crop in one
  answer, for a saved clip and one under the hand.
- Pressing I showed nothing for ten seconds, the card far down the list,
  the clip timeline waiting, then all of it at once. Tests passed because
  the stub made the clip in a second, where the list was already looking.
  What is known, where it lies, how long, its pieces, its captions, is
  drawn where the clip will be, the clip timeline above all. Check it far
  down the video with the list scrolled elsewhere, the speech model
  loading included.
- Five bugs in one afternoon, from a dashed crop frame to stepping back
  stuck in a word, came from comparing the playhead to a moment while the
  `<video>` element answered a seek with the shown frame's start, see
  [Playback](#playback-on-the-clip-or-on-the-video).
- A caption's start, taken from the clip's clock to the video's and back,
  landed before the caption, and a click on it lit no word. A seek meant
  to land in something lands a frame into it, where the engine says a
  step lands on a word, `LandOn`, as the arrow keys do.
- Arrowing through the clip list puts several frame requests in the air,
  and without a ticket the picture settles on the slowest: `Newest` in
  `frontend/src/lib/flow.ts`, with tests.
- An effect that mentions a job is rebuilt about once a second, as every
  job event replaces the job object, see `CLAUDE.md`.
- The clip timeline and the range picker call `setPointerCapture` on the
  way down, so the double-click that puts a cut back is decided on the
  track, by pointer position.
- A drag starting on a caption word does not refuse the pointer, and lets
  the word go once the hand moves, or the caret never lands where the word
  was clicked. With the old line back the word never takes the keyboard.

### Layout and pixels

- The workspace once sized the picture from the clip timeline while the
  picture decided the timeline's column: several frames per resize, a
  laggy window edge.
- The transcript's edge moved by `left` and its mark by `transform`: in an
  idle harness the line stood still on 6 frames of 240, the mark on none.
  The range picker's fill and the shade ahead of it both move by
  transform, with the same glide.
- `text-box: trim-both cap alphabetic` centres a name, and `overflow:
  hidden`, for the ellipsis, then clipped the y of `YouTube.mp4`.
- The bar's box is twice the window buttons' middle. Fullscreen takes the
  buttons away, and a middle of 0 left the box no height.
- `width: var(--the-anchor-width)` with `min-width: max-content` was placed
  at the anchor's width, came out wider, and spilled the other way. It
  took three rounds of testing, because in Chromium a ResizeObserver
  corrects the second pass, and the user saw the first.
- The clip timeline's info bubble fitted neither over nor under its mark
  and ran off the foot of the app, while probes at a comfortable height
  passed. `placeBubble` in `lib/bubble.ts` is the rule, with tests that
  put a mark everywhere.
- Every list is `Pick.svelte`. The sidebar's sort menu closed the sidebar
  on the Mac as the pointer reached a row, and it flashed open and shut
  with no row pickable. The probe passed because the harness's sidebar was
  pinned. `onopenchange` on `Pick` keeps the sidebar open. It also trusted
  a stale pointer when its list closed, so Escape or a click closed both.

## Playback: on the clip or on the video

The playhead is in a state, **on the clip** or **on the video**, set by the
gesture that put it there and nothing else, not a clock, not the queue's
position, not a rounding. `placeOf` and `playFrom` in
`frontend/src/lib/playhead.ts` read the gesture, and where on the clip a
moment is, the engine says, `SourceInfo.Places`.

| the gesture | leaves the playhead |
| --- | --- |
| picking a clip, from the list, a mark, the keys, a search, a clip made or put back | on the clip, at its start |
| I or O while the video plays, which makes a clip around the playhead | on that clip, where it is, on its way and once written |
| a click on the clip's start edge, a trim, a click on a caption | on the clip |
| a click on the clip's end edge, the clip playing to its end | on the clip, at its end |
| a click, a drag, a step or a seek into the frame that holds the moment a frame before the clip's end | on the clip, at its end |
| the same into any frame from the one that holds the clip's start on, cuts included | on the clip |
| the same into any frame before or after those | on the video |
| no clip chosen, or a clip chosen with no gesture, while the video plays | on the video |
| pausing, a jump over a cut, a loop | where it was |

**On the clip**, the space bar plays the clip from the playhead, jumping
its cuts, and stops at its end, or with loop on goes back to its start.
From a cut it plays from the cut's end, from before the start but inside
the clip's first frame from the start, and from the end it starts over, as
QuickTime does.

**On the video**, the space bar plays the video straight on, through the
chosen clip and its cuts, and stays on the video. The chosen clip is then
dimmed wherever drawn, `.frame.dim`, `.clipmark.selected.dim` and
`.chosen.dim` in `app.css`, in the frame the state changes, so a person
sees the clip's rules are not in play.

**The whole clip dims, as one thing.** Bright caption blocks in a dim
frame read as a clip half in play. Everything drawn for the chosen clip
on the clip timeline, its frame, pieces, cuts, trim edges, caption blocks
and thumbnails, sits in `.chosen`, which lays nothing out, and one rule
dims it with the crop frame and the range picker mark, `opacity:
var(--dimmed)`, a disabled button's brightness, so the caption colours stay
the person's. The waveform is the video's and does not dim. It is lit whole
while the hand is on the clip elsewhere. A new part drawn for the chosen
clip goes inside `.chosen`, or it stays bright. Prove it by reading every part's opacity, the
product down its ancestors, on the clip, on the video, and on the first
animation frame after the click.

**While it plays, the playhead is the sound being heard.** On every
animation frame the frame queue reports where the program is by the sound
card's clock, placed in the video through the program, cuts included, and
draws the frame for that position. So the playhead moves every frame,
jumps a cut with the sound, sits at most a frame ahead of the shown
frame's start, and never goes back while playing. A play or a seek while
playing reports its start at once, so the playhead is at a click in the
click's frame and waits there for the first sound, see [The
start](#the-start).

**A click while it plays lands where it was clicked.** Clicks across the
clip's edge were lost: the new program restarted the play from the old
moment, the listener set the playhead there, and the seek after it read
the playhead and went back. So `seek` takes both, `queue.seek(at,
program)`, and the play starts once. And a click sent its moment on the
way down and up, so with the hand held 150 ms the second sent the playhead
back. `scrub` sends nothing on the way up where the hand has not moved
since the last seek, and the queue takes a seek to the moment a play is
starting from as that play.

The proof, with the clip of three pieces: play, then click the clip
timeline 17 times, out of and into the clip in turn, some 120 to 300 ms
apart while the play starts, some a second apart. Read `data-playhead`,
the bars and `.chosen.dim` every animation frame, and check per click: on
the click two animation frames after the press, then following its own
play (on the clip's program, so look-ahead before a cut is not the cut),
never back, nothing played inside a cut or jumped outside the clip, the
dimming the place's. Before, the first eight clicks, all across the edge,
stayed where the play was. After, 0 of 17 broke the rule, with the hand
held 150 ms and with `?slowread=200`. With only the first half of the fix,
every held click went back once.

Paused, the playhead is exactly where it was put, or where the sound
stopped, and the picture is the frame holding it, nearly always decoded
already.

The proof of a play, with a clip of three pieces whose cuts are off frame
edges, set through `window.__pieces`: press the space bar or the play
button and for nine seconds read every animation frame the playhead,
`data-playhead` on `.screen`, and the frame number in the bars. Both cuts
take one frame's time, 33 or 50 ms, no frame from inside a cut is drawn,
none stays over 50 ms, the playhead never stands still after it first
moves nor goes back, it is under a frame from the shown frame on the
program, and it stops paused at the clip's end on its last frame. The same
with `?slowread=200`, with loop on across the seam, and played again from
the end. The tapped sound is the video's own to the sample across both
cuts, best lag 0, and nothing is heard past the end.

On an edge exactly, the side the gesture is about decides. From the end
edge the space bar starts the clip over. A trim puts the playhead on the
edge it drags, maybe a hair outside the pieces the video preview has, so
it says it is about the clip instead of being measured.

Loop is about the clip and changes no state: on the video with loop on,
the space bar plays straight on. A clip chosen while the video plays
leaves the playhead on the video, except one made there with I or O:
making it is the gesture, `claim` in `Player.svelte`, or it comes up dimmed
over the moment it was made from. The play goes on as it began, and the
next plays the clip.

**Where a gesture lands is a frame, not a second.** Compared in seconds,
one step right of a clip starting inside a frame landed on that frame's
start, before the clip, and the clip dimmed with its own first frame on
screen. So the place is decided by the frame the gesture landed in, and
only the engine says which frame is meant: the frame holding the moment,
where up to a millisecond before a frame's start counts as that frame, as
the app keeps moments to the millisecond. A clip comes with the moments
the playhead is on it from, at its end from and off it again from,
`places`, and the window compares the moment with them. Anything else
that asks which frame holds a moment looks it up in where the engine says
each frame holds from, `FrameTable` in `lib/frames/table.ts`, and counts
nothing itself. A copy of the count in
the window, held to Go by a shared case file, once named a frame the
playhead was not on when the two rounded apart. The engine keeps every
edge on the start of the frame the render cuts on, `PieceOnFrames` in
`engine/render.go`, so for an edge this is the short's frame. A second
rule for edges, the nearest frame, disagreed for about half the edges a
search left between frames. A trim of the end on frames, leaving the
playhead a frame before the end, is then at the end.

**Why it is a state.** It was a distance, within half a frame of the start
and before the end, and with the `<video>` element it broke three times: a
seek before every play, as the paused picture answered a hair before the
clip, the video played from its start after a search, from a clock still
at zero, and on the Mac straight through a clip's cuts, from a paused
video answering up to a frame early. A wider tolerance and a hold on the
playhead only moved where the rounding went wrong.

## The frame queue

The video preview plays frames the app decodes itself, in
`frontend/src/lib/frames/`. `movie.ts` builds what it plays from the frames
the engine names, and reads nothing of the video file. `queue.ts` drives
the Go side's frames and sound, the canvas and the sound card.
`Player.svelte` makes one `FrameQueue` per video and drives it with
`setProgram` (the clip's pieces, or null for the video straight on),
`seek`, `play` and `pause`. What the queue reports, `Shown`, is the
playhead, and nothing else sets it while playing. Since these lessons, which frame and which sound play where has
moved into Go, `engine.Play`, see [ARCHITECTURE.md](ARCHITECTURE.md) and
[VIDEO-PREVIEW.md](VIDEO-PREVIEW.md), and some names below, `heardAt`,
`VideoPlan`, `nextEdge`, `GoSound` and `plan.ts`, are gone. The numbers
are what a change must still show.

The queue's own page, `preview/frames/`, one canvas and `window.__queue`,
is the quickest way to look at it alone. `frames()` in `open.mjs` opens it
on a ten minute video by the same recipe, and `queueUntil` waits for a
queue report. `?read` reads the frame number back from the canvas. `?hear`
records every sample handed to the sound card, to match against the
video's sound decoded by ffmpeg, cut and faded as the render does, so a
sample missing, doubled or from inside a cut shows.

What the proofs read, the clip of three pieces with cuts off the frame
edges, 25 frames a second:

| | the frame queue, its own page | the frame queue, the workspace |
| --- | --- | --- |
| last frame before a cut to the first after it | 33 or 50 ms | 33 or 50 ms |
| the same with the file 200 ms late | 33 or 50 ms | 33 ms |
| frames from inside a cut drawn | none | none |
| the longest any frame stayed while playing | 50 ms | 50 ms |
| the playhead while playing | moves with the sound | still for no animation frame, never back |
| the playhead and the frame on screen | | less than a frame apart |
| from the press to the first new frame | 129 to 142 ms, from a frame's start | 74 to 81 ms, 107 after a minute paused |
| frames of a play passed over, begun anywhere | none in 11 plays | |
| the most animation frames a frame was drawn after it was due | 1 | |
| where the play stops | 65.000, frame 64.96 | 65, frame 64.96 |
| the sound across a cut | the video's own to the sample, best lag 0 | the same |
| the loop seam | the same as a cut | the same as a cut |
| paused frames | the frame that holds the moment, 11 of 11 | 14 of 14 gestures, 3 to 25 ms |
| frames open over a minute of play | 4 to 5 | |

A frame lasts 40 ms and the display draws every 16.7 ms, so one frame's
time shows as 33 or 50, and a gap above 50 is a frame held.

What it taught, each a round of probing:

- The Go side decodes the sound with ffmpeg, not the browser, so the video
  preview hears what the render is made of and the Mac's AAC decoder,
  which the cloud cannot run, is out of the picture. When the browser
  decoded, Chromium stamped sound with its own times, so it was matched to
  packets by order, and Opus cut its pre-skip twice.
- A flush of the sound ends a run where it was asked, between the packets
  fed before and after it, not once everything fed is out. A decoder that
  read on into the next run's packets played the sound before the second
  cut after it, 139 percent off, where the fixed one is 0.00.
- A Chromium AudioWorklet sometimes gives two render quanta the same
  `currentFrame`. The recorder labels the second the last one plus 128, or
  the check fails on right sound.
- `getOutputTimestamp` is the heard sound, but a stamp from before the
  sound card was held counts on through the pause, and WebKitGTK without a
  sound device stamps on another clock. A stamp is taken only when fresh
  and near the clock.
- The first frame of a play stays until the sound is heard. That is the
  picture keeping to the sound, not a hold.
- A paused seek cues the play with the paused frame first, so the space bar
  only starts the clock. Where the paused frame is not the play's first, in
  a cut or past the clip's end, it is decoded alone.
- A paused seek to a moment in the frame already on screen draws nothing,
  so it must report anyway, or whatever waits for it waits for ever.
- Frames drawn on a grid from where the play began came up to a frame late
  and skipped a piece's last frame, which the playback walk caught at five
  frames a second. The grid is the video's own frames, each drawn when the
  heard sound reaches its start, carried onto the program. A piece draws
  from the frame holding its start, or where the play began, to the frame
  holding the moment a frame before its end, the frames the render cuts,
  and the tick never steps past either without drawing it for one
  animation frame. A frame at a piece's edge can begin 10 ms before the cut
  and is then due with the one after it. The proof plays the clip from
  57.000, 57.001, 57.013, 57.020, 57.039, 59.213, 59.239, 61.731, 62.013,
  in the cut at 60.000 and from the end, reads every drawn frame back and
  finds the first animation frame its start was heard on: 0 frames
  skipped, none over one animation frame late, with `?slowread=200` too.
  The old grid skipped 1 to 3 frames in each of the 11 plays, 60.28 and
  62.16 from the clip's start, and drew frames up to 3 animation frames
  late.
- Plain sound in a MOV counts every sample as a moment, hundreds of
  millions in four hours. It is read by the chunk and turned into the sound
  card's numbers without a decoder.
- HE-AAC decodes at twice its core's rate. Where the file does not say so,
  the rate of the decoder's first sound is taken and the play restarts at
  it, before anything is heard.

The `<video>` element was left because a cut was a seek: each gap between
pieces held the last frame, a fifth of a second and more on the Mac, and
stopped the sound. No workaround made a cut take one frame's time.

### The start

From the space bar to the picture moving, from a pause after a click, in
the harness:

| part | ms |
| --- | ---: |
| the sound card's `resume()`, held since the pause | 2 to 6 |
| the clock set, from the press, the cue being ready | 3 to 11 |
| the first sound put ahead of the clock, `START_LEAD` | 25, was 50 |
| the sound card's output latency | 42 |
| the play's first frame, until the sound reaches the next frame's start | 0 to 40 |
| less the display frame the queue draws ahead | -17 |

So 74 to 81 ms in the workspace after a click or a pick, where the first
word is late in its frame, 107 ms after a minute paused, and 129 to 142 ms
on the queue's page, whose play starts on a frame's start. With 50 ms of
lead they were 91 to 111, 129 and 153 to 155. A pause mid play holds the
sound card and keeps the play, so the space bar only lets go and the
picture moves within one display frame, 5 to 8 ms.

The sound card stays held after a pause's fade: running it would save only
the `resume()` and cost a laptop power. `START_LEAD` puts the first sound
ahead of the card's clock, since sound put where the card has rendered
already starts late and out of step. The harness played to the sample from
2 ms, but the Mac's card renders 512 to 1024 samples at a time, 11 to 21
ms, and 25 ms covers that with room. Measure the parts with a wrapper
round the queue's `maybeBegin` and its sound card's `resume` on the queue's
page, and press to motion in the workspace from the bars on every
animation frame after a space bar `keydown` sent in the page.

## Before saying it is done here

- The walk for the fix fails on main and passes with it, see the testing
  skill and [Walks](TESTING.md#walks).
- `make interface`, the type check and the interface's own tests.
- The walks run in CI on four machines, and that result is the walk's
  proof. A single walk runs alone in the session only to write or fix it.
- Build the preview, and the motion bench if the change touches any of the
  five ways work in hand is shown.
- Say what to look at and what should happen in the app, in the order the
  user would do it.
