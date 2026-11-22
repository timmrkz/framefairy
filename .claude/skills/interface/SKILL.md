---
name: interface
description: How to change a user interface you cannot see, in any project with one, a web page, or a web interface inside a desktop app. A harness that renders the real interface, what to measure, how to prove a fix rather than claim it, and what a hand feels, a click answered in its frame, a drag that reshapes all it draws, work shown where it was started. Read it before changing anything a person sees, and above all when the user reports something that looks wrong. Where another interface skill says otherwise, this one wins.
---

# Changing an interface you cannot see

An agent without a screen cannot look at what it changes. Without care
it guesses, reports a fix, and the user finds the bug still there, a
round of their testing lost.

**The user** is the person the agent works for, who decides what the
product does. **The people using the product** are who it is for. The
project's instructions say where its harness, its stub, its end-to-end
tests and its checks are. This skill says how to use them, and what to
push for where they are missing.

## 1. A harness that shows the real thing

- **Render the real interface in a headless browser,** for example with
  Playwright, against the real back end where it can be reached and a
  stub where it cannot. A probe is a few lines: open, bring the state
  about, measure one thing, close. One-off probes live in a scratch
  folder, build output is never committed.
- **A probe waits on a signal, never a timeout:** a "settled" signal
  once nothing is in flight and the picture stands, or a wait for the
  thing itself. A probe that hangs waits for a signal that never came,
  so print every wait (in Playwright, `DEBUG=pw:api`) and the last one is
  the culprit.
- **The stub is told what to pretend,** one mode per state, for example
  a query string. Add a mode when the state you need is missing. A stub
  that is never busy hides everything that only happens while work runs.
- **The stub reports the way the back end does.** Where the interface
  hears of changes only through events, a mode that sends none lets a
  broken cancel pass, since the item stays "running". Send the events,
  with every field the real side sends.
- **The stub's answers are made by the back end's rules, not by hand.**
  Hand-made data looks fine in a picture and hides bugs: items that
  cannot be traced back to their source, ranges that overlap where the
  real side clamps them, two calls that build the same thing two ways.
  One list, and the real rules on it.
- **The stub takes as long as the real thing,** and has a mode where
  every call answers late by a random share, and one where answers
  arrive out of order. Instant answers make a waiting interface look
  finished in its first frame.
- **Test data says what it is.** Each item, row or picture carries its
  own index where a probe can read it, so a probe reads what was drawn,
  not the app's word for it. Make "empty" and "blank" differ, since black
  and nothing look alike.
- **A page error is a result.** Listen for uncaught errors in every
  probe. A loop that throws stops while all else looks fine.
- **One bench page for a shared component** shows every state of it at
  once: every share of progress, every kind of control, other accent
  colours. Nothing has to be started or caught, so it is the fastest
  look. What joins the component joins the bench in the same change.
- **Nothing checks a call reaches its method** across a boundary typed
  on one side only. A test that parses both sides and matches every call
  catches the missing argument every other test passes.

## 2. Measure the right thing

The layout's rect cannot see a painting bug. Decide which kind of bug it
is before picking an instrument:

| the thing is | measure with |
| --- | --- |
| in the wrong place, the wrong size | the layout rects |
| the wrong colour, weight, brightness | the pixels of a screenshot, per row or column |
| moving when it should not | both, and compare |
| happening at the wrong time | the state, every animation frame |

- **The box is not the ink.** Two boxes can line up to the hundredth of
  a pixel while the text in them ends four pixels apart, since an
  ellipsis fits whole characters. Screenshot it and find the first
  painted column from the edge.
- **While anything glides, the rect is the animation.** The style says
  what the interface decided, the rect what is drawn so far. Measure
  after `transitionend`, or read both and say which you mean.
- **A moving target cannot be clicked at a point worked out a moment
  ago.** To test the handler rather than the hit area, call `el.click()`.
- **A mark that stands for a point is wider than the point.** Press the
  middle of its box, not its left edge.

## 3. Prove it, do not claim it

- **Reproduce it first.** When the harness cannot, say so rather than
  imply a fix, and prove only that the cause is gone.
- **Put the old code back and measure again.** A number that changes
  when the fix is removed is a fix, one that does not is a coincidence,
  or a probe against a stub that never reached the broken state. A probe
  that has never failed is not yet evidence.
- **Check against what the user said.** They report precisely, and the
  pattern is evidence: which places it happened in and which not, and
  what those share, often finds the cause faster than the code.
- **Prove it on the path the person takes.** Start where they start, not
  at the inner call, with a stub as slow and as ordered as the real
  thing, and measure what they see.
- **Name every state before drawing work:** running, paused, waiting,
  done, failed. Look at each. Paused is a state of its own, and still: it
  keeps its progress and loses what moves.
- **Stale state waits to be shown.** Something kept for later shows only
  for what it was made for. Make the stale state happen before trusting
  a probe that says it is gone.

## 4. Two things of the same kind look the same

- **Find the sibling and take its size, colour and spacing.** Two heads
  of areas that differ read as a hierarchy that is not there.
- **The same kind of thing is one rule.** Three selection outlines are
  one class, one animation used twice is one component, never a copy
  that looks alike and drifts the day either changes.
- **A handover the eye sees as one thing is one row.** When a
  placeholder becomes the item, keep its key, or the list blinks between
  two rows. Hold what must not go in the same pass that builds the list.
- **A row that work fills is filled where it stands.** Items arriving
  between rows shuffle the stack. Give running work its places and fill
  them. Prove it by following each row's element every frame and
  counting the ones that moved.
- **A row still to come is already a row,** and answers the pointer.
- **Two places that show one fact read one value,** and the work that
  fact stands for waits on that value too, or the two part the moment
  either lags.
- **One line of text, one reporter.** Work that owns a status line holds
  it until done.
- **A person reads slower than the machine reports.** A new headline
  stays about 2.5 s, a count going up 1 s, a time left moves in steps of
  five seconds, while a progress fill moves at once. A step the person
  does not act on is not a headline.
- **A colour the person chose never marks a state,** and a dimmed colour
  is just another colour. Tell state by what cannot clash, brightness, size, a shape. Try it against white, a strong
  green and a colour near the accent.
- **Contrast is judged with daylight on the screen.** A faint fill reads
  as none: measure the share at which it shows in daylight. A state told apart only by a
  shade of dark grey needs a look in a bright room.
- **An element the keys never reach takes no focus,** or a click leaves
  a focus ring on it long after.

## 5. When the window looks wrong, measure it against a real one

- **Every other native window on the screen is a control group.** Take
  one screenshot holding yours and a few others, fix the scale by an
  object of known size (the macOS window buttons are 14 points across),
  and write a table of the offsets. "Looks wrong" becomes numbers, and a
  bug tuned for rounds is fixed in one change.
- **A constraint in a comment is a claim.** Check it, by how other apps
  do it, before building on it. **Adapting has no end condition, that is
  the smell:** when work around something keeps growing, test the thing
  everybody agreed was fixed. A cause has a fix, a symptom endless
  mitigation. More opinions do not help while the premise is wrong,
  since every briefing carries the framing and every answer comes from
  the same box. A counterexample does.

## 6. When it only breaks on one engine

- **Try that engine.** A product shipped in a platform's webview runs on
  that platform's engine, and where it keeps time or paints differently,
  a probe in another passes on the bug: a paused animation may resume
  from its last frame in one and from now in the other. Run the probe and
  the end-to-end test in the shipped engine (Playwright has WebKit,
  Chromium and Firefox).
- **A system webview under a virtual display** with a real pointer (for
  example X11's XTest) shows `:hover`, which a synthetic event from
  inside the page does not set. Read what is on screen, a transform or a
  box, not an animation's progress.
- **Headless may not read a GPU canvas back.** Give probes a switch to a
  2D canvas. **Read the engine's source before building on what it is
  said to do.** A rule built on a rumour that a browser clock steps back froze a
  moving marker, and the source showed an estimate that never goes back.

## 7. What a hand feels

- **A click is answered in the frame it lands in.** Everything the click
  changes shows it in the same frame, the control and every place that
  shows the same work, saying what is under way (Stopping, Starting) and
  standing still until the back end's answer replaces it. Never wait for
  an event or a timer to show what the click already knows. Only a change
  the system makes by itself may wait, so a step that lasted a moment
  does not flash. A screenshot never catches it, the end state is right
  and the in between wrong. Click inside the page and read every frame:

  ```js
  const seen = await page.evaluate(() => new Promise((done) => {
    const read = () => document.querySelector(".status")?.textContent.trim() ?? "(none)";
    const out = [];
    document.querySelector("button.start").click();
    const tick = () => { out.push(read()); out.length < 60 ? requestAnimationFrame(tick) : done(out); };
    requestAnimationFrame(tick);
  }));
  ```

  A run of `(none)`, or of the state before the click, is the bug.
- **A drag reshapes everything drawn from it while it moves.** List
  everything drawn from the dragged thing and make each follow the draft,
  not the saved state. When the back end works it out, ask it on the way,
  one question at a time, always about where the hand is now, and keep
  the last answer until the saved state returns, so nothing jumps on
  release. Better, one answer holds everything drawn from the thing, in
  the shape the saved state is sent in, so no part is missed. Test with
  the button held, moving in steps and reading every part after each. A
  number that changes only after the release is the bug. Then read every
  frame across the release, as for a click.
- **Work a click starts is shown where it will live,** from the moment it
  is asked for. The control shows it is busy, and what it makes is
  chosen, scrolled into view, and filled in as each part becomes known. The finished thing replaces it with no jump. Nothing
  appears whole after seconds of nothing. Check it with the view scrolled
  elsewhere and a stub as slow as the real thing, start-up included.
- **Decide by state, not by distance.** What a gesture is about is a
  state the gesture sets, not how near a clock or a position is to
  something: a distance can round back to where it started. Compare in
  the units the screen has, a pixel, a step, a row, not in a continuous
  value. A value taken through two conversions comes back a hair off, so
  a jump meant to land inside something aims one step in from its edge,
  never at the edge.
- **Answers do not come back in the order they were asked for.** Async
  work that paints carries a ticket, and an answer older than the newest
  one used is dropped. Compare against what is on screen, not what was
  last asked for, or A, B, A quickly skips the last A.
- **Never read back a value a call can have changed through a
  listener.** Keep it in a local. **A timer in a reactive effect on often
  replaced data never fires,** as the effect is rebuilt. Start it once,
  on mount, and let it ask when it fires.
- **A drag takes the pointer, and every click with it.** With pointer
  capture, a child's click handler never runs. What answers a click on a
  drag surface is decided by the surface. And `preventDefault` on the
  way down stops the text caret as well as a selection.

## 8. Layout and pixels

- **A layout that measures itself never settles.** Every size is one
  stylesheet expression from the window and the tokens. Script reads a
  size, never writes one back.
- **Whole pixels.** Rows are whole pixels high, an element as tall as
  itself and not its line of text. Something on a half pixel moves the
  moment opacity, a transform or a filter puts it on its own surface,
  which starts on a whole pixel. Measure what is painted, not the box
  the layout reports: an odd-sized mark centred in an even box lands on a
  half pixel, so fixes that trusted the box moved nothing. On a canvas, half a pixel is half the
  colour: draw in device pixels and round.
- **Two things that move together move by the same means.** An animated
  `left` runs on the main thread, a `transform` on the compositor, so
  side by side one steps while the other glides.
- **The box that is trimmed is the box that is clipped.** Trimming a
  text box to its letters and clipping it for an ellipsis cuts every
  descender. Trim, then pad back out evenly.
- **A number that stands for a position is not published as zero.** When
  it is unknown, leave it out so the stylesheet's fallback answers.
- **A floating thing is measured before the stylesheet has finished with
  it.** A placement library picks a side from a width the rules then
  change. Give it a width the placement does not write, like
  `max-content`.
- **A floating thing is read whole, or it was not placed.** Open every
  popup at several window sizes, small ones above all, and compare its
  box with `innerWidth` and `innerHeight`. `scrollHeight` above
  `clientHeight` means clipped only where `overflow` is not `visible`, or
  a hover bridge past the box reads as hidden text.
- **A floating list keeps what it hangs from in place.** A container
  that opens on hover stays open while a list or bubble of its own is
  open, since moving onto the list is leaving the container. When the
  list closes, the container asks where the pointer is rather than trust
  the old answer, and Escape closes one layer per press. Check it
  unpinned: hover it open, open the list, move onto a row step by step,
  and read the container's width at every step. Then every way a list
  closes, with the pointer on the trigger,
  the container and outside.

## 9. Before saying it is done

- An end-to-end test that does what the user did fails without the fix
  and passes with it, written as the testing skill says.
- The type check and the interface's own unit tests pass.
- The full end-to-end suite runs in CI. A single test runs alone in the
  session only to write or fix it.
- Build the harness, and the bench page when its component changed.
- Push, let CI run the rest, and hear how it went from its events.
- Say what to look at and what should happen, in the order the user
  would do it.
- Name what could not be verified. A fix reported as certain and found
  broken costs more than one reported as unverified.

## 10. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| wait for `networkidle` or a fixed timeout in a probe | passes before the picture stands, or hangs | a settled signal or the thing itself, section 1 |
| a screenshot as proof | catches the end state, not the frames between | read every frame, section 7 |
| judge alignment by the layout's boxes | the box is not what is painted | measure the painted pixels, sections 2 and 8 |
| test in Chromium only | the product ships another engine | test in the shipped engine, section 6 |
