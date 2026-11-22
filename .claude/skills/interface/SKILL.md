---
name: interface
description: How to change this app's interface without seeing its window. Use for any work on frontend/ - a layout, a control, a colour, a drawing on the clip timeline or the range picker - and above all when the user reports something that looks wrong. Covers the preview harness, what to measure, and how to prove a fix rather than claim one.
---

# Changing the interface without seeing it

A cloud session has no screen, so only the user can look at the app.
Without care a session guesses, reports a fix, and the user finds the bug
still there, a round of their testing lost. It has happened more than once.

## The harness

`frontend/preview/` builds the interface against a fake Go side and opens
it in a headless browser.

```
npx vite build --config frontend/preview/vite.config.ts
node <your probe>.mjs
```

A probe is short:

```js
import { workspace, boxes, rows } from "./frontend/preview/open.mjs";
const { page, stop } = await workspace({ scale: 2 });
// ... measure the one thing this is about ...
await stop();
```

`workspace()` opens an episode, puts the sidebar away, picks a clip and
steps the playhead onto its first word, because a clip starts a little
before its first word and a playhead on its start has no caption box to
measure. It fails when the video preview shows no picture or there is no
clip to pick. A query whose search has not landed clips yet, like `?found`
or `?growing`, passes `clip: false`. `screen()` is the same window with
nothing clicked, for the first run, the settings, the empty window.

Their steps wait with `settled(page)` until nothing is on its way and the
picture stands, see `preview/settled.ts` and `preview/standing.ts`. A probe
waits the same way, or with `pageUntil` for what shows a moment later,
never with `waitForTimeout`, which ESLint refuses. On the frame queue's
page, `queueUntil` waits on the queue's reports. A probe has no deadline,
so one that hangs waits for a signal that never came: run it with
`DEBUG=pw:api`, and the last wait Playwright prints is the one that hung.

`query` tells the fake Go side what to pretend. The modes are in
`frontend/preview/wails-stub.ts`:

| mode | what it pretends |
| --- | --- |
| *(none)* | a finished episode with clips and two searched windows |
| `?busy` | a search finding, with progress |
| `?unknown` | the same, with progress it cannot put a number on |
| `?transcribing` | a search hearing, ahead of the saved transcript, stuck there |
| `?growing` | a new episode: the Go side's first search hears from 10 minutes to its window's end, then finds, job events every 250 ms |
| `?lagging` | with `?growing`, the saved transcript trails what is heard, saved every 8 s of work like the engine |
| `?hear=100` | hearing at 100 seconds of audio a second, not 600, slow enough to watch |
| `?paused` | clips found, the episode read part way, nothing reading on. `?short` reads only to 2500 |
| `?found` | Search starts a search that really finishes, clips and all |
| `?interrupted` | a search recorded as cut off while finding. `?waiting`: cut off while hearing, at 15 minutes. Cancel leaves any running search stopped, like the Go side |
| `?failed` | a search recorded as failed, with its reason |
| `?rendering` | a render running on the first clip, with progress |
| `?measuring` | the waveform measured from page open, four hours in 12 s, a levels event every 0.5 s, in the Go side's order: where the clip timeline last looked, on, then from the start. `?unmeasured`: never measured, the waveform is the transcript's alone |
| `?slowhand` | a clip made with I or O takes 4 s to place its crop and 4 more to be heard past the transcript, so its card shows on the way, not a second |
| `?slowgo=80` | every Go call answers 0.5 to 1.5 times 80 ms late, per method, like the real bridge. Without it a workspace that waits on answers looks finished in its first frame |
| `?lagclips` | every clip list comes back 300 ms late, so a card on its way must hold its place until the list has its clip |
| `?asked=4` | with `?growing`, the Go side's first search asks for 4 clips, not the workspace's number, which the rows and target must follow |
| `?crossclips` | one clip list read comes back 400 ms late and the next at once, so the later read answers first. A card on its way and its clip must never both show |
| `?setup` | a bare machine: the first run, then an empty library. Download really runs both model installs, each on its own clock. One language model fits the machine, one does not |
| `?refuse` | the engine refuses edits: correcting a word and picking a caption face fail, to show a control's no |
| `?closing` | the Go side refuses every search, as while closing or removing the episode: Search returns an already failed job, with its event |
| `?slowread=200` | every range of the episode answered 200 ms late by `open.mjs`'s server, in the workspace and on the frame queue's page, to show the queue reading ahead so a cut never waits on the file |

Add a mode when the state you need is missing. A stub that is never busy
hides what only happens while something runs, like the first search never
starting.

Every search in the stub is one fake search taking the Go side's steps
from the same kind of record, hearing, then finding, clips landing one at
a time. What the interface asked for is on `window.__searches`. A mode
starts its search in the state it needs, never a made-up job.

**The job list is only ever brought up to date by events.** It is read once
at startup, then changed only by `onJob`, so in a mode that sends no
events a cancelled job stays running, and two probes passed against broken
code that way. A mode for anything that
starts, stops or reports sends events with the numbers the real side
sends: `?growing` once sent a fraction but no `covered`, so the
transcript's edge never moved.

**What the stub answers with is made the way the engine makes it, not made
up.** Made-up answers look fine in a picture and hid three bugs:

- Hand-written caption cues could not be clicked back to the episode's word
  they came from, so a probe about correcting one passed whatever happened.
  Cues are built from the words the clip's pieces hold now, on the clip's
  clock, a split word drawn as two.
- Words made from wherever a call started differed between a call about ten
  minutes and one about a clip, which shows only when stepping by words
  crosses from one to the other.
- Cues overlapped, where the engine clamps each to the start of the next.
  The window takes the first cue covering the playhead, so a word of the
  next cue lit nothing.

One list, and the engine's own rules on it.

`frontend/preview/dist/` is build output, never committed. Write one-off
probes in the scratchpad.

## The episode the harness can play

The harness passes every call under `/frames/`, the video preview's
picture and sound, to the Go side's own routes: `open.mjs` builds the app's test program once and starts
`TestHarnessFrames` in `cmd/framefairy-app/harness_test.go` on the first
call, on the decoder `make frames` builds or the one `FRAMEFAIRY_FRAMES`
names. Without the decoder the video preview shows no picture, so a probe
of the picture says so rather than pass. Headless Chromium reads nothing
back from a WebGPU canvas, so such a probe asks for the 2D canvas,
`window.__flatScreen`, as the walks do.

The episode is real, made by ffmpeg on the first run and kept in the temp
folder: four hours, 25 frames a second, 96 by 54, VP9 and Opus in an MP4,
about 45 MB, about four minutes to make. Without ffmpeg there is none, the
video preview says it cannot read the file, and a probe that needs it says
so rather than pass. Three things are deliberate:

**It is VP9 and Opus, not H.264 and AAC.** Playwright's Chromium has no
proprietary codecs.

**The picture says which frame it is.** The top half is the frame number
in 24 bars of four pixels, highest bit left, light for one, so a probe
reads the frame drawn, not the app's word for it. Read
`.screen canvas` with `getImageData` along a row through the middle of the
top half, at each bar's middle, since scaling blends the edges. The bottom
half is a grey climbing from the first second to the last, because black
and no picture look alike: around 28 is a picture.

**The sound says where it is.** A tone climbing in pitch over noise that
never repeats, so what the sound card got matches the episode's sound,
decoded by ffmpeg, sample by sample. Tap the app's own
`AudioContext` from an init script: a subclass that adds an `AudioWorklet`
recorder, and `AudioNode.prototype.connect` patched to send whatever goes
to the destination to the recorder too. Find where the program starts in
the recording by fitting, a few thousand samples either way.

`open.mjs` exports `episode()` and `episodeAt`, and for the frame queue's
page `framesEpisode()` and `framesAt`.

**A page error is a result.** Attach `page.on("pageerror", ...)` in any
probe that plays, or edits while playing. The old player's frame loop
threw `Cannot read properties of undefined (reading 'end')` when a cut was
put back while playing past it, which ended the loop while all else
looked fine. The throw was the only clean signal.

## The motion bench

`make motion` shows every way the app shows work in hand on one page, the
beam and its motes, the fill in a control and as a track, the shimmer and
the pulse, at every share, in every kind of control, with a colour picker
for other accents. Nothing has to be started or caught, so it is the
fastest look at a change to any of them.

It is built from `frontend/preview/motion/` by
`frontend/preview/motion.config.ts` with the app's own `app.css` and
`Busy.svelte`, so it shows what the workspace shows. Build it for a probe
with

```
npx vite build --config frontend/preview/motion.config.ts
```

Anything added to the five goes on that page in the same change.

## Measure the right thing

**`getBoundingClientRect` gives the layout position. It cannot see a
painting bug.** Info marks that stepped as they faded in kept a still
rect, and the bug was wrongly called unreproducible. Decide which kind of
bug it is before picking an instrument:

| the thing is | measure with |
| --- | --- |
| in the wrong place, the wrong size | `boxes()`, the layout rects |
| the wrong colour, weight, brightness | `rows()`, the pixels of a screenshot |
| moving when it should not | both, and compare |
| happening at the wrong time | the state, over time, in a loop |

`rows()` in `open.mjs` reads a screenshot's pixels back, averaged per row,
so it answers about change down the picture. It showed the waveform at 117
of 255 rather than 227, every bar straddling two pixels. A vertical edge
needs the columns instead.

**Two boxes can line up while what is in them does not.** The box is not
the ink. Three names reported the same right edge to the hundredth of a
pixel, and the one too long for its box ended four pixels short on screen,
because `text-overflow: ellipsis` fits whole characters. Whether two
things line up is a question about painted pixels: screenshot the box,
find its darkest value, and walk in from the right to the first column
well above it.

**While anything is gliding, the rect is the animation and not the value.**
Mid transition an element reports where it is painted, not where it was
sent. `style.left` is what the interface decided, `getBoundingClientRect`
what the browser has drawn so far: the transcript's edge seemed to move
after a pause while its style held 142px. Measure after the glide's
`transitionend`, never after a pause meant to be long enough, or read both
and say which you mean.

**A moving target cannot be clicked at a point worked out a moment ago.**
`page.mouse.click` missed the range picker's mark every time while the
transcript grew. To test the handler rather than the hit area, press the
element itself, `el.click()`.

## Prove it, do not claim it

Three things, in this order, every time.

**Reproduce it first.** A bug you cannot make happen is a bug you cannot
know you fixed. When the harness cannot reproduce it, say so rather than
imply a fix: the raster shift that made the info marks step never
reproduced in headless Chromium, and only the cause being gone could be
proved.

**Put the old code back and measure again.** A number that changes when
the fix is removed is a fix, one that does not is a coincidence. It is
also the only thing that catches a probe that proves nothing, against a
stub that cannot reach the broken state: twice in one session a fix looked
proved and reverting showed the harness had never produced the state. Run
every probe against the old code once. A probe that has never failed is
not yet evidence.

**Check against what the user actually said.** They report precisely, and
the pattern is evidence. The mark stepped beside the captions, the clip
list and the range picker, not beside New clips, the clip timeline or the
video preview, and the ones that stepped sat on a fractional pixel row.
That found the cause faster than reading code.

## Two things of the same kind look the same

The head of a settings group and the head of the clip list are both the
head of an area, so they share size and weight. A difference reads as a
hierarchy that is not there, and the user sees it at a glance. Before
adding a size, a colour or a spacing, find the sibling and take its.

**An animation used in two places is one component, never two that look
alike.** Work in hand is `Busy.svelte`: the beam, the motes, the fill with
its head, the glow ahead of it and the light passing over what is done.
The range picker's own copy of the fill lacked the glow and the motes, and
the user saw it at once. A place that shows work puts `Busy` in it,
`rim={false}` where there is no edge to run round, and how work looks
changes once, in `Busy.svelte` and `app.css`. The same for the shimmer
and the pulse.

**A handover the eye sees as one thing is one row.** A card on its way and
the clip it became were two rows of a keyed list, the card held by an
effect a render late, and the user saw the whole list blink. Counting rows
every frame passed. Logging `scrollHeight` on every forced layout, through
a patched `getBoundingClientRect`, caught the list a row short inside one
update, `animate:flip` having set the leaving row absolute. When a thing
changes what it is, keep its key, and hold what must not go in the same
pass that builds the list.

**A row that work fills is filled where it stands.** A search's cards came
in between rows at their moments in the episode, and `animate:flip` slid
the rows under them down each time, so the stack shuffled. A running
search keeps its rows by place: each clip comes into the row saying what
the search is doing and stays until the search ends, see `places` in
`ClipList.svelte`. Prove it by following each row's element every frame, a
`WeakMap` from element to number, and counting rows that move: the old
code moved rows on 112 frames of one search of six, the new one on none.

## When the window itself looks wrong, measure it against a real one

Every other native window is on the same screen, a control group for one
screenshot. The title bar "looked wrong" for round after round of tuning,
and was fixed in one change once it was put next to Terminal.

**Take one screenshot holding your window and a reference window, and fix
the scale with an object of known size.** A macOS traffic light is 14
points across and in every window: 28 pixels wide means a 2x shot. Then
"does it look right" becomes a table:

| | down from the window top | in from its left |
| --- | --- | --- |
| ours | 26.0 pt | 26.0 pt |
| VS Code | 17.0 pt | 18.0 pt |
| Terminal | 16.0 pt | 16.0 pt |
| Chrome | 20.0 pt | 20.0 pt |

Six to ten points out both ways, on every window: the whole bug, invisible
until written down.

**A constraint in a comment is a claim. Check it before building on it.**
`App.svelte` said nothing the app sets moves the three buttons. False:
Electron apps and VS Code move them, and `chrome_darwin.go` already reads
the frame of `standardWindowButton:`, so setting it is the same call the
other way round. Measuring, centring and rounding grew on that premise,
none of it needed. The cause was `UseToolbar: true` inside
`MacTitleBarHiddenInset`: a toolbar makes the bar taller, and macOS
insets the buttons to centre them in it.

**Adapting has no end condition. That is the smell.** When work around
something you cannot change keeps growing, stop and test the thing
everybody agreed was fixed. A cause has a fix, a symptom endless
mitigation.

**Asking for more opinions does not help while the premise is wrong.**
Every briefing carries the framing, so every answer comes from the same
box. A counterexample beats reasoning: the user asked how VS Code does it.

## When it only goes wrong on the Mac, try WebKit

The app on a Mac is WebKit, and where it keeps time or paints unlike
Chromium, a Chromium probe passes on a bug: a removed row's fill grew back
under the pointer for two rounds, as WebKit plays a paused animation on
from its timeline's last frame and Chromium from now.

Walk a sequence in Playwright's WebKit with `WALKBROWSER=webkit` in front
of `node frontend/preview/walks/sequences.mjs`, see Two browsers in
`docs/TESTING.md`, and a probe of `frontend/preview/open.mjs` the same
way. A cloud session installs Playwright's WebKit in the background as it
starts, see `/tmp/framefairy-webkit.log`.

For a probe outside the walks there is also the system's WebKitGTK 6.0,
with Python bindings for `/usr/bin/python3.12` (not the default
`python3`), `xvfb-run` for a screen and `libXtst` for a real pointer.
Serve `frontend/preview/dist` over http, open it in a `WebKit.WebView` in
a `Gtk.ApplicationWindow` with `set_decorated(False)` under
`GDK_BACKEND=x11`, move the pointer with `XTestFakeMotionEvent` from
`ctypes`, and read the page with `evaluate_javascript`. A synthetic event
from inside the page does not set `:hover`, an XTest pointer does. Read
what is on screen, a transform or a box, not an animation's progress.

## What goes wrong here in particular

Each of these cost at least one round of the user's testing.

**A layout that measures itself never settles.** The workspace once sized
the picture from the clip timeline while the picture decided the
timeline's column: several frames per resize, a laggy window edge. Every
size is one stylesheet expression, see CLAUDE.md.

**A fraction of a pixel is a bug waiting.** An element on a fractional row
moves once opacity, a transform or a filter puts it on its own compositing
surface, which starts on a whole pixel. Give rows whole heights, and
elements their own height, not their line of text's.

**Fractional drawing on a canvas is not a rounding error, it is half the
colour.** Work in device pixels, one column at a time, and round.

**An effect that mentions a job is rebuilt about once a second**, as every
job event replaces the job object, so a timer inside one never fires. See
CLAUDE.md.

**Answers do not come back in the order they were asked for.** Anything
async that paints needs a ticket, and an answer older than the newest one
used is dropped: `Newest` in `frontend/src/lib/flow.ts`, with tests.
Arrowing through the clip list puts several frame requests in the air, and
without it the picture settles on the slowest.

**Do not skip an ask by comparing against what was last asked for.**
Compare against what is on screen. Going A, B, A quickly skips the last A,
and B's picture stays.

**The box that is trimmed is the box that is clipped.** `text-box:
trim-both cap alphabetic` centres a name by making its box the letters,
and `overflow: hidden`, for the ellipsis, then clips every tail below the
baseline, the y of `YouTube.mp4`. Trim, then pad back out evenly, so the
middle stays and clipping happens at the padding.

**A number that stands for a position must not be published as zero.** The
bar's box is twice the window buttons' middle, fullscreen takes the
buttons away, and a middle of 0 left the box no height. Leave the
measurement out, so the stylesheet's fallback answers.

**A drag takes the pointer, and with it every click.** The clip timeline
and the range picker call `setPointerCapture` on the way down, so every
click and double-click goes to them and a child's handler never runs: the
double-click that puts a cut back is decided on the track, by pointer
position. What answers a click on a drag surface is decided by the surface
or stops the pointer going down.

**A drag that refuses the pointer refuses the caret with it.**
`preventDefault` on the way down stops a text selection, and also the
caret landing where a caption word was clicked. So a drag starting on a
caption word does not refuse the pointer, and lets the word go once the
hand moves. With the old line back the word never takes the keyboard.

**Two things that move together must move by the same means.** An animated
`left` or `width` runs on the main thread, a `transform` on the
compositor, two clocks, so side by side one steps while the other glides.
The user saw it beside the transcript's edge, and an idle harness shows
it: the line still on 6 frames of 240, the mark on none. The range
picker's fill and the shade ahead of it both move by transform, with the
same glide.

**Nothing checks that a call reaches its method.** The window calls the Go
side by name and position, and TypeScript knows only `api.ts`, so
`main.FrameFairy.CutClip expects 7 arguments, got 6` was found by hand
with every test green. `cmd/framefairy-app/bindings_test.go` now parses
both sides and walks the boundary. A new argument goes in `api.ts`, in
whatever passes it down, and in `wails-stub.ts`, or the harness cannot
reach it.

**A mark that stands for a moment is wider than the moment.** A handle is
twelve pixels wide and pulled six back, so a probe pressing at
`getBoundingClientRect().left` cut a sixth of a second off. Take the
middle of the box.

**Decide by the state the playhead is in, not by how far it is from
something.** Which word, which piece. A distance can round back into the
answer it started from, a key that cannot get out. Five bugs in one
afternoon, from a dashed crop frame to stepping back stuck in a word, came
from comparing the playhead to a moment while the `<video>` element
answered a seek with the shown frame's start. A moment through two clocks
still comes back a hair off, see below, and Playback is the clearest
case.

**Read the source before building on what a browser is said to do.** A
playing `<video>`'s clock in WebKit was taken for one that steps back, and
a forward-only rule froze the playhead. It is an estimate that never goes
back and runs ahead of the picture, `TimeProgressEstimator` in
`MediaPlayerPrivateRemote.cpp`, four lines.

**A floating thing is measured before the stylesheet has finished with
it.** The placement picks which way a list grows by measuring before the
rules run, so `width: var(--the-anchor-width)` with `min-width:
max-content` is placed at the anchor's width, comes out wider, and spills
the other way. Give it a width the placement does not write,
`max-content`. It took three rounds of testing, because in Chromium a
ResizeObserver corrects the second pass, and the user saw the first.

**A floating thing is read whole, or it was not placed.** The clip
timeline's info bubble fitted neither over nor under its mark and ran off
the foot of the app, while probes at a comfortable height passed. Open every floating thing at several app sizes, small ones
above all, and compare its box with `innerWidth` and `innerHeight`. For
clipping, `scrollHeight` above `clientHeight` counts only where `overflow`
is not `visible`, or a hover bridge past the box reads as hidden text.
`placeBubble` in `lib/bubble.ts` is the rule, with tests that put a mark
everywhere.

**Two things that show the same number must read the same number.** The
row waiting for the transcript read the saved transcript, the range
picker's line what was heard, seconds ahead, so the line passed the
window with the row at 95 percent. And the search waited on the written
transcript while the line showed the heard one, so the two parted the
moment either stopped. Give every place that shows one fact the same
value, and make the work it stands for wait on that value too.

**One line of text, one reporter.** ffmpeg framing a clip flicked the
search's row to "finding camera switches" twice a second. Work that owns
a line holds it (`Log.HoldProgress`) until done.

**A person reads slower than the engine reports.** A new headline stays
2.5 s, a count going up 1 s, while the fill and the time left move at
once. A time left moves in steps of
five seconds. A machine step the person does not act on, loading the
model, reading, thinking, is not a headline.

**A colour the person chose never marks a state of the interface.** A
caption block marked in the app's colour vanished on a purple caption,
and a dimmed colour is just another colour. Show the chosen
colour as chosen, and tell a state by what cannot clash: brightness,
size, a pill in the highlight colour where the short uses one. Try it
against white, a strong green and a colour near the app's.

**A moment that goes through two clocks can come back a hair early.** A
caption's start, taken from the clip's clock to the episode's and back,
landed before the caption, and a click on it lit no word. A seek meant to
land in something lands a frame into it, `intoWord`, as the arrow keys do.

**Stale state waits to be shown.** The `<video>` video preview drew its
last still whenever a seek looked stuck, so a frame from anywhere flashed
up. Show anything kept for later
only for what it was made for. The probe passed against broken code until
it loaded a still the way a busy machine does: reproduce the state before
trusting a probe that says it is gone.

**Contrast is judged with daylight on the screen.** A fill of 7 to 22
percent of the accent read as none by a window. A fill is 30 to 50
percent, and a state told apart only by a shade of dark grey needs a look
in a bright room.

**The rail is part of the app.** Between the rail and the workspace is
`--gap`, and only the side on the app's own border is `--edge`. A 24 pixel
edge where a 12 pixel gap belonged cost the settings column its room.

**A row still to come is already a row.** The clip list's placeholders
brighten under the pointer like a clip's row while they breathe.

**Paused is a state of its own, and it is still.** A pause once hid the
range picker's fill. Stopped work keeps its fill, `Busy still`, and loses
what moves. Before drawing any work,
name its states, running, paused, waiting, done, failed, and look at each
in the harness.

**Prove a fix on the path the person takes.** A pause passed with the Go
wait called directly, but the interface never reached it. A probe of what
the app does by itself starts where the person does, adding the episode,
and measures what they see, here the edge passing the window to the
search being asked for, with a stub like the real thing:
`?growing&lagging` saves every 8 s, as the engine does.

**A focusable thing keeps the focus a click gave it.** A caption block
with `tabindex` took focus on a click, and after the first arrow key a
`:focus-visible` ring stayed round a caption the video preview had left.
An element the keys never reach takes no focus. Probe by clicking,
pressing a key, and listing what matches `:focus-visible`.

**The same kind of thing is one rule.** The crop, the window and the clip
had three 2 pixel borders with three corners, now `.frame` in `app.css`.
Before styling something "like" another thing, share its rule.

## Playback: on the clip or on the video

For playing, the playhead is in a state, **on the clip** or **on the
video**, set by the gesture that put it there and nothing else, not a
clock, not the queue's position, not a rounding. The rules are `placeOf`
and `playFrom` in `frontend/src/lib/playhead.ts`, with tests.

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
the clip's first frame it plays from the start, and from the end it starts
over, as QuickTime does.

**On the video**, the space bar plays the episode straight on, through the
chosen clip and its cuts, and stays on the video. The chosen clip is then
dimmed wherever drawn, `.frame.dim`, `.clipmark.selected.dim` and
`.chosen.dim` in `app.css`, in the frame the state changes, so a person
sees the clip's rules are not in play.

**The whole clip dims, as one thing.** Bright caption blocks in a dim
frame read as a clip half in play. Everything drawn for the chosen clip on
the clip timeline, its frame, pieces, cuts, trim edges, caption blocks and
thumbnails, sits in `.chosen`, which lays nothing out, and one rule dims
it with the crop frame and the range picker mark, `opacity:
var(--dimmed)`, a disabled button's brightness, so the caption colours
stay the person's. The waveform is the episode's and does not dim. It is
lit whole while the hand is on the clip elsewhere. A new part drawn for
the chosen clip goes inside `.chosen`, or it stays bright. Prove it by
reading every part's opacity, the product down its ancestors, on the
clip, on the video, and on the first animation frame after the click.

**While it plays, the playhead is the sound being heard.** On every
animation frame the frame queue reports where the program is by the sound
card's clock, `heardAt` in `lib/frames/plan.ts`, placed in the episode
through the program, cuts included, and draws the frame for that
position. So the playhead moves every frame, jumps a cut with the sound,
sits at most a frame ahead of the shown frame's start, and never goes back
while playing. A play or a seek while playing reports its start at once,
so the playhead is at a click in the click's frame and waits there, the
new first frame on the canvas, for the first sound, see The start below.

**A click while it plays lands where it was clicked, every time.** Play
goes on from the click, cuts jumped inside the clip, straight on outside.
Clicks across the clip's edge were lost: the new program restarted the
play from the old moment, the listener set the playhead there, and the
seek after it read the playhead and went back. So `seek` takes both,
`queue.seek(at, program)`, and the play starts once. Never read a value a
call can have changed through a listener: keep the moment in a local.
And a click sent its moment on the way down and up, so with the hand held
150 ms the second sent the playhead back. `scrub` now sends nothing on the
way up where the hand has not moved since the last seek, and the queue
takes a seek to the moment a play is starting from as that play.

The proof, with the clip of three pieces: play, then click the clip
timeline 17 times, out of and into the clip in turn, some 120 to 300 ms
apart while the play starts, some a second apart. Read `data-playhead`,
the bars and `.chosen.dim` every animation frame, and check per click: on
the click two animation frames after the press, then following its own
play (on the clip's program, so look-ahead before a cut is not the cut),
never back, nothing played inside a cut or jumped outside the clip, the
dimming the place's. Before: the first
eight clicks, all across the edge, stayed where the play was. After: 0 of
17 broke the rule, with the hand held 150 ms and with `?slowread=200`.
With only the first half of the fix, every held click went back once.

Paused, the playhead is exactly where it was put, or where the sound
stopped, and the picture is the frame holding it, nearly always decoded
already.

The proof, with a clip of three pieces whose cuts are off frame edges,
set through `window.__pieces`: press the space bar or the play button and
for nine seconds read every animation frame the playhead, `data-playhead`
on `.screen`, and the frame number in the bars.
Both cuts take one frame's time, 33 or 50 ms, no frame from inside a cut
is drawn, none stays over 50 ms, the playhead never stands still after it
first moves nor goes back, it is under a frame from the shown frame on the
program, and it stops paused at the clip's end on its last frame. The
same with `?slowread=200`, with loop on across the seam, and played again
from the end. The tapped sound is the episode's own to the sample across
both cuts, best lag 0, and nothing is heard past the end.

On an edge exactly, the side the gesture is about decides. Both edges are
the clip's: from the end edge the space bar starts the clip over. A trim
puts the playhead on the edge it drags, maybe a hair outside the pieces
the video preview has, so it says it is about the clip instead of being
measured.

Loop is about the clip and changes no state: on the video with loop on,
the space bar plays straight on. A clip chosen while the video plays
leaves the playhead on the video, as no gesture put it on that clip,
except one made there with I or O, around the playhead: making it is the
gesture, `claim` in `Player.svelte`, or it comes up dimmed over the moment
it was made from. The play goes on as it began, and the next plays the
clip.

**Where a gesture lands is a frame, not a second.** Compared in seconds,
one step right of a clip starting inside a frame landed on that frame's
start, before the clip, and the clip dimmed with its own first frame on
screen. So `placeOf` takes the frame rate and asks which frame the gesture
landed in, the one `insideClip` draws the crop in. The frame holding the
clip's start is on the clip, the frame holding the moment a frame before
its end is its end. One rule says which frame is meant, `frameAt` in
`lib/flow.ts` and `rankAt` in `movie.ts`: the frame holding the moment,
where up to a millisecond before a frame's start counts as that frame,
`FRAME_HAIR`, as the app keeps moments to the millisecond. The engine
keeps every edge on the start of the frame the render cuts on,
`PieceOnFrames` in `engine/render.go`, so for an edge this is the short's
frame, and `frame.cases.json` holds both sides to each other. A second
rule for edges, the nearest frame, disagreed for about half the edges a
search left between frames. A trim of the end on frames, leaving the
playhead a frame before the end, is then at the end.

**Why it is a state.** It was a distance, within half a frame of the
start and before the end, and with the `<video>` element, whose playhead
was never where it was put, it broke three times: a seek before every
play, as the paused picture answered a hair before the clip, the episode
played from its start after a search, from a clock still at zero, and on
the Mac straight through a clip's cuts, from a paused video answering up
to a frame early. A wider tolerance and a hold on the playhead only moved
where the rounding went wrong. A gesture knows what it is about. A clock
only says how far it is from something, and that never settles.

## The frame queue

The video preview plays frames the app decodes itself, in
`frontend/src/lib/frames/`. `movie.ts` builds what it plays from the
frames the engine names, the grid the source carries, and reads nothing
of the episode file. `plan.ts` decides everything, with tests. `queue.ts`
is only glue to the Go side's frames and sound, the canvas and the sound
card. `Player.svelte` makes one `FrameQueue` per episode and drives it
with `setProgram` (the clip's pieces, or null for the episode straight
on), `seek`, `play` and `pause`. What the queue reports, `Shown`, is the
playhead, and nothing else sets it while playing.

The queue's own harness page, `preview/frames/`, one canvas and
`window.__queue`, is the quickest way to look at it alone. `frames()` in
`open.mjs` opens it on a ten minute episode by the same recipe, and
`queueUntil` waits for a queue report. `?read` reads the frame number back
from the canvas. `?hear` records every sample handed to the sound card, to
match against the episode's sound decoded by ffmpeg, cut and faded as the
render does, so a sample missing, doubled or from inside a cut shows. The
workspace measures the same on the person's path, see Playback.

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
| the sound across a cut | the episode's own to the sample, best lag 0 | the same |
| the loop seam | the same as a cut | the same as a cut |
| paused frames | the frame that holds the moment, 11 of 11 | 14 of 14 gestures, 3 to 25 ms |
| frames open over a minute of play | 4 to 5 | |

A frame lasts 40 ms and the display draws every 16.7 ms, so one frame's
time shows as 33 or 50, and a gap above 50 is a frame held.

**What it taught.** Each cost a round of probing:

- The Go side decodes the sound with ffmpeg, `GoSound` in `queue.ts`, not
  the browser, so the video preview hears what the render is made of and
  the Mac's AAC decoder, which the cloud cannot run, is out of the
  picture. When the browser decoded, Chromium stamped sound with its own
  times, so it was matched to packets by order, and Opus cut its pre-skip
  twice.
- A flush of the sound ends a run where it was asked, between the packets
  fed before and after it, not once everything fed is out. A decoder that
  read on into the next run's packets played the sound before the second
  cut after it, 139 percent off, where the fixed one is 0.00.
- A Chromium AudioWorklet sometimes gives two render quanta the same
  `currentFrame`. The recorder labels the second the last one plus 128, or
  the check fails on right sound.
- `getOutputTimestamp` is the heard sound, but a stamp from before the
  sound card was held counts on through the pause, and WebKitGTK without a
  sound device stamps on another clock. `heardAt` takes a stamp only when
  fresh and near the clock.
- The first frame of a play stays until the sound is heard, see The start.
  That is the picture keeping to the sound, not a hold.
- A paused seek cues the play with the paused frame first, so the space
  bar only starts the clock instead of decoding the same group of pictures
  again. Where the paused frame is not the play's first, in a cut or past
  the clip's end, it is decoded alone.
- A paused seek to a moment in the frame already on screen draws nothing,
  so it must report anyway, or whatever waits for it waits for ever.
- Frames drawn on a grid from where the play began came up to a frame late
  and skipped a piece's last frame, which the playback walk caught at five
  frames a second. The grid is the episode's own frames, each drawn when
  the heard sound reaches its start, carried onto the program, `VideoPlan`
  in `plan.ts`. A piece draws from the frame holding its start, or where
  the play began, to the frame holding the moment a frame before its end,
  the frames the render cuts, and the tick never steps past either without
  drawing it for one animation frame, `nextEdge`. A frame at a piece's edge
  can begin 10 ms before the cut and is then due with the one after it.
  The proof plays the clip from 57.000, 57.001, 57.013, 57.020, 57.039,
  59.213, 59.239, 61.731, 62.013, in the cut at 60.000 and from the end,
  reads every drawn frame back and finds the first animation frame its
  start was heard on: 0 frames skipped, none over one animation frame
  late, with `?slowread=200` too. The old grid skipped 1 to 3 frames in
  each of the 11 plays, 60.28 and 62.16 from the clip's start, and drew
  frames up to 3 animation frames late.
- Plain sound in a MOV counts every sample as a moment, hundreds of
  millions in four hours. It is read by the chunk and turned into the
  sound card's numbers without a decoder.
- HE-AAC decodes at twice its core's rate. Where the file does not say so,
  the rate of the decoder's first sound is taken and the play restarts at
  it, before anything is heard.

**The start.** From the space bar to the picture moving, from a pause
after a click, in the harness:

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

The sound card stays held after a pause's fade: running it would save
only the `resume()` and cost a laptop power. `START_LEAD` puts the first
sound ahead of the card's clock, since sound put where the card has
rendered already starts late and out of step. The harness played to the
sample from 2 ms, but the Mac's card renders 512 to 1024 samples at a
time, 11 to 21 ms, and 25 ms covers that with room. Measure the parts with
a wrapper round the queue's `maybeBegin` and its sound card's `resume` on
the queue's page, and press to motion in the workspace from the bars on
every animation frame after a space bar `keydown` sent in the page.

The `<video>` element was left because a cut was a seek: each gap between
pieces held the last frame, a fifth of a second and more on the Mac, and
stopped the sound. No workaround made a cut take one frame's time.

**WebKit, but not the Mac.** WebKitGTK, see When it only goes wrong on the
Mac, try WebKit, runs the page too with `gstreamer1.0-plugins-bad` and
`gstreamer1.0-libav`: VP9 needs `vp9parse`, H.264 needs `h264parse` and
`avdec_h264`. There an MP4 of H.264 with B-frames and AAC played every
frame in order, cuts one frame's time, but that is GStreamer. The Mac's
WebKit decodes with VideoToolbox and AudioToolbox, and only the user's
Mac shows how those order output, stamp sound and report latency.

## A click is answered in the frame it lands in

Continue left the Stopped row empty for six frames before Transcribing,
and Cancel said Cancelling at once while the row said Transcribing for the
second the search took to save. Every test was green: the end state was
right, the in between wrong, and the in between is what a hand feels.

The rule: **everything a click changes shows the change in the same
frame**, the control and every place showing the same work, the row, the
fill, the range picker. Say what is under way, Stopping, Starting, the
step it goes into, and stand still where it got to until the Go side's
answer replaces it. Never wait for a job event or a timer to show what the
click already knows. Only a change the engine makes by itself may wait, so
a step that lasted a moment does not flash.

A screenshot never catches it. Click inside `page.evaluate`, read the
thing on every `requestAnimationFrame` for a second, and print the runs of
frames that looked the same:

```js
const frames = await page.evaluate(() => new Promise((done) => {
  const seen = [], row = () => document.querySelector(".ghost.next")?.textContent.trim() ?? "(none)";
  document.querySelector("button.search").click();
  let n = 0;
  const tick = () => { seen.push(row()); if (++n < 60) requestAnimationFrame(tick); else done(seen); };
  requestAnimationFrame(tick);
}));
```

A run of `(none)`, or of the state before the click, is the bug. The stub
must take as long as the real thing: the probe passed against broken code
until the fake search waited 800 ms to stop, as the engine does.

## A drag reshapes everything drawn from it

The click rule, for the length of a drag. Dragging a clip edge moved the
edge and its wash, while the caption blocks waited for the hand to let
go. Tests looked at the end, the user at the middle.

When a drag changes a thing, list everything drawn from it and make each
follow the draft, not the saved state. When the engine works it out, like
a clip's captions, ask it on the way, one question at a time, always about
where the hand is now, and keep the last answer until the saved state
returns, so nothing jumps on release. `Shape` is the example.

The list is easy to make one short: a line once followed a dragged edge
while the marks it ran between stayed put. So the engine answers the hand
with everything drawn from the thing, in the shape the saved state is
sent, and every part is drawn from that one answer. `Framing` is the
example: pieces, marks, line and crop in one answer, for a saved clip and
one under the hand.

Catch it with `page.mouse.down()`, moving in steps and reading every part
of the list after each, before `page.mouse.up()`. A number that changes
only after the up is the bug. Then read every frame across the up, as for
a click.

## Work a click starts is built in front of the person

Answering the click is not enough when the work takes seconds. Pressing I
showed nothing for ten seconds, the card far down the list, the clip
timeline waiting, then all of it at once. Tests passed because the stub
made the clip in a second, where the list was already looking.

The rule: **what a click asks for is shown where it will live from the
moment it is asked for, and filled in as each part of it becomes known.**
The control wears the beam. What it makes is chosen and brought into
view. What is known, where it lies, how long, its pieces, its captions, is
drawn where the finished thing will be, the clip timeline above all. The
finished thing replaces it with no jump. Nothing appears whole after
seconds of nothing.

Check it far down the episode with the list scrolled elsewhere, and a
stub as slow as the app, the speech model loading included.

## A list that floats keeps what it hangs from in place

Every list is `Pick.svelte`, hung on its trigger's edge, which holds only
while the trigger stays put. The list is drawn over the whole app, so to a
container that opens on hover, moving onto it is leaving: the sidebar's
sort menu closed the sidebar on the Mac as the pointer reached a row, and
it flashed open and shut with no row pickable. The probe passed because
the harness's sidebar was pinned.

The rule: **a container that opens on hover stays open while a list or a
bubble of its own is open**, which is what `onopenchange` on `Pick` is
for. Check it unpinned: hover it open, open the list, move onto a row step
by step, and read the container's width at every step.

**And after the list closes, it asks where the pointer is.** The sidebar
trusted a stale pointer when its list closed, so Escape or a click closed
both. It stays open and lets the next pointer move decide, and Escape
closes one layer per press.
Check every way a list closes, a pick, Escape and a click, with the
pointer on the trigger, on the container and outside it.

## Before saying it is done

- The walk for the fix fails on main and passes with it, written as the
  testing skill in `.claude/skills/testing/` says: never waiting on the
  clock, checking what the user saw.
- `make interface`, the type check and the interface's own tests.
- The walks run in CI on four machines, and that result is the walk's
  proof, see CLAUDE.md. A single walk runs alone in the session only to
  write or fix that walk.
- Build the preview, and the motion bench if the change touches any of the
  five ways work in hand is shown.
- Push once those pass and let CI run the rest, and hear how it went from
  its events. Rerunning every walk in the cloud keeps the user waiting.
- Say what to look at and what should happen in the app, in the order the
  user would do it.
- Name what you could not verify. A fix reported as certain and found
  broken costs more than one reported as unverified.
