<!--
How a pull request is described, see CLAUDE.md. Plain words, short
sentences, no semicolons. Leave out a section that has nothing to say,
except How to test, which is always there. A pull request that adds
something rather than fixing it says in the opening sentences who asked
for what, and what it builds on under Root cause.
-->

<!--
No heading here. One or two sentences that open the description: who
saw the problem, where, what was wrong, and who tested what. They are
the summary, so no section below says what was wrong again. The plan
row goes in too. For example: "Plan row 2.186. On Tim's Mac, start.mp4
showed no picture and did not play, on Space or Play. Tim found it
testing #184, and the walk below proves the fix."
-->

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
