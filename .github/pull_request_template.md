<!--
How a pull request is described, see CLAUDE.md. Plain words, short
sentences, no semicolons. Leave out a section that has nothing to say,
except How to test, which is always there. A pull request that adds
something rather than fixing it says what was asked for under What was
wrong, and what it builds on under Root cause.
-->

<!--
One or two sentences of context: who saw the problem, where, and who
tested what. The plan row, like "Plan row 2.186. Tim saw it on his Mac
while testing #184."
-->

## What was wrong

<!-- A short summary of what a person saw, where, and how often. Numbers where there are any. -->

## Root cause

<!-- Why it happened, traced to the line that has it. -->

## The fix

<!-- What changes, with the files and the names in the code. The docs that changed with it. -->

## Proof

<!--
The tests that ensure it, and that they fail on main and pass here.
The walks, the measurements before and after, and whether make changed
passed. What could not be checked here, like WebKit on a Mac, is said.
-->

## How to test

<!--
Numbered steps to test it by hand, each one that can be taken as it
stands. The first is to update the app to this pull request from the
Updates page. Then what to do, what to look at and what to verify. A
change with nothing to see in the app says so, and says which test
proves it instead.
-->

1. Update the app to this pull request from the Updates page.
2.

## Found along the way, not fixed here

<!-- Each with the plan row it was logged as. -->
