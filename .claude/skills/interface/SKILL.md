---
name: interface
description: How to change the interface of the Frame Fairy app when you cannot see its window. Use for any work on frontend/ - a layout, a control, a colour, a drawing on the clip timeline or the range picker - and above all when Tim reports something that looks wrong. Covers the preview harness, what to measure, and how to prove a fix rather than claim one.
---

# Changing the interface without seeing it

A cloud session has no screen, so the app's window cannot be opened. Tim is
the only one who can look at it. That is the whole problem this skill is
about: without care, a session guesses, reports a fix, and Tim finds the
same bug still there. That has happened, more than once, and each time it
cost him a round of testing.

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
const { page, stop } = await workspace({ query: "?found", scale: 2 });
// ... measure the one thing this is about ...
await stop();
```

`workspace()` opens an episode, puts the sidebar away, picks a clip and
steps the playhead onto its first word. That last step is not tidiness: a
clip begins a little before the first thing said in it, the way the engine
cuts it, so a playhead on the clip's own start is in silence and there is
no caption box on screen at all. Every probe about the captions would be
measuring an empty picture.
`screen()` is the same window with nothing clicked, for anything that is
not the workspace: the first run, the settings, the empty window.
`query` tells the fake Go side what to pretend. The modes are in
`frontend/preview/wails-stub.ts`, and today they are:

| mode | what it pretends |
| --- | --- |
| *(none)* | a finished episode with clips and two searched stretches |
| `?busy` | a search finding, with progress |
| `?unknown` | the same, with progress it cannot put a number on |
| `?transcribing` | a search hearing, reporting ahead of the saved transcript, and never getting further |
| `?growing` | an episode just added: its first search, started by the Go side, hears from 10 minutes to the end of its window and then finds, with job events every 250 ms |
| `?lagging` | with `?growing`, the saved transcript trails what is heard, saved every 8 s of work the way the engine saves it |
| `?hear=100` | a search hears 100 seconds of audio a second instead of 600, so the hearing lasts long enough to look at |
| `?paused` | clips found, the episode read part way, nothing reading the rest. `?short` reads it only to 2500 |
| `?found` | New starts a search that really finishes, clips and all |
| `?interrupted` | a search whose record says it was cut off while it found. `?waiting` cut off while it heard, at 15 minutes. Cancel on any running search leaves it stopped, the way the Go side does |
| `?failed` | a search whose record says it failed, with its reason |
| `?rendering` | a render running on the first clip, with progress |
| `?measuring` | the episode's loudness, the waveform, measured from the moment the page opens, the four hours in twelve seconds, with a levels event every half second. Like the Go side it measures what the clip timeline last asked the waveform of first, then on from there, then from the start. `?unmeasured` never measured, an episode added before the measuring existed, so the waveform is the transcript's alone |
| `?slowhand` | a clip made with I or O takes four seconds to place its crop, and four more to be heard first where the transcript does not reach, so a probe can look at its card on the way. Without it, a second and a bit |
| `?lagclips` | every clip list comes back 300 ms late, the way a busy machine answers, so a card on its way has to hold its place until the list has the clip it became |
| `?asked=4` | with `?growing`, the first search the Go side starts by itself asks for 4 clips, not what the workspace would suggest for its window, so the rows and the target have to follow the search |
| `?crossclips` | one clip list read comes back 400 ms late and the next at once, so a read asked later answers first. A card on its way and the clip it became must never both be on screen |
| `?setup` | a machine with nothing on it, so the first run is the window. Both model installs really run and really finish, on their own clocks, and one language model fits the machine it pretends to be while the other does not |
| `?refuse` | an engine that says no to an edit. Correcting a word and picking a caption face both fail, which is how to see what a control shows once the answer is no rather than yes |
| `?slowread=200` | every range of the episode is answered 200 ms late, the way a busy disk or a long episode can, in the workspace and on the frame queue's page alike. The frame queue reads ahead so a cut never waits on the file, and this is how to show it, see The frame queue below. It is `open.mjs`'s server that is slow, not the stub |

Add a mode when the state you need is not there. A bug that only happens
while something is running cannot be found in a stub that is never busy:
the first search never starting was invisible until `?growing` sent job
events the way the Go side does.

Every search in the stub is one fake search that goes through the steps
the Go side goes through, from the same kind of record: hearing, then
finding, its clips landing one at a time. What the interface asked for is
on `window.__searches`. A mode that needs a search in another state
starts it there, rather than making up a job of its own.

**The job list is only ever brought up to date by events.** It is read once
at startup and changed after that by nothing but `onJob`. So a mode that
sends no events can never show work starting or stopping, whatever its
`Jobs` call answers: cancel a job in it and the window goes on holding a
job that is still running. Two probes were written against that and passed
against broken code before it was noticed. A mode meant for anything that
starts, stops or reports has to send events, and the numbers in them have
to be the ones the real side sends. `?growing` reported a fraction but
never a `covered`, so the transcript's edge never moved from the work at
all, and the first probe about pausing was measuring the track filling in
after load.

**What the stub answers with has to be made the way the engine makes it,
not made up.** Three phantoms came out of this one, and every time the
made-up answer looked perfectly reasonable in a picture. Its caption cues were four words written out by hand, which
looked right in a picture and could answer nothing: the words in the
caption box are clicked back to the word of the episode they came from,
through the clip's own pieces, and words that came from nowhere have
nowhere to go back to. A probe about correcting one would have passed
whatever the window did. The cues are built from the words the clip's pieces hold now, on
the clip's clock and with a split word drawn as two, the way the engine
builds them.

Then the words themselves were made from wherever a call asked to start,
so a call about ten minutes and a call about a clip answered with words at
different moments. Both read the same sentence and both drew fine, and the
two only disagree once something crosses from one to the other, which is
exactly what stepping by words does. And the cues were allowed to overlap,
where the engine clamps a cue to the start of the one after it: the window
takes the first cue that covers the playhead, so the box went on showing
the cue before while the playhead stood in a word of the cue after, and
that word lit nothing. One list, and the engine's own rules on it.

`frontend/preview/dist/` is build output and is not in the repository.
Write one-off probes outside the repository, in the scratchpad.

## The episode the harness can play

The video preview reads the episode file itself, see The frame queue
below, and the harness serves it a real one, made with ffmpeg on the first
run and kept in the temp folder: four hours at 25 frames a second, 96 by
54, VP9 and Opus in an MP4, about 45 MB, made in about four minutes. Without
ffmpeg there is no episode, the video preview says it cannot read the file,
and a probe that needs one has to say so rather than pass.

Three things about it are deliberate.

**It is VP9 and Opus, not H.264 and AAC.** The Chromium that comes with
Playwright is built without the proprietary codecs. The app itself runs in
a WebKit view and decodes what the Mac decodes.

**The picture says which frame it is.** Its top half is the frame number
in 24 bars of four pixels, the highest bit on the left, light for one, so
a probe reads back from the canvas the frame really drawn and never trusts
what the app says it drew. Read the bars from `.screen canvas` with
`getImageData`, a row through the middle of the top half, in the middle of
each bar: the frame is scaled up smoothly and the edges of a bar are a
blend. The bottom half is a grey that climbs from the first second to the
last, because black and no picture look the same: around 28 is a picture.

**The sound says where it is.** A tone whose pitch climbs with a little
noise under it that never repeats, so what the sound card was handed can be
laid over the episode's own sound, decoded by ffmpeg, sample by sample. The
sound card is the app's own `AudioContext`, so a probe taps it from an init
script: a subclass of `AudioContext` that adds an `AudioWorklet` recorder,
and `AudioNode.prototype.connect` patched to send whatever goes to the
destination to the recorder as well. Where the program starts in the
recording is found by fitting it, a few thousand samples either way.

`open.mjs` exports `episode()` and `episodeAt`, and `framesEpisode()` and
`framesAt` for the frame queue's own page, made by the same recipe.

**A page error is a result.** Attach `page.on("pageerror", ...)` in any
probe that plays or edits while playing. The old player's frame loop threw
`Cannot read properties of undefined (reading 'end')` when a cut was put
back while the clip played past it, which ended the loop for good, and the
throw was the only clean signal of it: everything else went on looking as
if it played.

## The motion bench

`make motion` opens every way the app shows work in hand on one page: the
beam and its motes, the fill in a control and as a track, the shimmer and
the pulse, each at every share and in every kind of control, with a colour
picker so the whole set can be seen in another accent. It is the fastest
way to look at a change to any of them, because nothing has to be started
and nothing has to be caught at the right moment.

It is built from `frontend/preview/motion/` by
`frontend/preview/motion.config.ts` and it uses the app's own `app.css` and
the app's own `Busy.svelte`, so what it shows is what the workspace shows.
Build it for a probe with

```
npx vite build --config frontend/preview/motion.config.ts
```

Anything added to the five belongs on that page in the same change.

## Measure the right thing

**`getBoundingClientRect` gives the layout position. It cannot see a
painting bug.** An element painted in the wrong place still reports the
right box. A whole investigation was wasted on this: the info marks were
stepping as they faded in, the rect was read twenty times over, it never
moved, and the conclusion drawn was that the bug did not reproduce. It
reproduced every time. It was never a layout bug.

So decide which kind of bug it is before picking an instrument:

| the thing is | measure with |
| --- | --- |
| in the wrong place, the wrong size | `boxes()`, the layout rects |
| the wrong colour, weight, brightness | `rows()`, the pixels of a screenshot |
| moving when it should not | both, and compare |
| happening at the wrong time | the state, over time, in a loop |

`rows()` in `open.mjs` takes a screenshot and reads the pixels back. It is
how the waveform was shown to be painting at 117 of 255 rather than 227:
every bar was landing across two pixels at part strength. It averages each
row, so it answers about something that changes down the picture. A vertical
edge changes across it, and needs the columns instead.

**Two boxes can line up while what is in them does not.** The rect is the
box, and the box is not the ink. Three names in a list all reported the
same right edge to the hundredth of a pixel, and on screen one of them
ended four pixels short of the other two: it was the one too long for its
box, and `text-overflow: ellipsis` fits whole characters, so whatever room
is left after the last one that fits plus the dots is simply unused. No
arithmetic gets it back and no rect can see it. Anything about whether two
things line up on screen is a question about painted pixels. The last lit
column of a screenshot is the answer, and it is worth writing the probe
that finds it: take the element's box, screenshot it, find the darkest
value in it, and walk in from the right for the first column holding
anything well above that.

**While anything is gliding, the rect is the animation and not the value.**
It is the rule above the other way round. An element part way through a
transition reports where it is being painted, which is not where it has
been told to go, so a number read then says nothing about whether the code
is right. Read the inline style beside the rect and the two answer
different questions: `style.left` is what the interface decided,
`getBoundingClientRect` is what the browser has drawn so far. The edge of
the transcript looked like it was still moving after a pause, with the
style holding at 142px the whole time. Wait for a glide to settle before
measuring, or measure both and say which one is being talked about.

**A moving target cannot be clicked at a point worked out a moment ago.**
By the time the click lands the element is somewhere else, and the click
goes to whatever is there now. `page.mouse.click` on a coordinate missed
the mark on the range picker every time while the transcript was growing.
Press the element itself, `el.click()`, when what is being tested is the
handler rather than the hit area.

## Prove it, do not claim it

Three things, in this order, every time.

**Reproduce it first.** If you cannot make the bug happen, you cannot know
you fixed it. When the harness cannot reproduce it, say so in the report
rather than implying a fix. Headless Chromium is not WebKit: the raster
shift that made the info marks step never reproduced here at all, on any
build. What could be proved was that the cause was gone.

**Put the old code back and measure again.** A number that changes when
the fix is removed is a fix. A number that does not is a coincidence. The
waveform, the transcript progress and the data race were all confirmed
this way, by reverting the change and watching the measurement go back.

This is also the only thing that catches a probe which proves nothing. A
probe written against a stub that cannot reach the broken state passes
either way, and reads exactly like a probe that passes because the code is
right. Twice in one session a fix looked proved and was not, and both times
it was reverting that said so: the measurement did not move, because the
harness had never been able to produce the state in the first place. Run
every probe against the old code once. A probe that has never failed is not
yet evidence.

**Check against what Tim actually said.** He reports precisely, and the
pattern in his report is evidence. When he said the mark stepped beside
the captions, the clip list and the range picker but not beside New clips,
the clip timeline or the video preview, that was six data points: the ones
that stepped were the ones sitting on a fractional pixel row. Five of six
matched exactly and the sixth had the smallest fraction. That pattern
found the cause faster than any amount of reading the code.

## Two things of the same kind look the same

The head of a settings group and the head of the clip list are both the
head of an area, so they are the same size and weight. A difference
between two things of the same kind reads as a hierarchy that is not
there, and it is the kind of thing Tim sees at a glance and nobody else
notices until he says it. Before adding a size, a colour or a spacing,
find the thing it is a sibling of and take that one's.

**An animation used in two places is one component, never two that look
alike.** Work in hand is `Busy.svelte`: the beam, the motes, the fill with
its head, the glow pushed ahead of it and the light passing over what is
done. The range picker once drew its own fill, a wash and a line made to
look like Busy's, and Tim saw at once that it was not the same: no glow
ahead of the head, no motes, a flat wash. A copy drifts from the original
the day either is changed. So a place that shows work puts `Busy` in it,
with `rim={false}` where there is no edge to run round, and a change to
how work looks is made once, in `Busy.svelte` and `app.css`, and is then
true everywhere. The same goes for the shimmer and the pulse.

**A handover the eye sees as one thing is one row.** A card on its way
and the clip it became were two rows of a keyed list, one going and one
coming, and the card was held for its clip by an effect, a render late.
No frame the harness sampled ever showed the gap: the probe counting rows
on every frame passed. What gave it away was reading the list's height on
every forced layout, `getBoundingClientRect` patched to log
`scrollHeight`: the list went a row short inside one update, the leaving
row set absolute by `animate:flip`, which clamps a scrolled list, and the
card came back sliding open. Tim saw it as the whole list blinking at the
end of a search. When a thing changes what it is, keep its key, and hold
what must not go in the same pass that builds the list.

**A row that work fills is filled where it stands.** A search's cards
came in at their moments in the episode, above the rows waiting or between
cards already there, and `animate:flip` slid every row under them down
one, each time a clip was named. Nothing ended wrong and every probe of
the end state passed. Tim saw the stack shuffle and cards that seemed to
be replaced rather than filled in. Now a running search keeps its rows by
place: each clip comes into the row that says what the search is doing,
and a row once a card stays there until the search is over, see `places`
in `ClipList.svelte`. Prove it by following each row by its element on
every frame, a `WeakMap` from element to a number, and counting rows that
move within the stack: the old code moved rows on 112 frames of one
search of six, the new one on none.

## When the window itself looks wrong, measure it against a real one

The app is a native window. Every other native window is on the same
screen, so the control group is free and costs one screenshot.

This is written down because it took a very long time to arrive at. The
title bar "looked wrong" for round after round, and everything anybody
tried was about the bar: its height, the line under it, where the name
sits in it, what it does in fullscreen. It was fixed in one change the
first time anybody put it next to Terminal and read the numbers.

**Take one screenshot holding your window and a reference window, and fix
the scale with an object of known size.** A macOS traffic light is 14
points across, and it is in every window, so it is both the thing being
measured and the ruler. If it comes back 28 pixels wide, the shot is 2x
and every other number halves. Then the question stops being "does this
look right" and becomes a table:

| | down from the window top | in from its left |
| --- | --- | --- |
| ours | 26.0 pt | 26.0 pt |
| VS Code | 17.0 pt | 18.0 pt |
| Terminal | 16.0 pt | 16.0 pt |
| Chrome | 20.0 pt | 20.0 pt |

Six to ten points out in both directions, on every window. That is the
whole bug, and it was invisible for as long as nobody wrote those four
rows down.

**A constraint in a comment is a claim. Check it before building on it.**
`App.svelte` said macOS puts the three buttons where it likes "and
nothing the app can set moves them". That is false: Electron apps
reposition them, VS Code does, and `chrome_darwin.go` already holds the
handle to do it, since it asks the window for `standardWindowButton:` and
reads the frame. Setting it is the same call the other way round.

Nobody checked, and everything after it followed honestly from a wrong
premise: measure the bar with cgo, measure the buttons, publish four
custom properties, centre the name against the buttons' middle, round it
all to whole pixels, hold the height steady through the fullscreen
animation, test all of it. Good work, every line of it, and none of it
needed to exist. The real answer was that the preset asked for a toolbar,
`UseToolbar: true` inside `MacTitleBarHiddenInset`, a toolbar makes the
bar taller, and macOS insets the buttons to centre them in it.

**Adapting has no end condition. That is the smell.** Fitting around
something you cannot change is never finished: there is always a better
centring, another transition, one more rounding. If the work keeps
growing and never closes, stop adding to it and go and test the thing
everybody agreed was fixed. A cause has a fix. A symptom has endless
mitigation, and endless mitigation is how a small thing becomes a month.

**Asking for more opinions does not help while the premise is wrong.**
Every briefing carries the framing with it, so every answer comes back
from inside the same box. What broke this open was not another opinion,
it was Tim asking how VS Code does it. A counterexample beats reasoning:
three applications were on screen doing the thing we had written down as
impossible.

## When it only goes wrong on the Mac, try WebKit

The harness is Chromium, and the app on a Mac is WebKit. Where the two
keep time or paint differently, a probe in Chromium passes and Tim still
sees the bug. The fill of a removed row grew back under the pointer on
his Mac for two rounds while every Chromium probe said it held still:
WebKit plays a paused animation on from its timeline's last frame, and
Chromium from now.

WebKit is in the cloud session: WebKitGTK 6.0 with its Python bindings
for `/usr/bin/python3.12` (not the default `python3`), `xvfb-run` for a
screen, and `libXtst` for a real pointer. Serve `frontend/preview/dist`
over http, open it in a `WebKit.WebView` in a `Gtk.ApplicationWindow`
with `set_decorated(False)` under `GDK_BACKEND=x11`, move the pointer
with `XTestFakeMotionEvent` from `ctypes`, and read the page back with
`evaluate_javascript`. A synthetic event from inside the page does not
set `:hover`, a pointer moved by XTest does. Read what is on screen, a
transform or a box, not what an animation says its progress is.

## What goes wrong here in particular

These are the traps this interface has actually fallen into. Each cost at
least one round of Tim's testing.

**A layout that measures itself never settles.** The rule is in CLAUDE.md
and it is there because the workspace once measured the clip timeline to
work out the picture's height, and the picture's width decided the
timeline's column. Two parts measuring each other need several frames per
resize, which is what a laggy window edge is. Every size is one expression
in the stylesheet now.

**A fraction of a pixel is a bug waiting.** An element on a fractional row
is painted in one place normally and another once opacity, a transform or
a filter puts it on a compositing surface of its own, because that surface
starts on a whole pixel. Give rows whole heights and elements their own
height rather than the height of the line of text they stand in.

**Fractional drawing on a canvas is not a rounding error, it is half the
colour.** A rectangle half a pixel wide is drawn at half strength. Work in
device pixels, one column at a time, and round.

**An effect that mentions a job is rebuilt about once a second**, because
every job event replaces the job object. A timer inside one never fires.
This is in CLAUDE.md too.

**Answers do not come back in the order they were asked for.** Anything
async that paints something needs a ticket, and an answer older than the
newest one already used is thrown away. `Newest` in `frontend/src/lib/
flow.ts` is that, with tests. Walking the clip list with the arrow keys
puts several frame requests in the air at once, and without this the
picture settles on whichever one happened to be slowest.

**Do not skip an ask by comparing against what was last asked for.**
Compare against what is on screen. Going A, B, A quickly, the last ask
matches the last request and is skipped, so B's picture stays.

**The box that is trimmed is the box that is clipped.** `text-box: trim-both
cap alphabetic` is the right way to centre a name on something else, because
it makes the box the letters rather than the line of text they stand in. But
`overflow: hidden`, which is what gives a long name its ellipsis, clips at
that same box, so every tail below the baseline goes. An episode called
`YouTube.mp4` lost the tail of its y that way. Trim the box, then pad it
back out evenly: the middle does not move and the clipping happens at the
padding.

**A number that stands for a position must not be published as zero.** The
bar's box is twice the middle of the window buttons. In fullscreen macOS
takes the buttons away, the middle came through as 0, and the box became
nothing tall. Leave a measurement out altogether rather than sending a zero,
so the stylesheet's own fallback is what answers.

**A drag takes the pointer, and with it every click.** The clip timeline
and the range picker both call `setPointerCapture` on the way down, so a
drag keeps working when it leaves the track. While a pointer is captured
every click and double-click is dealt to the element holding it, whatever
lies under the cursor. A handler on a child is never reached: the
double-click that puts a cut back had to be decided on the track and the
cut found by where the pointer was, because the handler on the cut itself
never ran once. Anything that has to answer a click on top of a drag
surface is either decided by the surface or stops the pointer going down.

**A drag that refuses the pointer refuses the caret with it.** The other
half of the same rule. `preventDefault` on the way down is what stops a
drag turning into a text selection, and it is also what stops the browser
putting the caret where a hand clicked, which is the whole of what
clicking a word in the caption box is for. The caption box takes a drag
and holds words that are corrected in place, so a drag that starts on a
word starts without refusing the pointer and lets the word go the moment
the hand moves instead. It measures cleanly either way: with the old line
back the word never takes the keyboard at all, and the caption line moves
the same whether the drag started on a word or on the box.

**Two things that move together must move by the same means.** A `left`
that is animated is worked out by the main thread on every frame, a
`transform` is carried by the compositor. Put one of each side by side and
they run on two clocks, one of them the clock that also has the app's own
work on it, so one glides and the other steps and the distance between
them changes. That is what Tim saw beside the transcript's edge. It shows
in the harness as well, on an idle browser, as the line standing still on
6 frames of 240 while the mark stood still on none.

**Nothing checks that a call reaches its method.** The window calls the Go
side by name and by position. TypeScript knows `api.ts` and nothing about
`main.go`, Go knows its methods and nothing about who calls them, and a
call one argument short is found by the hand that reaches for the control:
`main.FrameFairy.CutClip expects 7 arguments, got 6`, with the engine, the
tests and the fuzzing all green. `cmd/framefairy-app/bindings_test.go`
parses both sides and walks the whole boundary now. Adding an argument to
a method means adding it in `api.ts`, in whatever passes it down, and in
`wails-stub.ts`, or the harness cannot reach the new behaviour at all.

**A mark that stands for a moment is wider than the moment.** A handle is
twelve pixels across and pulled six back, so the left of its box is six
pixels before the time it stands for. A probe that worked out where to
press from `getBoundingClientRect().left` landed six pixels early on every
drag, and the cut came back a sixth of a second off: a real number, a real
measurement, and the wrong answer. Take the middle of the box.

**Decide by the state the playhead is in, not by how far it is from
something.** Which word is it in, which piece is it in. A distance can
round its way into the answer it started from, which is a key that does
nothing and cannot be got out of. A state cannot. Five bugs in one
afternoon came from comparing the playhead to a moment while the video
preview was a `<video>` element, which answered a seek with the start of
the frame it showed, a hair off where it was sent: the crop frame went
dashed at the start of its own clip, two words in five lit nothing,
stepping back could not get out of a word, the arrow keys walked the wrong
list, and playing seeked before it played. The frame queue puts a paused
playhead exactly where it was sent, but the rule stands: a moment that goes
through two clocks still comes back a hair off, see below. Whether the
space bar plays the clip or the episode is the clearest case, see
Playback below.

**Read the source before building on what a browser is said to do.** The
clock of a playing `<video>` in WebKit was once read as a clock that steps
back, and a forward-only rule was built on that, which held the playhead
still. It was an estimate that never goes back and runs ahead of the
picture, `TimeProgressEstimator` in `MediaPlayerPrivateRemote.cpp`, four
lines.

**A floating thing is measured before the stylesheet has finished with
it.** Which way a list grows from the edge it is hung on is decided from
the width the placement measures, and it measures before the rules run.
So `width: var(--the-anchor-width)` with `min-width: max-content` under it
is measure first and widen after: the placement works out where the
anchor's right edge puts a list of the anchor's width, the list then comes
out wider, and every pixel of the difference goes out the other way. Give
a floating thing a width that does not depend on anything the placement
writes, `max-content`, and it measures what it will get. This took three
rounds of Tim's testing to see, because the harness's own convergence
hides it: the size change trips a ResizeObserver and the second pass comes
out right in Chromium, and the first pass is what he was looking at.

**A floating thing is read whole, or it was not placed.** Over and under
are not the only two answers: the clip timeline's info bubble was too tall
for the room under its mark and for the room over it, so the placement
fell back to under and ran off the foot of the app, and every probe that
opened the bubble at a comfortable height passed. Check a floating thing
by opening every one of them at several app sizes, the small ones above
all, and comparing its box with `innerWidth` and `innerHeight`. And check
whether it clips: `scrollHeight` above `clientHeight` only counts where
`overflow` is not `visible`, or a hover bridge reaching past the box reads
as hidden text. `placeBubble` in `lib/bubble.ts` is the rule, with tests
that put a mark everywhere.

**Two things that show the same number must read the same number.** The
row waiting for the transcript filled by the saved transcript, and the
range picker's line by what had been heard, which runs seconds ahead. Tim
saw the line well past the window and the row at 95 percent. When two
places show one fact, find the one value both read and give it to both.

**One line of text, one reporter.** The search's row showed whatever last
reported progress, and ffmpeg framing a clip reported twice a second, so
the row flicked between the search and "finding camera switches". Work
that owns a line of text holds it (`Log.HoldProgress`) until it is done.

**A person reads slower than the engine reports.** A headline that changes
every half second is not read at all. Text about work stays long enough to
be read, two and a half seconds for a new headline, a second for a count
going up, while the fill and the time left move at once, because those are
one thing moving. A time left moves in steps of five seconds. A step of
the machine's that a person does not act on, loading the model, reading,
thinking, is not a headline.

**A colour the person chose never marks a state of the interface.** The
caption blocks first wore the caption's text colour and marked the one on
screen with the app's colour, and a blue or purple caption made the two
the same. Then the state was marked by dimming the colour, and a dimmed
colour is another colour. What the person chose is shown as they chose
it, and a state is told by what cannot clash with any choice: brightness,
size, a pill in the highlight colour where the short itself uses one. Try
a state against white, a strong green and a colour close to the app's own
before calling it done.

**What must stay together moves the same way.** Two things that slide side
by side, one by `width` and one by `transform`, part by a pixel or two
while they move, because one is laid out on the main thread every frame
and the other is carried by the compositor. The fill and the shade ahead
of it on the range picker both move by transform, with the same glide.

**A moment that goes through two clocks can come back a hair early.** A
caption's start went from the clip's clock to the episode's and back, and
landed before the caption, so a click on it lit no word. A seek meant to
land in something lands a frame into it, `intoWord`, the way the arrow
keys already did.

**Stale state waits to be shown.** The `<video>` video preview kept the
last still it had read from the file and drew it whenever the picture
looked stuck, and looping a clip made it look stuck for the length of one
seek, so a frame from anywhere in the episode flashed up. Anything kept for
later is shown only for what it was made for. The probe for this passed
against the broken code until it first loaded a still the way a busy
machine does: reproduce the state before trusting a probe that says the
state is gone.

**Contrast is judged with daylight on the screen.** A fill of 7 to 22
percent of the accent read as no fill at all on Tim's Mac by a window. A
fill is 30 to 50 percent, and anything that is only told apart by a shade
of dark grey needs a second look in a bright room.

**The rail is part of the app.** The space between the rail and the
workspace is the space between two things, `--gap`. Only the side that
meets the app's own border is an `--edge`. A 24 pixel edge where a 12
pixel gap belonged cost the settings column the room to show 100 percent.

**A row still to come is already a row.** The placeholders of the clip
list answer the pointer the way a clip's row does, brighter under it,
while they go on breathing. The same interface, before and after the
content arrives.

**Paused is a state of its own, and it is still.** The range picker drew
its fill only while the transcription ran, so a pause, by hand or by a
search that took the machine, took the fill away and left a mark beside
an edge nobody could see, and carrying on brought the fill back further
on. Work that stops keeps its fill, `Busy still`, and loses what moves.
Before drawing any work, name all its states, running, paused, waiting,
done, failed, and look at each one in the harness.

**Two numbers for one edge part the moment either stops.** The range
picker drew what had been heard and the search waited for what had been
written down, seconds and minutes of audio apart. When a number drives
what the person sees, the work it stands for waits on that same number.

**Prove a fix on the path the person takes.** The heard-based pause was
tested by calling the Go wait directly, and it passed, while the
interface only asked for the search once the saved transcript covered the
window, so the pause never came into play and Tim saw the same overrun
again. A probe for anything the app does by itself starts where the
person starts, adding the episode, and measures what they see, here the
time from the edge passing the window to the search being asked for,
with a stub that behaves like the real thing: `?growing&lagging` saves
the transcript every 8 s the way the engine does.

**A focusable thing keeps the focus a click gave it.** A caption block
with `tabindex` took the focus on a click, and the first arrow key made
`:focus-visible` true, so a ring stayed round a caption the video preview
had left. An element the keys never reach takes no focus at all. Probe by
clicking and then pressing a key, then list what matches `:focus-visible`.

**The same kind of thing is one rule.** The crop, the window and the
clip had three copies of a 2 pixel border with three different corners.
They are `.frame` in `app.css` now. Before styling something that is
"like" another thing, find the other thing and share its rule.

## Playback: on the clip or on the video

Where the playhead is, as far as playing goes, is a state and not a
measurement. It is **on the clip** or **on the video**, and the gesture
that put it there says which. Nothing else does: not a clock, not where
the queue says it is, not a rounding. The rules are
`placeOf` and `playFrom` in `frontend/src/lib/playhead.ts`, with tests.

| the gesture | leaves the playhead |
| --- | --- |
| picking a clip, from the list, a mark, the keys, a search, a clip made or put back | on the clip, at its start |
| a click on the clip's start edge, a trim, a click on a caption | on the clip |
| a click on the clip's end edge, the clip playing to its end | on the clip, at its end |
| a click, a drag, a step or a seek into the frame that holds the clip's last moment | on the clip, at its end |
| the same into any frame from the one that holds the clip's first moment on, cuts included | on the clip |
| the same into any frame before or after those | on the video |
| no clip chosen, or a clip chosen with no gesture, while the video plays | on the video |
| pausing, a jump over a cut, a loop | where it was |

**On the clip**, the space bar plays the clip from the playhead: its cuts
are jumped and it stops at its end, where the playhead stays, on the clip,
at its end. With loop on it goes back to its start instead. From a cut it
plays from the end of that cut, from inside the frame the clip begins in,
before its start, from the start, and from the end it starts over from the
start, the way QuickTime starts over at the end of a video.

**On the video**, the space bar plays the episode from the playhead,
straight on, through the chosen clip and its cuts, with no jump and no
stop. Playing through the clip leaves it on the video. The chosen clip is
dimmed wherever it is drawn while the playhead is on the video, `.frame.dim`,
`.clipmark.selected.dim` and `.chosen.dim` in `app.css`: the crop frame,
everything drawn for the clip on the clip timeline and its mark on the
range picker. A person sees that the
clip's rules are not in play, and the dimming changes in the frame the
state does.

**The whole clip dims, as one thing.** Only the frames were dimmed at
first, and Tim saw the caption blocks and the cut edges stay bright inside
a dim frame, which reads as a clip that is half in play. Everything drawn
for the chosen clip on the clip timeline, its frame, pieces, cuts and their
edges, trim edges, caption blocks and their edges and thumbnails, is held
by `.chosen`, which lays nothing out, and one rule in `app.css` dims it
with the crop frame and the range picker mark, `opacity: var(--dimmed)`,
the brightness of a button that cannot be pressed. Brightness, so the
colours a person chose for the captions stay theirs. The waveform on the
canvas is the episode's and not the clip's, so it does not dim. Lit whole
while the hand is on the clip somewhere else. A new part drawn for the
chosen clip goes inside `.chosen`, or it stays bright. Prove it by reading
the opacity of every part, the product down its ancestors, on the clip
and on the video, and on the first animation frame after the click that
moves the playhead.

**While it plays, the playhead is the sound being heard.** The frame
queue reports on every animation frame where the program is by the sound
card's clock, `heardAt` in `lib/frames/plan.ts`, located in the episode
through the program, cuts included, and the frame it draws on that same
animation frame is the frame for that same position. So the playhead
moves on every frame of the app, jumps a cut in the frame the sound does,
and stands in the frame on screen, at most a frame ahead of where that
frame begins. It never goes back while playing: the queue keeps the
furthest it got. A play or a seek while playing reports where it starts
at once, so the playhead is at a click in the frame of the click, and it
stands there until the first frame and the first sound are in hand, about
130 ms in the harness, of which 42 are the sound card's latency, with the
first frame of the new place already on the canvas. That is the picture
and the playhead waiting for the sound together, not a hold.

Paused, the playhead is exactly where it was put and the picture is the
frame that holds it. A pause keeps the playhead where the sound stopped and
draws the frame that holds it, which is nearly always decoded already.

The proof, in the workspace, with a clip of three pieces whose cuts are
not on frame edges set through `window.__pieces`: press the space bar, or
click the play button, and read on every animation frame the playhead,
`data-playhead` on `.screen`, and the frame number in the bars of the
canvas, for nine seconds. Both cuts take one frame's time, 33 or 50 ms,
no frame from inside a cut is drawn, no frame stays longer than 50 ms,
the playhead stands for no animation frame after it first moves and never
goes back, the playhead and the frame on screen are less than a frame
apart on the program, and it stops paused on the clip's end, the last
frame of the clip on screen. The same with `?slowread=200`, with loop on
across the seam, and played again from the end. The sound, tapped as
above, is the episode's own to the sample across both cuts, the best lag
0, and nothing is heard past the end.

Where a gesture lands exactly on an edge, the side it is about decides.
The start edge belongs to the clip, and so does the end edge: a click on
it is the clip's own edge, and the space bar starts the clip over from
there, which is what it does where a play of the clip stopped. A trim
puts the playhead on the edge it drags, which can be a hair outside the
pieces the video preview has at that moment, so it says it is about the
clip rather than being measured.

Two choices the words left open, made this way. Loop is about the clip
and changes no state: with the playhead on the video, loop on, the space
bar still plays straight on. And a clip chosen while the video plays,
which does not move the playhead, leaves it on the video, because no
gesture put it on that clip.

**Where a gesture lands is a frame, not a second.** What is on screen is
a frame. With the video element the playhead the next gesture started
from was the video's answer, the start of the frame it showed, and decided
by the second, a frame step left off a clip that starts inside a frame and
one step right landed on the start of that frame, before the clip, and
the clip dimmed while its own first frame was on screen: the old fault,
a comparison of seconds, in a new place. So `placeOf` takes the
episode's frame rate and asks which frame the gesture landed in, the
same frame `insideClip` draws the crop in. The frame that holds the
clip's first moment is on the clip, and the frame that holds its last is
its end. A trim of the end on frames, which leaves the playhead a frame
before the end, is then at the end, and the space bar starts the clip
over at once, as it did before.

**Why it is a state.** It was a distance: the clip played when the
playhead was within half a frame of its start and before its end. That
went wrong three times while the video preview was a `<video>` element,
every time from the same fact, the playhead was never where it was put:

- picking a clip and pressing the space bar seeked before it played,
  because the paused picture answered with its frame's start, a hair
  before the clip, and a seek can refuse a play
- the first play after a search took the clock of a video that had read
  nothing, zero, as the playhead, which measured as far outside the clip,
  and the episode played from its start (#97)
- the paused video on the Mac answers with where its frame begins, up to
  a frame early, so a clip starting more than half a frame into one
  measured as before the clip, and the space bar played the episode
  straight through its cuts and past its end. A wider tolerance and a hold
  on the playhead were tried for this, #99, and closed: each fix only moved
  the place the measurement could round to the wrong side

A gesture knows what it is about. A clock can only say how far it is
from something, and that is the question that never settles.

## The frame queue

The video preview plays from a queue of frames the app decodes itself,
plan row 2.122. It is in `frontend/src/lib/frames/`: `mp4.ts` reads the
index of the episode file, `plan.ts` decides everything, with tests, and
`queue.ts` is only the glue to the browser's decoders, the canvas and the
sound card. `Player.svelte` makes one `FrameQueue` per episode on its
canvas and drives it with four calls: `setProgram`, the clip's pieces or
null for the episode straight on, `seek`, `play` and `pause`. What the
queue reports, `Shown`, is the playhead and nothing else sets it while
playing.

The queue also has a page of its own in the harness, `preview/frames/`,
one canvas and `window.__queue` to drive, which is the quickest way to
look at the queue alone:

```
npx vite build --config frontend/preview/vite.config.ts
node frontend/preview/frames/cuts.mjs                 # the clip, once
node frontend/preview/frames/cuts.mjs loop slowread=200
node frontend/preview/frames/cuts.mjs seeks           # paused frames, a seek while playing
node frontend/preview/frames/cuts.mjs memory          # a minute straight on
```

`frames()` in `open.mjs` opens it on its own episode, ten minutes made by
the same recipe as the workspace's, see The episode the harness can play.
`?read` reads the frame number back from the canvas and `?hear` records
every sample handed to the sound card, and the probe lays it over the
episode's own sound decoded by ffmpeg, cut and faded the way the render
does it. A sample missing, doubled or taken from inside a cut no longer
matches. `SHIFT=1` moves the expected sound by one sample after each cut:
the check has to fail with it, and it does, from 2 percent off to 35.

In the workspace the same is measured on the path a person takes, see
the proof under Playback above: the clip picked from the list, the space
bar or the play button, the bars read from `.screen canvas`, the
playhead from `data-playhead`, and the sound card tapped from an init
script.

What the proofs read, the clip of three pieces with cuts off the frame
edges, 25 frames a second:

| | the frame queue, its own page | the frame queue, the workspace | the `<video>` path, the workspace |
| --- | --- | --- | --- |
| last frame before a cut to the first after it | 33 or 50 ms | 33 or 50 ms | 16 to 49 ms, 184 to 199 ms with `?slowseek=150` |
| the same with the file 200 ms late | 33 or 50 ms | 33 ms | |
| frames from inside a cut drawn | none | none | none |
| the longest any frame stayed while playing | 50 ms | 50 ms | 93 ms, 199 ms with `?slowseek=150`, 417 ms with `?webkitclock` |
| the playhead while playing | moves with the sound | still for no animation frame, never back | still for 1 animation frame, 10 with `?slowseek=150`, 24 with `?webkitclock` |
| the playhead and the frame on screen | | less than a frame apart | up to 2 frames apart |
| from the press to the first new frame | 146 ms | 134 to 151 ms, 33 ms from the end | 47 to 56 ms, 429 ms with `?webkitclock` |
| where the play stops | 65.000, frame 64.96 | 65, frame 64.96 | 64.976 to 64.994, frame 64.96 |
| the sound across a cut | the episode's own to the sample, best lag 0 | the same | stops for the seek |
| the loop seam | the same as a cut | the same as a cut | |
| paused frames | the frame that holds the moment, 11 of 11 | 14 of 14 gestures, 3 to 25 ms | a still from the engine while the seek lands |
| frames open over a minute of play | 4 to 5 | | |

A frame interval is 40 ms and the display draws every 16.7 ms, so one
frame's time shows as 33 or 50. A gap above 50 is a frame held. The
`<video>` path was measured on the harness of f10a0b6, the last commit
that had it, with its `?fps=25`, `?slowseek` and `?webkitclock`, which
went with it, and the frame on screen from `requestVideoFrameCallback`
through its `?framelag=1`. Its 16 ms is the frame after the cut put up
early, in the middle of the seek.

**What it taught.** Each of these cost a round of probing:

- Chromium stamps decoded sound with times of its own, counting on from
  the first packet after a start. Sound is matched to its packet by
  order, not by stamp.
- Opus told its pre-skip cuts it again from the first packet after every
  start, on top of the edit list. The decoder is told there is none.
- An AudioWorklet in Chromium now and then hands two render quanta the
  same `currentFrame`. The recorder labels a quantum by the last one plus
  128 when that happens, or the check fails on a sound that is right.
- `getOutputTimestamp` is the heard sound, but a stamp taken before the
  sound card was held counts on through the pause, and WebKitGTK without
  a sound device gives stamps on another clock. `heardAt` takes a stamp
  only when it is fresh and near the clock.
- The first frame of a play stays until the sound is heard, about 130 ms
  here, of which 42 are the sound card's latency. That is the picture
  keeping to the sound, not a hold.
- A paused seek used to decode the frame on its own, and the space bar
  then decoded the same group of pictures again for the play. Now a paused
  seek cues the play, the cue's first frame is the paused frame, and the
  space bar only starts the clock. Where the paused frame is not the
  first frame of the play, in a cut of the clip or past its end, the frame
  is decoded on its own as before.
- A paused seek to a moment in the frame already on screen draws nothing,
  so it has to say so anyway, or whatever waits for it waits for ever.
- Plain sound in a MOV counts every moment as a sample, hundreds of
  millions in four hours. It is read by the chunk, and turned into the
  sound card's numbers without a decoder.
- HE-AAC comes out of the decoder at twice its core's rate. The file
  usually says so, and where it does not, the rate the decoder puts the
  first sound out at is taken and the play starts over at it, before
  anything is heard.

The `<video>` element was left because a cut was a seek in the middle of
playing. Every gap between the pieces of a clip held the last frame before
it for as long as the seek took, a fifth of a second and more on a long
episode on the Mac, and the sound stopped with it, where the render plays
straight on. Around that one fact grew everything this skill used to say
about the video element: a still from the engine laid over the picture
while a seek landed, the playhead following `requestVideoFrameCallback`
because the clock ran ahead of the picture, waiting for the first frame of
a seek, a chase for seeks the webview dropped while the machine was busy,
and harness modes that imitated each of WebKit's answers. None of it
could make a cut take one frame's time.

**WebKit, but not the Mac.** WebKitGTK runs the page too, see below, once
`gstreamer1.0-plugins-bad` and `gstreamer1.0-libav` are installed: VP9
needs `vp9parse`, H.264 needs `h264parse` and `avdec_h264`. An MP4 of
H.264 with B-frames and AAC played the clip there with every frame in
order, the cuts one frame's time and the canvas the frame said. That is
GStreamer decoding. On the Mac, WebKit decodes with VideoToolbox and
AudioToolbox, and only Tim's Mac can say how those order their output,
stamp their sound and report the sound card's latency.

## A click is answered in the frame it lands in

Tim felt this twice in one afternoon, and both times every test was green.
Continue left the row the Stopped note stood in empty for six frames
before it said Transcribing. Cancel made its button say Cancelling at once,
while the row beside it went on saying Transcribing for the second the
search took to save what it heard. Nothing was wrong in the end state.
Everything was wrong in between, and the in between is what a hand feels.

The rule: **everything a click changes shows the change in the same
frame**, the control and every place that shows the same work, the row,
the fill, the range picker. Say what is under way, Stopping, Starting, the
step it goes into, and stand still where it got to. The Go side's answer
replaces that when it comes. Never wait for a job event or a timer's tick
to show what the click already knows. The only thing that may wait is a
change the engine makes by itself, a step that lasted a moment, so it does
not flash.

How to catch it, because a screenshot never will: click inside
`page.evaluate` and read the thing on every `requestAnimationFrame` for a
second, then print the runs of frames that looked the same.

```js
const frames = await page.evaluate(() => new Promise((done) => {
  const seen = [], row = () => document.querySelector(".ghost.next")?.textContent.trim() ?? "(none)";
  document.querySelector("button.new").click();
  let n = 0;
  const tick = () => { seen.push(row()); if (++n < 60) requestAnimationFrame(tick); else done(seen); };
  requestAnimationFrame(tick);
}));
```

A run of `(empty)`, or of the state before the click, is the bug. And the
stub has to take as long as the real thing takes: the fake search stopped
at once, so the second the real one spends saving could not be seen at
all, and the probe passed against broken code until the stub waited
800 ms the way the engine does.

## A drag reshapes everything drawn from it

The same rule as a click, over the length of a drag. Dragging a clip edge
moved the edge and the wash under the hand, and the caption blocks stayed
where the saved clip had them until the hand let go. Every test was green,
because every test looked at the end. Tim found it by looking at the
middle.

So when a drag changes a thing, list everything that is drawn from that
thing, and make each one follow the draft, not the saved state. When what
follows is the engine's to work out, like the captions of a clip, ask the
engine on the way, one question at a time and always about where the hand
is now, and keep the last answer until the saved state comes back, so
nothing jumps as the hand lets go. `Shape` is the example.

How to catch it: hold the mouse down with `page.mouse.down()`, move in
steps, and read the thing after each step, before `page.mouse.up()`. A
number that only changes after the up is the bug. Then read every frame
across the up, the way the probe above does for a click.

## Work a click starts is built in front of the person

Answering the click is not enough when the work takes seconds. Tim pressed
I to make a clip, and for ten seconds nothing he could see changed: the
card was made, but in its place far down the list, the button showed
nothing, and the clip timeline waited for the finished clip. Then
everything changed at once. Every test was green, and the stub made the
clip in a second, where the list was already looking.

The rule: **what a click asks for is shown where it will live from the
moment it is asked for, and filled in as each part of it becomes known.**
The control it was started from wears the beam. What it makes is chosen
and brought into view. What is known already, where it lies, how long it
will be, its pieces, its captions, is drawn where the finished thing is
drawn, the clip timeline above all, as the engine learns it. The finished
thing replaces what was built with no jump. Nothing appears whole after
seconds of nothing.

Check it by making the thing somewhere the screen is not looking: far
down the episode, with the list scrolled elsewhere. And make the stub as
slow as the app, the speech model loading included, or the gap it hides
is exactly the one Tim sees.

## A list that floats keeps what it hangs from in place

Every list in the app is `Pick.svelte`, and its list hangs on the edge of
its trigger. That holds only while the trigger stays where it was. The
list is drawn over the whole app, outside whatever the trigger stands in,
so to anything that opens on hover, moving onto the list is leaving. The
first sort menu in the sidebar was hung correctly, and on Tim's Mac the
sidebar closed under it: moving onto a row shrank the sidebar, the list
followed the trigger across and ended 40 pixels short of it, the pointer
came back over the sidebar, and it flashed open and shut until it gave up.
No row could be picked. The probe had passed because the sidebar in the
harness was pinned.

The rule: **a container that opens on hover stays open while a list or a
bubble of its own is open**, which is what `onopenchange` on `Pick` is for.
Check it with the container not pinned: open it by hovering, open the list,
move onto a row one step at a time, and read the container's width at
every step.

**And after the list closes, it asks where the pointer is.** Held open by
its list alone, the sidebar fell back on what it last knew of the pointer
the moment the list closed, and that was a pointer that had left: Escape
with the pointer off the sidebar, or a click on the sidebar itself, closed
the list and the sidebar with it. What it knew went stale while the list
was open. So it stays open and lets the next move of the pointer decide,
and Escape closes one layer per press. Check every way a list closes, a
pick, Escape and a click, with the pointer on the trigger, on the
container and outside it.

## Before saying it is done

- `make interface`, which is the type check and the interface's own tests.
- Build the preview, and the motion bench if the change touches any of the
  five ways work in hand is shown.
- `make test` and `make` only if the change touches Go. A change only to
  `frontend/` or `docs/` does not: the Go tests fuzz for ten thousand
  executions a target and take minutes, and no line of CSS can move them.
  CI runs everything anyway, on both systems. Waiting on them in a cloud
  session is Tim waiting.
- Say what to look at and what should happen, in the app, in the order
  Tim would do it. He is the one who can see it.
- Name what you could not verify. A fix reported as certain and found
  broken costs more than one reported as unverified.
