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
Windows ships. `SLOW` and the coverage counts would go with Chromium.
Turning all 239 sequences to WebKit at once is one pull request that
fixes everything WebKit finds, since its walks run before it merges. How
much that is was counted in #633, in one run of every walk in WebKit, at
c5f788b, a commit made for it and taken out again:

| part | sequences | failed or did not end in WebKit |
| --- | ---: | ---: |
| captions | 49 | 1 |
| report, fixed before | 43 | 0 |
| timeline | 39 | 4 |
| clips | 26 | 2 |
| playback | 24 | 2 |
| app | 23 | 0 |
| search | 22 | 1 |
| videos | 13 | 1 |
| random walks | 13 | 2 |

Eleven sequences of the 196 outside the report and two random walks, and
nine of the thirteen while a clip plays. Each is a fault to find, so all
at once is a pull request too large to review, and two walk jobs ran
past their seven minutes on walks that did not end. Dropped for now.
Kept from it: WebKit is where the next faults are, so it is walked
first, part by part, starting with the report and the app, which passed
whole.

### 5. What a test has to build

The machine. WebKit and its libraries in CI's image beside Chromium, or a
second image for the WebKit walks. A second image is a second thing to
build, pin, cache and keep current, and the Linux jobs pull the image
anyway. One image costs its size on every pull, which is measured on the
first run and split if it slows the jobs that never walk. Kept at first:
one image. Measured in #633, the browsers made Initialize containers take
73 to 80 s on every Linux job, where main took 23 to 30 s, so the image
became two, `base` for the Go jobs and `walks` with the browsers, both
built by `ci-image.yml` from one Dockerfile. Then it took 18 to 33 s.

## What converges

Four of the five keep one set of walks and one harness, 1, 3, 4 and 5.
A second harness (2) is dropped, and so is a hand list of walks for each
browser (3). A list of parts that stays only while they move over is
kept, because turning every part at once (4) is thirteen faults in one
pull request.

What only a Mac does is no simulation's to solve. A cursor revealed in
the middle of a field, a button that takes no focus from a click and a
mouse entering as a plain move come from how AppKit reaches WebKit, and
no browser on Linux has them. A walk acts them out on purpose, as
`attach, as on a Mac` does, and the rule they protect is plain code with
a unit test.

## Measured, and decided again

The first answer was every walk in both browsers, with WebKit on machines
of its own. The first runs, in #633, showed what that costs. These are
the walk jobs of one push, as GitHub shows them. Main is its last three
pushes, so its spread is the spread of those three. The pull request is
one run of 6809cf0, the report in WebKit on a fifth job:

| walk jobs | main, before | 6809cf0, after | change |
| --- | ---: | ---: | ---: |
| jobs | 4 | 5 | 25 % more |
| machine time (s) | 815 | 858 | 5 % more, ± 12 % |
| slowest job (s) | 264 | 203 | 23 % faster, ± 37 % |

Every part in both browsers would double the machine time and need about
four more jobs, or make each job twice as long. Tim, on 31 October: "What
I do not want to do is to slow down CI. What I do not want to do is to
launch even more jobs." So a pull request walks each walk in one browser,
WebKit, the fourth simulation. What that simulation dropped is kept at
night: the nightly walks walk in Chromium, so both engines are walked
every day and Windows is not left out. The list kept from the third
simulation turns round: it names the parts still in Chromium, so the
work stays the same at every step of the move.

The design chosen, one run of c869080 against the same three pushes of
main, the report in WebKit and the rest in Chromium in the same jobs:

| walk jobs | main, before | c869080, after | change |
| --- | ---: | ---: | ---: |
| jobs | 4 | 4 | 0 % more |
| machine time (s) | 815 | 927 | 14 % more, ± 12 % |
| slowest job (s) | 264 | 243 | 8 % faster, ± 37 % |

The machine time grew by a little more than main's own spread, from one
run. Part of it is the second browser each walker opens. Each pull
request that moves a part says what it costs in the same table.

One more way was weighed and dropped: Playwright's WebKit on the macOS
job, the engine nearest the app, with the Mac's own behaviour. It is a
Mac job more on every push, and the account runs five at once for every
branch together, see Keeping the tests quick in CLAUDE.md. It is the next
thing to weigh if WebKit on Linux passes what Tim then meets on his Mac.

## The decision

```
plain code      Go tests and Vitest, once: every rule said without a page
  │
walks           every walk and sequence, the same set
  │
harness         walks/bridge.mjs, browserFor: the part's browser, or WALKBROWSER
  │
Playwright      WebKit (Mac, Linux), every pull request
                Chromium (Windows), every night
```

Every walk runs in WebKit on every pull request that reaches the walks,
in the same four jobs as before. Until a part has moved to WebKit it is
named in `walks/webkit.mjs` and walks in Chromium, and the random walks
move last. The nightly walks walk everything in Chromium.

## What follows

- CI keeps its jobs. Its time grew by 14 % in the first run, see
  above, and each pull request that moves a part measures it again. A
  part that walks slower in WebKit shows in the shard times, and is fixed
  or split there.
- Each part that moves is a pull request of its own, with the faults it
  finds fixed in it, so each stays a size to review. Where the faults of
  several parts share a cause, like the ones while a clip plays, those
  parts move together. A walk that passes only
  in Chromium is a walk to fix, because the app on a Mac is WebKit.
- A fault only Chromium has is found by the night, a day late. Windows is
  not built yet, so that is the cheap side to be late on.
- `SLOW` and the walks' coverage need Chromium, so they run with
  `WALKBROWSER=chromium`, the coverage at night as before.
- It is wrong if WebKit on Linux passes what Tim then meets on his Mac
  with no Mac-only cause. Then the Mac's own WebKit, through the macOS
  job, is the next thing to weigh, at the cost of the scarce Mac machines.
- The nightly walks run with motion, `STILL=none`, and pull requests
  with reduced motion except for the steps `moving` names. Once every
  part is in WebKit, a fault in motion that only WebKit has is caught
  only by those steps. Tim chose Chromium at night, so it stays that way.
