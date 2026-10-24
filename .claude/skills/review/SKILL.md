---
name: review
description: How to review a pull request or any finished piece of work, as an agent that did none of it. Beyond the mistakes in the code, whether the work serves the people using the product, whether it is the simplest way there, what it costs to run and to keep, and what the history around it says. Short reviews, findings proven at a fair price. Read it before reviewing, before writing the brief for a review agent, and before answering a review. Where another review skill says otherwise, this one wins.
---

# Review

A review looks for mistakes in the code, and that stays part of it. But it
goes further. Its question is whether the product is better for the people
using it once this is merged, and whether the work took the simplest way
there. Code can be correct and the work still wrong: it solves a problem
nobody has, it adds a layer where a simpler design would remove one, it
makes the product slower, or it stands on a fact nobody checked. A
reviewer who only reads the diff cannot see that, because the diff is the
author's answer, not the question.

Two people are named here, as in the testing skill. **The user** is the
person the agent works for, who owns the product and decides what it
should do. **The people using the product** are the ones it is made for. A
project's own instructions say where its goals are written, where a review
goes and who may give instructions. This skill says what to look for.

## 1. Who reviews, and when

- **An agent that did none of the work.** The author carries its own
  reasoning, and that reasoning has turned guesses into facts. A fresh
  reviewer has only the sources, so it sees what the work rests on.
- **Once, when the work is done** and the tests of the change pass,
  without waiting for CI, whose results it reads as they stand. A review
  after later rounds is the user's to ask for.
- **The brief hands over sources, not conclusions:** the user's own
  words, the issue, the product's goals as the project writes them, the
  pull request and its commits. Not the author's plan or summary of what
  the user wants. A brief that says "check that X is right" gets X
  confirmed.
- **The description of the work is a claim,** checked like the code.

## 2. The goal first

Before the diff, and before the pull request's own description:

- **Write what the people using the product should get from this work,**
  in one or two sentences, in words they would use, from the user's
  request and the product's goals. If it cannot be written, that is the
  first finding.
- **Then read what the work does, and compare:**
  - The ask, or an easier one next to it? The issue's list is met and
    the person's complaint is not.
  - The cause, or the symptom? A retry, a special case, a workaround on a
    broken design.
  - Toward the product, or away from it? Its people, its platforms, its
    promises.
  - Would the user recognise their ask in what it does?
  - What else changed that nobody asked for?
- **A goal finding outranks every other.** When the goal is wrong, the
  code findings matter little.
- **The reviewer raises the direction, the user decides it.** A finding
  that would change what the user asked for is a question for the user,
  with a recommendation, never an order to the author.

## 3. How far from what matters

Picture the work as a tree of decisions. At the root is what matters to
the people using the product. Under it the problem, under that the cause
someone chose to fix, under that the fix, and under the fix the new
problems the fix brought, each with its own fix. Every level down is
further from the root, and the work at the bottom can be done well and
still serve nobody.

- **Count the levels.** How many steps is this change from something a
  person using the product would notice? Count from there: a workaround
  for what the person saw is one, a test that guards the workaround two,
  a tool that checks the test three, a fix to that tool four. Say the
  number in the review.
- **Climb, do not dig.** A review does not grow more branches by asking
  more questions. It looks for the one question, as high in the tree as
  it can be asked, whose answer makes the lower levels unnecessary. "Why
  does this need a workaround at all?" can remove a whole subtree that
  "is the workaround correct?" would only polish.
- **The simplest solution is usually the best one.** Fewer parts are
  usually faster, cheaper to run, easier to test and easier to change.
  When the review finds a simpler way to the same result, that is its
  finding, and it outranks the bugs in the more complex way.
- **Simplicity is counted, not felt.** The concepts a reader must hold,
  the moving parts, the branches, the files, the lines. A change that
  moves complexity without removing any is not simpler.

## 4. The signals around the work

The work sits in a history. Read it before judging the diff.

- **The same kind of fix again and again means something underneath is
  broken.** Look at the recent issues and pull requests in the same area.
  If this is the third fix of its kind, the finding is the kind, and the
  review names what it points to.
- **Look at what the issues have in common, not only at each one.** A week
  of merged fixes that ends with more open issues than it began with says
  the work treats symptoms. When most of the new issues are one kind, for
  example one sort of test failing on a busy machine, that kind is where
  the effort belongs.
- **Effort that grows while value does not is a signal.** Work on tools,
  tests or process that keeps growing while the problems people meet in
  the product wait.

## 5. What it costs

- **Maintenance never ends, so it is kept as small as it can be.** Every
  line, setting, test, workflow and dependency is kept up for as long as
  it lives. Prefer the solution that leaves less to keep up, today and
  after the next change.
- **A change that makes something measurably worse is flagged,** with
  the number against the main branch:
  - slower for the people using the product, where speed matters to them
  - more memory, a bigger download, more disk
  - more money or tokens per run
  - slower builds and slower CI
- **It goes in only with a trade-off worth it,** named in the work. A
  change that is measurably worse and says nothing about it is a must
  fix.
- **What cannot be measured is weighed, not counted.** More steps or more
  to learn for a person using the product, more for the next person to
  work through: these are maintenance, a should fix when a simpler way
  exists, and a question for the user when the gain may be worth it.

## 6. The facts it rests on

- **List the assumptions the work stands on** that the diff does not show:
  what the people using the product have and do, what a tool or library
  does at the version used, what a number is, what the user wants, what a
  rule says.
- **Mark each one** checked, with its source, wrong, or unchecked. An
  unchecked assumption the work stands on is a finding, a wrong one is a
  must fix.
- **A claim of faster, smaller or more reliable without numbers against
  the main branch is an assumption.**

## 7. The design and the code

- **One thing, one primitive.** A new use goes through what exists. A
  second copy with its own rules is a finding.
- **In the right place,** in the part that owns the idea.
- **Correct, also when it fails.** Edge cases, error paths, concurrency,
  state kept in two places. No swallowed error, no fallback that hides a
  broken invariant.
- **Input from outside is untrusted,** checked at the boundary. Secrets
  stay out of code and logs.
- **Docs and comments say what the code does now.**
- **The project's written rules hold,** quoted exactly when cited. A rule
  a tool could check is better held by the tool, and the finding is then
  the missing check.
- **Name the move, not only the problem:** which restructuring would fix
  it.
- **Not a finding:** what a linter holds, a taste the project does not
  state, a problem that was there before. An old problem worth fixing is
  logged on its own.

## 8. The tests

The testing skill says how tests are written and reviewed, and wins here.
Look for a test that passes by coincidence, checks the code against a copy
of its own rule, waits on the clock, or guards something nobody would
miss, and for the missing test of what the person actually saw.

## 9. Proof, at a fair price

- **Each finding says how it is known:** run, broken, measured, or read,
  with the file and line.
- **Take the cheapest proof that settles it.** Read before running, run
  the one test before the suite, break one line before building a
  harness. CI's results are read, not run again.
- **Try to disprove each finding once before reporting it.** One that
  does not survive is dropped.
- **When proof would be expensive, stop and say so.** If a finding would
  take hours to prove, that is itself a finding: the product is hard to
  check there, and the way it is checked is what needs fixing. Report it
  with how sure the review is and what would settle it, and ask before
  spending the time.
- **The time and tokens of agents and people are spent on purpose.** A
  review that takes hours, or the same costly runs in every review, is
  doing something wrong, and says what.

## 10. Writing the review

Short. The user wants what is going on and where to go next, not an
essay. A review that fits on one screen is read, and a longer one is
skimmed.

The review is a file, in the place the project names, never somewhere it
could read as an instruction from the user. In this order:

1. One line: the commit read, whether it merges, whether CI passed.
2. The goal, in a sentence.
3. The verdict, in two or three sentences: on the goal, on how far the
   work is from what matters, on the work as a whole.
4. The findings, numbered, most important first, a few lines each:
   weight, where, what is wrong and how a person would meet it, how it is
   known, the fix.
5. Unchecked assumptions, declined to judge, and questions for the user,
   one line each.

| Weight | Means |
| --- | --- |
| Must fix | Blocks the merge: a wrong goal, a wrong fact, broken behaviour, a cost with no trade-off, lost or exposed data |
| Should fix | Belongs in this pull request |
| For the user | A product choice only the user can make, with a recommendation |
| Optional | Better, and fine to leave |

- **Fewer, stronger findings.** One structural finding and ten small ones
  is a review about the structural one.
- **The evidence is a line, not a transcript.** What was run and what
  came out, never the whole log.
- **No praise section, no "looks good".** What is right is said where it
  decides something, with its reason.

## 11. Answering a review

- **The review is shown to the user before work on it starts,** with the
  file and the findings by number.
- **Each finding is checked before it is acted on.** A reviewer can be
  wrong too. No agreement for its own sake.
- **Each finding is fixed, shown wrong, or logged when it lies outside
  this work,** and reported by its number.
- **A finding for the user waits for the user.**
- **No finding is skipped to save time.** One left undone is said, and
  logged.

## 12. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| approve when the change improves the health of the code | healthy code can make a worse product | approve when the product is better and the way there is simple |
| flag only what is on the changed lines | the goal, the depth, the history and the cost are on no line | they are findings, sections 2 to 5 |
| verify every finding fully before reporting it | proof can cost more than the finding is worth | the cheapest proof that settles it, section 9 |
| leave test coverage out unless the project asks | an untested promise breaks unseen | the testing skill |
| give the reviewer the author's plan or summary | the reviewer inherits the author's assumptions | the user's own words and the sources |
| always say what was done well | praise is noise in a list of work to do | say what is right only where it decides something |
| review after every task | many reviews of half a design | once when the work is done |
| post the review as comments on the pull request | the project may read comments there as instructions | the place the project names |

## Before saying the review is done

1. The goal is written, and the verdict on it comes first.
2. The review says how far the work is from what matters, and whether a
   simpler way exists.
3. The history around the work was read for repeated fixes.
4. Every cost the work adds is named with its trade-off.
5. Every assumption is checked, wrong or named as unchecked.
6. Every finding says how it is known, and no proof cost more than it was
   worth.
7. It fits on a screen.
