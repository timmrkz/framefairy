---
name: Issue
about: Something that went wrong, the cause behind several of them, or work that was asked for
---

<!--
Sessions open issues through the API, which shows no template, so a
session reads this file and writes the issue the way it lays it out.
Plain words, short sentences, no semicolons.

WHEN TO OPEN AN ISSUE, AND WHEN NOT

- Not for what the pull request you are on caused, or a small thing on
  the code it touches. Fix it there, as a commit of its own, see Clean
  up now in CLAUDE.md. An issue starts a whole new round of work.
- Not for what nobody can meet. Say who meets it: Tim on his Mac, a
  customer, or only a test. What only a test meets, on a CI runner, on
  a loaded machine or with a file made for the test, is an issue about
  the test, see section 6 of the testing skill. An order of events in
  our own code that a test can make happen, quitting in the middle of a
  delete for example, counts even if nobody has met it yet.
- Search the open and the closed issues first, by the failing test's
  name, the function the stack stopped in and the words of the error.
  The same failure as an open issue goes into that issue. The same
  cause as an earlier one is an incident under that cause's problem,
  see docs/TRIAGE.md.

THE THREE KINDS

- An incident: one time something went wrong, a bug Tim saw, a CI run
  that failed, a test that hung. Most issues are one.
- A problem: the cause behind two or more incidents. It is labelled
  `problem`, and its incidents are its sub-issues on GitHub. It closes
  last, after its last sub-issue, and is reopened when the cause comes
  back.
- Work: a feature or a change Tim asked for. Work that takes more than
  one pull request has a Progress list at the top, one line per pull
  request, ticked with its number as each merges.

Every issue carries a kind label, `bug`, `feature`, `tests`,
`reliability`, `performance` or `shipping`, one priority and one size,
as docs/WORKFLOW.md defines them. Leave out a section that has nothing
to say.
-->

<!--
No heading here. At most 30 words: what goes wrong or what is asked,
who met it or asked for it, and where. For example: "On Tim's Mac,
start.mp4 shows no picture after a restart. Tim met it testing #184,
report of 16 October 14.02."
-->

## Progress

<!-- Work over several pull requests only. One line per pull request. -->

- [ ]

## Seen

<!--
Where it was met, so anyone can find it again: Tim's report with its
time and build, the log lines that show it, a CI run with its link and
commit, or the review or the code it was read from. A problem lists its
incidents here instead, and every fix tried so far with why the cause
stayed. Work quotes what Tim asked, in his words, with the date.
-->

## Cause

<!--
At most 30 words: why it happens, traced to the file and the name in
the code, or "Not known yet" and what would find it, the trace first.
-->

## What to build

<!--
At most 30 words: the change that takes the cause away, in one place,
and when it is done. Never a workaround, see Root causes only in
docs/TRIAGE.md.
-->
