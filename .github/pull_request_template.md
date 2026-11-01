<!--
How a pull request is described, the one place that says it. Plain words, short
sentences, no semicolons. The opening, Root cause and The fix are brief
and concrete: at most 30 words for each issue the pull request works on,
one short paragraph per issue. A pull request for three issues has three
paragraphs in each of the three. Proof may say as much as it needs. How
to test and Found along the way are always there. Leave out any other
section that has nothing to say.
-->

<!--
No heading here. For each issue, at most 30 words: what goes wrong or
what was asked, who met it and where, and what gets better. Then
`Fixes #N` for each issue this finishes, so merging closes it. For
example: "Fixes #245. On Tim's Mac, start.mp4 showed no picture and did
not play. Tim found it testing #184. Now it plays."
-->

## Root cause

<!--
For each issue, at most 30 words: why it happened, traced to the file
and the name in the code. Where it was met says whose fault it is: a
failure met only where a test runs is the test's or its environment's,
see section 6 of the testing skill. A pull request that adds something
says what it builds on instead.
-->

## The fix

<!--
For each issue, at most 30 words: what changes, with the files and the
names in the code. The docs that changed with it.
-->

## Proof

<!--
The tests that ensure it, and that they fail on main and pass here. The
walks, the measurements before and after, and whether CI passed on the
last commit. What could not be checked here, like WebKit on a Mac, is
said. This section may be as long as the proof needs.
-->

## How to test

<!--
A numbered list of steps Tim takes to see the change work, each one he
can take as it stands, in the app, in CI or wherever the change shows.
For the app, the first step is to update to this pull request from the
Updates page, then what to do, what to look at and what to verify. When
there is nothing to test, one line instead of the list: "Nothing to
test, because" and why, for example that only a test changed and CI
runs it.
-->

1. Update the app to this pull request from the Updates page.
2.

## Found along the way, not fixed here

<!-- Each with the issue it was logged as, or "Nothing". -->
