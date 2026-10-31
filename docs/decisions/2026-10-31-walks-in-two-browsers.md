# The walks run in two browsers

How the tests are shared out once there is more than one way to run
them. Decided on 31 October 2026, in the first pull request of #380.

## The problem

Tim asked, on 31 October, after the report's field in #574: "I want to
know how we structure the tests when there are multiple modalities to run
tests. Which tests do we run which way then? And why?"

The app runs in WebKit on a Mac and on Linux, and in WebView2, which is
Chromium, on Windows, see [PACKAGING.md](../PACKAGING.md). The walks ran
in Chromium alone. In #574 nearly every fault Tim met by hand was
WebKit's: the cursor revealed in the middle of the field, the cursor left
drawn after a scroll, the focus left on nothing after a button. Every
walk passed each time, and each fault was found again by hand in
WebKitGTK in the session.

Counted on main at d85532d: 239 sequences in eight parts (captions 49,
report 43, timeline 39, clips 26, playback 24, app 23, search 22, videos
13) and four random walks (`playback.mjs`, `searching.mjs`,
`timeline.mjs`, `words.mjs`), three seeds each and playback once more on
HEVC. Four Linux machines walk them in about four and three quarter
minutes each. `browserName` did not exist. The browser was named in one
place, `launch` in `walks/bridge.mjs`, and two features reach past it
into Chromium's own tools: `SLOW`, through the DevTools protocol, and
`WALKCOVERAGE`, through Chromium's line counts.

## Five simulations

### 1. Who may know about whom

The walks know nothing of the browser. Only the harness launches it, from
one setting, and a step asks which browser it is in only to act out
something one engine does and the other cannot. Layers: plain code under
the walks, the walks under the harness, the harness under Playwright.
Nothing crosses but one name, `browserName`. Adding Firefox or a third
engine later is one line in `launch`. Cost: the harness, the make
variable and one CI row.

### 2. What changes together

A WebKit set of its own, as #380 first proposed: a probe runner in Python
with WebKitGTK, keys modelled as `beforeinput`, `execCommand` and `input`.
It changes on its own schedule, so a step fixed in one set is still broken
in the other, and its keys are not keys, so the WebKit faults that come
from real key presses, like the cursor left drawn, do not show. Every
walk would need writing twice. Playwright's own WebKit for Linux takes
real key presses, so the reason for a second harness is gone. Dropped.

### 3. Where state lives

A list of which walks are about editing, run in WebKit, and the rest in
Chromium only. The list is state with no writer to keep it true: the next
walk about a field is put in no list, the same failure that made the walks
run whole, see [BUILD.md](../BUILD.md#why-the-walks-run-whole). Kept from
it, for a while only: a list of the parts not walked in WebKit yet,
`walks/webkit.mjs`, which only shrinks and is removed with its last name,
and which `--names` checks against the folders.

### 4. What runs together, and what it costs

WebKit alone, the engine of the Mac, which ships first. Half the
machines. But Windows runs Chromium, and its walks would be gone before
Windows ships. `SLOW` and the coverage counts would go with Chromium. All
239 sequences would turn to WebKit at once, before any is known to pass
there, so main would be red until every part was fixed. Dropped for now.
Kept from it: WebKit is where the next faults are, so it is walked
first, part by part, starting with the report.

### 5. What a test has to build

The machine. WebKit and its libraries in CI's image beside Chromium, or a
second image for the WebKit walks. A second image is a second thing to
build, pin, cache and keep current, and the Linux jobs pull the image
anyway. One image costs its size on every pull, which is measured on the
first run and split if it slows the jobs that never walk. Kept: one image.

## What converges

Four of the five keep one set of walks and one harness, 1, 3, 4 and 5.
Three keep both browsers, 1, 3 and 5. What only one needs: a second
harness (2) is dropped, and so is a hand list of walks for each browser
(3). A temporary list of parts not yet walked in WebKit stays, because
turning every part at once (4) leaves main red.

What only a Mac does is no simulation's to solve. A cursor revealed in
the middle of a field, a button that takes no focus from a click and a
mouse entering as a plain move come from how AppKit reaches WebKit, and
no browser on Linux has them. A walk acts them out on purpose, as
`attach, as on a Mac` does, and the rule they protect is plain code with
a unit test.

## The decision

```
plain code      Go tests and Vitest, once: every rule said without a page
  │
walks           every walk and sequence, the same set, in both browsers
  │
harness         walks/bridge.mjs, browserName from WALKBROWSER
  │
Playwright      Chromium (Windows)        WebKit (Mac, Linux)
```

Every walk runs in Chromium and in WebKit on every pull request that
reaches the walks. Until a part has walked in WebKit it is named in
`walks/webkit.mjs`, and the random walks join last. CI walks WebKit on
machines of its own, one now, as many as the time limit of a job asks for
once every part walks there.

## What follows

- The WebKit walks take Linux machines, which CI has more of, and no Mac.
  The answer comes when the slowest job ends, so it should come no later.
- Each part that turns to WebKit is a pull request of its own, with the
  faults it finds fixed in it, so a red WebKit job names one part.
- It is wrong if WebKit on Linux passes what Tim then meets on his Mac
  with no Mac-only cause. Then the Mac's own WebKit, through the macOS
  job, is the next thing to weigh, at the cost of the scarce Mac machines.
