# A push walks in Chromium, the night in both browsers

Which browser the walks of a pull request walk in, once WebKit was
measured to be the slow one. Decided on 5 November 2026, in the pull
request of #692. It replaces the browser of a push in
[the walks in two browsers](2026-10-31-walks-in-two-browsers.md), and
keeps everything else that record decided.

## The problem

Tim, in #692: the walks jobs take four to six minutes, and each should
take under three on the machines we have, with the same coverage. On the
pull request: "I want to know what's structurally wrong about our tests
and how we can speed them up more so that the same coverage is produced
in far less time."

Since 31 October every walk of a push walks in Playwright's WebKit for
Linux. That WebKit is WPE, which draws every frame through OpenGL, and a
machine with no graphics chip, CI's or a cloud session's, runs that on
the processor, through Mesa's llvmpipe. Drawing was 37 % of the
processor time of every walk in WebKit, measured once in a cloud session
on 3 November, see #692. Every walk, walked in each browser once in a
cloud session of four cores, six walkers, at the same commit, the
seconds the walkers walked together:

| All walks, one run each | WebKit, before (s) | Chromium, after (s) | Change      |
| ----------------------- | -----------------: | ------------------: | ----------- |
| every run               |               5353 |                3467 | 35 % faster |
| the sequences           |               4144 |                2694 | 35 % faster |
| the random walks        |               1209 |                 773 | 36 % faster |

One run each, so there is no spread. The cuts made before in this pull
request, a page kept for each walker and two drawing threads, took the
four CI jobs from 1248 to 1219 s of machine time, 2 % faster, one run
each and inside the noise, so the page kept was taken out again. The
browser is the cost that is left.

## Five simulations

### 1. Who may know about whom

The browser stays one setting of the harness, `browserFor` in
`walks/bridge.mjs`, and the walks know nothing of it. Changing the
browser of a push is changing what `browserFor` answers when nothing is
asked, and what the nightly workflow asks. No walk changes. Cost: one
line of code and the docs that name the browser.

### 2. What changes together

Make WebKit itself quicker: a graphics chip for the walks, a WebKit that
draws without OpenGL, or fewer frames drawn. A runner with a graphics
chip is a larger runner, which is more machine than we have. Playwright
builds its WebKit for Linux one way and brings no switch for its
drawing. Fewer frames would change what the walks see, so it would test
something else. Dropped. Kept from it: the two drawing threads, which
still help when WebKit walks at night.

### 3. Where state lives

Walk a chosen set in WebKit on a push, the walks about typing, the cursor
and focus, where #574's faults were, and the rest in Chromium. The set is
a list with no writer to keep it true, the reason the walks run whole,
see [BUILD.md](../BUILD.md#why-the-walks-run-whole). Dropped.

### 4. What runs together, and what it costs

Chromium on a push, WebKit at night. A pull request waits for the
quicker engine only, 35 % less walking, in the same four jobs. WebKit
still walks every walk once a day, so a fault only the Mac's engine has
shows by the next morning, as a fault only Chromium had did before.
Coverage stays in Chromium, which is the only browser that counts lines.
Cost: four more machines at night, which nobody waits for, and a WebKit
fault found a day late.

### 5. What a test has to build

Drop WebKit altogether: one browser, one image, no second engine to
keep walking. The quickest and the simplest, but the app on a Mac is
WebKit, the Mac ships first, and nearly every fault Tim met in #574 was
WebKit's alone. Without it a Mac-only fault is only found when Tim tries
the app. Kept as the answer if WebKit at night shows nothing for a long
time.

## What converges

Four of the five keep one set of walks walked whole by one harness, and
the browser a setting, 1, 3, 4 and 5. Making WebKit quick (2) is out of
our hands. A list of walks for each browser (3) is dropped again. The
fourth is the third simulation of the old record turned round: the
browser a pull request waits for is the quick one, and the night walks
both.

## The decision

```
plain code      Go tests and Vitest, once: every rule said without a page
  │
walks           every walk and sequence, the same set
  │
harness         walks/bridge.mjs, browserFor: Chromium, or WALKBROWSER
  │
Playwright      Chromium (Windows), every pull request and every night,
                counting lines at night
                WebKit (Mac, Linux), every night, on four machines of its own
```

## What follows

- A pull request walks 35 % less, measured in a session, and the pull
  request measures it again in CI.
- A fault only WebKit has is found by the night, a day late, and the
  walk that found it is fixed, never kept out of WebKit. No pull request
  waits on the night, so a red night is read by the weekly triage, which
  matches every red run of main's, the nightly ones among them, to an
  issue, see [TRIAGE.md](../TRIAGE.md#every-week). So a WebKit fault is
  logged within the week, not in the morning.
- The CI image keeps both browsers while the night walks in WebKit. A
  push could take an image with Chromium alone, once Ubuntu's snapshot
  serves a rebuild in the time a job has.
- It is wrong if WebKit at night finds faults Tim would have met before
  the next morning, then a push needs WebKit again. It is wrong the
  other way if WebKit at night finds nothing for weeks, then the fifth
  simulation, no WebKit at all, is the next thing to weigh.
