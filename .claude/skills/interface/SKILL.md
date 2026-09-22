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
| `?busy` | a search running, with progress |
| `?unknown` | the same, with progress it cannot put a number on |
| `?transcribing` | a transcription part way, reporting ahead of the saved transcript |
| `?growing` | a transcript that really grows, job events every 900 ms |
| `?paused` | clips found, the episode read part way, nothing reading the rest |
| `?found` | a search that runs and really finishes, clips and all |
| `?rendering` | a render running on the first clip, with progress |
| `?setup` | a machine with nothing on it, so the first run is the window. Both model installs really run and really finish, on their own clocks, and one language model fits the machine it pretends to be while the other does not |
| `?refuse` | an engine that says no to an edit. Correcting a word and picking a caption face both fail, which is how to see what a control shows once the answer is no rather than yes |

Add a mode when the state you need is not there. A bug that only happens
while something is running cannot be found in a stub that is never busy:
the first search never starting was invisible until `?growing` sent job
events the way the Go side does.

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
whatever the window did. The cues are built from the clip's words now, on
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

The video preview is a real `<video>` and the harness serves it a real
episode, made with ffmpeg on the first run and kept in the temp folder:
four hours at a frame a second, 192 by 108, about 700 KB. Without ffmpeg
there is no episode, playing fails, and a probe that needs it has to say
so rather than pass.

Two things about it are deliberate.

**It is VP9 in a WebM, not H.264 in an MP4.** The Chromium that comes with
Playwright is built without the proprietary codecs, so an MP4 loads,
errors and never plays, which looks exactly like a video that is paused.
The app itself runs in a WebKit view and plays the MP4 the Go side serves.

**It is a grey that climbs from the first second to the last, not black.**
Black is unreadable in the one way that matters: a black picture and no
picture at all look the same, so a probe cannot tell a video preview that
has gone blank from one showing a frame, and that is the whole question
about anything the player does. Grey answers it, and because the grey
climbs, a probe can also say roughly where in the episode the picture is
and that it moved. Roughly: the luma comes back through a limited range
and a lossy encoder, so it is good for *it is showing something* and *it
moved about that far*, not for a second exactly. Anything needing the
exact second reads the app's own clock.

Read it by taking a screenshot of the middle of `.screen` and averaging
the pixels. The middle, because the picture is letterboxed in the preview
and a sample anywhere else reads the black bars and calls a good picture
blank. Around 28 is a picture, 0 is nothing.

The fallback still is served too: the fake Go side answers `Still` with a
path per second, and the harness cuts that second out of the same episode
and hands it back as a PNG. Without that the `<img>` drawn over a stale
picture asked for a jpg and was handed a video, so the one thing that
covers a blank preview could not be reached here at all.

**A page error is a result.** Attach `page.on("pageerror", ...)` in any
probe that plays or edits while playing. The player's frame loop threw
`Cannot read properties of undefined (reading 'end')` when a cut was put
back while the clip played past it, which ended the loop for good, and the
throw is the only clean signal of it: the video's own `currentTime` goes
on rising either way, because the element is still playing. Measuring the
video's clock would have said everything was fine.

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

**The webview cannot always read the episode file** while the machine is
busy, so a seek is dropped silently and the picture stays where it was.
Anything that assumes a seek worked will be wrong while transcribing.

**The playhead is never where it was put.** This is the big one. A video
element answers with the moment of the frame it is showing, which begins a
little before the second it was sent to and can be a little after. Five
bugs in one afternoon were that one fact read exactly:

- the crop frame went dashed at the start of the very clip it belonged to,
  because the playhead sat a hair outside it
- stepping by words landed on word edges, where the caption's clock and
  the playhead's disagree by a thousandth of a millisecond, and two words
  in five lit nothing
- stepping back could not get out of a word at all, because it measured
  how far the playhead was from one, the clock had drifted past the word
  it was on, so back found that same word and sent the playhead where it
  already stood
- which list the arrow keys walk was read exactly, so the first press
  after picking a clip walked the wrong one and jumped out of the clip
- playing seeked before it played even with nothing to move, and a seek is
  the one thing that can refuse a play

So never compare a playhead to a moment. Compare it to a frame, or better,
do not compare it at all: **decide by the state the playhead is in, not by
how far it is from something.** Which word is it in, which piece is it in.
A distance can round its way into the answer it started from, which is a
key that does nothing and cannot be got out of. A state cannot.

And note where this could not be reproduced: the harness's video answers
with the exact second it was sent to, so none of these five happen in
Chromium at all. They are proved as rules in `lib/` with tests, and the
tests say so.

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
